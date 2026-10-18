import { Game } from "./game";
import { Connection, LoginCode, loginMessages } from "./net";

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
const hudPos = $("hud-pos");
const status = $("status");

let conn: Connection | null = null;
let game: Game | null = null;

function setStatus(text: string) {
  status.textContent = text;
  status.hidden = !text;
}

function showLogin(error = "") {
  game?.dispose();
  game = null;
  hud.hidden = true;
  setStatus("");
  loginOverlay.hidden = false;
  errorText.textContent = error;
  button.disabled = false;
  passwordInput.focus();
}

form.addEventListener("submit", async (ev) => {
  ev.preventDefault();
  errorText.textContent = "";
  button.disabled = true;
  const name = nameInput.value.trim();

  try {
    if (!conn?.connected) {
      conn = new Connection();
      await conn.connect(serverInput.value.trim());
    }
  } catch (err) {
    conn = null;
    showLogin((err as Error).message);
    return;
  }

  const c = conn;
  c.onClose = (reason) => {
    if (conn === c) conn = null;
    showLogin(reason);
  };
  c.onPacket = (p) => {
    if (game) {
      game.handle(p);
      return;
    }
    if (p.type !== "loginResult") return;
    if (p.code === LoginCode.OK || p.code === LoginCode.Created) {
      loginOverlay.hidden = true;
      hud.hidden = false;
      hudName.textContent = name;
      setStatus(p.code === LoginCode.Created ? `Welcome, ${name}! Loading Morroc…` : "Loading…");
      game = new Game(canvas, c, {
        setStatus,
        setPosition: (map, x, y) => (hudPos.textContent = `${map} (${x}, ${y})`),
      });
      game.selfName = name;
    } else {
      errorText.textContent = loginMessages[p.code] ?? `Login failed (${p.code}).`;
      button.disabled = false;
    }
  };
  c.login(name, passwordInput.value);
});

nameInput.focus();
