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
    "pay_dun00",  # Payon Cave level 1, entered from the Archer Village (pay_arche)
    "pay_dun01",  # Payon Cave level 2, below level 1
    # Training spots the Training Guide sends players to (TRAINING below).
    "pay_dun02",  # Payon Cave level 3: Munak, Bon Gun
    "pay_dun03",  # Payon Cave level 4: Sohee
    "pay_fild07",  # Payon field: Elder Willow, Eggyra, Bigfoot
    "anthell01",  # Ant Hell 1: Andre, Piere, Deniro
    "orcsdun01",  # Orc Dungeon 1: Orc Zombie
    "orcsdun02",  # Orc Dungeon 2: Zenorc, Orc Skeleton
]
# Already hand-written in main.go (Morroc and its two fields).
EXISTING = {"morocc", "moc_fild07", "moc_fild06"}
# Kafra files per city, and teleport destinations' names in the scripts.
KAFRA_FILES = {"prontera": "pron", "izlude": "izl", "geffen": "gef", "payon": "pay", "alberta": "alb", "morocc": "mor"}
CITY_NAMES = {"Prontera": "prontera", "Izlude": "izlude", "Geffen": "geffen", "Payon": "payon", "Alberta": "alberta", "Morroc": "morocc"}
# Shops of these kinds sell things KDoiRo doesn't have yet (pet eggs, taming items).
SKIP_SHOPS = {"Pet Merchant"}
# Weapon class -> the weapon's sprite layer name (as in items.go).
# Weapon sprites by type (item_db View) under sprite/인간족/<job>/<job>_<sex>_<name>.
# Many weapons have their own file named by item ID instead (e.g. 1116 Katana),
# which the client uses when it exists (see weapon_sprite in main).
WEAPON_SPRITES = {1: "단검", 2: "검", 3: "검", 4: "창", 5: "창", 6: "도끼", 7: "도끼", 8: "클럽", 10: "롯드", 11: "활"}
# Jobs whose folders hold weapon sprites (Novice and the 1st jobs).
JOB_FOLDERS = ["초보자", "검사", "마법사", "궁수", "성직자", "상인", "도둑"]
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

# A camp outside Payon Cave (in the Archer Village, by the entrance at 36,131):
# a Job Guide and dealers with Payon's stock, so players can change job and
# restock before going in. Not in 2004 RO. (name, x, y, facing); the lane from
# the cave exit (39,131) eastward is left clear.
CAVE_CAMP_MAP = "pay_arche"
CAVE_CAMP_STOCK = "payon"
CAVE_CAMP = [
    ("Job Guide", 40, 137, 4),
    ("Tool Dealer", 44, 137, 4),
    ("Weapon Dealer", 40, 126, 0),
    ("Armor Dealer", 44, 126, 0),
    ("Reset Maid", 52, 137, 4),  # free stat and skill reset (world.KindReset)
    ("Training Guide", 56, 137, 4),  # free warp to a map for your level (TRAINING)
]

