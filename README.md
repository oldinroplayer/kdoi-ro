# KDoiRo

A small Ragnarok Online–style game: a Go server (`server/`) and a WebGL
browser client (`webclient/`) that renders the original client's maps,
sprites and sounds. Those assets are not included. You extract them from your
own copy of the 2004 RO client installer, as described below.

> RO assets are © Gravity. Don't commit or redistribute the extracted files.

## 1. Unpack the client installer

Put the installer files (`Ragnarok Online.msi`, `Data1.cab`, …) in `client/`, then:

```bash
brew install msitools
```

```bash
msiextract -C installed "client/Ragnarok Online.msi"
```

```bash
mv "installed/Program Files/Gravity/RagnarokOnline" ro-client && rm -rf installed
```

`ro-client/` now holds `data.grf` (the asset archive) and `BGM/*.mp3` (music).

## 2. Extract the assets from data.grf

All commands use `tools/grf_extract.py` (Python 3, no dependencies) and write
into `ro-client/data/`. List anything in the archive with
`python3 tools/grf_extract.py ro-client/data.grf --list --filter <text>`.

**Maps.** `--map` extracts each map's `.gat`/`.gnd`/`.rsw`, its minimap, and every
model, texture, water frame and ambient sound it uses:

```bash
python3 tools/grf_extract.py ro-client/data.grf ro-client --map morocc --map moc_fild07
```

**Tables** (item names, descriptions and icons, map names, BGM per map):

```bash
for t in idnum2itemdisplaynametable idnum2itemresnametable idnum2itemdesctable mapnametable mp3nametable; do python3 tools/grf_extract.py ro-client/data.grf ro-client --filter "data\\$t.txt"; done
```

**Sprites** (Novice body and head, shadow, shop NPCs, monsters, visible equipment):

```bash
for s in '인간족\몸통\남\초보자_남.' '인간족\머리통\남\2_남.' 'shadow.' 'npc\1_m_smith.' 'npc\1_f_merchant_01.' 'npc\1_m_merchant.' '몬스터\drops.' '몬스터\peco_egg.' '몬스터\picky.' '몬스터\picky_.' '몬스터\yellow_plant.' '인간족\초보자\초보자_남_단검.' '인간족\초보자\초보자_남_검.' '인간족\초보자\초보자_남_1207.' '방패\초보자\초보자_남_가드.' '악세사리\남\남_햇.'; do python3 tools/grf_extract.py ro-client/data.grf ro-client --filter "sprite\\$s"; done
```

**Item icons and illustrations** (one per item in `server/internal/game/items.go`,
named by the Korean resource name from `idnum2itemresnametable.txt`):

```bash
for r in 나이프 소드 망고슈 코튼셔츠 후드 샌들 가드 햇 빨간포션 빨간허브 노란허브 사과 우유 젤로피 새의깃털 단단한껍질 빈병 끈적끈적한액체 부드러운털 프라콘 꽃 식물의줄기 새싹 솜털; do for d in item collection; do python3 tools/grf_extract.py ro-client/data.grf ro-client --filter "유저인터페이스\\$d\\$r.bmp"; done; done
```

**Sounds** (sprite frame sounds, hit sounds, UI sounds):

```bash
for w in poring_move poring_attack poring_damage poring_die peco_egg_heartbeat peco_egg_crash monster_shell picky_hop picky_damage picky_die monster_feather_s monster_plant player_clothes attack_short_sword attack_sword _hit_dagger _hit_sword _enemy_hit_normal1 _enemy_hit_normal2 _enemy_hit_normal3 _enemy_hit_normal4 _hit_fist1 _hit_fist2 _hit_fist3 _hit_fist4 levelup se_drink_potion se_equip_item_01 'effect\ef_portal'; do python3 tools/grf_extract.py ro-client/data.grf ro-client --filter "data\\wav\\$w.wav"; done
```

Each command prints `extracted N, skipped 0, failed 0`. If one prints
`extracted 0`, the name doesn't exist in your archive: check it with `--list`.

## 3. Link the assets into the web client

The client serves `public/data` and `public/bgm`. In this repo they are
symlinks to `../../ro-client/data` and `../../ro-client/BGM`; if they're
missing, recreate them:

```bash
ln -sfn ../../ro-client/data webclient/public/data && ln -sfn ../../ro-client/BGM webclient/public/bgm
```

## 4. Run

```bash
cd server && go run ./cmd/server
```

```bash
cd webclient && npm install && npm run dev
```

Open http://localhost:5173. See `server/README.md` for the protocol and
`webclient/README.md` for the client layout.

## Adding content

New content needs its files extracted too:

- **Map:** add it to the server's `-maps` flag, then run step 2's `--map <name>`.
- **Item:** extract its icon and illustration (`item\` and `collection\` by resource name);
  equipment shown on the character also needs its sprite (`Sprite` in `items.go`).
- **Monster:** extract `sprite\몬스터\<sprite>.spr/.act`, plus the sounds listed
  inside its `.act` (find them with `--list --filter <sprite>`).
- **NPC:** extract `sprite\npc\<sprite>.spr/.act`.
