//go:build embed_web

package web

import (
	"embed"
	"io/fs"
)

//go:embed dist
var assets embed.FS

func FS() fs.FS {
	return assets
}
