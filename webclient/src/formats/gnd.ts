// .gnd: ground mesh. A grid of "cubes" (each 2x2 GAT cells), each with corner
// heights and up to three textured surfaces: top, front (towards +y) and right (towards +x).
import { Reader, ver } from "./reader";

export interface GndSurface {
  u: [number, number, number, number];
  v: [number, number, number, number];
  texture: number;
  lightmap: number;
  color: [number, number, number, number]; // RGBA 0-255
}

export interface GndCube {
  heights: [number, number, number, number]; // SW, SE, NW, NE
  top: number;
  front: number;
  right: number;
}

export interface Gnd {
  width: number;
  height: number;
  zoom: number;
  textures: string[];
  lightmap: { count: number; cellW: number; cellH: number; data: Uint8Array };
  surfaces: GndSurface[];
  cubes: GndCube[];
}

export function parseGnd(buf: ArrayBuffer): Gnd {
  const r = new Reader(buf);
  r.magic("GRGN");
  const version = r.version();
  if (version < ver(1, 7)) throw new Error(`unsupported GND version ${version.toString(16)}`);

  const width = r.i32();
  const height = r.i32();
  const zoom = r.f32();

  const texCount = r.i32();
  const texNameLen = r.i32();
  const textures: string[] = [];
  for (let i = 0; i < texCount; i++) textures.push(r.str(texNameLen));

  const lmCount = r.i32();
  const cellW = r.i32();
  const cellH = r.i32();
  r.i32(); // grid size cell
  const lmSize = cellW * cellH * 4;
  const data = r.bytes(lmCount * lmSize).slice();

  const surfaceCount = r.i32();
  const surfaces: GndSurface[] = [];
  for (let i = 0; i < surfaceCount; i++) {
    const u: GndSurface["u"] = [r.f32(), r.f32(), r.f32(), r.f32()];
    const v: GndSurface["v"] = [r.f32(), r.f32(), r.f32(), r.f32()];
    const texture = r.i16();
    const lightmap = r.u16();
    const b = r.u8(), g = r.u8(), rr = r.u8(), a = r.u8();
    surfaces.push({ u, v, texture, lightmap, color: [rr, g, b, a] });
  }

  const cubes: GndCube[] = [];
  for (let i = 0; i < width * height; i++) {
    const heights: GndCube["heights"] = [r.f32(), r.f32(), r.f32(), r.f32()];
    cubes.push({ heights, top: r.i32(), front: r.i32(), right: r.i32() });
  }

  return { width, height, zoom, textures, lightmap: { count: lmCount, cellW, cellH, data }, surfaces, cubes };
}
