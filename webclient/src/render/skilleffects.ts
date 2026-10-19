// Skill effects: RO's STR animations (texture/effect/*.str), the magic circle
// under a caster, Heal's column of light, rising "AGI Up" text and sprite
// effects such as Blessing's angel (sprite/이팩트/축복).
import * as THREE from "three";
import { fetchData } from "../assets";
import { layerAt, parseStr, type Str } from "../formats/str";
import { CharacterSprite, loadEffectAssets, SPRITE_SCALE } from "./character";
import type { Effect } from "./effect";
import { CELL } from "./map";
import { loadTexture } from "./textures";

/** Where an effect is drawn, re-read every frame so it follows its unit. */
export type Anchor = () => THREE.Vector3;

interface LoadedStr {
  str: Str;
  textures: THREE.Texture[][];
}

const strCache = new Map<string, Promise<LoadedStr>>();

/** Loads texture/effect/<name>.str and the textures it uses. */
export function loadStr(name: string): Promise<LoadedStr> {
  let p = strCache.get(name);
  if (!p) {
    p = (async () => {
      const str = parseStr(await fetchData(`texture/effect/${name}.str`));
      const textures = await Promise.all(
        str.layers.map((l) => Promise.all(l.textures.map((t) => loadTexture(`texture/effect/${t}`)))),
      );
      return { str, textures };
    })();
    p.catch(() => strCache.delete(name));
    strCache.set(name, p);
  }
  return p;
}

/** Direct3D blend factors (D3DBLEND) to three.js. */
function blendFactor(d3d: number): THREE.BlendingDstFactor {
  switch (d3d) {
    case 1:
    case 8: // INVDESTALPHA: the canvas is opaque
      return THREE.ZeroFactor;
    case 3:
      return THREE.SrcColorFactor;
    case 4:
      return THREE.OneMinusSrcColorFactor;
    case 5:
      return THREE.SrcAlphaFactor;
    case 6:
      return THREE.OneMinusSrcAlphaFactor;
    case 9:
      return THREE.DstColorFactor;
    case 10:
      return THREE.OneMinusDstColorFactor;
    default: // ONE, DESTALPHA (1 on an opaque canvas)
      return THREE.OneFactor;
  }
}

/** Plays an STR animation once, facing the camera. */
export class StrEffect implements Effect {
  readonly object = new THREE.Group();
  private readonly start = performance.now();
  private readonly meshes: THREE.Mesh[] = [];

  constructor(
    private readonly loaded: LoadedStr,
    private readonly anchor: Anchor,
    private readonly scale = 1,
  ) {
    for (let i = 0; i < loaded.str.layers.length; i++) {
      const geo = new THREE.BufferGeometry();
      geo.setAttribute("position", new THREE.BufferAttribute(new Float32Array(12), 3));
      geo.setAttribute("uv", new THREE.BufferAttribute(new Float32Array(8), 2));
      geo.setIndex([0, 1, 2, 0, 2, 3]);
      const mat = new THREE.MeshBasicMaterial({
        transparent: true,
        depthWrite: false,
        depthTest: false,
        side: THREE.DoubleSide,
        blending: THREE.CustomBlending,
      });
      const mesh = new THREE.Mesh(geo, mat);
      mesh.frustumCulled = false;
      mesh.renderOrder = 6;
      mesh.visible = false;
      this.meshes.push(mesh);
      this.object.add(mesh);
    }
  }

