package face

import (
	"errors"
	"fmt"
	"image"
	"math"
	"os"
	"sort"
	"sync"

	"github.com/hwyc888/FaceSign/internal/inference"
	onnxface "github.com/leandroveronezi/go-onnxface"
	ort "github.com/yalue/onnxruntime_go"
)

const (
	byDetectorSize = 640
	byRecognizerSize = 112
	byEmbeddingSize = 512
)

type byRuntime struct {
	mu sync.Mutex
	detector *ort.DynamicAdvancedSession
	detectorInput string
	detectorOutputs []string
	recognizer *ort.DynamicAdvancedSession
	recognizerInput string
	recognizerOutput string
	input *ort.Tensor[float32]
}

type byCandidate struct {
	detection Detection
	score float32
}

// NewBy creates the SCRFD-10GF + ArcFace ResNet50 inference engine.
// Model files are supplied locally; this function never downloads or redistributes
// third-party pretrained weights.
func NewBy(runtimePath, detectorModel, recognizerModel string) (*Engine, error) {
	for label, path := range map[string]string{
		"ONNX Runtime": runtimePath,
		"SCRFD model": detectorModel,
		"ArcFace ResNet50 model": recognizerModel,
	} {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			return nil, fmt.Errorf("%s not found: %s", label, path)
		}
	}
	if err := onnxface.InitEnvironment(runtimePath); err != nil {
		return nil, fmt.Errorf("initialize ONNX Runtime: %w", err)
	}
	fail := func(err error) (*Engine, error) {
		_ = onnxface.CloseEnvironment()
		return nil, err
	}

	detectorInputs, detectorOutputs, err := ort.GetInputOutputInfo(detectorModel)
	if err != nil {
		return fail(fmt.Errorf("inspect SCRFD model: %w", err))
	}
	if len(detectorInputs) != 1 || len(detectorOutputs) != 9 {
		return fail(fmt.Errorf("unsupported SCRFD graph: expected one input and nine outputs, got %d inputs and %d outputs", len(detectorInputs), len(detectorOutputs)))
	}
	detector, err := ort.NewDynamicAdvancedSession(detectorModel, []string{detectorInputs[0].Name}, outputNames(detectorOutputs), nil)
	if err != nil {
		return fail(fmt.Errorf("load SCRFD model: %w", err))
	}

	recognizerInputs, recognizerOutputs, err := ort.GetInputOutputInfo(recognizerModel)
	if err != nil {
		_ = detector.Destroy()
		return fail(fmt.Errorf("inspect ArcFace model: %w", err))
	}
	if len(recognizerInputs) != 1 || len(recognizerOutputs) != 1 {
		_ = detector.Destroy()
		return fail(fmt.Errorf("unsupported ArcFace graph: expected one input and one output, got %d inputs and %d outputs", len(recognizerInputs), len(recognizerOutputs)))
	}
	recognizer, err := ort.NewDynamicAdvancedSession(recognizerModel, []string{recognizerInputs[0].Name}, []string{recognizerOutputs[0].Name}, nil)
	if err != nil {
		_ = detector.Destroy()
		return fail(fmt.Errorf("load ArcFace model: %w", err))
	}

	return &Engine{backend: "by", by: &byRuntime{
		detector: detector,
		detectorInput: detectorInputs[0].Name,
		detectorOutputs: outputNames(detectorOutputs),
		recognizer: recognizer,
		recognizerInput: recognizerInputs[0].Name,
		recognizerOutput: recognizerOutputs[0].Name,
	}}, nil
}

func outputNames(values []ort.InputOutputInfo) []string {
	names := make([]string, len(values))
	for i := range values {
		names[i] = values[i].Name
	}
	return names
}

func (e *Engine) closeBy() {
	if e == nil || e.by == nil {
		return
	}
	e.by.mu.Lock()
	defer e.by.mu.Unlock()
	if e.by.detector != nil {
		_ = e.by.detector.Destroy()
		e.by.detector = nil
	}
	if e.by.recognizer != nil {
		_ = e.by.recognizer.Destroy()
		e.by.recognizer = nil
	}
	_ = onnxface.CloseEnvironment()
}

