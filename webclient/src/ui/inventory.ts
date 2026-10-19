// Inventory window (Alt+E): Item / Equip / Etc tabs, like RO. Worn items are
// shown in the equipment window instead.
import { audio } from "../audio";
import type { Connection, InvItem } from "../net";
import type { ItemInfo } from "../items";
import type { PlayerState } from "../state";
import { formatZeny, itemCell, toast } from "./common";
import { GameWindow } from "./window";

type Tab = "item" | "equip" | "etc";

const tabOf = (item: ItemInfo): Tab => (item.slot ? "equip" : item.type === "healing" ? "item" : "etc");

export class InventoryWindow {
  readonly window = new GameWindow("inventory", "Inventory", { x: 16, y: 120, width: 300 });
  private tab: Tab = "item";
  private readonly tabs = document.createElement("div");
  private readonly grid = document.createElement("div");
  private readonly footer = document.createElement("div");

  constructor(
    private readonly state: PlayerState,
    private readonly conn: () => Connection | null,
  ) {
    this.tabs.className = "tabs";
    this.tabs.setAttribute("role", "tablist");
    for (const [id, label] of [["item", "Item"], ["equip", "Equip"], ["etc", "Etc"]] as const) {
      const b = document.createElement("button");
      b.type = "button";
      b.setAttribute("role", "tab");
      b.dataset.tab = id;
      b.textContent = label;
      b.addEventListener("click", () => {
        this.tab = id;
        this.render();
      });
      this.tabs.append(b);
    }
    this.grid.className = "item-grid";
    this.footer.className = "window-footer";
    this.window.body.append(this.tabs, this.grid, this.footer);
    state.subscribe(() => this.window.isOpen && this.render());
    const open = this.window.open.bind(this.window);
    this.window.open = () => {
      open();
      this.render();
    };
  }

  private render(): void {
    for (const b of this.tabs.querySelectorAll<HTMLButtonElement>("button")) {
      b.setAttribute("aria-selected", String(b.dataset.tab === this.tab));
    }
    const db = this.state.items;
    this.grid.replaceChildren();
    const shown = db
      ? this.state.inventory.filter((it) => {
          const info = db.get(it.itemId);
          return info && it.equip === 0 && tabOf(info) === this.tab;
        })
      : [];
    for (const it of shown) this.grid.append(this.cell(it, db!.get(it.itemId)!));
    // Pad with empty slots so the grid keeps its shape.
    for (let i = shown.length; i < Math.max(24, Math.ceil(shown.length / 6) * 6); i++) {
      const empty = document.createElement("div");
      empty.className = "item-cell empty";
      this.grid.append(empty);
    }

    const s = this.state.status;
    this.footer.replaceChildren();
    if (s) {
      const weight = document.createElement("span");
      weight.textContent = `Weight ${s.weight} / ${s.maxWeight}`;
      if (s.weight * 2 >= s.maxWeight) weight.className = "warn";
      const zeny = document.createElement("span");
      zeny.textContent = formatZeny(s.zeny);
      this.footer.append(weight, zeny);
    }
  }

  private cell(it: InvItem, info: ItemInfo): HTMLElement {
    // Every item can be dragged onto the ground to drop it; healing items also
    // onto the hotkey bar.
    const drag = JSON.stringify({ kind: "item", id: info.id, index: it.index, amount: it.amount, hotkey: info.type === "healing" });
    if (info.slot) {
      return itemCell(info, it.amount, {
        dragData: drag,
        action: "equip",
        onActivate: () => {
          const c = this.conn();
          if (!c) return;
          if (info.equipLevel > (this.state.status?.baseLevel ?? 1)) {
            toast(`You need base level ${info.equipLevel} to equip ${info.name}.`);
            return;
          }
          c.equip(it.index);
          audio.play("se_equip_item_01.wav");
        },
      });
    }
    if (info.type === "healing") {
      return itemCell(info, it.amount, {
        action: "use",
        onActivate: () => this.conn()?.useItem(it.index),
        dragData: drag,
      });
    }
    return itemCell(info, it.amount, { dragData: drag });
  }
}