# Vending stalls around the camp: Merchants and Blacksmiths sitting with a
# shop sign, like players vending. Prices are figured from the NPC shop price
# (item_db's): `discount` is a fraction of it (each item varied a little, the
# same every run); forged elemental weapons cost the base weapon's price plus
# an elemental stone's worth. (title, job, female, hair, hair colour, x, y,
# facing, discount, items), items as item ID or (item ID, element).
BLACKSMITH = 10  # the Blacksmith's class (a 2nd job: only drawn here)
FIRE, WATER, WIND, EARTH = 3, 1, 4, 2
VENDORS = [
    ("Potions 20% off!", 5, True, 7, 3, 45, 122, 0, 0.80, [501, 502, 503, 504, 645, 656]),
    ("Wings & Arrows cheap", 5, False, 4, 1, 49, 122, 0, 0.78, [601, 602, 1750, 1770, 1751, 1752]),
    ("Herbs and snacks", 5, True, 12, 5, 53, 122, 0, 0.85, [507, 508, 511, 512, 519, 517]),
    ("Fire weapons, forged today", BLACKSMITH, False, 2, 6, 45, 118, 0, 1.0,
     [(1207, FIRE), (1101, FIRE), (1104, FIRE), (1301, FIRE), (1501, FIRE)]),
    ("Ice blades", BLACKSMITH, True, 9, 2, 49, 118, 0, 1.0,
     [(1201, WATER), (1207, WATER), (1101, WATER), (1107, WATER), (1401, WATER)]),
    ("Wind weapons", BLACKSMITH, False, 5, 0, 53, 118, 0, 1.0,
     [(1204, WIND), (1210, WIND), (1104, WIND), (1116, WIND), (1351, WIND)]),
    ("Earth weapons", BLACKSMITH, True, 14, 4, 45, 114, 0, 1.0,
     [(1207, EARTH), (1101, EARTH), (1107, EARTH), (1504, EARTH), (1301, EARTH)]),
    ("Elemental daggers", BLACKSMITH, False, 11, 7, 49, 114, 0, 1.0,
     [(1201, FIRE), (1204, WATER), (1207, WIND), (1210, EARTH)]),
    ("Weapon clearance", BLACKSMITH, False, 3, 8, 53, 114, 0, 0.82, [1101, 1104, 1107, 1116, 1201, 1204, 1207, 1210]),
    ("Armor sale", 5, True, 6, 1, 40, 141, 4, 0.85, [2301, 2303, 2305, 2312, 2401, 2403]),
    ("Shields & garments", 5, False, 8, 3, 44, 141, 4, 0.88, [2101, 2103, 2501, 2503]),
    ("Hats! Hats! Hats!", 5, True, 15, 6, 48, 141, 4, 0.90, [2220, 2226, 2211, 2208, 2224, 2201]),
    ("Archer supply", 5, False, 1, 2, 44, 145, 4, 0.86, [1701, 1704, 1750, 1770, 1751, 1752]),
    ("Mage & Acolyte corner", 5, True, 10, 4, 48, 145, 4, 0.87, [1601, 1604, 1501, 1504, 2501, 503]),
]
VENDOR_ID_BASE = 55000


def vendor_price(item_id, base, discount, element):
    """A ware's price: the NPC price at the stall's discount, or for a forged
    weapon the base price plus an elemental stone (3000-6000), varied per item
    by a fixed amount so prices stay put between runs."""
    jitter = ((item_id * 2654435761 + element * 97) % 1000) / 1000  # 0..1, stable
    if element:
        return int(base * (1.05 + 0.25 * jitter) + 3000 + 3000 * jitter) // 10 * 10
    return max(1, int(base * (discount + (jitter - 0.5) * 0.10)))


# The Training Guide's menu: level ranges, each with maps suited to it (picked
# by their 2004 monsters' levels), as (label, map, x, y). The spots are where
# the 2004 warps into each map arrive (Ant Hell's outside entrance is a
# scripted warp: its usual landing spot).
TRAINING = [
    ("Lv 1-10", [
        ("Prontera South Field (Poring, Lunatic)", "prt_fild08", 170, 375),
        ("Payon Forest (Willow, Spore)", "pay_fild08", 20, 74),
        ("Geffen Field (Poporing, Fabre)", "gef_fild00", 46, 199),
    ]),
    ("Lv 10-20", [
        ("Rocker Field (Rocker)", "prt_fild07", 380, 186),
        ("Payon Cave 1 (Skeleton, Zombie)", "pay_dun00", 21, 183),
        ("Ant Hell 1 (Andre, Piere, Deniro)", "anthell01", 35, 262),
    ]),
    ("Lv 20-30", [
        ("Payon Cave 2 (Skeleton Soldier, Skeleton Archer)", "pay_dun01", 19, 33),
        ("Orc Dungeon 1 (Orc Zombie)", "orcsdun01", 32, 170),
        ("Payon Field (Elder Willow, Eggyra)", "pay_fild07", 23, 207),
    ]),
    ("Lv 30-40", [
        ("Payon Cave 3 (Munak, Bon Gun)", "pay_dun02", 19, 63),
        ("Payon Cave 4 (Sohee)", "pay_dun03", 155, 159),
        ("Orc Dungeon 2 (Zenorc, Orc Skeleton)", "orcsdun02", 21, 185),
    ]),
]
CAMP_ID_BASE = 54000
# The camp's Weapon and Armor Dealers sell every weapon and armor any 2004 NPC
# shop sells that Novices or 1st jobs can use, all for this many zeny.
CAVE_CAMP_PRICE = 1
# item_db Job bits for Novice and the six 1st jobs.
FIRST_JOBS_MASK = 0x7F
# Extra camp wares no 2004 NPC shop sold, by dealer: Goggles (2224).
CAVE_CAMP_EXTRAS = {"Armor Dealer": [2224]}
# A Kafra at the camp too (save point and teleports), in the row with the Job
# Guide and Tool Dealer: (x, y, facing), and where saving puts you.
CAVE_CAMP_KAFRA = (48, 137, 4)
CAVE_CAMP_SAVE = (48, 134)

