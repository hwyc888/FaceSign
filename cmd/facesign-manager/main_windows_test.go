//go:build windows

package main

import (
	"testing"
	"time"
)

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


func TestLaunchAsyncDoesNotBlockCaller(t *testing.T) {
	block := make(chan struct{})
	done := make(chan error, 1)
	started := time.Now()

	launchAsync(func() error {
		<-block
		return nil
	}, func(err error) {
		done <- err
	})

	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("launchAsync blocked the caller for %s", elapsed)
	}
	select {
	case <-done:
		t.Fatal("background action completed before it was released")
	default:
	}

	close(block)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("background action did not complete")
	}
}
