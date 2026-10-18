// WebSocket connection speaking the server's binary protocol (see server/README.md).

export const LoginCode = {
  OK: 0,
  Created: 1,
  WrongPassword: 2,
  InvalidName: 3,
  InvalidPassword: 4,
  AlreadyOnline: 5,
  ServerError: 6,
} as const;

export const loginMessages: Record<number, string> = {
  [LoginCode.WrongPassword]: "Wrong password for that character.",
  [LoginCode.InvalidName]: "Names are 4–23 letters, digits or underscores.",
  [LoginCode.InvalidPassword]: "Passwords are 4–72 characters.",
  [LoginCode.AlreadyOnline]: "That character is already online.",
  [LoginCode.ServerError]: "Server error, try again.",
};

export type ServerPacket =
  | { type: "loginResult"; code: number }
  | { type: "enterMap"; selfId: number; map: string; x: number; y: number; dir: number; job: number; speed: number; look: Look }
  | {
      type: "appear";
      id: number;
      name: string;
      job: number;
      x: number;
      y: number;
      dir: number;
      speed: number;
      dead: boolean;
      look: Look;
    }
  | { type: "look"; id: number; look: Look }
  | { type: "disappear"; id: number; died: boolean }
  | { type: "monster"; id: number; mobId: number; name: string; sprite: string; x: number; y: number; dir: number; speed: number }
  | {
      type: "damage";
      src: number;
      target: number;
      damage: number;
      miss: boolean;
      crit: boolean;
      /** Attack animation length in ms. */
      motion: number;
      /** How long the target flinches in ms (0 = no flinch). */
      hurt: number;
    }
  | { type: "dead"; id: number }
  | { type: "levelUp"; id: number; job: boolean }
  | { type: "heal"; id: number; amount: number }
  | { type: "move"; id: number; fromX: number; fromY: number; steps: [number, number][] }
  | { type: "stop"; id: number; x: number; y: number }
  | { type: "portal"; x: number; y: number }
  | { type: "npc"; id: number; name: string; sprite: string; x: number; y: number; dir: number }
  | { type: "shopOpen"; npcId: number; npcName: string; items: { itemId: number; price: number }[] }
  | { type: "shopResult"; code: number }
  | { type: "inventory"; items: InvItem[] }
  | { type: "status"; status: Status }
  | { type: "message"; text: string };

/** Visible equipment as item IDs (0 = none). */
export interface Look {
  weapon: number;
  shield: number;
  headTop: number;
  headMid: number;
  headLow: number;
}

export interface InvItem {
  index: number;
  itemId: number;
  amount: number;
  /** Equip slot bitmask it's worn in; 0 when not worn. */
  equip: number;
}

export interface Status {
  baseLevel: number;
  jobLevel: number;
  /** STR, AGI, VIT, INT, DEX, LUK */
  stats: number[];
  raiseCosts: number[];
  statusPoints: number;
  zeny: number;
  hp: number;
  maxHp: number;
  sp: number;
  maxSp: number;
  atk: number;
  atk2: number;
  matkMin: number;
  matkMax: number;
  def: number;
  def2: number;
  mdef: number;
  mdef2: number;
  hit: number;
  flee: number;
  flee2: number;
  crit: number;
  aspd: number;
  weight: number;
  maxWeight: number;
  baseExp: number;
  /** EXP needed for the next level; 0 at the level cap. */
  baseNext: number;
  jobExp: number;
  jobNext: number;
}

export const ShopCode = { OK: 0, NoZeny: 1, Overweight: 2, Full: 3, Invalid: 4, Sold: 5 } as const;

export const shopMessages: Record<number, string> = {
  [ShopCode.OK]: "Purchase complete.",
  [ShopCode.NoZeny]: "You don't have enough zeny.",
  [ShopCode.Overweight]: "You can't carry that much weight.",
  [ShopCode.Full]: "Your inventory is full.",
  [ShopCode.Invalid]: "The shopkeeper won't trade that.",
  [ShopCode.Sold]: "Sold.",
};

const textDecoder = new TextDecoder();
const textEncoder = new TextEncoder();

