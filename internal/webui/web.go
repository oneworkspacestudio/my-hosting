package webui

import (
	"embed"
	"io/fs"
)

//go:embed dist/* dist/assets/*
var assets embed.FS

func Files() fs.FS {
	f, e := fs.Sub(assets, "dist")
	if e != nil {
		panic(e)
	}
	return f
}
