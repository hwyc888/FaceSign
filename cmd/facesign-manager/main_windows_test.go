//go:build windows

package main

import (
	"os"
	"path/filepath"
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

func TestNormalizeUpgradePackageDirAcceptsScriptsFolder(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "FaceSign.exe"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	scripts := filepath.Join(root, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scripts, "upgrade.ps1"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := normalizeUpgradePackageDir(scripts); got != root {
		t.Fatalf("normalizeUpgradePackageDir()=%q want %q", got, root)
	}
}

func TestValidateUpgradePackage(t *testing.T) {
	root := t.TempDir()
	for _, relative := range []string{
		"FaceSign.exe",
		"FaceSignManager.exe",
		"onnxruntime.dll",
		filepath.Join("scripts", "install.ps1"),
		filepath.Join("scripts", "upgrade.ps1"),
	} {
		path := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	oldInstallDir := installDirFlag
	installDirFlag = filepath.Join(root, "installed")
	defer func() { installDirFlag = oldInstallDir }()

	if err := validateUpgradePackage(root); err != nil {
		t.Fatalf("valid package rejected: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "scripts", "upgrade.ps1")); err != nil {
		t.Fatal(err)
	}
	if err := validateUpgradePackage(root); err == nil {
		t.Fatal("package without upgrade.ps1 was accepted")
	}
}

func TestPowerShellLiteralEscapesApostrophe(t *testing.T) {
	if got, want := powerShellLiteral("C:\\Teacher's\\FaceSign"), "'C:\\Teacher''s\\FaceSign'"; got != want {
		t.Fatalf("powerShellLiteral()=%q want %q", got, want)
	}
}

func TestPotentiallyBlockingManagerActionsAreAsync(t *testing.T) {
	for _, id := range []int{
		idStart,
		idStop,
		idRestart,
		idEnableStartup,
		idDisableStartup,
		idOpenWeb,
		idOpenLog,
		idOpenDir,
		idRefresh,
		idUpgrade,
	} {
		if !isAsyncActionButton(id) {
			t.Fatalf("manager action id=%d can block the UI thread", id)
		}
	}
	if isAsyncActionButton(idExit) {
		t.Fatal("exit must remain an immediate UI action")
	}
}