function decode(buf: ArrayBuffer): ServerPacket | null {
  const v = new DataView(buf);
  let o = 0;
  const u8 = () => v.getUint8(o++);
  const u16 = () => ((o += 2), v.getUint16(o - 2, true));
  const u32 = () => ((o += 4), v.getUint32(o - 4, true));
  const look = (): Look => ({ weapon: u16(), shield: u16(), headTop: u16(), headMid: u16(), headLow: u16() });
  const str = () => {
    const n = u8();
    o += n;
    return textDecoder.decode(new Uint8Array(buf, o - n, n));
  };

  switch (u16()) {
    case 0x8001:
      return { type: "loginResult", code: u8() };
    case 0x8002:
      return { type: "enterMap", selfId: u32(), map: str(), x: u16(), y: u16(), dir: u8(), job: u16(), speed: u16(), look: look() };
    case 0x8003: {
      const id = u32(), name = str(), job = u16(), x = u16(), y = u16(), dir = u8(), speed = u16(), dead = u8() === 1;
      return { type: "appear", id, name, job, x, y, dir, speed, dead, look: look() };
    }
    case 0x8013:
      return { type: "look", id: u32(), look: look() };
    case 0x8004:
      return { type: "disappear", id: u32(), died: u8() === 1 };
    case 0x8005: {
      const id = u32(), fromX = u16(), fromY = u16(), n = u16();
      const steps: [number, number][] = [];
      for (let i = 0; i < n; i++) steps.push([u16(), u16()]);
      return { type: "move", id, fromX, fromY, steps };
    }
    case 0x8006:
      return { type: "stop", id: u32(), x: u16(), y: u16() };
    case 0x8007:
      return { type: "portal", x: u16(), y: u16() };
    case 0x8008:
      return { type: "npc", id: u32(), name: str(), sprite: str(), x: u16(), y: u16(), dir: u8() };
    case 0x8009: {
      const npcId = u32(), npcName = str(), n = u16();
      const items = [];
      for (let i = 0; i < n; i++) items.push({ itemId: u16(), price: u32() });
      return { type: "shopOpen", npcId, npcName, items };
    }
    case 0x800a:
      return { type: "shopResult", code: u8() };
    case 0x800b: {
      const n = u16();
      const items: InvItem[] = [];
      for (let i = 0; i < n; i++) items.push({ index: u16(), itemId: u16(), amount: u16(), equip: u16() });
      return { type: "inventory", items };
    }
    case 0x800c: {
      const baseLevel = u16(), jobLevel = u16();
      const stats = [u8(), u8(), u8(), u8(), u8(), u8()];
      const raiseCosts = [u8(), u8(), u8(), u8(), u8(), u8()];
      const statusPoints = u16(), zeny = u32();
      const [hp, maxHp, sp, maxSp, atk, atk2, matkMin, matkMax, def, def2, mdef, mdef2, hit, flee, flee2, crit, aspd] =
        Array.from({ length: 17 }, u16);
      const status: Status = {
        baseLevel, jobLevel, stats, raiseCosts, statusPoints, zeny,
        hp, maxHp, sp, maxSp, atk, atk2, matkMin, matkMax, def, def2, mdef, mdef2, hit, flee, flee2, crit, aspd,
        weight: u32(), maxWeight: u32(),
        baseExp: u32(), baseNext: u32(), jobExp: u32(), jobNext: u32(),
      };
      return { type: "status", status };
    }
    case 0x800d:
      return { type: "message", text: str() };
    case 0x800e:
      return { type: "monster", id: u32(), mobId: u16(), name: str(), sprite: str(), x: u16(), y: u16(), dir: u8(), speed: u16() };
    case 0x800f: {
      const src = u32(), target = u32(), damage = u32(), flags = u8();
      return { type: "damage", src, target, damage, miss: (flags & 1) !== 0, crit: (flags & 2) !== 0, motion: u16(), hurt: u16() };
    }
    case 0x8010:
      return { type: "dead", id: u32() };
    case 0x8011:
      return { type: "levelUp", id: u32(), job: u8() === 1 };
    case 0x8012:
      return { type: "heal", id: u32(), amount: u16() };
    default:
      return null;
  }
}

