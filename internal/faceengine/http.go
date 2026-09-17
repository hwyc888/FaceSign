package faceengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
)

const maxImageBytes = 12 << 20

type HTTPServer struct {
	engine *Engine
}

func NewHTTPHandler(engine *Engine) http.Handler {
	s := &HTTPServer{engine: engine}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /api/v1/recognition/subjects/", s.handleSubjects)
	mux.HandleFunc("POST /api/v1/recognition/faces/", s.handleEnroll)
	mux.HandleFunc("POST /api/v1/recognition/recognize", s.handleRecognize)
	return mux
}

func (s *HTTPServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.engine.Health())
}

func (s *HTTPServer) handleSubjects(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"subjects": s.engine.Subjects()})
}

func (s *HTTPServer) handleEnroll(w http.ResponseWriter, r *http.Request) {
	subject := strings.TrimSpace(r.URL.Query().Get("subject"))
	if subject == "" {
		writeProblem(w, http.StatusBadRequest, "subject 不能为空")
		return
	}
	threshold, ok := parseThreshold(w, r)
	if !ok {
		return
	}
	img, ok := readMultipartImage(w, r)
	if !ok {
		return
	}
	id, err := s.engine.Enroll(subject, img, threshold)
	if err != nil {
		switch {
		case errors.Is(err, ErrNoFace):
			writeProblem(w, http.StatusUnprocessableEntity, "未检测到清晰正脸，请调整光线和角度后重试")
		case errors.Is(err, ErrMultipleFaces):
			writeProblem(w, http.StatusConflict, "画面中检测到多张人脸，请只保留一名学生")
		default:
			writeProblem(w, http.StatusInternalServerError, "人脸样本保存失败: "+err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"image_id": id, "subject": subject})
}

func (s *HTTPServer) handleRecognize(w http.ResponseWriter, r *http.Request) {
	threshold, ok := parseThreshold(w, r)
	if !ok {
		return
	}
	img, ok := readMultipartImage(w, r)
	if !ok {
		return
	}
	results, err := s.engine.Recognize(img, threshold)
	if err != nil {
		if errors.Is(err, ErrNoFace) {
			writeJSON(w, http.StatusOK, map[string]any{"result": []FaceResult{}})
			return
		}
		writeProblem(w, http.StatusInternalServerError, "人脸识别失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": results})
}

func parseThreshold(w http.ResponseWriter, r *http.Request) (float64, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("det_prob_threshold"))
	if raw == "" {
		return 0.80, true
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value < 0 || value > 1 {
		writeProblem(w, http.StatusBadRequest, "det_prob_threshold 必须在 0 到 1 之间")
		return 0, false
	}
	return value, true
}

func readMultipartImage(w http.ResponseWriter, r *http.Request) (image.Image, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImageBytes+(1<<20))
	if err := r.ParseMultipartForm(maxImageBytes); err != nil {
		writeProblem(w, http.StatusBadRequest, "无法读取上传图片")
		return nil, false
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "缺少图片文件")
		return nil, false
	}
	defer file.Close()
	if header.Size > maxImageBytes {
		writeProblem(w, http.StatusRequestEntityTooLarge, "图片过大")
		return nil, false
	}
	img, err := decodeImageLimited(file)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "无法解析图片")
		return nil, false
	}
	return img, true
}

func decodeImageLimited(file multipart.File) (image.Image, error) {
	limited := io.LimitReader(file, maxImageBytes+1)
	img, _, err := image.Decode(limited)
	if err != nil {
		return nil, err
	}
	if img.Bounds().Dx() <= 0 || img.Bounds().Dy() <= 0 {
		return nil, fmt.Errorf("empty image")
	}
	return img, nil
}

func writeProblem(w http.ResponseWriter, status int, detail string) {
	writeJSONStatus(w, status, map[string]string{"detail": detail})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	writeJSONStatus(w, status, value)
}

func writeJSONStatus(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
