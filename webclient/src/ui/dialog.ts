// NPC dialog: the NPC's words and the replies you can give (RO's message box).
import type { Connection } from "../net";
import { GameWindow } from "./window";

export class DialogWindow {
  readonly window = new GameWindow("dialog", "NPC", { x: 240, y: 140, width: 360 });
  private npcId = 0;
  /** True while the NPC is waiting for one of our options. */
  private awaiting = false;
  private readonly name = document.createElement("h3");
  private readonly text = document.createElement("div");
  private readonly options = document.createElement("div");

  constructor(private readonly conn: () => Connection | null) {
    this.name.className = "dialog-name";
    this.text.className = "dialog-text";
    this.options.className = "dialog-options";
    this.window.body.append(this.name, this.text, this.options);
    // Closing the window mid-question tells the NPC we walked away.
    this.window.onClose = () => {
      if (this.awaiting) this.conn()?.npcReply(this.npcId, 255);
      this.awaiting = false;
    };
  }

  show(npcId: number, name: string, text: string, options: string[]): void {
    this.npcId = npcId;
    this.awaiting = options.length > 0;
    this.name.textContent = name;
    this.text.replaceChildren(...text.split("\n").map((line) => {
      const p = document.createElement("p");
      p.textContent = line;
      return p;
    }));
    const buttons = options.length
      ? options.map((label, i) => this.button(label, () => this.reply(i)))
      : [this.button("Close", () => this.window.close())];
    this.options.replaceChildren(...buttons);
    this.window.open();
    buttons[0]?.focus();
  }

  private reply(choice: number): void {
    this.awaiting = false;
    this.conn()?.npcReply(this.npcId, choice);
    this.window.close();
  }

  private button(label: string, onClick: () => void): HTMLButtonElement {
    const b = document.createElement("button");
    b.type = "button";
    b.className = "secondary dialog-option";
    b.textContent = label;
    b.addEventListener("click", onClick);
    return b;
  }
}
