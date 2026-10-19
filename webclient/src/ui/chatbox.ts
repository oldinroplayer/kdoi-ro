// The chat box (bottom left): a log of what's said nearby, whispers and
// system messages, and a line to type in. Like RO: Enter starts typing,
// "/w Name message" whispers, "/r message" replies to the last whisper.
import { EMOTE_COMMANDS, EMOTE_HELP } from "../emotes";
import type { Connection } from "../net";

export type ChatLineKind = "public" | "self" | "whisper-in" | "whisper-out" | "room" | "party" | "system" | "error";

const MAX_LINES = 150;

export interface ChatCommands {
  /** "/room": open the chat room creation window. */
  openRoomCreator(): void;
}

export class ChatBox {
  private readonly log: HTMLElement;
  private readonly input: HTMLInputElement;
  private lastWhisperFrom = "";

  constructor(
    root: HTMLElement,
    private readonly conn: () => Connection | null,
    private readonly commands: ChatCommands,
  ) {
    this.log = root.querySelector(".chat-log")!;
    this.input = root.querySelector("input")!;
    const form = root.querySelector("form")!;
    form.addEventListener("submit", (ev) => {
      ev.preventDefault();
      const text = this.input.value.trim();
      this.input.value = "";
      if (text) this.submit(text);
      else this.input.blur(); // Enter on an empty line returns to the game
    });
    this.input.addEventListener("keydown", (ev) => {
      if (ev.key === "Escape") {
        ev.stopPropagation();
        this.input.blur();
      }
    });
  }

  /** Starts typing (Enter in the game view). */
  focus(): void {
    this.input.focus();
  }

  get typing(): boolean {
    return document.activeElement === this.input;
  }

  clear(): void {
    this.log.replaceChildren();
    this.lastWhisperFrom = "";
  }

  add(kind: ChatLineKind, text: string): void {
    const atBottom = this.log.scrollHeight - this.log.scrollTop - this.log.clientHeight < 8;
    const line = document.createElement("div");
    line.className = `chat-line ${kind}`;
    line.textContent = text;
    this.log.append(line);
    while (this.log.childElementCount > MAX_LINES) this.log.firstElementChild!.remove();
    if (atBottom) this.log.scrollTop = this.log.scrollHeight;
  }

  /** Someone whispered to us. */
  whisperFrom(from: string, text: string): void {
    this.lastWhisperFrom = from;
    this.add("whisper-in", `(From ${from}) : ${text}`);
  }

  private pendingWhispers: { to: string; text: string }[] = [];

  /** The server's answer to a whisper we sent (in order). */
  whisperResult(code: number, to: string): void {
    // Results come back in order, but a whisper the server dropped (sent too
    // fast) never gets one, so match on the name.
    const i = this.pendingWhispers.findIndex((w) => w.to.toLowerCase() === to.toLowerCase());
    const sent = i >= 0 ? this.pendingWhispers.splice(i, 1)[0] : undefined;
    if (code === 0) this.add("whisper-out", `(To ${to}) : ${sent?.text ?? ""}`);
    else if (code === 1) this.add("error", `${to} isn't online.`);
    else this.add("error", "You can't whisper to yourself.");
  }

  private submit(text: string): void {
    const conn = this.conn();
    if (!conn) return;
    // "%message" is RO's party chat.
    if (text.startsWith("%")) {
      if (text.length > 1) conn.partyChat(text.slice(1).trim());
      return;
    }
    const emote = EMOTE_COMMANDS[text.toLowerCase()];
    if (emote !== undefined) {
      conn.emote(emote);
      return;
    }
    const command = /^\/(\w+)\s*(.*)$/.exec(text);
    if (!command) {
      conn.chat(text);
      return;
    }
    const [, name, rest] = command;
    switch (name.toLowerCase()) {
      case "w":
      case "whisper": {
        const m = /^(\S+)\s+(.+)$/.exec(rest);
        if (!m) {
          this.add("error", "Usage: /w Name message");
          return;
        }
        this.whisper(m[1], m[2]);
        return;
      }
      case "r":
      case "reply":
        if (!this.lastWhisperFrom) this.add("error", "Nobody has whispered to you yet.");
        else if (rest) this.whisper(this.lastWhisperFrom, rest);
        return;
      case "room":
      case "chat":
        this.commands.openRoomCreator();
        return;
      case "p":
      case "party":
        if (rest) conn.partyChat(rest);
        return;
      case "organize":
        if (rest) conn.createParty(rest);
        else this.add("error", "Usage: /organize Party Name");
        return;
      case "invite":
        if (rest) conn.inviteToParty(rest);
        else this.add("error", "Usage: /invite Name");
        return;
      case "leave":
        conn.leaveParty();
        return;
      case "help":
        this.add("system", "Enter to chat · /w Name message to whisper · /r message to reply · /room (Alt+C) to open a chat room");
        this.add("system", "Party (Alt+Z): /organize Name · /invite Name · /leave · %message or /p message to talk to your party");
        this.add("system", `Emoticons: ${EMOTE_HELP}`);
        return;
      default:
        this.add("error", `Unknown command /${name}. Type /help.`);
    }
  }

  private whisper(to: string, text: string): void {
    this.pendingWhispers.push({ to, text });
    if (this.pendingWhispers.length > 20) this.pendingWhispers.shift();
    this.conn()?.whisper(to, text);
  }
}
