//go:build !webdist

// Package web exposes the built browser client. This build carries no assets;
// use `make web` to embed them or pass --assets to the launcher.
package web

import "io/fs"

// Assets returns nil when the client was not embedded.
func Assets() fs.FS { return nil }
