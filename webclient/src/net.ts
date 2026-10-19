// WebSocket connection speaking the server's binary protocol (see server/README.md).

export const LoginCode = {
  OK: 0,
  Created: 1,
  WrongPassword: 2,
  InvalidName: 3,
  InvalidPassword: 4,
  AlreadyOnline: 5,
  ServerError: 6,
  ResumeFailed: 7,
} as const;

/** WebSocket close code: the server is restarting for an update. */
export const CLOSE_UPDATING = 4000;

export const loginMessages: Record<number, string> = {
  [LoginCode.WrongPassword]: "Wrong password for that character.",
  [LoginCode.InvalidName]: "Names are 4–23 letters, digits or underscores.",
  [LoginCode.InvalidPassword]: "Passwords are 4–72 characters.",
  [LoginCode.AlreadyOnline]: "That character is already online.",
  [LoginCode.ServerError]: "Server error, try again.",
  [LoginCode.ResumeFailed]: "Your session after the update expired. Please log in again.",
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
      sitting: boolean;
    }
  | { type: "look"; id: number; look: Look }
  | { type: "sit"; id: number; sitting: boolean }
  | { type: "turn"; id: number; dir: number }
  | { type: "skills"; points: number; levels: Map<number, number> }
  /** skillId 0 means the cast was cancelled. */
  | { type: "skillCast"; src: number; target: number; skillId: number; castMs: number }
  | { type: "skillUse"; src: number; target: number; skillId: number; level: number }
  /** An NPC talking to us; no options means just a Close button. */
  | { type: "npcDialog"; npcId: number; name: string; text: string; options: string[] }
  | { type: "job"; id: number; job: number }
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
      /** The skill that did it (0 = a normal attack). */
      skillId: number;
      /** Blows the damage is split over (bolts, Double Strafe, Double Attack). */
      hits: number;
    }
  /** A buff starting (ms > 0) or ending (ms = 0) on a unit. */
  | { type: "buff"; id: number; skillId: number; level: number; ms: number }
  | { type: "dead"; id: number }
  /** kind: 0 base level, 1 job level, 2 job change. */
  | { type: "levelUp"; id: number; kind: number }
  | { type: "heal"; id: number; amount: number }
  | { type: "move"; id: number; fromX: number; fromY: number; steps: [number, number][] }
  | { type: "stop"; id: number; x: number; y: number }
  | { type: "portal"; x: number; y: number }
  | { type: "npc"; id: number; name: string; sprite: string; x: number; y: number; dir: number }
  /** sellRate: percent of half price the shop pays (more than 100 with Overcharge). */
  | { type: "shopOpen"; npcId: number; npcName: string; items: { itemId: number; price: number }[]; sellRate: number }
  | { type: "shopResult"; code: number }
  | { type: "inventory"; items: InvItem[] }
  | { type: "status"; status: Status }
  | { type: "message"; text: string }
  /** The server is restarting for an update: wait up to `wait` seconds, then resume with the token. */
  | { type: "serverUpdating"; wait: number; token: string }
  /** kind 0: said out loud nearby; 1: said in our chat room. */
  | { type: "chat"; id: number; kind: number; name: string; text: string }
  | { type: "whisperFrom"; from: string; text: string }
  /** code 0 delivered, 1 not online, 2 that's you. */
  | { type: "whisperResult"; code: number; to: string }
  | { type: "roomSign"; roomId: number; ownerId: number; title: string; members: number; limit: number; public: boolean }
  | { type: "roomGone"; roomId: number }
  | { type: "roomEnter"; roomId: number; title: string; limit: number; public: boolean; owner: string; members: string[] }
  /** change 0 left, 1 joined, 2 kicked. */
  | { type: "roomMember"; change: number; name: string }
  | { type: "roomResult"; code: number }
  /** reason 0 we left, 1 we were kicked. */
  | { type: "roomLeft"; reason: number }
  | { type: "roomSettings"; title: string; limit: number; public: boolean; owner: string }
  /** An item on the ground; sub-cell position 0-15; dropped = it just fell. */
  | { type: "floorItem"; id: number; itemId: number; amount: number; x: number; y: number; subX: number; subY: number; dropped: boolean }
  | { type: "floorItemGone"; id: number }
  | { type: "pickUp"; id: number }
  /** An emoticon over a unit's head (index into emotion.act). */
  | { type: "emotion"; id: number; kind: number }
  | { type: "partyInfo"; party: PartyInfo }
  | { type: "partyInvited"; partyId: number; partyName: string; from: string }
  | { type: "partyLeft" }
  /** A unit's new walk speed (ms per straight cell), e.g. under Increase AGI. */
  | { type: "speed"; id: number; speed: number }
  /** A one-off effect on a unit (kind 1: teleporting away with a wing). */
  | { type: "effect"; id: number; kind: number };

