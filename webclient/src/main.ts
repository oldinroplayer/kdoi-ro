import { audio } from "./audio";
import { Game } from "./game";
import { jobName } from "./render/character";
import { ItemDb } from "./items";
import { SkillDb } from "./skills";
import { Connection, LoginCode, loginMessages, type ServerPacket } from "./net";
import { PlayerState } from "./state";
import { formatZeny, hideTooltip, toast } from "./ui/common";
import { EquipmentWindow } from "./ui/equipment";
import { InventoryWindow } from "./ui/inventory";
import { Hotbar } from "./ui/hotbar";
import { DialogWindow } from "./ui/dialog";
import { BuffBar } from "./ui/buffs";
import { AutoBot } from "./bot";
import { EMOTE_HOTKEYS } from "./emotes";
import { DropAmountWindow } from "./ui/dropamount";
import { HairstyleWindow } from "./ui/hairstyle";
import { PartyInviteWindow, PartyWindow } from "./ui/party";
import { DRAG_TYPE } from "./ui/common";
import { ChatBox } from "./ui/chatbox";
import { ChatRoomWindow, RoomPasswordWindow, RoomSettingsWindow, roomResultMessage } from "./ui/chatroom";
import { Minimap } from "./ui/minimap";
import { SkillWindow } from "./ui/skillwindow";
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
// A built client (release or dev target) is served by the game server itself:
// connect back to it. Only Vite's dev server (which has import.meta.hot) talks
// to a separately running server on :7000.
if (!import.meta.hot) {
  serverInput.value = `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/ws`;
}
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
const dialog = new DialogWindow(currentConn);
const skillWindow = new SkillWindow(state, currentConn, useSkill);
const hotbar = new Hotbar($("hotbar"), state, {
  useItem: (index) => conn?.useItem(index),
  useSkill,
  toggleSit: () => game?.toggleSit(),
});

/** Uses a skill from the skill window or hotkey bar. */
function useSkill(skillId: number) {
  const info = state.skillDb?.get(skillId);
  const level = state.skills.get(skillId) ?? 0;
  if (!info) return;
  if (!level) {
    toast(`You haven't learned ${info.displayName} yet.`);
    return;
  }
  if (info.target === "passive") {
    toast(`${info.displayName} is a passive skill.`);
    return;
  }
  if (info.target === "enemy" || info.target === "friend") {
    game?.beginTargeting(skillId, info.target, info.displayName); // then click the target
    return;
  }
  conn?.useSkill(skillId);
}
new BuffBar($("buffs"), state);

// Auto attack (Alt+B): attacks the nearest monster, wanders to find more.
const botBadge = $("bot-badge");
const bot = new AutoBot(
  () => game,
  currentConn,
  (running, reason) => {
    botBadge.hidden = !running;
    $("menu-bot").firstChild!.textContent = running ? "Stop Auto Attack " : "Auto Attack ";
    toast(reason ?? (running ? "Auto attack on: hunting the nearest monsters." : "Auto attack off."));
  },
);
const dropAmount = new DropAmountWindow(currentConn);
const hairstyle = new HairstyleWindow(state, currentConn);
const partyWindow = new PartyWindow(state, currentConn);
const partyInvite = new PartyInviteWindow(currentConn);
/** Whether we've already asked this session for a hairstyle. */
let askedForHair = false;

// Dragging an inventory item onto the map drops it on the ground.
canvas.addEventListener("dragover", (ev) => {
  if (game && ev.dataTransfer?.types.includes(DRAG_TYPE)) ev.preventDefault();
});
canvas.addEventListener("drop", (ev) => {
  const raw = ev.dataTransfer?.getData(DRAG_TYPE);
  if (!raw || !game) return;
  const data = JSON.parse(raw) as { kind: string; id: number; index?: number };
  // Hotkey slots dropped here aren't accepted, so the bar clears them.
  if (data.kind !== "item" || data.index === undefined) return;
  ev.preventDefault();
  const it = state.inventory.find((i) => i.index === data.index);
  if (!it) return;
  if (it.equip) {
    toast("Take it off before dropping it.");
    return;
  }
  dropAmount.drop(it.index, state.items?.get(it.itemId)?.name ?? "items", it.amount);
});
const roomSettings = new RoomSettingsWindow(currentConn);
const roomPassword = new RoomPasswordWindow(currentConn);
const chatRoom = new ChatRoomWindow(currentConn, () => state.name, (s) => roomSettings.show(s));
const chatBox = new ChatBox($("chat"), currentConn, { openRoomCreator: openRoomCreator });

