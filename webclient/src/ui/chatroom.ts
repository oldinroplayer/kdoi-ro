// Chat rooms, RO style: a titled sign over the owner's head that others click
// to join. The room window lists members and holds the room's conversation;
// the owner can kick members, hand the room over and change its settings.
import type { Connection, RoomSettings } from "../net";
import { toast } from "./common";
import { GameWindow } from "./window";

interface Room {
  id: number;
  title: string;
  limit: number;
  public: boolean;
  owner: string;
  members: string[];
}

const RESULT_MESSAGES: Record<number, string> = {
  1: "That chat room is full.",
  2: "Wrong password.",
  3: "You were kicked from that room and can't come back.",
  4: "That chat room is gone or too far away.",
  5: "You're already in a chat room.",
  6: "You can't do that right now.",
  7: "Check the room settings: a title, 2–20 people, and a password for a private room.",
};

export class ChatRoomWindow {
  readonly window = new GameWindow("chatroom", "Chat Room", { x: 240, y: 120, width: 380 });
  private room: Room | null = null;
  private readonly heading = document.createElement("div");
  private readonly members = document.createElement("ul");
  private readonly log = document.createElement("div");
  private readonly input = document.createElement("input");
  private readonly settingsButton = document.createElement("button");

  constructor(
    private readonly conn: () => Connection | null,
    private readonly selfName: () => string,
    private readonly openSettings: (current: RoomSettings) => void,
  ) {
    this.heading.className = "room-heading";
    this.members.className = "room-members";
    this.log.className = "room-log";
    this.log.setAttribute("role", "log");

    const form = document.createElement("form");
    form.className = "room-form";
    this.input.maxLength = 120;
    this.input.placeholder = "Say something to the room";
    this.input.setAttribute("aria-label", "Message to the room");
    const send = button("Send", "submit");
    form.append(this.input, send);
    form.addEventListener("submit", (ev) => {
      ev.preventDefault();
      const text = this.input.value.trim();
      this.input.value = "";
      if (text) this.conn()?.chat(text);
    });
    this.input.addEventListener("keydown", (ev) => {
      if (ev.key === "Escape") {
        ev.stopPropagation();
        this.input.blur();
      }
    });

    const actions = document.createElement("div");
    actions.className = "room-actions";
    this.settingsButton.type = "button";
    this.settingsButton.className = "secondary";
    this.settingsButton.textContent = "Settings";
    this.settingsButton.addEventListener("click", () => {
      const r = this.room;
      if (r) this.openSettings({ title: r.title, limit: r.limit, public: r.public, password: "" });
    });
    const leave = button("Leave");
    leave.addEventListener("click", () => this.conn()?.leaveRoom());
    actions.append(this.settingsButton, leave);

    this.window.body.append(this.heading, this.members, this.log, form, actions);
    // Closing the window leaves the room, as in RO.
    this.window.onClose = () => {
      if (this.room) this.conn()?.leaveRoom();
    };
  }

  get inRoom(): boolean {
    return this.room !== null;
  }

  enter(r: Room): void {
    this.room = { ...r, members: [...r.members] };
    this.log.replaceChildren();
    this.line("system", `You entered "${r.title}".`);
    this.render();
    this.window.open();
    this.input.focus();
  }

  member(change: number, name: string): void {
    const r = this.room;
    if (!r) return;
    if (change === 1) {
      r.members.push(name);
      this.line("system", `${name} joined.`);
    } else {
      r.members = r.members.filter((m) => m !== name);
      this.line("system", change === 2 ? `${name} was kicked.` : `${name} left.`);
    }
    this.render();
  }

  settings(title: string, limit: number, isPublic: boolean, owner: string): void {
    const r = this.room;
    if (!r) return;
    if (owner !== r.owner) {
      this.line("system", `${owner} is now the owner.`);
      // Keep the owner first, as the server does.
      r.members = [owner, ...r.members.filter((m) => m !== owner)];
    }
    Object.assign(r, { title, limit, public: isPublic, owner });
    this.render();
  }

  message(name: string, text: string): void {
    this.line(name === this.selfName() ? "self" : "public", `${name} : ${text}`);
  }

  /** We left or were kicked: close without telling the server again. */
  left(reason: number): void {
    this.room = null;
    this.window.close();
    if (reason === 1) toast("You were kicked from the chat room.");
  }

  /** Clears the room (logging out). */
  reset(): void {
    this.room = null;
    this.window.close();
  }

