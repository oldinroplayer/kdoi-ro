// Minimap: RO's own map image (texture/유저인터페이스/map/<map>.bmp) with
// markers for you, other players, NPCs and warp portals. North is up.
import { fetchData, loadKeyedCanvas } from "../assets";
import type { MinimapView } from "../game";

const SIZE = 150; // CSS pixels
const ZOOMS = [1, 2, 4];
const FPS = 10;

const colors = {
  player: "#5aa9ff",
  party: "#ff7ad9", // RO's pink party markers
  npc: "#ffa94d",
  portal: "#ff5a5a",
  self: "#ffffff",
};

export class Minimap {
  private readonly canvas: HTMLCanvasElement;
  private readonly ctx: CanvasRenderingContext2D;
  private readonly title: HTMLElement;
  private readonly coords: HTMLElement;
  private readonly zoomIn: HTMLButtonElement;
  private readonly zoomOut: HTMLButtonElement;
  private readonly collapse: HTMLButtonElement;
  private zoomIndex = 0;
  private collapsed = false;
  private map = "";
  private image: HTMLCanvasElement | null = null;
  private names: Promise<Map<string, string>> | null = null;
  private timer = 0;

  constructor(
    private readonly root: HTMLElement,
    private readonly view: () => MinimapView | null,
  ) {
    root.className = "minimap";
    root.hidden = true;

    const head = document.createElement("div");
    head.className = "minimap-head";
    this.title = document.createElement("span");
    this.title.className = "minimap-title";
    this.zoomOut = button("−", "Zoom out minimap", () => this.setZoom(this.zoomIndex - 1));
    this.zoomIn = button("+", "Zoom in minimap", () => this.setZoom(this.zoomIndex + 1));
    this.collapse = button("▾", "Hide minimap", () => this.setCollapsed(!this.collapsed));
    head.append(this.title, this.zoomOut, this.zoomIn, this.collapse);

    this.canvas = document.createElement("canvas");
    this.canvas.setAttribute("role", "img");
    const dpr = Math.min(window.devicePixelRatio || 1, 2);
    this.canvas.width = this.canvas.height = SIZE * dpr;
    this.canvas.style.width = this.canvas.style.height = `${SIZE}px`;
    this.ctx = this.canvas.getContext("2d")!;
    this.ctx.scale(dpr, dpr);

    this.coords = document.createElement("div");
    this.coords.className = "minimap-coords";

    root.append(head, this.canvas, this.coords);
    this.restore();
    this.updateButtons();
  }

  start(): void {
    this.stop();
    this.root.hidden = false;
    this.timer = window.setInterval(() => this.draw(), 1000 / FPS);
  }

  stop(): void {
    window.clearInterval(this.timer);
    this.root.hidden = true;
    this.map = "";
    this.image = null;
  }

  private setZoom(i: number): void {
    this.zoomIndex = Math.max(0, Math.min(ZOOMS.length - 1, i));
    this.save();
    this.updateButtons();
    this.draw();
  }

  private setCollapsed(c: boolean): void {
    this.collapsed = c;
    this.root.classList.toggle("collapsed", c);
    this.collapse.textContent = c ? "▸" : "▾";
    this.collapse.setAttribute("aria-label", c ? "Show minimap" : "Hide minimap");
    this.save();
  }

  private updateButtons(): void {
    this.zoomOut.disabled = this.zoomIndex === 0;
    this.zoomIn.disabled = this.zoomIndex === ZOOMS.length - 1;
  }

  private async loadMap(map: string): Promise<void> {
    this.map = map;
    this.image = null;
    this.title.textContent = map;
    this.names ??= loadMapNames();
    const [image, names] = await Promise.all([
      loadKeyedCanvas(`texture/유저인터페이스/map/${map}.bmp`).catch(() => null),
      this.names,
    ]);
    if (this.map !== map) return; // changed again while loading
    this.image = image;
    const name = names.get(map) ?? map;
    this.title.textContent = name;
    this.canvas.setAttribute("aria-label", `Minimap of ${name}`);
  }

