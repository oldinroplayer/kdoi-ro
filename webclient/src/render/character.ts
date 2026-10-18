// Character sprites: body + head + shadow composited on a canvas shown on a
// camera-facing billboard, like the original client's 2D-in-3D characters.
import * as THREE from "three";
import { fetchData } from "../assets";
import { parseAct, type Act, type ActFrame } from "../formats/act";
import { parseSpr } from "../formats/spr";

/** World units per sprite pixel. */
export const SPRITE_SCALE = 1 / 7;

const CANVAS = 256;
/** Where the character's feet are on the canvas. */
const FOOT_X = 128;
const FOOT_Y = 176;

/** Animation state; the ACT action index is state * 8 + direction. */
export const State = { Idle: 0, Walk: 1 } as const;
export type State = (typeof State)[keyof typeof State];

interface SpriteSet {
  act: Act;
  indexed: HTMLCanvasElement[];
  rgba: HTMLCanvasElement[];
}

async function loadSpriteSet(path: string): Promise<SpriteSet> {
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
  head: SpriteSet;
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

export class CharacterSprite {
  readonly object: THREE.Mesh;
  private readonly canvas = document.createElement("canvas");
  private readonly ctx: CanvasRenderingContext2D;
  private readonly texture: THREE.CanvasTexture;
  private drawn = "";

  constructor(
    private readonly assets: CharacterAssets,
    private readonly name: string,
    private readonly isSelf: boolean,
  ) {
    this.canvas.width = this.canvas.height = CANVAS;
    this.ctx = this.canvas.getContext("2d")!;
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

  /** Frame length in ms for the current state, scaled by walk speed (150 = normal). */
  frameDelay(state: State, speed: number): number {
    const action = this.assets.body.act.actions[state * 8];
    const base = action?.delay ?? 150;
    return state === State.Walk ? (base * speed) / 150 : base;
  }

  frameCount(state: State): number {
    return this.assets.body.act.actions[state * 8]?.frames.length ?? 1;
  }

  /** dir is the sprite direction (0 = facing camera, clockwise). */
  draw(state: State, dir: number, frame: number): void {
    const key = `${state}:${dir}:${frame}`;
    if (key === this.drawn) return;
    this.drawn = key;

    const { body, head, shadow } = this.assets;
    const ctx = this.ctx;
    ctx.clearRect(0, 0, CANVAS, CANVAS);

    const shadowFrame = shadow.act.actions[0]?.frames[0];
    if (shadowFrame) drawFrame(ctx, shadow, shadowFrame, 0, 0);

    const bodyAction = body.act.actions[state * 8 + dir];
    // Idle frames choose the head's facing; always look straight ahead.
    const bodyFrame = bodyAction?.frames[state === State.Idle ? 0 : frame % bodyAction.frames.length];
    if (bodyFrame) drawFrame(ctx, body, bodyFrame, 0, 0);

    const headAction = head.act.actions[state * 8 + dir];
    const headFrame = headAction?.frames[state === State.Idle ? 0 : frame % headAction.frames.length];
    if (headFrame && bodyFrame) {
      const ba = bodyFrame.anchors[0] ?? { x: 0, y: 0 };
      const ha = headFrame.anchors[0] ?? { x: 0, y: 0 };
      drawFrame(ctx, head, headFrame, ba.x - ha.x, ba.y - ha.y);
    }

    ctx.font = "bold 12px system-ui, sans-serif";
    ctx.textAlign = "center";
    ctx.textBaseline = "top";
    ctx.lineWidth = 3;
    ctx.strokeStyle = "rgba(0,0,0,0.85)";
    ctx.fillStyle = this.isSelf ? "#ffffff" : "#e8f0ff";
    ctx.strokeText(this.name, FOOT_X, FOOT_Y + 12);
    ctx.fillText(this.name, FOOT_X, FOOT_Y + 12);

    this.texture.needsUpdate = true;
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
