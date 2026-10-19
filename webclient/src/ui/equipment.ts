// Equipment window (Alt+Q): worn items around a preview of the character.
import { audio } from "../audio";
import type { Connection } from "../net";
import { CharacterSprite, loadEquipLayers, loadPlayerAssets, PlayerAction } from "../render/character";
import type { PlayerState } from "../state";
import { itemCell } from "./common";
import { GameWindow } from "./window";

// RO equip location bitmask (server: internal/game/items.go).
const slots = {
  headTop: 0x0100,
  headMid: 0x0200,
  headLow: 0x0001,
  armor: 0x0010,
  weapon: 0x0002,
  shield: 0x0020,
  garment: 0x0004,
  shoes: 0x0040,
  accL: 0x0008,
  accR: 0x0080,
  ammo: 0x8000,
};

const left: [number, string][] = [
  [slots.headTop, "Upper Headgear"],
  [slots.headLow, "Lower Headgear"],
  [slots.weapon, "Weapon"],
  [slots.garment, "Garment"],
  [slots.accL, "Accessory"],
];
const right: [number, string][] = [
  [slots.headMid, "Middle Headgear"],
  [slots.armor, "Armor"],
  [slots.shield, "Shield"],
  [slots.shoes, "Shoes"],
  [slots.accR, "Accessory"],
];

export class EquipmentWindow {
  readonly window = new GameWindow("equipment", "Equipment", { x: 330, y: 120, width: 420 });
  private readonly leftCol = document.createElement("div");
  private readonly rightCol = document.createElement("div");
  /** The arrow slot, under both columns (as in RO's window). */
  private readonly ammoRow = document.createElement("div");
  private readonly preview = document.createElement("canvas");
  private dir = 0;

  constructor(
    private readonly state: PlayerState,
    private readonly conn: () => Connection | null,
  ) {
    const layout = document.createElement("div");
    layout.className = "equip-layout";
    this.leftCol.className = "equip-col";
    this.rightCol.className = "equip-col right";

    const center = document.createElement("div");
    center.className = "equip-preview";
    this.preview.width = 120;
    this.preview.height = 230;
    this.preview.setAttribute("aria-label", "Character preview");
    const turn = document.createElement("button");
    turn.type = "button";
    turn.className = "small-button";
    turn.textContent = "Turn ↻";
    turn.addEventListener("click", () => {
      this.dir = (this.dir + 1) % 8;
      void this.drawPreview();
    });
    center.append(this.preview, turn);

    layout.append(this.leftCol, center, this.rightCol);
    this.ammoRow.className = "equip-ammo";
    const hint = document.createElement("p");
    hint.className = "window-hint";
    hint.textContent = "Double-click an item in your inventory to wear it; double-click here to take it off.";
    this.window.body.append(layout, this.ammoRow, hint);

    state.subscribe(() => this.window.isOpen && this.render());
    const open = this.window.open.bind(this.window);
    this.window.open = () => {
      open();
      this.render();
    };
  }

  private render(): void {
    this.leftCol.replaceChildren(...left.map(([mask, label]) => this.slot(mask, label)));
    this.rightCol.replaceChildren(...right.map(([mask, label]) => this.slot(mask, label)));
    this.ammoRow.replaceChildren(this.slot(slots.ammo, "Arrow"));
    void this.drawPreview();
  }

  private slot(mask: number, label: string): HTMLElement {
    const row = document.createElement("div");
    row.className = "equip-slot";
    const worn = this.state.equippedIn(mask);
    const info = worn && this.state.items?.get(worn.itemId);
    const text = document.createElement("span");
    text.className = "equip-name";
    if (worn && info) {
      const fullName = this.state.items?.label(info.id, worn.cards, worn.element) ?? info.name;
      row.append(
        itemCell(info, worn.amount, {
          name: fullName,
          cards: info.slots ? { slots: info.slots, names: worn.cards.map((c) => this.state.items?.get(c)?.name ?? `Card ${c}`) } : undefined,
          label: `${label}: ${info.name}`,
          action: "take off",
          onActivate: () => {
            this.conn()?.unequip(worn.index);
            audio.play("se_equip_item_01.wav");
          },
        }),
      );
      // Arrows show how many are left.
      text.textContent = mask === slots.ammo ? `${info.name} × ${worn.amount}` : fullName;
    } else {
      const empty = document.createElement("div");
      empty.className = "item-cell empty";
      row.append(empty);
      text.textContent = label;
      text.classList.add("muted");
    }
    row.append(text);
    return row;
  }

  private sprite: CharacterSprite | null = null;
  private spriteJob = 0;
  private spriteFemale = false;

  private async drawPreview(): Promise<void> {
    const job = this.state.status?.job ?? 0;
    const female = this.state.female;
    if (!this.sprite || this.spriteJob !== job || this.spriteFemale !== female) {
      this.sprite?.dispose();
      this.sprite = new CharacterSprite(await loadPlayerAssets(job, female));
      this.spriteJob = job;
      this.spriteFemale = female;
    }
    // Show the battle stance so the weapon and shield are visible, like RO's own preview.
    const worn = (slot: number) => this.state.equippedIn(slot)?.itemId ?? 0;
    const look = {
      weapon: worn(slots.weapon),
      shield: worn(slots.shield),
      headTop: worn(slots.headTop),
      headMid: worn(slots.headMid),
      headLow: worn(slots.headLow),
      hair: this.state.hair,
      hairColor: this.state.hairColor,
      female,
    };
    this.sprite.setEquipment(await loadEquipLayers(look, (id) => this.state.items?.get(id)?.sprite || undefined, job));
    this.sprite.draw(PlayerAction.Ready, this.dir, 0);
    const ctx = this.preview.getContext("2d")!;
    ctx.imageSmoothingEnabled = false;
    ctx.clearRect(0, 0, this.preview.width, this.preview.height);
    // The sprite canvas is 256x256 with the feet at (128, 176); crop 60x115 around
    // the body and show it at 2x.
    ctx.drawImage(this.sprite.canvas, 98, 80, 60, 115, 0, 0, 120, 230);
  }
}
