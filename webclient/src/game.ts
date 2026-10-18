// The in-game view: renders the current map, players, NPCs and monsters, and
// turns clicks into Move / Attack / NpcClick packets.
import * as THREE from "three";
import { audio, falloff } from "./audio";
import { heightAt, isWalkable } from "./formats/gat";
import type { Connection, Look, ServerPacket } from "./net";
import {
  CharacterSprite,
  loadEquipLayers,
  loadMonsterAssets,
  loadNoviceAssets,
  loadNpcAssets,
  MonsterAction,
  PlayerAction,
  SPRITE_SCALE,
  type CharacterAssets,
} from "./render/character";
import { FloatingTexts, NameLabels } from "./render/labels";
import { CELL, loadMap, roToWorld, worldToRo, type MapView } from "./render/map";
import { Portal } from "./render/portal";

type EnterMap = Extract<ServerPacket, { type: "enterMap" }>;

/** What the minimap draws: positions in map cells (y = 0 at the south edge). */
export interface MinimapView {
  map: string;
  width: number;
  height: number;
  self: { x: number; y: number; dir: number } | null;
  players: [number, number][];
  npcs: [number, number][];
  portals: [number, number][];
}
type Damage = Extract<ServerPacket, { type: "damage" }>;

interface Segment {
  fx: number;
  fy: number;
  tx: number;
  ty: number;
  t0: number;
  dur: number;
}

type Kind = "player" | "monster" | "npc";

/** A one-off animation (attack, flinch) played over `duration` ms. */
interface OneShot {
  group: number;
  start: number;
  duration: number;
}

class Unit {
  /** Fractional cell coordinates of the feet (cell centre = x + 0.5). */
  x: number;
  y: number;
  /** Server facing: 0 N, 1 NW, 2 W, 3 SW, 4 S, 5 SE, 6 E, 7 NE. */
  dir: number;
  walk: Segment[] = [];
  walkStart = 0;
  oneShot: OneShot | null = null;
  /** Players lying knocked out. */
  dead = false;
  /** When a monster's death animation started (it's removed afterwards). */
  dyingSince = 0;
  /** Last time this unit hit or was hit; players hold a battle stance for a while. */
  lastCombat = 0;
  /** Visible equipment (players only). */
  look: Look | null = null;
  /** Last animation frame shown, so each frame's sound plays once. */
  lastFrameKey = "";
  readonly bornAt = performance.now();

  constructor(
    readonly id: number,
    readonly kind: Kind,
    readonly name: string,
    readonly speed: number,
    readonly sprite: CharacterSprite,
    readonly label: HTMLElement,
    x: number,
    y: number,
    dir: number,
  ) {
    this.x = x + 0.5;
    this.y = y + 0.5;
    this.dir = dir;
  }
}

/** Cells from an NPC within which the server lets you talk to it (world.NpcRange). */
const NPC_RANGE = 4;

export interface GameHud {
  setStatus(text: string): void;
  setPosition(map: string, x: number, y: number): void;
  /** Our character was knocked out (true) or is back on its feet (false). */
  setDead(dead: boolean): void;
  /** Sprite name of an item shown on characters (from the item database). */
  itemSprite(itemId: number): string | undefined;
}

/** How long players stay in battle stance (weapon drawn) after fighting. */
const COMBAT_STANCE_MS = 5000;

const DIAGONAL = 1.4;
/** Sprite pixels from the feet down to where the name label starts (below the shadow). */
const LABEL_DROP = 10;
/** Sprite pixels from the feet up to where damage numbers appear. */
const HEAD_HEIGHT = 75;
const PITCH = THREE.MathUtils.degToRad(50);