func (e *Engine) detectSCRFD(img image.Image, threshold float64) ([]Detection, error) {
	if img == nil || img.Bounds().Dx() < 40 || img.Bounds().Dy() < 40 {
		return nil, ErrNoFace
	}
	if threshold <= 0 {
		threshold = 0.50
	}
	threshold = math.Max(0.05, math.Min(0.99, threshold))

	e.by.mu.Lock()
	defer e.by.mu.Unlock()
	if e.by.detector == nil {
		return nil, errors.New("SCRFD detector is closed")
	}
	releaseInference := inference.Acquire()
	defer releaseInference()

	inputData, scale, err := scrfdInput(img)
	if err != nil {
		return nil, err
	}
	input, err := ort.NewTensor[float32](ort.NewShape(1, 3, byDetectorSize, byDetectorSize), inputData)
	if err != nil {
		return nil, fmt.Errorf("create SCRFD input tensor: %w", err)
	}
	defer input.Destroy()

	outputs := make([]ort.Value, len(e.by.detectorOutputs))
	if err := e.by.detector.Run([]ort.Value{input}, outputs); err != nil {
		destroyValues(outputs)
		return nil, fmt.Errorf("run SCRFD detector: %w", err)
	}
	defer destroyValues(outputs)

	tensors := make([]*ort.Tensor[float32], len(outputs))
	for i, value := range outputs {
		tensor, ok := value.(*ort.Tensor[float32])
		if !ok {
			return nil, fmt.Errorf("SCRFD output %d is not float32", i)
		}
		tensors[i] = tensor
	}
	return decodeSCRFD(tensors, scale, img.Bounds(), float32(threshold)), nil
}

func destroyValues(values []ort.Value) {
	for _, value := range values {
		if value != nil {
			_ = value.Destroy()
		}
	}
}

func scrfdInput(img image.Image) ([]float32, float64, error) {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil, 0, ErrNoFace
	}
	scale := math.Min(float64(byDetectorSize)/float64(width), float64(byDetectorSize)/float64(height))
	newWidth := int(math.Round(float64(width) * scale))
	newHeight := int(math.Round(float64(height) * scale))
	if newWidth < 1 || newHeight < 1 {
		return nil, 0, ErrNoFace
	}
	if newWidth > byDetectorSize {
		newWidth = byDetectorSize
	}
	if newHeight > byDetectorSize {
		newHeight = byDetectorSize
	}
	const plane = byDetectorSize * byDetectorSize
	data := make([]float32, 3*plane)
	for y := 0; y < newHeight; y++ {
		for x := 0; x < newWidth; x++ {
			sx := float64(bounds.Min.X) + (float64(x)+0.5)/scale - 0.5
			sy := float64(bounds.Min.Y) + (float64(y)+0.5)/scale - 0.5
			r, g, b := sampleRGB(img, sx, sy)
			i := y*byDetectorSize + x
			data[i] = (r - 127.5) / 128.0
			data[plane+i] = (g - 127.5) / 128.0
			data[2*plane+i] = (b - 127.5) / 128.0
		}
	}
	return data, scale, nil
}

func decodeSCRFD(outputs []*ort.Tensor[float32], scale float64, bounds image.Rectangle, threshold float32) []Detection {
	data := make([][]float32, len(outputs))
	for i := range outputs {
		data[i] = outputs[i].GetData()
	}
	return decodeSCRFDData(data, scale, bounds, threshold)
}

