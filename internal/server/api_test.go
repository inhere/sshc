package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inhere/sshc/internal/core"
)

func TestConfigSummaryAPI(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{
		LogsPath:     "./logs",
		AuthProfiles: []core.AuthProfile{{Name: "dev-root", User: "root", KeyPath: "~/.ssh/id_rsa"}},
		Hosts:        []core.Host{{Name: "devhost", IP: "10.0.0.8", AuthRef: "dev-root", Port: 22}},
	}); err != nil {
		t.Fatal(err)
	}
	srv := newTestServer(t, Config{Addr: "127.0.0.1:0"})
	rec := requestJSON(t, srv, http.MethodGet, "/api/config/summary", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"host_count":1`) || !strings.Contains(rec.Body.String(), `"auth_count":1`) || !strings.Contains(rec.Body.String(), `"doctor_ok":true`) {
		t.Fatalf("summary body = %s", rec.Body.String())
	}
}

func TestHostsCRUDAndMaskSecrets(t *testing.T) {
	withTempConfig(t)
	srv := newTestServer(t, Config{Addr: "127.0.0.1:0"})
	createBody := `{"name":"devhost","ip":"10.0.0.8","user":"root","password":"secret","port":22,"group":"testing","tags":["testing","app","app"]}`
	rec := requestJSON(t, srv, http.MethodPost, "/api/hosts", createBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret") || !strings.Contains(rec.Body.String(), `"password_enc":"***"`) {
		t.Fatalf("create leaked secret: %s", rec.Body.String())
	}
	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Hosts) != 1 || config.Hosts[0].Password != "secret" {
		t.Fatalf("saved hosts = %+v", config.Hosts)
	}
	if strings.Join(config.Hosts[0].Tags, ",") != "app,testing" {
		t.Fatalf("saved tags = %+v", config.Hosts[0].Tags)
	}

	updateBody := `{"name":"devhost","ip":"10.0.0.8","user":"root","port":22,"group":"prod","remark":"updated","tags":["prod","app"]}`
	rec = requestJSON(t, srv, http.MethodPut, "/api/hosts/devhost", updateBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", rec.Code, rec.Body.String())
	}
	config, err = core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Hosts[0].Password != "secret" || config.Hosts[0].Group != "prod" || config.Hosts[0].Remark != "updated" || strings.Join(config.Hosts[0].Tags, ",") != "app,prod" {
		t.Fatalf("updated host = %+v", config.Hosts[0])
	}

	config.AuthProfiles = []core.AuthProfile{{Name: "dev-root", User: "root", KeyPath: "~/.ssh/id_rsa"}}
	if err := core.SaveConfig(config); err != nil {
		t.Fatal(err)
	}
	updateBody = `{"name":"devhost","ip":"10.0.0.8","auth_ref":"dev-root","port":22,"group":"prod","remark":"updated"}`
	rec = requestJSON(t, srv, http.MethodPut, "/api/hosts/devhost", updateBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("update auth ref status = %d, body = %s", rec.Code, rec.Body.String())
	}
	config, err = core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Hosts[0].Password != "" || config.Hosts[0].PasswordEnc != "" || config.Hosts[0].AuthRef != "dev-root" {
		t.Fatalf("updated profile host = %+v", config.Hosts[0])
	}

	rec = requestJSON(t, srv, http.MethodGet, "/api/hosts/devhost", "")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("show status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rec = requestJSON(t, srv, http.MethodGet, "/api/hosts", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ip":"10.*.*.8"`) || strings.Contains(rec.Body.String(), `"ip":"10.0.0.8"`) {
		t.Fatalf("list masked status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rec = requestJSON(t, srv, http.MethodGet, "/api/hosts?show_ip=1", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ip":"10.0.0.8"`) {
		t.Fatalf("list show_ip status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rec = requestJSON(t, srv, http.MethodDelete, "/api/hosts/devhost", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", rec.Code, rec.Body.String())
	}
	config, err = core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Hosts) != 0 {
		t.Fatalf("hosts = %+v", config.Hosts)
	}
}

func TestHostsUpdatePreservesEmbeddedKeySecrets(t *testing.T) {
	withTempConfig(t)
	srv := newTestServer(t, Config{Addr: "127.0.0.1:0"})
	createBody := `{"name":"devhost","ip":"10.0.0.8","user":"root","key_path":"~/.ssh/id_rsa","key_data":"EMBEDDED_HOST_KEY","key_passphrase":"host-key-secret","port":22}`
	rec := requestJSON(t, srv, http.MethodPost, "/api/hosts", createBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "EMBEDDED_HOST_KEY") || strings.Contains(rec.Body.String(), "host-key-secret") || !strings.Contains(rec.Body.String(), `"key_data_enc":"***"`) || !strings.Contains(rec.Body.String(), `"key_passphrase_enc":"***"`) {
		t.Fatalf("create leaked embedded key secret: %s", rec.Body.String())
	}

	var masked core.Host
	decodeResponseData(t, rec, &masked)
	masked.Remark = "updated"
	body, err := json.Marshal(masked)
	if err != nil {
		t.Fatal(err)
	}
	rec = requestJSON(t, srv, http.MethodPut, "/api/hosts/devhost", string(body))
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", rec.Code, rec.Body.String())
	}
	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Hosts[0].KeyData != "EMBEDDED_HOST_KEY" || config.Hosts[0].KeyPassphrase != "host-key-secret" || config.Hosts[0].Remark != "updated" {
		t.Fatalf("updated host = %+v", config.Hosts[0])
	}

	config.AuthProfiles = []core.AuthProfile{{Name: "dev-root", User: "root", Password: "secret"}}
	if err := core.SaveConfig(config); err != nil {
		t.Fatal(err)
	}
	rec = requestJSON(t, srv, http.MethodPut, "/api/hosts/devhost", `{"name":"devhost","ip":"10.0.0.8","auth_ref":"dev-root","port":22}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("auth ref update status = %d, body = %s", rec.Code, rec.Body.String())
	}
	config, err = core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Hosts[0].AuthRef != "dev-root" || config.Hosts[0].KeyData != "" || config.Hosts[0].KeyDataEnc != "" || config.Hosts[0].KeyPassphrase != "" || config.Hosts[0].KeyPassphraseEnc != "" {
		t.Fatalf("auth ref host = %+v", config.Hosts[0])
	}
}

