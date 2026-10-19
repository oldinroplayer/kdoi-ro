// Dropping items on the ground: dragging a stack out of the inventory onto the
// map asks how many, like RO's amount dialog.
import type { Connection } from "../net";
import { GameWindow } from "./window";

export class DropAmountWindow {
  readonly window = new GameWindow("drop-amount", "Drop Items", { x: 280, y: 200, width: 260 });
  private readonly prompt = document.createElement("p");
  private readonly amount = document.createElement("input");
  private index = 0;

  constructor(private readonly conn: () => Connection | null) {
    const form = document.createElement("form");
    form.className = "room-settings";
    this.amount.type = "number";
    this.amount.min = "1";
    this.amount.required = true;
    this.amount.setAttribute("aria-label", "How many");
    const drop = document.createElement("button");
    drop.type = "submit";
    drop.textContent = "Drop";
    form.append(this.prompt, this.amount, drop);
    form.addEventListener("submit", (ev) => {
      ev.preventDefault();
      const n = Math.floor(Number(this.amount.value));
      if (n >= 1 && n <= Number(this.amount.max)) this.conn()?.dropItem(this.index, n);
      this.window.close();
    });
    this.window.body.append(form);
  }

  /** Drops a single item at once, or asks how many of a stack. */
  drop(index: number, name: string, owned: number): void {
    if (owned <= 1) {
      this.conn()?.dropItem(index, 1);
      return;
    }
    this.index = index;
    this.prompt.textContent = `How many ${name} to drop?`;
    this.amount.max = String(owned);
    this.amount.value = String(owned);
    this.window.open();
    this.amount.select();
  }
}
