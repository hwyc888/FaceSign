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
		"setTimeout(resolve, 120)",
		"runAutoRecognitionLoop",
		"setTimeout(runAutoRecognitionLoop, 120)",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("fast liveness client flow missing %q", want)
		}
	}
	if strings.Contains(script, "setInterval(() => recognizeFrame") {
		t.Fatal("auto recognition must use a sequential timeout loop instead of overlapping interval scans")
	}
}