export class Game {
  /** Our character's name (EnterMap doesn't carry it); set by the login screen. */
  selfName = "";
  private readonly renderer: THREE.WebGLRenderer;
  private readonly scene = new THREE.Scene();
  private readonly camera = new THREE.PerspectiveCamera(30, 1, 5, 4000);
  private readonly raycaster = new THREE.Raycaster();
  private readonly cursor: THREE.Mesh;
  private readonly labels: NameLabels;
  private readonly floats: FloatingTexts;
  private map: MapView | null = null;
  private playerAssets: CharacterAssets | null = null;
  /** Monster sprites already loaded, by sprite name. */
  private readonly monsterAssets = new Map<string, CharacterAssets>();
  private readonly units = new Map<number, Unit>();
  private readonly npcs = new Map<number, Unit>();
  private portals: Portal[] = [];
  private portalCells: [number, number][] = [];
  /** Bumped on every map change, so late-loading sprites for an old map are dropped. */
  private mapGeneration = 0;
  /** NPC we're walking towards to talk to, and when we set off. */
  private pendingNpc: number | null = null;
  private pendingSince = 0;
  private hovered: Unit | null = null;
  /** When each of the map's ambient sounds may play next. */
  private ambientNext: number[] = [];
  private nextAmbientCheck = 0;
  private selfId = 0;
  private loading = false;
  private queue: ServerPacket[] = [];
  private yaw = 0;
  private distance = 320;
  private readonly target = new THREE.Vector3();
  private frame = 0;
  private disposed = false;
  private readonly cleanup: (() => void)[] = [];

  constructor(
    private readonly canvas: HTMLCanvasElement,
    private readonly conn: Connection,
    private readonly hud: GameHud,
  ) {
    this.renderer = new THREE.WebGLRenderer({ canvas, antialias: true });
    this.labels = new NameLabels(canvas);
    this.floats = new FloatingTexts(canvas);
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    this.scene.background = new THREE.Color(0x0e0d12);

    const cursorGeo = new THREE.PlaneGeometry(CELL * 0.9, CELL * 0.9);
    cursorGeo.rotateX(-Math.PI / 2);
    this.cursor = new THREE.Mesh(
      cursorGeo,
      new THREE.MeshBasicMaterial({ color: 0xffffff, transparent: true, opacity: 0.35, depthWrite: false }),
    );
    this.cursor.renderOrder = 1;
    this.cursor.visible = false;
    this.scene.add(this.cursor);

    this.bindInput();
    this.resize();
    const loop = () => {
      if (this.disposed) return;
      this.frame = requestAnimationFrame(loop);
      this.render(performance.now());
    };
    loop();
  }

  handle(p: ServerPacket): void {
    if (p.type === "enterMap") {
      void this.enterMap(p);
      return;
    }
    if (this.loading) {
      this.queue.push(p);
      return;
    }
    switch (p.type) {
      case "appear":
        this.addPlayer(p.id, p.name, p.speed, p.x, p.y, p.dir, p.dead, p.look);
        break;
      case "look": {
        const u = this.units.get(p.id);
        if (u) {
          u.look = p.look;
          this.applyLook(u);
        }
        break;
      }
      case "monster":
        void this.addMonster(p.id, p.name, p.sprite, p.speed, p.x, p.y, p.dir);
        break;
      case "disappear": {
        const u = this.units.get(p.id);
        if (u && p.died) {
          u.dyingSince = performance.now();
          u.walk = [];
          u.label.remove();
        } else {
          this.removeUnit(p.id);
        }
        break;
      }
      case "move":
        this.startWalk(p.id, p.fromX, p.fromY, p.steps);
        break;
      case "stop": {
        const u = this.units.get(p.id);
        if (u) {
          u.walk = [];
          u.x = p.x + 0.5;
          u.y = p.y + 0.5;
        }
        break;
      }
      case "portal":
        this.addPortal(p.x, p.y);
        break;
      case "npc":
        void this.addNpc(p.id, p.name, p.sprite, p.x, p.y, p.dir);
        break;
      case "damage":
        this.showDamage(p);
        break;
      case "dead": {
        const u = this.units.get(p.id);
        if (u) {
          u.dead = true;
          u.walk = [];
          u.oneShot = null;
        }
        if (p.id === this.selfId) this.hud.setDead(true);
        break;
      }
      case "levelUp": {
        const u = this.units.get(p.id);
        if (u) {
          this.floats.spawn(p.job ? "JOB LEVEL UP" : "LEVEL UP!", p.job ? "level job" : "level", this.headOf(u));
          audio.play("levelup.wav", this.volumeAt(u));
        }
        break;
      }
      case "heal": {
        const u = this.units.get(p.id);
        if (u) {
          if (p.amount > 0) this.floats.spawn(`+${p.amount}`, "heal", this.headOf(u));
          audio.play("se_drink_potion.wav", this.volumeAt(u));
        }
        break;
      }
    }
  }

