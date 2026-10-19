// Party window (Alt+Z): create a party, invite players, see members' maps and
// HP, chat with "/p" or "%", and (as leader) kick or switch EXP sharing.
import type { Connection, PartyMember } from "../net";
import { jobName } from "../render/character";
import type { PlayerState } from "../state";
import { GameWindow } from "./window";

export class PartyWindow {
  readonly window = new GameWindow("party", "Party", { x: 24, y: 140, width: 320 });

  constructor(
    private readonly state: PlayerState,
    private readonly conn: () => Connection | null,
  ) {
    state.subscribe(() => this.window.isOpen && this.render());
    const open = this.window.open.bind(this.window);
    this.window.open = () => {
      open();
      this.render();
    };
  }

  private render(): void {
    const party = this.state.party;
    this.window.body.replaceChildren(party ? this.partyView() : this.createView());
  }

  /** No party yet: name one and found it. */
  private createView(): HTMLElement {
    const box = document.createElement("div");
    box.className = "party-empty";
    const note = document.createElement("p");
    note.className = "muted";
    note.textContent = "You're not in a party. Found one, or wait for an invitation.";
    const form = inlineForm("Party name", "Create", 23, (name) => this.conn()?.createParty(name));
    box.append(note, form);
    return box;
  }

  private partyView(): HTMLElement {
    const party = this.state.party!;
    const me = this.state.name.toLowerCase();
    const leading = party.leader.toLowerCase() === me;
    const box = document.createElement("div");
    box.className = "party-view";

    const head = document.createElement("div");
    head.className = "party-head";
    const title = document.createElement("strong");
    title.textContent = party.name;
    const share = document.createElement("button");
    share.type = "button";
    share.className = "secondary small";
    share.textContent = party.evenExp ? "EXP: Even share" : "EXP: Each take";
    share.disabled = !leading;
    share.title = leading ? "Click to switch (even share needs levels within 10)" : "Only the leader can change this";
    share.addEventListener("click", () => this.conn()?.setPartyExp(!party.evenExp));
    head.append(title, share);

    const list = document.createElement("ul");
    list.className = "party-members";
    for (const m of party.members) list.append(this.memberRow(m, party.leader, leading, me));

    box.append(head, list);
    if (leading && party.members.length < 12) {
      box.append(inlineForm("Invite by name", "Invite", 23, (name) => this.conn()?.inviteToParty(name)));
    }
    const hint = document.createElement("p");
    hint.className = "muted party-hint";
    hint.textContent = "Party chat: start a line with % or /p";
    const leave = document.createElement("button");
    leave.type = "button";
    leave.className = "secondary";
    leave.textContent = "Leave party";
    leave.addEventListener("click", () => this.conn()?.leaveParty());
    const actions = document.createElement("div");
    actions.className = "room-actions";
    actions.append(leave);
    box.append(hint, actions);
    return box;
  }

  private memberRow(m: PartyMember, leader: string, leading: boolean, me: string): HTMLElement {
    const li = document.createElement("li");
    li.className = m.online ? "online" : "offline";
    const dot = document.createElement("span");
    dot.className = "party-dot";
    dot.title = m.online ? "Online" : "Offline";
    const info = document.createElement("div");
    info.className = "party-info";
    const name = document.createElement("div");
    name.className = "party-name";
    name.textContent = (m.name.toLowerCase() === leader.toLowerCase() ? "★ " : "") + m.name;
    const detail = document.createElement("div");
    detail.className = "party-detail";
    detail.textContent = m.online ? `${jobName(m.job)} Lv ${m.baseLevel} · ${m.map} (${m.x}, ${m.y})` : "Offline";
    info.append(name, detail);
    if (m.online && m.maxHp > 0) {
      const bar = document.createElement("div");
      bar.className = "bar hp party-hp";
      const fill = document.createElement("div");
      fill.className = "bar-fill";
      fill.style.width = `${Math.min(100, (100 * m.hp) / m.maxHp)}%`;
      const text = document.createElement("span");
      text.className = "bar-text";
      text.textContent = `${m.hp} / ${m.maxHp}`;
      bar.append(fill, text);
      info.append(bar);
    }
    li.append(dot, info);
    if (leading && m.name.toLowerCase() !== me) {
      const kick = document.createElement("button");
      kick.type = "button";
      kick.className = "secondary small";
      kick.textContent = "Kick";
      kick.addEventListener("click", () => this.conn()?.kickFromParty(m.name));
      li.append(kick);
    }
    return li;
  }
}

/** Asks whether to join a party we were invited to. */
export class PartyInviteWindow {
  readonly window = new GameWindow("party-invite", "Party Invitation", { x: 280, y: 180, width: 300 });
  private readonly text = document.createElement("p");
  private partyId = 0;
  private answered = true;

  constructor(private readonly conn: () => Connection | null) {
    const actions = document.createElement("div");
    actions.className = "room-actions";
    const decline = document.createElement("button");
    decline.type = "button";
    decline.className = "secondary";
    decline.textContent = "Decline";
    decline.addEventListener("click", () => this.reply(false));
    const accept = document.createElement("button");
    accept.type = "button";
    accept.textContent = "Join";
    accept.addEventListener("click", () => this.reply(true));
    actions.append(decline, accept);
    this.window.body.append(this.text, actions);
    // Closing the window declines, so the inviter isn't left waiting.
    this.window.onClose = () => {
      if (!this.answered) this.reply(false);
    };
  }

  show(partyId: number, partyName: string, from: string): void {
    this.partyId = partyId;
    this.answered = false;
    this.text.textContent = `${from} invites you to join the party "${partyName}".`;
    this.window.open();
  }

  private reply(accept: boolean): void {
    if (this.answered) return;
    this.answered = true;
    this.conn()?.replyPartyInvite(this.partyId, accept);
    this.window.close();
  }
}

function inlineForm(placeholder: string, label: string, maxLength: number, submit: (value: string) => void): HTMLFormElement {
  const form = document.createElement("form");
  form.className = "room-form";
  const input = document.createElement("input");
  input.placeholder = placeholder;
  input.maxLength = maxLength;
  input.setAttribute("aria-label", placeholder);
  const button = document.createElement("button");
  button.type = "submit";
  button.textContent = label;
  form.append(input, button);
  form.addEventListener("submit", (ev) => {
    ev.preventDefault();
    const value = input.value.trim();
    if (!value) return;
    input.value = "";
    submit(value);
  });
  return form;
}
