// A simple auto-attack bot, run by the client: it attacks the nearest monster
// in sight, picks up the loot lying around between fights and, when there is
// nothing to do, wanders to a random nearby cell to find a monster. It keeps
// clear of warp portals so it never walks itself off the map. The server
// still does the walking and fighting; the bot only sends the same Attack and
// Move requests a player's clicks would.
import type { Game } from "./game";
import type { Connection } from "./net";

/** How often the bot looks around (ms). */
const TICK_MS = 500;
/** How far (in cells) the bot looks for monsters, about a screen's worth. */
const SIGHT = 14;
/** No swing at the target for this long: ask the server to attack again. */
const REATTACK_MS = 3000;
/** Never reached the target in this long: skip it for a while (unreachable). */
const GIVE_UP_MS = 10000;
const IGNORE_MS = 60000;
/** A target that strayed near a warp is left alone this long. */
const WARP_IGNORE_MS = 10000;
/** How far (in cells) the bot goes for items on the ground. */
const LOOT_RANGE = 10;
/** Ask again this often while walking to an item (the killer's first pick
 * lasts 3 s, and the server drops the request if it can't pick up yet). */
const LOOT_RETRY_MS = 2000;
/** Item still there after this long: skip it (too heavy, unreachable, not ours). */
const LOOT_GIVE_UP_MS = 8000;
/** How far a wander step goes, in cells. */
const WANDER_MIN = 6;
const WANDER_MAX = 14;
/** How close (in cells) to a warp's centre the bot never goes: the server
 * warps anyone within a warp's span (up to 5 cells), plus one to spare. */
const WARP_MARGIN = 6;

type Cell = { x: number; y: number };

export class AutoBot {
  private timer = 0;
  private target = 0;
  private targetSince = 0;
  private lastAttackSent = 0;
  private map = "";
  /** Monsters we couldn't reach, until when to leave them alone. */
  private readonly ignored = new Map<number, number>();
  private loot = 0;
  private lootSince = 0;
  private lootSent = 0;
  /** Items we couldn't pick up, until when to leave them alone. */
  private readonly ignoredItems = new Map<number, number>();
  /** The current map's warps (from the last look around). */
  private warps: Cell[] = [];

  constructor(
    private readonly game: () => Game | null,
    private readonly conn: () => Connection | null,
    /** Told when the bot starts or stops (with a reason when it stops by itself). */
    private readonly onChange: (running: boolean, reason?: string) => void,
  ) {}

  get running(): boolean {
    return this.timer !== 0;
  }

  toggle(): void {
    if (this.running) this.stop();
    else this.start();
  }

  start(): void {
    if (this.running) return;
    this.reset();
    this.timer = window.setInterval(() => this.tick(), TICK_MS);
    this.onChange(true);
    this.tick();
  }

  stop(reason?: string): void {
    if (!this.running) return;
    clearInterval(this.timer);
    this.timer = 0;
    this.onChange(false, reason);
  }

  private reset(): void {
    this.target = 0;
    this.loot = 0;
    this.ignored.clear();
    this.ignoredItems.clear();
  }