func decodeSCRFDData(outputs [][]float32, scale float64, bounds image.Rectangle, threshold float32) []Detection {
	strides := []int{8, 16, 32}
	candidates := make([]byCandidate, 0, 64)
	for level, stride := range strides {
		gridWidth := byDetectorSize / stride
		gridHeight := byDetectorSize / stride
		anchors := gridWidth * gridHeight * 2
		scores := outputs[level]
		boxes := outputs[level+3]
		landmarks := outputs[level+6]
		if len(scores) < anchors || len(boxes) < anchors*4 || len(landmarks) < anchors*10 {
			continue
		}
		for index := 0; index < anchors; index++ {
			score := scores[index]
			if score < threshold {
				continue
			}
			cell := index / 2
			gridX := cell % gridWidth
			gridY := cell / gridWidth
			cx := float64(gridX * stride)
			cy := float64(gridY * stride)
			offset := index * 4
			x1 := int(math.Round((cx - float64(boxes[offset])*float64(stride)) / scale)) + bounds.Min.X
			y1 := int(math.Round((cy - float64(boxes[offset+1])*float64(stride)) / scale)) + bounds.Min.Y
			x2 := int(math.Round((cx + float64(boxes[offset+2])*float64(stride)) / scale)) + bounds.Min.X
			y2 := int(math.Round((cy + float64(boxes[offset+3])*float64(stride)) / scale)) + bounds.Min.Y
			rect := image.Rect(x1, y1, x2, y2).Intersect(bounds)
			if rect.Empty() || rect.Dx() < 8 || rect.Dy() < 8 {
				continue
			}
			points := [5]image.Point{}
			koff := index * 10
			for point := 0; point < 5; point++ {
				px := (cx + float64(landmarks[koff+point*2])*float64(stride)) / scale
				py := (cy + float64(landmarks[koff+point*2+1])*float64(stride)) / scale
				points[point] = image.Pt(int(math.Round(px))+bounds.Min.X, int(math.Round(py))+bounds.Min.Y)
			}
			candidates = append(candidates, byCandidate{
				detection: Detection{Rectangle: rect, Landmarks: points, Score: score},
				score: score,
			})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	kept := make([]Detection, 0, len(candidates))
	for _, candidate := range candidates {
		keep := true
		for _, prior := range kept {
			if rectIoU(candidate.detection.Rectangle, prior.Rectangle) > 0.40 {
				keep = false
				break
			}
		}
		if keep {
			kept = append(kept, candidate.detection)
			if len(kept) >= 100 {
				break
			}
		}
	}
	return kept
}

func rectIoU(a, b image.Rectangle) float64 {
	intersection := a.Intersect(b)
	if intersection.Empty() {
		return 0
	}
	area := float64(intersection.Dx() * intersection.Dy())
	union := float64(a.Dx()*a.Dy()+b.Dx()*b.Dy()) - area
	if union <= 0 {
		return 0
	}
	return area / union
}

func (e *Engine) featureArcFace(img image.Image, detected Detection) ([]float32, error) {
	if img == nil || detected.Rectangle.Empty() {
		return nil, ErrNoFace
	}
	e.by.mu.Lock()
	defer e.by.mu.Unlock()
	if e.by.recognizer == nil {
		return nil, errors.New("ArcFace recognizer is closed")
	}
	releaseInference := inference.Acquire()
	defer releaseInference()

	aligned, err := alignArcFace112(img, detected.Landmarks)
	if err != nil {
		return nil, err
	}
	input, err := ort.NewTensor[float32](ort.NewShape(1, 3, byRecognizerSize, byRecognizerSize), aligned)
	if err != nil {
		return nil, fmt.Errorf("create ArcFace input tensor: %w", err)
	}
	defer input.Destroy()
	outputs := []ort.Value{nil}
	if err := e.by.recognizer.Run([]ort.Value{input}, outputs); err != nil {
		destroyValues(outputs)
		return nil, fmt.Errorf("run ArcFace recognizer: %w", err)
	}
	defer destroyValues(outputs)
	tensor, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, errors.New("ArcFace output is not float32")
	}
	feature := append([]float32(nil), tensor.GetData()...)
	if len(feature) != byEmbeddingSize {
		return nil, fmt.Errorf("ArcFace returned %d values; expected %d", len(feature), byEmbeddingSize)
	}
	normalizeFeature(feature)
	return feature, nil
}

func normalizeFeature(feature []float32) {
	var norm float64
	for _, value := range feature {
		norm += float64(value) * float64(value)
	}
	if norm <= 0 {
		return
	}
	inv := float32(1 / math.Sqrt(norm))
	for i := range feature {
		feature[i] *= inv
	}
}

func alignArcFace112(img image.Image, landmarks [5]image.Point) ([]float32, error) {
	template := [5][2]float64{
		{38.2946, 51.6963},
		{73.5318, 51.5014},
		{56.0252, 71.7366},
		{41.5493, 92.3655},
		{70.7299, 92.2041},
	}
	var normal [4][4]float64
	var rhs [4]float64
	for i, point := range landmarks {
		x, y := float64(point.X), float64(point.Y)
		tx, ty := template[i][0], template[i][1]
		rows := [][5]float64{{x, -y, 1, 0, tx}, {y, x, 0, 1, ty}}
		for _, row := range rows {
			for a := 0; a < 4; a++ {
				rhs[a] += row[a] * row[4]
				for b := 0; b < 4; b++ {
					normal[a][b] += row[a] * row[b]
				}
			}
		}
	}
	params, ok := solve4(normal, rhs)
	if !ok {
		return nil, errors.New("cannot estimate ArcFace alignment transform")
	}
	a, b, tx, ty := params[0], params[1], params[2], params[3]
	det := a*a + b*b
	if det < 1e-12 {
		return nil, errors.New("invalid ArcFace alignment transform")
	}
	const plane = byRecognizerSize * byRecognizerSize
	data := make([]float32, 3*plane)
	for y := 0; y < byRecognizerSize; y++ {
		for x := 0; x < byRecognizerSize; x++ {
			dx, dy := float64(x)-tx, float64(y)-ty
			sx := (a*dx + b*dy) / det
			sy := (-b*dx + a*dy) / det
			r, g, blue := sampleRGB(img, sx, sy)
			index := y*byRecognizerSize + x
			data[index] = (r - 127.5) / 127.5
			data[plane+index] = (g - 127.5) / 127.5
			data[2*plane+index] = (blue - 127.5) / 127.5
		}
	}
	return data, nil
}

func solve4(matrix [4][4]float64, vector [4]float64) ([4]float64, bool) {
	for col := 0; col < 4; col++ {
		pivot := col
		for row := col + 1; row < 4; row++ {
			if math.Abs(matrix[row][col]) > math.Abs(matrix[pivot][col]) {
				pivot = row
			}
		}
		if math.Abs(matrix[pivot][col]) < 1e-12 {
			return [4]float64{}, false
		}
		matrix[col], matrix[pivot] = matrix[pivot], matrix[col]
		vector[col], vector[pivot] = vector[pivot], vector[col]
		divisor := matrix[col][col]
		for j := col; j < 4; j++ {
			matrix[col][j] /= divisor
		}
		vector[col] /= divisor
		for row := 0; row < 4; row++ {
			if row == col {
				continue
			}
			factor := matrix[row][col]
			for j := col; j < 4; j++ {
				matrix[row][j] -= factor * matrix[col][j]
			}
			vector[row] -= factor * vector[col]
		}
	}
	return vector, true
}

func sampleRGB(img image.Image, x, y float64) (float32, float32, float32) {
	bounds := img.Bounds()
	if x < float64(bounds.Min.X) || y < float64(bounds.Min.Y) ||
		x > float64(bounds.Max.X-1) || y > float64(bounds.Max.Y-1) {
		return 0, 0, 0
	}
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	x1, y1 := x0+1, y0+1
	if x1 >= bounds.Max.X { x1 = x0 }
	if y1 >= bounds.Max.Y { y1 = y0 }
	fx, fy := x-float64(x0), y-float64(y0)
	r00,g00,b00 := rgbAt(img,x0,y0)
	r10,g10,b10 := rgbAt(img,x1,y0)
	r01,g01,b01 := rgbAt(img,x0,y1)
	r11,g11,b11 := rgbAt(img,x1,y1)
	blend := func(a,b,c,d float32) float32 {
		top := a*(1-float32(fx))+b*float32(fx)
		bottom := c*(1-float32(fx))+d*float32(fx)
		return top*(1-float32(fy))+bottom*float32(fy)
	}
	return blend(r00,r10,r01,r11), blend(g00,g10,g01,g11), blend(b00,b10,b01,b11)
}

func rgbAt(img image.Image, x,y int) (float32,float32,float32) {
	r,g,b,_ := img.At(x,y).RGBA()
	return float32(r)/257.0, float32(g)/257.0, float32(b)/257.0
}
