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
		"assets/js/recognition.js",
		"assets/js/enrollment.js",
		"assets/js/classes.js",
		"assets/js/students.js",
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
	for _, selector := range []string{"up", "down", "rename", "delete"} {
		want := "$$('[data-class-" + selector + "]').forEach"
		if !strings.Contains(script, want) {
			t.Fatalf("class action selector %q must use querySelectorAll before forEach", selector)
		}
	}
}
