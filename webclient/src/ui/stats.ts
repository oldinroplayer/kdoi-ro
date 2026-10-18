// Status window (Alt+A): base stats with + buttons, and derived battle stats.
import type { Connection } from "../net";
import type { PlayerState } from "../state";
import { formatZeny } from "./common";
import { GameWindow } from "./window";

const statNames = ["STR", "AGI", "VIT", "INT", "DEX", "LUK"];
const statHelp = [
  "Strength: attack power and carrying capacity",
  "Agility: flee rate and attack speed",
  "Vitality: max HP and defence",
  "Intelligence: magic attack, max SP and magic defence",
  "Dexterity: hit rate and attack speed",
  "Luck: critical rate and perfect dodge",
];

export class StatusWindow {
  readonly window = new GameWindow("status", "Status", { x: 16, y: 420, width: 380 });

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
    const s = this.state.status;
    const body = this.window.body;
    body.replaceChildren();
    if (!s) return;

    const head = document.createElement("div");
    head.className = "status-head";
    head.append(
      kv("Name", this.state.name),
      kv("Job", "Novice"),
      kv("Base Lv", String(s.baseLevel)),
      kv("Job Lv", String(s.jobLevel)),
      kv("HP", `${s.hp} / ${s.maxHp}`),
      kv("SP", `${s.sp} / ${s.maxSp}`),
      kv("Base EXP", s.baseNext ? `${s.baseExp} / ${s.baseNext}` : "MAX"),
      kv("Job EXP", s.jobNext ? `${s.jobExp} / ${s.jobNext}` : "MAX"),
    );

    const grid = document.createElement("div");
    grid.className = "status-grid";

    const base = document.createElement("div");
    base.className = "status-base";
    statNames.forEach((name, i) => {
      const row = document.createElement("div");
      row.className = "stat-row";
      row.title = statHelp[i];
      const label = document.createElement("span");
      label.className = "stat-name";
      label.textContent = name;
      const value = document.createElement("span");
      value.className = "stat-value";
      value.textContent = String(s.stats[i]);
      const plus = document.createElement("button");
      plus.type = "button";
      plus.className = "stat-plus";
      plus.textContent = "+";
      plus.setAttribute("aria-label", `Raise ${name} (costs ${s.raiseCosts[i]} points)`);
      plus.disabled = s.raiseCosts[i] > s.statusPoints || s.stats[i] >= 99;
      plus.addEventListener("click", () => this.conn()?.raiseStat(i));
      const cost = document.createElement("span");
      cost.className = "stat-cost";
      cost.textContent = String(s.raiseCosts[i]);
      cost.title = "Status points needed to raise";
      row.append(label, value, plus, cost);
      base.append(row);
    });
    const points = document.createElement("div");
    points.className = "status-points";
    points.textContent = `Status Points ${s.statusPoints}`;
    base.append(points);

    const derived = document.createElement("div");
    derived.className = "status-derived";
    derived.append(
      kv("Atk", `${s.atk} + ${s.atk2}`),
      kv("Def", `${s.def} + ${s.def2}`),
      kv("Matk", `${s.matkMin} ~ ${s.matkMax}`),
      kv("Mdef", `${s.mdef} + ${s.mdef2}`),
      kv("Hit", String(s.hit)),
      kv("Flee", `${s.flee} + ${s.flee2}`),
      kv("Critical", String(s.crit)),
      kv("Aspd", String(s.aspd)),
    );

    grid.append(base, derived);

    const footer = document.createElement("div");
    footer.className = "window-footer";
    const weight = document.createElement("span");
    weight.textContent = `Weight ${s.weight} / ${s.maxWeight}`;
    const zeny = document.createElement("span");
    zeny.textContent = formatZeny(s.zeny);
    footer.append(weight, zeny);

    body.append(head, grid, footer);
  }
}

function kv(key: string, value: string): HTMLElement {
  const row = document.createElement("div");
  row.className = "kv";
  const k = document.createElement("span");
  k.textContent = key;
  const v = document.createElement("span");
  v.textContent = value;
  row.append(k, v);
  return row;
}
