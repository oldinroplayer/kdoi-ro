#!/usr/bin/env bash
# Builds a release of KDoiRo into build/: the game server binary, the web
# client, the extracted game files it serves, and run.sh to start it all.
#
#   ./build.sh                      build for this machine
#   ./build.sh --os linux --arch amd64   cross-compile the server (e.g. for a VPS)
#   ./build.sh --link-assets        symlink the game files instead of copying (~400 MB)
#   ./build.sh --no-assets          leave the game files out (add data/ and bgm/ yourself)
#
# build/save/ (the character database) is kept between builds.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT="$ROOT/build"
GOOS_TARGET="$(go env GOOS)"
GOARCH_TARGET="$(go env GOARCH)"
ASSETS=copy

while [[ $# -gt 0 ]]; do
  case "$1" in
    --os) GOOS_TARGET="$2"; shift 2 ;;
    --arch) GOARCH_TARGET="$2"; shift 2 ;;
    --link-assets) ASSETS=link; shift ;;
    --no-assets) ASSETS=none; shift ;;
    -h|--help) sed -n '2,11p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown option: $1 (see --help)" >&2; exit 1 ;;
  esac
done

step() { printf '\n==> %s\n' "$*"; }
need() { command -v "$1" >/dev/null || { echo "$1 is required but not installed" >&2; exit 1; }; }
need go
need npm

if [[ "$ASSETS" != none && ! -d "$ROOT/ro-client/data" ]]; then
  echo "ro-client/data not found: extract the client files first (see README.md), or pass --no-assets" >&2
  exit 1
fi

step "Cleaning build/ (keeping build/save)"
mkdir -p "$OUT/save"
rm -rf "$OUT/web" "$OUT/data" "$OUT/bgm" "$OUT/kdoiro-server" "$OUT/kdoiro-server.exe" "$OUT/run.sh" "$OUT/README.txt"

VERSION="$(git -C "$ROOT" describe --always --dirty 2>/dev/null || echo dev)"
BIN="kdoiro-server"
[[ "$GOOS_TARGET" == windows ]] && BIN="kdoiro-server.exe"

step "Testing and building the server ($GOOS_TARGET/$GOARCH_TARGET, $VERSION)"
(
  cd "$ROOT/server"
  go vet ./...
  go test ./... >/dev/null
  # Pure Go (the SQLite driver needs no cgo), so cross-compiling just works.
  CGO_ENABLED=0 GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" \
    go build -trimpath -ldflags "-s -w" -o "$OUT/$BIN" ./cmd/server
)

step "Building the web client"
(
  cd "$ROOT/webclient"
  [[ -d node_modules ]] || npm ci
  npx tsc --noEmit
  npx vite build --outDir "$OUT/web" --emptyOutDir --logLevel warn
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
    step "Leaving the game files out (put them in build/data and build/bgm)"
    ;;
esac

step "Writing run.sh and README.txt"
cat > "$OUT/run.sh" <<EOF
#!/usr/bin/env bash
# Starts KDoiRo: open http://localhost:7000 (or this machine's address) in a browser.
# Set ADDR to listen elsewhere, e.g. ADDR=:8080 ./run.sh
cd "\$(dirname "\$0")"
exec ./$BIN -addr "\${ADDR:-:7000}" -web web -data data -bgm bgm \\
  -db save/kdoiro.db -import-json save/characters.json "\$@"
EOF
chmod +x "$OUT/run.sh"

cat > "$OUT/README.txt" <<EOF
KDoiRo $VERSION ($GOOS_TARGET/$GOARCH_TARGET)

Start:   ./run.sh            then open http://localhost:7000
Port:    ADDR=:8080 ./run.sh
Other server flags can be added after run.sh (./$BIN -h lists them).

Folders:
  web/   the browser client
  data/  maps, sprites and sounds extracted from the Ragnarok Online client
  bgm/   background music
  save/  the character database (kdoiro.db); back this up

Players on other machines connect to http://<this machine>:7000; the page
connects back to the same server. Put it behind HTTPS (a reverse proxy) for
anything beyond a local network.

data/ and bgm/ come from Gravity's Ragnarok Online client: keep this build
for your own use and don't redistribute those folders.
EOF

step "Done"
du -sh "$OUT"/* 2>/dev/null | sed 's|'"$OUT"'/|  build/|'
echo
echo "Start it with: build/run.sh"
