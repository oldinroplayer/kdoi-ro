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
  loadPlayerAssets,
  loadNpcAssets,
  MonsterAction,
  PlayerAction,
  SPRITE_SCALE,
  type CharacterAssets,
} from "./render/character";
import { FloatingTexts, NameLabels } from "./render/labels";
import { CELL, loadMap, roToWorld, worldToRo, type MapView } from "./render/map";
import { HitRing, loadRingTexture, type Effect } from "./render/effect";
import {
  CastCircle,
  LightColumn,
  loadStr,
  castTextures,
  RingCylinder,
  RisingImage,
  SpriteEffect,
  StrEffect,
  type Anchor,
} from "./render/skilleffects";
import { loadEffectAssets, loadItemAssets, playerAttackGroup } from "./render/character";
import { ArrowStreak, FadeColumn, FireBlast, FlashCross, RisingStreaks } from "./render/rofx";
import { loadTexture } from "./render/textures";
import { Skill } from "./skills";
import { Portal } from "./render/portal";

type EnterMap = Extract<ServerPacket, { type: "enterMap" }>;

/** What the minimap draws: positions in map cells (y = 0 at the south edge). */
export interface MinimapView {
  map: string;
  width: number;
  height: number;
  self: { x: number; y: number; dir: number } | null;
  players: [number, number][];
  /** Party members among the players. */
  party: [number, number][];
  npcs: [number, number][];
  portals: [number, number][];
}
type Damage = Extract<ServerPacket, { type: "damage" }>;
type SkillUse = Extract<ServerPacket, { type: "skillUse" }>;

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
  /** Players sitting on the ground (regenerating faster). */
  sitting = false;
  /** When a monster's death animation started (it's removed afterwards). */
  dyingSince = 0;
  /** Last time this unit hit or was hit; players hold a battle stance for a while. */
  lastCombat = 0;
  /** Visible equipment (players only). */
  look: Look | null = null;
  /** RO class ID (players only). */
  job = 0;
  /** Cast bar while casting a skill. */
  cast: { el: HTMLElement; start: number; ms: number } | null = null;
  /** Magic circle under the unit while it casts. */
  castCircle: CastCircle | null = null;
  /** What the unit last said, shown over its head for a few seconds. */
  speech: { el: HTMLElement; until: number } | null = null;
  /** Last animation frame shown, so each frame's sound plays once. */
  lastFrameKey = "";
  readonly bornAt = performance.now();

  constructor(
    readonly id: number,
    readonly kind: Kind,
    readonly name: string,
    /** ms per straight cell; changes with buffs like Increase AGI. */
    public speed: number,
    /** Replaced when a player changes job. */
    public sprite: CharacterSprite,
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
  /** A short notice for the player. */
  notice(text: string): void;
  /** A skill's element (for the colour of its magic circle). */
  skillElement(skillId: number): number;
  /** The player clicked a chat room's sign. */
  joinRoom(roomId: number, isPublic: boolean, title: string): void;
  /** An item's display and resource names, from the item database. */
  itemInfo(itemId: number): { name: string; resName: string } | undefined;
}

/** An item lying on the ground. */
interface FloorItem {
  id: number;
  /** Fractional cell position (sub-cell offset included). */
  x: number;
  y: number;
  label: HTMLElement;
  sprite: CharacterSprite | null; // null until its sprite has loaded
  /** When it fell (for the drop bounce); 0 if it was already lying there. */
  droppedAt: number;
}

/** How long a dropped item takes to bounce to rest (ms). */
const DROP_MS = 500;

/** A chat room sign over its owner's head. */
interface RoomSign {
  el: HTMLElement;
  ownerId: number;
  public: boolean;
  title: string;
}

/** How long a speech bubble stays up (ms). */
const SPEECH_MS = 5000;

/** What the auto-attack bot sees (see bot.ts). Cells are whole map cells. */
export interface BotView {
  map: string;
  self: { x: number; y: number; dead: boolean; sitting: boolean; walking: boolean };
  /** Live monsters on the map. */
  monsters: { id: number; x: number; y: number }[];
  /** Items lying on the ground. */
  items: { id: number; x: number; y: number }[];
  /** Warp portals' centre cells. */
  warps: { x: number; y: number }[];
  /** Our last swing (hit or miss) at a monster, and when (performance.now()). */
  lastSwing: { target: number; at: number };
}

/** A skill waiting for the player to click its target. */
interface Targeting {
  skillId: number;
  kind: "enemy" | "friend";
  name: string;
}

/** How long players stay in battle stance (weapon drawn) after fighting. */
const COMBAT_STANCE_MS = 5000;

const DIAGONAL = 1.4;
/** Sprite pixels from the feet down to where the name label starts (below the shadow). */
const LABEL_DROP = 10;
/** Sprite pixels from the feet up to where damage numbers appear. */
const HEAD_HEIGHT = 75;
/** Sprite pixels from the feet up to the middle of a body (where hits land). */
const BODY_HEIGHT = 30;
const PITCH = THREE.MathUtils.degToRad(50);
/** Backdrop beyond the map edges outdoors. */
const BACKGROUND = 0x0e0d12;
/** Dungeon tint over the fog table's values: a little stronger and a darker
 * shade of its colour, for the gloom of the cave. */
const DUNGEON_TINT_STRENGTH = 1.5;
const DUNGEON_TINT_SHADE = 0.6;
/** Sprite pixels above the head where a multi-hit attack's yellow total shows. */
const COMBO_RAISE = 28;
/** msg.spr's frame with the red starburst critical hits are drawn on. */
const CRIT_BURST_FRAME = 3;
/** Played on top of the hit sound for critical hits. The 2004 client has no
 * sound of its own for them, so this is a heavier impact from its effects. */
