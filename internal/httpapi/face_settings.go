package httpapi

import (
	"net/http"

	"github.com/hwyc888/FaceSign/internal/domain"
	"github.com/hwyc888/FaceSign/internal/face"
)

type faceSettingsResponse struct {
	Provider           string  `json:"provider"`
	ServiceURL         string  `json:"service_url"`
	APIKeyConfigured   bool    `json:"api_key_configured"`
	Similarity         float64 `json:"similarity"`
	DetectionThreshold float64 `json:"detection_threshold"`
}

func faceSettingsForResponse(settings domain.FaceSettings) faceSettingsResponse {
	return faceSettingsResponse{
		Provider:           settings.Provider,
		ServiceURL:         settings.ServiceURL,
		APIKeyConfigured:   settings.APIKey != "",
		Similarity:         settings.Similarity,
		DetectionThreshold: settings.DetectionThreshold,
	}
}

func (s *Server) handleGetFaceSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, faceSettingsForResponse(s.faces.Settings()))
}

func (s *Server) handleUpdateFaceSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider           string  `json:"provider"`
		ServiceURL         string  `json:"service_url"`
		APIKey             string  `json:"api_key"`
		Similarity         float64 `json:"similarity"`
		DetectionThreshold float64 `json:"detection_threshold"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	current := s.faces.Settings()
	apiKey := req.APIKey
	if apiKey == "" {
		apiKey = current.APIKey
	}
	settings, err := face.ValidateSettings(domain.FaceSettings{
		Provider:           req.Provider,
		ServiceURL:         req.ServiceURL,
		APIKey:             apiKey,
		Similarity:         req.Similarity,
		DetectionThreshold: req.DetectionThreshold,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if settings.Provider == "compreface" && settings.APIKey == "" {
		writeError(w, http.StatusBadRequest, "启用 CompreFace 前必须填写 API Key")
		return
	}
	if err := s.repo.SaveFaceSettings(r.Context(), settings); err != nil {
		writeError(w, http.StatusInternalServerError, "保存人脸识别设置失败")
		return
	}
	if err := s.faces.Configure(settings); err != nil {
		writeError(w, http.StatusInternalServerError, "应用人脸识别设置失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, faceSettingsForResponse(settings))
}

func (s *Server) handleCheckFaceService(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.faces.Status(r.Context()))
}