  private tick(): void {
    const view = this.game()?.botView();
    const conn = this.conn();
    if (!view || !conn?.connected) return;
    if (view.self.dead) {
      this.stop("You were knocked out, so auto attack stopped.");
      return;
    }
    if (view.self.sitting) return; // resting: stand up (Insert) to carry on
    if (view.map !== this.map) {
      this.map = view.map; // warped: forget the old map's monsters
      this.reset();
    }
    const now = performance.now();
    this.warps = view.warps;
    for (const [id, until] of this.ignored) if (until < now) this.ignored.delete(id);
    for (const [id, until] of this.ignoredItems) if (until < now) this.ignoredItems.delete(id);

    // Keep fighting the current target while it lives.
    const current = view.monsters.find((m) => m.id === this.target);
    if (current && this.risky(view.self, current)) {
      // It wandered by a warp: chasing it could carry us off the map.
      this.ignored.set(this.target, now + WARP_IGNORE_MS);
      this.target = 0;
      conn.move(view.self.x, view.self.y); // a move ends the server's chase
    } else if (current) {
      const swungAt = view.lastSwing.target === this.target ? view.lastSwing.at : 0;
      if (now - swungAt < REATTACK_MS) return; // the fight is on
      if (now - this.targetSince > GIVE_UP_MS && swungAt < this.targetSince) {
        this.ignored.set(this.target, now + IGNORE_MS); // never got to it
        this.target = 0;
      } else {
        if (now - this.lastAttackSent > REATTACK_MS) this.attack(conn, this.target, now);
        return;
      }
    }

    // Between fights, pick up what's lying around first.
    const { x, y } = view.self;
    if (this.collectLoot(conn, view.items, x, y, now)) return;

    // Pick the nearest monster in sight.
    let best: { id: number; d: number } | null = null;
    for (const m of view.monsters) {
      const d = Math.max(Math.abs(m.x - x), Math.abs(m.y - y));
      if (d <= SIGHT && !this.ignored.has(m.id) && (!best || d < best.d) && !this.risky(view.self, m)) best = { id: m.id, d };
    }
    if (best) {
      this.target = best.id;
      this.targetSince = now;
      this.attack(conn, best.id, now);
      return;
    }

    // Nothing around: wander somewhere nearby once the last walk is done.
    this.target = 0;
    if (!view.self.walking) this.wander(conn, x, y);
  }

  /** Whether going from one cell to another gets near a warp: the goal is
   * within the margin, or the straight line there (roughly the route walked)
   * comes inside it closer than we already are. So right after arriving by
   * warp the bot can still walk away from it. */
  private risky(from: Cell, to: Cell): boolean {
    const dist = (a: Cell, b: Cell) => Math.max(Math.abs(a.x - b.x), Math.abs(a.y - b.y));
    const steps = Math.max(dist(from, to), 1);
    for (const w of this.warps) {
      if (dist(to, w) <= WARP_MARGIN) return true;
      const start = dist(from, w);
      for (let i = 1; i < steps; i++) {
        const cell = {
          x: Math.round(from.x + ((to.x - from.x) * i) / steps),
          y: Math.round(from.y + ((to.y - from.y) * i) / steps),
        };
        const d = dist(cell, w);
        if (d <= WARP_MARGIN && d < start) return true;
      }
    }
    return false;
  }

  /** Walks to and picks up the nearest item in reach; true while busy with it. */
  private collectLoot(conn: Connection, items: { id: number; x: number; y: number }[], x: number, y: number, now: number): boolean {
    const current = items.find((it) => it.id === this.loot);
    if (current) {
      if (now - this.lootSince > LOOT_GIVE_UP_MS) {
        this.ignoredItems.set(this.loot, now + IGNORE_MS);
      } else {
        if (now - this.lootSent > LOOT_RETRY_MS) this.pickUp(conn, this.loot, now);
        return true;
      }
    }
    this.loot = 0;
    let best: { id: number; d: number } | null = null;
    for (const it of items) {
      const d = Math.max(Math.abs(it.x - x), Math.abs(it.y - y));
      if (d <= LOOT_RANGE && !this.ignoredItems.has(it.id) && (!best || d < best.d) && !this.risky({ x, y }, it)) {
        best = { id: it.id, d };
      }
    }
    if (!best) return false;
    this.loot = best.id;
    this.lootSince = now;
    this.pickUp(conn, best.id, now);
    return true;
  }

  private pickUp(conn: Connection, id: number, now: number): void {
    this.lootSent = now;
    conn.pickUp(id);
  }

  private attack(conn: Connection, id: number, now: number): void {
    this.lastAttackSent = now;
    conn.attack(id);
  }

  private wander(conn: Connection, x: number, y: number): void {
    const game = this.game();
    if (!game) return;
    for (let i = 0; i < 20; i++) {
      const angle = Math.random() * Math.PI * 2;
      const dist = WANDER_MIN + Math.random() * (WANDER_MAX - WANDER_MIN);
      const tx = Math.round(x + Math.cos(angle) * dist);
      const ty = Math.round(y + Math.sin(angle) * dist);
      if (game.walkable(tx, ty) && !this.risky({ x, y }, { x: tx, y: ty })) {
        conn.move(tx, ty);
        return;
      }
    }
  }
}
