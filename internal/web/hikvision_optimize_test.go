package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hwyc888/FaceSign/internal/store"
)

func TestOptimizeHikvisionStreamXML(t *testing.T) {
	input := []byte(`<?xml version="1.0"?><StreamingChannel><Video><videoCodecType>H.265</videoCodecType><videoResolutionWidth>1920</videoResolutionWidth><videoResolutionHeight>1080</videoResolutionHeight><maxFrameRate>3000</maxFrameRate><videoQualityControlType>VBR</videoQualityControlType><GovLength>50</GovLength></Video></StreamingChannel>`)
	updated, fps, gop, changed, err := optimizeHikvisionStreamXML(input)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || fps != 25 || gop != 25 {
		t.Fatalf("unexpected optimization: changed=%v fps=%d gop=%d", changed, fps, gop)
	}
	text := string(updated)
	for _, want := range []string{"<videoCodecType>H.264</videoCodecType>", "<maxFrameRate>2500</maxFrameRate>", "<GovLength>25</GovLength>", "<videoResolutionWidth>1920</videoResolutionWidth>", "<videoQualityControlType>VBR</videoQualityControlType>"} {
		if !strings.Contains(text, want) {
			t.Fatalf("optimized XML missing %s: %s", want, text)
		}
	}
}

func TestHikvisionH264OptimizationEndpointWritesAndReadsBack(t *testing.T) {
	var mu sync.Mutex
	streamXML := `<?xml version="1.0"?><StreamingChannel><Video><videoCodecType>H.265</videoCodecType><maxFrameRate>3000</maxFrameRate><GovLength>50</GovLength><videoResolutionWidth>1280</videoResolutionWidth></Video></StreamingChannel>`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ISAPI/Streaming/channels/101" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprint(w, streamXML)
		case http.MethodPut:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			streamXML = string(body)
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprint(w, `<ResponseStatus><statusCode>1</statusCode></ResponseStatus>`)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer upstream.Close()

	st, err := store.Open(filepath.Join(t.TempDir(), "hikvision-optimize.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	camera, err := st.CreateCamera(context.Background(), store.CameraInput{
		Name: "Hikvision", Kind: "network", Protocol: "rtsp",
		StreamURL: "rtsp://127.0.0.1:554/Streaming/channels/101",
		SnapshotURL: upstream.URL + "/ISAPI/Streaming/channels/1/picture",
		AuthMode: "none", Width: 1280, Height: 720, FPS: 5, TimeoutMS: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}

	s := &Server{store: st, networkCameraFrames: make(map[int64]*networkCameraFrameCache)}
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/cameras/%d/optimize-h264", camera.ID), nil)
	rec := httptest.NewRecorder()
	s.cameraAction(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"codec":"H.264"`, `"fps":25`, `"gop":25`, `"verified":true`} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %s: %s", want, body)
		}
	}
	mu.Lock()
	finalXML := streamXML
	mu.Unlock()
	if !strings.Contains(finalXML, "<videoCodecType>H.264</videoCodecType>") ||
		!strings.Contains(finalXML, "<maxFrameRate>2500</maxFrameRate>") ||
		!strings.Contains(finalXML, "<GovLength>25</GovLength>") {
		t.Fatalf("camera config was not optimized: %s", finalXML)
	}
}
