package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

func FS() (fs.FS, error) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, err
	}
	if _, err := sub.Open("index.html"); err != nil {
		return nil, err
	}
	return sub, nil
}
