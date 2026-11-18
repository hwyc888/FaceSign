package liveness

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hwyc888/FaceSign/internal/inference"
	ort "github.com/yalue/onnxruntime_go"
	xdraw "golang.org/x/image/draw"
)

const (
	inputSize = 128
	silentInputSize = 80
)

type modelKind uint8

const (
	modelMN3 modelKind = iota
	modelMiniFASV2SE
	modelSilentV2
	modelSilentV1SE
)

func modelKindForPath(path string) modelKind {
	switch strings.ToLower(filepath.Base(path)) {
	case "minifasnet-v2se.onnx":
		return modelMiniFASV2SE
	case "minifasnetv2.onnx":
		return modelSilentV2
	case "minifasnetv1se.onnx":
		return modelSilentV1SE
	default:
		return modelMN3
	}
}

func (e *Engine) ModelName() string {
	if e == nil || len(e.pools) == 0 { return "未加载" }
	switch e.kind {
	case modelSilentV2:
		if len(e.pools) >= 2 { return "MiniFASNetV2 + V1SE" }
		return "MiniFASNetV2"
	case modelSilentV1SE:
		return "MiniFASNetV1SE"
	case modelMiniFASV2SE:
		return "MiniFASNetV2-SE"
	default:
		return "anti-spoof-mn3"
	}
}

var ErrInvalidFace = errors.New("invalid face crop")

type engineWorker struct {
	input *ort.Tensor[float32]
	output *ort.Tensor[float32]
	session *ort.AdvancedSession
	kind modelKind
	scale float64
}

type modelPool struct {
	kind modelKind
	scale float64
	workers []*engineWorker
	available chan *engineWorker
}

type Engine struct {
	kind modelKind
	mu sync.RWMutex
	pools []*modelPool
	closed bool
}

const engineWorkerCount = inference.MaxConcurrent

type modelLoadSpec struct {
	path string
	kind modelKind
	scale float64
}

func New(modelPath string) (*Engine, error) {
	info, err := os.Stat(modelPath)
	if err != nil || info.IsDir() { return nil, fmt.Errorf("anti-spoof model not found: %s", modelPath) }
	if !ort.IsInitialized() { return nil, errors.New("ONNX Runtime environment is not initialized") }

	kind := modelKindForPath(modelPath)
	specs := []modelLoadSpec{{path: modelPath, kind: kind}}
	if kind == modelSilentV2 {
		v1sePath := filepath.Join(filepath.Dir(modelPath), "MiniFASNetV1SE.onnx")
		if info, err := os.Stat(v1sePath); err != nil || info.IsDir() {
			return nil, fmt.Errorf("Silent-Face ensemble model not found: %s", v1sePath)
		}
		specs[0].scale = 2.7
		specs = append(specs, modelLoadSpec{path: v1sePath, kind: modelSilentV1SE, scale: 4.0})
	}

	engine := &Engine{kind: kind}
	for _, spec := range specs {
		pool, err := newModelPool(spec)
		if err != nil {
			engine.Close()
			return nil, fmt.Errorf("load anti-spoof model %s: %w", filepath.Base(spec.path), err)
		}
		engine.pools = append(engine.pools, pool)
	}
	return engine, nil
}

func newModelPool(spec modelLoadSpec) (*modelPool, error) {
	pool := &modelPool{kind: spec.kind, scale: spec.scale, available: make(chan *engineWorker, engineWorkerCount)}
	for i := 0; i < engineWorkerCount; i++ {
		worker, err := newEngineWorker(spec)
		if err != nil { pool.close(); return nil, fmt.Errorf("worker %d: %w", i+1, err) }
		pool.workers = append(pool.workers, worker)
		pool.available <- worker
	}
	return pool, nil
}

