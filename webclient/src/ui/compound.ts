// Insert card (double-click a card): pick the equipment to compound it into.
// Only unequipped equipment of the card's kind with a free slot is offered,
// as the client does; the server checks again.
import { audio } from "../audio";
import type { Connection, InvItem } from "../net";
import type { PlayerState } from "../state";
import { itemCell } from "./common";
import { GameWindow } from "./window";

export class CompoundWindow {
  readonly window = new GameWindow("compound", "Insert Card", { x: 330, y: 160, width: 280 });
  private card: InvItem | null = null;

  constructor(
    private readonly state: PlayerState,
    private readonly conn: () => Connection | null,
  ) {
    // The card or the equipment may change while it's open (or be used up).
    state.subscribe(() => this.window.isOpen && this.render());
  }

  /** Opens the picker for a card in the inventory. */
  show(card: InvItem): void {
    this.card = card;
    this.render();
    this.window.open();
  }

  private render(): void {
    const items = this.state.items;
    const card = this.card && this.state.inventory.find((it) => it.index === this.card!.index && it.itemId === this.card!.itemId);
    const cardInfo = card && items?.get(card.itemId);
    const body = this.window.body;
    if (!card || !cardInfo || !items) {
      this.window.close(); // used up
      return;
    }
    const intro = document.createElement("p");
    intro.className = "muted";
    intro.textContent = `Put ${cardInfo.name} into which item? It stays there for good.`;

    const fits = this.state.inventory.filter((it) => {
      const info = items.get(it.itemId);
      return (
        !!info && it.equip === 0 && info.type !== "ammo" && (info.slot & cardInfo.cardLoc) !== 0 && it.cards.length < info.slots
      );
    });
    const list = document.createElement("ul");
    list.className = "compound-list";
    for (const it of fits) {
      const info = items.get(it.itemId)!;
      const name = items.label(it.itemId, it.cards, it.element);
      const li = document.createElement("li");
      const pick = document.createElement("button");
      pick.type = "button";
      pick.className = "secondary compound-pick";
      pick.textContent = name;
      pick.addEventListener("click", () => {
        this.conn()?.compound(card.index, it.index);
        audio.play("se_equip_item_01.wav");
        this.window.close();
      });
      li.append(itemCell(info, 1, { name, cards: { slots: info.slots, names: it.cards.map((c) => items.get(c)?.name ?? `Card ${c}`) } }), pick);
      list.append(li);
    }
    if (fits.length === 0) {
      const none = document.createElement("p");
      none.textContent = "Nothing it fits: it needs unequipped equipment of its kind with a free slot.";
      body.replaceChildren(intro, none);
      return;
    }
    body.replaceChildren(intro, list);
  }
}