  private async enterMap(p: EnterMap) {
    this.loading = true;
    this.queue = [];
    this.mapGeneration++;
    this.pendingNpc = null;
    this.hovered = null;
    for (const id of [...this.units.keys()]) this.removeUnit(id);
    this.clearPortals();
    this.clearNpcs();
    this.hud.setDead(false);
    const sameMap = this.map?.name === p.map;
    if (this.map && !sameMap) audio.play("effect/ef_portal.wav"); // warped
    void audio.playMapMusic(p.map);
    if (this.map && !sameMap) {
      this.scene.remove(this.map.root);
      this.map.dispose();
      this.map = null;
    }

    try {
      const [map, assets] = await Promise.all([
        sameMap ? Promise.resolve(this.map!) : loadMap(p.map, (msg) => this.hud.setStatus(msg)),
        loadNoviceAssets(),
      ]);
      if (this.disposed) {
        if (!sameMap) map.dispose();
        return;
      }
      if (!sameMap) {
        this.map = map;
        this.scene.add(map.root);
      }
      this.ambientNext = map.sounds.map(() => 0);
      this.playerAssets = assets;
      this.selfId = p.selfId;
      // The server doesn't echo our own name in EnterMap; the login screen tells us.
      this.addPlayer(p.selfId, this.selfName, p.speed, p.x, p.y, p.dir, false, p.look);

      // Load every monster sprite the map needs before showing them.
      this.hud.setStatus("Loading monsters…");
      const sprites = new Set<string>();
      for (const q of this.queue) if (q.type === "monster") sprites.add(q.sprite);
      await Promise.all([...sprites].map((s) => this.monsterSprite(s)));
      this.hud.setStatus("");
    } catch (err) {
      console.error(err);
      this.hud.setStatus(`Failed to load ${p.map}: ${(err as Error).message}`);
      return;
    } finally {
      this.loading = false;
    }
    const queued = this.queue;
    this.queue = [];
    for (const q of queued) this.handle(q);
  }

  private async monsterSprite(sprite: string): Promise<CharacterAssets | null> {
    const cached = this.monsterAssets.get(sprite);
    if (cached) return cached;
    try {
      const assets = await loadMonsterAssets(sprite);
      this.monsterAssets.set(sprite, assets);
      return assets;
    } catch (err) {
      console.warn("monster sprite failed", sprite, err);
      return null;
    }
  }

  private addPlayer(id: number, name: string, speed: number, x: number, y: number, dir: number, dead: boolean, look: Look) {
    if (!this.playerAssets) return;
    const u = this.addUnit(id, "player", name, speed, this.playerAssets, x, y, dir, id === this.selfId ? "self" : "player");
    u.dead = dead;
    u.look = look;
    this.applyLook(u);
  }

  /** Loads and applies a player's equipment sprites. */
  private applyLook(u: Unit) {
    if (!u.look) return;
    const look = u.look;
    void loadEquipLayers(look, (id) => this.hud.itemSprite(id)).then((layers) => {
      // Skip if the unit left or changed equipment again meanwhile.
      if (this.units.get(u.id) === u && u.look === look) u.sprite.setEquipment(layers);
    });
  }

  /** A snapshot for the minimap, or null while no map is loaded. */
  minimap(): MinimapView | null {
    if (!this.map || this.loading) return null;
    const self = this.units.get(this.selfId);
    const players: [number, number][] = [];
    for (const u of this.units.values()) {
      if (u.kind === "player" && u.id !== this.selfId) players.push([u.x, u.y]);
    }
    return {
      map: this.map.name,
      width: this.map.gat.width,
      height: this.map.gat.height,
      self: self ? { x: self.x, y: self.y, dir: self.dir } : null,
      players,
      npcs: [...this.npcs.values()].map((n) => [n.x, n.y]),
      portals: this.portalCells,
    };
  }

  /** Re-applies every player's equipment, e.g. once item data has loaded. */
  refreshLooks(): void {
    for (const u of this.units.values()) if (u.kind === "player") this.applyLook(u);
  }

