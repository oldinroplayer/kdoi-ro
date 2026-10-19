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
python3 tools/grf_extract.py ro-client/data.grf ro-client --map morocc --map moc_fild07 --map moc_fild06
```

**Tables** (item and skill names and descriptions, map names, BGM per map):

```bash
for t in idnum2itemdisplaynametable idnum2itemresnametable idnum2itemdesctable skillnametable skilldesctable mapnametable mp3nametable; do python3 tools/grf_extract.py ro-client/data.grf ro-client --filter "data\\$t.txt"; done
```

**Sprites** (Novice body and head, shadow, shop NPCs, monsters, visible equipment and weapon swing trails):

```bash
for s in '인간족\몸통\남\초보자_남.' '인간족\머리통\남\2_남.' 'shadow.' 'npc\1_m_smith.' 'npc\1_f_merchant_01.' 'npc\1_m_merchant.' '몬스터\drops.' '몬스터\peco_egg.' '몬스터\picky.' '몬스터\picky_.' '몬스터\yellow_plant.' '몬스터\andre.' '몬스터\ant_egg.' '몬스터\condor.' '몬스터\deniro.' '몬스터\magnolia.' '몬스터\muka.' '몬스터\pecopeco.' '몬스터\piere.' '인간족\초보자\초보자_남_단검.' '인간족\초보자\초보자_남_검.' '인간족\초보자\초보자_남_1207.' '인간족\초보자\초보자_남_단검_검광.' '인간족\초보자\초보자_남_검_검광.' '방패\초보자\초보자_남_가드.' '악세사리\남\남_햇.' '악세사리\남\남_장식용알껍질.' 'npc\1_m_jobguider.'; do python3 tools/grf_extract.py ro-client/data.grf ro-client --filter "sprite\\$s"; done
```

**1st-job sprites** (body, every weapon and swing trail, and the Guard shield for
Swordman, Mage, Archer, Acolyte, Merchant and Thief):

```bash
for j in 검사 마법사 궁수 성직자 상인 도둑; do for f in "인간족\몸통\남\${j}_남." "인간족\${j}\${j}_남_" "방패\${j}\${j}_남_가드."; do python3 tools/grf_extract.py ro-client/data.grf ro-client --filter "sprite\\$f"; done; done
```

**Items on the ground** (one sprite per item, by resource name):

```bash
for r in 빨간포션 빨간허브 노란허브 초록허브 사과 고기 우유 새싹 꽃 빈병 옐로우젬스톤 검은국자 에르늄원석 식물의줄기 젤로피 가렛 셀 자르곤 솜털 새의깃털 새의발톱 새의부리 단단한껍질 끈적끈적한액체 부드러운털 선인장침 벌레껍질 레드블러드 옐로우라이브 윈드오브버듀어 별의가루 철광석 프라콘 소드 나이프 망고슈 보우 가드 햇 코튼셔츠 샌들 후드 장식용알껍질 헌후라이펜; do python3 tools/grf_extract.py ro-client/data.grf ro-client --filter "sprite\\아이템\\$r."; done
```

**Effects** (the white ring shown when an attack lands):

```bash
python3 tools/grf_extract.py ro-client/data.grf ro-client --filter 'texture\effect\ring_white.tga'
```

**Item icons and illustrations** (one per item in `server/internal/game/items.go`,
named by the Korean resource name from `idnum2itemresnametable.txt`):

```bash
for r in 나이프 소드 망고슈 코튼셔츠 후드 샌들 가드 햇 빨간포션 빨간허브 노란허브 사과 우유 젤로피 새의깃털 단단한껍질 빈병 끈적끈적한액체 부드러운털 프라콘 꽃 식물의줄기 새싹 솜털 장식용알껍질 가렛 셀 자르곤 헌후라이펜 검은국자 벌레껍질 옐로우라이브 레드블러드 윈드오브버듀어 별의가루 철광석 에르늄원석 선인장침 초록허브 새의부리 새의발톱 옐로우젬스톤 고기 보우; do for d in item collection; do python3 tools/grf_extract.py ro-client/data.grf ro-client --filter "유저인터페이스\\$d\\$r.bmp"; done; done
```

**Skill icons** (named after the skill's internal name, lower-case):

```bash
for k in nv_basic nv_firstaid sm_sword sm_recovery sm_bash sm_magnum mg_srecovery mg_coldbolt mg_firebolt mg_lightningbolt al_dp al_heal al_incagi al_blessing mc_inccarry mc_discount mc_overcharge mc_mammonite ac_owl ac_vulture ac_double tf_double tf_miss tf_poison; do python3 tools/grf_extract.py ro-client/data.grf ro-client --filter "유저인터페이스\\item\\$k.bmp"; done
```

**Skill effects** (STR animations and their textures, magic circles, "AGI Up",
Blessing's angel sprite, and the skill sounds):

```bash
for f in firehit1.str icecrash.str lightning.str invenom.str fire.str lens_r.bmp pokjuk_c.bmp firering.bmp invenom_ring.bmp freeze_a.bmp freeze_ice.bmp freeze_iceb.bmp freeze_icec.bmp freeze_ice_part.bmp light_a.bmp light_b.bmp light_c.bmp light_d.bmp light_e.bmp light_bang_a.bmp light_bang_b.bmp cir_a.bmp cir_b.bmp cir_c.bmp fire_a.bmp fire_b.bmp pika02.bmp lord_blast_b.bmp magic_red.tga magic_blue.tga magic_green.tga magic_violet.tga magic_sky.tga agi_up.bmp; do python3 tools/grf_extract.py ro-client/data.grf ro-client --filter "texture\\effect\\$f"; done
python3 tools/grf_extract.py ro-client/data.grf ro-client --filter 'sprite\이팩트\축복.'
for w in ef_bash ef_magnumbreak ef_firearrow1 ef_firearrow2 ef_firearrow3 ef_icearrow1 ef_icearrow2 ef_icearrow3 ef_lightbolt1 ef_lightbolt2 ef_lightbolt3 ef_beginspell ef_incagility ef_blessing ef_coin1 thief_invenom; do python3 tools/grf_extract.py ro-client/data.grf ro-client --filter "data\\wav\\effect\\$w.wav"; done
for w in _heal_effect _hit_arrow; do python3 tools/grf_extract.py ro-client/data.grf ro-client --filter "data\\wav\\$w.wav"; done
```

**Sounds** (sprite frame sounds, hit sounds, UI sounds):

```bash
for w in poring_move poring_attack poring_damage poring_die peco_egg_heartbeat peco_egg_crash monster_shell picky_hop picky_damage picky_die monster_feather_s monster_plant player_clothes attack_short_sword attack_sword _hit_dagger _hit_sword _enemy_hit_normal1 _enemy_hit_normal2 _enemy_hit_normal3 _enemy_hit_normal4 _hit_fist1 _hit_fist2 _hit_fist3 _hit_fist4 levelup se_drink_potion se_equip_item_01 'effect\ef_portal' andre_attack andre_move andre_stand monster_shell monster_insect andre_die ant_egg_die condor_damage condor_die condor_attack monster_feather monster_feather_s condor_stand magnolia_attack magnolia_die monster_poring muka_stand muka_move muka_attack muka_die monster_plant kocot_stand kocot_stand2 kocot_foot1 kocot_foot2 kocot_attack kocot_damage kocot_die; do python3 tools/grf_extract.py ro-client/data.grf ro-client --filter "data\\wav\\$w.wav"; done
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

