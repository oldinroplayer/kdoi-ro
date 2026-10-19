#!/usr/bin/env bash
# Builds KDoiRo into targets/<target>/: the game server, the web client, the
# game files it serves, and run.sh to start it all.
#
#   ./build.sh                 the release target (same as ./build.sh release)
#   ./build.sh dev             the dev target
#   ./build.sh all             both
#
# release: tests first, stripped server, minified client, game files copied (a
#          self-contained folder), listens on :7000.
# dev:     no tests, server with debug info, unminified client with source maps,
#          game files linked (quick to rebuild), listens on :7001 so it can run
#          next to release.
#
# Options (after the target):
#   --os linux --arch amd64    cross-compile the server (e.g. for a VPS)
#   --link-assets / --copy-assets / --no-assets   override how game files are added
#   --update                   then switch the running server to the new build: players
#                              see "updating", wait, and rejoin by themselves
#
# Each target keeps its character database in targets/<target>/save/ across builds.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGETS_DIR="$ROOT/targets"
GOOS_TARGET="$(go env GOOS)"
GOARCH_TARGET="$(go env GOARCH)"
ASSETS_OVERRIDE=""
UPDATE=""
TARGETS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    release|dev) TARGETS+=("$1"); shift ;;
    all) TARGETS+=(release dev); shift ;;
    --os) GOOS_TARGET="$2"; shift 2 ;;
    --arch) GOARCH_TARGET="$2"; shift 2 ;;
    --link-assets) ASSETS_OVERRIDE=link; shift ;;
    --copy-assets) ASSETS_OVERRIDE=copy; shift ;;
    --no-assets) ASSETS_OVERRIDE=none; shift ;;
    --update) UPDATE=1; shift ;;
    -h|--help) sed -n '2,23p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown option: $1 (see --help)" >&2; exit 1 ;;
  esac
done
[[ ${#TARGETS[@]} -gt 0 ]] || TARGETS=(release)

step() { printf '\n==> [%s] %s\n' "$TARGET" "$*"; }
need() { command -v "$1" >/dev/null || { echo "$1 is required but not installed" >&2; exit 1; }; }
need go
need npm
[[ -d "$ROOT/webclient/node_modules" ]] || (cd "$ROOT/webclient" && npm ci)

VERSION="$(git -C "$ROOT" describe --always --dirty 2>/dev/null || echo dev)"
BIN="kdoiro-server"
[[ "$GOOS_TARGET" == windows ]] && BIN="kdoiro-server.exe"

# Characters saved by the old single build/ folder move to the release target,
# but only while no server has the database open.
migrate_old_save() {
  local old="$ROOT/build/save" new="$1/save"
  [[ -f "$old/kdoiro.db" && ! -e "$new/kdoiro.db" ]] || return 0
  if command -v lsof >/dev/null && lsof "$old/kdoiro.db" >/dev/null 2>&1; then
    echo "    note: build/save/kdoiro.db is in use by a running server; stop it and build again to move your characters to targets/release/save/"
    return 0
  fi
  mv "$old"/kdoiro.db* "$new"/
  [[ -f "$old/characters.json" ]] && mv "$old/characters.json" "$new"/
  echo "    moved your characters from build/save/ to targets/release/save/ (build/ can be deleted)"
}

build_target() {
  local TARGET="$1" OUT="$TARGETS_DIR/$1" PORT ASSETS LDFLAGS
  case "$TARGET" in
    release) PORT=7000; ASSETS=copy; LDFLAGS="-s -w" ;;
    dev) PORT=7001; ASSETS=link; LDFLAGS="" ;;
  esac
  [[ -n "$ASSETS_OVERRIDE" ]] && ASSETS="$ASSETS_OVERRIDE"
  if [[ "$ASSETS" != none && ! -d "$ROOT/ro-client/data" ]]; then
    echo "ro-client/data not found: extract the client files first (see README.md), or pass --no-assets" >&2
    exit 1
  fi

  step "Cleaning targets/$TARGET (keeping save/)"
  mkdir -p "$OUT/save"
  rm -rf "$OUT/web" "$OUT/data" "$OUT/bgm" "$OUT/$BIN" "$OUT/run.sh" "$OUT/README.txt"
  [[ "$TARGET" == release ]] && migrate_old_save "$OUT"

  if [[ "$TARGET" == release ]]; then
    step "Testing the server"
    (cd "$ROOT/server" && go vet ./... && go test ./... >/dev/null)
  fi

  step "Building the server ($GOOS_TARGET/$GOARCH_TARGET)"
  # Pure Go (the SQLite driver needs no cgo), so cross-compiling just works.
  # -buildvcs=false: no git stamp (unused), so a git that won't run (e.g. an
  # unaccepted Xcode license on macOS) can't fail the build.
  (cd "$ROOT/server" && CGO_ENABLED=0 GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" \
    go build -trimpath -buildvcs=false -ldflags "$LDFLAGS" -o "$OUT/$BIN" ./cmd/server)

  step "Building the web client"
  (
    cd "$ROOT/webclient"
    npx tsc --noEmit
    if [[ "$TARGET" == release ]]; then
      npx vite build --outDir "$OUT/web" --emptyOutDir --logLevel warn
    else
      npx vite build --mode development --minify false --sourcemap --outDir "$OUT/web" --emptyOutDir --logLevel warn
    fi
  )

  case "$ASSETS" in
    copy)
      step "Copying the game files (maps, sprites, sounds, music)"
      cp -R "$ROOT/ro-client/data" "$OUT/data"
      [[ -d "$ROOT/ro-client/BGM" ]] && cp -R "$ROOT/ro-client/BGM" "$OUT/bgm"
      ;;
    link)
      step "Linking the game files"
      ln -s "$ROOT/ro-client/data" "$OUT/data"
      [[ -d "$ROOT/ro-client/BGM" ]] && ln -s "$ROOT/ro-client/BGM" "$OUT/bgm"
      ;;
    none)
      step "Leaving the game files out (put them in data/ and bgm/)"
      ;;
  esac

  step "Writing run.sh and README.txt"
  cat > "$OUT/run.sh" <<EOF
