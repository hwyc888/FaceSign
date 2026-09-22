package web

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hwyc888/FaceSign/internal/store"
)

const cameraAgentOnlineWindow = 15 * time.Second

type cameraAgentFrame struct {
	JPEG []byte
	Width int
	Height int
	LastSeen time.Time
}

func hashCameraAgentSecret(secret string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(secret)))
	return hex.EncodeToString(sum[:])
}

func (s *Server) cameraAgentFrameUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost { methodNotAllowed(w); return }
	agentID := strings.TrimSpace(r.Header.Get("X-FaceSign-Agent-ID"))
	if agentID == "" { writeError(w, http.StatusUnauthorized, errors.New("缺少Agent ID")); return }
	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(authorization, "Bearer ") { writeError(w, http.StatusUnauthorized, errors.New("缺少Agent连接密钥")); return }
	secret := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
	camera, err := s.store.CameraByAgentID(r.Context(), agentID)
	if err != nil || camera.Kind != "agent" || camera.AgentSecretHash == "" {
		writeError(w, http.StatusUnauthorized, errors.New("Agent未在FaceSign中注册")); return
	}
	actualHash := hashCameraAgentSecret(secret)
	if subtle.ConstantTimeCompare([]byte(actualHash), []byte(camera.AgentSecretHash)) != 1 {
		writeError(w, http.StatusUnauthorized, errors.New("Agent连接密钥不正确")); return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	if err != nil || len(data) == 0 { writeError(w, http.StatusBadRequest, errors.New("Agent没有上传有效图像")); return }
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil { writeError(w, http.StatusBadRequest, errors.New("Agent上传的不是有效JPEG图像")); return }
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 90}); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("Agent图像编码失败")); return
	}
	bounds := img.Bounds()
	state := cameraAgentFrame{JPEG: append([]byte(nil), out.Bytes()...), Width: bounds.Dx(), Height: bounds.Dy(), LastSeen: time.Now()}
	s.cameraAgentMu.Lock()
	s.cameraAgentFrames[agentID] = state
	s.cameraAgentMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "camera_id": camera.ID, "width": state.Width, "height": state.Height})
}

func (s *Server) latestCameraAgentFrame(agentID string) ([]byte, int, int, error) {
	s.cameraAgentMu.RLock()
	state, ok := s.cameraAgentFrames[agentID]
	s.cameraAgentMu.RUnlock()
	if !ok { return nil, 0, 0, errors.New("客户端Camera Agent尚未上传画面") }
	if time.Since(state.LastSeen) > cameraAgentOnlineWindow { return nil, 0, 0, errors.New("客户端Camera Agent已离线或画面已过期") }
	return append([]byte(nil), state.JPEG...), state.Width, state.Height, nil
}

func (s *Server) decorateCameraAgentState(camera *store.Camera) {
	if camera == nil || camera.Kind != "agent" || camera.AgentID == "" { return }
	s.cameraAgentMu.RLock()
	state, ok := s.cameraAgentFrames[camera.AgentID]
	s.cameraAgentMu.RUnlock()
	if !ok { return }
	camera.AgentLastSeen = state.LastSeen.Format(time.RFC3339)
	camera.AgentOnline = time.Since(state.LastSeen) <= cameraAgentOnlineWindow
}
