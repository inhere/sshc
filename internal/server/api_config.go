package server

import (
	"net/http"

	"github.com/gookit/rux/v2"
	"github.com/inhere/sshc/internal/core"
)

type configSummary struct {
	Path       string             `json:"path"`
	LogsPath   string             `json:"logs_path"`
	Defaults   core.Defaults      `json:"defaults"`
	HostCount  int                `json:"host_count"`
	AuthCount  int                `json:"auth_count"`
	Readonly   bool               `json:"readonly"`
	Doctor     []core.DoctorIssue `json:"doctor"`
	DoctorOK   bool               `json:"doctor_ok"`
	Source     string             `json:"source"`
	APIVersion string             `json:"api_version"`
}

func (s *Server) handleConfigSummary(c *rux.Context) {
	config, err := core.LoadConfig()
	if err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}
	path, err := core.StorePath()
	if err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}
	issues := core.CheckConfig(*config)
	writeOK(c, configSummary{
		Path:       path,
		LogsPath:   config.LogsPath,
		Defaults:   config.Defaults,
		HostCount:  len(config.Hosts),
		AuthCount:  len(config.AuthProfiles),
		Readonly:   s.config.Readonly,
		Doctor:     issues,
		DoctorOK:   !core.HasDoctorErrors(issues),
		Source:     "config",
		APIVersion: "v1",
	})
}