  private async addMonster(id: number, name: string, sprite: string, speed: number, x: number, y: number, dir: number) {
    const generation = this.mapGeneration;
    const assets = this.monsterAssets.get(sprite) ?? (await this.monsterSprite(sprite));
    if (!assets || generation !== this.mapGeneration || this.disposed) return;
    this.addUnit(id, "monster", name, speed, assets, x, y, dir, "monster");
  }

  private addUnit(
    id: number,
    kind: Kind,
    name: string,
    speed: number,
    assets: CharacterAssets,
    x: number,
    y: number,
    dir: number,
    labelKind: "self" | "player" | "monster",
  ): Unit {
    this.removeUnit(id);
    const sprite = new CharacterSprite(assets);
    const u = new Unit(id, kind, name, speed, sprite, this.labels.create(name, labelKind), x, y, dir);
    this.units.set(id, u);
    this.scene.add(sprite.object);
    return u;
  }

  private removeUnit(id: number) {
    const u = this.units.get(id);
    if (!u) return;
    this.scene.remove(u.sprite.object);
    u.sprite.dispose();
    u.label.remove();
    this.units.delete(id);
    if (this.hovered === u) this.hovered = null;
  }

  private addPortal(x: number, y: number) {
    if (!this.map) return;
    const portal = new Portal(CELL * 1.6);
    roToWorld((x + 0.5) * CELL, heightAt(this.map.gat, x + 0.5, y + 0.5), (y + 0.5) * CELL, portal.object.position);
    portal.object.position.y += 0.3;
    this.portals.push(portal);
    this.portalCells.push([x + 0.5, y + 0.5]);
    this.scene.add(portal.object);
  }

  private clearPortals() {
    for (const p of this.portals) {
      this.scene.remove(p.object);
      p.dispose();
    }
    this.portals = [];
    this.portalCells = [];
  }

  private async addNpc(id: number, name: string, spriteName: string, x: number, y: number, dir: number) {
    const generation = this.mapGeneration;
    let assets: CharacterAssets;
    try {
      assets = await loadNpcAssets(spriteName);
    } catch (err) {
      console.warn("NPC sprite failed", spriteName, err);
      return;
    }
    if (generation !== this.mapGeneration || this.disposed) return;
    this.removeNpc(id);
    const sprite = new CharacterSprite(assets);
    const npc = new Unit(id, "npc", name, 0, sprite, this.labels.create(name, "npc"), x, y, dir);
    this.npcs.set(id, npc);
    this.scene.add(sprite.object);
  }

  private removeNpc(id: number) {
    const n = this.npcs.get(id);
    if (!n) return;
    this.scene.remove(n.sprite.object);
    n.sprite.dispose();
    n.label.remove();
    this.npcs.delete(id);
  }

  private clearNpcs() {
    for (const id of [...this.npcs.keys()]) this.removeNpc(id);
  }

  /** Talks to an NPC now if close enough, otherwise walks next to it first. */
  private talkTo(npc: Unit) {
    const self = this.units.get(this.selfId);
    if (!self || !this.map) return;
    const sx = Math.floor(self.x), sy = Math.floor(self.y);
    const nx = Math.floor(npc.x), ny = Math.floor(npc.y);
    if (self.walk.length === 0 && chebyshev(sx, sy, nx, ny) <= NPC_RANGE) {
      this.pendingNpc = null;
      this.conn.npcClick(npc.id);
      return;
    }
    // Nearest walkable cell to us within 2 cells of the NPC.
    let best: [number, number] | null = null;
    let bestDist = Infinity;
    for (let dy = -2; dy <= 2; dy++) {
      for (let dx = -2; dx <= 2; dx++) {
        const cx = nx + dx, cy = ny + dy;
        if ((dx === 0 && dy === 0) || !isWalkable(this.map.gat, cx, cy)) continue;
        const d = Math.hypot(cx - sx, cy - sy);
        if (d < bestDist) {
          bestDist = d;
          best = [cx, cy];
        }
      }
    }
    if (!best) return;
    this.pendingNpc = npc.id;
    this.pendingSince = performance.now();
    this.conn.move(best[0], best[1]);
  }

