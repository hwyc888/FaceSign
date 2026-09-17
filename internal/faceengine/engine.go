package faceengine

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	onnxface "github.com/leandroveronezi/go-onnxface"
	"github.com/leandroveronezi/go-onnxface/sface"
	"github.com/leandroveronezi/go-onnxface/yunet"
	_ "modernc.org/sqlite"
)

var (
	ErrNoFace        = errors.New("no face detected")
	ErrMultipleFaces = errors.New("multiple faces detected")
)

type Sample struct {
	ID        string
	Subject   string
	Feature   []float32
	CreatedAt int64
}

type SubjectMatch struct {
	Subject    string  `json:"subject"`
	Similarity float64 `json:"similarity"`
}

type FaceResult struct {
	Subjects []SubjectMatch `json:"subjects"`
}

type Health struct {
	OK          bool   `json:"ok"`
	Engine      string `json:"engine"`
	Device      string `json:"device"`
	GPURequired bool   `json:"gpu_required"`
	Runtime     string `json:"runtime"`
	Subjects    int    `json:"subjects"`
	Samples     int    `json:"samples"`
}

type Engine struct {
	mu         sync.Mutex
	detector   *yunet.Detector
	recognizer *sface.Recognizer
	db         *sql.DB
	samples    []Sample
}

func New(runtimePath, detectorModel, recognizerModel, dataPath string) (*Engine, error) {
	for label, path := range map[string]string{
		"ONNX Runtime": runtimePath,
		"YuNet model":  detectorModel,
		"SFace model":  recognizerModel,
	} {
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			return nil, fmt.Errorf("%s not found: %s", label, path)
		}
	}
	if strings.TrimSpace(dataPath) == "" {
		return nil, fmt.Errorf("face data path cannot be empty")
	}
	if err := os.MkdirAll(filepath.Dir(dataPath), 0o700); err != nil {
		return nil, fmt.Errorf("create face data directory: %w", err)
	}

	if err := onnxface.InitEnvironment(runtimePath); err != nil {
		return nil, fmt.Errorf("initialize ONNX Runtime: %w", err)
	}

	detector, err := yunet.NewDetector(detectorModel)
	if err != nil {
		_ = onnxface.CloseEnvironment()
		return nil, fmt.Errorf("load YuNet model: %w", err)
	}
	recognizer, err := sface.NewRecognizer(recognizerModel)
	if err != nil {
		detector.Close()
		_ = onnxface.CloseEnvironment()
		return nil, fmt.Errorf("load SFace model: %w", err)
	}

	db, err := sql.Open("sqlite", dataPath)
	if err != nil {
		recognizer.Close()
		detector.Close()
		_ = onnxface.CloseEnvironment()
		return nil, fmt.Errorf("open face database: %w", err)
	}
	db.SetMaxOpenConns(1)

	e := &Engine{
		detector:   detector,
		recognizer: recognizer,
		db:         db,
		samples:    make([]Sample, 0),
	}
	if err := e.initDatabase(context.Background()); err != nil {
		e.Close()
		return nil, err
	}
	if err := e.loadSamples(context.Background()); err != nil {
		e.Close()
		return nil, err
	}
	return e, nil
}

func (e *Engine) Close() {
	if e.db != nil {
		_ = e.db.Close()
	}
	if e.recognizer != nil {
		e.recognizer.Close()
	}
	if e.detector != nil {
		e.detector.Close()
	}
	_ = onnxface.CloseEnvironment()
}

func (e *Engine) Health() Health {
	e.mu.Lock()
	defer e.mu.Unlock()
	seen := make(map[string]struct{})
	for _, sample := range e.samples {
		seen[sample.Subject] = struct{}{}
	}
	return Health{
		OK:          true,
		Engine:      "facesign-nativecpu",
		Device:      "cpu",
		GPURequired: false,
		Runtime:     onnxface.Version(),
		Subjects:    len(seen),
		Samples:     len(e.samples),
	}
}

func (e *Engine) Subjects() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	seen := make(map[string]struct{})
	for _, sample := range e.samples {
		seen[sample.Subject] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for subject := range seen {
		out = append(out, subject)
	}
	sort.Strings(out)
	return out
}

