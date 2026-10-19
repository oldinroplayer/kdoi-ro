// STR: RO's keyframed 2D effect animations (texture/effect/*.str), e.g. the
// fire burst of Fire Bolt or the ice of Cold Bolt. An effect is a stack of
// layers; each layer is a textured quad whose position, corners, angle, colour
// and texture frame are keyframed at `fps` frames per second.
import { Reader } from "./reader";

/** One keyframe. type 0 holds absolute values; a type 1 frame right after it
 * holds per-frame deltas that animate it until the next keyframe. */
export interface StrKey {
  frame: number;
  type: number;
  /** Centre on a 640x640 screen whose middle (320, 320) is the anchor. */
  x: number;
  y: number;
  /** Texture rectangle: left, top, right, bottom. */
  uv: [number, number, number, number];
  /** Corner offsets from the centre: x0..x3 then y0..y3 (TL, TR, BR, BL). */
  xy: number[];
  /** Texture index (fractional while animating). */
  texFrame: number;
  texAnim: number;
  delay: number;
  /** Rotation, 1024 = a full turn. */
  angle: number;
  /** RGBA, 0-255. */
  color: [number, number, number, number];
  /** Direct3D blend factors. */
  srcBlend: number;
  dstBlend: number;
}

export interface StrLayer {
  textures: string[];
  keys: StrKey[];
}

export interface Str {
  fps: number;
  frames: number;
  layers: StrLayer[];
}

const latin = new TextDecoder("euc-kr");

export function parseStr(buf: ArrayBuffer): Str {
  const r = new Reader(buf);
  if (r.str(4) !== "STRM") throw new Error("not an STR file");
  r.u32(); // version (0x94)
  const fps = r.u32();
  const frames = r.u32();
  const layerCount = r.u32();
  r.skip(16);
  const layers: StrLayer[] = [];
  for (let l = 0; l < layerCount; l++) {
    const textures: string[] = [];
    const texCount = r.i32();
    for (let i = 0; i < texCount; i++) {
      const raw = r.bytes(128);
      const end = raw.indexOf(0);
      textures.push(latin.decode(raw.subarray(0, end < 0 ? 128 : end)));
    }
    const keys: StrKey[] = [];
    const keyCount = r.i32();
    for (let i = 0; i < keyCount; i++) {
      const frame = r.i32(), type = r.i32();
      const x = r.f32(), y = r.f32();
      const uvs = Array.from({ length: 8 }, () => r.f32());
      const xy = Array.from({ length: 8 }, () => r.f32());
      const texFrame = r.f32();
      const texAnim = r.u32();
      const delay = r.f32();
      const angle = r.f32();
      const color: [number, number, number, number] = [r.f32(), r.f32(), r.f32(), r.f32()];
      const srcBlend = r.u32(), dstBlend = r.u32();
      r.u32(); // mtpreset
      keys.push({
        frame, type, x, y, uv: [uvs[0], uvs[1], uvs[2], uvs[3]], xy, texFrame, texAnim, delay, angle, color, srcBlend, dstBlend,
      });
    }
    layers.push({ textures, keys });
  }
  return { fps, frames, layers };
}

/** A layer's state at a frame, or null when it isn't drawn then. */
export function layerAt(layer: StrLayer, frame: number): StrKey | null {
  const keys = layer.keys;
  if (keys.length === 0) return null;
  let start = -1;
  let next = -1;
  let last = 0;
  for (let i = 0; i < keys.length; i++) {
    const k = keys[i];
    last = Math.max(last, k.frame);
    if (k.frame <= frame) {
      if (k.type === 0) start = i;
      else if (k.type === 1) next = i;
    }
  }
  if (start < 0 || (next < 0 && last < frame)) return null;
  const from = keys[start];
  // No delta key for this keyframe: it holds still until the next keyframe.
  if (next !== start + 1 || keys[next].frame !== from.frame) {
    if (next >= 0 && last <= frame) return null;
    return from;
  }
  const d = keys[next];
  const t = frame - from.frame;
  const count = Math.max(layer.textures.length, 1);
  let texFrame = from.texFrame;
  switch (d.texAnim) {
    case 1:
      texFrame = from.texFrame + d.texFrame * t;
      break;
    case 2:
      texFrame = Math.min(from.texFrame + d.delay * t, count - 1);
      break;
    case 3:
      texFrame = (from.texFrame + d.delay * t) % count;
      break;
    case 4:
      texFrame = (((from.texFrame - d.delay * t) % count) + count) % count;
      break;
  }
  return {
    ...from,
    x: from.x + d.x * t,
    y: from.y + d.y * t,
    xy: from.xy.map((v, i) => v + d.xy[i] * t),
    angle: from.angle + d.angle * t,
    color: from.color.map((v, i) => v + d.color[i] * t) as StrKey["color"],
    texFrame,
  };
}
