// Sound: map BGM (an <audio> element streaming from /bgm) and sound effects
// (Web Audio, decoded .wav files from /data/wav). Browsers only allow audio
// after a user gesture, so unlock() is called from the login button.
import { fetchData } from "./assets";

export interface AudioSettings {
  music: number; // 0-1
  effects: number; // 0-1
  muted: boolean;
}

const SETTINGS_KEY = "kdoiro.audio";
const defaults: AudioSettings = { music: 0.5, effects: 0.7, muted: false };

class AudioEngine {
  settings: AudioSettings = loadSettings();
  private ctx: AudioContext | null = null;
  private effectsGain: GainNode | null = null;
  private readonly buffers = new Map<string, Promise<AudioBuffer | null>>();
  /** Last start time per sound, so a crowd of monsters doesn't stack the same clip. */
  private readonly lastPlayed = new Map<string, number>();
  private readonly music = new Audio();
  private musicFile = "";
  private bgmTable: Promise<Map<string, string>> | null = null;

  constructor() {
    this.music.loop = true;
    this.music.preload = "auto";
  }

  /** Call from a user gesture (click/keypress) so the browser allows sound. */
  unlock(): void {
    if (!this.ctx) {
      this.ctx = new AudioContext();
      this.effectsGain = this.ctx.createGain();
      this.effectsGain.connect(this.ctx.destination);
      this.applySettings();
    }
    void this.ctx.resume();
    if (this.musicFile && this.music.paused) void this.music.play().catch(() => {});
  }

  update(change: Partial<AudioSettings>): void {
    this.settings = { ...this.settings, ...change };
    try {
      localStorage.setItem(SETTINGS_KEY, JSON.stringify(this.settings));
    } catch {
      // Storage unavailable: settings last for this visit only.
    }
    this.applySettings();
  }

  private applySettings(): void {
    const { music, effects, muted } = this.settings;
    this.music.volume = muted ? 0 : music;
    if (this.effectsGain) this.effectsGain.gain.value = muted ? 0 : effects;
  }

  /**
   * Plays a sound effect from data/wav/ at a volume (0-1). Repeats of the same
   * clip within 60 ms are dropped.
   */
  play(file: string, volume = 1): void {
    if (!this.ctx || !this.effectsGain || volume <= 0.01 || this.settings.muted) return;
    const key = file.toLowerCase();
    const now = performance.now();
    if (now - (this.lastPlayed.get(key) ?? -1e9) < 60) return;
    this.lastPlayed.set(key, now);

    void this.buffer(key).then((buf) => {
      if (!buf || !this.ctx || !this.effectsGain) return;
      const src = this.ctx.createBufferSource();
      src.buffer = buf;
      const gain = this.ctx.createGain();
      gain.gain.value = Math.min(1, volume);
      src.connect(gain).connect(this.effectsGain);
      src.start();
    });
  }

  private buffer(file: string): Promise<AudioBuffer | null> {
    let p = this.buffers.get(file);
    if (!p) {
      p = fetchData(`wav/${file}`)
        .then((data) => this.ctx!.decodeAudioData(data))
        .catch((err) => {
          console.warn("sound missing", file, err);
          return null;
        });
      this.buffers.set(file, p);
    }
    return p;
  }

  /** Starts the map's BGM from data/mp3nametable.txt (keeps playing if unchanged). */
  async playMapMusic(mapName: string): Promise<void> {
    this.bgmTable ??= loadBgmTable();
    const file = (await this.bgmTable).get(mapName.toLowerCase());
    if (!file) {
      this.stopMusic();
      return;
    }
    if (file === this.musicFile && !this.music.paused) return;
    this.musicFile = file;
    this.music.src = encodeURI(`/bgm/${file}`);
    this.applySettings();
    if (this.ctx) void this.music.play().catch(() => {}); // after unlock()
  }

  stopMusic(): void {
    this.musicFile = "";
    this.music.pause();
    this.music.removeAttribute("src");
    this.music.load();
  }
}

/** "morocc.rsw#bgm\\11.mp3#" lines -> map name -> "11.mp3". */
async function loadBgmTable(): Promise<Map<string, string>> {
  const out = new Map<string, string>();
  try {
    const text = new TextDecoder("euc-kr").decode(await fetchData("mp3nametable.txt"));
    for (const line of text.split(/\r?\n/)) {
      const m = /^([^#]+)\.rsw#bgm\\+([^#]+)#/i.exec(line.trim());
      if (m) out.set(m[1].toLowerCase(), m[2]);
    }
  } catch (err) {
    console.warn("no BGM table", err);
  }
  return out;
}

function loadSettings(): AudioSettings {
  try {
    const saved = localStorage.getItem(SETTINGS_KEY);
    if (saved) return { ...defaults, ...JSON.parse(saved) };
  } catch {
    // Fall back to defaults.
  }
  return { ...defaults };
}

export const audio = new AudioEngine();

/** Hearing range in map cells for positional sounds. */
export const HEARING_RANGE = 18;

/** Volume for a sound `cells` away from the listener (linear falloff). */
export function falloff(cells: number, range = HEARING_RANGE): number {
  return Math.max(0, 1 - cells / range);
}
