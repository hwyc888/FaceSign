package web

import (
	"strings"
	"testing"
)

func TestFrontendModulesAreEmbedded(t *testing.T) {
	modules := []string{
		"assets/js/core.js",
		"assets/js/navigation.js",
		"assets/js/camera.js",
		"assets/js/cameras.js",
		"assets/js/seating.js",
		"assets/js/recognition.js",
		"assets/js/settings.js",
		"assets/js/enrollment.js",
		"assets/js/classes.js",
		"assets/js/seat_editor.js",
		"assets/js/students.js",
		"assets/js/photo_import.js",
		"assets/js/attendance.js",
		"assets/js/boot.js",
	}
	for _, path := range modules {
		data, err := assets.ReadFile(path)
		if err != nil { t.Fatalf("read %s: %v", path, err) }
		if len(data) == 0 { t.Fatalf("%s is empty", path) }
	}
}

func TestClassActionSelectorsUseQuerySelectorAll(t *testing.T) {
	data, err := assets.ReadFile("assets/js/classes.js")
	if err != nil { t.Fatal(err) }
	script := string(data)
	for _, selector := range []string{"up", "down", "edit", "delete"} {
		want := "$$('[data-class-" + selector + "]').forEach"
		if !strings.Contains(script, want) {
			t.Fatalf("class action selector %q must use querySelectorAll before forEach", selector)
		}
	}
}


func TestEnrollmentWorkbenchKeepsCameraVisible(t *testing.T) {
	data, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	if !strings.Contains(html, `id="samplePanel"`) {
		t.Fatal("student enrollment must include the side sample panel")
	}
	if !strings.Contains(html, `class="capture-workbench"`) {
		t.Fatal("student enrollment must use the camera-first capture workbench")
	}
	if strings.Contains(html, `id="samplesModal"`) {
		t.Fatal("face samples must not use a modal that covers the camera")
	}
}


func TestPhotoImportUI(t *testing.T) {
	data, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, want := range []string{`id="photoImportFile"`, `id="photoImportPreview"`, `/js/photo_import.js`} {
		if !strings.Contains(html, want) {
			t.Fatalf("photo import UI missing %s", want)
		}
	}
}


