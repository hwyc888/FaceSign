package web

import (
	"strings"
	"testing"
)

func TestClassActionSelectorsUseQuerySelectorAll(t *testing.T) {
	data, err := assets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)

	for _, selector := range []string{"up", "down", "rename", "delete"} {
		want := "$$('[data-class-" + selector + "]').forEach"
		if !strings.Contains(script, want) {
			t.Fatalf("class action selector %q must use querySelectorAll before forEach", selector)
		}
	}
}
