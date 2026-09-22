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
		good := "$('[" + selector + "]').forEach"
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
