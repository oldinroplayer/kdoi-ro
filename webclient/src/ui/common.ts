// Shared UI bits: toasts, the item tooltip, item cells, and the item info window.
import { itemIcon, itemIllustration, renderDescription, type ItemInfo } from "../items";
import { GameWindow } from "./window";

export const formatZeny = (z: number) => `${z.toLocaleString("en-US")} z`;

export function toast(text: string): void {
  const box = document.getElementById("toasts")!;
  const el = document.createElement("div");
  el.className = "toast";
  el.setAttribute("role", "status");
  el.textContent = text;
  box.append(el);
  setTimeout(() => el.classList.add("leaving"), 2600);
  setTimeout(() => el.remove(), 3000);
}

const tooltip = () => document.getElementById("tooltip")!;

export function showTooltip(text: string, ev: MouseEvent): void {
  const t = tooltip();
  t.textContent = text;
  t.hidden = false;
  moveTooltip(ev);
}

export function moveTooltip(ev: MouseEvent): void {
  const t = tooltip();
  const x = Math.min(ev.clientX + 14, window.innerWidth - t.offsetWidth - 8);
  const y = Math.min(ev.clientY + 16, window.innerHeight - t.offsetHeight - 8);
  t.style.transform = `translate(${x}px, ${y}px)`;
}

export function hideTooltip(): void {
  tooltip().hidden = true;
}

/**
 * An item slot: icon, optional amount badge, hover tooltip, right-click for
 * details, and an optional double-click action.
 */
export function itemCell(
  item: ItemInfo,
  amount: number,
  opts: { onActivate?: () => void; label?: string; action?: string; dragData?: string } = {},
): HTMLElement {
  const cell = document.createElement("button");
  cell.type = "button";
  cell.className = "item-cell";
  const name = amount > 1 ? `${item.name} × ${amount}` : item.name;
  cell.setAttribute("aria-label", opts.label ?? name);

  const img = document.createElement("img");
  img.alt = "";
  img.width = img.height = 24;
  void itemIcon(item).then((src) => {
    if (src) img.src = src;
  });
  cell.append(img);
  if (amount > 1) {
    const badge = document.createElement("span");
    badge.className = "item-amount";
    badge.textContent = String(amount);
    cell.append(badge);
  }

  const hint = opts.onActivate
    ? `\nDouble-click to ${opts.action ?? "use"} · Right-click for info`
    : "\nRight-click for info";
  cell.addEventListener("mouseenter", (ev) => showTooltip(name + hint, ev));
  cell.addEventListener("mousemove", moveTooltip);
  cell.addEventListener("mouseleave", hideTooltip);
  cell.addEventListener("contextmenu", (ev) => {
    ev.preventDefault();
    hideTooltip();
    showItemInfo(item);
  });
  if (opts.dragData) makeDraggable(cell, opts.dragData);
  if (opts.onActivate) {
    const activate = opts.onActivate;
    cell.addEventListener("dblclick", () => {
      hideTooltip();
      activate();
    });
    // Keyboard: Enter/Space act like a double-click.
    cell.addEventListener("keydown", (ev) => {
      if (ev.key === "Enter" || ev.key === " ") {
        ev.preventDefault();
        activate();
      }
    });
  }
  return cell;
}

/** MIME type for things dragged onto the hotkey bar. */
export const DRAG_TYPE = "application/x-kdoiro-hotkey";

/** Lets an element be dragged onto the hotkey bar carrying `data`. */
export function makeDraggable(el: HTMLElement, data: string): void {
  el.draggable = true;
  el.addEventListener("dragstart", (ev) => {
    hideTooltip();
    ev.dataTransfer?.setData(DRAG_TYPE, data);
    if (ev.dataTransfer) ev.dataTransfer.effectAllowed = "copyMove";
  });
}

let infoWindow: GameWindow | null = null;

/** The info window: a picture, a title and an RO-formatted description. */
export function showInfo(title: string, art: Promise<string>, description: string, artClass = "item-info-art"): void {
  infoWindow ??= new GameWindow("item-info", "Info", { x: 420, y: 120, width: 300 });
  const body = infoWindow.body;
  body.replaceChildren();
  body.classList.add("item-info");

  const head = document.createElement("div");
  head.className = "item-info-head";
  const img = document.createElement("img");
  img.alt = "";
  img.className = artClass;
  void art.then((src) => {
    if (src) img.src = src;
    else img.remove();
  });
  const h = document.createElement("h3");
  h.textContent = title;
  head.append(img, h);

  const desc = document.createElement("div");
  desc.className = "item-info-desc";
  renderDescription(description || "No description.", desc);
  body.append(head, desc);
  infoWindow.open();
}

export function showItemInfo(item: ItemInfo): void {
  showInfo(item.name, itemIllustration(item), item.description);
}
