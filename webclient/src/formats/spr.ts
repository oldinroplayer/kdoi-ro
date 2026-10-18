// .spr: sprite sheet of palette-indexed (RLE in 2.1+) and RGBA images.
import { Reader, ver } from "./reader";

export interface SprImage {
  width: number;
  height: number;
  rgba: Uint8ClampedArray; // straight RGBA, top-down
}

export interface Spr {
  indexed: SprImage[];
  rgba: SprImage[];
}

export function parseSpr(buf: ArrayBuffer): Spr {
  const r = new Reader(buf);
  r.magic("SP");
  const v = r.version();
  // Stored as minor then major for SPR, unlike the map formats.
  const version = ((v & 0xff) << 8) | (v >> 8);
  const indexedCount = r.u16();
  const rgbaCount = version >= ver(2, 0) ? r.u16() : 0;

  const raw: { w: number; h: number; px: Uint8Array }[] = [];
  for (let i = 0; i < indexedCount; i++) {
    const w = r.u16();
    const h = r.u16();
    if (version >= ver(2, 1)) {
      const size = r.u16();
      const src = r.bytes(size);
      const px = new Uint8Array(w * h);
      for (let s = 0, d = 0; s < size && d < px.length; ) {
        const c = src[s++];
        if (c === 0) {
          // Run of transparent pixels.
          d += src[s++];
        } else {
          px[d++] = c;
        }
      }
      raw.push({ w, h, px });
    } else {
      raw.push({ w, h, px: r.bytes(w * h).slice() });
    }
  }

  const rgba: SprImage[] = [];
  for (let i = 0; i < rgbaCount; i++) {
    const w = r.u16();
    const h = r.u16();
    // Stored ABGR, bottom-up.
    const src = r.bytes(w * h * 4);
    const out = new Uint8ClampedArray(w * h * 4);
    for (let y = 0; y < h; y++) {
      for (let x = 0; x < w; x++) {
        const s = ((h - 1 - y) * w + x) * 4;
        const d = (y * w + x) * 4;
        out[d] = src[s + 3];
        out[d + 1] = src[s + 2];
        out[d + 2] = src[s + 1];
        out[d + 3] = src[s];
      }
    }
    rgba.push({ width: w, height: h, rgba: out });
  }

  // Palette: last 1024 bytes, RGBA with unused alpha; index 0 is transparent.
  const pal = new Uint8Array(buf, buf.byteLength - 1024, 1024);
  const indexed = raw.map(({ w, h, px }) => {
    const out = new Uint8ClampedArray(w * h * 4);
    for (let i = 0; i < px.length; i++) {
      const c = px[i];
      if (c === 0) continue;
      out[i * 4] = pal[c * 4];
      out[i * 4 + 1] = pal[c * 4 + 1];
      out[i * 4 + 2] = pal[c * 4 + 2];
      out[i * 4 + 3] = 255;
    }
    return { width: w, height: h, rgba: out };
  });

  return { indexed, rgba };
}
