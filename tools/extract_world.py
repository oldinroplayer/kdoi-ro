#!/usr/bin/env python3
"""Extract the client files the generated world content needs.

Reads the Go files written by tools/gen_world.py (plus the hand-written
items.go, monsters.go and main.go) and extracts from data.grf:

- monster sprites (sprite/몬스터/<sprite>.spr/.act) and the sounds their
  animations play (named inside each .act),
- NPC sprites (sprite/npc/<sprite>.spr/.act),
- item icons, illustrations and ground sprites (by the item's Korean resource
  name from idnum2itemresnametable.txt).

Maps themselves are extracted with `tools/grf_extract.py --map <name>`.

Usage: python3 tools/extract_world.py [ro-client/data.grf] [ro-client]
"""
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import grf_extract  # noqa: E402

ROOT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..")
GO = {
    "mobs": ["server/internal/game/mobs_data.go", "server/internal/game/monsters.go"],
    "items": ["server/internal/game/items_data.go", "server/internal/game/items.go"],
    "npcs": ["server/cmd/server/world_data.go", "server/cmd/server/main.go"],
}


def read(rel):
    with open(os.path.join(ROOT, rel), encoding="utf-8") as f:
        return f.read()


def resource_names(client_dir):
    """Item ID -> Korean resource name."""
    out = {}
    raw = open(os.path.join(client_dir, "data", "idnum2itemresnametable.txt"), "rb").read()
    for line in raw.decode("cp949", "replace").replace("\r", "\n").split("\n"):
        p = line.split("#")
        if len(p) > 1 and p[0].strip().isdigit():
            out[int(p[0])] = p[1]
    return out


def act_sounds(path):
    """Sound file names listed at the end of an ACT (version 2.1+)."""
    try:
        data = open(path, "rb").read()
    except OSError:
        return []
    return [s.decode("cp949", "replace") for s in re.findall(rb"([\w\-]+\.wav)\0", data)]


def main():
    grf = sys.argv[1] if len(sys.argv) > 1 else os.path.join(ROOT, "ro-client", "data.grf")
    out = sys.argv[2] if len(sys.argv) > 2 else os.path.join(ROOT, "ro-client")

    mobs = sorted({m for f in GO["mobs"] for m in re.findall(r'Sprite: "([^"]+)"', read(f))})
    npcs = sorted({m for f in GO["npcs"] for m in re.findall(r'Sprite: "([^"]+)"', read(f))})
    item_ids = sorted({int(i) for f in GO["items"] for i in re.findall(r"\bID: (\d+)", read(f))})
    # Headgear and masks are worn as accessory sprites, one per gender.
    headgear = sorted({m.group(2) for f in GO["items"]
                       for m in re.finditer(r'Slot: (0x[0-9a-f]+)\b.*Sprite: "([^"]+)"', read(f))
                       if int(m.group(1), 16) & 0x301})
    res = resource_names(out)
    names = sorted({res[i] for i in item_ids if i in res})

    wanted = []
    wanted += [f"data\\sprite\\몬스터\\{m}.{ext}" for m in mobs for ext in ("spr", "act")]
    wanted += [f"data\\sprite\\npc\\{n}.{ext}" for n in npcs for ext in ("spr", "act")]
    wanted += [f"data\\sprite\\악세사리\\{sex}\\{sex}_{h}.{ext}"
               for h in headgear for sex in ("남", "여") for ext in ("spr", "act")]
    # Effects placed on maps (Payon Cave's torch fire), as in game.ts MAP_EFFECT_SPRITES,
    # and msg (the critical hit starburst).
    wanted += [f"data\\sprite\\이팩트\\{n}.{ext}" for n in ("torch_01", "msg") for ext in ("spr", "act")]
    wanted += ["data\\wav\\effect\\ef_hit2.wav"]  # game.ts CRIT_SOUND
    for r in names:
        wanted += [f"data\\texture\\유저인터페이스\\item\\{r}.bmp", f"data\\texture\\유저인터페이스\\collection\\{r}.bmp",
                   f"data\\sprite\\아이템\\{r}.spr", f"data\\sprite\\아이템\\{r}.act"]

    with open(grf, "rb") as f:
        entries = {e[0].lower(): e for e in grf_extract.read_table(f)}
        found = [entries[w.lower()] for w in wanted if w.lower() in entries]
        missing = [w for w in wanted if w.lower() not in entries]
        grf_extract.extract_entries(f, found, out)

        # Monster animation sounds (footsteps, attacks, deaths).
        sounds = set()
        for m in mobs:
            sounds |= set(act_sounds(os.path.join(out, "data", "sprite", "몬스터", f"{m}.act")))
        found = [entries[k] for k in (f"data\\wav\\{s}".lower() for s in sorted(sounds)) if k in entries]
        grf_extract.extract_entries(f, found, out)

    print(f"{len(mobs)} monsters, {len(npcs)} NPCs, {len(names)} items, {len(headgear)} headgear, {len(sounds)} sounds")
    for w in missing:
        print("not in the GRF:", w)


if __name__ == "__main__":
    main()
