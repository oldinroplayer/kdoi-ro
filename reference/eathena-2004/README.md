# eAthena data, November 2004

Reference data for building KDoiRo to match the 2004 client (patch dated
2004-08-03). These are the `db/` and `npc/` folders from the **first commit of
eAthena's public SVN, 4 November 2004** (svn r2), the closest public snapshot
to that patch:

- Mirror: <https://github.com/markantonay/eathena>
- Commit: `f21cc1761f90201a6b3cff48c3a0d168b5e31f82`
  (archive: <https://codeload.github.com/markantonay/eathena/tar.gz/f21cc1761f90201a6b3cff48c3a0d168b5e31f82>)

eAthena is GPL v2 (see `LICENCE`). Files are copied unchanged; some comments are
in Korean/Japanese (EUC-KR/Shift-JIS), so use `grep -a` when searching.

The server doesn't read these files; values are copied into Go by hand, with a
comment pointing back here (e.g. `server/internal/game/monsters.go`).

## What's where

| Need | File |
|---|---|
| Monster stats and drops | `db/mob_db.txt` |
| Item stats, prices, use scripts | `db/item_db.txt` |
| Monster spawns | `npc/mobs/fields/<region>.txt`, `npc/mobs/dungeons/*.txt` |
| Warps between maps | `npc/warps/cities/*.txt`, `npc/warps/fields/*.txt`, `npc/warps/dungeons/*.txt` |
| Shops (what each NPC sells) | `npc/merchants/shops.txt` |
| Town NPCs and dialogue | `npc/cities/<town>.txt` |
| Job change quests | `npc/jobs/1-1/` (1st jobs), `npc/jobs/novice/` |
| Kafra (storage, teleport) | `npc/kafras/` |
| Monster skills | `db/mob_skill_db.txt` |

## Reading the formats

**`db/mob_db.txt`**, one monster per line:

```
ID,Name,JName,LV,HP,SP,EXP,JEXP,Range1,ATK1,ATK2,DEF,MDEF,STR,AGI,VIT,INT,DEX,LUK,
Range2,Range3,Scale,Race,Element,Mode,Speed,ADelay,aMotion,dMotion,
Drop1id,Drop1per,... Drop8id,Drop8per, MEXP,...
```

- `Range1` is the attack range, `Range2` the sight range, `Range3` the chase range.
- `Speed` is ms per cell. `ADelay` is ms between attacks. `aMotion` and
  `dMotion` are the attack and hurt animation lengths in ms.
- Drop rates are out of 10000.
- `Mode` is a bitmask: 1 can move, 2 looter, 4 aggressive, 8 assist,
  16 cast sensor, 32 boss, 64 plant, 128 can attack.

**`db/item_db.txt`**:

```
ID,Name,Default,Type,Price,Sell,Weight,ATK,DEF,Range,Slot,Job,Gender,Loc,wLV,eLV,View,{UseScript},{EquipScript}
```

- `Type`: 0 healing, 2 usable, 3 etc, 4 weapon, 5 armor, 6 card.
- `Weight` is in tenths (70 = 7).
- Heal amounts are in the use script, e.g. `itemheal rand(45,64),0`.

**Spawns**, `map.gat,x,y,xs,ys<TAB>monster<TAB>Name<TAB>MobID,Count,Delay1,Delay2,Event`:

- `x,y = 0,0` means anywhere on the map.
- `Delay1`/`Delay2` are the respawn time and its random extra, in ms.

**Warps**, `map.gat,x,y,0<TAB>warp<TAB>name<TAB>xs,ys,destmap.gat,destx,desty`:

- The warp triggers within `xs`/`ys` cells of `x,y`.

**Shops**, `map.gat,x,y,dir<TAB>shop<TAB>Name<TAB>sprite,itemID:price,...`:

- A price of `-1` means the item_db price.
- The sprite number is the NPC's class ID, not a file name.
