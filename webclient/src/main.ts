import { audio } from "./audio";
import { Game } from "./game";
import { ItemDb } from "./items";
import { Connection, LoginCode, loginMessages, type ServerPacket } from "./net";
import { PlayerState } from "./state";
import { formatZeny, hideTooltip, toast } from "./ui/common";
import { EquipmentWindow } from "./ui/equipment";
import { InventoryWindow } from "./ui/inventory";
import { Minimap } from "./ui/minimap";
import { ShopWindow } from "./ui/shop";
import { SoundWindow } from "./ui/sound";
import { StatusWindow } from "./ui/stats";
import { closeAllWindows, closeTopWindow } from "./ui/window";

const $ = <T extends HTMLElement>(id: string) => document.getElementById(id) as T;

const canvas = $<HTMLCanvasElement>("view");
const loginOverlay = $("login");
const form = $<HTMLFormElement>("login-form");
const nameInput = $<HTMLInputElement>("name");
const passwordInput = $<HTMLInputElement>("password");
const serverInput = $<HTMLInputElement>("server");
const errorText = $("login-error");
const button = $<HTMLButtonElement>("login-button");
const hud = $("hud");
const hudName = $("hud-name");
const hudLevel = $("hud-level");
const hudPos = $("hud-pos");
const hudZeny = $("hud-zeny");
const hpBar = $("hp-bar");
const spBar = $("sp-bar");
const baseExpBar = $("base-exp");
const jobExpBar = $("job-exp");
const deathDialog = $("death");
const respawnButton = $<HTMLButtonElement>("respawn-button");
const status = $("status");
const menuButton = $<HTMLButtonElement>("menu-button");
const menu = $("menu");
const logoutButton = $<HTMLButtonElement>("logout-button");

let conn: Connection | null = null;
let game: Game | null = null;

const state = new PlayerState();
const currentConn = () => conn;
const inventory = new InventoryWindow(state, currentConn);
const equipment = new EquipmentWindow(state, currentConn);
const statusWindow = new StatusWindow(state, currentConn);
const shop = new ShopWindow(state, currentConn);
const sound = new SoundWindow();
const minimap = new Minimap($("minimap"), () => game?.minimap() ?? null);

function setStatus(text: string) {
  status.textContent = text;
  status.hidden = !text;
}

// HUD: level, HP/SP bars and zeny follow the latest Status packet.
state.subscribe(() => {
  const s = state.status;
  if (!s) return;
  hudLevel.textContent = `Novice · Base Lv ${s.baseLevel} · Job Lv ${s.jobLevel}`;
  setBar(hpBar, s.hp, s.maxHp, "HP");
  setBar(spBar, s.sp, s.maxSp, "SP");
  setExpBar(baseExpBar, s.baseExp, s.baseNext, "Base");
  setExpBar(jobExpBar, s.jobExp, s.jobNext, "Job");
  hudZeny.textContent = formatZeny(s.zeny);
});

function setExpBar(bar: HTMLElement, value: number, next: number, label: string) {
  const pct = next ? Math.floor((1000 * value) / next) / 10 : 100;
  setBar(bar, value, next, label, next ? `${label} EXP ${pct.toFixed(1)}%` : `${label} EXP MAX`);
}

function setBar(bar: HTMLElement, value: number, max: number, label: string, text?: string) {
  const fill = bar.querySelector<HTMLElement>(".bar-fill")!;
  fill.style.width = `${max ? Math.min(100, (100 * value) / max) : 100}%`;
  bar.querySelector(".bar-text")!.textContent = text ?? `${label} ${value} / ${max}`;
  bar.setAttribute("aria-valuenow", String(value));
  bar.setAttribute("aria-valuemax", String(max));
}

/** Packets for our own character's UI; world packets go to the Game. */
function handleUiPacket(p: ServerPacket): boolean {
  switch (p.type) {
    case "inventory":
      state.inventory = p.items;
      state.changed();
      return true;
    case "status": {
      const first = state.status === null;
      state.status = p.status;
      state.changed();
      if (first && p.status.statusPoints > 0) {
        toast(`You have ${p.status.statusPoints} status points to spend. Open Status (Alt+A).`);
      }
      return true;
    }
    case "shopOpen":
      state.shop = { npcId: p.npcId, npcName: p.npcName, items: p.items };
      shop.show(state.shop);
      return true;
    case "shopResult":
      shop.result(p.code);
      return true;
    case "message":
      toast(p.text);
      return true;
    case "enterMap":
      shop.window.close(); // shops don't follow you to another map
      return false;
    default:
      return false;
  }
}

// --- Menu and shortcuts ------------------------------------------------------

let menuOpen = false;

function setMenuOpen(open: boolean) {
  menuOpen = open;
  menu.hidden = !open;
  menuButton.setAttribute("aria-expanded", String(open));
  if (open) menu.querySelector<HTMLButtonElement>("button")?.focus();
}

