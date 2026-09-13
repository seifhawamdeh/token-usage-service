package dashboard

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web/dist/*
var staticFS embed.FS

func StaticHandler() http.Handler {
	sub, err := fs.Sub(staticFS, "web/dist")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(sub))
}
