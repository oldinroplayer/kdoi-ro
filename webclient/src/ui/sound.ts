// Sound settings window: music and effects volume, and mute.
import { audio } from "../audio";
import { GameWindow } from "./window";

export class SoundWindow {
  readonly window = new GameWindow("sound", "Sound", { x: 420, y: 90, width: 280 });

  constructor() {
    const body = this.window.body;
    body.classList.add("sound-settings");
    body.append(
      this.slider("Music", audio.settings.music, (v) => audio.update({ music: v })),
      this.slider("Effects", audio.settings.effects, (v) => {
        audio.update({ effects: v });
        audio.play("_enemy_hit_normal1.wav"); // let the player hear the new level
      }),
      this.muteToggle(),
    );
  }

  private slider(label: string, value: number, onChange: (v: number) => void): HTMLElement {
    const row = document.createElement("label");
    row.className = "sound-row";
    const name = document.createElement("span");
    name.textContent = label;
    const input = document.createElement("input");
    input.type = "range";
    input.min = "0";
    input.max = "100";
    input.value = String(Math.round(value * 100));
    const pct = document.createElement("output");
    pct.textContent = `${input.value}%`;
    input.addEventListener("input", () => (pct.textContent = `${input.value}%`));
    input.addEventListener("change", () => onChange(Number(input.value) / 100));
    row.append(name, input, pct);
    return row;
  }

  private muteToggle(): HTMLElement {
    const row = document.createElement("label");
    row.className = "sound-mute";
    const box = document.createElement("input");
    box.type = "checkbox";
    box.checked = audio.settings.muted;
    box.addEventListener("change", () => audio.update({ muted: box.checked }));
    row.append(box, document.createTextNode(" Mute all sound"));
    return row;
  }
}
