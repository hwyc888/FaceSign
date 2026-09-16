package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
)

const maxJSONBody = 1 << 20
const maxImageBody = 6 << 20

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "请求数据格式错误: "+err.Error())
		return false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "请求体只能包含一个 JSON 对象")
		return false
	}
	return true
}

func parsePathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "无效的编号")
		return 0, false
	}
	return id, true
}

func readMultipartImage(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImageBody)
	if err := r.ParseMultipartForm(maxImageBody); err != nil {
		return nil, fmt.Errorf("图片上传失败: %w", err)
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return nil, fmt.Errorf("缺少人脸图片")
	}
	defer file.Close()
	if !allowedImageType(header) {
		return nil, fmt.Errorf("仅支持 JPEG 或 PNG 图片")
	}
	data, err := io.ReadAll(io.LimitReader(file, 5<<20+1))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("图片为空")
	}
	if len(data) > 5<<20 {
		return nil, fmt.Errorf("图片不能超过 5MB")
	}
	return data, nil
}

func allowedImageType(header *multipart.FileHeader) bool {
	contentType := strings.ToLower(header.Header.Get("Content-Type"))
	return contentType == "image/jpeg" || contentType == "image/png" || contentType == "image/jpg" || contentType == ""
}
