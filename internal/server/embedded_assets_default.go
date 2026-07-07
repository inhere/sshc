//go:build !embed_web

package server

import "github.com/gookit/rux/v2"

func serveEmbeddedAssets(_ *rux.Context) bool {
	return false
}
