package main

import (
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSmokeCertificateTrust остаётся ограниченным указанным CA и не отключает проверку имени сервера.
// @args t — контекст проверки доверия локальному сертификату без изменения системного хранилища.
func TestSmokeCertificateTrust(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	configuration, err := smokeTLS(path)
	if err != nil || configuration.InsecureSkipVerify || configuration.RootCAs == nil {
		t.Fatalf("scoped trust is invalid: %v", err)
	}
	if _, err := server.Certificate().Verify(x509.VerifyOptions{Roots: configuration.RootCAs, DNSName: "127.0.0.1"}); err != nil {
		t.Fatal("explicit local CA was not trusted")
	}
	if _, err := server.Certificate().Verify(x509.VerifyOptions{Roots: configuration.RootCAs, DNSName: "unrelated.example.invalid"}); err == nil {
		t.Fatal("certificate hostname validation was bypassed")
	}
	if err := os.WriteFile(path, []byte("invalid PEM"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := smokeTLS(path); err == nil {
		t.Fatal("invalid CA silently accepted")
	}
	if _, err := smokeTLS(filepath.Dir(path)); err == nil {
		t.Fatal("directory accepted as CA file")
	}
}

// TestTargetGuards запрещает неявный production, утечку через URL и тестовые мутации рабочей среды.
// @args t — контекст проверки обязательной изоляции окружений.
func TestTargetGuards(t *testing.T) {
	for _, test := range []struct {
		env, url string
		exercise bool
		allowed  bool
	}{
		{"", "https://meet.example.test", false, false},
		{"local", "http://127.0.0.1:8080", true, true},
		{"local", "https://meet.example.test", true, false},
		{"staging", "http://meet.example.test", true, false},
		{"staging", "https://localhost", true, false},
		{"staging", "https://meet.example.test", true, true},
		{"production", "https://meet.example.test", false, true},
		{"production", "https://meet.example.test", true, false},
		{"production", "https://user:password@meet.example.test", false, false},
		{"production", "https://meet.example.test/?token=secret", false, false},
	} {
		_, err := validateTarget(test.env, test.url, test.exercise)
		if (err == nil) != test.allowed {
			t.Errorf("guard result mismatch for environment %q", test.env)
		}
	}
}

// TestMinimalSmokeUsesRealLoginContract проверяет имя accessToken и обязательное совпадение версий.
// @args t — контекст HTTP-проверки фактического контракта входа без настоящих учётных данных.
func TestMinimalSmokeUsesRealLoginContract(t *testing.T) {
	t.Setenv("SMOKE_EMAIL", "smoke@example.test")
	t.Setenv("SMOKE_PASSWORD", "test-only-password")
	sha := strings.Repeat("a", 40)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version", "/version.json":
			w.Write([]byte(`{"version":"v1.0.0-test","commit":"` + sha + `"}`))
		case "/api/v1/auth/login":
			w.Write([]byte(`{"accessToken":"test-token"}`))
		case "/api/v1/auth/me", "/api/v1/capabilities", "/api/v1/webrtc/config", "/api/v1/admin/summary":
			if r.Header.Get("Authorization") != "Bearer test-token" {
				t.Error("login token was not used")
				w.WriteHeader(401)
				return
			}
			if r.URL.Path == "/api/v1/admin/summary" {
				w.WriteHeader(403)
			}
		default:
			w.Write([]byte("<html>Meet</html>"))
		}
	}))
	defer server.Close()
	result, err := run("local", server.URL, "v1.0.0-test", sha, false)
	if err != nil || len(result.Checks) != 6 {
		t.Fatalf("minimal smoke failed: %v", err)
	}
	if _, err = run("local", server.URL, "v1.0.1", sha, false); err == nil {
		t.Fatal("version mismatch did not block smoke")
	}
}
