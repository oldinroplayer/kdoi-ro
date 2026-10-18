// Character sprites: body + head + shadow composited on a canvas shown on a
// camera-facing billboard, like the original client's 2D-in-3D characters.
// Names are drawn separately as screen-space labels (see labels.ts).
import * as THREE from "three";
import { fetchData } from "../assets";
import { parseAct, type Act, type ActFrame } from "../formats/act";
import { parseSpr } from "../formats/spr";
import type { Look } from "../net";

/** World units per sprite pixel. */
export const SPRITE_SCALE = 1 / 7;

const CANVAS = 256;
/** Where the character's feet are on the canvas. */
const FOOT_X = 128;
const FOOT_Y = 176;

/**
 * Player ACT action groups; the action index is group * 8 + direction.
 * Weapons and shields only have frames in Ready (battle stance) and attacks.
 */
export const PlayerAction = { Idle: 0, Walk: 1, Ready: 4, Attack: 10, Hurt: 6, Dead: 8 } as const;

/** Monster ACT action groups. */
export const MonsterAction = { Idle: 0, Walk: 1, Attack: 2, Hurt: 3, Die: 4 } as const;

interface SpriteSet {
  act: Act;
  indexed: HTMLCanvasElement[];
  rgba: HTMLCanvasElement[];
}

const spriteSets = new Map<string, Promise<SpriteSet>>();

function loadSpriteSet(path: string): Promise<SpriteSet> {
  let p = spriteSets.get(path);
  if (!p) {
    p = fetchSpriteSet(path);
    p.catch(() => spriteSets.delete(path)); // allow a retry after a failed load
    spriteSets.set(path, p);
  }
  return p;
}

async function fetchSpriteSet(path: string): Promise<SpriteSet> {
  const [sprBuf, actBuf] = await Promise.all([fetchData(`${path}.spr`), fetchData(`${path}.act`)]);
  const spr = parseSpr(sprBuf);
  const toCanvas = (img: { width: number; height: number; rgba: Uint8ClampedArray }) => {
    const c = document.createElement("canvas");
    c.width = Math.max(1, img.width);
    c.height = Math.max(1, img.height);
    if (img.width && img.height) {
      c.getContext("2d")!.putImageData(new ImageData(new Uint8ClampedArray(img.rgba), img.width, img.height), 0, 0);
    }
    return c;
  };
  return { act: parseAct(actBuf), indexed: spr.indexed.map(toCanvas), rgba: spr.rgba.map(toCanvas) };
}

export interface CharacterAssets {
  body: SpriteSet;
  head?: SpriteSet; // players have a separate head; NPC sprites are one piece
  shadow: SpriteSet;
}

let novice: Promise<CharacterAssets> | null = null;

/** Male Novice body with the default hairstyle. */
export function loadNoviceAssets(): Promise<CharacterAssets> {
  novice ??= Promise.all([
    loadSpriteSet("sprite/인간족/몸통/남/초보자_남"),
    loadSpriteSet("sprite/인간족/머리통/남/2_남"),
    loadSpriteSet("sprite/shadow"),
  ]).then(([body, head, shadow]) => ({ body, head, shadow }));
  return novice;
}

const oneSpriteAssets = new Map<string, Promise<CharacterAssets>>();

/** A single-sprite unit (NPC or monster) with a shadow. */
function loadOneSprite(path: string): Promise<CharacterAssets> {
  let p = oneSpriteAssets.get(path);
  if (!p) {
    p = Promise.all([loadSpriteSet(path), loadSpriteSet("sprite/shadow")]).then(([body, shadow]) => ({ body, shadow }));
    oneSpriteAssets.set(path, p);
  }
  return p;
}

/** An NPC sprite from data/sprite/npc/. */
export const loadNpcAssets = (sprite: string) => loadOneSprite(`sprite/npc/${sprite}`);

/** A monster sprite from data/sprite/몬스터/. */
export const loadMonsterAssets = (sprite: string) => loadOneSprite(`sprite/몬스터/${sprite}`);

/** Sprite layers for visible equipment (male Novice for now). */
export interface EquipLayers {
  weapon?: SpriteSet;
  shield?: SpriteSet;
  headgear: SpriteSet[];
}

export const noEquipment: EquipLayers = { headgear: [] };

/**
 * Loads the sprites for a look. spriteOf maps an item ID to its sprite name
 * (from the item database); items without one, or missing files, are skipped.
 */
export async function loadEquipLayers(look: Look, spriteOf: (itemId: number) => string | undefined): Promise<EquipLayers> {
  const load = async (itemId: number, path: (name: string) => string) => {
    const name = itemId ? spriteOf(itemId) : undefined;
    if (!name) return undefined;
    try {
      return await loadSpriteSet(path(name));
    } catch (err) {
      console.warn("equipment sprite missing", itemId, name, err);
      return undefined;
    }
  };
  const [weapon, shield, top, mid, low] = await Promise.all([
    load(look.weapon, (n) => `sprite/인간족/초보자/초보자_남_${n}`),
    load(look.shield, (n) => `sprite/방패/초보자/초보자_남_${n}`),
    load(look.headTop, (n) => `sprite/악세사리/남/남_${n}`),
    load(look.headMid, (n) => `sprite/악세사리/남/남_${n}`),
    load(look.headLow, (n) => `sprite/악세사리/남/남_${n}`),
  ]);
  // RO draws lower, then middle, then upper headgear.
  return { weapon, shield, headgear: [low, mid, top].filter((s): s is SpriteSet => !!s) };
}