func (e *Engine) Enroll(subject string, img image.Image, detectionThreshold float64) (string, error) {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return "", fmt.Errorf("subject cannot be empty")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	faces, err := e.detectLocked(img, detectionThreshold)
	if err != nil {
		return "", err
	}
	if len(faces) == 0 {
		return "", ErrNoFace
	}
	if len(faces) > 1 {
		return "", ErrMultipleFaces
	}
	aligned := e.recognizer.Align(img, faces[0].Landmarks)
	feature, err := e.recognizer.Feature(aligned)
	if err != nil {
		return "", fmt.Errorf("extract face feature: %w", err)
	}
	id, err := randomID()
	if err != nil {
		return "", err
	}
	createdAt := time.Now().Unix()
	if _, err := e.db.Exec(
		"INSERT INTO face_embeddings(id, subject, embedding, created_at) VALUES(?,?,?,?)",
		id, subject, encodeFeature(feature), createdAt,
	); err != nil {
		return "", fmt.Errorf("save face sample: %w", err)
	}
	e.samples = append(e.samples, Sample{
		ID:        id,
		Subject:   subject,
		Feature:   feature,
		CreatedAt: createdAt,
	})
	return id, nil
}

func (e *Engine) Recognize(img image.Image, detectionThreshold float64) ([]FaceResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	faces, err := e.detectLocked(img, detectionThreshold)
	if err != nil {
		return nil, err
	}
	results := make([]FaceResult, 0, len(faces))
	for _, detected := range faces {
		aligned := e.recognizer.Align(img, detected.Landmarks)
		feature, err := e.recognizer.Feature(aligned)
		if err != nil {
			return nil, fmt.Errorf("extract face feature: %w", err)
		}

		bestSubject := ""
		bestCosine := -1.0
		for _, sample := range e.samples {
			cosine := onnxface.Match(feature, sample.Feature, onnxface.DistanceCosine)
			if cosine > bestCosine {
				bestCosine = cosine
				bestSubject = sample.Subject
			}
		}

		result := FaceResult{Subjects: make([]SubjectMatch, 0, 1)}
		if bestSubject != "" {
			// Preserve the previous local engine's 0..1 scale so existing
			// FaceSign similarity thresholds remain compatible.
			similarity := math.Max(0, math.Min(1, (bestCosine+1)/2))
			result.Subjects = append(result.Subjects, SubjectMatch{
				Subject:    bestSubject,
				Similarity: similarity,
			})
		}
		results = append(results, result)
	}
	return results, nil
}

func (e *Engine) detectLocked(img image.Image, threshold float64) ([]onnxface.Face, error) {
	if img == nil || img.Bounds().Dx() < 40 || img.Bounds().Dy() < 40 {
		return nil, ErrNoFace
	}
	if threshold <= 0 {
		threshold = 0.80
	}
	if threshold < 0.10 {
		threshold = 0.10
	}
	if threshold > 0.99 {
		threshold = 0.99
	}
	e.detector.ScoreThreshold = float32(threshold)
	faces, err := e.detector.Detect(img)
	if err != nil {
		return nil, fmt.Errorf("detect face: %w", err)
	}
	return faces, nil
}

func (e *Engine) initDatabase(ctx context.Context) error {
	statements := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=10000",
		`CREATE TABLE IF NOT EXISTS face_embeddings (
			id TEXT PRIMARY KEY,
			subject TEXT NOT NULL,
			embedding BLOB NOT NULL,
			created_at INTEGER NOT NULL
		)`,
		"CREATE INDEX IF NOT EXISTS idx_face_embeddings_subject ON face_embeddings(subject)",
	}
	for _, statement := range statements {
		if _, err := e.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize face database: %w", err)
		}
	}
	return nil
}

func (e *Engine) loadSamples(ctx context.Context) error {
	rows, err := e.db.QueryContext(ctx, "SELECT id, subject, embedding, created_at FROM face_embeddings ORDER BY created_at")
	if err != nil {
		return fmt.Errorf("read face samples: %w", err)
	}
	defer rows.Close()

	samples := make([]Sample, 0)
	for rows.Next() {
		var sample Sample
		var raw []byte
		if err := rows.Scan(&sample.ID, &sample.Subject, &raw, &sample.CreatedAt); err != nil {
			return fmt.Errorf("scan face sample: %w", err)
		}
		feature, err := decodeFeature(raw)
		if err != nil {
			return fmt.Errorf("decode face sample %s: %w", sample.ID, err)
		}
		sample.Feature = feature
		samples = append(samples, sample)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read face samples: %w", err)
	}
	e.samples = samples
	return nil
}

func encodeFeature(feature []float32) []byte {
	raw := make([]byte, len(feature)*4)
	for i, value := range feature {
		binary.LittleEndian.PutUint32(raw[i*4:], math.Float32bits(value))
	}
	return raw
}

func decodeFeature(raw []byte) ([]float32, error) {
	if len(raw) == 0 || len(raw)%4 != 0 {
		return nil, fmt.Errorf("invalid embedding length %d", len(raw))
	}
	feature := make([]float32, len(raw)/4)
	for i := range feature {
		feature[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
	}
	return feature, nil
}

func randomID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate face sample id: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}
