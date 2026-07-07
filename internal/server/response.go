package server

import (
	"encoding/json"
	"net/http"

	"github.com/gookit/rux/v2"
)

type response struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

func writeOK(c *rux.Context, data any) {
	writeJSON(c, http.StatusOK, response{OK: true, Data: data})
}

func writeJSON(c *rux.Context, status int, value any) {
	c.Resp.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.Resp.WriteHeader(status)
	_ = json.NewEncoder(c.Resp).Encode(value)
}