export class CharacterSprite {
  readonly object: THREE.Mesh;
  /** The composited frame; also usable as a 2D preview (e.g. the equipment window). */
  readonly canvas = document.createElement("canvas");
  private readonly ctx: CanvasRenderingContext2D;
  private readonly texture: THREE.CanvasTexture;
  private drawn = "";
  private equipment: EquipLayers = noEquipment;

  constructor(private readonly assets: CharacterAssets) {
    this.canvas.width = this.canvas.height = CANVAS;
    this.ctx = this.canvas.getContext("2d", { willReadFrequently: true })!;
    this.texture = new THREE.CanvasTexture(this.canvas);
    this.texture.colorSpace = THREE.SRGBColorSpace;
    this.texture.magFilter = THREE.NearestFilter;
    this.texture.minFilter = THREE.LinearFilter;
    this.texture.generateMipmaps = false;

    const size = CANVAS * SPRITE_SCALE;
    const geo = new THREE.PlaneGeometry(size, size);
    // Put the feet pixel at the mesh origin.
    geo.translate((CANVAS / 2 - FOOT_X) * SPRITE_SCALE, (FOOT_Y - CANVAS / 2) * SPRITE_SCALE, 0);
    const mat = new THREE.MeshBasicMaterial({ map: this.texture, transparent: true, alphaTest: 0.05, depthWrite: false });
    this.object = new THREE.Mesh(geo, mat);
    this.object.renderOrder = 2;
  }

  /** Frame length in ms of an action group. */
  frameDelay(group: number): number {
    return this.assets.body.act.actions[group * 8]?.delay || 150;
  }

  frameCount(group: number): number {
    return this.assets.body.act.actions[group * 8]?.frames.length ?? 1;
  }

  /** Total length of an action group's animation in ms. */
  duration(group: number): number {
    return this.frameDelay(group) * this.frameCount(group);
  }

  /** The sound a body frame plays when shown (from the ACT), if any. */
  frameSound(group: number, dir: number, frame: number): string | null {
    const act = this.assets.body.act;
    const action = act.actions[group * 8 + dir];
    const f = action?.frames[frame % Math.max(1, action.frames.length)];
    return f && f.sound >= 0 ? (act.sounds[f.sound] ?? null) : null;
  }

  setEquipment(layers: EquipLayers): void {
    this.equipment = layers;
    this.drawn = ""; // force a redraw
  }

  /** dir is the sprite direction (0 = facing camera, clockwise); frame wraps. */
  draw(group: number, dir: number, frame: number): void {
    const key = `${group}:${dir}:${frame}`;
    if (key === this.drawn) return;
    this.drawn = key;

    const { body, head, shadow } = this.assets;
    const ctx = this.ctx;
    ctx.clearRect(0, 0, CANVAS, CANVAS);

    const shadowFrame = shadow.act.actions[0]?.frames[0];
    if (shadowFrame) drawFrame(ctx, shadow, shadowFrame, 0, 0);

    const frameOf = (set: SpriteSet | undefined) => {
      const action = set?.act.actions[group * 8 + dir];
      return action?.frames[frame % Math.max(1, action.frames.length)];
    };
    const { weapon, shield, headgear } = this.equipment;
    // Facing away (west through north-east), the shield arm is behind the body.
    const shieldBehind = dir >= 2 && dir <= 5;
    const shieldFrame = frameOf(shield);
    if (shield && shieldFrame && shieldBehind) drawFrame(ctx, shield, shieldFrame, 0, 0);

    const bodyFrame = frameOf(body);
    if (bodyFrame) drawFrame(ctx, body, bodyFrame, 0, 0);

    // Head and headgear hang off the body's anchor point.
    const anchored = (set: SpriteSet | undefined) => {
      const f = frameOf(set);
      if (!set || !f || !bodyFrame) return;
      const ba = bodyFrame.anchors[0] ?? { x: 0, y: 0 };
      const a = f.anchors[0] ?? { x: 0, y: 0 };
      drawFrame(ctx, set, f, ba.x - a.x, ba.y - a.y);
    };
    anchored(head);
    for (const hg of headgear) anchored(hg);

    const weaponFrame = frameOf(weapon);
    if (weapon && weaponFrame) drawFrame(ctx, weapon, weaponFrame, 0, 0);
    if (shield && shieldFrame && !shieldBehind) drawFrame(ctx, shield, shieldFrame, 0, 0);

    this.texture.needsUpdate = true;
  }

  /** Whether the pixel under a plane UV (from a raycast hit) is opaque. */
  hitTest(uv: THREE.Vector2): boolean {
    const x = Math.floor(uv.x * CANVAS);
    const y = Math.floor((1 - uv.y) * CANVAS);
    if (x < 0 || y < 0 || x >= CANVAS || y >= CANVAS) return false;
    return this.ctx.getImageData(x, y, 1, 1).data[3] > 32;
  }

  dispose(): void {
    this.object.geometry.dispose();
    (this.object.material as THREE.Material).dispose();
    this.texture.dispose();
  }
}

function drawFrame(ctx: CanvasRenderingContext2D, set: SpriteSet, frame: ActFrame, ox: number, oy: number) {
  for (const layer of frame.layers) {
    if (layer.sprite < 0) continue;
    const img = (layer.type === 1 ? set.rgba : set.indexed)[layer.sprite];
    if (!img) continue;
    ctx.save();
    ctx.translate(FOOT_X + layer.x + ox, FOOT_Y + layer.y + oy);
    if (layer.rotation) ctx.rotate((layer.rotation * Math.PI) / 180);
    ctx.scale(layer.scaleX * (layer.mirror ? -1 : 1), layer.scaleY);
    ctx.globalAlpha = layer.color[3] / 255;
    ctx.drawImage(img, -Math.floor(img.width / 2), -Math.floor(img.height / 2));
    ctx.restore();
  }
}