func TestReadonlyRejectsWrites(t *testing.T) {
	withTempConfig(t)
	srv := newTestServer(t, Config{Addr: "127.0.0.1:0", Readonly: true})
	rec := requestJSON(t, srv, http.MethodPost, "/api/hosts", `{"name":"devhost","ip":"10.0.0.8","user":"root"}`)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "readonly") {
		t.Fatalf("readonly response = %d %s", rec.Code, rec.Body.String())
	}
}

func TestAuthCRUDMasksSecretsAndRefusesUsedDelete(t *testing.T) {
	withTempConfig(t)
	srv := newTestServer(t, Config{Addr: "127.0.0.1:0"})
	rec := requestJSON(t, srv, http.MethodPost, "/api/auth-profiles", `{"name":"dev-root","user":"root","password":"secret","remark":"ops"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("auth create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret") || !strings.Contains(rec.Body.String(), `"password_enc":"***"`) {
		t.Fatalf("auth create leaked secret: %s", rec.Body.String())
	}

	rec = requestJSON(t, srv, http.MethodPut, "/api/auth-profiles/dev-root", `{"name":"dev-root","user":"root","key_path":"~/.ssh/id_rsa","remark":"key auth"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("auth update status = %d, body = %s", rec.Code, rec.Body.String())
	}
	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.AuthProfiles[0].Password != "secret" || config.AuthProfiles[0].KeyPath != "~/.ssh/id_rsa" {
		t.Fatalf("auth profile = %+v", config.AuthProfiles[0])
	}

	config.Hosts = []core.Host{{Name: "devhost", IP: "10.0.0.8", AuthRef: "dev-root", Port: 22}}
	if err := core.SaveConfig(config); err != nil {
		t.Fatal(err)
	}
	rec = requestJSON(t, srv, http.MethodDelete, "/api/auth-profiles/dev-root", "")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "used by hosts") {
		t.Fatalf("auth delete response = %d %s", rec.Code, rec.Body.String())
	}
}

