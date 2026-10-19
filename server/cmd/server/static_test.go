package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, srv *httptest.Server, path string) (int, string) {
	t.Helper()
	res, err := http.Get(srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func TestMountWeb(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "web", "index.html"), "client")
	write(t, filepath.Join(dir, "data", "Sprite", "인간족", "Body_A.spr"), "sprite")
	write(t, filepath.Join(dir, "music", "01.mp3"), "music")
	// The dev target links its game folders.
	if err := os.Symlink(filepath.Join(dir, "music"), filepath.Join(dir, "bgm")); err != nil {
		t.Fatal(err)
	}

	// The same routes as main, so pattern clashes show up here.
	mux := http.NewServeMux()
	mux.Handle("/ws", http.NotFoundHandler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("GET /items.json", serveItems)
	if err := mountWeb(mux, filepath.Join(dir, "web"), filepath.Join(dir, "data"), filepath.Join(dir, "bgm")); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(mux)
	defer srv.Close()

	for path, want := range map[string]string{
		"/": "client",
		"/data/sprite/%EC%9D%B8%EA%B0%84%EC%A1%B1/body_a.spr": "sprite", // lowercased by the client
		"/bgm/01.mp3": "music",
		"/healthz":    "ok",
	} {
		if code, body := get(t, srv, path); code != 200 || body != want {
			t.Errorf("GET %s = %d %q, want %q", path, code, body, want)
		}
	}
	if code, _ := get(t, srv, "/data/../web/index.html"); code != 404 {
		t.Errorf("path escape answered %d", code)
	}
	if code, _ := get(t, srv, "/data/missing.spr"); code != 404 {
		t.Errorf("missing file answered %d", code)
	}
	if err := mountWeb(http.NewServeMux(), filepath.Join(dir, "nope"), dir, ""); err == nil {
		t.Error("a missing web folder should be an error")
	}
}
