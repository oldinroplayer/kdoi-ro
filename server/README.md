# KDoiRo server

A minimal Ragnarok-style game server in Go, spoken to over WebSocket.

- Log in with a character name + password. An unknown name creates the
  character (job Novice) at Morroc (156, 93); a known name checks the password.
- Players on the same map see each other appear, walk and leave.
- Walking is server-authoritative: the client asks for a destination, the server
  finds an 8-direction path on the map's `.gat` walk grid and broadcasts it.
- The last position is saved on logout.

## Run

Needs the extracted map files (see `tools/grf_extract.py` at the repo root).

```bash
go run ./cmd/server
```

Flags:

| Flag | Default | |
|---|---|---|
| `-addr` | `:7000` | listen address; WebSocket at `/ws`, health check at `/healthz` |
| `-data` | `../ro-client/data` | directory with `<map>.gat` files |
| `-maps` | `morocc,moc_fild07` | maps to load |
| `-db` | `data/characters.json` | character file (created on first login) |
| `-origins` | `localhost:*,127.0.0.1:*` | browser origins allowed to connect |

```bash
go test -race ./...
```

## Protocol

One packet per WebSocket **binary** message: `u16` opcode, then fields.
All integers are little-endian. `str` is a `u8` byte length followed by UTF-8.
Coordinates are map cells, `y = 0` at the south edge.

### Client → server

| Op | Name | Fields |
|---|---|---|
| `0x0001` | Login | `name str`, `password str` |
| `0x0002` | Move | `x u16`, `y u16` — walk toward this cell |

The first packet must be Login (within 30 s, at most 5 attempts). Move is
ignored if sent less than 100 ms after the previous one, or if the cell is
unreachable within 32 steps.

### Server → client

| Op | Name | Fields |
|---|---|---|
| `0x8001` | LoginResult | `code u8` |
| `0x8002` | EnterMap | `selfId u32`, `map str`, `x u16`, `y u16`, `dir u8`, `job u16`, `speed u16` |
| `0x8003` | PlayerAppear | `id u32`, `name str`, `job u16`, `x u16`, `y u16`, `dir u8`, `speed u16` |
| `0x8004` | PlayerDisappear | `id u32` |
| `0x8005` | PlayerMove | `id u32`, `fromX u16`, `fromY u16`, `count u16`, then `count` × (`x u16`, `y u16`) |
| `0x8006` | PlayerStop | `id u32`, `x u16`, `y u16` (reserved for position corrections) |

LoginResult codes: `0` logged in, `1` character created, `2` wrong password,
`3` invalid name (4–23 of `A-Z a-z 0-9 _`), `4` invalid password (4–72 bytes),
`5` already online, `6` server error. After `0`/`1`, EnterMap follows, then a
PlayerAppear for everyone already on the map (plus a PlayerMove for anyone
mid-walk).

**Walking:** PlayerMove lists cells after `(fromX, fromY)`. Each straight step
takes `speed` ms, each diagonal step `speed × 1.4` ms. A new PlayerMove for the
same id replaces the previous walk; its first cell is the step already in
progress, so the client can keep animating smoothly.

**Directions** (`dir`): 0 N, 1 NW, 2 W, 3 SW, 4 S, 5 SE, 6 E, 7 NE.

**Job IDs:** 0 Novice.
