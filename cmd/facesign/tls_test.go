package main

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistentTLSIdentityReusesRootCAAndCoversExtraHosts(t *testing.T) {
	dir := t.TempDir()
	first, err := ensureTLSIdentity(dir, "facesign.test,10.20.30.40")
	if err != nil {
		t.Fatal(err)
	}
	caBefore, err := os.ReadFile(first.CACertPath)
	if err != nil {
		t.Fatal(err)
	}
	serverBefore, err := os.ReadFile(first.ServerCertPath)
	if err != nil {
		t.Fatal(err)
	}

	second, err := ensureTLSIdentity(dir, "facesign.test,10.20.30.40")
	if err != nil {
		t.Fatal(err)
	}
	caAfter, _ := os.ReadFile(second.CACertPath)
	serverAfter, _ := os.ReadFile(second.ServerCertPath)
	if !bytes.Equal(caBefore, caAfter) {
		t.Fatal("root CA changed across restart")
	}
	if !bytes.Equal(serverBefore, serverAfter) {
		t.Fatal("still-valid server certificate was unexpectedly replaced")
	}

	caCert, err := loadCertificate(first.CACertPath)
	if err != nil {
		t.Fatal(err)
	}
	if caCert.NotAfter.Sub(time.Now()) < 49*365*24*time.Hour {
		t.Fatalf("root CA is not long-lived: %s", caCert.NotAfter)
	}
	serverCert, err := loadCertificate(first.ServerCertPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := serverCert.VerifyHostname("facesign.test"); err != nil {
		t.Fatal(err)
	}
	if err := serverCert.VerifyHostname("10.20.30.40"); err != nil {
		t.Fatal(err)
	}
	if serverCert.NotAfter.Sub(serverCert.NotBefore) > 399*24*time.Hour {
		t.Fatalf("server certificate validity is too long: %s", serverCert.NotAfter.Sub(serverCert.NotBefore))
	}
	roots := x509.NewCertPool()
	roots.AddCert(caCert)
	if _, err := serverCert.Verify(x509.VerifyOptions{Roots: roots, DNSName: "facesign.test"}); err != nil {
		t.Fatalf("server certificate does not verify against persistent root CA: %v", err)
	}
}

func TestHTTPSRedirectKeepsRootCADownloadOnHTTP(t *testing.T) {
	dir := t.TempDir()
	caPath := filepath.Join(dir, rootCAFileName)
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("test")}), 0o644); err != nil {
		t.Fatal(err)
	}
	app := tlsBootstrapHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), caPath)
	handler := httpsRedirectHandler(app, "0.0.0.0:8443")

	redirectReq := httptest.NewRequest(http.MethodGet, "http://192.168.1.10:8080/api/version?x=1", nil)
	redirectReq.Host = "192.168.1.10:8080"
	redirectRec := httptest.NewRecorder()
	handler.ServeHTTP(redirectRec, redirectReq)
	if redirectRec.Code != http.StatusPermanentRedirect {
		t.Fatalf("redirect status=%d", redirectRec.Code)
	}
	if got := redirectRec.Header().Get("Location"); got != "https://192.168.1.10:8443/api/version?x=1" {
		t.Fatalf("unexpected redirect location %q", got)
	}

	caReq := httptest.NewRequest(http.MethodGet, "http://192.168.1.10:8080/"+rootCAFileName, nil)
	caRec := httptest.NewRecorder()
	handler.ServeHTTP(caRec, caReq)
	if caRec.Code != http.StatusOK {
		t.Fatalf("root CA status=%d", caRec.Code)
	}
}