func newEngineWorker(spec modelLoadSpec) (*engineWorker, error) {
	inputDim, outputDim := inputSize, 2
	inputName, outputName := "actual_input_1", "output1"
	if spec.kind == modelMiniFASV2SE {
		inputName, outputName = "input", "output"
	}
	if spec.kind == modelSilentV2 || spec.kind == modelSilentV1SE {
		inputDim, outputDim = silentInputSize, 3
		inputName, outputName = "input", "output"
	}
	input, err := ort.NewEmptyTensor[float32](ort.NewShape(1, 3, int64(inputDim), int64(inputDim)))
	if err != nil { return nil, fmt.Errorf("create anti-spoof input tensor: %w", err) }
	output, err := ort.NewEmptyTensor[float32](ort.NewShape(1, int64(outputDim)))
	if err != nil { input.Destroy(); return nil, fmt.Errorf("create anti-spoof output tensor: %w", err) }
	options, err := ort.NewSessionOptions()
	if err != nil { output.Destroy(); input.Destroy(); return nil, fmt.Errorf("create anti-spoof session options: %w", err) }
	if err := options.SetIntraOpNumThreads(1); err != nil { _=options.Destroy(); output.Destroy(); input.Destroy(); return nil, fmt.Errorf("configure anti-spoof intra-op threads: %w", err) }
	if err := options.SetInterOpNumThreads(1); err != nil { _=options.Destroy(); output.Destroy(); input.Destroy(); return nil, fmt.Errorf("configure anti-spoof inter-op threads: %w", err) }
	session, err := ort.NewAdvancedSession(spec.path, []string{inputName}, []string{outputName}, []ort.Value{input}, []ort.Value{output}, options)
	_ = options.Destroy()
	if err != nil { output.Destroy(); input.Destroy(); return nil, fmt.Errorf("load anti-spoof model: %w", err) }
	return &engineWorker{input: input, output: output, session: session, kind: spec.kind, scale: spec.scale}, nil
}

func (p *modelPool) acquire(ctx context.Context) (*engineWorker, func(), error) {
	select {
	case worker := <-p.available:
		return worker, func(){ p.available <- worker }, nil
	case <-ctx.Done():
		return nil, nil, fmt.Errorf("wait for anti-spoof worker: %w", ctx.Err())
	}
}

func (p *modelPool) close() {
	if p == nil { return }
	for _, worker := range p.workers { worker.close() }
	p.workers = nil
}

func (w *engineWorker) close() {
	if w == nil { return }
	if w.session != nil { _=w.session.Destroy(); w.session=nil }
	if w.output != nil { _=w.output.Destroy(); w.output=nil }
	if w.input != nil { _=w.input.Destroy(); w.input=nil }
}

func (e *Engine) Close() {
	if e == nil { return }
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed { return }
	e.closed = true
	for _, pool := range e.pools { pool.close() }
	e.pools = nil
}

func (e *Engine) Score(img image.Image, faceRect image.Rectangle) (float64, error) {
	if img == nil || faceRect.Empty() || faceRect.Intersect(img.Bounds()).Empty() { return 0, ErrInvalidFace }
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	e.mu.RLock()
	if e.closed || len(e.pools)==0 { e.mu.RUnlock(); return 0, errors.New("anti-spoof engine is closed") }
	defer e.mu.RUnlock()
	releaseInference, err := inference.AcquireContext(ctx)
	if err != nil { return 0, fmt.Errorf("wait for CPU inference slot: %w", err) }
	defer releaseInference()

	var silentLiveSum float64
	var silentCount int
	for _, pool := range e.pools {
		worker, release, err := pool.acquire(ctx)
		if err != nil { return 0, err }
		if worker.kind == modelSilentV2 || worker.kind == modelSilentV1SE {
			crop, err := silentFaceCrop(img, faceRect, worker.scale)
			if err != nil { release(); return 0, err }
			fillSilentInput(worker.input.GetData(), crop)
		} else if worker.kind == modelMiniFASV2SE {
			fillMiniFASInput(worker.input.GetData(), resizeMiniFASFaceCrop(img, faceRect))
		} else {
			crop := expandedFaceRect(faceRect, img.Bounds())
			if crop.Empty() { release(); return 0, ErrInvalidFace }
			fillInput(worker.input.GetData(), resizeFaceCrop(img, crop))
		}
		if err := worker.session.Run(); err != nil { release(); return 0, fmt.Errorf("run anti-spoof model: %w", err) }
		data := worker.output.GetData()
		if worker.kind == modelSilentV2 || worker.kind == modelSilentV1SE {
			if len(data)<3 { release(); return 0, errors.New("Silent-Face model returned invalid output") }
			probs := softmax3(data)
			silentLiveSum += probs[1]
			silentCount++
			release()
			continue
		}
		if len(data)<2 { release(); return 0, errors.New("anti-spoof model returned invalid output") }
		score := realProbability(float64(data[0]), float64(data[1]))
		release()
		return score, nil
	}
	if silentCount==0 { return 0, errors.New("no Silent-Face model was executed") }
	return silentLiveSum/float64(silentCount), nil
}

