#!/usr/bin/env python3
"""Generate the cities and fields beyond Morroc from the November 2004 eAthena data.

For the maps listed in MAPS it reads reference/eathena-2004/ and writes:

- server/cmd/server/world_data.go: the maps, the warps between them, monster
  spawns, outdoor shops and the Kafras (save point and teleport service).
- server/internal/game/mobs_data.go: every monster spawned there (mob_db).
- server/internal/game/items_data.go: every item those monsters drop or those
  shops sell (item_db).

Hand-written entries (Morroc and its fields, monsters.go, items.go) win over
generated ones. NPC and monster sprite names come from the 2004 client's ID table
(recovered from ragexe.exe), checked against the extracted GRF listing.

Usage: python3 tools/gen_world.py [grf-listing.txt]
  The listing is `python3 tools/grf_extract.py ro-client/data.grf x --list`
  output; sprites missing from it are reported and their monsters skipped.
"""
import glob
import json
import os
import re
import struct
import subprocess
import sys

ROOT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..")
REF = os.path.join(ROOT, "reference", "eathena-2004")

# Cities and every land exit each had in 2004 (direction from the city).
MAPS = {
    "prontera": {"N": "prt_castle", "S": "prt_fild08", "W": "prt_fild05", "E": "prt_fild06"},
    "izlude": {"W": "prt_fild08"},
    "geffen": {"N": "gef_fild04", "E": "gef_fild00", "W": "gef_fild07"},
    "payon": {"N": "pay_arche", "S": "pay_fild01", "E": "pay_fild08", "W": "pay_gld"},
    "alberta": {"W": "pay_fild03"},
}
# Fields further out, reached through the maps above.
EXTRA_FIELDS = [
    "prt_fild07",  # west of prt_fild08 (south of Prontera): Rockers
    "moc_fild12",  # south of Morroc
]
# Already hand-written in main.go (Morroc and its two fields).
EXISTING = {"morocc", "moc_fild07", "moc_fild06"}
# Kafra files per city, and teleport destinations' names in the scripts.
KAFRA_FILES = {"prontera": "pron", "izlude": "izl", "geffen": "gef", "payon": "pay", "alberta": "alb", "morocc": "mor"}
CITY_NAMES = {"Prontera": "prontera", "Izlude": "izlude", "Geffen": "geffen", "Payon": "payon", "Alberta": "alberta", "Morroc": "morocc"}
# Shops of these kinds sell things KDoiRo doesn't have yet (pet eggs, taming items).
SKIP_SHOPS = {"Pet Merchant"}
# Weapon class -> the weapon's sprite layer name (as in items.go).
WEAPON_SPRITES = {1: "단검", 2: "검", 11: "활"}
NPC_ID_BASE = 51000

# A Warper at the centre of every town (not in the 2004 data): the open cell
# nearest each map's centre in its main walkable area, found on the client's
# maps. Each one sends players to any other town's Warper, for free.
WARPERS = {  # town -> (display name, x, y)
    "morocc": ("Morroc", 160, 160),
    "prontera": ("Prontera", 151, 191),
    "izlude": ("Izlude", 138, 133),
    "geffen": ("Geffen", 128, 112),
    "payon": ("Payon", 100, 100),
    "alberta": ("Alberta", 135, 133),
}
WARPER_ID_BASE = 52000

# The cities' Weapon, Armor and Tool Dealers stand indoors in 2004, and KDoiRo
# doesn't load interiors: their stock goes to dealers standing beside each
# town's Warper (Morroc's existing dealers near the spawn get it instead).
INTERIORS = {"morocc_in": "morocc", "prt_in": "prontera", "payon_in01": "payon", "payon_in02": "payon",
             "geffen_in": "geffen", "alberta_in": "alberta", "izlude_in": "izlude"}
DEALERS = ["Weapon Dealer", "Armor Dealer", "Tool Dealer"]
# Where each dealer stands relative to the Warper (inside its open 5x5 square).
DEALER_OFFSETS = {"Weapon Dealer": (-2, 0), "Armor Dealer": (2, 0), "Tool Dealer": (0, 2)}
DEALER_ID_BASE = 53000