  /** Opens the pending NPC once we're in range; gives up if we stopped short. */
  private updatePendingNpc() {
    if (this.pendingNpc === null) return;
    const self = this.units.get(this.selfId);
    const npc = this.npcs.get(this.pendingNpc);
    if (!self || !npc) {
      this.pendingNpc = null;
      return;
    }
    // One cell of slack: the drawn position can run slightly ahead of the server's.
    const d = chebyshev(Math.floor(self.x), Math.floor(self.y), Math.floor(npc.x), Math.floor(npc.y));
    if (d <= NPC_RANGE - 1) {
      this.pendingNpc = null;
      this.conn.npcClick(npc.id);
    } else if (self.walk.length === 0 && performance.now() - this.pendingSince > 1000) {
      // Not walking a second after setting off: the server found no path.
      this.pendingNpc = null;
    }
  }

  private showDamage(p: Damage) {
    const now = performance.now();
    const src = this.units.get(p.src);
    const target = this.units.get(p.target);
    if (src) src.lastCombat = now;
    if (target) target.lastCombat = now;
    if (src && !src.dead) {
      if (target) src.dir = dirFor(target.x - src.x, target.y - src.y, src.dir);
      const group = src.kind === "monster" ? MonsterAction.Attack : PlayerAction.Attack;
      src.oneShot = { group, start: now, duration: Math.max(p.motion, 250) };
    }
    if (!target) return;
    if (p.hurt > 0 && !target.dead) {
      const group = target.kind === "monster" ? MonsterAction.Hurt : PlayerAction.Hurt;
      target.oneShot = { group, start: now, duration: Math.max(p.hurt, 200) };
    }
    // Show the number as the attack lands, partway through the swing.
    const delay = Math.min(p.motion, 400) / 2;
    const kind = p.miss ? "miss" : p.target === this.selfId ? "hurt" : p.crit ? "crit" : "damage";
    const text = p.miss ? "Miss" : p.crit ? `${p.damage}!` : String(p.damage);
    const hitSound = p.miss ? null : this.hitSound(src);
    setTimeout(() => {
      if (this.disposed) return;
      this.floats.spawn(text, kind, this.headOf(target));
      if (hitSound) audio.play(hitSound, this.volumeAt(target));
    }, delay);
  }

  /** The impact sound for a hit, by attacker: weapon type for players, fists for monsters. */
  private hitSound(src: Unit | undefined): string {
    const n = 1 + Math.floor(Math.random() * 4);
    if (!src || src.kind === "monster") return `_hit_fist${n}.wav`;
    const weapon = src.look?.weapon ?? 0;
    if (weapon >= 1100 && weapon < 1200) return "_hit_sword.wav";
    if (weapon >= 1200 && weapon < 1300) return "_hit_dagger.wav";
    return `_enemy_hit_normal${n}.wav`; // bare hands
  }

  /** Volume for a sound made by a unit, fading with distance from us. */
  private volumeAt(u: Unit): number {
    const self = this.units.get(this.selfId);
    if (!self || u === self) return 1;
    return falloff(chebyshev(u.x, u.y, self.x, self.y));
  }

  /** Plays the map's ambient sounds (wind, water...) near us, each on its own cycle. */
  private updateAmbient(now: number) {
    if (!this.map || now < this.nextAmbientCheck) return;
    this.nextAmbientCheck = now + 250;
    const self = this.units.get(this.selfId);
    if (!self) return;
    const sx = self.x * CELL, sz = self.y * CELL;
    this.map.sounds.forEach((s, i) => {
      const d = Math.hypot(s.x - sx, s.z - sz);
      if (d > s.range || now < this.ambientNext[i]) return;
      this.ambientNext[i] = now + Math.max(s.cycle, 1) * 1000;
      audio.play(s.file, Math.min(1, s.volume) * (1 - d / s.range));
    });
  }

  /** A world-space anchor above a unit's head that follows it. */
  private headOf(u: Unit): () => THREE.Vector3 {
    let last = new THREE.Vector3();
    return () => {
      if (!this.map) return last;
      const feet = roToWorld(u.x * CELL, heightAt(this.map.gat, u.x, u.y), u.y * CELL);
      const up = new THREE.Vector3(0, HEAD_HEIGHT * SPRITE_SCALE, 0).applyQuaternion(this.camera.quaternion);
      last = feet.add(up);
      return last;
    };
  }