const CRIT_SOUND = "effect/ef_hit2.wav";
/** Sprites for effects placed on maps, by the client's effect ID. */
const MAP_EFFECT_SPRITES: Record<number, string> = {
  47: "torch_01", // torch fire (Payon Cave braziers)
};
/** How quickly sprites take on the light of a new cell (per frame). */
const SPRITE_LIGHT_EASE = 0.15;
const cellLight = new THREE.Color();

export class Game {
  /** Our character's name (EnterMap doesn't carry it); set by the login screen. */
  selfName = "";
  private readonly renderer: THREE.WebGLRenderer;
  private readonly scene = new THREE.Scene();
  private readonly camera = new THREE.PerspectiveCamera(30, 1, 5, 4000);
  /** Covers the view in a dungeon's fog colour (Payon Cave's dark blue-violet). */
  private readonly dungeonTint = createScreenTint();
  private readonly raycaster = new THREE.Raycaster();
  private readonly cursor: THREE.Mesh;
  private readonly labels: NameLabels;
  private readonly floats: FloatingTexts;
  private map: MapView | null = null;
  /** Player body sprites already loaded, by job. */
  private readonly playerAssets = new Map<string, CharacterAssets>();
  /** Monster sprites already loaded, by sprite name. */
  private readonly monsterAssets = new Map<string, CharacterAssets>();
  private readonly units = new Map<number, Unit>();
  private readonly npcs = new Map<number, Unit>();
  private portals: Portal[] = [];
  private effects: Effect[] = [];
  private portalCells: [number, number][] = [];
  /** Bumped on every map change, so late-loading sprites for an old map are dropped. */
  private mapGeneration = 0;
  /** We just teleported with a wing: show the light around us once we arrive. */
  private teleportArrival = false;
  /** NPC we're walking towards to talk to, and when we set off. */
  private pendingNpc: number | null = null;
  private pendingSince = 0;
  private hovered: Unit | null = null;
  private targeting: Targeting | null = null;
  /** Lowercase names of our party members (pink on the minimap). */
  private partyMembers = new Set<string>();
  private readonly signs = new Map<number, RoomSign>();
  private readonly floorItems = new Map<number, FloorItem>();
  private hoveredItem: FloorItem | null = null;
  /** When each of the map's ambient sounds may play next. */
  private ambientNext: number[] = [];
  private nextAmbientCheck = 0;
  private selfId = 0;
  /** Our last attack on a monster, for the bot to see the fight is on. */
  private lastSwing = { target: 0, at: 0 };
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
    // Critical hits show their number on the client's red starburst (msg.spr frame 3).
    void loadEffectAssets("msg")
      .then((msg) => {
        const burst = msg.body.indexed[CRIT_BURST_FRAME];
        if (burst && !this.disposed) this.floats.setCritBurst(burst.toDataURL());
      })
      .catch((err) => console.warn("critical burst missing", err));
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    this.scene.background = new THREE.Color(BACKGROUND);

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
        void this.addPlayer(p.id, p.name, p.job, p.speed, p.x, p.y, p.dir, p.dead, p.look, p.sitting);
        break;
      case "skillCast": {
        const u = this.units.get(p.src);
        if (!u) break;
        u.cast?.el.remove();
        u.cast = null;
        u.castCircle?.stop();
        u.castCircle = null;
        if (p.skillId && p.castMs > 0) {
          u.cast = { el: this.labels.createBar(), start: performance.now(), ms: p.castMs };
          void this.playCastCircle(u, this.hud.skillElement(p.skillId), p.castMs);
          audio.play("effect/ef_beginspell.wav", this.volumeAt(u));
        }
        break;
      }
      case "skillUse": {
        const u = this.units.get(p.src);
        if (!u) break;
        u.cast?.el.remove();
        u.cast = null;
        u.castCircle?.stop();
        u.castCircle = null;
        u.lastCombat = performance.now(); // skill users drop into battle stance
        this.playSkillUse(p);
        break;
      }
      case "turn": {
        const u = this.units.get(p.id);
        if (u && !u.walk.length) u.dir = p.dir;
        break;
      }
      case "sit": {
        const u = this.units.get(p.id);
        if (u) {
          u.sitting = p.sitting;
          u.walk = [];
          u.oneShot = null;
        }
        break;
      }
      case "job": {
        const u = this.units.get(p.id);
        if (u && u.kind === "player") void this.setJob(u, p.job);
        break;
      }
      case "look": {
        const u = this.units.get(p.id);
        if (u) {
          const genderChanged = (u.look?.female ?? false) !== p.look.female;
          u.look = p.look;
          if (genderChanged && u.kind === "player") void this.setJob(u, u.job); // new body
          else this.applyLook(u);
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
      case "roomSign":
        this.updateSign(p);
        break;
      case "floorItem":
        void this.addFloorItem(p);
        break;
      case "floorItemGone":
        this.removeFloorItem(p.id);
        break;
      case "speed": {
        const u = this.units.get(p.id);
        if (u) this.setSpeed(u, p.speed, performance.now());
        break;
      }
      case "effect": {
        const u = this.units.get(p.id);
        if (!u || p.kind !== 1) break;
        // Teleporting (wings): RO's blue light (ring_blue) and ef_teleportation.
        // Others see it where the unit vanishes; we see it around us on arrival.
        audio.play("effect/ef_teleportation.wav", this.volumeAt(u));
        if (p.id === this.selfId) {
          this.teleportArrival = true;
        } else {
          const at = this.anchorOf(u)(); // a fixed spot: the unit is about to vanish
          this.playTeleportLight(() => at);
        }
        break;
      }
      case "emotion": {
        const u = this.units.get(p.id);
        if (u) {
          void this.whenLoaded(
            loadEffectAssets("emotion"),
            (assets) => new SpriteEffect(assets, this.anchorOf(u, HEAD_HEIGHT + 10), p.kind),
          );
        }
        break;
      }
      case "pickUp": {
        // RO's pick-up motion (body action 3: bending down to the ground).
        const u = this.units.get(p.id);
        if (u && !u.dead) {
          const duration = Math.max(u.sprite.duration(PlayerAction.PickUp), 200);
          u.oneShot = { group: PlayerAction.PickUp, start: performance.now(), duration };
        }
        break;
      }
      case "roomGone":
        this.signs.get(p.roomId)?.el.remove();
        this.signs.delete(p.roomId);
        break;
      case "dead": {
        const u = this.units.get(p.id);
        if (u) {
          u.dead = true;
          u.sitting = false;
          u.walk = [];
          u.oneShot = null;
        }
        if (p.id === this.selfId) this.hud.setDead(true);
        break;
      }
      case "levelUp": {
        const u = this.units.get(p.id);
        if (u) this.playLevelUp(u, p.kind);
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
    this.cancelTargeting();
    this.clearSigns(); // the server re-sends the new map's rooms
    this.clearFloorItems(); // ... and what lies on its ground
    this.pendingNpc = null;
    this.hovered = null;
    for (const id of [...this.units.keys()]) this.removeUnit(id);
    this.clearPortals();
    this.clearNpcs();
    this.clearEffects();
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
      // Every job on the map (ours included) needs its body sprite first.
      const jobs = new Set([p.job]);
      for (const q of this.queue) if (q.type === "appear") jobs.add(q.job);
      const [map] = await Promise.all([
        sameMap ? Promise.resolve(this.map!) : loadMap(p.map, (msg) => this.hud.setStatus(msg)),
        ...[...jobs].map((j) => this.jobSprite(j)),
      ]);
      if (this.disposed) {
        if (!sameMap) map.dispose();
        return;
      }
      if (!sameMap) {
        this.map = map;
        // Dungeons sit in darkness; outdoors keeps the usual backdrop.
        (this.scene.background as THREE.Color).set(isDungeon(map.name) ? 0x000000 : BACKGROUND);
        this.scene.add(map.root);
        this.setDungeonTint(map);
      }
      this.ambientNext = map.sounds.map(() => 0);
      this.addMapEffects(map);
      this.selfId = p.selfId;
      // The server doesn't echo our own name in EnterMap; the login screen tells us.
      await this.addPlayer(p.selfId, this.selfName, p.job, p.speed, p.x, p.y, p.dir, false, p.look, false);
      if (this.teleportArrival) {
        this.teleportArrival = false;
        const self = this.units.get(this.selfId);
        if (self) this.playTeleportLight(this.anchorOf(self));
      }

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

  private async jobSprite(job: number, female = false): Promise<CharacterAssets> {
    const key = `${job}:${female}`;
    const cached = this.playerAssets.get(key);
    if (cached) return cached;
    const assets = await loadPlayerAssets(job, female);
    this.playerAssets.set(key, assets);
    return assets;
  }

  private async addPlayer(
    id: number,
    name: string,
    job: number,
    speed: number,
    x: number,
    y: number,
    dir: number,
    dead: boolean,
    look: Look,
    sitting: boolean,
  ) {
    const generation = this.mapGeneration;
    const assets = this.playerAssets.get(`${job}:${look.female}`) ?? (await this.jobSprite(job, look.female));
    if (generation !== this.mapGeneration || this.disposed) return;
    const u = this.addUnit(id, "player", name, speed, assets, x, y, dir, id === this.selfId ? "self" : "player");
    u.job = job;
    u.dead = dead;
    u.sitting = sitting;
    u.look = look;
    this.applyLook(u);
  }

  /** Swaps a player's body sprite after a job or gender change, keeping everything else. */
  private async setJob(u: Unit, job: number) {
    const assets = await this.jobSprite(job, u.look?.female ?? false);
    if (this.units.get(u.id) !== u) return;
    u.job = job;
    this.scene.remove(u.sprite.object);
    u.sprite.dispose();
    u.sprite = new CharacterSprite(assets);
    this.scene.add(u.sprite.object);
    u.lastFrameKey = "";
    this.applyLook(u); // weapons and shields are per job too
  }

  /** Loads and applies a player's equipment sprites. */
  private applyLook(u: Unit) {
    if (!u.look) return;
    const look = u.look;
    void loadEquipLayers(look, (id) => this.hud.itemSprite(id), u.job).then((layers) => {
      // Skip if the unit left or changed equipment again meanwhile.
      if (this.units.get(u.id) === u && u.look === look) u.sprite.setEquipment(layers);
    });
  }

  /** A snapshot for the minimap, or null while no map is loaded. */
  minimap(): MinimapView | null {
    if (!this.map || this.loading) return null;
    const self = this.units.get(this.selfId);
    const players: [number, number][] = [];
    const party: [number, number][] = [];
    for (const u of this.units.values()) {
      if (u.kind !== "player" || u.id === this.selfId) continue;
      (this.partyMembers.has(u.name.toLowerCase()) ? party : players).push([u.x, u.y]);
    }
    return {
      map: this.map.name,
      width: this.map.gat.width,
      height: this.map.gat.height,
      self: self ? { x: self.x, y: self.y, dir: self.dir } : null,
      players,
      party,
      npcs: [...this.npcs.values()].map((n) => [n.x, n.y]),
      portals: this.portalCells,
    };
  }

  /** Sits down or stands up (RO's Insert key). */
  /** A snapshot for the auto-attack bot, or null while no map is shown. */
  botView(): BotView | null {
    const self = this.units.get(this.selfId);
    if (!self || !this.map || this.loading) return null;
    const monsters = [...this.units.values()]
      .filter((u) => u.kind === "monster" && !u.dead && !u.dyingSince)
      .map((u) => ({ id: u.id, x: Math.floor(u.x), y: Math.floor(u.y) }));
    return {
      map: this.map.name,
      self: { x: Math.floor(self.x), y: Math.floor(self.y), dead: self.dead, sitting: self.sitting, walking: self.walk.length > 0 },
      monsters,
      items: [...this.floorItems.values()].map((it) => ({ id: it.id, x: Math.floor(it.x), y: Math.floor(it.y) })),
      warps: this.portalCells.map(([x, y]) => ({ x: Math.floor(x), y: Math.floor(y) })),
      lastSwing: this.lastSwing,
    };
  }

  /** Our party's member names (empty set: no party). */
  setPartyMembers(names: string[]): void {
    this.partyMembers = new Set(names.map((n) => n.toLowerCase()));
  }

  /** Whether a cell can be walked on (for picking wander destinations). */
  walkable(x: number, y: number): boolean {
    return !!this.map && isWalkable(this.map.gat, x, y);
  }

  toggleSit(): void {
    const self = this.units.get(this.selfId);
    if (!self || self.dead || !this.conn.connected) return;
    this.conn.sit(!self.sitting);
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
    u.cast?.el.remove();
    u.castCircle?.stop();
    u.speech?.el.remove();
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
    if (self.sitting) {
      this.hud.notice("You're sitting. Press Insert or the Sit button to stand up.");
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
    if (p.src === this.selfId) this.lastSwing = { target: p.target, at: now };
    // Magic (motion 0) is cast, not swung: the caster only turns to face.
    const magic = p.motion === 0;
    if (src && !src.dead) {
      if (target) src.dir = dirFor(target.x - src.x, target.y - src.y, src.dir);
      if (!magic) src.oneShot = { group: this.attackGroup(src), start: now, duration: Math.max(p.motion, 250) };
    }
    if (!target) return;
    if (p.hurt > 0 && !target.dead) {
      const group = target.kind === "monster" ? MonsterAction.Hurt : PlayerAction.Hurt;
      target.oneShot = { group, start: now, duration: Math.max(p.hurt, 200) };
    }
    // Show the number as the attack lands, partway through the swing. Damage
    // split over several blows (bolts, Double Strafe, Double Attack) shows one
    // number per blow.
    const hits = Math.max(1, p.hits);
    const first = magic ? 150 : Math.min(p.motion, 400) / 2;
    const spacing = magic ? 200 : 150;
    const each = Math.floor(p.damage / hits);
    const kind = p.miss ? "miss" : p.target === this.selfId ? "hurt" : p.crit ? "crit" : "damage";
    // Bows shoot an arrow per blow, landing as its number shows.
    if (!magic && src && this.shootsArrows(src)) {
      const flight = Math.max(first * 0.6, 90);
      for (let i = 0; i < hits; i++) this.playArrow(src, target, flight, first - flight + i * spacing);
    }
    // Several blows on a monster also count up a yellow total over its head,
    // as the client does for Double Attack and multi-hit skills.
    const combo = hits > 1 && !p.miss && p.damage > 0 && target.kind === "monster";
    const comboAnchor = combo ? this.anchorOf(target, HEAD_HEIGHT + COMBO_RAISE) : null;
    let comboEl: HTMLElement | undefined;
    for (let i = 0; i < hits; i++) {
      const amount = i === hits - 1 ? p.damage - each * (hits - 1) : each;
      const text = p.miss ? "Miss" : String(amount);
      const runningTotal = i === hits - 1 ? p.damage : each * (i + 1);
      setTimeout(() => {
        if (this.disposed) return;
        this.floats.spawn(text, kind, this.headOf(target));
        if (comboAnchor) comboEl = this.floats.combo(runningTotal, comboAnchor, i === hits - 1, comboEl);
        if (!p.miss) this.playHit(p.skillId, i, src, target);
        // A critical lands harder: an extra heavy impact over the weapon's hit.
        if (p.crit && i === 0) audio.play(CRIT_SOUND, this.volumeAt(target));
      }, first + i * spacing);
    }
  }

  /** The blue light of a teleport (texture/effect/ring_blue.tga). */
  private playTeleportLight(anchor: Anchor) {
    void this.whenLoaded(loadTexture("texture/effect/ring_blue.tga"), (tex) => new RingCylinder(tex, anchor));
  }

  /** Whether a unit attacks with a bow (players holding one). */
  private shootsArrows(u: Unit): boolean {
    const weapon = u.kind === "player" ? (u.look?.weapon ?? 0) : 0;
    return weapon >= 1700 && weapon < 1750;
  }

  /** One arrow streak (texture/effect/allow_a.bmp) from a unit's chest to the target's. */
  private playArrow(src: Unit, target: Unit, ms: number, delay: number) {
    void this.whenLoaded(
      loadTexture("texture/effect/allow_a.bmp"),
      (tex) => new ArrowStreak(tex, this.anchorOf(src, BODY_HEIGHT), this.anchorOf(target, BODY_HEIGHT), ms, Math.max(delay, 0)),
    );
  }

  /** The sound and effect of one blow landing. */
  private playHit(skillId: number, i: number, src: Unit | undefined, target: Unit) {
    const n = (i % 3) + 1;
    const volume = this.volumeAt(target);
    const bolt = (str: string, sound: string) => {
      void this.playStr(str, target, BODY_HEIGHT);
      audio.play(`effect/${sound}${n}.wav`, volume);
    };
    switch (skillId) {
      case Skill.FireBolt:
        return bolt("firehit1", "ef_firearrow");
      case Skill.ColdBolt:
        return bolt("icecrash", "ef_icearrow");
      case Skill.LightningBolt:
        return bolt("lightning", "ef_lightbolt");
      case Skill.Envenom:
        void this.playStr("invenom", target, BODY_HEIGHT);
        audio.play("effect/thief_invenom.wav", volume);
        break;
      case Skill.Bash:
        // RO draws Bash as a white flash across the target (alpha_center.tga).
        void this.whenLoaded(loadTexture("texture/effect/alpha_center.tga"), (tex) => new FlashCross(tex, this.anchorOf(target, BODY_HEIGHT), 0xfff4d0));
        audio.play("effect/ef_bash.wav", volume);
        break;
      case Skill.Mammonite:
        // A hand grabbing coins out of a purse, with coins spilling (maemor.str).
        void this.playStr("maemor", target, BODY_HEIGHT);
        audio.play("effect/ef_coin2.wav", volume);
        break;
      default:
        audio.play(this.hitSound(src), volume);
    }
    void this.playHitRing(target);
  }

  /** Effects shown when a skill goes off (attack skills also show their hits). */
  private playSkillUse(p: SkillUse) {
    const src = this.units.get(p.src);
    const target = this.units.get(p.target) ?? src;
    if (!src || !target) return;
    const volume = this.volumeAt(target);
    switch (p.skillId) {
      case Skill.MagnumBreak:
        // A blast of fire bursting out around the caster (대폭발.tga).
        if (!src.dead) src.oneShot = { group: this.attackGroup(src), start: performance.now(), duration: 400 };
        void this.whenLoaded(loadTexture("texture/effect/대폭발.tga"), (tex) => new FireBlast(tex, this.anchorOf(src)));
        audio.play("effect/ef_magnumbreak.wav", this.volumeAt(src));
        break;
      case Skill.Heal:
        // A column of light on the target, bright at its feet (alpha_down.tga).
        void this.whenLoaded(loadTexture("texture/effect/alpha_down.tga"), (tex) => new FadeColumn(tex, this.anchorOf(target), 0xc8ffd4));
        audio.play("_heal_effect.wav", volume);
        break;
      case Skill.IncAgi:
        // Blue streaks of light shooting up around the target (ac_center2.tga), with "AGI Up" rising out.
        void this.whenLoaded(loadTexture("texture/effect/ac_center2.tga"), (tex) => new RisingStreaks(tex, this.anchorOf(target), 0x5aa8ff));
        void this.playRisingImage("texture/effect/agi_up.bmp", target);
        audio.play("effect/ef_incagility.wav", volume);
        break;
      case Skill.DoubleStrafe:
        // The shot's flash at the archer (arrowshot.str); the two arrows fly with the hits.
        void this.playStr("arrowshot", src, BODY_HEIGHT);
        break;
      case Skill.Blessing:
        void this.playSprite("축복", target);
        audio.play("effect/ef_blessing.wav", volume);
        break;
      case Skill.FirstAid:
        this.addEffect(new LightColumn(this.anchorOf(target), 0xffffff, 500));
        break;
    }
  }

  /** A unit's feet, raised `px` sprite pixels towards the top of the screen. */
  private anchorOf(u: Unit, px = 0): Anchor {
    let last = new THREE.Vector3();
    const up = new THREE.Vector3();
    return () => {
      if (!this.map) return last;
      last = roToWorld(u.x * CELL, heightAt(this.map.gat, u.x, u.y), u.y * CELL);
      if (px) last.add(up.set(0, px * SPRITE_SCALE, 0).applyQuaternion(this.camera.quaternion));
      return last;
    };
  }

  private addEffect(e: Effect) {
    this.effects.push(e);
    this.scene.add(e.object);
  }

  /** Runs an effect once its files have loaded, unless the map changed meanwhile. */
  private async whenLoaded<T>(load: Promise<T>, make: (loaded: T) => Effect) {
    const generation = this.mapGeneration;
    try {
      const loaded = await load;
      if (this.disposed || generation !== this.mapGeneration) return;
      this.addEffect(make(loaded));
    } catch (err) {
      console.warn("effect failed to load", err);
    }
  }

  private playStr(name: string, u: Unit, px: number) {
    return this.whenLoaded(loadStr(name), (str) => new StrEffect(str, this.anchorOf(u, px)));
  }

  private playRisingImage(path: string, u: Unit) {
    return this.whenLoaded(loadTexture(path), (tex) => new RisingImage(tex, this.anchorOf(u, HEAD_HEIGHT)));
  }

  private playSprite(name: string, u: Unit, px = HEAD_HEIGHT + 20) {
    return this.whenLoaded(loadEffectAssets(name), (assets) => new SpriteEffect(assets, this.anchorOf(u, px)));
  }

  /** RO's level up effects. kind: 0 base level, 1 job level, 2 job change. */
  private playLevelUp(u: Unit, kind: number) {
    const volume = this.volumeAt(u);
    if (kind === 0) {
      // The white angel with a halo spreading its wings over the character,
      // with feathers drifting down (texture/effect/angel.str).
      void this.playStr("angel", u, 0);
      audio.play("levelup.wav", volume);
      return;
    }
    // Job level: the burst of rings and sparks spelling "Job Lv Up".
    void this.playStr("joblvup", u, 0);
    audio.play("effect/st_job_level_up.wav", volume);
    if (kind === 2) {
      // A new job: the angel too, and a word for it (RO has no banner of its own).
      void this.playStr("angel", u, 0);
      this.floats.spawn("JOB CHANGE!", "level", this.headOf(u));
    }
  }

  private playCastCircle(u: Unit, element: number, ms: number) {
    return this.whenLoaded(castTextures(element), (tex) => {
      const circle = new CastCircle(tex, this.anchorOf(u), ms);
      u.castCircle?.stop();
      u.castCircle = circle;
      return circle;
    });
  }

  /** The live unit of a kind nearest to a cell, within `range` cells. */
  private nearestUnit(cell: [number, number] | null, kind: Kind, range: number): Unit | null {
    if (!cell) return null;
    let best: Unit | null = null;
    let bestD = range + 1;
    for (const u of this.units.values()) {
      if (u.kind !== kind || u.dead || u.dyingSince) continue;
      const d = chebyshev(Math.floor(u.x), Math.floor(u.y), cell[0], cell[1]);
      if (d < bestD) {
        best = u;
        bestD = d;
      }
    }
    return best;
  }

  /** Waits for a click on the target of an enemy or friend skill. */
  beginTargeting(skillId: number, kind: "enemy" | "friend", name: string): void {
    this.targeting = { skillId, kind, name };
    this.canvas.style.cursor = "crosshair";
    this.hud.setStatus(`${name}: click ${kind === "enemy" ? "a monster" : "a player (or yourself)"} · Esc to cancel`);
  }

  /** Stops waiting for a skill target; returns whether one was pending. */
  cancelTargeting(): boolean {
    if (!this.targeting) return false;
    this.targeting = null;
    this.canvas.style.cursor = "";
    this.hud.setStatus("");
    return true;
  }

  /** Attack animation: monsters have one; players pick by job, sex and weapon. */
  private attackGroup(u: Unit): number {
    if (u.kind === "monster") return MonsterAction.Attack;
    return playerAttackGroup(u.job, !!u.look?.female, u.look?.weapon ?? 0);
  }

  /** RO's white shockwave ring on a unit that was just hit. */
  private async playHitRing(u: Unit) {
    const generation = this.mapGeneration;
    const texture = await loadRingTexture();
    if (this.disposed || generation !== this.mapGeneration || !this.map) return;
    const map = this.map;
    const up = new THREE.Vector3();
    const effect = new HitRing(texture, () => {
      // Centred on the body (a little above the feet), pulled towards the camera.
      const feet = roToWorld(u.x * CELL, heightAt(map.gat, u.x, u.y), u.y * CELL);
      up.set(0, BODY_HEIGHT * SPRITE_SCALE, 0).applyQuaternion(this.camera.quaternion);
      return feet.add(up).add(this.camera.position.clone().sub(feet).normalize().multiplyScalar(10));
    });
    this.effects.push(effect);
    this.scene.add(effect.object);
  }

  private updateEffects(now: number) {
    this.effects = this.effects.filter((e) => {
      if (e.update(now, this.camera)) return true;
      this.scene.remove(e.object);
      e.dispose();
      return false;
    });
  }

  /**
   * In the 2004 client a dungeon's tint is its fog-table entry (deleting the
   * entry or typing /fog takes the purple away). Underground the whole view is
   * covered evenly, so it's a screen overlay here: the fog colour at the
   * table's strength over everything, sprites and effects included. Outdoor
   * fog stays off.
   */
  private setDungeonTint(map: MapView) {
    const tint = this.dungeonTint;
    const fog = isDungeon(map.name) ? map.fog : null;
    tint.visible = !!fog;
    if (!fog) return;
    const mat = tint.material as THREE.MeshBasicMaterial;
    mat.color.copy(fog.color).multiplyScalar(DUNGEON_TINT_SHADE);
    mat.opacity = Math.min(fog.strength * DUNGEON_TINT_STRENGTH, 1);
    if (!tint.parent) this.scene.add(tint);
  }

  /** Effects the map places in its scenery, like the torch fires in Payon Cave. */
  private addMapEffects(map: MapView) {
    for (const e of map.effects) {
      const sprite = MAP_EFFECT_SPRITES[e.id];
      if (!sprite) continue;
      // The height counts up from the ground here (Payon Cave's torches sit at
      // +4, the brazier's base), unlike model positions where y points down;
      // the fire sprite itself is drawn well above that point, onto the bowl.
      const at = roToWorld(e.x, -e.y, e.z);
      // Nudged towards the camera so the brazier's rim doesn't cut the flame.
      const anchor = () => this.camera.position.clone().sub(at).normalize().multiplyScalar(4).add(at);
      void loadEffectAssets(sprite)
        .then((assets) => {
          if (this.map !== map || this.disposed) return;
          this.addEffect(new SpriteEffect(assets, anchor, 0, true));
        })
        .catch((err) => console.warn("map effect missing", e.id, sprite, err));
    }
  }

  private clearEffects() {
    for (const e of this.effects) {
      this.scene.remove(e.object);
      e.dispose();
    }
    this.effects = [];
  }

  /** The impact sound for a hit, by attacker: weapon type for players, fists for monsters. */
  private hitSound(src: Unit | undefined): string {
    const n = 1 + Math.floor(Math.random() * 4);
    if (!src || src.kind === "monster") return `_hit_fist${n}.wav`;
    const weapon = src.look?.weapon ?? 0;
    if (weapon >= 1700 && weapon < 1800) return "_hit_arrow.wav";
    if (weapon >= 1100 && weapon < 1200) return "_hit_sword.wav";
    if (weapon >= 1200 && weapon < 1300) return "_hit_dagger.wav";
    if (weapon >= 1300 && weapon < 1400) return "_hit_axe.wav";
    if (weapon >= 1400 && weapon < 1500) return "_hit_spear.wav";
    if (weapon >= 1500 && weapon < 1600) return "_hit_mace.wav";
    if (weapon >= 1600 && weapon < 1700) return "_hit_rod.wav";
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

    if (u.cast) {
      const k = (now - u.cast.start) / u.cast.ms;
      if (k >= 1) {
        u.cast.el.remove();
        u.cast = null;
      } else {
        (u.cast.el.firstElementChild as HTMLElement).style.width = `${k * 100}%`;
        const feet = roToWorld(u.x * CELL, heightAt(this.map!.gat, u.x, u.y), u.y * CELL);
        const below = new THREE.Vector3(0, -(LABEL_DROP + 14) * SPRITE_SCALE, 0).applyQuaternion(this.camera.quaternion);
        this.labels.place(u.cast.el, below.add(feet), this.camera);
      }
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
    if (u.speech) {
      if (now > u.speech.until) {
        u.speech.el.remove();
        u.speech = null;
      } else {
        this.labels.placeAbove(u.speech.el, this.headOf(u)(), this.camera);
      }
    }
  }

  /** Changes a unit's walk speed, re-timing the rest of a walk in progress so
   * it keeps in step with the server (which applies it from the next step). */
  private setSpeed(u: Unit, speed: number, now: number) {
    if (speed <= 0 || speed === u.speed) return;
    const ratio = speed / u.speed;
    u.speed = speed;
    let t = now;
    u.walk.forEach((s, i) => {
      if (i === 0) {
        // Keep the current step where it is; only what's left of it speeds up.
        const k = Math.min(1, Math.max(0, (now - s.t0) / s.dur));
        s.dur *= ratio;
        s.t0 = now - k * s.dur;
        t = s.t0 + s.dur;
        return;
      }
      s.dur *= ratio;
      s.t0 = t;
      t += s.dur;
    });
  }

  /** Shows what a unit said in a bubble over its head. */
  showSpeech(id: number, text: string): void {
    const u = this.units.get(id);
    if (!u || !this.map) return;
    u.speech?.el.remove();
    u.speech = { el: this.labels.createSpeech(text), until: performance.now() + SPEECH_MS };
  }

  private updateSign(p: Extract<ServerPacket, { type: "roomSign" }>) {
    let sign = this.signs.get(p.roomId);
    if (!sign) {
      const roomId = p.roomId;
      const el = this.labels.createSign(() => {
        const s = this.signs.get(roomId);
        if (s && s.ownerId !== this.selfId) this.hud.joinRoom(roomId, s.public, s.title);
      });
      sign = { el, ownerId: p.ownerId, public: p.public, title: p.title };
      this.signs.set(p.roomId, sign);
    }
    Object.assign(sign, { ownerId: p.ownerId, public: p.public, title: p.title });
    sign.el.textContent = `${p.public ? "" : "🔒 "}${p.title} (${p.members}/${p.limit})`;
    sign.el.title = p.public ? "Click to join" : "Private room: click to enter the password";
  }

  private async addFloorItem(p: Extract<ServerPacket, { type: "floorItem" }>) {
    this.removeFloorItem(p.id);
    const info = this.hud.itemInfo(p.itemId);
    const name = (info?.name ?? `Item ${p.itemId}`) + (p.amount > 1 ? ` × ${p.amount}` : "");
    const label = this.labels.create(name, "item");
    label.style.visibility = "hidden"; // shown on hover, as in RO
    const item: FloorItem = {
      id: p.id,
      x: p.x + p.subX / 16,
      y: p.y + p.subY / 16,
      label,
      sprite: null,
      droppedAt: p.dropped ? performance.now() : 0,
    };
    this.floorItems.set(p.id, item);
    if (!info?.resName) return;
    const generation = this.mapGeneration;
    try {
      const assets = await loadItemAssets(info.resName);
      if (this.disposed || generation !== this.mapGeneration || this.floorItems.get(p.id) !== item) return;
      item.sprite = new CharacterSprite(assets);
      item.sprite.draw(0, 0, 0);
      item.sprite.object.renderOrder = 1;
      this.scene.add(item.sprite.object);
    } catch (err) {
      console.warn("item sprite failed", info.resName, err);
    }
  }

  private removeFloorItem(id: number) {
    const item = this.floorItems.get(id);
    if (!item) return;
    if (item.sprite) {
      this.scene.remove(item.sprite.object);
      item.sprite.dispose();
    }
    item.label.remove();
    if (this.hoveredItem === item) this.hoveredItem = null;
    this.floorItems.delete(id);
  }

  private clearFloorItems() {
    for (const id of [...this.floorItems.keys()]) this.removeFloorItem(id);
  }

  /** Positions floor items, bouncing fresh drops up and down onto the ground. */
  private placeFloorItems(now: number) {
    const up = new THREE.Vector3();
    for (const item of this.floorItems.values()) {
      if (!item.sprite) {
        item.label.hidden = true;
        continue;
      }
      this.placeSprite(item.sprite, item.label, item.x, item.y);
      const t = item.droppedAt ? (now - item.droppedAt) / DROP_MS : 1;
      if (t < 1) {
        const lift = Math.sin(Math.PI * t) * 24 * (1 - t * 0.5); // sprite pixels
        item.sprite.object.position.add(up.set(0, lift * SPRITE_SCALE, 0).applyQuaternion(this.camera.quaternion));
      }
      item.label.style.visibility = this.hoveredItem === item ? "" : "hidden";
    }
  }

  /** The floor item whose visible pixels are under the mouse. */
  private pickFloorItem(ev: MouseEvent): FloorItem | null {
    const byObject = new Map<THREE.Object3D, FloorItem>();
    for (const item of this.floorItems.values()) if (item.sprite) byObject.set(item.sprite.object, item);
    if (byObject.size === 0) return null;
    this.ray(ev);
    for (const hit of this.raycaster.intersectObjects([...byObject.keys()], false)) {
      const item = byObject.get(hit.object);
      if (item?.sprite && hit.uv && item.sprite.hitTest(hit.uv)) return item;
    }
    return null;
  }

  private placeSigns() {
    for (const sign of this.signs.values()) {
      const owner = this.units.get(sign.ownerId);
      if (!owner || !this.map) {
        sign.el.hidden = true;
        continue;
      }
      this.labels.placeAbove(sign.el, this.headOf(owner)(), this.camera);
    }
  }

  private clearSigns() {
    for (const sign of this.signs.values()) sign.el.remove();
    this.signs.clear();
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
    if (u.sitting) {
      return [PlayerAction.Sit, 0]; // frame 0: head facing forward
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
    // Take on the light where it stands (dark caves, tree shade), easing
    // between cells so walking through doesn't flicker.
    const mat = sprite.object.material as THREE.MeshBasicMaterial;
    mat.color.lerp(this.map!.lightAt(x, y, cellLight), SPRITE_LIGHT_EASE);
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
      this.placeSigns();
      this.placeFloorItems(now);
      this.updatePendingNpc();
      this.updateAmbient(now);
      this.updateEffects(now);
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
    // Players are only clickable while choosing a friend skill's target.
    const friend = this.targeting?.kind === "friend";
    const candidates = [
      ...this.npcs.values(),
      ...[...this.units.values()].filter(
        (u) => (u.kind === "monster" && !u.dyingSince) || (friend && u.kind === "player" && !u.dead),
      ),
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
    this.canvas.style.cursor = this.targeting || u?.kind === "monster" ? "crosshair" : u ? "pointer" : "";
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
        if (this.cancelTargeting()) return;
        rotating = true;
        lastX = ev.clientX;
        return;
      }
      if (ev.button !== 0 || !this.conn.connected) return;
      const self = this.units.get(this.selfId);
      if (!self || self.dead) return;
      const u = this.pickUnit(ev);
      if (this.targeting) {
        const t = this.targeting;
        const want = t.kind === "enemy" ? "monster" : "player";
        // Heal can also be aimed at a monster: it burns the undead (the server
        // refuses anything else).
        const healOnMonster = t.skillId === Skill.Heal && u?.kind === "monster";
        // A near miss on a small or moving target still counts.
        const picked = u?.kind === want || healOnMonster ? u : this.nearestUnit(this.pick(ev), want, 2);
        this.cancelTargeting();
        if (picked) {
          this.pendingNpc = null;
          this.conn.useSkill(t.skillId, 0, picked.id);
        } else {
          this.hud.notice(`${t.name} needs ${t.kind === "enemy" ? "a monster" : "a player"} as its target.`);
        }
        return;
      }
      if (self.sitting && u?.kind !== "npc") {
        // Seated: turn to face where we clicked instead of walking or attacking.
        const at = u ? [u.x, u.y] : this.pick(ev)?.map((c) => c + 0.5);
        if (at) {
          const dir = dirFor(at[0] - self.x, at[1] - self.y, self.dir);
          if (dir !== self.dir) this.conn.turn(dir);
        }
        return;
      }
      if (u?.kind === "npc") {
        this.talkTo(u);
        return;
      }
      this.pendingNpc = null;
      if (u?.kind === "monster") {
        this.conn.attack(u.id);
        return;
      }
      const item = this.pickFloorItem(ev);
      if (item) {
        this.conn.pickUp(item.id); // the server walks us over first
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
      this.hoveredItem = u || rotating ? null : this.pickFloorItem(ev);
      this.canvas.style.cursor =
        this.targeting || u?.kind === "monster" ? "crosshair" : u || this.hoveredItem ? "pointer" : "";
      this.updateCursor(u || this.hoveredItem ? null : this.pick(ev));
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
    this.clearSigns();
    this.clearFloorItems();
    this.clearPortals();
    this.clearNpcs();
    this.clearEffects();
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

/** Caves, sewers and pyramids: underground maps with a black backdrop. */
function isDungeon(map: string): boolean {
  return /dun|sewb|pryd/.test(map);
}

/** A full-screen quad drawn last, blending a colour over the whole view. */
function createScreenTint(): THREE.Mesh {
  const mat = new THREE.MeshBasicMaterial({ transparent: true, depthTest: false, depthWrite: false });
  // Position straight in clip space, so it always fills the screen.
  mat.onBeforeCompile = (shader) => {
    shader.vertexShader = shader.vertexShader.replace(
      "#include <project_vertex>",
      "vec4 mvPosition = vec4(position, 1.0);\ngl_Position = vec4(position.xy, 0.0, 1.0);",
    );
  };
  const mesh = new THREE.Mesh(new THREE.PlaneGeometry(2, 2), mat);
  mesh.frustumCulled = false;
  mesh.renderOrder = 1000;
  mesh.visible = false;
  return mesh;
}