# The August 2004 client still has the old 200x200 Payon; eAthena's November
# data already describes the renewed one. Its exits were found by flood-filling
# the old map (south 90,22; north 175,189; east 189,70); the west guild exit
# at x=16 is unchanged. Warps keyed by (source map, x, y) -> replacement, or
# None to drop; NPCs keyed by (map, x, y) -> (x, y, save x, save y).
OLD_PAYON_WARPS = {
    ("payon", 122, 27): ("payon", 90, 22, 3, 2, "pay_fild01", 333, 356),
    ("payon", 228, 330): ("payon", 175, 189, 4, 2, "pay_arche", 81, 22),
    ("payon", 267, 90): ("payon", 189, 70, 2, 4, "pay_fild08", 20, 74),
    ("pay_fild01", 333, 360): ("pay_fild01", 333, 360, 5, 3, "payon", 90, 26),
    ("pay_arche", 81, 18): ("pay_arche", 81, 18, 5, 2, "payon", 175, 185),
    ("pay_fild08", 17, 74): None,  # the renewed town's; 17,75 leads to the old east gate
}
OLD_PAYON_NPCS = {("payon", 181, 104): (163, 100, 163, 97)}


def read_lines(path):
    """Lines without // comments; the files mix encodings, so decode loosely."""
    with open(path, "rb") as f:
        for raw in f.read().decode("latin-1").splitlines():
            line = raw.split("//")[0].rstrip()
            if line.strip():
                yield line


def csv_rows(name):
    rows = {}
    for line in read_lines(os.path.join(REF, "db", name)):
        if line[0].isdigit():
            f = line.split(",")
            rows.setdefault(int(f[0]), f)
    return rows


def num(s, default=0):
    try:
        return int(float(s))
    except ValueError:
        return default


def all_maps():
    maps = list(MAPS)
    for exits in MAPS.values():
        maps += [m for m in exits.values() if m not in maps]
    return maps + [m for m in EXTRA_FIELDS if m not in maps]


def tabs(line):
    return [p for p in line.split("\t") if p.strip()]


def parse_warps(loaded):
    """Warps whose both ends are loaded (and not both already hand-written)."""
    out = []
    for path in glob.glob(os.path.join(REF, "npc", "warps", "**", "*.txt"), recursive=True):
        if "disabled" in path:
            continue
        for line in read_lines(path):
            p = tabs(line)
            if len(p) < 4 or p[1].strip() != "warp":
                continue
            a, b = p[0].split(","), p[3].split(",")
            src, dst = a[0].replace(".gat", ""), b[2].replace(".gat", "")
            if src in loaded and dst in loaded and not (src in EXISTING and dst in EXISTING):
                w = (src, int(a[1]), int(a[2]), int(b[0]), int(b[1]), dst, int(b[3]), int(b[4]))
                w = OLD_PAYON_WARPS.get(w[:3], w)
                if w:
                    out.append(w)
    return sorted(set(out))


def parse_spawns(loaded):
    out = []
    for path in sorted(glob.glob(os.path.join(REF, "npc", "mobs", "fields", "*.txt"))):
        for line in read_lines(path):
            p = tabs(line)
            if len(p) < 4 or p[1].strip() != "monster":
                continue
            a = p[0].split(",")
            m = a[0].replace(".gat", "")
            if m not in loaded or m in EXISTING:
                continue
            f = p[3].split(",")
            out.append((m, num(a[1]), num(a[2]), num(a[3]), num(a[4]), num(f[0]), num(f[1]), num(f[2]), num(f[3]), p[2].strip()))
    return out


def parse_shops(loaded):
    out = []
    for line in read_lines(os.path.join(REF, "npc", "merchants", "shops.txt")):
        p = tabs(line)
        if len(p) < 4 or p[1].strip() != "shop" or p[2].strip() in SKIP_SHOPS:
            continue
        a = p[0].split(",")
        m = a[0].strip().replace(".gat", "")
        if m not in loaded or m in EXISTING:
            continue
        f = p[3].split(",")
        items = [int(x.split(":")[0]) for x in f[1:]]
        out.append((m, int(a[1]), int(a[2]), int(a[3].strip()) % 8, p[2].strip(), int(f[0]), items))
    return out