  update(now: number, camera: THREE.Camera): boolean {
    const { str, textures } = this.loaded;
    const frame = Math.floor(((now - this.start) / 1000) * str.fps);
    if (frame >= str.frames) return false;
    this.object.position.copy(this.anchor());
    this.object.quaternion.copy(camera.quaternion);
    const s = SPRITE_SCALE * this.scale;
    for (let i = 0; i < this.meshes.length; i++) {
      const mesh = this.meshes[i];
      const key = layerAt(str.layers[i], frame);
      const tex = textures[i];
      if (!key || tex.length === 0 || key.color[3] <= 0) {
        mesh.visible = false;
        continue;
      }
      mesh.visible = true;
      const mat = mesh.material as THREE.MeshBasicMaterial;
      const t = tex[Math.min(Math.max(Math.floor(key.texFrame), 0), tex.length - 1)];
      if (mat.map !== t) {
        mat.map = t;
        mat.needsUpdate = true;
      }
      mat.blendSrc = blendFactor(key.srcBlend) as THREE.BlendingSrcFactor;
      mat.blendDst = blendFactor(key.dstBlend);
      mat.color.setRGB(key.color[0] / 255, key.color[1] / 255, key.color[2] / 255, THREE.SRGBColorSpace);
      mat.opacity = Math.min(key.color[3] / 255, 1);

      const a = (key.angle / 1024) * Math.PI * 2;
      const cos = Math.cos(a), sin = Math.sin(a);
      const pos = mesh.geometry.getAttribute("position") as THREE.BufferAttribute;
      for (let c = 0; c < 4; c++) {
        const cx = key.xy[c], cy = key.xy[c + 4];
        const rx = cx * cos - cy * sin, ry = cx * sin + cy * cos;
        // Screen y points down; the anchor sits at (320, 320).
        pos.setXYZ(c, (key.x - 320 + rx) * s, -(key.y - 320 + ry) * s, 0);
      }
      pos.needsUpdate = true;
      const uv = mesh.geometry.getAttribute("uv") as THREE.BufferAttribute;
      const [u0, v0, u1, v1] = key.uv;
      uv.setXY(0, u0, v0);
      uv.setXY(1, u1, v0);
      uv.setXY(2, u1, v1);
      uv.setXY(3, u0, v1);
      uv.needsUpdate = true;
    }
    return true;
  }

  dispose(): void {
    for (const m of this.meshes) {
      m.geometry.dispose();
      (m.material as THREE.Material).dispose();
    }
  }
}

const ELEMENT_COLOURS: Record<number, [string, number]> = {
  1: ["blue", 0x6aa8ff], // water
  3: ["red", 0xff6a4a], // fire
  4: ["violet", 0xd08cff], // wind
  6: ["sky", 0xfff2c0], // holy
};

/** Textures and tint for a cast in an element (others are green). */
export async function castTextures(element: number) {
  const [name, tint] = ELEMENT_COLOURS[element] ?? ["green", 0x8cff9a];
  const [ring, sparkle] = await Promise.all([
    loadTexture("texture/effect/magic_ring_a.bmp"),
    loadTexture(`texture/effect/magic_${name}.tga`),
  ]);
  return { ring, sparkle, tint };
}

/** RO's casting effect: a spinning star ring on the ground, tinted by the
 * skill's element, and a column of coloured sparkles around the caster. */
export class CastCircle implements Effect {
  readonly object = new THREE.Group();
  private readonly ring: THREE.Mesh;
  private readonly column: THREE.Mesh;
  private readonly start = performance.now();
  private stopped = false;

  constructor(
    textures: Awaited<ReturnType<typeof castTextures>>,
    private readonly anchor: Anchor,
    private readonly ms: number,
  ) {
    const ringGeo = new THREE.PlaneGeometry(CELL * 5, CELL * 5);
    ringGeo.rotateX(-Math.PI / 2);
    this.ring = new THREE.Mesh(
      ringGeo,
      new THREE.MeshBasicMaterial({
        map: textures.ring,
        color: textures.tint,
        transparent: true,
        depthWrite: false,
        blending: THREE.AdditiveBlending,
        side: THREE.DoubleSide,
      }),
    );
    this.ring.renderOrder = 1;

    const height = 70 * SPRITE_SCALE;
    const colGeo = new THREE.CylinderGeometry(CELL * 0.9, CELL * 0.9, height, 24, 1, true);
    colGeo.translate(0, height / 2, 0);
    const sparkle = textures.sparkle.clone();
    sparkle.repeat.set(3, 1);
    sparkle.needsUpdate = true;
    this.column = new THREE.Mesh(
      colGeo,
      new THREE.MeshBasicMaterial({
        map: sparkle,
        transparent: true,
        depthWrite: false,
        blending: THREE.AdditiveBlending,
        side: THREE.DoubleSide,
      }),
    );
    this.column.renderOrder = 3;
    this.object.add(this.ring, this.column);
  }

  /** Ends the effect early (cast cancelled or finished). */
  stop(): void {
    this.stopped = true;
  }

