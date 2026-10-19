// Loads BMP/JPG textures. RO uses pure magenta (255, 0, 255) as the transparent colour key.
import * as THREE from "three";
import { dataUrl } from "../assets";
import { parseTga } from "../formats/tga";

const cache = new Map<string, Promise<THREE.Texture>>();

/** path is relative to data/, e.g. "texture/모로코/foo.bmp". */
export function loadTexture(path: string): Promise<THREE.Texture> {
  const key = path.replace(/\\/g, "/").toLowerCase();
  let p = cache.get(key);
  if (!p) {
    p = load(key).catch((err) => {
      console.warn("texture missing", key, err);
      return missingTexture();
    });
    cache.set(key, p);
  }
  return p;
}

async function load(path: string): Promise<THREE.Texture> {
  const res = await fetch(dataUrl(path));
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  if (path.endsWith(".tga")) {
    // Browsers can't decode TGA; these carry real alpha, so no colour key.
    const img = parseTga(await res.arrayBuffer());
    const canvas = document.createElement("canvas");
    canvas.width = img.width;
    canvas.height = img.height;
    canvas.getContext("2d")!.putImageData(img, 0, 0);
    return configure(new THREE.CanvasTexture(canvas));
  }
  const bitmap = await createImageBitmap(await res.blob());
  const canvas = document.createElement("canvas");
  canvas.width = bitmap.width;
  canvas.height = bitmap.height;
  const ctx = canvas.getContext("2d", { willReadFrequently: true })!;
  ctx.drawImage(bitmap, 0, 0);
  bitmap.close();

  if (path.endsWith(".bmp")) {
    const img = ctx.getImageData(0, 0, canvas.width, canvas.height);
    const d = img.data;
    for (let i = 0; i < d.length; i += 4) {
      if (d[i] > 250 && d[i + 1] < 5 && d[i + 2] > 250) d[i + 3] = 0;
    }
    ctx.putImageData(img, 0, 0);
  }
  return configure(new THREE.CanvasTexture(canvas));
}

export function configure(tex: THREE.Texture): THREE.Texture {
  // RO texture coordinates have v = 0 at the top of the image.
  tex.flipY = false;
  tex.colorSpace = THREE.SRGBColorSpace;
  tex.wrapS = tex.wrapT = THREE.RepeatWrapping;
  tex.anisotropy = 4;
  tex.needsUpdate = true;
  return tex;
}

let missing: THREE.Texture | null = null;
function missingTexture(): THREE.Texture {
  if (!missing) {
    missing = new THREE.DataTexture(new Uint8Array([255, 255, 255, 255]), 1, 1);
    missing.needsUpdate = true;
  }
  return missing;
}