/** A party member as the party window shows it (offline: name only). */
export interface PartyMember {
  name: string;
  online: boolean;
  map: string;
  x: number;
  y: number;
  hp: number;
  maxHp: number;
  job: number;
  baseLevel: number;
}

export interface PartyInfo {
  id: number;
  name: string;
  leader: string;
  evenExp: boolean;
  members: PartyMember[];
}

/** A chat room's title, size and privacy. */
export interface RoomSettings {
  title: string;
  limit: number;
  public: boolean;
  password: string;
}

/** Visible equipment as item IDs (0 = none). */
export interface Look {
  weapon: number;
  shield: number;
  headTop: number;
  headMid: number;
  headLow: number;
  /** Hairstyle 1..19 (0 = not chosen yet: draw the default and ask). */
  hair: number;
  hairColor: number;
  female: boolean;
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
  /** RO class ID: 0 Novice, 1 Swordman ... 6 Thief. */
  job: number;
  /** Job level stat bonuses (STR..LUK), shown as "9 + 1". */
  bonus: number[];
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
  const look = (): Look => ({
    weapon: u16(), shield: u16(), headTop: u16(), headMid: u16(), headLow: u16(), hair: u16(), hairColor: u16(),
    female: u16() === 1,
  });
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
      const appearance = look();
      return { type: "appear", id, name, job, x, y, dir, speed, dead, look: appearance, sitting: u8() === 1 };
    }
    case 0x8014:
      return { type: "sit", id: u32(), sitting: u8() === 1 };
    case 0x8015:
      return { type: "turn", id: u32(), dir: u8() };
    case 0x8016: {
      const points = u16(), n = u16();
      const levels = new Map<number, number>();
      for (let i = 0; i < n; i++) levels.set(u16(), u8());
      return { type: "skills", points, levels };
    }
    case 0x8017:
      return { type: "skillCast", src: u32(), target: u32(), skillId: u16(), castMs: u16() };
    case 0x8018:
      return { type: "skillUse", src: u32(), target: u32(), skillId: u16(), level: u8() };
    case 0x8019: {
      const npcId = u32(), name = str();
      const len = u16();
      o += len;
      const text = textDecoder.decode(new Uint8Array(buf, o - len, len));
      const n = u8();
      const options: string[] = [];
      for (let i = 0; i < n; i++) options.push(str());
      return { type: "npcDialog", npcId, name, text, options };
    }
    case 0x801c:
      return { type: "chat", id: u32(), kind: u8(), name: str(), text: str() };
    case 0x801d:
      return { type: "whisperFrom", from: str(), text: str() };
    case 0x801e:
      return { type: "whisperResult", code: u8(), to: str() };
    case 0x801f:
      return { type: "roomSign", roomId: u32(), ownerId: u32(), title: str(), members: u8(), limit: u8(), public: u8() === 1 };
    case 0x8020:
      return { type: "roomGone", roomId: u32() };
    case 0x8021: {
      const roomId = u32(), title = str(), limit = u8(), isPublic = u8() === 1, owner = str(), n = u8();
      const members: string[] = [];
      for (let i = 0; i < n; i++) members.push(str());
      return { type: "roomEnter", roomId, title, limit, public: isPublic, owner, members };
    }
    case 0x8022:
      return { type: "roomMember", change: u8(), name: str() };
    case 0x8023:
      return { type: "roomResult", code: u8() };
    case 0x8024:
      return { type: "roomLeft", reason: u8() };
    case 0x8025:
      return { type: "roomSettings", title: str(), limit: u8(), public: u8() === 1, owner: str() };
    case 0x8026:
      return {
        type: "floorItem", id: u32(), itemId: u16(), amount: u16(), x: u16(), y: u16(), subX: u8(), subY: u8(), dropped: u8() === 1,
      };
    case 0x8027:
      return { type: "floorItemGone", id: u32() };
    case 0x802d:
      return { type: "speed", id: u32(), speed: u16() };
    case 0x802e:
      return { type: "serverUpdating", wait: u16(), token: str() };
    case 0x802f:
      return { type: "effect", id: u32(), kind: u8() };
    case 0x802a: {
      const id = u32(), name = str(), leader = str(), evenExp = u8() === 1, n = u8();
      const members: PartyMember[] = [];
      for (let i = 0; i < n; i++) {
        members.push({
          name: str(), online: u8() === 1, map: str(), x: u16(), y: u16(),
          hp: u32(), maxHp: u32(), job: u16(), baseLevel: u16(),
        });
      }
      return { type: "partyInfo", party: { id, name, leader, evenExp, members } };
    }
    case 0x802b:
      return { type: "partyInvited", partyId: u32(), partyName: str(), from: str() };
    case 0x802c:
      return { type: "partyLeft" };
    case 0x8029:
      return { type: "emotion", id: u32(), kind: u8() };
    case 0x8028:
      return { type: "pickUp", id: u32() };
    case 0x801b:
      return { type: "buff", id: u32(), skillId: u16(), level: u8(), ms: u32() };
    case 0x801a:
      return { type: "job", id: u32(), job: u16() };
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
      return { type: "shopOpen", npcId, npcName, items, sellRate: u16() };
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
        job: u16(), bonus: [u8(), u8(), u8(), u8(), u8(), u8()],
      };
      return { type: "status", status };
    }
    case 0x800d:
      return { type: "message", text: str() };
    case 0x800e:
      return { type: "monster", id: u32(), mobId: u16(), name: str(), sprite: str(), x: u16(), y: u16(), dir: u8(), speed: u16() };
    case 0x800f: {
      const src = u32(), target = u32(), damage = u32(), flags = u8();
      const motion = u16(), hurt = u16();
      return { type: "damage", src, target, damage, miss: (flags & 1) !== 0, crit: (flags & 2) !== 0, motion, hurt, skillId: u16(), hits: u8() };
    }
    case 0x8010:
      return { type: "dead", id: u32() };
    case 0x8011:
      return { type: "levelUp", id: u32(), kind: u8() };
    case 0x8012:
      return { type: "heal", id: u32(), amount: u16() };
    default:
      return null;
  }
}

