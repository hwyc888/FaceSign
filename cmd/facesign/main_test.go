package main

import "testing"

func TestBrowserURL(t *testing.T) {
	tests := map[string]string{
		"0.0.0.0:8080": "http://127.0.0.1:8080/",
		"127.0.0.1:8080": "http://127.0.0.1:8080/",
		":9090": "http://127.0.0.1:9090/",
		"localhost:8080": "http://localhost:8080/",
		"[::]:8080": "http://127.0.0.1:8080/",
	}
	for input, want := range tests {
		if got := browserURL(input); got != want {
			t.Fatalf("browserURL(%q) = %q, want %q", input, got, want)
		}
	}
}


func TestSecureBrowserURL(t *testing.T) {
	tests := map[string]string{
		"0.0.0.0:8443": "https://127.0.0.1:8443/",
		"127.0.0.1:9443": "https://127.0.0.1:9443/",
		"localhost:8443": "https://localhost:8443/",
		"[::]:8443": "https://127.0.0.1:8443/",
	}
	for input, want := range tests {
		if got := secureBrowserURL(input); got != want {
			t.Fatalf("secureBrowserURL(%q) = %q, want %q", input, got, want)
		}
	}
}


func TestBackgroundRequested(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "none", args: nil, want: false},
		{name: "enabled long", args: []string{"--background=true"}, want: true},
		{name: "enabled flag", args: []string{"--background"}, want: true},
		{name: "disabled", args: []string{"--background=false"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := backgroundRequested(tt.args); got != tt.want {
				t.Fatalf("backgroundRequested(%v)=%v want %v", tt.args, got, tt.want)
			}
		})
	}
}