  private startWalk(id: number, fromX: number, fromY: number, steps: [number, number][]) {
    const u = this.units.get(id);
    if (!u || steps.length === 0 || u.dyingSince) return;
    const now = performance.now();
    const stepMs = (ax: number, ay: number, bx: number, by: number) =>
      u.speed * (ax !== bx && ay !== by ? DIAGONAL : 1);

    // Continue from where the sprite is drawn (it may be mid-step) to avoid snapping.
    const walking = u.walk.length > 0;
    let sx = walking ? u.x : fromX + 0.5;
    let sy = walking ? u.y : fromY + 0.5;
    const first = steps[0];
    const full = Math.hypot(first[0] - fromX, first[1] - fromY) || 1;
    const left = Math.hypot(first[0] + 0.5 - sx, first[1] + 0.5 - sy);
    let t = now;
    const segs: Segment[] = [];
    let prev: [number, number] = [fromX, fromY];
    for (let i = 0; i < steps.length; i++) {
      const [cx, cy] = steps[i];
      let dur = stepMs(prev[0], prev[1], cx, cy);
      if (i === 0) dur *= Math.min(left / full, 1.5);
      segs.push({ fx: sx, fy: sy, tx: cx + 0.5, ty: cy + 0.5, t0: t, dur });
      t += dur;
      sx = cx + 0.5;
      sy = cy + 0.5;
      prev = [cx, cy];
    }
    if (!walking) u.walkStart = now;
    u.walk = segs;
    u.oneShot = null;
  }

  private updateUnit(u: Unit, now: number) {
    while (u.walk.length && now >= u.walk[0].t0 + u.walk[0].dur) {
      const s = u.walk.shift()!;
      u.x = s.tx;
      u.y = s.ty;
      u.dir = dirFor(s.tx - s.fx, s.ty - s.fy, u.dir);
    }
    if (u.walk.length) {
      const s = u.walk[0];
      const k = Math.max(0, (now - s.t0) / s.dur);
      u.x = s.fx + (s.tx - s.fx) * k;
      u.y = s.fy + (s.ty - s.fy) * k;
      u.dir = dirFor(s.tx - s.fx, s.ty - s.fy, u.dir);
    }

    const [group, frame] = this.animationFrame(u, now);
    const dir = this.spriteDir(u.dir);
    u.sprite.draw(group, dir, frame);
    // Frames can carry a sound (footsteps, swings, cries); "atk" only marks the hit.
    const key = `${group}:${frame}`;
    if (key !== u.lastFrameKey) {
      u.lastFrameKey = key;
      const sound = u.sprite.frameSound(group, dir, frame);
      if (sound && sound !== "atk") {
        const volume = this.volumeAt(u);
        if (volume > 0) audio.play(sound, volume);
      }
    }
    this.placeSprite(u.sprite, u.label, u.x, u.y);
  }

  /** Which action group and frame a unit shows right now. */
  private animationFrame(u: Unit, now: number): [number, number] {
    const s = u.sprite;
    const monster = u.kind === "monster";
    if (u.dyingSince) {
      const n = s.frameCount(MonsterAction.Die);
      return [MonsterAction.Die, Math.min(n - 1, Math.floor((now - u.dyingSince) / s.frameDelay(MonsterAction.Die)))];
    }
    if (u.dead) {
      return [PlayerAction.Dead, s.frameCount(PlayerAction.Dead) - 1];
    }
    if (u.walk.length) {
      const group = monster ? MonsterAction.Walk : PlayerAction.Walk;
      const delay = (s.frameDelay(group) * u.speed) / 150;
      return [group, Math.floor((now - u.walkStart) / delay)];
    }
    if (u.oneShot) {
      const { group, start, duration } = u.oneShot;
      if (now - start < duration) {
        const n = s.frameCount(group);
        return [group, Math.min(n - 1, Math.floor(((now - start) / duration) * n))];
      }
      u.oneShot = null;
    }
    // Players who just fought stay in battle stance, with weapon and shield out.
    if (!monster && now - u.lastCombat < COMBAT_STANCE_MS) {
      return [PlayerAction.Ready, Math.floor((now - u.lastCombat) / s.frameDelay(PlayerAction.Ready))];
    }
    // Monsters breathe/bob while idle; players hold the first idle frame (head straight).
    return [0, monster ? Math.floor((now - u.bornAt) / s.frameDelay(0)) : 0];
  }

