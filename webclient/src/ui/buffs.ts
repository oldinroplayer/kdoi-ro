// Buff icons under the HUD card: the skill's icon with the seconds left,
// like RO's status icons on the right of the screen.
import { skillIcon } from "../skills";
import type { PlayerState } from "../state";

export class BuffBar {
  private readonly icons = new Map<number, { el: HTMLElement; time: HTMLElement }>();

  constructor(
    private readonly root: HTMLElement,
    private readonly state: PlayerState,
  ) {
    state.subscribe(() => this.render());
    setInterval(() => this.tick(), 250);
  }

  private render(): void {
    for (const [id, icon] of this.icons) {
      if (!this.state.buffs.has(id)) {
        icon.el.remove();
        this.icons.delete(id);
      }
    }
    for (const [id, buff] of this.state.buffs) {
      const info = this.state.skillDb?.get(id);
      let icon = this.icons.get(id);
      if (!icon) {
        const el = document.createElement("div");
        el.className = "buff";
        const img = document.createElement("img");
        img.alt = "";
        if (info) void skillIcon(info).then((src) => (img.src = src)).catch(() => {});
        const time = document.createElement("span");
        el.append(img, time);
        this.root.append(el);
        icon = { el, time };
        this.icons.set(id, icon);
      }
      icon.el.title = `${info?.displayName ?? "Buff"} Lv ${buff.level}`;
    }
    this.root.hidden = this.icons.size === 0;
    this.tick();
  }

  private tick(): void {
    const now = performance.now();
    for (const [id, icon] of this.icons) {
      const buff = this.state.buffs.get(id);
      if (!buff) continue;
      const s = Math.max(0, Math.ceil((buff.until - now) / 1000));
      icon.time.textContent = s >= 60 ? `${Math.ceil(s / 60)}m` : `${s}s`;
      icon.el.classList.toggle("ending", s <= 10);
    }
  }
}
