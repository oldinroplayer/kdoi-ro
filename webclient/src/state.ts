// The player's own state as last reported by the server; UI windows render from it.
import type { InvItem, Status } from "./net";
import type { ItemDb } from "./items";
import type { SkillDb } from "./skills";

export interface ShopState {
  npcId: number;
  npcName: string;
  items: { itemId: number; price: number }[];
  /** Percent of half price the shop pays for our items (Overcharge raises it). */
  sellRate: number;
}

/** A buff on our character: its level and when it wears off (performance.now() ms). */
export interface Buff {
  level: number;
  until: number;
}

type Listener = () => void;

export class PlayerState {
  name = "";
  items: ItemDb | null = null;
  skillDb: SkillDb | null = null;
  /** Learned level per skill in our tree (0 = not learned), and unspent points. */
  skills = new Map<number, number>();
  skillPoints = 0;
  inventory: InvItem[] = [];
  status: Status | null = null;
  shop: ShopState | null = null;
  /** Running buffs by skill ID. */
  buffs = new Map<number, Buff>();
  private readonly listeners = new Set<Listener>();

  subscribe(fn: Listener): () => void {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }

  changed(): void {
    for (const fn of this.listeners) fn();
  }

  reset(): void {
    this.name = "";
    this.inventory = [];
    this.status = null;
    this.skills = new Map();
    this.skillPoints = 0;
    this.shop = null;
    this.buffs = new Map();
    this.changed();
  }

  /** The item worn in an equip slot, if any. */
  equippedIn(slot: number): InvItem | undefined {
    return this.inventory.find((it) => (it.equip & slot) !== 0);
  }
}
