// The in-game view: renders the current map and players, and turns clicks into Move packets.
import * as THREE from "three";
import { heightAt, isWalkable } from "./formats/gat";
import type { Connection, ServerPacket } from "./net";
import { CharacterSprite, loadNoviceAssets, State, type CharacterAssets } from "./render/character";
import { CELL, loadMap, roToWorld, worldToRo, type MapView } from "./render/map";

type EnterMap = Extract<ServerPacket, { type: "enterMap" }>;

interface Segment {
  fx: number;
  fy: number;
  tx: number;
  ty: number;
  t0: number;
  dur: number;
}

class Entity {
  /** Fractional cell coordinates of the feet (cell centre = x + 0.5). */
  x: number;
  y: number;
  /** Server facing: 0 N, 1 NW, 2 W, 3 SW, 4 S, 5 SE, 6 E, 7 NE. */
  dir: number;
  walk: Segment[] = [];
  walkStart = 0;

  constructor(
    readonly id: number,
    readonly name: string,
    readonly speed: number,
    readonly sprite: CharacterSprite,
    x: number,
    y: number,
    dir: number,
  ) {
    this.x = x + 0.5;
    this.y = y + 0.5;
    this.dir = dir;
  }
}

export interface GameHud {
  setStatus(text: string): void;
  setPosition(map: string, x: number, y: number): void;
}

const DIAGONAL = 1.4;
const PITCH = THREE.MathUtils.degToRad(50);

