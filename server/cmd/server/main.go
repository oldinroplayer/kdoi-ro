// Command server runs the KDoiRo game server: a WebSocket endpoint at /ws.
package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"kdoiro/server/internal/account"
	"kdoiro/server/internal/game"
	"kdoiro/server/internal/gat"
	"kdoiro/server/internal/server"
	"kdoiro/server/internal/world"
)

func main() {
	addr := flag.String("addr", ":7000", "listen address")
	dataDir := flag.String("data", "../ro-client/data", "directory holding the extracted .gat map files")
	mapList := flag.String("maps", strings.Join(append([]string{"morocc", "moc_fild07", "moc_fild06"}, cityMaps...), ","),
		"comma-separated maps to load (content on other maps is left out)")
	dbPath := flag.String("db", "data/kdoiro.db", "SQLite character database (created if missing)")
	importJSON := flag.String("import-json", "data/characters.json",
		"old JSON character file to copy into an empty database on start (ignored if missing)")
	origins := flag.String("origins", "localhost:*,127.0.0.1:*", "comma-separated browser origin patterns allowed to connect")
	webDir := flag.String("web", "", "built web client to serve at / (with -data at /data and -bgm at /bgm); empty = API only")
	bgmDir := flag.String("bgm", "", "BGM folder served at /bgm when -web is set")
	flag.Parse()

	if err := run(*addr, *dataDir, *mapList, *dbPath, *importJSON, *origins, *webDir, *bgmDir); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(addr, dataDir, mapList, dbPath, importJSON, origins, webDir, bgmDir string) error {
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
	defer store.Close() // after the sessions below have saved their characters
	if _, statErr := os.Stat(importJSON); statErr == nil {
		n, err := store.ImportJSON(importJSON)
		if err != nil {
			return fmt.Errorf("importing %s: %w", importJSON, err)
		}
		if n > 0 {
			slog.Info("imported characters from JSON", "file", importJSON, "count", n, "db", dbPath)
		}
	}

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// From the November 2004 eAthena warps (reference/eathena-2004/npc/warps/):
	// cities/morroc.txt and fields/morroc_fild.txt.
	warps := []world.Warp{
		{Map: "morocc", X: 160, Y: 297, SpanX: 2, SpanY: 2, DestMap: "moc_fild07", DestX: 198, DestY: 25},
		{Map: "moc_fild07", X: 198, Y: 21, SpanX: 2, SpanY: 2, DestMap: "morocc", DestX: 160, DestY: 294},
		{Map: "moc_fild07", X: 381, Y: 201, SpanX: 2, SpanY: 16, DestMap: "moc_fild06", DestX: 28, DestY: 201},
		{Map: "moc_fild06", X: 18, Y: 198, SpanX: 1, SpanY: 18, DestMap: "moc_fild07", DestX: 378, DestY: 201},
	}
	// Shopkeepers around the Morroc spawn plaza. The Sword and Bow are for the 1st jobs.
	npcs := []world.NPC{
		{ID: 50001, Name: "Weapon Dealer", Sprite: "1_m_smith", Map: "morocc", X: 151, Y: 98, Dir: 4, Shop: []int{1201, 1207, 1101, 1701}},
		{ID: 50002, Name: "Armor Dealer", Sprite: "1_f_merchant_01", Map: "morocc", X: 161, Y: 98, Dir: 4, Shop: []int{2301, 2401, 2101, 2501, 2220}},
		{ID: 50003, Name: "Tool Dealer", Sprite: "1_m_merchant", Map: "morocc", X: 156, Y: 101, Dir: 4, Shop: []int{501}},
		// One NPC for all six 1st jobs (in RO each job has its own guild and test).
		{ID: 50004, Name: "Job Guide", Sprite: "1_m_jobguider", Map: "morocc", X: 146, Y: 93, Dir: 4, Kind: world.KindJobChange},
	}
	// Monsters from the November 2004 eAthena spawns
	// (reference/eathena-2004/npc/mobs/fields/morocc.txt).
	spawns := []world.Spawn{
		{Map: "moc_fild07", MobID: 1113, Count: 70}, // Drops
		{Map: "moc_fild07", MobID: 1047, Count: 50}, // Peco Peco Egg
		{Map: "moc_fild07", MobID: 1050, Count: 30}, // Picky (with shell)
		{Map: "moc_fild07", MobID: 1049, Count: 30}, // Picky
		{Map: "moc_fild07", X: 162, Y: 333, SpanX: 12, SpanY: 12, MobID: 1081, Count: 5, // Yellow Plant
			Delay: 6 * time.Minute, Variance: 3 * time.Minute},
		{Map: "moc_fild06", MobID: 1138, Count: 10}, // Magnolia
		{Map: "moc_fild06", MobID: 1160, Count: 5},  // Piere
		{Map: "moc_fild06", MobID: 1105, Count: 5},  // Deniro
		{Map: "moc_fild06", MobID: 1095, Count: 5},  // Andre
		{Map: "moc_fild06", MobID: 1055, Count: 30}, // Muka
		{Map: "moc_fild06", MobID: 1019, Count: 70}, // PecoPeco
		{Map: "moc_fild06", MobID: 1009, Count: 50}, // Condor
		{Map: "moc_fild06", MobID: 1097, Count: 10}, // Ant Egg
	}
	// Morroc's dealers also sell their indoor 2004 stock (world_data.go).
	for i := range npcs {
		for _, id := range morroccDealerStock[npcs[i].Name] {
			if !slices.Contains(npcs[i].Shop, id) {
				npcs[i].Shop = append(npcs[i].Shop, id)
			}
		}
	}
	// The cities beyond Morroc (world_data.go, generated by tools/gen_world.py),
	// minus anything on a map that isn't loaded.
	loaded := func(names ...string) bool {
		for _, n := range names {
			if maps[n] == nil {
				return false
			}
		}
		return true
	}
	for _, wp := range cityWarps {
		if loaded(wp.Map, wp.DestMap) {
			warps = append(warps, wp)
		}
	}
	for _, sp := range citySpawns {
		if loaded(sp.Map) {
			spawns = append(spawns, sp)
		}
	}
	for _, n := range cityNPCs {
		if loaded(n.Map) {
			npcs = append(npcs, n)
		}
	}
	w, err := world.New(world.Config{
		Maps:      maps,
		Warps:     warps,
		NPCs:      npcs,
		Spawns:    spawns,
		SavePoint: world.Location{Map: spawn.Map, X: spawn.X, Y: spawn.Y},
	})
	if err != nil {
		return err
	}
	worldCtx, stopWorld := context.WithCancel(context.Background())
	worldDone := make(chan struct{})
	go func() { w.Run(worldCtx); close(worldDone) }()

	sessionCtx, endSessions := context.WithCancel(context.Background())
	srv := server.New(sessionCtx, store, w, spawn, strings.Split(origins, ","))

	mux := http.NewServeMux()
	mux.Handle("/ws", srv)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok\n")) })
	mux.HandleFunc("GET /items.json", serveItems)
	mux.HandleFunc("GET /skills.json", serveSkills)
	if webDir != "" {
		if err := mountWeb(mux, webDir, dataDir, bgmDir); err != nil {
			endSessions()
			stopWorld()
			return err
		}
	}
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

// serveItems publishes the item database (stats, slots, prices) for clients.
// Names, descriptions and icons come from the client's own data files.
func serveItems(w http.ResponseWriter, _ *http.Request) {
	items := make([]game.ItemDef, 0, len(game.Items))
	for _, it := range game.Items {
		items = append(items, it)
	}
	slices.SortFunc(items, func(a, b game.ItemDef) int { return cmp.Compare(a.ID, b.ID) })
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // public, read-only data
	json.NewEncoder(w).Encode(items)
}

// serveSkills publishes the skill database (levels, SP, targets, requirements).
// Names, descriptions and icons come from the client's own data files.
func serveSkills(w http.ResponseWriter, _ *http.Request) {
	skills := make([]*game.SkillDef, 0, len(game.Skills))
	for _, s := range game.Skills {
		skills = append(skills, s)
	}
	slices.SortFunc(skills, func(a, b *game.SkillDef) int { return cmp.Compare(a.ID, b.ID) })
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // public, read-only data
	json.NewEncoder(w).Encode(skills)
}
