// Appearance window: pick a gender and one of the 19 hairstyles in 9 hair
// colours, with a preview of your character. Shown on first login (until you
// choose) and from the menu any time after.
import type { Connection } from "../net";
import {
  CharacterSprite,
  DEFAULT_HAIR,
  HAIR_COLORS,
  HAIR_STYLES,
  loadHead,
  loadPlayerAssets,
  noEquipment,
  PlayerAction,
} from "../render/character";
import type { PlayerState } from "../state";
import { GameWindow } from "./window";

/** Where the head sits in the 256x256 sprite canvas (feet at 128, 176). */
const HEAD_CROP = [101, 74, 54, 54] as const;
const BODY_CROP = [96, 74, 64, 122] as const;

export class HairstyleWindow {
  readonly window = new GameWindow("hairstyle", "Appearance", { x: 220, y: 90, width: 440 });
  private readonly genders = document.createElement("div");
  private female = false;
  private readonly intro = document.createElement("p");
  private readonly preview = document.createElement("canvas");
  private readonly styles = document.createElement("div");
  private readonly colors = document.createElement("div");
  private style = DEFAULT_HAIR;
  private color = 0;
  private dir = 0;
  /** One off-screen sprite renders every preview and thumbnail. */
  private sprite: CharacterSprite | null = null;
  private spriteKey = "";
  private renderToken = 0;

  constructor(
    private readonly state: PlayerState,
    private readonly conn: () => Connection | null,
  ) {
    this.intro.className = "hair-intro";
    this.preview.width = BODY_CROP[2] * 2;
    this.preview.height = BODY_CROP[3] * 2;
    this.preview.className = "hair-preview";

    const turn = document.createElement("button");
    turn.type = "button";
    turn.className = "secondary small";
    turn.textContent = "Turn ↻";
    turn.addEventListener("click", () => {
      this.dir = (this.dir + 1) % 8;
      void this.renderPreview();
    });
    const left = document.createElement("div");
    left.className = "hair-left";
    left.append(this.preview, turn);

    this.styles.className = "hair-grid";
    this.styles.setAttribute("role", "radiogroup");
    this.styles.setAttribute("aria-label", "Hairstyle");
    this.colors.className = "hair-colors";
    this.colors.setAttribute("role", "radiogroup");
    this.colors.setAttribute("aria-label", "Hair colour");
    const right = document.createElement("div");
    right.className = "hair-right";
    this.genders.className = "hair-genders";
    this.genders.setAttribute("role", "radiogroup");
    this.genders.setAttribute("aria-label", "Gender");
    right.append(label("Gender"), this.genders, label("Hairstyle"), this.styles, label("Colour"), this.colors);

    const main = document.createElement("div");
    main.className = "hair-main";
    main.append(left, right);

    const actions = document.createElement("div");
    actions.className = "room-actions";
    const cancel = document.createElement("button");
    cancel.type = "button";
    cancel.className = "secondary";
    cancel.textContent = "Cancel";
    cancel.addEventListener("click", () => this.window.close());
    const apply = document.createElement("button");
    apply.type = "button";
    apply.textContent = "Apply";
    apply.addEventListener("click", () => {
      this.conn()?.setAppearance(this.style, this.color, this.female);
      this.window.close();
    });
    actions.append(cancel, apply);

    this.window.body.append(this.intro, main, actions);
  }

  /** Opens the window; firstTime explains why it appeared by itself. */
  show(firstTime = false): void {
    this.intro.textContent = firstTime
      ? "Welcome! Choose your character's gender, hairstyle and hair colour. You can change them later from the menu."
      : "Choose a gender, hairstyle and hair colour.";
    this.style = this.state.hair || DEFAULT_HAIR;
    this.color = this.state.hairColor;
    this.female = this.state.female;
    this.dir = 0;
    this.window.open();
    void this.renderAll();
  }