  /** Positions a billboard and its name label at fractional cell (x, y). */
  private placeSprite(sprite: CharacterSprite, label: HTMLElement, x: number, y: number) {
    const feet = roToWorld(x * CELL, heightAt(this.map!.gat, x, y), y * CELL);
    // Anchor the name just below the sprite's shadow, measured along the screen's "down".
    const below = new THREE.Vector3(0, -LABEL_DROP * SPRITE_SCALE, 0).applyQuaternion(this.camera.quaternion);
    this.labels.place(label, below.add(feet), this.camera);
    // Pull the billboard towards the camera so it isn't cut by the ground or walls it stands next to.
    const toCam = this.camera.position.clone().sub(feet).normalize().multiplyScalar(8);
    sprite.object.position.copy(feet).add(toCam);
    sprite.object.quaternion.copy(this.camera.quaternion);
  }

  private spriteDir(serverDir: number): number {
    const camSteps = Math.round(this.yaw / (Math.PI / 4));
    return (((4 - serverDir + camSteps) % 8) + 8) % 8;
  }

  private render(now: number) {
    if (this.map) {
      this.map.update(now);
      for (const p of this.portals) p.update(now);
      const self = this.units.get(this.selfId);
      if (self) {
        roToWorld(self.x * CELL, heightAt(this.map.gat, self.x, self.y), self.y * CELL, this.target);
        this.hud.setPosition(this.map.name, Math.floor(self.x), Math.floor(self.y));
      }
      const c = Math.cos(PITCH) * this.distance;
      this.camera.position.set(
        this.target.x + Math.sin(this.yaw) * c,
        this.target.y + Math.sin(PITCH) * this.distance,
        this.target.z + Math.cos(this.yaw) * c,
      );
      this.camera.lookAt(this.target);
      this.camera.updateMatrixWorld();
      for (const u of [...this.units.values()]) {
        // Remove monsters once their death animation has played out.
        if (u.dyingSince && now - u.dyingSince > Math.max(u.sprite.duration(MonsterAction.Die), 400) + 300) {
          this.removeUnit(u.id);
          continue;
        }
        this.updateUnit(u, now);
      }
      for (const n of this.npcs.values()) {
        n.sprite.draw(PlayerAction.Idle, this.spriteDir(n.dir), 0);
        this.placeSprite(n.sprite, n.label, n.x, n.y);
      }
      this.updatePendingNpc();
      this.updateAmbient(now);
      this.floats.update(this.camera, now);
    }
    this.renderer.render(this.scene, this.camera);
  }

  private ray(ev: MouseEvent) {
    const r = this.canvas.getBoundingClientRect();
    const ndc = new THREE.Vector2(((ev.clientX - r.left) / r.width) * 2 - 1, -((ev.clientY - r.top) / r.height) * 2 + 1);
    this.raycaster.setFromCamera(ndc, this.camera);
  }

  /** Map cell under the mouse, or null. */
  private pick(ev: MouseEvent): [number, number] | null {
    if (!this.map) return null;
    this.ray(ev);
    const hit = this.raycaster.intersectObjects(this.map.ground, false)[0];
    if (!hit) return null;
    const [x, , z] = worldToRo(hit.point);
    const cx = Math.floor(x / CELL), cy = Math.floor(z / CELL);
    if (cx < 0 || cy < 0 || cx >= this.map.gat.width || cy >= this.map.gat.height) return null;
    return [cx, cy];
  }

  /** The NPC or live monster whose visible pixels are under the mouse, nearest first. */
  private pickUnit(ev: MouseEvent): Unit | null {
    const candidates = [
      ...this.npcs.values(),
      ...[...this.units.values()].filter((u) => u.kind === "monster" && !u.dyingSince),
    ];
    if (candidates.length === 0) return null;
    this.ray(ev);
    const byObject = new Map(candidates.map((u) => [u.sprite.object as THREE.Object3D, u]));
    for (const hit of this.raycaster.intersectObjects([...byObject.keys()], false)) {
      const u = byObject.get(hit.object);
      if (u && hit.uv && u.sprite.hitTest(hit.uv)) return u;
    }
    return null;
  }

