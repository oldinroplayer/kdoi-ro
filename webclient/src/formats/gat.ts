// .gat: per-cell walkability and heights. Cell (x, y) is at y*width + x, y = 0 at the south edge.
import { Reader } from "./reader";

export interface Gat {
  width: number;
  height: number;
  /** 4 corner heights per cell (SW, SE, NW, NE), RO units with +y pointing down. */
  heights: Float32Array;
  types: Uint8Array;
}

export function parseGat(buf: ArrayBuffer): Gat {
  const r = new Reader(buf);
  r.magic("GRAT");
  r.version();
  const width = r.i32();
  const height = r.i32();
  const heights = new Float32Array(width * height * 4);
  const types = new Uint8Array(width * height);
  for (let i = 0; i < width * height; i++) {
    for (let k = 0; k < 4; k++) heights[i * 4 + k] = r.f32();
    types[i] = r.u32();
  }
  return { width, height, heights, types };
}

export function isWalkable(g: Gat, x: number, y: number): boolean {
  if (x < 0 || y < 0 || x >= g.width || y >= g.height) return false;
  const t = g.types[y * g.width + x];
  return t === 0 || t === 3;
}

/** Ground height at a fractional cell position (bilinear over the cell's corners). */
export function heightAt(g: Gat, fx: number, fy: number): number {
  const x = Math.min(Math.max(Math.floor(fx), 0), g.width - 1);
  const y = Math.min(Math.max(Math.floor(fy), 0), g.height - 1);
  const u = Math.min(Math.max(fx - x, 0), 1);
  const v = Math.min(Math.max(fy - y, 0), 1);
  const i = (y * g.width + x) * 4;
  const h = g.heights;
  const south = h[i] * (1 - u) + h[i + 1] * u;
  const north = h[i + 2] * (1 - u) + h[i + 3] * u;
  return south * (1 - v) + north * v;
}
