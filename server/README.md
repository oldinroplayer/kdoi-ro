# KDoiRo server

A minimal Ragnarok-style game server in Go, spoken to over WebSocket.

- Log in with a character name + password. An unknown name creates the
  character (job Novice) at Morroc (156, 93); a known name checks the password.
- Players on the same map see each other appear, walk and leave.
- Walking is server-authoritative: the client asks for a destination, the server
  finds an 8-direction path on the map's `.gat` walk grid and broadcasts it.
- Warps move players between maps (Morroc's north gate ↔ moc_fild07).
- NPC shops near the Morroc spawn (weapons, armor, potions); new characters
  start with 2,000 zeny.
- Inventory, equipment (RO's 10 equip slots) and stats: STR/AGI/VIT/INT/DEX/LUK
  start at 1 with 48 status points; derived stats use pre-renewal formulas.
  Rules live in `internal/game`; the item database is served at `/items.json`.
- Monsters on moc_fild07 (rAthena's pre-renewal spawns and stats): they wander,
  fight back when hit, drop loot straight into the killer's inventory, and respawn.
- Combat: click to auto-attack at your ASPD; hit/flee, crits, DEF and VIT
  reduce damage. HP/SP regenerate; Red Potions and herbs heal. At 0 HP you're
  knocked out and can return to the save point (Morroc) with half HP.
- EXP is shared by damage dealt; level ups grant status points and refill HP/SP.
  Base level is capped at 30 for now (the EXP table stops there).
- Shops buy anything you carry (except worn equipment) at half price.
- The last position (map and cell) is saved on logout.

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
| `0x0003` | NpcClick | `npcId u32` — talk to an NPC (within 4 cells) |
| `0x0004` | ShopBuy | `npcId u32`, `count u16`, then `count` × (`itemId u16`, `amount u16`) |
| `0x0005` | Equip | `index u16` (inventory index) |
| `0x0006` | Unequip | `index u16` |
| `0x0007` | StatusUp | `stat u8` — 0 STR, 1 AGI, 2 VIT, 3 INT, 4 DEX, 5 LUK |
| `0x0008` | Attack | `targetId u32` — auto-attack a monster until it dies or you move |
| `0x0009` | UseItem | `index u16` — drink/eat a healing item |
| `0x000A` | Respawn | — return to the save point after being knocked out |
| `0x000B` | ShopSell | `npcId u32`, `count u16`, then `count` × (`index u16`, `amount u16`) |

The first packet must be Login (within 30 s, at most 5 attempts). Move is
ignored if sent less than 100 ms after the previous one, or if the cell is
unreachable within 32 steps.

### Server → client

| Op | Name | Fields |
|---|---|---|
| `0x8001` | LoginResult | `code u8` |
| `0x8002` | EnterMap | `selfId u32`, `map str`, `x u16`, `y u16`, `dir u8`, `job u16`, `speed u16`, then the look (5 × `u16`, as in PlayerAppear) |
| `0x8003` | PlayerAppear | `id u32`, `name str`, `job u16`, `x u16`, `y u16`, `dir u8`, `speed u16`, `dead u8`, then the look: item IDs (0 = none) of `weapon u16`, `shield u16`, `headTop u16`, `headMid u16`, `headLow u16` |
| `0x8004` | PlayerDisappear | `id u32`, `reason u8` (0 left, 1 died) — any unit, player or monster |
| `0x8005` | PlayerMove | `id u32`, `fromX u16`, `fromY u16`, `count u16`, then `count` × (`x u16`, `y u16`) — any unit |
| `0x8006` | PlayerStop | `id u32`, `x u16`, `y u16` — a unit stopped walking here (e.g. reached attack range) |
| `0x8007` | WarpPortal | `x u16`, `y u16` — a warp on the current map |
| `0x8008` | NpcAppear | `id u32`, `name str`, `sprite str` (under `data/sprite/npc/`), `x u16`, `y u16`, `dir u8` |
| `0x8009` | ShopOpen | `npcId u32`, `npcName str`, `count u16`, then `count` × (`itemId u16`, `price u32`) |
| `0x800A` | ShopResult | `code u8` — 0 ok, 1 not enough zeny, 2 overweight, 3 inventory full, 4 invalid / too far, 5 sold |
| `0x800B` | Inventory | `count u16`, then `count` × (`index u16`, `itemId u16`, `amount u16`, `equip u16` slot mask, 0 = not worn) |
| `0x800C` | Status | `baseLv u16`, `jobLv u16`, 6 × `stat u8`, 6 × `raiseCost u8`, `statusPoints u16`, `zeny u32`, then u16 each: `hp maxHp sp maxSp atk atk2 matkMin matkMax def def2 mdef mdef2 hit flee flee2 crit aspd`, then u32 each: `weight maxWeight baseExp baseNext jobExp jobNext` (next = 0 at the cap) |
| `0x800D` | Message | `text str` — a notice for the player (e.g. why an action failed, loot) |
| `0x800E` | MonsterAppear | `id u32`, `mobId u16`, `name str`, `sprite str` (under `data/sprite/몬스터/`), `x u16`, `y u16`, `dir u8`, `speed u16` |
| `0x800F` | Damage | `src u32`, `target u32`, `damage u32`, `flags u8` (1 miss, 2 crit), `motion u16` (attack anim ms), `hurt u16` (target flinch ms, 0 = none) |
| `0x8010` | UnitDead | `id u32` — a player was knocked out (monsters use PlayerDisappear reason 1) |
| `0x8011` | LevelUp | `id u32`, `kind u8` (0 base, 1 job) |
| `0x8012` | Heal | `id u32`, `amount u16` |
| `0x8013` | PlayerLook | `id u32`, then the look (5 × `u16`) — visible equipment changed |

LoginResult codes: `0` logged in, `1` character created, `2` wrong password,
`3` invalid name (4–23 of `A-Z a-z 0-9 _`), `4` invalid password (4–72 bytes),
`5` already online, `6` server error. After `0`/`1`, EnterMap follows, then a
WarpPortal per warp and an NpcAppear per NPC on the map, then a PlayerAppear for
everyone already there (plus a PlayerMove for anyone mid-walk), then Inventory
and Status. Inventory and Status are re-sent in full after every change.

**Warps:** stepping within ±2 cells of a warp moves the player to another map.
The client receives the same EnterMap sequence again and should unload the old
map and its players. Configured in `cmd/server/main.go`:

| From | To |
|---|---|
| morocc (160, 297) — north gate | moc_fild07 (198, 25) |
| moc_fild07 (198, 21) | morocc (160, 294) |

**Walking:** PlayerMove lists cells after `(fromX, fromY)`. Each straight step
takes `speed` ms, each diagonal step `speed × 1.4` ms. A new PlayerMove for the
same id replaces the previous walk; its first cell is the step already in
progress, so the client can keep animating smoothly.

**Directions** (`dir`): 0 N, 1 NW, 2 W, 3 SW, 4 S, 5 SE, 6 E, 7 NE.

**Job IDs:** 0 Novice.