## Reference data

`reference/eathena-2004/` holds the monster, item, spawn, warp, shop and NPC
data from eAthena's November 2004 snapshot, the closest public match to this
client. Use it when adding maps, monsters, items or NPCs; its README explains
the file formats.

Job numbers (HP/SP growth, weight, attack speed, job bonuses, EXP tables) are
generated from it into `server/internal/game/jobs_data.go`:

```bash
python3 tools/gen_jobs.py
```

The 1st-job skills' numbers (SP, cast time, delay, range, duration, weapons,
prerequisites) are generated into `server/internal/game/skills_data.go`; what each
skill does is written by hand in `game/skillrules.go` and `world/skills.go`:

```bash
python3 tools/gen_skills.py
```

## Adding content

New content needs its files extracted too:

- **Map:** add it to the server's `-maps` flag, then run step 2's `--map <name>`.
- **Item:** extract its icon and illustration (`item\` and `collection\` by resource name)
  and its ground sprite (`sprite\아이템\<resource name>.spr/.act`);
  equipment shown on the character also needs its sprite (`Sprite` in `items.go`).
- **Monster:** extract `sprite\몬스터\<sprite>.spr/.act`, plus the sounds listed
  inside its `.act` (find them with `--list --filter <sprite>`).
- **NPC:** extract `sprite\npc\<sprite>.spr/.act`.
- **Skill:** extract its icon (`item\<internal name>.bmp`, e.g. `sm_bash.bmp`), plus any
  effect it shows: an `.str` from `texture\effect\` needs the textures it names
  (list them with `--list --filter <name>.str`, or read them from the file).
