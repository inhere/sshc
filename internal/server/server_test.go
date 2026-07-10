package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHealthAPI(t *testing.T) {
	srv, err := New(Config{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got struct {
		OK   bool `json:"ok"`
		Data struct {
			Name     string `json:"name"`
			Readonly bool   `json:"readonly"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.OK || got.Data.Name != AppName {
		t.Fatalf("health = %+v", got)
	}
}

func TestReadonlyVisibleInHealth(t *testing.T) {
	srv, err := New(Config{Addr: "127.0.0.1:0", Readonly: true})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	srv.Handler().ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), `"readonly":true`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestRejectNonLoopbackWithoutToken(t *testing.T) {
	if _, err := New(Config{Addr: "0.0.0.0:8822"}); err == nil || !strings.Contains(err.Error(), "--token is required") {
		t.Fatalf("err = %v", err)
	}
	if _, err := New(Config{Addr: "192.168.1.20:8822"}); err == nil || !strings.Contains(err.Error(), "--token is required") {
		t.Fatalf("err = %v", err)
	}
	if _, err := New(Config{Addr: "0.0.0.0:8822", Token: "secret"}); err != nil {
		t.Fatalf("token should allow non-loopback: %v", err)
	}
}

func TestAssetsFallback(t *testing.T) {
	srv, err := New(Config{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/hosts", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "sshc serve") {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestWebDirServesFileAndSPAFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("web index"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("console.log('ok')"), 0600); err != nil {
		t.Fatal(err)
	}
	srv, err := New(Config{Addr: "127.0.0.1:0", WebDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "console.log") {
		t.Fatalf("file status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/hosts/devhost", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "web index" {
		t.Fatalf("fallback status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/../secret.txt", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("traversal status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
