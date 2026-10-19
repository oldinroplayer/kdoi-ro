// Item database: game rules from the server's /items.json, plus display names,
// descriptions and icons from the original client's data tables (as RO does).
import { fetchData, loadKeyedCanvas } from "./assets";

export interface ItemInfo {
  id: number;
  type: "healing" | "weapon" | "armor" | "ammo" | "warp" | "card" | "etc";
  slot: number; // equip location bitmask, 0 if not equippable
  /** Card slots in this equipment (0-4). */
  slots: number;
  /** Cards: the equipment they go into (an equip location bitmask). */
  cardLoc: number;
  atk: number;
  def: number;
  weight: number;
  price: number;
  equipLevel: number;
  name: string;
  resName: string; // Korean resource name used for icon files
  description: string; // may contain RO colour codes (^RRGGBB)
  /** Arrows: the element they give bow shots (0 neutral, 3 fire...). */
  element?: number;
  /** Sprite layer name for equipment shown on the character, e.g. "단검". */
  sprite: string;
}

const cp949 = new TextDecoder("euc-kr");

export class ItemDb {
  private constructor(
    private readonly items: Map<number, ItemInfo>,
    /** Card ID -> the word it adds to an item's name ("Lucky"). */
    private readonly cardNames: Map<number, string>,
    /** Cards whose word goes after the item's name instead of before. */
    private readonly postfixCards: Set<number>,
  ) {}

  static async load(wsUrl: string): Promise<ItemDb> {
    const http = new URL(wsUrl);
    http.protocol = http.protocol === "wss:" ? "https:" : "http:";
    http.pathname = "/items.json";
    http.search = "";

    const [defs, names, resNames, descs, cardNames, postfixCards] = await Promise.all([
      fetch(http).then((r) => {
        if (!r.ok) throw new Error(`items.json: HTTP ${r.status}`);
        return r.json() as Promise<Partial<ItemInfo>[]>;
      }),
      fetchData("idnum2itemdisplaynametable.txt").then(parseTable),
      fetchData("idnum2itemresnametable.txt").then(parseTable),
      fetchData("idnum2itemdesctable.txt").then(parseDescTable),
      // The client's card naming tables; an old data set without them just
      // names items plainly.
      fetchData("cardprefixnametable.txt").then(parseTable).catch(() => new Map<number, string>()),
      fetchData("cardpostfixnametable.txt").then(parseIds).catch(() => new Set<number>()),
    ]);

    const items = new Map<number, ItemInfo>();
    for (const d of defs) {
      const id = d.id!;
      items.set(id, {
        id,
        type: d.type!,
        slot: d.slot ?? 0,
        slots: d.slots ?? 0,
        cardLoc: d.cardLoc ?? 0,
        atk: d.atk ?? 0,
        def: d.def ?? 0,
        weight: d.weight ?? 0,
        price: d.price ?? 0,
        equipLevel: d.equipLevel ?? 0,
        name: (names.get(id) ?? d.name ?? `Item ${id}`).replace(/_/g, " "),
        resName: resNames.get(id) ?? "",
        description: descs.get(id) ?? "",
        sprite: d.sprite ?? "",
      });
    }
    return new ItemDb(items, cardNames, postfixCards);
  }

  get(id: number): ItemInfo | undefined {
    return this.items.get(id);
  }

  /**
   * An item's name as the client shows it: its cards' words around the name
   * (two of a card: "Double", three "Triple", four "Quadruple"), then its
   * slot count, e.g. "Double Vital Knife [3]".
   */
  label(id: number, cards: readonly number[] = [], element = 0): string {
    const info = this.items.get(id);
    let name = info?.name ?? `Item ${id}`;
    // A Blacksmith's forged weapon: its element before the name ("Fire Main Gauche").
    if (FORGED[element]) name = `${FORGED[element]} ${name}`;
    const counts = new Map<number, number>();
    for (const c of cards) counts.set(c, (counts.get(c) ?? 0) + 1);
    const before: string[] = [];
    const after: string[] = [];
    for (const [card, n] of counts) {
      const word = this.cardNames.get(card);
      if (!word) continue;
      const text = `${MULTIPLE[n] ?? ""}${word}`;
      (this.postfixCards.has(card) ? after : before).push(text);
    }
    if (before.length) name = `${before.join(" ")} ${name}`;
    if (after.length) name = `${name} ${after.join(" ")}`;
    return info?.slots ? `${name} [${info.slots}]` : name;
  }
}

/** How the client names forged weapons by element (water, earth, fire, wind). */
const FORGED: Record<number, string> = { 1: "Ice", 2: "Earth", 3: "Fire", 4: "Wind" };

/** How the client names two, three or four of the same card. */
const MULTIPLE: Record<number, string> = { 2: "Double ", 3: "Triple ", 4: "Quadruple " };

/** "4014#" lines -> the IDs listed. */
function parseIds(buf: ArrayBuffer): Set<number> {
  const out = new Set<number>();
  for (const line of cp949.decode(buf).split(/\r?\n/)) {
    const m = /^(\d+)#/.exec(line);
    if (m) out.add(Number(m[1]));
  }
  return out;
}

/** "1201#Knife#" lines -> id -> value. */
function parseTable(buf: ArrayBuffer): Map<number, string> {
  const out = new Map<number, string>();
  for (const line of cp949.decode(buf).split(/\r?\n/)) {
    const m = /^(\d+)#([^#]*)#/.exec(line);
    if (m) out.set(Number(m[1]), m[2]);
  }
  return out;
}

/** Blocks of "1201#", description lines, "#". */
function parseDescTable(buf: ArrayBuffer): Map<number, string> {
  const out = new Map<number, string>();
  const re = /^(\d+)#\r?\n([\s\S]*?)^#/gm;
  const text = cp949.decode(buf);
  for (let m = re.exec(text); m; m = re.exec(text)) out.set(Number(m[1]), m[2].trim());
  return out;
}

/** Renders RO description text (with ^RRGGBB colour codes) into safe DOM nodes. */
export function renderDescription(text: string, into: HTMLElement): void {
  into.replaceChildren();
  for (const line of text.split(/\r?\n/)) {
    const p = document.createElement("div");
    let color = "";
    for (const part of line.split(/(\^[0-9a-fA-F]{6})/)) {
      if (/^\^[0-9a-fA-F]{6}$/.test(part)) {
        color = part === "^000000" ? "" : `#${part.slice(1)}`;
        continue;
      }
      if (!part) continue;
      const span = document.createElement("span");
      span.textContent = part;
      // RO's dark grey (^777777) reads poorly on a dark panel; brighten it.
      if (color) span.style.color = color === "#777777" ? "var(--accent)" : color;
      p.append(span);
    }
    if (!p.childNodes.length) p.append(document.createElement("br"));
    into.append(p);
  }
}

const imageCache = new Map<string, Promise<string>>();

/** Item icon (24x24) as a data URL with the magenta key made transparent. */
export function itemIcon(item: ItemInfo): Promise<string> {
  return keyedImage(`texture/유저인터페이스/item/${item.resName}.bmp`);
}

/** Larger item illustration shown in the item info window. */
export function itemIllustration(item: ItemInfo): Promise<string> {
  return keyedImage(`texture/유저인터페이스/collection/${item.resName}.bmp`);
}

/** A BMP from data/ as a data URL, magenta made transparent (cached). */
export function keyedImage(path: string): Promise<string> {
  let p = imageCache.get(path);
  if (!p) {
    p = loadKeyedCanvas(path)
      .then((canvas) => canvas?.toDataURL() ?? "")
      .catch(() => "");
    imageCache.set(path, p);
  }
  return p;
}
