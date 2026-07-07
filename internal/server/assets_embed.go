//go:build embed_web

package server

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/gookit/rux/v2"
	webassets "github.com/inhere/sshc/web"
)

func serveEmbeddedAssets(c *rux.Context) bool {
	dist, err := fs.Sub(webassets.FS(), "dist")
	if err != nil {
		return false
	}
	path := strings.TrimPrefix(c.Req.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if file, err := dist.Open(path); err == nil {
		_ = file.Close()
		http.FileServer(http.FS(dist)).ServeHTTP(c.Resp, c.Req)
		return true
	}
	if file, err := dist.Open("index.html"); err == nil {
		_ = file.Close()
		c.Req.URL.Path = "/"
		http.FileServer(http.FS(dist)).ServeHTTP(c.Resp, c.Req)
		return true
	}
	return false
}
