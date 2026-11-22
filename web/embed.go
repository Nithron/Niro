// Package web embeds the built dashboard (web/dist, produced by `pnpm build`)
// so the Go binary serves it without a separate web server.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist returns the built dashboard. It contains only .gitkeep until the
// dashboard has been built; the API then answers with a hint to build it.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // the embed pattern guarantees dist exists
	}
	return sub
}
