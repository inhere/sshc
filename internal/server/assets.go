package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gookit/rux/v2"
)

const fallbackIndexHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>sshc</title>
  <style>
    body{margin:0;font-family:system-ui,-apple-system,Segoe UI,sans-serif;background:#f7f8fa;color:#20242a}
    main{max-width:760px;margin:12vh auto;padding:0 24px}
    h1{font-size:28px;margin:0 0 12px}
    p{line-height:1.6;color:#4a5565}
    code{background:#eceff3;border-radius:4px;padding:2px 6px}
  </style>
</head>
<body>
<main>
  <h1>sshc serve</h1>
  <p>The local web server is running. Build the web UI and start with <code>--web-dir ./web/dist</code> to load the full interface.</p>
</main>
</body>
</html>`

func (s *Server) handleAssets(c *rux.Context) {
	if strings.HasPrefix(c.Req.URL.Path, "/api/") {
		http.NotFound(c.Resp, c.Req)
		return
	}
	if s.config.WebDir != "" && s.serveWebDir(c) {
		return
	}
	c.Resp.Header().Set("Content-Type", "text/html; charset=utf-8")
	c.Resp.WriteHeader(http.StatusOK)
	_, _ = c.Resp.Write([]byte(fallbackIndexHTML))
}

func (s *Server) serveWebDir(c *rux.Context) bool {
	requestPath := strings.TrimPrefix(c.Req.URL.Path, "/")
	if requestPath == "" {
		requestPath = "index.html"
	}
	cleanPath := filepath.Clean(filepath.FromSlash(requestPath))
	fullPath := filepath.Join(s.config.WebDir, cleanPath)
	if !withinDir(s.config.WebDir, fullPath) {
		http.NotFound(c.Resp, c.Req)
		return true
	}
	if fileInfo, err := os.Stat(fullPath); err == nil && !fileInfo.IsDir() {
		http.ServeFile(c.Resp, c.Req, fullPath)
		return true
	}
	indexPath := filepath.Join(s.config.WebDir, "index.html")
	if fileInfo, err := os.Stat(indexPath); err == nil && !fileInfo.IsDir() {
		http.ServeFile(c.Resp, c.Req, indexPath)
		return true
	}
	return false
}

func withinDir(root, path string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