func TestAuthUpdatePreservesEmbeddedKeySecrets(t *testing.T) {
	withTempConfig(t)
	srv := newTestServer(t, Config{Addr: "127.0.0.1:0"})
	createBody := `{"name":"deploy-key","user":"deploy","key_path":"~/.ssh/id_ed25519","key_data":"EMBEDDED_PROFILE_KEY","key_passphrase":"profile-key-secret","remark":"ops"}`
	rec := requestJSON(t, srv, http.MethodPost, "/api/auth-profiles", createBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("auth create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "EMBEDDED_PROFILE_KEY") || strings.Contains(rec.Body.String(), "profile-key-secret") || !strings.Contains(rec.Body.String(), `"key_data_enc":"***"`) || !strings.Contains(rec.Body.String(), `"key_passphrase_enc":"***"`) {
		t.Fatalf("auth create leaked embedded key secret: %s", rec.Body.String())
	}

	var masked core.AuthProfile
	decodeResponseData(t, rec, &masked)
	masked.Remark = "updated"
	body, err := json.Marshal(masked)
	if err != nil {
		t.Fatal(err)
	}
	rec = requestJSON(t, srv, http.MethodPut, "/api/auth-profiles/deploy-key", string(body))
	if rec.Code != http.StatusOK {
		t.Fatalf("auth update status = %d, body = %s", rec.Code, rec.Body.String())
	}
	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.AuthProfiles[0].KeyData != "EMBEDDED_PROFILE_KEY" || config.AuthProfiles[0].KeyPassphrase != "profile-key-secret" || config.AuthProfiles[0].Remark != "updated" {
		t.Fatalf("auth profile = %+v", config.AuthProfiles[0])
	}

	rec = requestJSON(t, srv, http.MethodPut, "/api/auth-profiles/deploy-key", `{"name":"deploy-key","user":"deploy","key_path":"~/.ssh/other","remark":"new path"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("auth key path update status = %d, body = %s", rec.Code, rec.Body.String())
	}
	config, err = core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.AuthProfiles[0].KeyPath != "~/.ssh/other" || config.AuthProfiles[0].KeyData != "" || config.AuthProfiles[0].KeyDataEnc != "" || config.AuthProfiles[0].KeyPassphrase != "" || config.AuthProfiles[0].KeyPassphraseEnc != "" {
		t.Fatalf("new path auth profile = %+v", config.AuthProfiles[0])
	}
}

func TestHostTrustUsesHook(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{Hosts: []core.Host{{Name: "devhost", IP: "10.0.0.8", User: "root", Port: 22}}}); err != nil {
		t.Fatal(err)
	}
	var got core.Host
	t.Cleanup(setTrustHostKeyForTest(func(host core.Host) (core.HostKeyTrustResult, error) {
		got = host
		return core.HostKeyTrustResult{Status: "added", Address: "10.0.0.8:22"}, nil
	}))
	srv := newTestServer(t, Config{Addr: "127.0.0.1:0"})
	rec := requestJSON(t, srv, http.MethodPost, "/api/hosts/devhost/trust", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"added"`) {
		t.Fatalf("trust response = %d %s", rec.Code, rec.Body.String())
	}
	if got.Name != "devhost" || got.IP != "10.0.0.8" {
		t.Fatalf("trusted host = %+v", got)
	}
}

func TestLogsAPI(t *testing.T) {
	withTempConfig(t)
	host := core.Host{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22}
	if err := core.AppendRunLog(host, core.RunLogRecord{
		TaskID:  "task-api",
		Target:  "devhost",
		Command: "printf",
		Status:  "success",
		Output:  "one\ntwo\nthree\n",
	}); err != nil {
		t.Fatal(err)
	}
	srv := newTestServer(t, Config{Addr: "127.0.0.1:0"})
	rec := requestJSON(t, srv, http.MethodGet, "/api/logs?target=devhost", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"task_id":"task-api"`) {
		t.Fatalf("logs list = %d %s", rec.Code, rec.Body.String())
	}
	rec = requestJSON(t, srv, http.MethodGet, "/api/logs/task-api?target=devhost", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"command":"printf"`) {
		t.Fatalf("logs show = %d %s", rec.Code, rec.Body.String())
	}
	rec = requestJSON(t, srv, http.MethodGet, "/api/logs/task-api/output?target=devhost&tail=2", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "two\\nthree\\n") {
		t.Fatalf("logs output = %d %s", rec.Code, rec.Body.String())
	}
}

func newTestServer(t *testing.T, config Config) *Server {
	t.Helper()
	srv, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

func requestJSON(t *testing.T, srv *Server, method, path, body string) *httptest.ResponseRecorder {
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
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func withTempConfig(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	t.Cleanup(core.SetUserHomeDirForTest(func() (string, error) { return home, nil }))
	path := filepath.Join(home, core.ConfigFileName)
	t.Setenv(core.ConfigEnvKey, path)
	return path
}

func decodeResponseData(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	var envelope struct {
		OK   bool            `json:"ok"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK {
		t.Fatalf("response not ok: %s", rec.Body.String())
	}
	if err := json.Unmarshal(envelope.Data, dst); err != nil {
		t.Fatal(err)
	}
}
