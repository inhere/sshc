package server

import "github.com/gookit/rux/v2"

type healthData struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Readonly bool   `json:"readonly"`
}

func (s *Server) handleHealth(c *rux.Context) {
	writeOK(c, healthData{
		Name:     AppName,
		Version:  "",
		Readonly: s.config.Readonly,
	})
}