func silentFaceCrop(img image.Image, faceRect image.Rectangle, scale float64) (*image.RGBA, error) {
	bounds := img.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	boxW, boxH := faceRect.Dx(), faceRect.Dy()
	if boxW<=0 || boxH<=0 || srcW<=1 || srcH<=1 { return nil, ErrInvalidFace }
	s := math.Min(float64(srcH-1)/float64(boxH), math.Min(float64(srcW-1)/float64(boxW), scale))
	if s<=0 { return nil, ErrInvalidFace }
	x := float64(faceRect.Min.X-bounds.Min.X)
	y := float64(faceRect.Min.Y-bounds.Min.Y)
	centerX, centerY := float64(boxW)/2+x, float64(boxH)/2+y
	newW,newH := float64(boxW)*s,float64(boxH)*s
	left,top := centerX-newW/2,centerY-newH/2
	right,bottom := centerX+newW/2,centerY+newH/2
	if left<0 { right-=left; left=0 }
	if top<0 { bottom-=top; top=0 }
	if right>float64(srcW-1) { left-=right-float64(srcW-1); right=float64(srcW-1) }
	if bottom>float64(srcH-1) { top-=bottom-float64(srcH-1); bottom=float64(srcH-1) }
	x1,y1,x2,y2 := int(left),int(top),int(right),int(bottom)
	if x2<x1 || y2<y1 { return nil,ErrInvalidFace }
	srcRect := image.Rect(x1+bounds.Min.X,y1+bounds.Min.Y,x2+bounds.Min.X+1,y2+bounds.Min.Y+1)
	out:=image.NewRGBA(image.Rect(0,0,silentInputSize,silentInputSize))
	xdraw.BiLinear.Scale(out,out.Bounds(),img,srcRect,xdraw.Src,nil)
	return out,nil
}

func fillSilentInput(dst []float32, img *image.RGBA) {
	const plane=silentInputSize*silentInputSize
	for y:=0;y<silentInputSize;y++ {
		for x:=0;x<silentInputSize;x++ {
			px:=img.RGBAAt(x,y); i:=y*silentInputSize+x
			dst[i]=float32(px.B); dst[plane+i]=float32(px.G); dst[2*plane+i]=float32(px.R)
		}
	}
}

func softmax3(values []float32) [3]float64 {
	maxValue:=float64(values[0])
	for i:=1;i<3;i++ { if float64(values[i])>maxValue { maxValue=float64(values[i]) } }
	var expSum float64
	var probs [3]float64
	for i:=0;i<3;i++ { probs[i]=math.Exp(float64(values[i])-maxValue); expSum+=probs[i] }
	if expSum<=0 || math.IsNaN(expSum) { return [3]float64{} }
	for i:=0;i<3;i++ { probs[i]/=expSum }
	return probs
}