# Starter monsters (levels 1-4, good to about base level 10) on the open
# ground in front of Payon Cave, where new characters start. Not in the 2004
# data: pay_arche has no spawns there. (map, x, y, span x, span y, mob, count,
# respawn ms, respawn variance ms, name), as in eAthena's monster lines.
CAVE_CAMP_SPAWNS = [
    ("pay_arche", 72, 136, 22, 10, 1002, 10, 5000, 0, "Poring"),
    ("pay_arche", 72, 136, 22, 10, 1007, 8, 5000, 0, "Fabre"),
    ("pay_arche", 72, 136, 22, 10, 1008, 5, 5000, 0, "Pupa"),
    ("pay_arche", 72, 136, 22, 10, 1063, 8, 5000, 0, "Lunatic"),
    ("pay_arche", 72, 136, 22, 10, 1010, 6, 5000, 0, "Willow"),
]

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
    paths = glob.glob(os.path.join(REF, "npc", "mobs", "fields", "*.txt")) + \
        glob.glob(os.path.join(REF, "npc", "mobs", "dungeons", "*.txt"))
    for path in sorted(paths):
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
    return out + [s for s in CAVE_CAMP_SPAWNS if s[0] in loaded]


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
    """Drops what KDoiRo can't use yet: at Tool Dealers, anything but HP
    healing items and wings."""
    out = {}
    for city, shops in dealers.items():
        for name, (cls, items) in shops.items():
            keep = []
            for i in items:
                r = item_db.get(i)
                if not r:
                    continue
                if name == "Tool Dealer" and not heals(r) and not is_wing(r):
                    continue
                keep.append(i)
            out.setdefault(city, {})[name] = (cls, keep)
    return out


def camp_stock(item_db):
    """The camp dealers' wares: every weapon and armor any 2004 NPC shop sells
    that Novices or 1st jobs can use, by dealer, in item_db order."""
    sold = set()
    for line in read_lines(os.path.join(REF, "npc", "merchants", "shops.txt")):
        p = tabs(line)
        if len(p) >= 4 and p[1].strip() == "shop":
            sold |= {int(x.split(":")[0]) for x in p[3].split(",")[1:]}
    out = {"Weapon Dealer": [], "Armor Dealer": []}
    for i in sorted(sold):
        r = item_db.get(i)
        if not r or not num(r[11]) & FIRST_JOBS_MASK:
            continue
        if num(r[3]) in (4, 10):  # weapons and their arrows
            out["Weapon Dealer"].append(i)
        elif num(r[3]) == 5:
            out["Armor Dealer"].append(i)
    for dealer, extras in CAVE_CAMP_EXTRAS.items():
        out[dealer] = sorted(set(out[dealer]) | {i for i in extras if i in item_db})
    return out


def is_wing(r):
    """Fly and Butterfly Wings: usable items whose script warps."""
    return num(r[3]) == 2 and 'warp "' in ",".join(r[17:])


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