export class Game {
  /** Our character's name (EnterMap doesn't carry it); set by the login screen. */
  selfName = "";
  private readonly renderer: THREE.WebGLRenderer;
  private readonly scene = new THREE.Scene();
  private readonly camera = new THREE.PerspectiveCamera(30, 1, 5, 4000);
  private readonly raycaster = new THREE.Raycaster();
  private readonly cursor: THREE.Mesh;
  private map: MapView | null = null;
  private assets: CharacterAssets | null = null;
  private readonly entities = new Map<number, Entity>();
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
        this.addEntity(p.id, p.name, p.speed, p.x, p.y, p.dir);
        break;
      case "disappear":
        this.removeEntity(p.id);
        break;
      case "move":
        this.startWalk(p.id, p.fromX, p.fromY, p.steps);
        break;
      case "stop": {
        const e = this.entities.get(p.id);
        if (e) {
          e.walk = [];
          e.x = p.x + 0.5;
          e.y = p.y + 0.5;
        }
        break;
      }
    }
  }

  private async enterMap(p: EnterMap) {
    this.loading = true;
    this.queue = [];
    for (const id of [...this.entities.keys()]) this.removeEntity(id);
    if (this.map) {
      this.scene.remove(this.map.root);
      this.map.dispose();
      this.map = null;
    }

    try {
      const [map, assets] = await Promise.all([
        loadMap(p.map, (msg) => this.hud.setStatus(msg)),
        loadNoviceAssets(),
      ]);
      if (this.disposed) {
        map.dispose();
        return;
      }
      this.map = map;
      this.assets = assets;
      this.scene.add(map.root);
      this.selfId = p.selfId;
      // The server doesn't echo our own name in EnterMap; the HUD label covers it.
      this.addEntity(p.selfId, this.selfName, p.speed, p.x, p.y, p.dir);
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

  private addEntity(id: number, name: string, speed: number, x: number, y: number, dir: number) {
    if (!this.assets) return;
    this.removeEntity(id);
    const sprite = new CharacterSprite(this.assets, name, id === this.selfId);
    const e = new Entity(id, name, speed, sprite, x, y, dir);
    this.entities.set(id, e);
    this.scene.add(sprite.object);
  }

  private removeEntity(id: number) {
    const e = this.entities.get(id);
    if (!e) return;
    this.scene.remove(e.sprite.object);
    e.sprite.dispose();
    this.entities.delete(id);
  }

  private startWalk(id: number, fromX: number, fromY: number, steps: [number, number][]) {
    const e = this.entities.get(id);
    if (!e || steps.length === 0) return;
    const now = performance.now();
    const stepMs = (ax: number, ay: number, bx: number, by: number) =>
      e.speed * (ax !== bx && ay !== by ? DIAGONAL : 1);

    // Continue from where the sprite is drawn (it may be mid-step) to avoid snapping.
    const walking = e.walk.length > 0;
    let sx = walking ? e.x : fromX + 0.5;
    let sy = walking ? e.y : fromY + 0.5;
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
    if (!walking) e.walkStart = now;
    e.walk = segs;
  }

  private updateEntity(e: Entity, now: number) {
    while (e.walk.length && now >= e.walk[0].t0 + e.walk[0].dur) {
      const s = e.walk.shift()!;
      e.x = s.tx;
      e.y = s.ty;
      e.dir = dirFor(s.tx - s.fx, s.ty - s.fy, e.dir);
    }
    let state: State = State.Idle;
    if (e.walk.length) {
      const s = e.walk[0];
      const k = Math.max(0, (now - s.t0) / s.dur);
      e.x = s.fx + (s.tx - s.fx) * k;
      e.y = s.fy + (s.ty - s.fy) * k;
      e.dir = dirFor(s.tx - s.fx, s.ty - s.fy, e.dir);
      state = State.Walk;
    }

    const camSteps = Math.round(this.yaw / (Math.PI / 4));
    const spriteDir = (((4 - e.dir + camSteps) % 8) + 8) % 8;
    const frame =
      state === State.Walk
        ? Math.floor((now - e.walkStart) / e.sprite.frameDelay(state, e.speed)) % e.sprite.frameCount(state)
        : 0;
    e.sprite.draw(state, spriteDir, frame);

    const map = this.map!;
    const feet = roToWorld(e.x * CELL, heightAt(map.gat, e.x, e.y), e.y * CELL);
    // Pull the billboard towards the camera so it isn't cut by the ground or walls it stands next to.
    const toCam = this.camera.position.clone().sub(feet).normalize().multiplyScalar(8);
    e.sprite.object.position.copy(feet).add(toCam);
    e.sprite.object.quaternion.copy(this.camera.quaternion);
  }

  private render(now: number) {
    if (this.map) {
      this.map.update(now);
      const self = this.entities.get(this.selfId);
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
      for (const e of this.entities.values()) this.updateEntity(e, now);
    }
    this.renderer.render(this.scene, this.camera);
  }

  /** Map cell under the mouse, or null. */
  private pick(ev: MouseEvent): [number, number] | null {
    if (!this.map) return null;
    const r = this.canvas.getBoundingClientRect();
    const ndc = new THREE.Vector2(((ev.clientX - r.left) / r.width) * 2 - 1, -((ev.clientY - r.top) / r.height) * 2 + 1);
    this.raycaster.setFromCamera(ndc, this.camera);
    const hit = this.raycaster.intersectObjects(this.map.ground, false)[0];
    if (!hit) return null;
    const [x, , z] = worldToRo(hit.point);
    const cx = Math.floor(x / CELL), cy = Math.floor(z / CELL);
    if (cx < 0 || cy < 0 || cx >= this.map.gat.width || cy >= this.map.gat.height) return null;
    return [cx, cy];
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
      if (ev.button !== 0) return;
      const cell = this.pick(ev);
      if (cell && this.conn.connected) this.conn.move(cell[0], cell[1]);
    });
    on(window, "mouseup", () => (rotating = false));
    on(this.canvas, "mousemove", (ev) => {
      if (rotating) {
        this.yaw -= (ev.clientX - lastX) * 0.008;
        lastX = ev.clientX;
      }
      this.updateCursor(this.pick(ev));
    });
    on(this.canvas, "mouseleave", () => this.updateCursor(null));
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
    for (const id of [...this.entities.keys()]) this.removeEntity(id);
    if (this.map) {
      this.scene.remove(this.map.root);
      this.map.dispose();
    }
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