menuButton.addEventListener("click", () => setMenuOpen(!menuOpen));
logoutButton.addEventListener("click", logout);
for (const [id, win] of [
  ["menu-inventory", inventory.window],
  ["menu-equipment", equipment.window],
  ["menu-status", statusWindow.window],
  ["menu-sound", sound.window],
] as const) {
  $(id).addEventListener("click", () => {
    setMenuOpen(false);
    win.toggle();
  });
}

/** F1: drink the first healing item in the inventory. */
function quickHeal() {
  const db = state.items;
  const potion = db && state.inventory.find((it) => db.get(it.itemId)?.type === "healing");
  if (!potion) {
    toast("You have no healing items.");
    return;
  }
  conn?.useItem(potion.index);
}

function setDead(dead: boolean) {
  deathDialog.hidden = !dead;
  if (dead) respawnButton.focus();
}

respawnButton.addEventListener("click", () => {
  conn?.respawn();
  deathDialog.hidden = true;
});

// RO's shortcuts: Alt+E inventory, Alt+Q equipment, Alt+A status.
const shortcuts: Record<string, () => void> = {
  KeyE: () => inventory.window.toggle(),
  KeyQ: () => equipment.window.toggle(),
  KeyA: () => statusWindow.window.toggle(),
};

window.addEventListener("keydown", (ev) => {
  if (hud.hidden) return;
  if (ev.altKey && shortcuts[ev.code]) {
    ev.preventDefault();
    shortcuts[ev.code]();
    return;
  }
  if (ev.key === "F1" && !(ev.target instanceof Element && ev.target.closest("input"))) {
    ev.preventDefault();
    quickHeal();
    return;
  }
  if (ev.key === "Escape") {
    // Esc closes the menu, then the top window, and otherwise opens the menu.
    if (menuOpen) setMenuOpen(false);
    else if (!closeTopWindow()) setMenuOpen(true);
  }
});

// Close the menu when clicking anywhere else (including the game view).
window.addEventListener("mousedown", (ev) => {
  if (menuOpen && !(ev.target instanceof Element && ev.target.closest(".hud-menu"))) setMenuOpen(false);
});

// --- Login / logout -----------------------------------------------------------

/** Closing the socket makes the server save the character. */
function logout() {
  const c = conn;
  conn = null;
  if (c) {
    c.onClose = () => {};
    c.onPacket = () => {};
    c.close();
  }
  passwordInput.value = "";
  showLogin();
}

function showLogin(error = "") {
  game?.dispose();
  game = null;
  setMenuOpen(false);
  closeAllWindows();
  hideTooltip();
  state.reset();
  setDead(false);
  audio.stopMusic();
  minimap.stop();
  hud.hidden = true;
  setStatus("");
  loginOverlay.hidden = false;
  errorText.textContent = error;
  button.disabled = false;
  passwordInput.focus();
}

form.addEventListener("submit", async (ev) => {
  ev.preventDefault();
  audio.unlock(); // the login click is the user gesture browsers need for sound
  errorText.textContent = "";
  button.disabled = true;
  const name = nameInput.value.trim();
  const serverUrl = serverInput.value.trim();

  try {
    if (!conn?.connected) {
      conn = new Connection();
      await conn.connect(serverUrl);
    }
  } catch (err) {
    conn = null;
    showLogin((err as Error).message);
    return;
  }

  // Item names and stats load alongside the map; windows render once ready.
  if (!state.items) {
    ItemDb.load(serverUrl)
      .then((db) => {
        state.items = db;
        state.changed();
        game?.refreshLooks(); // equipment sprites need item data
      })
      .catch((err) => {
        console.error(err);
        toast("Couldn't load item data from the server.");
      });
  }

  const c = conn;
  c.onClose = (reason) => {
    if (conn === c) conn = null;
    showLogin(reason);
  };
  c.onPacket = (p) => {
    if (game) {
      if (!handleUiPacket(p)) game.handle(p);
      return;
    }
    if (p.type !== "loginResult") return;
    if (p.code === LoginCode.OK || p.code === LoginCode.Created) {
      loginOverlay.hidden = true;
      hud.hidden = false;
      hudName.textContent = name;
      state.name = name;
      setStatus(p.code === LoginCode.Created ? `Welcome, ${name}! Loading Morroc…` : "Loading…");
      game = new Game(canvas, c, {
        setStatus,
        setPosition: (map, x, y) => (hudPos.textContent = `${map} (${x}, ${y})`),
        setDead,
        itemSprite: (id) => state.items?.get(id)?.sprite || undefined,
      });
      game.selfName = name;
      minimap.start();
    } else {
      errorText.textContent = loginMessages[p.code] ?? `Login failed (${p.code}).`;
      button.disabled = false;
    }
  };
  c.login(name, passwordInput.value);
});

nameInput.focus();
