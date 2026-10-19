# KDoiRo server

A minimal Ragnarok-style game server in Go, spoken to over WebSocket.

- Log in with a character name + password. An unknown name creates the
  character (job Novice) at the camp outside Payon Cave (pay_arche 46, 131);
  a known name checks the password.
- Players on the same map see each other appear, walk and leave.
- Walking is server-authoritative: the client asks for a destination, the server
  finds an 8-direction path on the map's `.gat` walk grid and broadcasts it.
- Warps move players between maps (Morroc's north gate ↔ moc_fild07).
- Five more cities from the 2004 data — Prontera, Izlude, Geffen, Payon and
  Alberta — each with every land exit it had (Prontera: castle north, fields
  south/west/east; Geffen: north/east/west; Payon: Archer Village north, forest
  south, east field, guild grounds west; Izlude and Alberta: one field each),
  their 2004 monsters and outdoor shops. Beyond them: prt_fild07 (Rockers, west
  of Prontera's south field), moc_fild12 (south of Morroc) and Payon Cave
  level 1 (pay_dun00: Zombies, Skeletons, Familiars, Poporings), entered from
  the Archer Village north of Payon. A camp by the cave entrance has a Job
  Guide and Weapon, Armor and Tool Dealers with Payon's stock (not in 2004 RO). Kafra Employees save your respawn point
  and teleport between cities for free (2004 destinations; the prices are waived).
- Each town's 2004 Weapon, Armor and Tool Dealers (indoors in RO, where
  KDoiRo doesn't go) sell their full stock: beside the Warper in the new cities,
  and through Morroc's dealers near the spawn. Arrows and Tool Dealer items
  that don't heal HP yet (wings, SP potions) are left out.
- A Warper stands at the centre of every town (Morroc included) and sends you
  to any other town's Warper for free. Positions are in `WARPERS` in
  `tools/gen_world.py`.
- NPC shops near the Morroc spawn (weapons, armor, potions); new characters
  start with 2,000 zeny.
- Inventory, equipment (RO's 10 equip slots) and stats: STR/AGI/VIT/INT/DEX/LUK
  start at 1 with 48 status points; derived stats use pre-renewal formulas.
  Rules live in `internal/game`; the item database is served at `/items.json`.
- Monsters on moc_fild07 and moc_fild06 (2004 spawns and stats): they wander,
  fight back when hit, drop loot on the ground around the corpse, and respawn.
  Whoever dealt the most damage gets first pick for 3 s; items vanish after 60 s.
  Players pick items up by clicking them (walking over first) and can drop
  inventory items on the ground for anyone to take.
  Monsters with the Assist flag (Andre, Deniro, Piere, PecoPeco, Condor) join in
  when one of their kind within 10 cells is attacked.
- Appearance: male or female, and the 2004 client's 19 hairstyles in 9
  colours. New characters (and ones from before hairstyles) have none chosen,
  so the client asks on their first login; it can be changed any time from the
  menu. Stored in the `hair`, `hair_color` and `female` columns (schema
  version 3); characters from before genders are male.
- Parties (RO 2004 style): a leader founds one by name, invites online players
  (up to 12), and can kick members or switch EXP between each take and even
  share (even share needs the online members within 10 base levels, and splits a
  kill's EXP among members alive on that map). Members see each other's map,
  position and HP (refreshed every second), chat with `%`, and share first pick
  of each other's drops. Parties are saved (`parties`, `party_members`, schema
  version 4); a leaving leader hands over to the next member.
- Emoticons: the client's 47 (`/!`, `/?`, `/ho`... or Alt+1–0), shown to
  everyone within 14 cells, at most one a second.
- Server console: the terminal running the server takes commands —
  `exp <name> <base> [job]` gives an online character EXP live (with level ups
  and points), `zeny <name> <amount>` gives (or, negative, takes) zeny,
  `reset <name> [stats|skills|all]` gives back their stat and/or skill points
  (the Reset Maid at the Payon Cave camp does the same for free),
  `warpall <map> <x> <y>` moves every online player there now and every saved
  character there for their next login, `saveall <map> <x> <y>` sets everyone's
  respawn point there, `exprate [rate]` shows or sets the monster EXP rate
  live (`exprate 2` = x2, `1.5` works too; `-exp-rate 3` starts at x3),
  `droprate [rate]` does the same for monster drop chances,
  `chatmode server|local` makes public chat reach everyone online or only
  players within 14 cells (the default), `update` restarts into a newer build
  without logging anyone out (below),
  `who` lists who's online, `help` lists them all.
- Maps sleep while no player is on them: their monsters stop thinking,
  wandering and fighting until someone arrives, and stay frozen where they
  were. A map stays awake for a minute after its last player leaves, so warping
  out and back finds it unchanged. Respawn timers and floor item expiry keep
  running. (`internal/world/activity.go`)
- Combat: click to auto-attack at your ASPD; hit/flee, crits, DEF and VIT
  reduce damage. HP/SP regenerate; Red Potions and herbs heal. At 0 HP you're
  knocked out and can return to the save point (their Kafra save, or the
  Payon Cave camp) with half HP.
- EXP is shared by damage dealt; level ups grant status points and refill HP/SP.
  Base level is capped at 30 for now (the EXP table stops there).
- Shops buy anything you carry (except worn equipment) at half price.
- Jobs: Novice plus the six 1st jobs (Swordman, Mage, Archer, Acolyte, Merchant,
  Thief). The Job Guide in Morroc (146, 93) changes a Novice at Job Level 10
  with every skill point spent (the 2004 job quests' rule). Each job has its own
  HP/SP growth, weight limit, attack speed per weapon type, job level stat
  bonuses and EXP table (base levels to 99, 1st-job levels to 50), generated
  from the 2004 data by `tools/gen_jobs.py`. Equipment checks the 2004 item_db
  job masks (e.g. Acolytes can't use daggers).
- Skills: 1 skill point per job level; the skill database is served at
  `/skills.json`. Novices have Basic Skill (Lv 3 unlocks sitting, as in RO) and
  First Aid (a granted quest skill: 3 SP heals 5 HP). Skills can have SP costs,
  cast times (moving or being hit cancels the cast) and after-cast delays.
  Each 1st job has its 2004 skills (numbers from `tools/gen_skills.py`): Bash,
  Magnum Break (fire burst with knockback), Sword Mastery, HP Recovery; Fire,
  Cold and Lightning Bolt, SP Recovery; Double Strafe, Owl's and Vulture's Eye;
  Heal, Increase AGI, Blessing, Divine Protection; Mammonite (costs zeny),
  Discount, Overcharge, Enlarge Weight Limit; Double Attack, Improve Dodge,
  Envenom. Targeted skills walk into range first; buffs expire on a timer.
  Increase AGI also makes walking 25% faster while it lasts (eAthena 2004).
- Elements and races (2004 mob_db and attr_fix.txt): every monster has an
  element and level, a race and a size. Attacks and skills hit with their
  element (normal attacks, Bash, Mammonite and Double Strafe neutral; Magnum
  Break and Fire Bolt fire, Cold Bolt water, Lightning Bolt wind, Envenom
  poison), scaled by the element table: fire does 125% to the undead, neutral
  25% to ghosts, and 0 where a monster is immune. Heal can be aimed at
  undead-element monsters for half its amount as holy damage. Divine Protection
  blocks 3 damage per level from undead and demon monsters.
- Aggressive monsters (mode flag 4: Zombies, for example) attack the nearest
  player within their view range; cast sensors (flag 16: Skeletons) go after a
  player who starts casting nearby. Others still only fight back.
- Chat: lines carry 14 cells (eAthena's AREA_SIZE); whispers reach anyone online
  by name. Chat rooms: a titled sign over the owner's head (public or with a
  password, 2–20 people); members chat privately, can't walk or fight, and the
  owner can kick (kicked players can't rejoin), hand over or edit the room.
  At most 5 chat lines per 3 seconds.
- Sitting doubles HP/SP regeneration (every 3 s / 4 s instead of 6 s / 8 s);
  sitting players can't walk, attack or use skills until they stand up, but can
  turn to face where they click.
- Characters live in a SQLite database (`data/kdoiro.db`, pure-Go driver, no
  cgo): a `characters` table plus `inventory` and `skills` tables, written in one
  transaction on logout. The schema is versioned with `PRAGMA user_version`;
  add a migration to `migrations` in `internal/account/store.go` to change it.
  On first start an existing `data/characters.json` (the old store) is imported
  into the empty database, passwords included; the JSON file is left as is.

## Run

Needs the extracted map files (see `tools/grf_extract.py` at the repo root).

```bash
go run ./cmd/server
```

Flags:

| Flag | Default | |
|---|---|---|
| `-addr` | `:7000` | listen address; WebSocket at `/ws`, health check at `/healthz`, players online and rates at `/status.json` |
| `-data` | `../ro-client/data` | directory with `<map>.gat` files |
| `-maps` | `morocc,moc_fild07,moc_fild06` | maps to load |
| `-db` | `data/kdoiro.db` | SQLite character database (created if missing) |
| `-import-json` | `data/characters.json` | old JSON store to import into an empty database (ignored if missing) |
| `-web` | (none) | built web client to serve at `/`, with `-data` served at `/data` (looked up case-insensitively) and `-bgm` at `/bgm`; empty = game API only (development uses Vite) |
| `-bgm` | (none) | music folder served at `/bgm` when `-web` is set |
| `-origins` | `localhost:*,127.0.0.1:*` | browser origins allowed to connect |
| `-exp-rate` | `1` | monster EXP multiplier (the console's `exprate` changes it live) |
| `-drop-rate` | `1` | monster drop chance multiplier, capped at a sure drop (the console's `droprate` changes it live) |
| `-pidfile` | (none) | write the process ID here, so `./build.sh <target> --update` can signal it |

```bash
go test -race ./...
```

### Restarting without logging players out

Every clean shutdown is resumable. That covers Ctrl-C, SIGTERM, closing the
terminal (SIGHUP), and an update: SIGUSR1, or `update` in the server console,
which `./build.sh dev --update` and `./dev.sh` (with a server already running)
send.

The server stops accepting connections and saves everyone. Each player gets a
ServerUpdating packet with a one-time resume token, and the connection closes
with code 4000. After an update the server exits with code 75 and `run.sh`
starts the new build at once. After Ctrl-C, start it again within the wait.
Meanwhile each page shows "Server restarting" and checks `/healthz` until a
server answers (up to 30 s, then it offers to keep waiting). It then reloads
into the current client, logs back in with the token, and turns auto attack
back on if it was running. Characters come back where they were. Monsters,
ground items, chat rooms, buffs and console settings start fresh.

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
| `0x000C` | Sit | `sit u8` — 1 sit down, 0 stand up |
| `0x000E` | Learn | `skillId u16` — spend a skill point |
| `0x0010` | NpcReply | `npcId u32`, `choice u8` — the option picked in an NpcDialog (255 = walked away) |
| `0x0011` | Chat | `text str` — said out loud, or to your chat room if you're in one |
| `0x0012` | Whisper | `to str`, `text str` |
| `0x0013` | RoomCreate | `title str`, `limit u8` (2–20), `public u8`, `password str` (needed when not public) |
| `0x0014` | RoomJoin | `roomId u32`, `password str` |
| `0x0015` | RoomLeave | — |
| `0x0016` | RoomKick | `name str` (owner only) |
| `0x0017` | RoomUpdate | same fields as RoomCreate (owner only) |
| `0x0018` | RoomOwner | `name str` — hand the room to another member (owner only) |
| `0x0019` | PickUp | `floorItemId u32` — walk over and pick up an item on the ground |
| `0x001A` | DropItem | `index u16`, `amount u16` — drop inventory items at your feet |
| `0x001C` | Emote | `type u8` (0–46) |
| `0x001D` | PartyCreate | `name str` |
| `0x001E` | PartyInvite | `name str` (leader only) |
| `0x001F` | PartyReply | `partyId u32`, `accept u8` |
| `0x0020` | PartyLeave | — |
| `0x0021` | PartyKick | `name str` (leader only) |
| `0x0022` | PartyChat | `text str` |
| `0x0023` | PartyEXP | `even u8` (leader only) |
| `0x0024` | Resume | `token str` — instead of Login, after a ServerUpdating (one use, 2 minutes) |
| `0x001B` | SetAppearance | `style u8` (1–19), `color u8` (0–8), `female u8` — change hairstyle, hair colour and gender; everyone on the map gets a PlayerLook |
| `0x000F` | UseSkill | `skillId u16`, `level u16` (0 = highest learned), `targetId u32` (ignored for self skills) |
| `0x000D` | Turn | `dir u8` — face a direction without moving (0 N, 1 NW … 7 NE); ignored while walking |

The first packet must be Login (within 30 s, at most 5 attempts). Move is
ignored if sent less than 100 ms after the previous one, or if the cell is
unreachable within 32 steps.

### Server → client

| Op | Name | Fields |
|---|---|---|
| `0x8001` | LoginResult | `code u8` |
| `0x8002` | EnterMap | `selfId u32`, `map str`, `x u16`, `y u16`, `dir u8`, `job u16`, `speed u16`, then the look (8 × `u16`, as in PlayerAppear) |
| `0x8003` | PlayerAppear | `id u32`, `name str`, `job u16`, `x u16`, `y u16`, `dir u8`, `speed u16`, `dead u8`, then the look: item IDs (0 = none) of `weapon u16`, `shield u16`, `headTop u16`, `headMid u16`, `headLow u16`, then `hair u16` (1–19, 0 = not chosen yet), `hairColor u16` (0–8) and `female u16` (1 = female), then `sitting u8` |
| `0x8004` | PlayerDisappear | `id u32`, `reason u8` (0 left, 1 died) — any unit, player or monster |
| `0x8005` | PlayerMove | `id u32`, `fromX u16`, `fromY u16`, `count u16`, then `count` × (`x u16`, `y u16`) — any unit |
| `0x8006` | PlayerStop | `id u32`, `x u16`, `y u16` — a unit stopped walking here (e.g. reached attack range) |
| `0x8007` | WarpPortal | `x u16`, `y u16` — a warp on the current map |
| `0x8008` | NpcAppear | `id u32`, `name str`, `sprite str` (under `data/sprite/npc/`), `x u16`, `y u16`, `dir u8` |
| `0x8009` | ShopOpen | `npcId u32`, `npcName str`, `count u16`, then `count` × (`itemId u16`, `price u32`, after Discount), then `sellRate u16` (percent of half price paid when selling; above 100 with Overcharge) |
| `0x800A` | ShopResult | `code u8` — 0 ok, 1 not enough zeny, 2 overweight, 3 inventory full, 4 invalid / too far, 5 sold |
| `0x800B` | Inventory | `count u16`, then `count` × (`index u16`, `itemId u16`, `amount u16`, `equip u16` slot mask, 0 = not worn) |
| `0x800C` | Status | `baseLv u16`, `jobLv u16`, 6 × `stat u8`, 6 × `raiseCost u8`, `statusPoints u16`, `zeny u32`, then u16 each: `hp maxHp sp maxSp atk atk2 matkMin matkMax def def2 mdef mdef2 hit flee flee2 crit aspd`, then u32 each: `weight maxWeight baseExp baseNext jobExp jobNext` (next = 0 at the cap), then `job u16` and 6 × `jobBonus u8` (STR..LUK) |
| `0x800D` | Message | `text str` — a notice for the player (e.g. why an action failed, loot) |
| `0x800E` | MonsterAppear | `id u32`, `mobId u16`, `name str`, `sprite str` (under `data/sprite/몬스터/`), `x u16`, `y u16`, `dir u8`, `speed u16` |
| `0x800F` | Damage | `src u32`, `target u32`, `damage u32`, `flags u8` (1 miss, 2 crit), `motion u16` (attack anim ms), `hurt u16` (target flinch ms, 0 = none), `skillId u16` (0 = normal attack), `hits u8` (blows the damage is split over: bolts, Double Strafe, Double Attack) |
| `0x8010` | UnitDead | `id u32` — a player was knocked out (monsters use PlayerDisappear reason 1) |
| `0x8011` | LevelUp | `id u32`, `kind u8` (0 base level, 1 job level, 2 job change) |
| `0x8012` | Heal | `id u32`, `amount u16` |
| `0x8013` | PlayerLook | `id u32`, then the look (8 × `u16`) — visible equipment, hair or gender changed |
| `0x8014` | PlayerSit | `id u32`, `sitting u8` |
| `0x8015` | PlayerTurn | `id u32`, `dir u8` |
| `0x8016` | SkillList | `skillPoints u16`, `count u16`, then `count` × (`skillId u16`, `level u8`) — your whole tree, 0 = not learned |
| `0x8017` | SkillCast | `src u32`, `target u32`, `skillId u16`, `castMs u16` — casting started (`skillId` 0 = cancelled) |
| `0x8019` | NpcDialog | `npcId u32`, `name str`, `text` (`u16` length + UTF-8, may contain newlines), `count u8`, then `count` × `option str` (0 options = just Close) |
| `0x801A` | PlayerJob | `id u32`, `job u16` — a player changed job (new body sprite) |
| `0x801B` | Buff | `id u32`, `skillId u16`, `level u8`, `ms u32` — a buff started (or ended, `ms` 0); sent only to the buffed player |
| `0x801C` | ChatMessage | `id u32` (speaker), `kind u8` (0 out loud, 1 in a chat room, 2 to the party), `name str`, `text str` |
| `0x801D` | WhisperFrom | `from str`, `text str` |
| `0x801E` | WhisperResult | `code u8` (0 delivered, 1 not online, 2 yourself), `to str` |
| `0x801F` | RoomSign | `roomId u32`, `ownerId u32`, `title str`, `members u8`, `limit u8`, `public u8` — show or update a room's sign |
| `0x8020` | RoomGone | `roomId u32` |
| `0x8021` | RoomEnter | `roomId u32`, `title str`, `limit u8`, `public u8`, `owner str`, `count u8`, then `count` × `name str` — you're in |
| `0x8022` | RoomMember | `change u8` (0 left, 1 joined, 2 kicked), `name str` |
| `0x8023` | RoomResult | `code u8`: 1 full, 2 wrong password, 3 kicked earlier, 4 gone or too far, 5 already in a room, 6 can't now, 7 bad settings |
| `0x8024` | RoomLeft | `reason u8` (0 left, 1 kicked) — you're out |
| `0x8025` | RoomSettings | `title str`, `limit u8`, `public u8`, `owner str` — sent to members after an edit or owner change |
| `0x8026` | FloorItem | `id u32`, `itemId u16`, `amount u16`, `x u16`, `y u16`, `subX u8`, `subY u8` (0–15 within the cell), `dropped u8` (1 = just fell) |
| `0x8027` | FloorItemGone | `id u32` — picked up or expired |
| `0x8028` | PlayerPickUp | `id u32` — play a player's pick-up motion |
| `0x8029` | Emotion | `id u32`, `type u8` — an emoticon over a unit's head |
| `0x802A` | PartyInfo | `partyId u32`, `name str`, `leader str`, `evenExp u8`, `count u8`, then `count` × (`name str`, `online u8`, `map str`, `x u16`, `y u16`, `hp u32`, `maxHp u32`, `job u16`, `baseLevel u16`; offline members have only a name) |
| `0x802B` | PartyInvited | `partyId u32`, `partyName str`, `from str` |
| `0x802C` | PartyLeft | — you're no longer in a party |
| `0x802D` | PlayerSpeed | `id u32`, `speed u16` (ms per straight cell) — a player's walk speed changed (Increase AGI: 25% faster while it lasts) |
| `0x802E` | ServerUpdating | `wait u16` (seconds), `token str` — the server is restarting into a new build; the connection then closes with code 4000. Wait for the server to answer again, reload and send Resume |
| `0x802F` | Effect | `id u32`, `kind u8` — a one-off effect on a unit: `1` teleporting away (Fly/Butterfly Wing) |
| `0x8018` | SkillUse | `src u32`, `target u32`, `skillId u16`, `level u8` — a skill went off (its effect follows, e.g. Heal) |

LoginResult codes: `0` logged in, `1` character created, `2` wrong password,
`3` invalid name (4–23 of `A-Z a-z 0-9 _`), `4` invalid password (4–72 bytes),
`5` already online, `6` server error, `7` resume token expired or used. After `0`/`1`, EnterMap follows, then a
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
| moc_fild07 (381, 201) ±2,16 — east edge | moc_fild06 (28, 201) |
| moc_fild06 (18, 198) ±1,18 — west edge | moc_fild07 (378, 201) |

Warps, spawns and monster stats follow eAthena's November 2004 data in
`../reference/eathena-2004/` (the client is from August 2004; later RO patches
renumbered the Sograt fields, e.g. moc_fild07's east exit became moc_fild20).

**Walking:** PlayerMove lists cells after `(fromX, fromY)`. Each straight step
takes `speed` ms, each diagonal step `speed × 1.4` ms. A new PlayerMove for the
same id replaces the previous walk; its first cell is the step already in
progress, so the client can keep animating smoothly.

**Directions** (`dir`): 0 N, 1 NW, 2 W, 3 SW, 4 S, 5 SE, 6 E, 7 NE.

**Job IDs:** 0 Novice.