def exe_tables():
    """Name tables the 2004 client fills in code, recovered from ragexe.exe: it
    writes them with `mov [reg+id*4], offset "NAME"` instructions (32-bit
    displacement, or 8-bit for the first 32 IDs). Returns (NPC and monster
    sprite names by class ID, headgear sprite names by item View ID)."""
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

    names, headgear = {}, {}
    for m in re.finditer(rb"\xc7[\x80-\x87](....)(....)\x8b", exe, re.S):
        disp, imm = struct.unpack("<II", m.group(1) + m.group(2))
        off = offset(imm)
        if off is None or disp % 4 or disp > 8000:
            continue
        s = exe[off:exe.find(b"\0", off)]
        if 2 < len(s) < 40 and re.fullmatch(rb"[0-9A-Za-z_]+", s):
            names.setdefault(disp // 4, []).append(s.decode().lower())

    def accessory(imm):
        off = offset(imm)
        if off is None:
            return None
        s = exe[off:exe.find(b"\0", off)]
        if not s.startswith(b"_") or not 1 < len(s) < 40:
            return None
        try:
            return s.decode("cp949")[1:]  # "_햇" -> sprite/악세사리/<sex>/<sex>_햇
        except UnicodeDecodeError:
            return None

    for m in re.finditer(rb"\xc7[\x80-\x87](....)(....)", exe, re.S):
        disp, imm = struct.unpack("<II", m.group(1) + m.group(2))
        name = accessory(imm)
        if name and disp % 4 == 0 and disp < 12000:
            headgear[disp // 4] = name
    for m in re.finditer(rb"\xc7[\x40-\x47](.)(....)", exe, re.S):
        name = accessory(struct.unpack("<I", m.group(2))[0])
        if name and m.group(1)[0] % 4 == 0:
            headgear.setdefault(m.group(1)[0] // 4, name)
    return names, headgear


def go_str(s):
    return json.dumps(s, ensure_ascii=False)


def main():
    listing = open(sys.argv[1], encoding="utf-8", errors="replace").read().lower() if len(sys.argv) > 1 else None

    def has_sprite(folder, name):
        return listing is None or f"sprite\\{folder}\\{name}.spr" in listing

    def weapon_sprite(item_id, view):
        """The weapon's own sprite (named by item ID) if any job has one, else its type's."""
        if listing is not None and any(f"sprite\\인간족\\{j}\\{j}_남_{item_id}.spr" in listing for j in JOB_FOLDERS):
            return str(item_id)
        return WEAPON_SPRITES.get(view)

    loaded = set(all_maps()) | EXISTING
    mob_db, item_db = csv_rows("mob_db.txt"), csv_rows("item_db.txt")
    npc_names, headgear_names = exe_tables()
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
    camp = camp_stock(item_db)
    for stock in camp.values():
        wanted |= set(stock)
    # Vending stalls: their wares, priced from the NPC price.
    vendors = []
    for title, job, female, hair, color, x, y, facing, discount, wares in VENDORS:
        stock = []
        for ware in wares:
            item_id, element = ware if isinstance(ware, tuple) else (ware, 0)
            if item_id not in item_db:
                problems.append(f"vendor {title!r}: item {item_id} not in item_db")
                continue
            wanted.add(item_id)
            stock.append((item_id, vendor_price(item_id, num(item_db[item_id][4]), discount, element), element))
        vendors.append((title, job, female, hair, color, x, y, facing, stock))
    items = {i: item_db[i] for i in sorted(wanted) if i in item_db}
    problems += [f"item {i} missing from item_db" for i in sorted(wanted) if i not in item_db]

    write_mobs(mobs, items)
    write_items(items, headgear_names, weapon_sprite)
    write_elements(mob_db, set(mobs))
    kafras = parse_kafras(loaded)
    # The camp Kafra dresses and teleports like Payon's own.
    payon = next((k for k in kafras if k[0] == CAVE_CAMP_STOCK), None)
    if payon and CAVE_CAMP_MAP in loaded:
        x, y, d = CAVE_CAMP_KAFRA
        kafras.append((CAVE_CAMP_MAP, x, y, d, payon[4], (CAVE_CAMP_MAP, *CAVE_CAMP_SAVE), payon[6]))
    write_world(spawns, shops, kafras, parse_warps(loaded), npc_sprite, dealers, camp, vendors)
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


def write_elements(mob_db, generated):
    """The element damage table (db/attr_fix.txt) and every monster's element,
    race, size and mode flags (hand-written monsters.go ones too)."""
    table, level = [], None
    for line in read_lines(os.path.join(REF, "db", "attr_fix.txt")):
        head = re.match(r"\s*(\d+),10\s*$", line)
        if head:
            level = int(head.group(1))
            table.append([])
            continue
        row = [int(v) for v in re.findall(r"-?\d+", line)]
        if level and len(row) == 10:
            table[-1].append(row)
    assert len(table) == 4 and all(len(t) == 10 for t in table), "unexpected attr_fix.txt layout"

    hand = {int(i) for i in re.findall(r"^\t(\d+): \{", open(os.path.join(ROOT, "server/internal/game/monsters.go")).read(), re.M)}
    lines = [
        "// Code generated by tools/gen_world.py from reference/eathena-2004/db; DO NOT EDIT.",
        "",
        "package game",
        "",
        "// attrFix[level-1][attack element][defending element] is the damage percent",
        "// an attack of one element does to a target of another (db/attr_fix.txt).",
        "// Negative values heal the target.",
        "var attrFix = [4][NumElements][NumElements]int{",
    ]
    for t in table:
        lines.append("\t{")
        lines += ["\t\t{" + ", ".join(str(v) for v in row) + "}," for row in t]
        lines.append("\t},")
    lines += ["}", "", "// mobTraits are each monster's element, race, size and behaviour (mob_db).",
              "var mobTraits = map[int]MobTraits{"]
    for mid in sorted(hand | generated):
        r = mob_db[mid]
        element, size, race, mode = num(r[23]), num(r[21]), num(r[22]), num(r[24])
        lines.append(f"\t{mid}: {{Element: {element % 10}, ElementLevel: {max(element // 20, 1)}, Race: {race}, Size: {size}, "
                     f"ViewRange: {num(r[19])}, Aggressive: {str(bool(mode & 4)).lower()}, "
                     f"CastSensor: {str(bool(mode & 16)).lower()}}}, // {r[2]}")
    lines += ["}", ""]
    write("server/internal/game/elements_data.go", lines)


# Equip location bits that show on the head (top, middle, lower headgear).
HEAD_SLOTS = 0x100 | 0x200 | 0x001


# Card script bonuses KDoiRo applies (game.Bonus): one-value bonuses by field,
# and two-value ones (race or element, percent) by array field.
CARD_STATS = {"bStr": 0, "bAgi": 1, "bVit": 2, "bInt": 3, "bDex": 4, "bLuk": 5}
CARD_FIELDS = {
    "bMaxHP": "MaxHP", "bMaxSP": "MaxSP", "bMaxHPrate": "MaxHPRate", "bMaxSPrate": "MaxSPRate",
    "bBaseAtk": "Atk", "bAtk": "Atk", "bDef": "Def", "bMdef": "Mdef", "bHit": "Hit", "bFlee": "Flee",
    "bFlee2": "Flee2", "bCritical": "Crit", "bHPrecovRate": "HPRegenRate", "bSPrecovRate": "SPRegenRate",
}
CARD_ARRAYS = {"bAddRace": ("AddRace", "NumRaces"), "bAddEle": ("AddEle", "NumElements"),
               "bSubRace": ("SubRace", "NumRaces"), "bSubEle": ("SubEle", "NumElements")}


def card_bonus(script):
    """A card's script as a game.Bonus literal, or None if nothing applies."""
    stats, fields, arrays = [0] * 6, {}, {}
    for m in re.finditer(r"bonus\s+(b\w+)\s*,\s*(-?\d+)\s*;", script):
        name, value = m.group(1), int(m.group(2))
        if name == "bAllStats":
            stats = [v + value for v in stats]
        elif name in CARD_STATS:
            stats[CARD_STATS[name]] += value
        elif name in CARD_FIELDS:
            fields[CARD_FIELDS[name]] = fields.get(CARD_FIELDS[name], 0) + value
    for m in re.finditer(r"bonus2\s+(b\w+)\s*,\s*(\d+)\s*,\s*(-?\d+)\s*;", script):
        if m.group(1) in CARD_ARRAYS:
            arrays.setdefault(m.group(1), {})[int(m.group(2))] = int(m.group(3))
    parts = []
    if any(stats):
        parts.append(f"Stats: [NumStats]int{{{', '.join(map(str, stats))}}}")
    parts += [f"{f}: {v}" for f, v in fields.items() if v]
    for name, values in arrays.items():
        field, size = CARD_ARRAYS[name]
        parts.append(f"{field}: [{size}]int{{{', '.join(f'{k}: {v}' for k, v in sorted(values.items()))}}}")
    return "&Bonus{" + ", ".join(parts) + "}" if parts else None


def write_items(items, headgear_names, weapon_sprite):
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
        warp = re.search(r'warp\s+"(Random|SavePoint)"', ",".join(r[17:])) if typ == 2 else None
        if heal:
            fields += ["Type: TypeHealing", f"HealMin: {heal[0]}", f"HealMax: {heal[1]}"]
        elif warp:
            # Fly Wing (a random spot on the map) and Butterfly Wing (the save point).
            fields += ["Type: TypeWarp", "Warp: " + ("WarpRandom" if warp.group(1) == "Random" else "WarpSavePoint")]
        elif typ == 6:
            # Cards: the equipment they go into, and what they add.
            fields += ["Type: TypeCard", f"CardLoc: {num(r[13]):#04x}"]
            bonus = card_bonus(",".join(r[17:]))
            if bonus:
                fields.append(f"Bonus: {bonus}")
        elif typ in (4, 5):
            loc = num(r[13])
            fields += [f"Type: {'TypeWeapon' if typ == 4 else 'TypeArmor'}", f"Slot: {loc:#04x}"]
            if num(r[10]):
                fields.append(f"Slots: {num(r[10])}")
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
                sprite = weapon_sprite(iid, view)
                if sprite:
                    fields.append(f"Sprite: {go_str(sprite)}")
            elif loc & HEAD_SLOTS and num(r[16]) in headgear_names:
                # Headgear and masks: the client's accessory sprite for the item's View.
                fields.append(f"Sprite: {go_str(headgear_names[num(r[16])])}")
        elif typ == 10:
            # Arrows: worn in the ammo slot; their element and critical bonus
            # come from the item's script.
            fields += ["Type: TypeAmmo", f"Slot: {num(r[13]):#04x}"]
            if num(r[7]):
                fields.append(f"Atk: {num(r[7])}")
            if num(r[11]):
                fields.append(f"Jobs: {num(r[11])}")
            script = ",".join(r[17:])
            if m := re.search(r"bAtkEle,\s*(\d+)", script):
                fields.append(f"Element: {m.group(1)}")
            if m := re.search(r"bCritical,\s*(\d+)", script):
                fields.append(f"Crit: {m.group(1)}")
        else:
            fields.append("Type: TypeEtc")
        lines.append("\t{" + ", ".join(fields) + "},")
    lines += ["}", ""]
    write("server/internal/game/items_data.go", lines)


def write_world(spawns, shops, kafras, warps, npc_sprite, dealers, camp, vendors):
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
    # The camp outside Payon Cave.
    for i, (name, x, y, facing) in enumerate(CAVE_CAMP, 1):
        head = f"\t{{ID: {CAMP_ID_BASE + i}, Name: {go_str(name)}, "
        if name == "Job Guide":
            lines.append(head + f'Sprite: "1_m_jobguider", Map: {go_str(CAVE_CAMP_MAP)}, X: {x}, Y: {y}, Dir: {facing}, Kind: world.KindJobChange}},')
            continue
        if name == "Reset Maid":
            lines.append(head + f'Sprite: "4_f_maid", Map: {go_str(CAVE_CAMP_MAP)}, X: {x}, Y: {y}, Dir: {facing}, Kind: world.KindReset}},')
            continue
        if name == "Training Guide":
            ranges = ", ".join(
                f"{{Name: {go_str(label)}, Spots: []world.Teleport{{"
                + ", ".join(f"{{Name: {go_str(n)}, Map: {go_str(m)}, X: {tx}, Y: {ty}}}" for n, m, tx, ty in spots)
                + "}}" for label, spots in TRAINING)
            lines.append(head + f'Sprite: "4_f_job_hunter", Map: {go_str(CAVE_CAMP_MAP)}, X: {x}, Y: {y}, Dir: {facing}, '
                         f"Kind: world.KindTraining,")
            lines.append(f"\t\tTraining: []world.TrainingRange{{{ranges}}}}},")
            continue
        cls, stock = dealers[CAVE_CAMP_STOCK][name]
        price = ""
        if name in camp:  # every class's weapons or armor, at a token price
            stock, price = camp[name], f", Price: {CAVE_CAMP_PRICE}"
        lines.append(head + f"Sprite: {go_str(npc_sprite(cls))}, Map: {go_str(CAVE_CAMP_MAP)}, X: {x}, Y: {y}, Dir: {facing}, "
                     f"Shop: []int{{{', '.join(map(str, stock))}}}{price}}},")
    # Vending stalls around the camp (VENDORS).
    for i, (title, job, female, hair, color, x, y, facing, stock) in enumerate(vendors, 1):
        wares = ", ".join(f"{{ItemID: {it}, Price: {pr}" + (f", Element: {el}" if el else "") + "}"
                          for it, pr, el in stock)
        lines.append(f"\t{{ID: {VENDOR_ID_BASE + i}, Name: {go_str(('Merchant' if job == 5 else 'Blacksmith'))}, "
                     f"Map: {go_str(CAVE_CAMP_MAP)}, X: {x}, Y: {y}, Dir: {facing}, Vendor: &world.Vendor{{"
                     f"Title: {go_str(title)}, Job: {job}, Female: {str(female).lower()}, Hair: {hair}, HairColor: {color},")
        lines.append(f"\t\tStock: []world.VendItem{{{wares}}}}}}},")
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