  update(now: number): boolean {
    const t = now - this.start;
    if (this.stopped || t > this.ms + 100) return false;
    this.object.position.copy(this.anchor());
    this.object.position.y += 0.2;
    const grow = Math.min(1, t / 200);
    this.ring.rotation.y = t / 700;
    this.ring.scale.setScalar(0.4 + 0.6 * grow);
    (this.ring.material as THREE.MeshBasicMaterial).opacity = grow;
    this.column.rotation.y = -t / 400;
    const map = (this.column.material as THREE.MeshBasicMaterial).map!;
    map.offset.y = -t / 1500; // sparkles drift upwards
    (this.column.material as THREE.MeshBasicMaterial).opacity = 0.7 * grow;
    return true;
  }

  dispose(): void {
    for (const m of [this.ring, this.column]) {
      m.geometry.dispose();
      const mat = m.material as THREE.MeshBasicMaterial;
      if (m === this.column) mat.map?.dispose(); // the clone; the original stays cached
      mat.dispose();
    }
  }
}

let columnTexture: THREE.Texture | null = null;

/** Soft vertical glow: bright at the bottom, fading out at the top and sides. */
function glowTexture(): THREE.Texture {
  if (columnTexture) return columnTexture;
  const c = document.createElement("canvas");
  c.width = 64;
  c.height = 128;
  const ctx = c.getContext("2d")!;
  const v = ctx.createLinearGradient(0, 0, 0, 128);
  v.addColorStop(0, "rgba(255,255,255,0)");
  v.addColorStop(0.6, "rgba(255,255,255,0.55)");
  v.addColorStop(1, "rgba(255,255,255,1)");
  ctx.fillStyle = v;
  ctx.fillRect(0, 0, 64, 128);
  columnTexture = new THREE.CanvasTexture(c);
  columnTexture.colorSpace = THREE.SRGBColorSpace;
  return columnTexture;
}

/** Heal's column of light rising around the target. */
export class LightColumn implements Effect {
  readonly object: THREE.Mesh;
  private readonly start = performance.now();

  constructor(
    private readonly anchor: Anchor,
    color: THREE.ColorRepresentation,
    private readonly ms = 900,
  ) {
    const height = 90 * SPRITE_SCALE;
    const geo = new THREE.CylinderGeometry(CELL * 0.8, CELL * 0.6, height, 24, 1, true);
    geo.translate(0, height / 2, 0);
    const mat = new THREE.MeshBasicMaterial({
      map: glowTexture(),
      color,
      transparent: true,
      depthWrite: false,
      blending: THREE.AdditiveBlending,
      side: THREE.DoubleSide,
    });
    this.object = new THREE.Mesh(geo, mat);
    this.object.renderOrder = 4;
  }

  update(now: number): boolean {
    const t = (now - this.start) / this.ms;
    if (t >= 1) return false;
    this.object.position.copy(this.anchor());
    this.object.scale.set(1, Math.min(1, t * 4), 1);
    this.object.rotation.y = t * 2;
    (this.object.material as THREE.MeshBasicMaterial).opacity = t < 0.6 ? 1 : 1 - (t - 0.6) / 0.4;
    return true;
  }

  dispose(): void {
    this.object.geometry.dispose();
    (this.object.material as THREE.Material).dispose();
  }
}

/**
 * RO's rising light cylinder: a ring texture (texture/effect/ring_*.tga, a
 * strip of light streaks, solid at the bottom and spiky at the top) wrapped
 * around the unit. It shoots up, turns, and fades. Two layers turning opposite
 * ways give the streaks depth. Teleporting (wings) uses it with ring_blue.
 */
export class RingCylinder implements Effect {
  readonly object = new THREE.Group();
  private readonly start = performance.now();
  private readonly layers: { mesh: THREE.Mesh; spin: number }[] = [];

  constructor(
    texture: THREE.Texture,
    private readonly anchor: Anchor,
    private readonly ms = 1200,
  ) {
    const height = 95 * SPRITE_SCALE;
    for (const [radius, spin, repeats] of [
      [CELL * 0.75, 1.6, 3],
      [CELL * 0.55, -2.2, 2],
    ] as const) {
      const geo = new THREE.CylinderGeometry(radius, radius * 0.9, height, 28, 1, true);
      geo.translate(0, height / 2, 0);
      const tex = texture.clone();
      // Image top (spikes) at the cylinder's top: flip v, wrap around a few times.
      tex.repeat.set(repeats, -1);
      tex.offset.set(0, 1);
      tex.needsUpdate = true;
      const mesh = new THREE.Mesh(
        geo,
        new THREE.MeshBasicMaterial({
          map: tex,
          transparent: true,
          depthWrite: false,
          blending: THREE.AdditiveBlending,
          side: THREE.DoubleSide,
        }),
      );
      mesh.renderOrder = 4;
      this.layers.push({ mesh, spin });
      this.object.add(mesh);
    }
  }

