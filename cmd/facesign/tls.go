package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	rootCAFileName          = "facesign-root-ca.crt"
	rootCAKeyFileName       = "facesign-root-ca.key"
	serverCertFileName      = "facesign-server.crt"
	serverKeyFileName       = "facesign-server.key"
	rootCALifetimeYears     = 50
	serverCertLifetimeDays  = 397
	serverRenewBefore       = 30 * 24 * time.Hour
)

type tlsIdentityFiles struct {
	CACertPath     string
	CAKeyPath      string
	ServerCertPath string
	ServerKeyPath  string
}

func ensureTLSIdentity(dir, extraHosts string) (tlsIdentityFiles, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return tlsIdentityFiles{}, err
	}
	files := tlsIdentityFiles{
		CACertPath:     filepath.Join(dir, rootCAFileName),
		CAKeyPath:      filepath.Join(dir, rootCAKeyFileName),
		ServerCertPath: filepath.Join(dir, serverCertFileName),
		ServerKeyPath:  filepath.Join(dir, serverKeyFileName),
	}
	now := time.Now()
	caCert, caKey, err := loadOrCreateRootCA(files, now)
	if err != nil {
		return tlsIdentityFiles{}, err
	}
	hosts, err := collectTLSHosts(extraHosts)
	if err != nil {
		return tlsIdentityFiles{}, err
	}
	if !serverCertificateUsable(files, caCert, hosts, now) {
		if err := createServerCertificate(files, caCert, caKey, hosts, now); err != nil {
			return tlsIdentityFiles{}, err
		}
	}
	return files, nil
}

func loadOrCreateRootCA(files tlsIdentityFiles, now time.Time) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	certExists := fileExists(files.CACertPath)
	keyExists := fileExists(files.CAKeyPath)
	if certExists != keyExists {
		return nil, nil, errors.New("FaceSign根CA文件不完整；为避免客户端信任被意外更换，请恢复缺失文件，或在确认要重置所有客户端信任后同时删除根CA证书和私钥")
	}
	if certExists {
		cert, err := loadCertificate(files.CACertPath)
		if err != nil {
			return nil, nil, fmt.Errorf("read root CA certificate: %w", err)
		}
		key, err := loadECPrivateKey(files.CAKeyPath)
		if err != nil {
			return nil, nil, fmt.Errorf("read root CA private key: %w", err)
		}
		if !cert.IsCA || !publicKeyMatches(cert, key) {
			return nil, nil, errors.New("FaceSign根CA证书与私钥不匹配；拒绝自动替换长期信任身份")
		}
		if !now.Before(cert.NotAfter) {
			return nil, nil, errors.New("FaceSign根CA已经过期；需要重新建立客户端信任")
		}
		return cert, key, nil
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, nil, err
	}
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "FaceSign"
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"FaceSign"},
			CommonName:   "FaceSign Local Root CA - " + hostname,
		},
		NotBefore:             now.Add(-24 * time.Hour),
		NotAfter:              now.AddDate(rootCALifetimeYears, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	if err := writeCertificate(files.CACertPath, der, 0o644); err != nil {
		return nil, nil, err
	}
	if err := writeECPrivateKey(files.CAKeyPath, key); err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

func createServerCertificate(files tlsIdentityFiles, caCert *x509.Certificate, caKey *ecdsa.PrivateKey, hosts []string, now time.Time) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := randomSerial()
	if err != nil {
		return err
	}
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{Organization: []string{"FaceSign"}, CommonName: "FaceSign HTTPS Server"},
		NotBefore:             now.Add(-24 * time.Hour),
		NotAfter:              now.Add(serverCertLifetimeDays * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	if template.NotAfter.After(caCert.NotAfter.Add(-24 * time.Hour)) {
		template.NotAfter = caCert.NotAfter.Add(-24 * time.Hour)
	}
	for _, host := range hosts {
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, host)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		return err
	}
	if err := writeCertificate(files.ServerCertPath, der, 0o644); err != nil {
		return err
	}
	return writeECPrivateKey(files.ServerKeyPath, key)
}