export class Connection {
  private ws: WebSocket | null = null;
  onPacket: (p: ServerPacket) => void = () => {};
  onClose: (reason: string, code: number) => void = () => {};

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
        if (opened) this.onClose(e.reason || "Disconnected from server.", e.code);
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

  /** Logs back in after a server update with the token it handed out. */
  resume(token: string): void {
    this.sendFields(0x0024, [token]);
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

  /** Sit down (true) or stand up (false). */
  sit(sitting: boolean): void {
    this.send(0x000c, (v) => v.setUint8(2, sitting ? 1 : 0), 1);
  }

  /** Answer an NPC dialog: an option index, or 255 to walk away. */
  npcReply(npcId: number, choice: number): void {
    this.send(
      0x0010,
      (v) => {
        v.setUint32(2, npcId, true);
        v.setUint8(6, choice);
      },
      5,
    );
  }

  /** Spend a skill point on a skill. */
  learnSkill(skillId: number): void {
    this.send(0x000e, (v) => v.setUint16(2, skillId, true), 2);
  }

  /** Use a skill (level 0 = highest learned) on a target unit (0 for self skills). */
  useSkill(skillId: number, level = 0, targetId = 0): void {
    this.send(
      0x000f,
      (v) => {
        v.setUint16(2, skillId, true);
        v.setUint16(4, level, true);
        v.setUint32(6, targetId, true);
      },
      8,
    );
  }

  /** Face a direction without moving (0 N, 1 NW ... 7 NE). */
  turn(dir: number): void {
    this.send(0x000d, (v) => v.setUint8(2, dir), 1);
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

  /** Say something: out loud to everyone nearby, or to our chat room. */
  chat(text: string): void {
    this.sendFields(0x0011, [text]);
  }

  whisper(to: string, text: string): void {
    this.sendFields(0x0012, [to, text]);
  }

  createRoom(s: RoomSettings): void {
    this.sendFields(0x0013, roomFields(s));
  }

  updateRoom(s: RoomSettings): void {
    this.sendFields(0x0017, roomFields(s));
  }

  joinRoom(roomId: number, password = ""): void {
    this.sendFields(0x0014, [{ u32: roomId }, password]);
  }

  leaveRoom(): void {
    this.sendFields(0x0015, []);
  }

  kickFromRoom(name: string): void {
    this.sendFields(0x0016, [name]);
  }

  /** Hand our chat room to another member. */
  giveRoom(name: string): void {
    this.sendFields(0x0018, [name]);
  }

  /** Sends strings (u8 length + UTF-8, at most 255 bytes) and numbers. */
  private sendFields(op: number, fields: (string | { u8: number } | { u32: number })[]): void {
    const parts: number[] = [op & 0xff, op >> 8];
    for (const f of fields) {
      if (typeof f === "string") {
        const b = textEncoder.encode(f).slice(0, 255);
        parts.push(b.length, ...b);
      } else if ("u8" in f) {
        parts.push(f.u8 & 0xff);
      } else {
        parts.push(f.u32 & 0xff, (f.u32 >> 8) & 0xff, (f.u32 >> 16) & 0xff, (f.u32 >>> 24) & 0xff);
      }
    }
    this.ws?.send(new Uint8Array(parts));
  }

  createParty(name: string): void {
    this.sendFields(0x001d, [name]);
  }

  inviteToParty(name: string): void {
    this.sendFields(0x001e, [name]);
  }

  replyPartyInvite(partyId: number, accept: boolean): void {
    this.sendFields(0x001f, [{ u32: partyId }, { u8: accept ? 1 : 0 }]);
  }

  leaveParty(): void {
    this.sendFields(0x0020, []);
  }

  kickFromParty(name: string): void {
    this.sendFields(0x0021, [name]);
  }

  partyChat(text: string): void {
    this.sendFields(0x0022, [text]);
  }

  /** Even EXP share (true) or each take (false); leader only. */
  setPartyExp(even: boolean): void {
    this.sendFields(0x0023, [{ u8: even ? 1 : 0 }]);
  }

  /** Show an emoticon over our head (0..46). */
  emote(kind: number): void {
    this.sendFields(0x001c, [{ u8: kind }]);
  }

  /** Walk to an item on the ground and pick it up. */
  pickUp(floorItemId: number): void {
    this.send(0x0019, (v) => v.setUint32(2, floorItemId, true), 4);
  }

  /** Change our hairstyle (1..19), hair colour (0..8) and gender. */
  setAppearance(style: number, color: number, female: boolean): void {
    this.sendFields(0x001b, [{ u8: style }, { u8: color }, { u8: female ? 1 : 0 }]);
  }

  /** Drop inventory items on the ground at our feet. */
  dropItem(index: number, amount: number): void {
    this.send(
      0x001a,
      (v) => {
        v.setUint16(2, index, true);
        v.setUint16(4, amount, true);
      },
      4,
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

function roomFields(s: RoomSettings) {
  return [s.title, { u8: s.limit }, { u8: s.public ? 1 : 0 }, s.public ? "" : s.password];
}