export class Connection {
  private ws: WebSocket | null = null;
  onPacket: (p: ServerPacket) => void = () => {};
  onClose: (reason: string) => void = () => {};

  connect(url: string): Promise<void> {
    return new Promise((resolve, reject) => {
      const ws = new WebSocket(url);
      ws.binaryType = "arraybuffer";
      let opened = false;
      ws.onopen = () => {
        opened = true;
        resolve();
      };
      ws.onerror = () => {
        if (!opened) reject(new Error(`Can't reach the server at ${url}.`));
      };
      ws.onclose = (e) => {
        this.ws = null;
        if (opened) this.onClose(e.reason || "Disconnected from server.");
      };
      ws.onmessage = (e) => {
        if (!(e.data instanceof ArrayBuffer)) return;
        try {
          const p = decode(e.data);
          if (p) this.onPacket(p);
        } catch (err) {
          console.error("bad packet", err);
        }
      };
      this.ws = ws;
    });
  }

  get connected(): boolean {
    return this.ws?.readyState === WebSocket.OPEN;
  }

  close(): void {
    this.ws?.close();
  }

  login(name: string, password: string): void {
    const n = textEncoder.encode(name).slice(0, 255);
    const p = textEncoder.encode(password).slice(0, 255);
    const b = new Uint8Array(2 + 1 + n.length + 1 + p.length);
    const v = new DataView(b.buffer);
    v.setUint16(0, 0x0001, true);
    b[2] = n.length;
    b.set(n, 3);
    b[3 + n.length] = p.length;
    b.set(p, 4 + n.length);
    this.ws?.send(b);
  }

  npcClick(npcId: number): void {
    this.send(0x0003, (v) => v.setUint32(2, npcId, true), 4);
  }

  /** Buys an order from an NPC shop: [itemId, amount] pairs. */
  buy(npcId: number, lines: [number, number][]): void {
    this.send(
      0x0004,
      (v) => {
        v.setUint32(2, npcId, true);
        v.setUint16(6, lines.length, true);
        lines.forEach(([id, amount], i) => {
          v.setUint16(8 + i * 4, id, true);
          v.setUint16(10 + i * 4, amount, true);
        });
      },
      6 + lines.length * 4,
    );
  }

  equip(index: number): void {
    this.send(0x0005, (v) => v.setUint16(2, index, true), 2);
  }

  unequip(index: number): void {
    this.send(0x0006, (v) => v.setUint16(2, index, true), 2);
  }

  /** Auto-attack a monster until it dies or we move. */
  attack(targetId: number): void {
    this.send(0x0008, (v) => v.setUint32(2, targetId, true), 4);
  }

  useItem(index: number): void {
    this.send(0x0009, (v) => v.setUint16(2, index, true), 2);
  }

  /** Return to the save point after being knocked out. */
  respawn(): void {
    this.send(0x000a, () => {}, 0);
  }

  /** Sells [inventory index, amount] pairs to an NPC shop. */
  sell(npcId: number, lines: [number, number][]): void {
    this.send(
      0x000b,
      (v) => {
        v.setUint32(2, npcId, true);
        v.setUint16(6, lines.length, true);
        lines.forEach(([index, amount], i) => {
          v.setUint16(8 + i * 4, index, true);
          v.setUint16(10 + i * 4, amount, true);
        });
      },
      6 + lines.length * 4,
    );
  }

  /** stat: 0 STR, 1 AGI, 2 VIT, 3 INT, 4 DEX, 5 LUK */
  raiseStat(stat: number): void {
    this.send(0x0007, (v) => v.setUint8(2, stat), 1);
  }

  private send(op: number, fill: (v: DataView) => void, bodySize: number): void {
    const v = new DataView(new ArrayBuffer(2 + bodySize));
    v.setUint16(0, op, true);
    fill(v);
    this.ws?.send(v.buffer);
  }

  move(x: number, y: number): void {
    const b = new DataView(new ArrayBuffer(6));
    b.setUint16(0, 0x0002, true);
    b.setUint16(2, x, true);
    b.setUint16(4, y, true);
    this.ws?.send(b.buffer);
  }
}
