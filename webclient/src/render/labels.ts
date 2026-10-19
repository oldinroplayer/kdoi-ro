// Character name labels drawn as HTML over the canvas, like the original client's
// screen-space names: crisp at any zoom and never clipped by the 3D scene.
import * as THREE from "three";

/** Extra gap in CSS pixels between the anchor point and the top of the name. */
const OFFSET_Y = 2;

export class NameLabels {
  private readonly root = document.createElement("div");
  private readonly ndc = new THREE.Vector3();

  constructor(private readonly canvas: HTMLCanvasElement) {
    this.root.className = "name-labels";
    // Directly after the canvas, so the HUD (later in the DOM) stays on top.
    canvas.after(this.root);
  }

  /** A cast-time bar, positioned with place() like a name. */
  createBar(): HTMLElement {
    const el = document.createElement("div");
    el.className = "cast-bar";
    el.append(document.createElement("div"));
    this.root.append(el);
    return el;
  }

  /** A speech bubble ("Name : text") shown over a unit's head. */
  createSpeech(text: string): HTMLElement {
    const el = document.createElement("div");
    el.className = "speech-bubble";
    el.textContent = text;
    this.root.append(el);
    return el;
  }

  /** A chat room's sign over its owner's head; clicking it joins. */
  createSign(onClick: () => void): HTMLElement {
    const el = document.createElement("button");
    el.type = "button";
    el.className = "room-sign";
    el.addEventListener("click", onClick);
    this.root.append(el);
    return el;
  }

  create(name: string, kind: "self" | "player" | "npc" | "monster" | "item"): HTMLElement {
    const el = document.createElement("div");
    el.className = `name-label ${kind}`;
    el.textContent = name;
    this.root.append(el);
    return el;
  }

  /** Positions a label under a world-space point (the character's feet). */
  place(el: HTMLElement, feet: THREE.Vector3, camera: THREE.Camera): void {
    this.ndc.copy(feet).project(camera);
    const visible = this.ndc.z > -1 && this.ndc.z < 1;
    el.hidden = !visible;
    if (!visible) return;
    const x = ((this.ndc.x + 1) / 2) * this.canvas.clientWidth;
    const y = ((1 - this.ndc.y) / 2) * this.canvas.clientHeight + OFFSET_Y;
    el.style.transform = `translate(${Math.round(x)}px, ${Math.round(y)}px) translateX(-50%)`;
  }

  /** Positions an element so its bottom edge sits on a world-space point. */
  placeAbove(el: HTMLElement, point: THREE.Vector3, camera: THREE.Camera): void {
    this.ndc.copy(point).project(camera);
    const visible = this.ndc.z > -1 && this.ndc.z < 1;
    el.hidden = !visible;
    if (!visible) return;
    const x = ((this.ndc.x + 1) / 2) * this.canvas.clientWidth;
    const y = ((1 - this.ndc.y) / 2) * this.canvas.clientHeight;
    el.style.transform = `translate(${Math.round(x)}px, ${Math.round(y)}px) translate(-50%, -100%)`;
  }

  dispose(): void {
    this.root.remove();
  }
}

interface Floater {
  el: HTMLElement;
  anchor: () => THREE.Vector3;
  born: number;
}

/** How long floating text (damage numbers, LEVEL UP) stays up, in ms. */
const FLOAT_LIFE = 1200;

/**
 * Text that pops up over a unit and drifts upward: damage numbers, misses,
 * heals and level ups. The anchor is re-read every frame so it follows the unit.
 */
export class FloatingTexts {
  private readonly root = document.createElement("div");
  private readonly items: Floater[] = [];
  private readonly ndc = new THREE.Vector3();

  constructor(private readonly canvas: HTMLCanvasElement) {
    this.root.className = "floating-texts";
    canvas.after(this.root);
  }

  /** The image critical hits' numbers sit on (a data URL), once loaded. */
  setCritBurst(url: string): void {
    this.root.style.setProperty("--crit-burst", `url("${url}")`);
  }

  /** kind: damage, crit, hurt (damage to us), miss, heal, level, "level job". */
  spawn(text: string, kind: string, anchor: () => THREE.Vector3): void {
    const el = document.createElement("div");
    el.className = `floater ${kind}`;
    const span = document.createElement("span");
    span.textContent = text;
    el.append(span);
    this.root.append(el);
    this.items.push({ el, anchor, born: performance.now() });
  }

  update(camera: THREE.Camera, now: number): void {
    for (let i = this.items.length - 1; i >= 0; i--) {
      const f = this.items[i];
      if (now - f.born > FLOAT_LIFE) {
        f.el.remove();
        this.items.splice(i, 1);
        continue;
      }
      this.ndc.copy(f.anchor()).project(camera);
      const x = ((this.ndc.x + 1) / 2) * this.canvas.clientWidth;
      const y = ((1 - this.ndc.y) / 2) * this.canvas.clientHeight;
      f.el.style.transform = `translate(${Math.round(x)}px, ${Math.round(y)}px) translate(-50%, -100%)`;
    }
  }

  dispose(): void {
    this.root.remove();
  }
}