  private draw(): void {
    const v = this.view();
    if (!v) return;
    if (v.map !== this.map) void this.loadMap(v.map);
    if (v.self) this.coords.textContent = `${Math.floor(v.self.x)}, ${Math.floor(v.self.y)}`;
    if (this.collapsed) return;

    const ctx = this.ctx;
    ctx.clearRect(0, 0, SIZE, SIZE);
    ctx.fillStyle = "#14110d";
    ctx.fillRect(0, 0, SIZE, SIZE);

    // Visible window in map cells: the whole map at 1x, centred on us when zoomed.
    const zoom = ZOOMS[this.zoomIndex];
    const spanX = v.width / zoom, spanY = v.height / zoom;
    const cx = v.self?.x ?? v.width / 2, cy = v.self?.y ?? v.height / 2;
    const x0 = clamp(cx - spanX / 2, 0, v.width - spanX);
    const y0 = clamp(cy - spanY / 2, 0, v.height - spanY);
    // Cell -> minimap pixel (north up, so y flips).
    const toPx = (x: number, y: number): [number, number] => [
      ((x - x0) / spanX) * SIZE,
      SIZE - ((y - y0) / spanY) * SIZE,
    ];

    if (this.image) {
      const iw = this.image.width, ih = this.image.height;
      ctx.imageSmoothingEnabled = zoom < 4;
      ctx.drawImage(
        this.image,
        (x0 / v.width) * iw,
        (1 - (y0 + spanY) / v.height) * ih,
        (spanX / v.width) * iw,
        (spanY / v.height) * ih,
        0,
        0,
        SIZE,
        SIZE,
      );
    }

    const pulse = 0.5 + 0.5 * Math.sin(performance.now() / 250);
    for (const [x, y] of v.portals) dot(ctx, toPx(x, y), 3 + pulse, colors.portal);
    for (const [x, y] of v.npcs) dot(ctx, toPx(x, y), 2.5, colors.npc);
    for (const [x, y] of v.players) dot(ctx, toPx(x, y), 2.5, colors.player);
    for (const [x, y] of v.party) dot(ctx, toPx(x, y), 3, colors.party);
    if (v.self) arrow(ctx, toPx(v.self.x, v.self.y), v.self.dir);
  }

  private restore(): void {
    try {
      const saved = JSON.parse(localStorage.getItem("kdoiro.minimap") ?? "{}");
      if (typeof saved.zoom === "number") this.zoomIndex = clamp(saved.zoom, 0, ZOOMS.length - 1);
      if (saved.collapsed) this.setCollapsed(true);
    } catch {
      // Defaults.
    }
  }

  private save(): void {
    try {
      localStorage.setItem("kdoiro.minimap", JSON.stringify({ zoom: this.zoomIndex, collapsed: this.collapsed }));
    } catch {
      // Not remembered this time.
    }
  }
}

function dot(ctx: CanvasRenderingContext2D, [x, y]: [number, number], r: number, color: string) {
  if (x < -r || y < -r || x > SIZE + r || y > SIZE + r) return;
  ctx.beginPath();
  ctx.arc(x, y, r, 0, Math.PI * 2);
  ctx.fillStyle = color;
  ctx.fill();
  ctx.lineWidth = 1;
  ctx.strokeStyle = "rgba(0,0,0,0.8)";
  ctx.stroke();
}

/** Our position, pointing the way we face (server dir: 0 = north, counter-clockwise). */
function arrow(ctx: CanvasRenderingContext2D, [x, y]: [number, number], dir: number) {
  ctx.save();
  ctx.translate(x, y);
  ctx.rotate((-dir * Math.PI) / 4);
  ctx.beginPath();
  ctx.moveTo(0, -6);
  ctx.lineTo(4.5, 5);
  ctx.lineTo(0, 2.5);
  ctx.lineTo(-4.5, 5);
  ctx.closePath();
  ctx.fillStyle = colors.self;
  ctx.fill();
  ctx.lineWidth = 1.5;
  ctx.strokeStyle = "#000";
  ctx.stroke();
  ctx.restore();
}

function button(text: string, label: string, onClick: () => void): HTMLButtonElement {
  const b = document.createElement("button");
  b.type = "button";
  b.className = "minimap-button";
  b.textContent = text;
  b.setAttribute("aria-label", label);
  b.title = label;
  b.addEventListener("click", onClick);
  return b;
}

const clamp = (v: number, lo: number, hi: number) => Math.min(Math.max(v, lo), Math.max(lo, hi));

/** mapnametable.txt: "morocc.rsw#Morroc Town#" -> morocc -> "Morroc Town". */
async function loadMapNames(): Promise<Map<string, string>> {
  const out = new Map<string, string>();
  try {
    const text = new TextDecoder("euc-kr").decode(await fetchData("mapnametable.txt"));
    for (const line of text.split(/\r?\n/)) {
      const m = /^([^#]+)\.rsw#([^#]+)#/i.exec(line.trim());
      if (m) out.set(m[1].toLowerCase(), m[2]);
    }
  } catch {
    // Fall back to file names.
  }
  return out;
}