#!/usr/bin/env bash
# Starts KDoiRo ($TARGET): open http://localhost:$PORT (or this machine's address).
# Set ADDR to listen elsewhere, e.g. ADDR=:8080 ./run.sh
# When the server exits to update (exit code 75: the console's "update", or
# ./build.sh $TARGET --update), the new build starts right away.
cd "\$(dirname "\$0")"
while :; do
  ./$BIN -addr "\${ADDR:-:$PORT}" -web web -data data -bgm bgm \\
    -db save/kdoiro.db -import-json save/characters.json -pidfile save/server.pid "\$@"
  code=\$?
  [[ \$code -eq 75 ]] || exit \$code
  echo "==> Starting the new build"
done
EOF
  chmod +x "$OUT/run.sh"

  cat > "$OUT/README.txt" <<EOF
KDoiRo $VERSION — $TARGET build ($GOOS_TARGET/$GOARCH_TARGET)

Start:   ./run.sh            then open http://localhost:$PORT
Port:    ADDR=:8080 ./run.sh
Other server flags can be added after run.sh (./$BIN -h lists them),
e.g. ./run.sh -exp-rate 3 -drop-rate 2 to start with x3 EXP and x2 drops.

While it runs, its terminal takes commands:
  who                       who is online
  exp <name> <base> [job]   give an online player EXP (levels up live)
  zeny <name> <amount>      give an online player zeny (negative takes it away)
  reset <name> [stats|skills|all]  give an online player back their stat/skill points
  warpall <map> <x> <y>     move everyone there (offline characters at next login)
  saveall <map> <x> <y>     set everyone's respawn point there
  update                    restart into a newer build (after ./build.sh $TARGET);
                            players wait and rejoin by themselves
  exprate [rate]            show or set the monster EXP rate (exprate 2 = x2)
  droprate [rate]           show or set the monster drop rate (droprate 2 = x2)
  chatmode [server|local]   public chat to everyone online, or only nearby
  help                      all commands

Folders:
  web/   the browser client$( [[ "$TARGET" == dev ]] && echo " (unminified, with source maps)")
  data/  maps, sprites and sounds extracted from the Ragnarok Online client$( [[ "$ASSETS" == link ]] && echo " (a link to ro-client/data)")
  bgm/   background music
  save/  the character database (kdoiro.db); back this up

Players on other machines connect to http://<this machine>:$PORT; the page
connects back to the same server. Put it behind HTTPS (a reverse proxy) for
anything beyond a local network.

data/ and bgm/ come from Gravity's Ragnarok Online client: keep this build
for your own use and don't redistribute those folders.
EOF

  if [[ -n "$UPDATE" ]]; then
    local pid=""
    [[ -f "$OUT/save/server.pid" ]] && pid="$(cat "$OUT/save/server.pid")"
    # Only a live kdoiro-server: a stale pidfile may name some other process now.
    if [[ -n "$pid" ]] && [[ "$(ps -o comm= -p "$pid" 2>/dev/null)" == *kdoiro-server ]]; then
      step "Switching the running server (pid $pid) to the new build"
      kill -USR1 "$pid"
    else
      step "No running $TARGET server to update; start it with targets/$TARGET/run.sh"
    fi
  fi

  step "Done: targets/$TARGET"
  du -sh "$OUT"/* 2>/dev/null | sed 's|'"$OUT"'/|  |'
  echo "  start it with: targets/$TARGET/run.sh"
}

for TARGET in "${TARGETS[@]}"; do
  build_target "$TARGET"
done
