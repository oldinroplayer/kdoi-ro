#!/usr/bin/env python3
"""Extract files from a Ragnarok Online GRF archive (version 0x200).

Usage:
    grf_extract.py <file.grf> <out_dir> [--list] [--filter SUBSTRING] [--map NAME ...]

--map extracts just that map (gat/gnd/rsw + minimap) plus the models,
textures, sounds and water animation it references.

Entry names are stored in CP949 (Korean) with backslash separators; they are
decoded and written as native paths, so Korean folder names are preserved.
DES-encrypted entries (flags 0x02/0x04, used by older 0x1xx GRFs) are not
supported and are reported as skipped.
"""
import argparse
import os
import re
import struct
import sys
import zlib

HEADER_SIZE = 46
MAGIC = b"Master of Magic\0"
FLAG_FILE = 0x01
FLAG_ENCRYPTED = 0x02 | 0x04


def read_table(f):
    header = f.read(HEADER_SIZE)
    if header[:16] != MAGIC:
        sys.exit("not a GRF file")
    table_offset, seed, count, version = struct.unpack("<IIII", header[30:46])
    if version != 0x200:
        sys.exit(f"unsupported GRF version {version:#x} (only 0x200)")

    f.seek(HEADER_SIZE + table_offset)
    comp_size, _ = struct.unpack("<II", f.read(8))
    table = zlib.decompress(f.read(comp_size))

    entries = []
    pos = 0
    while pos < len(table):
        end = table.index(b"\0", pos)
        name = table[pos:end].decode("cp949", errors="replace")
        pos = end + 1
        comp, comp_aligned, real, flags, offset = struct.unpack_from("<IIIBI", table, pos)
        pos += 17
        entries.append((name, comp, comp_aligned, real, flags, offset))

    expected = count - seed - 7
    if len(entries) != expected:
        print(f"warning: table has {len(entries)} entries, header says {expected}", file=sys.stderr)
    return entries


def read_entry(f, entry):
    name, comp, comp_aligned, real, flags, offset = entry
    if flags & FLAG_ENCRYPTED:
        raise ValueError(f"encrypted entry: {name}")
    f.seek(HEADER_SIZE + offset)
    raw = f.read(comp_aligned)
    try:
        return zlib.decompress(raw[:comp])
    except zlib.error:
        # Fall back to treating the entry as stored uncompressed.
        return raw[:real]


# Runs of name-like bytes (ASCII or CP949) ending in a referenced file extension.
REF_PATTERN = re.compile(rb"[\x21-\x7e\x81-\xfe ]+?\.(?:rsm|bmp|tga|jpg|wav)(?=\x00)", re.IGNORECASE)


def find_refs(data, prefix, index):
    """Return index keys for file names embedded in `data`, resolved under `prefix`.

    Names live in fixed-size, zero-padded fields, but the preceding field may
    leave stray bytes glued to the front, so try each suffix of a match until
    one names a real entry.
    """
    found = set()
    for m in REF_PATTERN.finditer(data):
        raw = m.group(0)
        for start in range(len(raw)):
            key = prefix + raw[start:].decode("cp949", errors="replace").lower().replace("/", "\\")
            if key in index:
                found.add(key)
                break
    return found


def map_dependencies(f, index, map_name):
    """Collect every entry a map needs: gat/gnd/rsw, minimap, models, textures, sounds, water."""
    base = f"data\\{map_name.lower()}"
    needed = {base + ext for ext in (".gat", ".gnd", ".rsw")}
    missing = [n for n in needed if n not in index]
    if missing:
        sys.exit(f"map '{map_name}' not found in GRF: {missing}")
    minimap = f"data\\texture\\유저인터페이스\\map\\{map_name.lower()}.bmp"
    if minimap in index:
        needed.add(minimap)

    rsw = read_entry(f, index[base + ".rsw"])
    major, minor = rsw[4], rsw[5]
    if (major, minor) >= (1, 8):
        water_type = struct.unpack_from("<i", rsw, 170)[0]
        needed |= {k for k in index if k.startswith(f"data\\texture\\워터\\water{water_type}")}

    models = find_refs(rsw, "data\\model\\", index)
    needed |= models
    needed |= find_refs(rsw, "data\\wav\\", index)
    needed |= find_refs(read_entry(f, index[base + ".gnd"]), "data\\texture\\", index)
    for model in models:
        needed |= find_refs(read_entry(f, index[model]), "data\\texture\\", index)
    return needed


def extract_entries(f, entries, out_dir):
    """Writes entries under out_dir (keeping their data\\... paths).

    Returns (extracted, skipped, failed) counts.
    """
    out_root = os.path.abspath(out_dir)
    done = skipped = failed = 0
    for i, entry in enumerate(entries, 1):
        name, _, _, real, flags, _ = entry
        if not flags & FLAG_FILE:
            continue  # directory entry
        if flags & FLAG_ENCRYPTED:
            skipped += 1
            print(f"skip (encrypted): {name}", file=sys.stderr)
            continue

        rel = name.replace("\\", "/").lstrip("/")
        path = os.path.normpath(os.path.join(out_root, rel))
        if not path.startswith(out_root + os.sep):
            failed += 1
            print(f"skip (unsafe path): {name}", file=sys.stderr)
            continue

        data = read_entry(f, entry)
        if len(data) != real:
            failed += 1
            print(f"size mismatch ({len(data)} != {real}): {name}", file=sys.stderr)
            continue

        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "wb") as out:
            out.write(data)
        done += 1
        if i % 2000 == 0:
            print(f"  {i}/{len(entries)}", file=sys.stderr)
    return done, skipped, failed


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("grf")
    ap.add_argument("out_dir", nargs="?", default=".")
    ap.add_argument("--list", action="store_true", help="list entries instead of extracting")
    ap.add_argument("--filter", help="only entries whose path contains this (case-insensitive)")
    ap.add_argument("--map", action="append", default=[],
                    help="only this map and the models/textures it uses (repeatable, e.g. --map morocc)")
    args = ap.parse_args()

    with open(args.grf, "rb") as f:
        entries = read_table(f)
        if args.map:
            index = {e[0].lower(): e for e in entries}
            needed = set()
            for map_name in args.map:
                deps = map_dependencies(f, index, map_name)
                print(f"{map_name}: {len(deps)} files", file=sys.stderr)
                needed |= deps
            entries = [index[k] for k in sorted(needed)]
        if args.filter:
            needle = args.filter.lower()
            entries = [e for e in entries if needle in e[0].lower()]

        if args.list:
            for name, _, _, real, flags, _ in entries:
                print(f"{real:>10}  {flags:#04x}  {name}")
            return

        done, skipped, failed = extract_entries(f, entries, args.out_dir)

    print(f"extracted {done}, skipped {skipped}, failed {failed}")


if __name__ == "__main__":
    main()
