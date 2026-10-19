// The server line on the login screen and in game (under the minimap):
// whether the server is up, how many are playing, and its EXP and drop rates
// (the server's /status.json).

interface ServerStats {
  players: number;
  expRate: number;
  dropRate: number;
}

/** How often to refresh while the login screen is showing (ms). */
const REFRESH_MS = 10_000;

const rate = (r: number) => `x${Number(r.toFixed(2))}`;

export class ServerStatsLine {
  private timer = 0;
  private request = 0;

  constructor(
    private readonly root: HTMLElement,
    private readonly serverUrl: () => string,
  ) {}

  /** Refreshes now and every few seconds until stop(). */
  start(): void {
    this.stop();
    void this.refresh();
    this.timer = window.setInterval(() => void this.refresh(), REFRESH_MS);
  }

  stop(): void {
    clearInterval(this.timer);
    this.timer = 0;
  }

  async refresh(): Promise<void> {
    const request = ++this.request;
    let stats: ServerStats | null = null;
    try {
      const url = new URL(this.serverUrl());
      url.protocol = url.protocol === "wss:" ? "https:" : "http:";
      url.pathname = "/status.json";
      url.search = "";
      const res = await fetch(url, { cache: "no-store" });
      if (res.ok) stats = (await res.json()) as ServerStats;
    } catch {
      // Unreachable or a bad address: shown as offline.
    }
    if (request !== this.request) return; // a newer refresh is on its way
    this.show(stats);
  }

  private show(stats: ServerStats | null): void {
    const set = (cls: string, text: string) => {
      const el = this.root.querySelector<HTMLElement>(`.${cls}`);
      if (el) el.textContent = text;
    };
    this.root.classList.toggle("online", !!stats);
    this.root.classList.toggle("offline", !stats);
    if (!stats) {
      set("stat-online", "Server offline");
      set("stat-exp", "–");
      set("stat-drop", "–");
      return;
    }
    set("stat-online", `${stats.players} ${stats.players === 1 ? "player" : "players"} online`);
    set("stat-exp", rate(stats.expRate));
    set("stat-drop", rate(stats.dropRate));
  }
}
