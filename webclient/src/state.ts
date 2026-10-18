// The player's own state as last reported by the server; UI windows render from it.
import type { InvItem, Status } from "./net";
import type { ItemDb } from "./items";

export interface ShopState {
  npcId: number;
  npcName: string;
  items: { itemId: number; price: number }[];
}

type Listener = () => void;

export class PlayerState {
  name = "";
  items: ItemDb | null = null;
  inventory: InvItem[] = [];
  status: Status | null = null;
  shop: ShopState | null = null;
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
    this.shop = null;
    this.changed();
  }

  /** The item worn in an equip slot, if any. */
  equippedIn(slot: number): InvItem | undefined {
    return this.inventory.find((it) => (it.equip & slot) !== 0);
  }
}
