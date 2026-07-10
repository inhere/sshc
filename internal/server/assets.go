package server

import (
	"net/http"
	"os"
	"path"
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
	if serveEmbeddedAssets(c) {
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
	if hasPathTraversal(requestPath) {
		http.NotFound(c.Resp, c.Req)
		return true
	}
	root, err := os.OpenRoot(s.config.WebDir)
	if err != nil {
		return false
	}
	defer root.Close()

	if isSPAAssetRequest(requestPath) {
		return serveIndexFile(c, root)
	}
	http.FileServerFS(root.FS()).ServeHTTP(c.Resp, c.Req)
	return true
}

func isSPAAssetRequest(requestPath string) bool {
	cleanPath := strings.Trim(path.Clean("/"+requestPath), "/")
	if cleanPath == "" {
		return true
	}
	return !strings.Contains(path.Base(cleanPath), ".")
}

func serveIndexFile(c *rux.Context, root *os.Root) bool {
	file, err := root.Open("index.html")
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return false
	}
	http.ServeContent(c.Resp, c.Req, "index.html", info.ModTime(), file)
	return true
}

func hasPathTraversal(requestPath string) bool {
	for part := range strings.SplitSeq(strings.ReplaceAll(requestPath, "\\", "/"), "/") {
		if part == ".." {
			return true
		}
	}
	return false
}
