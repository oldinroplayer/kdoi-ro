// Draggable in-game windows (inventory, equipment, ...). Positions are remembered
// per browser for convenience; everything works without storage.

const open: GameWindow[] = [];
let topZ = 10;

export class GameWindow {
  readonly el: HTMLElement;
  readonly body: HTMLElement;
  onClose: () => void = () => {};

  constructor(
    private readonly id: string,
    title: string,
    private readonly defaults: { x: number; y: number; width: number },
  ) {
    this.el = document.createElement("section");
    this.el.className = "game-window";
    this.el.hidden = true;
    this.el.style.width = `${defaults.width}px`;
    this.el.setAttribute("role", "dialog");
    this.el.setAttribute("aria-label", title);

    const bar = document.createElement("header");
    bar.className = "game-window-bar";
    const h = document.createElement("h2");
    h.textContent = title;
    const close = document.createElement("button");
    close.type = "button";
    close.className = "game-window-close";
    close.setAttribute("aria-label", `Close ${title}`);
    close.textContent = "×";
    close.addEventListener("click", () => this.close());
    bar.append(h, close);

    this.body = document.createElement("div");
    this.body.className = "game-window-body";
    this.el.append(bar, this.body);
    document.getElementById("windows")!.append(this.el);

    this.el.addEventListener("mousedown", () => this.raise());
    this.enableDrag(bar);
  }

  get isOpen(): boolean {
    return !this.el.hidden;
  }

  open(): void {
    if (this.isOpen) {
      this.raise();
      return;
    }
    this.el.hidden = false;
    this.restorePosition();
    this.raise();
    open.push(this);
  }

  close(): void {
    if (!this.isOpen) return;
    this.el.hidden = true;
    open.splice(open.indexOf(this), 1);
    this.onClose();
  }

  toggle(): void {
    if (this.isOpen) this.close();
    else this.open();
  }

  private raise(): void {
    this.el.style.zIndex = String(++topZ);
    const i = open.indexOf(this);
    if (i >= 0) open.push(...open.splice(i, 1));
  }

  private enableDrag(handle: HTMLElement): void {
    handle.addEventListener("mousedown", (ev) => {
      if (ev.button !== 0 || (ev.target as Element).closest("button")) return;
      ev.preventDefault();
      const startX = ev.clientX - this.el.offsetLeft;
      const startY = ev.clientY - this.el.offsetTop;
      const move = (e: MouseEvent) => this.moveTo(e.clientX - startX, e.clientY - startY);
      const up = () => {
        window.removeEventListener("mousemove", move);
        window.removeEventListener("mouseup", up);
        this.savePosition();
      };
      window.addEventListener("mousemove", move);
      window.addEventListener("mouseup", up);
    });
  }

  private moveTo(x: number, y: number): void {
    // Keep the title bar reachable.
    const maxX = window.innerWidth - 60;
    const maxY = window.innerHeight - 32;
    this.el.style.left = `${Math.round(Math.min(Math.max(x, 60 - this.el.offsetWidth), maxX))}px`;
    this.el.style.top = `${Math.round(Math.min(Math.max(y, 0), maxY))}px`;
  }

  private restorePosition(): void {
    let pos = { x: this.defaults.x, y: this.defaults.y };
    try {
      const saved = localStorage.getItem(`kdoiro.window.${this.id}`);
      if (saved) pos = JSON.parse(saved);
    } catch {
      // Storage unavailable; use defaults.
    }
    // Narrow screens: start near the left edge.
    const x = Math.min(pos.x, Math.max(8, window.innerWidth - this.el.offsetWidth - 8));
    this.moveTo(x, pos.y);
  }

  private savePosition(): void {
    try {
      localStorage.setItem(
        `kdoiro.window.${this.id}`,
        JSON.stringify({ x: this.el.offsetLeft, y: this.el.offsetTop }),
      );
    } catch {
      // Ignore: position just won't be remembered.
    }
  }
}

/** Closes the most recently focused window; returns false if none was open. */
export function closeTopWindow(): boolean {
  const top = open[open.length - 1];
  if (!top) return false;
  top.close();
  return true;
}

export function closeAllWindows(): void {
  for (const w of [...open]) w.close();
}