func TestSeatBoardUI(t *testing.T) {
	data, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, want := range []string{`id="checkinSeatBoard"`, `id="checkinSeatClass"`, `/js/seating.js`, `/js/seat_editor.js`, `id="classSeatRows"`, `id="classSeatsPerRow"`, `id="classLateAfter"`, `id="classDeadline"`, `id="classSeatEditor"`, `id="seatEditorBoard"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("seat board UI missing %s", want)
		}
	}
}


func TestSeatBoardStatusFilters(t *testing.T) {
	data, err := assets.ReadFile("assets/js/seating.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	for _, want := range []string{"data-seat-filter", "signed", "late", "waiting", "absent", "empty", "filtered-out"} {
		if !strings.Contains(script, want) {
			t.Fatalf("seat status filter missing %q", want)
		}
	}
}

func TestSeatEditorSupportsMoveAndSwap(t *testing.T) {
	data, err := assets.ReadFile("assets/js/seat_editor.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	for _, want := range []string{"openClassSeatEditor", "target_seat_no", "交换座位", "未编座位"} {
		if !strings.Contains(script, want) {
			t.Fatalf("seat editor missing %q", want)
		}
	}
}


func TestStudentListSelectorsUseQuerySelectorAll(t *testing.T) {
	data, err := assets.ReadFile("assets/js/students.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	for _, selector := range []string{"data-student-seat", "data-faces"} {
		good := "$$('[" + selector + "]').forEach"
		if !strings.Contains(script, good) {
			t.Fatalf("%s querySelectorAll binding is missing", selector)
		}
	}
	for _, line := range strings.Split(script, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "$(") && strings.Contains(line, ").forEach") {
			t.Fatalf("single-element selector cannot be iterated with forEach: %s", line)
		}
	}
}

func TestSeatEditorCollectionSelectorsUseQuerySelectorAll(t *testing.T) {
	data, err := assets.ReadFile("assets/js/seat_editor.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	for _, want := range []string{
		"$$('[data-editor-seat]').forEach",
		"$$('[data-editor-student]').forEach",
		"$$('[data-editor-student-id]').forEach",
		"$$('.seat-editor-cell').forEach",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("seat editor collection selector must use querySelectorAll: %s", want)
		}
	}
	for _, line := range strings.Split(script, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "$(") && strings.Contains(line, ").forEach") {
			t.Fatalf("single-element selector cannot be iterated with forEach: %s", line)
		}
	}
}

func TestSeatEditorSupportsNativeDragDrop(t *testing.T) {
	data, err := assets.ReadFile("assets/js/seat_editor.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	for _, want := range []string{
		`draggable="true"`,
		"ondragstart",
		"ondragover",
		"ondrop",
		"dataTransfer.setData",
		"confirmSwap: false",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("seat editor drag/drop missing %q", want)
		}
	}
}


func TestCameraSettingsUI(t *testing.T) {
	data, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, want := range []string{
		`id="cameraForm"`,
		`id="cameraDevice"`,
		`id="cameraProtocol"`,
		`id="cameraSnapshotURL"`,
		`id="cameraStreamURL"`,
		`id="camerasBody"`,
		`id="cameraAgentID"`,
		`id="cameraAgentSecret"`,
		`id="generateCameraAgentSecret"`,
		`/js/cameras.js`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("camera settings UI missing %s", want)
		}
	}
}

func TestCameraFrontendSupportsLocalAndNetworkSources(t *testing.T) {
	data, err := assets.ReadFile("assets/js/cameras.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	for _, want := range []string{
		"enumerateDevices",
		"deviceId",
		"http_snapshot",
		"mjpeg",
		"rtsp",
		"agent_id",
		"Camera Agent",
		"/api/cameras/",
		"Digest",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("camera frontend missing %q", want)
		}
	}
}


func TestNetworkCameraPreviewAutoReconnects(t *testing.T) {
	data, err := assets.ReadFile("assets/js/camera.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	for _, want := range []string{
		"networkPreviewRetryTimer",
		"networkPreviewGeneration",
		"image.onerror",
		"setTimeout(() =>",
		"/stream?t=",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("network camera preview reconnect logic missing %q", want)
		}
	}
}

func TestEnrollmentFaceGuideCanBeMoved(t *testing.T) {
	htmlData, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(htmlData)
	for _, want := range []string{
		`id="enrollFaceGuide"`,
		`tabindex="0"`,
		`Home键恢复居中`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("movable enrollment guide UI missing %s", want)
		}
	}

	jsData, err := assets.ReadFile("assets/js/enrollment.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(jsData)
	for _, want := range []string{
		"pointerdown",
		"pointermove",
		"setPointerCapture",
		"pointerup",
		"dblclick",
		"ArrowLeft",
		"ArrowRight",
		"ArrowUp",
		"ArrowDown",
		"resetEnrollmentFaceGuide",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("movable enrollment guide logic missing %q", want)
		}
	}

	cssData, err := assets.ReadFile("assets/style.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(cssData)
	for _, want := range []string{
		".face-guide.dragging",
		"pointer-events:auto",
		"touch-action:none",
		"cursor:grab",
	} {
		if !strings.Contains(css, want) {
			t.Fatalf("movable enrollment guide style missing %q", want)
		}
	}
}


func TestDuplicateEnrollmentSupportsProfileUpdateReview(t *testing.T) {
	htmlData, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(htmlData)
	for _, want := range []string{
		`id="duplicateUpdateProfile"`,
		`id="duplicateUpdateModal"`,
		`id="duplicateUpdateForm"`,
		`id="duplicateDiffModal"`,
		`id="duplicateDiffList"`,
		`id="duplicateConfirmUpdate"`,
		`id="duplicateKeepOriginal"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("duplicate profile update UI missing %s", want)
		}
	}

	jsData, err := assets.ReadFile("assets/js/enrollment.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(jsData)
	for _, want := range []string{
		"duplicateProfileChanges",
		"资料与原记录一致，无需更新",
		"跨班自动清空",
		"pendingDuplicateProfileUpdate",
		"method: 'PUT'",
		"确认并保存",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("duplicate profile update flow missing %q", want)
		}
	}
}