func resizeMiniFASFaceCrop(img image.Image, faceRect image.Rectangle) *image.RGBA {
	bounds:=img.Bounds()
	side:=int(math.Max(float64(faceRect.Dx()),float64(faceRect.Dy()))*1.5); if side<1 { side=1 }
	cx:=float64(faceRect.Min.X+faceRect.Max.X)/2; cy:=float64(faceRect.Min.Y+faceRect.Max.Y)/2
	x0:=int(math.Floor(cx-float64(side)/2)); y0:=int(math.Floor(cy-float64(side)/2))
	crop:=image.NewRGBA(image.Rect(0,0,side,side))
	for y:=0;y<side;y++ { sy:=bounds.Min.Y+reflect101(y0+y-bounds.Min.Y,bounds.Dy()); for x:=0;x<side;x++ { sx:=bounds.Min.X+reflect101(x0+x-bounds.Min.X,bounds.Dx()); r,g,b,_:=img.At(sx,sy).RGBA(); crop.SetRGBA(x,y,color.RGBA{R:uint8(r>>8),G:uint8(g>>8),B:uint8(b>>8),A:255}) } }
	out:=image.NewRGBA(image.Rect(0,0,inputSize,inputSize)); xdraw.CatmullRom.Scale(out,out.Bounds(),crop,crop.Bounds(),xdraw.Src,nil); return out
}

func reflect101(pos,n int) int { if n<=1{return 0}; for pos<0 || pos>=n { if pos<0 {pos=-pos} else {pos=2*n-2-pos} }; return pos }

func fillMiniFASInput(dst []float32,img *image.RGBA) {
	const plane=inputSize*inputSize
	for y:=0;y<inputSize;y++ { for x:=0;x<inputSize;x++ { px:=img.RGBAAt(x,y); i:=y*inputSize+x; dst[i]=float32(px.R)/255; dst[plane+i]=float32(px.G)/255; dst[2*plane+i]=float32(px.B)/255 } }
}

func resizeFaceCrop(img image.Image,crop image.Rectangle)*image.RGBA { out:=image.NewRGBA(image.Rect(0,0,inputSize,inputSize)); xdraw.BiLinear.Scale(out,out.Bounds(),img,crop,xdraw.Src,nil); return out }

func expandedFaceRect(rect,bounds image.Rectangle) image.Rectangle {
	if rect.Empty()||bounds.Empty(){return image.Rectangle{}}
	cx:=float64(rect.Min.X+rect.Max.X)/2; cy:=float64(rect.Min.Y+rect.Max.Y)/2; w:=float64(rect.Dx())*1.10; h:=float64(rect.Dy())*1.05
	return image.Rect(int(math.Round(cx-w/2)),int(math.Round(cy-h/2)),int(math.Round(cx+w/2)),int(math.Round(cy+h/2))).Intersect(bounds)
}

func fillInput(dst []float32,img image.Image) {
	const plane=inputSize*inputSize
	means:=[3]float64{151.2405,119.5950,107.8395}; scales:=[3]float64{63.0105,56.4570,55.0035}
	for y:=0;y<inputSize;y++ { for x:=0;x<inputSize;x++ { r16,g16,b16,_:=img.At(x,y).RGBA(); r,g,b:=float64(r16)/257.0,float64(g16)/257.0,float64(b16)/257.0; i:=y*inputSize+x; dst[i]=float32((r-means[0])/scales[0]); dst[plane+i]=float32((g-means[1])/scales[1]); dst[2*plane+i]=float32((b-means[2])/scales[2]) } }
}

func realProbability(realValue,spoofValue float64)float64 {
	if realValue>=0&&spoofValue>=0 { sum:=realValue+spoofValue; if sum>0&&math.Abs(sum-1)<0.05{return clamp01(realValue/sum)} }
	maxValue:=math.Max(realValue,spoofValue); realExp:=math.Exp(realValue-maxValue); spoofExp:=math.Exp(spoofValue-maxValue); return clamp01(realExp/(realExp+spoofExp))
}
func clamp01(value float64)float64 { if value<0{return 0};if value>1{return 1};return value }
