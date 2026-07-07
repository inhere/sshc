package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gookit/rux/v2"
	"github.com/inhere/sshc/internal/core"
)

func (s *Server) handleAuthList(c *rux.Context) {
	config, err := core.LoadConfig()
	if err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}
	profiles := make([]core.AuthProfile, 0, len(config.AuthProfiles))
	for _, profile := range config.AuthProfiles {
		profiles = append(profiles, core.MaskAuthProfile(profile))
	}
	writeOK(c, profiles)
}

func (s *Server) handleAuthShow(c *rux.Context) {
	config, err := core.LoadConfig()
	if err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}
	profile, ok := findAuth(config.AuthProfiles, c.Param("name"))
	if !ok {
		writeError(c, http.StatusNotFound, fmt.Errorf("auth profile %q not found", c.Param("name")))
		return
	}
	writeOK(c, core.MaskAuthProfile(profile))
}

func (s *Server) handleAuthCreate(c *rux.Context) {
	if s.rejectReadonly(c) {
		return
	}
	var profile core.AuthProfile
	if err := readJSON(c, &profile); err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	normalizeAPIAuth(&profile)
	if profile.Name == "" {
		writeError(c, http.StatusBadRequest, errors.New("auth profile name is required"))
		return
	}
	config, err := core.LoadConfig()
	if err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}
	if _, ok := findAuth(config.AuthProfiles, profile.Name); ok {
		writeError(c, http.StatusConflict, fmt.Errorf("auth profile %q already exists", profile.Name))
		return
	}
	config.AuthProfiles = append(config.AuthProfiles, profile)
	if err := saveCheckedConfig(config); err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	writeJSON(c, http.StatusCreated, response{OK: true, Data: core.MaskAuthProfile(profile)})
}

func (s *Server) handleAuthUpdate(c *rux.Context) {
	if s.rejectReadonly(c) {
		return
	}
	name := c.Param("name")
	var profile core.AuthProfile
	if err := readJSON(c, &profile); err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	normalizeAPIAuth(&profile)
	if profile.Name == "" {
		profile.Name = name
	}
	if profile.Name != name {
		writeError(c, http.StatusBadRequest, errors.New("auth profile name cannot be changed by update API"))
		return
	}
	config, err := core.LoadConfig()
	if err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}
	idx, old, ok := findAuthIndex(config.AuthProfiles, name)
	if !ok {
		writeError(c, http.StatusNotFound, fmt.Errorf("auth profile %q not found", name))
		return
	}
	if profile.Password == "" && profile.PasswordEnc == "" {
		profile.PasswordEnc = old.PasswordEnc
	}
	config.AuthProfiles[idx] = profile
	if err := saveCheckedConfig(config); err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	writeOK(c, core.MaskAuthProfile(profile))
}

func (s *Server) handleAuthDelete(c *rux.Context) {
	if s.rejectReadonly(c) {
		return
	}
	name := c.Param("name")
	config, err := core.LoadConfig()
	if err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}
	if used := hostsUsingAuth(config.Hosts, name); len(used) > 0 {
		writeError(c, http.StatusConflict, fmt.Errorf("auth profile %q is used by hosts: %s", name, strings.Join(used, ", ")))
		return
	}
	idx, _, ok := findAuthIndex(config.AuthProfiles, name)
	if !ok {
		writeError(c, http.StatusNotFound, fmt.Errorf("auth profile %q not found", name))
		return
	}
	config.AuthProfiles = append(config.AuthProfiles[:idx], config.AuthProfiles[idx+1:]...)
	if err := saveCheckedConfig(config); err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	writeOK(c, map[string]string{"deleted": name})
}

func normalizeAPIAuth(profile *core.AuthProfile) {
	profile.Name = strings.TrimSpace(profile.Name)
	profile.User = strings.TrimSpace(profile.User)
	profile.KeyPath = strings.TrimSpace(profile.KeyPath)
	profile.Remark = strings.TrimSpace(profile.Remark)
}

func findAuth(profiles []core.AuthProfile, name string) (core.AuthProfile, bool) {
	_, profile, ok := findAuthIndex(profiles, name)
	return profile, ok
}

func findAuthIndex(profiles []core.AuthProfile, name string) (int, core.AuthProfile, bool) {
	name = strings.TrimSpace(name)
	for i, profile := range profiles {
		if strings.TrimSpace(profile.Name) == name {
			return i, profile, true
		}
	}
	return -1, core.AuthProfile{}, false
}

func hostsUsingAuth(hosts []core.Host, authName string) []string {
	var names []string
	for _, host := range hosts {
		if strings.TrimSpace(host.AuthRef) == authName {
			names = append(names, host.Name)
		}
	}
	return names
}