  private render(): void {
    const r = this.room;
    if (!r) return;
    const isOwner = r.owner === this.selfName();
    this.heading.textContent = `${r.public ? "" : "🔒 "}${r.title} (${r.members.length}/${r.limit})`;
    this.settingsButton.hidden = !isOwner;
    this.members.replaceChildren(
      ...r.members.map((name) => {
        const li = document.createElement("li");
        const label = document.createElement("span");
        label.textContent = name === r.owner ? `★ ${name}` : name;
        li.append(label);
        if (isOwner && name !== r.owner) {
          const give = button("Make owner");
          give.classList.add("small");
          give.addEventListener("click", () => this.conn()?.giveRoom(name));
          const kick = button("Kick");
          kick.classList.add("small");
          kick.addEventListener("click", () => this.conn()?.kickFromRoom(name));
          li.append(give, kick);
        }
        return li;
      }),
    );
  }

  private line(kind: "system" | "self" | "public", text: string): void {
    const el = document.createElement("div");
    el.className = `chat-line ${kind}`;
    el.textContent = text;
    this.log.append(el);
    this.log.scrollTop = this.log.scrollHeight;
  }
}

/** Creating a room, or (for its owner) changing its settings. */
export class RoomSettingsWindow {
  readonly window = new GameWindow("roomsettings", "Chat Room Settings", { x: 260, y: 160, width: 320 });
  private readonly title = document.createElement("input");
  private readonly limit = document.createElement("input");
  private readonly isPublic = document.createElement("input");
  private readonly password = document.createElement("input");
  private readonly submit = document.createElement("button");
  private updating = false;

  constructor(private readonly conn: () => Connection | null) {
    const form = document.createElement("form");
    form.className = "room-settings";
    this.title.maxLength = 36;
    this.title.required = true;
    this.limit.type = "number";
    this.limit.min = "2";
    this.limit.max = "20";
    this.limit.required = true;
    this.isPublic.type = "checkbox";
    this.password.maxLength = 8;
    this.password.autocomplete = "off";
    this.isPublic.addEventListener("change", () => this.syncPassword());
    this.submit.type = "submit";
    form.append(
      field("Title", this.title),
      field("Max people", this.limit),
      field("Public (anyone can join)", this.isPublic, true),
      field("Password", this.password),
      this.submit,
    );
    form.addEventListener("submit", (ev) => {
      ev.preventDefault();
      const s: RoomSettings = {
        title: this.title.value.trim(),
        limit: Number(this.limit.value),
        public: this.isPublic.checked,
        password: this.password.value.trim(),
      };
      if (!s.public && !s.password) {
        toast("A private room needs a password.");
        return;
      }
      if (this.updating) this.conn()?.updateRoom(s);
      else this.conn()?.createRoom(s);
      this.window.close();
    });
    this.window.body.append(form);
  }

  /** Opens the form to create a room (no settings) or edit ours. */
  show(current?: RoomSettings): void {
    this.updating = !!current;
    this.title.value = current?.title ?? "";
    this.limit.value = String(current?.limit ?? 10);
    this.isPublic.checked = current?.public ?? true;
    this.password.value = "";
    this.submit.textContent = current ? "Save" : "Open chat room";
    this.syncPassword();
    this.window.open();
    this.title.focus();
  }

  private syncPassword(): void {
    this.password.disabled = this.isPublic.checked;
  }
}

/** Asks for a private room's password before joining. */
export class RoomPasswordWindow {
  readonly window = new GameWindow("roompassword", "Private Chat Room", { x: 280, y: 180, width: 280 });
  private readonly prompt = document.createElement("p");
  private readonly password = document.createElement("input");
  private roomId = 0;

  constructor(private readonly conn: () => Connection | null) {
    const form = document.createElement("form");
    form.className = "room-settings";
    this.password.type = "password";
    this.password.maxLength = 8;
    this.password.autocomplete = "off";
    form.append(this.prompt, field("Password", this.password), button("Join", "submit"));
    form.addEventListener("submit", (ev) => {
      ev.preventDefault();
      this.conn()?.joinRoom(this.roomId, this.password.value);
      this.window.close();
    });
    this.window.body.append(form);
  }

  ask(roomId: number, title: string): void {
    this.roomId = roomId;
    this.prompt.textContent = `"${title}" is private.`;
    this.password.value = "";
    this.window.open();
    this.password.focus();
  }
}

/** Explains a failed create/join. */
export function roomResultMessage(code: number): string {
  return RESULT_MESSAGES[code] ?? "You can't join that chat room.";
}

function button(label: string, type: "button" | "submit" = "button"): HTMLButtonElement {
  const b = document.createElement("button");
  b.type = type;
  b.textContent = label;
  return b;
}

function field(label: string, input: HTMLInputElement, inline = false): HTMLLabelElement {
  const l = document.createElement("label");
  l.className = inline ? "field inline" : "field";
  const span = document.createElement("span");
  span.textContent = label;
  if (inline) l.append(input, span);
  else l.append(span, input);
  return l;
}
