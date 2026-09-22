//go:build windows

package main

import "testing"

func TestParseKeyValueLog(t *testing.T) {
	values := parseKeyValueLog("version=abc123\r\nhttp_listen=0.0.0.0:8080\r\nhttps_listen=0.0.0.0:8443\r\nurl=https://127.0.0.1:8443/\r\n")
	if values["version"] != "abc123" {
		t.Fatalf("version=%q", values["version"])
	}
	if values["https_listen"] != "0.0.0.0:8443" {
		t.Fatalf("https_listen=%q", values["https_listen"])
	}
	if values["url"] != "https://127.0.0.1:8443/" {
		t.Fatalf("url=%q", values["url"])
	}
}

func TestParseKeyValueLogIgnoresMalformedLines(t *testing.T) {
	values := parseKeyValueLog("bad line\nversion=v1\n")
	if len(values) != 1 || values["version"] != "v1" {
		t.Fatalf("unexpected values: %#v", values)
	}
}
