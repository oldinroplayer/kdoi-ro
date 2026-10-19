package main

import (
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// mountWeb serves the built web client at / and the extracted game files at
// /data and /bgm, so a single server runs the whole game (release builds).
// In development the Vite dev server does this instead.
func mountWeb(mux *http.ServeMux, webDir, dataDir, bgmDir string) error {
	if st, err := os.Stat(filepath.Join(webDir, "index.html")); err != nil || st.IsDir() {
		return fmt.Errorf("-web %s: no index.html (build the client first)", webDir)
	}
	data, err := newCaseInsensitiveFS(dataDir)
	if err != nil {
		return fmt.Errorf("-data %s: %w", dataDir, err)
	}
	mux.Handle("GET /data/", http.StripPrefix("/data", data))
	if bgmDir != "" {
		bgm, err := newCaseInsensitiveFS(bgmDir)
		if err != nil {
			return fmt.Errorf("-bgm %s: %w", bgmDir, err)
		}
		mux.Handle("GET /bgm/", http.StripPrefix("/bgm", bgm))
	}
	// Plain "/" (any method): "GET /" would clash with the method-less /ws and
	// /healthz routes. The file server only answers GET and HEAD anyway.
	mux.Handle("/", http.FileServer(http.Dir(webDir)))
	slog.Info("serving the web client", "web", webDir, "files", data.count)
	return nil
}

// caseInsensitiveFS serves a directory looking files up case-insensitively:
// the client lowercases RO's mixed-case paths, which only works as-is on
// case-insensitive file systems (macOS, Windows), not on Linux.
type caseInsensitiveFS struct {
	root  string
	files map[string]string // lowercase slash path -> real relative path
	count int
}

func newCaseInsensitiveFS(root string) (*caseInsensitiveFS, error) {
	// The dev target links data/ to ro-client/data; walking doesn't follow a
	// linked root, so resolve it first.
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	c := &caseInsensitiveFS{root: root, files: map[string]string{}}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		c.files[strings.ToLower(filepath.ToSlash(rel))] = rel
		c.count++
		return nil
	})
	return c, err
}

func (c *caseInsensitiveFS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rel, ok := c.files[strings.ToLower(strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/"))]
	if !ok {
		http.NotFound(w, r)
		return
	}
	// Game files don't change between releases' runs; let browsers keep them.
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, filepath.Join(c.root, rel))
}
