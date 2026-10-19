// Skill window (Alt+S): your skill tree, levels, and skill points to spend.
// Double-click a usable skill to use it, or drag it onto the hotkey bar.
import type { Connection } from "../net";
import { skillIcon, skillSp, type SkillInfo } from "../skills";
import type { PlayerState } from "../state";
import { hideTooltip, makeDraggable, moveTooltip, showInfo, showTooltip } from "./common";
import { GameWindow } from "./window";

export class SkillWindow {
  readonly window = new GameWindow("skills", "Skills", { x: 330, y: 120, width: 340 });
  private readonly list = document.createElement("ul");
  private readonly footer = document.createElement("div");

  constructor(
    private readonly state: PlayerState,
    private readonly conn: () => Connection | null,
    private readonly use: (skillId: number) => void,
  ) {
    this.list.className = "skill-list";
    this.footer.className = "window-footer";
    const hint = document.createElement("p");
    hint.className = "window-hint";
    hint.textContent = "Double-click a skill to use it, or drag it onto the hotkey bar. Right-click for details.";
    this.window.body.append(this.list, this.footer, hint);
    state.subscribe(() => this.window.isOpen && this.render());
    const open = this.window.open.bind(this.window);
    this.window.open = () => {
      open();
      this.render();
    };
  }

  private render(): void {
    const db = this.state.skillDb;
    this.list.replaceChildren();
    if (!db) return;
    for (const [id, level] of [...this.state.skills].sort((a, b) => a[0] - b[0])) {
      const info = db.get(id);
      if (info) this.list.append(this.row(info, level));
    }
    this.footer.replaceChildren();
    const points = document.createElement("span");
    points.textContent = `Skill Points ${this.state.skillPoints}`;
    if (this.state.skillPoints > 0) points.className = "status-points";
    this.footer.append(points);
  }

  private row(s: SkillInfo, level: number): HTMLElement {
    const li = document.createElement("li");
    li.className = level ? "skill-row" : "skill-row unlearned";

    const icon = document.createElement("button");
    icon.type = "button";
    icon.className = "item-cell skill-icon";
    icon.setAttribute("aria-label", s.displayName);
    const img = document.createElement("img");
    img.alt = "";
    img.width = img.height = 24;
    void skillIcon(s).then((src) => {
      if (src) img.src = src;
    });
    icon.append(img);
    const usable = level > 0 && s.target !== "passive";
    if (usable) {
      makeDraggable(icon, JSON.stringify({ kind: "skill", id: s.id }));
      icon.addEventListener("dblclick", () => this.use(s.id));
      icon.addEventListener("keydown", (ev) => {
        if (ev.key === "Enter" || ev.key === " ") {
          ev.preventDefault();
          this.use(s.id);
        }
      });
    }
    const tip = `${s.displayName}${usable ? "\nDouble-click to use · Drag to the hotkey bar" : ""}\nRight-click for details`;
    for (const el of [icon, li]) {
      el.addEventListener("contextmenu", (ev) => {
        ev.preventDefault();
        hideTooltip();
        showInfo(s.displayName, skillIcon(s), s.description, "skill-info-icon");
      });
    }
    icon.addEventListener("mouseenter", (ev) => showTooltip(tip, ev));
    icon.addEventListener("mousemove", moveTooltip);
    icon.addEventListener("mouseleave", hideTooltip);

    const name = document.createElement("div");
    name.className = "skill-name";
    const title = document.createElement("span");
    title.textContent = s.displayName;
    const meta = document.createElement("span");
    meta.className = "skill-meta";
    const kind = s.target === "passive" ? "Passive" : s.quest ? "Quest skill" : `SP ${skillSp(s, Math.max(level, 1))}`;
    meta.textContent = `Lv ${level} / ${s.maxLevel} · ${kind}`;
    name.append(title, meta);

    li.append(icon, name);
    if (!s.quest) {
      const plus = document.createElement("button");
      plus.type = "button";
      plus.className = "stat-plus";
      plus.textContent = "+";
      plus.setAttribute("aria-label", `Learn ${s.displayName} level ${level + 1}`);
      plus.disabled = !this.canLearn(s, level);
      plus.addEventListener("click", () => this.conn()?.learnSkill(s.id));
      li.append(plus);
    }
    return li;
  }

  private canLearn(s: SkillInfo, level: number): boolean {
    return (
      this.state.skillPoints > 0 &&
      level < s.maxLevel &&
      s.requires.every((r) => (this.state.skills.get(r.id) ?? 0) >= r.level)
    );
  }
}
