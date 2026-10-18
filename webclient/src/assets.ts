// Fetches extracted client files from /data (public/data -> ro-client/data).
//
// RO references use backslashes and mixed case; extracted files are lowercase.

export function dataUrl(path: string): string {
  const clean = path.replace(/\\/g, "/").replace(/^\/+/, "").toLowerCase();
  return encodeURI(`/data/${clean}`);
}

export async function fetchData(path: string): Promise<ArrayBuffer> {
  const res = await fetch(dataUrl(path));
  if (!res.ok) throw new Error(`${path}: HTTP ${res.status}`);
  return res.arrayBuffer();
}

/**
 * Loads a BMP/JPG into a canvas, making RO's magenta key colour (255, 0, 255)
 * transparent. Resolves to null if the file is missing.
 */
export async function loadKeyedCanvas(path: string): Promise<HTMLCanvasElement | null> {
  const res = await fetch(dataUrl(path));
  if (!res.ok) return null;
  const bitmap = await createImageBitmap(await res.blob());
  const canvas = document.createElement("canvas");
  canvas.width = bitmap.width;
  canvas.height = bitmap.height;
  const ctx = canvas.getContext("2d")!;
  ctx.drawImage(bitmap, 0, 0);
  bitmap.close();
  const img = ctx.getImageData(0, 0, canvas.width, canvas.height);
  const d = img.data;
  for (let i = 0; i < d.length; i += 4) {
    if (d[i] > 250 && d[i + 1] < 5 && d[i + 2] > 250) d[i + 3] = 0;
  }
  ctx.putImageData(img, 0, 0);
  return canvas;
}
