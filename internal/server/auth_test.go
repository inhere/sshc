package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inhere/sshc/internal/core"
)

func TestTokenAuthGuardsAPI(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{
		Hosts: []core.Host{{Name: "devhost", IP: "10.0.0.8", User: "root", Port: 22}},
	}); err != nil {
		t.Fatal(err)
	}
	srv := newTestServer(t, Config{Addr: "127.0.0.1:0", Token: "secret"})

	rec := requestJSON(t, srv, http.MethodGet, "/api/hosts", "")
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "login required") {
		t.Fatalf("unauthorized response = %d %s", rec.Code, rec.Body.String())
	}

	rec = requestJSON(t, srv, http.MethodPost, "/api/auth/login", `{"token":"bad"}`)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "invalid token") {
		t.Fatalf("bad login response = %d %s", rec.Code, rec.Body.String())
	}

	loginRec := requestJSON(t, srv, http.MethodPost, "/api/auth/login", `{"token":"secret"}`)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login response = %d %s", loginRec.Code, loginRec.Body.String())
	}
	sessionCookie := loginRec.Result().Cookies()[0]
	var loginData loginResponse
	decodeResponseData(t, loginRec, &loginData)
	if loginData.CSRF == "" {
		t.Fatalf("missing csrf: %s", loginRec.Body.String())
	}

	rec = requestJSONWithAuth(t, srv, http.MethodGet, "/api/hosts", "", sessionCookie, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "devhost") {
		t.Fatalf("authorized read response = %d %s", rec.Code, rec.Body.String())
	}

	createBody := `{"name":"newhost","ip":"10.0.0.9","user":"root"}`
	rec = requestJSONWithAuth(t, srv, http.MethodPost, "/api/hosts", createBody, sessionCookie, "")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "csrf") {
		t.Fatalf("missing csrf response = %d %s", rec.Code, rec.Body.String())
	}
	rec = requestJSONWithAuth(t, srv, http.MethodPost, "/api/hosts", createBody, sessionCookie, loginData.CSRF)
	if rec.Code != http.StatusCreated {
		t.Fatalf("csrf write response = %d %s", rec.Code, rec.Body.String())
	}
}

func TestTokenDisabledAllowsAPI(t *testing.T) {
	withTempConfig(t)
	srv := newTestServer(t, Config{Addr: "127.0.0.1:0"})
	rec := requestJSON(t, srv, http.MethodGet, "/api/hosts", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
}

func TestTokenIsClearedFromServerConfig(t *testing.T) {
	srv := newTestServer(t, Config{Addr: "127.0.0.1:0", Token: "secret"})
	if srv.Config().Token != "" {
		t.Fatalf("token should be cleared from public config")
	}
}

func TestGenerateToken(t *testing.T) {
	first, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || second == "" || first == second {
		t.Fatalf("tokens = %q %q", first, second)
	}
}

func requestJSONWithAuth(t *testing.T, srv *Server, method, path, body string, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if csrf != "" {
		req.Header.Set(csrfHeaderName, csrf)
	}
	srv.Handler().ServeHTTP(rec, req)
	return rec
}
