//go:build webdist

// Package web exposes the built browser client. Build it first with
// `make web`; the webdist tag embeds web/dist into the executable.
package web

import (
	"embed"
	"io/fs"
)

//go:embed dist
var dist embed.FS

// Assets returns the built client rooted at index.html.
func Assets() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil
	}
	return sub
}