  private setHovered(u: Unit | null) {
    if (this.hovered === u) return;
    this.hovered?.label.classList.remove("hover");
    this.hovered = u;
    u?.label.classList.add("hover");
    this.canvas.style.cursor = !u ? "" : u.kind === "monster" ? "crosshair" : "pointer";
  }

  private updateCursor(cell: [number, number] | null) {
    if (!cell || !this.map) {
      this.cursor.visible = false;
      return;
    }
    const [cx, cy] = cell;
    const h = heightAt(this.map.gat, cx + 0.5, cy + 0.5);
    roToWorld((cx + 0.5) * CELL, h, (cy + 0.5) * CELL, this.cursor.position);
    this.cursor.position.y += 0.3;
    const walkable = isWalkable(this.map.gat, cx, cy);
    (this.cursor.material as THREE.MeshBasicMaterial).color.set(walkable ? 0xffffff : 0xff5050);
    this.cursor.visible = true;
  }

  private bindInput() {
    const on = <K extends keyof HTMLElementEventMap>(
      el: HTMLElement | Window,
      type: K,
      fn: (ev: HTMLElementEventMap[K]) => void,
      opts?: AddEventListenerOptions,
    ) => {
      el.addEventListener(type, fn as EventListener, opts);
      this.cleanup.push(() => el.removeEventListener(type, fn as EventListener));
    };

    let rotating = false;
    let lastX = 0;
    on(this.canvas, "contextmenu", (ev) => ev.preventDefault());
    on(this.canvas, "mousedown", (ev) => {
      if (ev.button === 2) {
        rotating = true;
        lastX = ev.clientX;
        return;
      }
      if (ev.button !== 0 || !this.conn.connected) return;
      const self = this.units.get(this.selfId);
      if (!self || self.dead) return;
      const u = this.pickUnit(ev);
      if (u?.kind === "npc") {
        this.talkTo(u);
        return;
      }
      this.pendingNpc = null;
      if (u?.kind === "monster") {
        this.conn.attack(u.id);
        return;
      }
      const cell = this.pick(ev);
      if (cell) this.conn.move(cell[0], cell[1]);
    });
    on(window, "mouseup", () => (rotating = false));
    on(this.canvas, "mousemove", (ev) => {
      if (rotating) {
        this.yaw -= (ev.clientX - lastX) * 0.008;
        lastX = ev.clientX;
      }
      const u = rotating ? null : this.pickUnit(ev);
      this.setHovered(u);
      this.updateCursor(u ? null : this.pick(ev));
    });
    on(this.canvas, "mouseleave", () => {
      this.updateCursor(null);
      this.setHovered(null);
    });
    on(
      this.canvas,
      "wheel",
      (ev) => {
        ev.preventDefault();
        this.distance = THREE.MathUtils.clamp(this.distance * (ev.deltaY > 0 ? 1.1 : 0.9), 80, 600);
      },
      { passive: false },
    );
    on(window, "resize", () => this.resize());
  }

  private resize() {
    const w = this.canvas.clientWidth, h = this.canvas.clientHeight;
    this.renderer.setSize(w, h, false);
    this.camera.aspect = w / Math.max(h, 1);
    this.camera.updateProjectionMatrix();
  }

  dispose(): void {
    this.disposed = true;
    cancelAnimationFrame(this.frame);
    for (const f of this.cleanup) f();
    for (const id of [...this.units.keys()]) this.removeUnit(id);
    this.clearPortals();
    this.clearNpcs();
    if (this.map) {
      this.scene.remove(this.map.root);
      this.map.dispose();
    }
    this.labels.dispose();
    this.floats.dispose();
    this.renderer.dispose();
  }
}

/** Server facing for a step (north is +y), keeping `fallback` when not moving. */
function dirFor(dx: number, dy: number, fallback: number): number {
  const sx = Math.sign(Math.round(dx * 1000)), sy = Math.sign(Math.round(dy * 1000));
  if (sx === 0 && sy === 0) return fallback;
  const table: Record<string, number> = {
    "0,1": 0, "-1,1": 1, "-1,0": 2, "-1,-1": 3, "0,-1": 4, "1,-1": 5, "1,0": 6, "1,1": 7,
  };
  return table[`${sx},${sy}`];
}

function chebyshev(ax: number, ay: number, bx: number, by: number): number {
  return Math.max(Math.abs(ax - bx), Math.abs(ay - by));
}