func serverCertificateUsable(files tlsIdentityFiles, caCert *x509.Certificate, hosts []string, now time.Time) bool {
	if !fileExists(files.ServerCertPath) || !fileExists(files.ServerKeyPath) {
		return false
	}
	cert, err := loadCertificate(files.ServerCertPath)
	if err != nil {
		return false
	}
	key, err := loadECPrivateKey(files.ServerKeyPath)
	if err != nil {
		return false
	}
	if !publicKeyMatches(cert, key) || cert.CheckSignatureFrom(caCert) != nil {
		return false
	}
	if now.Before(cert.NotBefore) || cert.NotAfter.Sub(now) <= serverRenewBefore {
		return false
	}
	for _, host := range hosts {
		if err := cert.VerifyHostname(host); err != nil {
			return false
		}
	}
	return true
}

func collectTLSHosts(extraHosts string) ([]string, error) {
	values := map[string]struct{}{
		"localhost": {},
		"127.0.0.1": {},
		"::1":       {},
	}
	if hostname, err := os.Hostname(); err == nil && strings.TrimSpace(hostname) != "" {
		hostname = strings.TrimSpace(hostname)
		values[hostname] = struct{}{}
		if !strings.Contains(hostname, ".") {
			values[hostname+".local"] = struct{}{}
		}
	}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, addr := range addrs {
			var ip net.IP
			switch value := addr.(type) {
			case *net.IPNet:
				ip = value.IP
			case *net.IPAddr:
				ip = value.IP
			}
			if ip == nil || ip.IsUnspecified() {
				continue
			}
			values[ip.String()] = struct{}{}
		}
	}
	for _, raw := range strings.Split(extraHosts, ",") {
		host := strings.TrimSpace(raw)
		if host == "" {
			continue
		}
		if net.ParseIP(host) == nil && !validDNSName(host) {
			return nil, fmt.Errorf("invalid tls host %q", host)
		}
		values[host] = struct{}{}
	}
	hosts := make([]string, 0, len(values))
	for host := range values {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	return hosts, nil
}

func validDNSName(value string) bool {
	if len(value) == 0 || len(value) > 253 || strings.Contains(value, "..") {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r == '-' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
				return false
			}
		}
	}
	return true
}

func tlsBootstrapHandler(next http.Handler, caPath string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+rootCAFileName {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		data, err := os.ReadFile(caPath)
		if err != nil {
			http.Error(w, "root CA unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/x-x509-ca-cert")
		w.Header().Set("Content-Disposition", "attachment; filename=FaceSign-Root-CA.crt")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			_, _ = w.Write(data)
		}
	})
}

func httpsRedirectHandler(next http.Handler, httpsListen string) http.Handler {
	_, httpsPort, err := net.SplitHostPort(httpsListen)
	if err != nil {
		httpsPort = "8443"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/"+rootCAFileName {
			next.ServeHTTP(w, r)
			return
		}
		host := requestHost(r.Host)
		if host == "" {
			host = "127.0.0.1"
		}
		target := "https://" + net.JoinHostPort(host, httpsPort) + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusPermanentRedirect)
	})
}

func requestHost(hostport string) string {
	if host, _, err := net.SplitHostPort(hostport); err == nil {
		return host
	}
	return strings.Trim(strings.TrimSpace(hostport), "[]")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func loadCertificate(path string) (*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("invalid certificate PEM")
	}
	return x509.ParseCertificate(block.Bytes)
}

func loadECPrivateKey(path string) (*ecdsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "EC PRIVATE KEY" {
		return nil, errors.New("invalid EC private key PEM")
	}
	return x509.ParseECPrivateKey(block.Bytes)
}

func writeCertificate(path string, der []byte, mode os.FileMode) error {
	return os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), mode)
}

func writeECPrivateKey(path string, key *ecdsa.PrivateKey) error {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	return os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0o600)
}

func publicKeyMatches(cert *x509.Certificate, key *ecdsa.PrivateKey) bool {
	publicKey, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return false
	}
	return publicKey.X.Cmp(key.X) == 0 && publicKey.Y.Cmp(key.Y) == 0
}

func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, limit)
}
