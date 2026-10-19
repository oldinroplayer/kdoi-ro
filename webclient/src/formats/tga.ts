// .tga: Truevision images (RO uses them for effect textures with alpha).
// Supports uncompressed (type 2) and RLE (type 10) true-colour, 24/32 bpp.

export function parseTga(buf: ArrayBuffer): ImageData {
  const d = new Uint8Array(buf);
  const idLength = d[0];
  const type = d[2];
  const w = d[12] | (d[13] << 8);
  const h = d[14] | (d[15] << 8);
  const bpp = d[16];
  const topDown = (d[17] & 0x20) !== 0;
  if ((type !== 2 && type !== 10) || (bpp !== 24 && bpp !== 32)) {
    throw new Error(`unsupported TGA (type ${type}, ${bpp} bpp)`);
  }
  const bytes = bpp / 8;
  const pixels = new Uint8ClampedArray(w * h * 4); // in file order (BGRA)
  let src = 18 + idLength;
  const put = (i: number, at: number) => {
    pixels[i * 4] = d[at + 2];
    pixels[i * 4 + 1] = d[at + 1];
    pixels[i * 4 + 2] = d[at];
    pixels[i * 4 + 3] = bytes === 4 ? d[at + 3] : 255;
  };
  if (type === 2) {
    for (let i = 0; i < w * h; i++, src += bytes) put(i, src);
  } else {
    for (let i = 0; i < w * h; ) {
      const header = d[src++];
      const count = (header & 0x7f) + 1;
      if (header & 0x80) {
        for (let k = 0; k < count && i < w * h; k++) put(i++, src);
        src += bytes;
      } else {
        for (let k = 0; k < count && i < w * h; k++, src += bytes) put(i++, src);
      }
    }
  }

  // Return top-down rows.
  const out = new ImageData(w, h);
  for (let y = 0; y < h; y++) {
    const from = (topDown ? y : h - 1 - y) * w * 4;
    out.data.set(pixels.subarray(from, from + w * 4), y * w * 4);
  }
  return out;
}