def parse_dealers():
    """city -> dealer name -> (sprite class, item IDs), merging a city's
    dealers of the same kind (Prontera has two Weapon Dealers)."""
    out = {}
    for line in read_lines(os.path.join(REF, "npc", "merchants", "shops.txt")):
        p = tabs(line)
        if len(p) < 4 or p[1].strip() != "shop" or p[2].strip() not in DEALERS:
            continue
        city = INTERIORS.get(p[0].split(",")[0].strip().replace(".gat", ""))
        if not city:
            continue
        f = p[3].split(",")
        cls, items = out.setdefault(city, {}).get(p[2].strip(), (int(f[0]), []))
        items += [int(x.split(":")[0]) for x in f[1:] if int(x.split(":")[0]) not in items]
        out[city][p[2].strip()] = (cls, items)
    return out


def dealer_stock(dealers, item_db):
    """Drops what KDoiRo can't use yet: ammunition (bows don't use arrows) and,
    at Tool Dealers, anything but HP healing items."""
    out = {}
    for city, shops in dealers.items():
        for name, (cls, items) in shops.items():
            keep = []
            for i in items:
                r = item_db.get(i)
                if not r or num(r[3]) == 10:
                    continue
                if name == "Tool Dealer" and not heals(r):
                    continue
                keep.append(i)
            out.setdefault(city, {})[name] = (cls, keep)
    return out


def heals(r):
    """(min, max) HP an item_db row's use script heals, or None."""
    if num(r[3]) != 0:
        return None
    m = re.search(r"itemheal\s*(?:rand\((\d+),(\d+)\)|(\d+))\s*,", ",".join(r[17:]))
    if not m or (m.group(3) or m.group(2)) in ("0", None):
        return None
    return (m.group(1), m.group(2)) if m.group(1) else (m.group(3), m.group(3))


def parse_kafras(loaded):
    """Kafras in loaded cities: position, sprite ID, save point, teleports."""
    tele = {}  # destination name -> (map, x, y)
    for line in read_lines(os.path.join(REF, "npc", "kafras", "functions_kafras.txt")):
        m = re.search(r'@wrpD\$\[@num\] == "([^"]+)"\) warp "(\w+)\.gat",\s*(\d+),\s*(\d+)', line)
        if m:
            tele[m.group(1)] = (m.group(2), int(m.group(3)), int(m.group(4)))
    out = []
    for city, short in KAFRA_FILES.items():
        if city not in loaded:
            continue
        text = "\n".join(read_lines(os.path.join(REF, "npc", "kafras", f"kafras_{short}.txt")))
        dests = re.search(r'setarray @wrpD\$\[0\],([^;]+);', text)
        prices = re.search(r'setarray @wrpP\[0\],([^;]+);', text)
        names = re.findall(r'"([^"]+)"', dests.group(1)) if dests else []
        costs = [int(x) for x in re.findall(r"\d+", prices.group(1))] if prices else []
        teleports = [(n, *tele[n], c) for n, c in zip(names, costs) if n in tele and CITY_NAMES.get(n) in loaded]
        # Each Kafra script block: header line, then its body up to the next header.
        blocks = re.split(r"\n(?=\w+\.gat,\d+,\d+,\d+\tscript\tKafra)", "\n" + text)
        for block in blocks:
            h = re.match(r"(\w+)\.gat,(\d+),(\d+),(\d+)\tscript\tKafra\t(\d+)", block.strip())
            if not h or h.group(1) != city:
                continue
            saves = re.findall(r'savepoint "(\w+)\.gat",(\d+),(\d+)', block)
            save = next((s for s in saves if s[0] == city), saves[0] if saves else (city, int(h.group(2)), int(h.group(3))))
            x, y, save = int(h.group(2)), int(h.group(3)), (save[0], int(save[1]), int(save[2]))
            if (city, x, y) in OLD_PAYON_NPCS:
                x, y, sx, sy = OLD_PAYON_NPCS[(city, x, y)]
                save = (city, sx, sy)
            out.append((city, x, y, int(h.group(4)) % 8, int(h.group(5)), save, teleports))
    return out


