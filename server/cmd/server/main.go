// Command server runs the KDoiRo game server: a WebSocket endpoint at /ws.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"kdoiro/server/internal/account"
	"kdoiro/server/internal/gat"
	"kdoiro/server/internal/server"
	"kdoiro/server/internal/world"
)

func main() {
	addr := flag.String("addr", ":7000", "listen address")
	dataDir := flag.String("data", "../ro-client/data", "directory holding the extracted .gat map files")
	mapList := flag.String("maps", "morocc,moc_fild07", "comma-separated maps to load")
	dbPath := flag.String("db", "data/characters.json", "character database file")
	origins := flag.String("origins", "localhost:*,127.0.0.1:*", "comma-separated browser origin patterns allowed to connect")
	flag.Parse()

	if err := run(*addr, *dataDir, *mapList, *dbPath, *origins); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(addr, dataDir, mapList, dbPath, origins string) error {
	maps := map[string]*gat.Map{}
	for _, name := range strings.Split(mapList, ",") {
		name = strings.TrimSpace(name)
		m, err := gat.Load(filepath.Join(dataDir, name+".gat"))
		if err != nil {
			return err
		}
		maps[name] = m
		slog.Info("loaded map", "name", name, "size", [2]int{m.Width, m.Height})
	}
	spawn := server.Spawn{Map: "morocc", X: 156, Y: 93}
	if _, ok := maps[spawn.Map]; !ok {
		return errors.New("spawn map morocc is not loaded")
	}

	store, err := account.Open(dbPath)
	if err != nil {
		return err
	}

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	worldCtx, stopWorld := context.WithCancel(context.Background())
	w := world.New(maps)
	worldDone := make(chan struct{})
	go func() { w.Run(worldCtx); close(worldDone) }()

	sessionCtx, endSessions := context.WithCancel(context.Background())
	srv := server.New(sessionCtx, store, w, spawn, strings.Split(origins, ","))

	mux := http.NewServeMux()
	mux.Handle("/ws", srv)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok\n")) })
	httpSrv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	errCh := make(chan error, 1)
	go func() { errCh <- httpSrv.ListenAndServe() }()
	slog.Info("listening", "addr", addr, "ws", "/ws")

	select {
	case err := <-errCh:
		endSessions()
		stopWorld()
		return err
	case <-sigCtx.Done():
	}

	// Shut down in order: stop accepting, end sessions (each saves its
	// character via the world), then stop the world.
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpSrv.Shutdown(shutdownCtx)
	endSessions()
	srv.Wait()
	stopWorld()
	<-worldDone
	return nil
}