func TestSeatEditorVisualStatesAreSessionOnly(t *testing.T) {
	jsData, err := assets.ReadFile("assets/js/seat_editor.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(jsData)
	for _, want := range []string{
		"classSeatEditorAdjustedSeats = new Set()",
		"classSeatEditorAdjustedSeats.has(seatNo)",
		"classSeatEditorAdjustedSeats.add(sourceSeatNo)",
		"classSeatEditorAdjustedSeats.add(Number(targetSeatNo))",
		"preserveAdjustments: true",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("seat editor transient adjustment tracking missing %q", want)
		}
	}

	cssData, err := assets.ReadFile("assets/style.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(cssData)
	for _, want := range []string{
		".seat-editor-cell.occupied{background:#e6f0ff",
		".seat-editor-cell.empty{background:#fff",
		".seat-editor-cell.adjusted{background:#fff0f0",
	} {
		if !strings.Contains(css, want) {
			t.Fatalf("seat editor visual state missing %q", want)
		}
	}
}


func TestRecognitionClientShowsPersonTracksAndBestFaceQuality(t *testing.T) {
	data, err := assets.ReadFile("assets/js/recognition.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	for _, want := range []string{
		"tracked_person_count",
		"waiting_face_count",
		"quality_score",
		"best_quality",
		"等待露脸",
		"等待更清晰人脸",
		"person.face_visible",
		"qualityThreshold",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("person-track/best-face UI missing %q", want)
		}
	}
}

func TestRecognitionClientUsesFastLivenessCadence(t *testing.T) {
	data, err := assets.ReadFile("assets/js/recognition.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	for _, want := range []string{
		"performance.now() - startedAt < 2100",
		"result.liveness_max_frames",
		"face.liveness_timed_out",
		"function recognitionFrameIntervalMS()",
		"return 120;",
		"setTimeout(resolve, recognitionFrameIntervalMS())",
		"runAutoRecognitionLoop",
		"setTimeout(runAutoRecognitionLoop, recognitionFrameIntervalMS())",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("fast liveness client flow missing %q", want)
		}
	}
	if strings.Contains(script, "setInterval(() => recognizeFrame") {
		t.Fatal("auto recognition must use a sequential timeout loop instead of overlapping interval scans")
	}
}


func TestAttendanceShowsFirstLatestAndRecognitionCount(t *testing.T) {
	htmlData, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(htmlData)
	for _, want := range []string{"首次签到", "最近识别", "识别次数"} {
		if !strings.Contains(html, want) {
			t.Fatalf("attendance table missing %q", want)
		}
	}

	attendanceData, err := assets.ReadFile("assets/js/attendance.js")
	if err != nil {
		t.Fatal(err)
	}
	attendanceScript := string(attendanceData)
	for _, want := range []string{"a.last_seen_at || a.checked_at", "a.recognition_count || 1", "colspan=\"7\""} {
		if !strings.Contains(attendanceScript, want) {
			t.Fatalf("attendance list missing %q", want)
		}
	}

	seatingData, err := assets.ReadFile("assets/js/seating.js")
	if err != nil {
		t.Fatal(err)
	}
	seatingScript := string(seatingData)
	for _, want := range []string{"最近识别", "student.last_seen_at", "student.recognition_count"} {
		if !strings.Contains(seatingScript, want) {
			t.Fatalf("seat board latest recognition display missing %q", want)
		}
	}
}


func TestAutoStartCheckinSettingUIAndFlow(t *testing.T) {
	htmlData, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(htmlData)
	for _, want := range []string{
		`id="autoStartCheckin"`,
		`id="autoStartCheckinState"`,
		`/js/settings.js`,
		"进入“人脸签到”时自动打开摄像头并开启自动识别",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("auto check-in setting UI missing %q", want)
		}
	}

	settingsData, err := assets.ReadFile("assets/js/settings.js")
	if err != nil {
		t.Fatal(err)
	}
	settingsScript := string(settingsData)
	for _, want := range []string{
		"/api/settings",
		"auto_start_checkin",
		"enterCheckinPageAutoStart",
		"setAutoRecognitionEnabled(true)",
		"if (cameraOpen)",
		"await startCamera()",
	} {
		if !strings.Contains(settingsScript, want) {
			t.Fatalf("auto check-in setting logic missing %q", want)
		}
	}

	navigationData, err := assets.ReadFile("assets/js/navigation.js")
	if err != nil {
		t.Fatal(err)
	}
	navigationScript := string(navigationData)
	for _, want := range []string{
		"name !== 'checkin'",
		"stopAutoRecognition()",
		"enterCheckinPageAutoStart()",
	} {
		if !strings.Contains(navigationScript, want) {
			t.Fatalf("check-in page lifecycle missing %q", want)
		}
	}

	recognitionData, err := assets.ReadFile("assets/js/recognition.js")
	if err != nil {
		t.Fatal(err)
	}
	recognitionScript := string(recognitionData)
	for _, want := range []string{
		"function stopAutoRecognition()",
		"async function setAutoRecognitionEnabled(enabled)",
		"classList.contains('active')",
	} {
		if !strings.Contains(recognitionScript, want) {
			t.Fatalf("auto recognition lifecycle missing %q", want)
		}
	}
}