  private async spriteFor(job: number, female: boolean): Promise<CharacterSprite> {
    const key = `${job}:${female}`;
    if (!this.sprite || this.spriteKey !== key) {
      const assets = await loadPlayerAssets(job, female);
      this.sprite?.dispose();
      this.sprite = new CharacterSprite(assets);
      this.spriteKey = key;
    }
    return this.sprite;
  }

  /** Draws the character with a head into target, cropped. */
  private async drawInto(
    target: HTMLCanvasElement,
    style: number,
    color: number,
    dir: number,
    crop: readonly [number, number, number, number],
  ): Promise<void> {
    const female = this.female;
    const [sprite, head] = await Promise.all([
      this.spriteFor(this.state.status?.job ?? 0, female),
      loadHead(style, color, female),
    ]);
    sprite.setEquipment({ ...noEquipment, head });
    sprite.draw(PlayerAction.Idle, dir, 0);
    const ctx = target.getContext("2d")!;
    ctx.imageSmoothingEnabled = false;
    ctx.clearRect(0, 0, target.width, target.height);
    ctx.drawImage(sprite.canvas, crop[0], crop[1], crop[2], crop[3], 0, 0, target.width, target.height);
  }

  private async renderAll(): Promise<void> {
    const token = ++this.renderToken;
    this.genders.replaceChildren(
      ...[false, true].map((female) => {
        const b = document.createElement("button");
        b.type = "button";
        b.className = "secondary hair-gender";
        b.textContent = female ? "Female" : "Male";
        b.setAttribute("role", "radio");
        b.setAttribute("aria-checked", String(female === this.female));
        b.addEventListener("click", () => {
          this.female = female;
          void this.renderAll();
        });
        return b;
      }),
    );
    await this.renderPreview();
    // Style thumbnails in the chosen colour.
    const styleButtons: HTMLButtonElement[] = [];
    for (let s = 1; s <= HAIR_STYLES; s++) {
      const b = this.choice(`Hairstyle ${s}`, s === this.style, () => {
        this.style = s;
        void this.renderAll();
      });
      styleButtons.push(b);
    }
    this.styles.replaceChildren(...styleButtons);
    // Colour swatches: the chosen style in each colour.
    const colorButtons: HTMLButtonElement[] = [];
    for (let c = 0; c < HAIR_COLORS; c++) {
      colorButtons.push(
        this.choice(`Colour ${c + 1}`, c === this.color, () => {
          this.color = c;
          void this.renderAll();
        }),
      );
    }
    this.colors.replaceChildren(...colorButtons);
    // Thumbnails are drawn one at a time with the shared sprite; stop if the
    // choice changed meanwhile.
    for (let s = 1; s <= HAIR_STYLES; s++) {
      if (token !== this.renderToken) return;
      await this.drawInto(styleButtons[s - 1].querySelector("canvas")!, s, this.color, 0, HEAD_CROP);
    }
    for (let c = 0; c < HAIR_COLORS; c++) {
      if (token !== this.renderToken) return;
      await this.drawInto(colorButtons[c].querySelector("canvas")!, this.style, c, 0, HEAD_CROP);
    }
    if (token === this.renderToken) await this.renderPreview();
  }

  private renderPreview(): Promise<void> {
    return this.drawInto(this.preview, this.style, this.color, this.dir, BODY_CROP);
  }

  private choice(name: string, selected: boolean, onPick: () => void): HTMLButtonElement {
    const b = document.createElement("button");
    b.type = "button";
    b.className = "hair-choice";
    b.title = name;
    b.setAttribute("role", "radio");
    b.setAttribute("aria-label", name);
    b.setAttribute("aria-checked", String(selected));
    const c = document.createElement("canvas");
    c.width = c.height = HEAD_CROP[2];
    b.append(c);
    b.addEventListener("click", onPick);
    return b;
  }
}

function label(text: string): HTMLElement {
  const h = document.createElement("h3");
  h.className = "hair-label";
  h.textContent = text;
  return h;
}
