// Builds the GND ground mesh in RO space (x east, y down, z north).
import * as THREE from "three";
import type { Gnd, GndSurface } from "../formats/gnd";
import { createMaterial, type SceneLight } from "./material";
import { loadTexture } from "./textures";

interface Bucket {
  pos: number[];
  uv: number[];
  lmuv: number[];
  tint: number[];
  normal: number[];
}

type Corner = [number, number, number]; // RO-space position

export async function buildGround(gnd: Gnd, light: SceneLight): Promise<THREE.Mesh[]> {
  const lm = buildLightmapAtlas(gnd);
  const buckets = new Map<number, Bucket>();
  const z = gnd.zoom;

  const quad = (s: GndSurface, c: [Corner, Corner, Corner, Corner]) => {
    if (s.texture < 0) return;
    let b = buckets.get(s.texture);
    if (!b) buckets.set(s.texture, (b = { pos: [], uv: [], lmuv: [], tint: [], normal: [] }));

    const n = faceNormal(c[0], c[1], c[2]);
    const lmUv = lm.uv(s.lightmap);
    for (const i of [0, 1, 2, 2, 1, 3]) {
      b.pos.push(...c[i]);
      b.uv.push(s.u[i], s.v[i]);
      b.lmuv.push(...lmUv[i]);
      b.tint.push(s.color[0] / 255, s.color[1] / 255, s.color[2] / 255);
      b.normal.push(...n);
    }
  };

  for (let y = 0; y < gnd.height; y++) {
    for (let x = 0; x < gnd.width; x++) {
      const cube = gnd.cubes[y * gnd.width + x];
      const h = cube.heights;
      const x0 = x * z, x1 = (x + 1) * z;
      const z0 = y * z, z1 = (y + 1) * z;

      if (cube.top >= 0) {
        quad(gnd.surfaces[cube.top], [[x0, h[0], z0], [x1, h[1], z0], [x0, h[2], z1], [x1, h[3], z1]]);
      }
      if (cube.front >= 0 && y + 1 < gnd.height) {
        const nh = gnd.cubes[(y + 1) * gnd.width + x].heights;
        quad(gnd.surfaces[cube.front], [[x0, h[2], z1], [x1, h[3], z1], [x0, nh[0], z1], [x1, nh[1], z1]]);
      }
      if (cube.right >= 0 && x + 1 < gnd.width) {
        const nh = gnd.cubes[y * gnd.width + x + 1].heights;
        quad(gnd.surfaces[cube.right], [[x1, h[3], z1], [x1, h[1], z0], [x1, nh[2], z1], [x1, nh[0], z0]]);
      }
    }
  }

  const meshes: THREE.Mesh[] = [];
  await Promise.all(
    [...buckets].map(async ([texIndex, b]) => {
      const tex = await loadTexture(`texture/${gnd.textures[texIndex]}`);
      const geo = new THREE.BufferGeometry();
      geo.setAttribute("position", new THREE.Float32BufferAttribute(b.pos, 3));
      geo.setAttribute("uv", new THREE.Float32BufferAttribute(b.uv, 2));
      geo.setAttribute("lmuv", new THREE.Float32BufferAttribute(b.lmuv, 2));
      geo.setAttribute("tint", new THREE.Float32BufferAttribute(b.tint, 3));
      geo.setAttribute("normal", new THREE.Float32BufferAttribute(b.normal, 3));
      geo.computeBoundingSphere();
      const mesh = new THREE.Mesh(geo, createMaterial(tex, light, { lightmap: lm.texture, alphaTest: 0.01 }));
      mesh.name = "ground";
      meshes.push(mesh);
    }),
  );
  return meshes;
}

function faceNormal(a: Corner, b: Corner, c: Corner): Corner {
  const v = new THREE.Vector3(...b).sub(new THREE.Vector3(...a));
  const w = new THREE.Vector3(...c).sub(new THREE.Vector3(...a));
  const n = v.cross(w).normalize();
  return [n.x, n.y, n.z];
}

/** Packs every 8x8 lightmap into one texture. Alpha holds shadow, RGB coloured light. */
function buildLightmapAtlas(gnd: Gnd) {
  const { count, cellW, cellH, data } = gnd.lightmap;
  const per = cellW * cellH;
  const cols = Math.max(1, Math.ceil(Math.sqrt(count)));
  const rows = Math.max(1, Math.ceil(count / cols));
  const W = cols * cellW, H = rows * cellH;
  const px = new Uint8Array(W * H * 4);

  for (let i = 0; i < count; i++) {
    const base = i * per * 4;
    const ox = (i % cols) * cellW, oy = Math.floor(i / cols) * cellH;
    for (let j = 0; j < per; j++) {
      const d = ((oy + Math.floor(j / cellW)) * W + ox + (j % cellW)) * 4;
      px[d] = data[base + per + j * 3];
      px[d + 1] = data[base + per + j * 3 + 1];
      px[d + 2] = data[base + per + j * 3 + 2];
      px[d + 3] = data[base + j];
    }
  }

  const texture = new THREE.DataTexture(px, W, H);
  texture.magFilter = THREE.LinearFilter;
  texture.minFilter = THREE.LinearFilter;
  texture.needsUpdate = true;

  // The outer texel of each lightmap is padding; sample the inner 6x6.
  const uv = (id: number): [number, number][] => {
    const cx = (id % cols) * cellW, cy = Math.floor(id / cols) * cellH;
    const u0 = (cx + 1) / W, u1 = (cx + cellW - 1) / W;
    const v0 = (cy + 1) / H, v1 = (cy + cellH - 1) / H;
    return [[u0, v0], [u1, v0], [u0, v1], [u1, v1]];
  };
  return { texture, uv };
}
