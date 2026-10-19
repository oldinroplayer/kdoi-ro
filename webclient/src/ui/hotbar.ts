// Hotkey bar (F1–F9): drop skills, healing items and wings here and press the key
// (or click the slot) to use them. Slots are remembered per character in this
// browser; drag a slot off the bar or right-click it to clear it.
import { itemIcon } from "../items";
import { skillIcon } from "../skills";
import type { PlayerState } from "../state";
import { DRAG_TYPE, hideTooltip, makeDraggable, moveTooltip, showTooltip } from "./common";

export type HotkeySlot = { kind: "item"; id: number } | { kind: "skill"; id: number } | null;

const SLOTS = 9;
/** New characters start with a Red Potion on F1 and First Aid on F2. */
const defaults: HotkeySlot[] = [{ kind: "item", id: 501 }, { kind: "skill", id: 142 }];

export class Hotbar {
  private slots: HotkeySlot[] = [];
  private readonly cells: HTMLButtonElement[] = [];
  private character = "";

  constructor(
    private readonly root: HTMLElement,
    private readonly state: PlayerState,
    private readonly actions: { useItem(index: number): void; useSkill(id: number): void; toggleSit(): void },
  ) {
    root.className = "hotbar";
    root.hidden = true;
    for (let i = 0; i < SLOTS; i++) this.cells.push(this.cell(i));
    const sit = document.createElement("button");
    sit.type = "button";
    sit.className = "hotbar-sit";
    sit.textContent = "Sit";
    sit.title = "Sit / stand (Insert)";
    sit.addEventListener("click", () => actions.toggleSit());
    root.append(...this.cells, sit);
    state.subscribe(() => !root.hidden && this.render());
  }

  /** Shows the bar for a character, loading its saved slots. */
  show(character: string): void {
    this.character = character;
    this.slots = this.load();
    this.root.hidden = false;
    this.render();
  }

  hide(): void {
    this.root.hidden = true;
  }

  /** F1–F9: returns true if the key was a hotkey. */
  handleKey(ev: KeyboardEvent): boolean {
    const m = /^F([1-9])$/.exec(ev.key);
    if (!m || this.root.hidden) return false;
    ev.preventDefault();
    this.activate(Number(m[1]) - 1);
    return true;
  }

  private activate(i: number): void {
    const slot = this.slots[i];
    if (!slot) return;
    if (slot.kind === "skill") {
      this.actions.useSkill(slot.id);
      return;
    }
    const entry = this.state.inventory.find((it) => it.itemId === slot.id && it.equip === 0);
    if (entry) this.actions.useItem(entry.index);
  }

  private cell(i: number): HTMLButtonElement {
    const b = document.createElement("button");
    b.type = "button";
    b.className = "item-cell hotbar-slot";
    b.addEventListener("click", () => this.activate(i));
    b.addEventListener("contextmenu", (ev) => {
      ev.preventDefault();
      this.set(i, null);
    });
    b.addEventListener("dragover", (ev) => {
      if (ev.dataTransfer?.types.includes(DRAG_TYPE)) {
        ev.preventDefault();
        b.classList.add("drop");
      }
    });
    b.addEventListener("dragleave", () => b.classList.remove("drop"));
    b.addEventListener("drop", (ev) => {
      b.classList.remove("drop");
      const raw = ev.dataTransfer?.getData(DRAG_TYPE);
      if (!raw) return;
      ev.preventDefault();
      const data = JSON.parse(raw) as { kind: "item" | "skill"; id: number; from?: number; hotkey?: boolean };
      if (data.hotkey === false) return; // equipment and etc items can't be used from the bar
      if (data.from !== undefined) {
        // Moved from another slot: swap.
        const other = this.slots[i];
        this.slots[data.from] = other;
      }
      this.set(i, { kind: data.kind, id: data.id });
    });
    // Dragging a slot off the bar clears it.
    b.addEventListener("dragend", (ev) => {
      if (ev.dataTransfer?.dropEffect === "none" && this.slots[i]) this.set(i, null);
    });
    b.addEventListener("mouseleave", hideTooltip);
    b.addEventListener("mousemove", moveTooltip);
    return b;
  }

  private set(i: number, slot: HotkeySlot): void {
    this.slots[i] = slot;
    this.save();
    this.render();
  }

  private render(): void {
    this.cells.forEach((b, i) => {
      const slot = this.slots[i] ?? null;
      b.replaceChildren();
      b.draggable = false;
      const key = document.createElement("span");
      key.className = "hotbar-key";
      key.textContent = `F${i + 1}`;
      b.append(key);
      b.classList.toggle("empty", !slot);
      b.classList.remove("used-up");
      let label = `F${i + 1}: empty — drag a skill, potion or wing here`;
      const img = document.createElement("img");
      img.alt = "";
      img.width = img.height = 24;

      if (slot?.kind === "item") {
        const info = this.state.items?.get(slot.id);
        const count = this.state.inventory
          .filter((it) => it.itemId === slot.id && it.equip === 0)
          .reduce((n, it) => n + it.amount, 0);
        if (info) {
          void itemIcon(info).then((src) => src && (img.src = src));
          b.append(img);
          const badge = document.createElement("span");
          badge.className = "item-amount";
          badge.textContent = String(count);
          b.append(badge);
          b.classList.toggle("used-up", count === 0);
          label = `F${i + 1}: ${info.name} (${count})`;
        }
      } else if (slot?.kind === "skill") {
        const info = this.state.skillDb?.get(slot.id);
        const level = this.state.skills.get(slot.id) ?? 0;
        if (info) {
          void skillIcon(info).then((src) => src && (img.src = src));
          b.append(img);
          b.classList.toggle("used-up", level === 0);
          label = `F${i + 1}: ${info.displayName}${level ? ` Lv ${level}` : " (not learned)"}`;
        }
      }
      if (slot) makeDraggable(b, JSON.stringify({ ...slot, from: i }));
      b.setAttribute("aria-label", label);
      b.onmouseenter = (ev) => showTooltip(label + (slot ? "\nClick or press the key to use · Right-click to clear" : ""), ev);
    });
  }

  private storageKey(): string {
    return `kdoiro.hotbar.${this.character.toLowerCase()}`;
  }

  private load(): HotkeySlot[] {
    try {
      const saved = localStorage.getItem(this.storageKey());
      if (saved) return JSON.parse(saved);
    } catch {
      // Fall through to defaults.
    }
    return [...defaults];
  }

  private save(): void {
    try {
      localStorage.setItem(this.storageKey(), JSON.stringify(this.slots));
    } catch {
      // Not remembered this time.
    }
  }
}
