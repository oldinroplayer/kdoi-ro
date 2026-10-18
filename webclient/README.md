# KDoiRo web client

Browser client (TypeScript + three.js / WebGL) for the KDoiRo server.

- Login screen: character name + password (new names create a Novice).
- Renders the map straight from the original client files: GND ground with
  lightmaps, RSW-placed RSM models, animated water.
- Characters are SPR/ACT sprites (Novice body + head + shadow) on camera-facing
  billboards, with walk animation and 8 facing directions.
- Left-click to walk, right-drag to rotate the camera, mouse wheel to zoom.

## Run

Start the server (`cd ../server && go run ./cmd/server`), then:

```bash
npm install
npm run dev
```

Open http://localhost:5173. Map and sprite files are served from
`public/data`, a symlink to `../ro-client/data` (see `tools/grf_extract.py`).

## Layout

| Path | |
|---|---|
| `src/formats/` | Parsers for RO files: `gat`, `gnd`, `rsw`, `rsm`, `spr`, `act` |
| `src/render/` | three.js builders: ground, models, water, character sprites, shared shader |
| `src/net.ts` | WebSocket connection and binary protocol (see `server/README.md`) |
| `src/game.ts` | Scene, camera, players, walking interpolation, click picking |
| `src/main.ts` | Login screen and HUD |

RO space is x east, y **down**, z north. Everything map-related lives in one
group rotated 180° about X, which turns it into three.js's y-up space without
mirroring (`roToWorld` / `worldToRo` in `src/render/map.ts`).