def npc_sprite_table():
    """The 2004 client's NPC ID -> sprite name table, recovered from ragexe.exe:
    it fills the table with `mov [reg+id*4], offset "NAME"` instructions."""
    exe = open(os.path.join(ROOT, "ro-client", "ragexe.exe"), "rb").read()
    pe = struct.unpack_from("<I", exe, 0x3C)[0]
    nsec = struct.unpack_from("<H", exe, pe + 6)[0]
    optsz = struct.unpack_from("<H", exe, pe + 20)[0]
    base = struct.unpack_from("<I", exe, pe + 24 + 28)[0]
    secs = []
    o = pe + 24 + optsz
    for _ in range(nsec):
        _, va, rsz, rptr = struct.unpack_from("<IIII", exe, o + 8)
        secs.append((va, rsz, rptr))
        o += 40

    def offset(v):
        for va, rsz, rptr in secs:
            if base + va <= v < base + va + rsz:
                return v - base - va + rptr
        return None

    names = {}
    for m in re.finditer(rb"\xc7[\x80-\x87](....)(....)\x8b", exe, re.S):
        disp, imm = struct.unpack("<II", m.group(1) + m.group(2))
        off = offset(imm)
        if off is None or disp % 4 or disp > 8000:
            continue
        s = exe[off:exe.find(b"\0", off)]
        if 2 < len(s) < 40 and re.fullmatch(rb"[0-9A-Za-z_]+", s):
            names.setdefault(disp // 4, []).append(s.decode().lower())
    return names


def go_str(s):
    return json.dumps(s, ensure_ascii=False)


def main():
    listing = open(sys.argv[1], encoding="utf-8", errors="replace").read().lower() if len(sys.argv) > 1 else None

    def has_sprite(folder, name):
        return listing is None or f"sprite\\{folder}\\{name}.spr" in listing

    loaded = set(all_maps()) | EXISTING
    mob_db, item_db = csv_rows("mob_db.txt"), csv_rows("item_db.txt")
    npc_names = npc_sprite_table()
    problems = []

    def npc_sprite(npc_id):
        for name in npc_names.get(npc_id, []):
            if has_sprite("npc", name):
                return name
        problems.append(f"no sprite for NPC class {npc_id}; using 4_m_01")
        return "4_m_01"

    # Monsters.
    spawns, mobs = [], {}
    for s in parse_spawns(loaded):
        row = mob_db.get(s[5])
        if not row:
            problems.append(f"unknown mob {s[5]} on {s[0]}")
            continue
        # The client's own ID table names most sprites (e.g. Willow is "wilow").
        sprite = next((n for n in npc_names.get(s[5], []) + [row[1].lower()] if has_sprite("몬스터", n)), None)
        if not sprite:
            problems.append(f"no sprite for {row[1]} ({s[5]}); skipped")
            continue
        mobs[s[5]] = (row, sprite)
        spawns.append(s)

    # Items: drops of those monsters plus shop wares.
    shops = parse_shops(loaded)
    wanted = set()
    for row, _ in mobs.values():
        wanted |= {num(row[i]) for i in range(29, 45, 2) if num(row[i]) and num(row[i + 1])}
    for shop in shops:
        wanted |= set(shop[6])
    dealers = dealer_stock(parse_dealers(), item_db)
    for shops_of_city in dealers.values():
        for _, stock in shops_of_city.values():
            wanted |= set(stock)
    items = {i: item_db[i] for i in sorted(wanted) if i in item_db}
    problems += [f"item {i} missing from item_db" for i in sorted(wanted) if i not in item_db]

    write_mobs(mobs, items)
    write_items(items)
    write_world(spawns, shops, parse_kafras(loaded), parse_warps(loaded), npc_sprite, dealers)
    for p in problems:
        print("note:", p)
    print(f"{len(all_maps())} maps, {len(spawns)} spawns of {len(mobs)} monsters, {len(items)} items, {len(shops)} shops")


def write_mobs(mobs, items):
    lines = [
        "// Code generated by tools/gen_world.py from reference/eathena-2004/db/mob_db.txt; DO NOT EDIT.",
        "",
        "package game",
        "",
        "// generatedMobs are the monsters of the cities' fields (see gen_world.py).",
        "// Hand-written entries in monsters.go take precedence.",
        "var generatedMobs = []*MobDef{",
    ]
    for mid in sorted(mobs):
        r, sprite = mobs[mid]
        mode = num(r[24])
        stats = ", ".join(f"{k}: {num(r[i])}" for k, i in zip(["STR", "AGI", "VIT", "INT", "DEX", "LUK"], range(13, 19)) if num(r[i]))
        drops = [(num(r[i]), num(r[i + 1])) for i in range(29, 45, 2) if num(r[i]) in items and num(r[i + 1]) > 0]
        flags = []
        if mode & 1:
            flags.append("Mobile: true")
        if mode & 128 and num(r[8]) > 0:
            flags.append("Fights: true")
        if mode & 64:
            flags.append("Plant: true")
        if mode & 8:
            flags.append("Assist: true")
        lines += [
            "\t{",
            f"\t\tID: {mid}, Name: {go_str(r[2].replace('_', ' '))}, Sprite: {go_str(sprite)}, Level: {num(r[3])}, HP: {num(r[4])},",
            f"\t\tBaseExp: {num(r[6])}, JobExp: {num(r[7])}, Atk1: {num(r[9])}, Atk2: {num(r[10])}, Def: {num(r[11])}, Mdef: {num(r[12])},",
            f"\t\tStats: [NumStats]int{{{stats}}}, Range: {max(num(r[8]), 1)}, ChaseRange: {max(num(r[20]), 1)},",
            f"\t\tWalkSpeed: {num(r[25])}, AttackDelay: {num(r[26])}, AttackMotion: {num(r[27])}, DamageMotion: {num(r[28])},",
        ]
        if flags:
            lines.append("\t\t" + ", ".join(flags) + ",")
        if drops:
            lines.append("\t\tDrops: []Drop{" + ", ".join(f"{{{i}, {p}}}" for i, p in drops) + "},")
        lines.append("\t},")
    lines += ["}", ""]
    write("server/internal/game/mobs_data.go", lines)


def write_items(items):
    lines = [
        "// Code generated by tools/gen_world.py from reference/eathena-2004/db/item_db.txt; DO NOT EDIT.",
        "",
        "package game",
        "",
        "// generatedItems are the drops and shop wares of the cities (see gen_world.py).",
        "// Hand-written entries in items.go take precedence. Usable items other than",
        "// HP healing (wings, SP potions) and cards are plain loot for now.",
        "var generatedItems = []ItemDef{",
    ]
    for iid, r in items.items():
        typ = num(r[3])
        fields = [f"ID: {iid}", f"Name: {go_str(r[2].replace('_', ' '))}", f"Weight: {num(r[6]) // 10}", f"Price: {num(r[4])}"]
        heal = heals(r)
        if heal:
            fields += ["Type: TypeHealing", f"HealMin: {heal[0]}", f"HealMax: {heal[1]}"]
        elif typ in (4, 5):
            loc = num(r[13])
            fields += [f"Type: {'TypeWeapon' if typ == 4 else 'TypeArmor'}", f"Slot: {loc:#04x}"]
            if num(r[7]):
                fields.append(f"Atk: {num(r[7])}")
            if num(r[8]):
                fields.append(f"Def: {num(r[8])}")
            if num(r[15]) > 1:
                fields.append(f"EquipLevel: {num(r[15])}")
            if num(r[11]):
                fields.append(f"Jobs: {num(r[11])}")
            if typ == 4:
                view = num(r[16])
                fields.append(f"WeaponType: {view}")
                if num(r[9]) > 1:
                    fields.append(f"Range: {num(r[9])}")
                if view in WEAPON_SPRITES:
                    fields.append(f"Sprite: {go_str(WEAPON_SPRITES[view])}")
        else:
            fields.append("Type: TypeEtc")
        lines.append("\t{" + ", ".join(fields) + "},")
    lines += ["}", ""]
    write("server/internal/game/items_data.go", lines)


def write_world(spawns, shops, kafras, warps, npc_sprite, dealers):
    lines = [
        "// Code generated by tools/gen_world.py from reference/eathena-2004/npc; DO NOT EDIT.",
        "",
        "package main",
        "",
        "import (",
        '\t"time"',
        "",
        '\t"kdoiro/server/internal/world"',
        ")",
        "",
        "// cityMaps are the maps beyond Morroc: each city and its land exits.",
        "var cityMaps = []string{" + ", ".join(go_str(m) for m in all_maps()) + "}",
        "",
        "// cityWarps link the cities to their fields (and Morroc's fields, if any).",
        "var cityWarps = []world.Warp{",
    ]
    for w in warps:
        lines.append(f"\t{{Map: {go_str(w[0])}, X: {w[1]}, Y: {w[2]}, SpanX: {w[3]}, SpanY: {w[4]}, DestMap: {go_str(w[5])}, DestX: {w[6]}, DestY: {w[7]}}},")
    lines += ["}", "", "// citySpawns are the 2004 field spawns (npc/mobs/fields/).", "var citySpawns = []world.Spawn{"]
    for m, x, y, sx, sy, mob, count, d1, d2, name in spawns:
        extra = ""
        if d1 or d2:
            extra = f", Delay: {d1} * time.Millisecond, Variance: {d2} * time.Millisecond"
        pos = f"X: {x}, Y: {y}, SpanX: {sx}, SpanY: {sy}, " if x or y else ""
        lines.append(f"\t{{Map: {go_str(m)}, {pos}MobID: {mob}, Count: {count}{extra}}}, // {name}")
    lines += ["}", "", "// cityNPCs are the outdoor shops, the Kafras and the town Warpers.", "var cityNPCs = []world.NPC{"]
    nid = NPC_ID_BASE
    for m, x, y, d, name, cls, wares in shops:
        nid += 1
        lines.append(f"\t{{ID: {nid}, Name: {go_str(name)}, Sprite: {go_str(npc_sprite(cls))}, Map: {go_str(m)}, X: {x}, Y: {y}, Dir: {d}, "
                     f"Shop: []int{{{', '.join(map(str, wares))}}}}},")
    for city, x, y, d, cls, save, teleports in kafras:
        nid += 1
        tp = ", ".join(f"{{Name: {go_str(n)}, Map: {go_str(m)}, X: {tx}, Y: {ty}, Price: {c}}}" for n, m, tx, ty, c in teleports)
        lines += [
            f"\t{{ID: {nid}, Name: \"Kafra Employee\", Sprite: {go_str(npc_sprite(cls))}, Map: {go_str(city)}, X: {x}, Y: {y}, Dir: {d}, Kind: world.KindKafra,",
            f"\t\tKafra: &world.Kafra{{Save: world.Location{{Map: {go_str(save[0])}, X: {save[1]}, Y: {save[2]}}}, Teleports: []world.Teleport{{{tp}}}}}}},",
        ]
    # Town Warpers: arrive two cells south of the destination's Warper, facing it.
    for i, (town, (name, x, y)) in enumerate(WARPERS.items(), 1):
        dests = ", ".join(f"{{Name: {go_str(n)}, Map: {go_str(t)}, X: {tx}, Y: {ty - 2}}}"
                          for t, (n, tx, ty) in WARPERS.items() if t != town)
        lines += [
            f"\t{{ID: {WARPER_ID_BASE + i}, Name: \"Warper\", Sprite: \"4_m_teleporter\", Map: {go_str(town)}, X: {x}, Y: {y}, Dir: 4, Kind: world.KindWarper,",
            f"\t\tWarps: []world.Teleport{{{dests}}}}},",
        ]
    # Dealers beside each Warper, with the city's indoor 2004 stock.
    nid = DEALER_ID_BASE
    for town, (_, wx, wy) in WARPERS.items():
        if town in EXISTING:
            continue
        for name in DEALERS:
            cls, stock = dealers.get(town, {}).get(name, (0, []))
            if not stock:
                continue
            nid += 1
            dx, dy = DEALER_OFFSETS[name]
            lines.append(f"\t{{ID: {nid}, Name: {go_str(name)}, Sprite: {go_str(npc_sprite(cls))}, Map: {go_str(town)}, "
                         f"X: {wx + dx}, Y: {wy + dy}, Dir: 4, Shop: []int{{{', '.join(map(str, stock))}}}}},")
    lines += ["}", ""]
    # Morroc's hand-written dealers (main.go) add the 2004 stock to theirs.
    lines += ["// morroccDealerStock is Morroc's indoor 2004 dealer stock, by dealer name.",
              "var morroccDealerStock = map[string][]int{"]
    for name in DEALERS:
        stock = dealers.get("morocc", {}).get(name, (0, []))[1]
        if stock:
            lines.append(f"\t{go_str(name)}: {{{', '.join(map(str, stock))}}},")
    lines += ["}", ""]
    write("server/cmd/server/world_data.go", lines)


def write(rel, lines):
    path = os.path.join(ROOT, rel)
    with open(path, "w") as f:
        f.write("\n".join(lines))
    subprocess.run(["gofmt", "-w", path], check=False)
    print("wrote", rel)


if __name__ == "__main__":
    main()
