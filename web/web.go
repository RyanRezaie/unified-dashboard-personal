// Package web embeds the frontend so the binary is the whole deployment
// unit — one static file to copy into a container, no asset directory to keep
// in sync with it.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:static
var files embed.FS

// FS returns the static directory rooted so that "/" serves dashboard.html.
func FS() fs.FS {
	sub, err := fs.Sub(files, "static")
	if err != nil {
		// Only reachable if the embed directive above is wrong, which is a
		// build-time mistake, not a runtime condition.
		panic(err)
	}
	return sub
}
