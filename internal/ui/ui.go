// Package ui embeds the frontend: plain HTML, CSS and JS with no build step.
package ui

import (
	"embed"
	"io/fs"
)

//go:embed static
var static embed.FS

// FS returns the frontend rooted at static/.
func FS() fs.FS {
	sub, err := fs.Sub(static, "static")
	if err != nil {
		panic(err)
	}
	return sub
}