/** Alt+C or /room: open a chat room (or edit ours). */
function openRoomCreator() {
  if (chatRoom.inRoom) toast("Leave your chat room first.");
  else roomSettings.show();
}

function joinRoom(roomId: number, isPublic: boolean, title: string) {
  if (chatRoom.inRoom) toast("You're already in a chat room.");
  else if (isPublic) conn?.joinRoom(roomId);
  else roomPassword.ask(roomId, title);
}
const minimap = new Minimap($("minimap"), () => game?.minimap() ?? null);

function setStatus(text: string) {
  status.textContent = text;
  status.hidden = !text;
}

// HUD: level, HP/SP bars and zeny follow the latest Status packet.
state.subscribe(() => {
  const s = state.status;
  if (!s) return;
  hudLevel.textContent = `${jobName(s.job)} · Base Lv ${s.baseLevel} · Job Lv ${s.jobLevel}`;
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
      state.shop = { npcId: p.npcId, npcName: p.npcName, items: p.items, sellRate: p.sellRate };
      shop.show(state.shop);
      return true;
    case "shopResult":
      shop.result(p.code);
      return true;
    case "message":
      toast(p.text);
      chatBox.add("system", p.text);
      return true;
    case "chat":
      if (p.kind === 1) {
        chatRoom.message(p.name, p.text);
      } else if (p.kind === 2) {
        chatBox.add("party", `(Party) ${p.name} : ${p.text}`);
      } else {
        chatBox.add(p.name === state.name ? "self" : "public", `${p.name} : ${p.text}`);
        game?.showSpeech(p.id, `${p.name} : ${p.text}`);
      }
      return true;
    case "partyInfo":
      state.party = p.party;
      state.changed();
      game?.setPartyMembers(p.party.members.map((m) => m.name));
      return true;
    case "partyLeft":
      state.party = null;
      state.changed();
      game?.setPartyMembers([]);
      return true;
    case "partyInvited":
      partyInvite.show(p.partyId, p.partyName, p.from);
      return true;
    case "whisperFrom":
      chatBox.whisperFrom(p.from, p.text);
      return true;
    case "whisperResult":
      chatBox.whisperResult(p.code, p.to);
      return true;
    case "roomEnter":
      roomSettings.window.close();
      chatRoom.enter({ id: p.roomId, title: p.title, limit: p.limit, public: p.public, owner: p.owner, members: p.members });
      return true;
    case "roomMember":
      chatRoom.member(p.change, p.name);
      return true;
    case "roomSettings":
      chatRoom.settings(p.title, p.limit, p.public, p.owner);
      return true;
    case "roomLeft":
      chatRoom.left(p.reason);
      return true;
    case "roomResult":
      toast(roomResultMessage(p.code));
      return true;
    case "npcDialog":
      dialog.show(p.npcId, p.name, p.text, p.options);
      return true;
    case "buff":
      // Only our own buffs are sent to us.
      if (p.ms > 0) state.buffs.set(p.skillId, { level: p.level, until: performance.now() + p.ms });
      else state.buffs.delete(p.skillId);
      state.changed();
      return true;
    case "skills":
      state.skills = p.levels;
      state.skillPoints = p.points;
      state.changed();
      return true;
    case "enterMap":
      shop.window.close(); // shops and NPC talks don't follow you to another map
      dialog.window.close();
      state.selfId = p.selfId;
      state.hair = p.look.hair;
      state.hairColor = p.look.hairColor;
      state.female = p.look.female;
      state.changed();
      // First login (no hairstyle chosen yet): ask once per session.
      if (p.look.hair === 0 && !askedForHair) {
        askedForHair = true;
        hairstyle.show(true);
      }
      return false;
    case "look":
      if (p.id === state.selfId) {
        state.hair = p.look.hair;
        state.hairColor = p.look.hairColor;
        state.female = p.look.female;
        state.changed();
      }
      return false; // the game redraws the character too
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
$("menu-sit").addEventListener("click", () => {
  setMenuOpen(false);
  game?.toggleSit();
});
$("menu-party").addEventListener("click", () => {
  setMenuOpen(false);
  partyWindow.window.toggle();
});
$("menu-hair").addEventListener("click", () => {
  setMenuOpen(false);
  hairstyle.show();
});
$("menu-bot").addEventListener("click", () => {
  setMenuOpen(false);
  bot.toggle();
});
$("menu-room").addEventListener("click", () => {
  setMenuOpen(false);
  openRoomCreator();
});
for (const [id, win] of [
  ["menu-inventory", inventory.window],
  ["menu-equipment", equipment.window],
  ["menu-status", statusWindow.window],
  ["menu-sound", sound.window],
  ["menu-skills", skillWindow.window],
] as const) {
  $(id).addEventListener("click", () => {
    setMenuOpen(false);
    win.toggle();
  });
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
  KeyS: () => skillWindow.window.toggle(),
  KeyC: openRoomCreator,
  KeyB: () => bot.toggle(),
  KeyZ: () => partyWindow.window.toggle(),
};

window.addEventListener("keydown", (ev) => {
  if (hud.hidden) return;
  // Alt+1..Alt+0: emoticons.
  const digit = /^Digit(\d)$/.exec(ev.code);
  if (ev.altKey && digit) {
    ev.preventDefault();
    conn?.emote(EMOTE_HOTKEYS[(Number(digit[1]) + 9) % 10]);
    return;
  }
  if (ev.altKey && shortcuts[ev.code]) {
    ev.preventDefault();
    shortcuts[ev.code]();
    return;
  }
  if (ev.key === "Insert" && !(ev.target instanceof Element && ev.target.closest("input"))) {
    ev.preventDefault();
    game?.toggleSit();
    return;
  }
  const typing = ev.target instanceof Element && !!ev.target.closest("input, textarea");
  if (!typing && hotbar.handleKey(ev)) return;
  if (ev.key === "Enter" && !typing) {
    ev.preventDefault();
    chatBox.focus(); // Enter starts typing, as in RO
    return;
  }
  if (ev.key === "Escape") {
    // Esc cancels choosing a skill target, closes the menu, then the top
    // window, and otherwise opens the menu.
    if (game?.cancelTargeting()) return;
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
  bot.stop();
  askedForHair = false;
  state.reset();
  chatRoom.reset();
  chatBox.clear();
  setDead(false);
  audio.stopMusic();
  minimap.stop();
  hotbar.hide();
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

  // Skill and item data load alongside the map; windows render once ready.
  if (!state.skillDb) {
    SkillDb.load(serverUrl)
      .then((db) => {
        state.skillDb = db;
        state.changed();
      })
      .catch((err) => {
        console.error(err);
        toast("Couldn't load skill data from the server.");
      });
  }
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
      setStatus(p.code === LoginCode.Created ? `Welcome, ${name}! Loading the Archer Village…` : "Loading…");
      game = new Game(canvas, c, {
        setStatus,
        setPosition: (map, x, y) => (hudPos.textContent = `${map} (${x}, ${y})`),
        setDead,
        itemSprite: (id) => state.items?.get(id)?.sprite || undefined,
        notice: toast,
        skillElement: (id) => state.skillDb?.get(id)?.element ?? 0,
        joinRoom,
        itemInfo: (id) => state.items?.get(id),
      });
      chatBox.add("system", "Welcome to KDoiRo! Click to walk, talk or attack · Enter to chat · /help for chat commands · Alt+C opens a chat room");
      game.selfName = name;
      minimap.start();
      hotbar.show(name);
    } else {
      errorText.textContent = loginMessages[p.code] ?? `Login failed (${p.code}).`;
      button.disabled = false;
    }
  };
  c.login(name, passwordInput.value);
});

nameInput.focus();