func TestCameraSettingsSupportsPreSaveConnectionDiagnostics(t *testing.T) {
	htmlData, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(htmlData)
	for _, want := range []string{
		`id="testCameraConfig"`,
		`id="cameraTestHeadline"`,
		`id="cameraTestChecks"`,
		"优先检查 RTSP/MJPEG 连续流",
		"HTTP Snapshot 回退",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("camera connection test UI missing %q", want)
		}
	}

	jsData, err := assets.ReadFile("assets/js/cameras.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(jsData)
	for _, want := range []string{
		"cameraFormPayload",
		"testCurrentCameraConfig",
		"/api/cameras/test",
		"renderCameraConnectionTest",
		"cameraTestFetchErrorMessage",
		"FaceSign 服务仍在线，但摄像头测试请求被异常中断",
		"无法连接 FaceSign 服务",
		"preview_base64",
		"camera_id: editingCameraID || 0",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("camera connection diagnostics missing %q", want)
		}
	}
}


func TestCameraAuthenticationAutoDetectUI(t *testing.T) {
	htmlData, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(htmlData)
	for _, want := range []string{
		`<option value="auto">自动检测（推荐）</option>`,
		"自动选择 Digest / Basic / 无认证",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("camera auto authentication UI missing %q", want)
		}
	}

	jsData, err := assets.ReadFile("assets/js/cameras.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(jsData)
	for _, want := range []string{
		"$('#cameraAuthMode').value = 'auto'",
		"camera.auth_mode === 'auto' ? '自动检测'",
		"result.detected_auth",
		"自动检测认证",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("camera auto authentication flow missing %q", want)
		}
	}
}


func TestNetworkCameraUsesWebRTCH264WithMJPEGFallbackAndDirectRecognition(t *testing.T) {
	cameraData, err := assets.ReadFile("assets/js/camera.js")
	if err != nil {
		t.Fatal(err)
	}
	cameraScript := string(cameraData)
	for _, want := range []string{
		"function activeNetworkCameraImage()",
		"function activeNetworkCameraVideo()",
		"function startWebRTCH264Preview(",
		"function startMJPEGPreviewFallback(",
		"function stopNetworkPreview()",
		"new RTCPeerConnection()",
		"addTransceiver('video', {direction: 'recvonly'})",
		"/api/cameras/${activeCamera.id}/webrtc",
		"/api/cameras/${activeCamera.id}/stream",
		"peer.setRemoteDescription(answer)",
		"WebRTC H.264 preview unavailable; using MJPEG fallback",
		"function updateCameraRealtimeStatus()",
		"getVideoPlaybackQuality",
		"peer.getStats()",
		"packetsReceived",
		"packetsLost",
		"framesPerSecond",
		"recordRecognitionRealtimeSample",
		"friendlyCameraRealtimeReason",
		"检测到 H.265/HEVC，请把摄像头视频编码改为 H.264",
		"WebRTC ICE 协商失败或超时",
		"WebRTC 已连接，但没有收到可播放的 H.264 视频帧",
	} {
		if !strings.Contains(cameraScript, want) {
			t.Fatalf("network WebRTC preview missing %q", want)
		}
	}
	for _, obsolete := range []string{
		"runNetworkPreviewLoop",
		"networkPreviewTimer",
		"networkPreviewObjectURL",
		"networkPreviewBusy",
	} {
		if strings.Contains(cameraScript, obsolete) {
			t.Fatalf("obsolete network preview polling remains: %q", obsolete)
		}
	}

	htmlData, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(htmlData)
	for _, want := range []string{
		`id="cameraNetworkWebRTC"`,
		`id="enrollCameraNetworkWebRTC"`,
		`data-camera-realtime-status`,
		`data-camera-stat="mode"`,
		`data-camera-stat="video"`,
		`data-camera-stat="drop"`,
		`data-camera-stat="network"`,
		`data-camera-stat="recognition"`,
		`data-camera-stat="reason"`,
		"WebRTC/H.264 实时预览",
		"浏览器硬件解码",
		"自动回退 MJPEG",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("network WebRTC preview UI missing %q", want)
		}
	}

	recognitionData, err := assets.ReadFile("assets/js/recognition.js")
	if err != nil {
		t.Fatal(err)
	}
	recognitionScript := string(recognitionData)
	for _, want := range []string{
		"/api/cameras/${activeCamera.id}/recognize",
		"api('/api/recognize'",
		"function recognitionFrameIntervalMS()",
		"Math.min(Number(activeCamera.fps || 5), 5)",
		"setTimeout(runAutoRecognitionLoop, recognitionFrameIntervalMS())",
		"recordRecognitionRealtimeSample(performance.now() - performanceStartedAt)",
	} {
		if !strings.Contains(recognitionScript, want) {
			t.Fatalf("network direct recognition flow missing %q", want)
		}
	}
}

