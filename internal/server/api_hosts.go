package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gookit/rux/v2"
	"github.com/inhere/sshc/internal/core"
)

var trustHostKey = core.TrustHostKey

func setTrustHostKeyForTest(fn func(core.Host) (core.HostKeyTrustResult, error)) func() {
	old := trustHostKey
	trustHostKey = fn
	return func() { trustHostKey = old }
}

func (s *Server) handleHostsList(c *rux.Context) {
	config, err := core.LoadConfig()
	if err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}
	showIP := c.Query("show_ip") == "1" || strings.EqualFold(c.Query("show_ip"), "true")
	hosts := make([]core.Host, 0, len(config.Hosts))
	for _, host := range config.Hosts {
		masked := core.MaskHost(host)
		if !showIP {
			masked.IP = maskHostIP(masked.IP)
		}
		hosts = append(hosts, masked)
	}
	writeOK(c, hosts)
}

func (s *Server) handleHostsShow(c *rux.Context) {
	config, err := core.LoadConfig()
	if err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}
	host, ok := findHost(config.Hosts, c.Param("name"))
	if !ok {
		writeError(c, http.StatusNotFound, fmt.Errorf("host %q not found", c.Param("name")))
		return
	}
	writeOK(c, core.MaskHost(host))
}

func (s *Server) handleHostsCreate(c *rux.Context) {
	if s.rejectReadonly(c) {
		return
	}
	var host core.Host
	if err := readJSON(c, &host); err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	normalizeAPIHost(&host)
	if host.Name == "" {
		writeError(c, http.StatusBadRequest, errors.New("host name is required"))
		return
	}
	config, err := core.LoadConfig()
	if err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}
	if _, ok := findHost(config.Hosts, host.Name); ok {
		writeError(c, http.StatusConflict, fmt.Errorf("host %q already exists", host.Name))
		return
	}
	config.Hosts = append(config.Hosts, host)
	if err := saveCheckedConfig(config); err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	writeJSON(c, http.StatusCreated, response{OK: true, Data: core.MaskHost(host)})
}

func (s *Server) handleHostsUpdate(c *rux.Context) {
	if s.rejectReadonly(c) {
		return
	}
	name := c.Param("name")
	var host core.Host
	if err := readJSON(c, &host); err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	normalizeAPIHost(&host)
	if host.Name == "" {
		host.Name = name
	}
	if host.Name != name {
		writeError(c, http.StatusBadRequest, errors.New("host name cannot be changed by update API"))
		return
	}
	config, err := core.LoadConfig()
	if err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}
	idx, old, ok := findHostIndex(config.Hosts, name)
	if !ok {
		writeError(c, http.StatusNotFound, fmt.Errorf("host %q not found", name))
		return
	}
	if host.Password == "" && host.PasswordEnc == "" && host.AuthRef == "" && host.KeyPath == "" {
		host.PasswordEnc = old.PasswordEnc
	}
	config.Hosts[idx] = host
	if err := saveCheckedConfig(config); err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	writeOK(c, core.MaskHost(host))
}

func (s *Server) handleHostsDelete(c *rux.Context) {
	if s.rejectReadonly(c) {
		return
	}
	name := c.Param("name")
	config, err := core.LoadConfig()
	if err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}
	idx, _, ok := findHostIndex(config.Hosts, name)
	if !ok {
		writeError(c, http.StatusNotFound, fmt.Errorf("host %q not found", name))
		return
	}
	config.Hosts = append(config.Hosts[:idx], config.Hosts[idx+1:]...)
	if err := saveCheckedConfig(config); err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	writeOK(c, map[string]string{"deleted": name})
}

func (s *Server) handleHostsTrust(c *rux.Context) {
	if s.rejectReadonly(c) {
		return
	}
	config, err := core.LoadConfig()
	if err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}
	host, ok := findHost(config.Hosts, c.Param("name"))
	if !ok {
		writeError(c, http.StatusNotFound, fmt.Errorf("host %q not found", c.Param("name")))
		return
	}
	result, err := trustHostKey(host)
	if err != nil {
		writeError(c, http.StatusBadGateway, err)
		return
	}
	writeOK(c, hostTrustResponse{
		Host:           core.MaskHost(result.Host),
		Address:        result.Address,
		KnownHostsPath: result.KnownHostsPath,
		KeyType:        result.KeyType,
		Fingerprint:    result.Fingerprint,
		Status:         result.Status,
	})
}

type hostTrustResponse struct {
	Host           core.Host `json:"host"`
	Address        string    `json:"address"`
	KnownHostsPath string    `json:"known_hosts_path"`
	KeyType        string    `json:"key_type"`
	Fingerprint    string    `json:"fingerprint"`
	Status         string    `json:"status"`
}

func findHost(hosts []core.Host, name string) (core.Host, bool) {
	_, host, ok := findHostIndex(hosts, name)
	return host, ok
}

func maskHostIP(ip string) string {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return ip
	}
	for _, part := range parts {
		if part == "" {
			return ip
		}
	}
	return parts[0] + ".*.*." + parts[3]
}

func findHostIndex(hosts []core.Host, name string) (int, core.Host, bool) {
	name = strings.TrimSpace(name)
	for i, host := range hosts {
		if strings.TrimSpace(host.Name) == name {
			return i, host, true
		}
	}
	return -1, core.Host{}, false
}
