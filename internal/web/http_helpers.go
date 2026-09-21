package web

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

	"github.com/hwyc888/FaceSign/internal/face"
)

func readImage(w http.ResponseWriter, r *http.Request) (image.Image, error) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		return nil, fmt.Errorf("invalid multipart body: %w", err)
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		return nil, errors.New("image file is required")
	}
	defer file.Close()
	return decodeImage(file)
}

func decodeImage(file multipart.File) (image.Image, error) {
	img, _, err := image.Decode(io.LimitReader(file, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	return img, nil
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid json: %w", err)
	}
	return nil
}

func writeFaceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, face.ErrNoFace):
		writeError(w, http.StatusUnprocessableEntity, errors.New("未检测到清晰人脸，请正对摄像头并靠近一些"))
	case errors.Is(err, face.ErrMultipleFaces):
		writeError(w, http.StatusUnprocessableEntity, errors.New("录入时只能有一张明显人脸，请确保镜头前只有一人"))
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
}