func TestCameraRealtimeStatusOverlayStyles(t *testing.T) {
	cssData, err := assets.ReadFile("assets/style.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(cssData)
	for _, want := range []string{
		".camera-realtime-status",
		".camera-stat",
		".camera-stat.good",
		".camera-stat.warn",
		".camera-stat.bad",
		".camera-stat.reason",
	} {
		if !strings.Contains(css, want) {
			t.Fatalf("camera realtime status style missing %q", want)
		}
	}
}

func TestCameraFrameDimensionsHandlesClosedCamera(t *testing.T) {
	cameraData, err := assets.ReadFile("assets/js/camera.js")
	if err != nil {
		t.Fatal(err)
	}
	cameraScript := string(cameraData)
	start := strings.Index(cameraScript, "function cameraFrameDimensions")
	end := strings.Index(cameraScript, "async function capture")
	if start < 0 || end <= start {
		t.Fatal("cameraFrameDimensions function not found")
	}
	frameDimensions := cameraScript[start:end]
	if !strings.Contains(frameDimensions, "if (activeCamera && activeCamera.kind !== 'local')") {
		t.Fatal("cameraFrameDimensions must guard activeCamera before reading network camera dimensions")
	}
	if strings.Contains(frameDimensions, "activeCamera?.kind !== 'local'") {
		t.Fatal("null camera still enters the network dimension branch")
	}
}

func TestNetworkCameraPresetSimpleConfiguration(t *testing.T) {
	htmlData, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(htmlData)
	for _, want := range []string{
		`id="cameraPreset"`,
		`id="cameraIP"`,
		"只填写 IP 地址",
		"海康 Hikvision",
		"大华 Dahua",
		"宇视 Uniview",
		"VIVOTEK / 晶睿",
		"AXIS",
		"其他品牌 / 自定义（高级）",
		`id="cameraAdvancedToggle"`,
		"data-camera-advanced",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("simple network camera setup UI missing %q", want)
		}
	}

	jsData, err := assets.ReadFile("assets/js/cameras.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(jsData)
	for _, want := range []string{
		"const CAMERA_PRESETS",
		"normalizeCameraIP",
		"cameraPresetFromCamera",
		"cameraIPFromCamera",
		"cameraNetworkDisplay",
		"$$('[data-camera-local]').forEach",
		"$$('[data-camera-network]').forEach",
		"$$('[data-camera-agent]').forEach",
		"$$('[data-camera-advanced]').forEach",
		"/Streaming/channels/101",
		"/ISAPI/Streaming/channels/1/picture",
		"/cam/realmonitor?channel=1&subtype=0",
		"/cgi-bin/snapshot.cgi?channel=1",
		"/media/video1",
		"/LAPI/V1.0/Channels/1/Media/Video/Streams/0/Snapshot",
		"/live.sdp",
		"/cgi-bin/viewer/video.jpg?streamid=0",
		"/axis-media/media.amp",
		"/axis-cgi/jpg/image.cgi?camera=1",
		"applyCameraPreset({requireIP: true})",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("network camera preset logic missing %q", want)
		}
	}
	if !strings.Contains(script, "不要带 http://、端口或路径") {
		t.Fatal("IP-only validation guidance missing")
	}
}


func TestNetworkCameraUIExplainsContinuousStreamPrimaryAndSnapshotFallback(t *testing.T) {
	htmlData, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(htmlData)
	for _, want := range []string{
		"RTSP 连续流 + HTTP 抓图回退（推荐）",
		"MJPEG 连续流（主通道）",
		"HTTP/HTTPS 单帧抓图（兼容模式）",
		"WebRTC/H.264 实时预览",
		"浏览器硬件解码",
		"自动回退 MJPEG",
		"人脸识别仍建议 3–5 FPS",
		"HTTP Snapshot 仅作最终兼容回退",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("continuous camera UI missing %q", want)
		}
	}

	jsData, err := assets.ReadFile("assets/js/cameras.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(jsData)
	for _, want := range []string{
		"result.fallback_used",
		"result.primary_mode === 'rtsp'",
		"连续流连接成功",
		"连接成功（抓图回退）",
		"当前通道：",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("continuous camera diagnostics UI missing %q", want)
		}
	}
}