  update(now: number): boolean {
    const t = (now - this.start) / this.ms;
    if (t >= 1) return false;
    this.object.position.copy(this.anchor());
    // Shoot up fast, then hold while fading.
    const rise = 1 - Math.pow(1 - Math.min(1, t / 0.3), 3);
    const fade = t < 0.55 ? 1 : 1 - (t - 0.55) / 0.45;
    for (const { mesh, spin } of this.layers) {
      mesh.scale.set(1, Math.max(rise, 0.01), 1);
      mesh.rotation.y = spin * t * Math.PI;
      (mesh.material as THREE.MeshBasicMaterial).opacity = fade;
    }
    return true;
  }

  dispose(): void {
    for (const { mesh } of this.layers) {
      mesh.geometry.dispose();
      const mat = mesh.material as THREE.MeshBasicMaterial;
      mat.map?.dispose(); // the clone; the loaded texture stays cached
      mat.dispose();
    }
  }
}

/** An image (e.g. texture/effect/agi_up.bmp) floating up from a unit's head. */
export class RisingImage implements Effect {
  readonly object: THREE.Mesh;
  private readonly start = performance.now();

  constructor(
    texture: THREE.Texture,
    private readonly anchor: Anchor,
    private readonly ms = 1500,
  ) {
    const img = texture.image as { width: number; height: number };
    const geo = new THREE.PlaneGeometry(img.width * SPRITE_SCALE, img.height * SPRITE_SCALE);
    const mat = new THREE.MeshBasicMaterial({
      map: texture,
      transparent: true,
      depthWrite: false,
      depthTest: false,
      blending: THREE.AdditiveBlending,
    });
    this.object = new THREE.Mesh(geo, mat);
    this.object.renderOrder = 7;
  }

  update(now: number, camera: THREE.Camera): boolean {
    const t = (now - this.start) / this.ms;
    if (t >= 1) return false;
    const up = new THREE.Vector3(0, 30 * t * SPRITE_SCALE, 0).applyQuaternion(camera.quaternion);
    this.object.position.copy(this.anchor()).add(up);
    this.object.quaternion.copy(camera.quaternion);
    (this.object.material as THREE.MeshBasicMaterial).opacity = t < 0.7 ? 1 : 1 - (t - 0.7) / 0.3;
    return true;
  }

  dispose(): void {
    this.object.geometry.dispose();
    (this.object.material as THREE.Material).dispose();
  }
}

/** Plays an effect sprite's first action once (e.g. sprite/이팩트/축복), or
 * over and over for effects placed on a map (torch fire). */
export class SpriteEffect implements Effect {
  readonly object: THREE.Mesh;
  private readonly start = performance.now();
  private readonly sprite: CharacterSprite;
  private readonly frameMs: number;
  private readonly frames: number;

  /** action picks one of the sprite's actions (e.g. an emoticon); 0 by default. */
  constructor(
    assets: Awaited<ReturnType<typeof loadEffectAssets>>,
    private readonly anchor: Anchor,
    private readonly action = 0,
    private readonly loop = false,
  ) {
    this.sprite = new CharacterSprite(assets);
    this.object = this.sprite.object;
    this.object.renderOrder = 7;
    const mat = this.object.material as THREE.MeshBasicMaterial;
    // A looping map effect sits in the scenery, so walls and pillars hide it.
    mat.depthTest = loop;
    ({ delay: this.frameMs, frames: this.frames } = this.sprite.actionTiming(action));
  }

  update(now: number, camera: THREE.Camera): boolean {
    if (this.frames === 0) return this.loop;
    let frame = Math.floor((now - this.start) / this.frameMs);
    if (frame >= this.frames) {
      if (!this.loop) return false;
      frame %= this.frames;
    }
    // draw() addresses actions as group * 8 + direction.
    this.sprite.draw(this.action >> 3, this.action & 7, frame);
    this.object.position.copy(this.anchor());
    this.object.quaternion.copy(camera.quaternion);
    return true;
  }

  dispose(): void {
    this.sprite.dispose();
  }
}
