// Turns RSM models into three.js geometry and places RSW instances.
import * as THREE from "three";
import { fetchData } from "../assets";
import { parseRsm, type Rsm, type RsmNode } from "../formats/rsm";
import type { RswModel } from "../formats/rsw";
import { createMaterial, type SceneLight } from "./material";
import { loadTexture } from "./textures";

interface ModelPart {
  geometry: THREE.BufferGeometry;
  material: THREE.Material;
}

const cache = new Map<string, Promise<ModelPart[]>>();

export function loadModel(file: string, light: SceneLight): Promise<ModelPart[]> {
  const key = file.toLowerCase();
  let p = cache.get(key);
  if (!p) {
    p = fetchData(`model/${file}`)
      .then((buf) => buildModel(parseRsm(buf), light))
      .catch((err) => {
        console.warn("model failed", file, err);
        return [];
      });
    cache.set(key, p);
  }
  return p;
}

export function clearModelCache(): void {
  cache.clear();
}

/** Bakes the node hierarchy into one geometry per texture, in model space. */
async function buildModel(rsm: Rsm, light: SceneLight): Promise<ModelPart[]> {
  const only = rsm.nodes.length === 1;
  const local = new Map<RsmNode, THREE.Matrix4>();
  const box = new THREE.Box3();
  const v = new THREE.Vector3();

  const visit = (node: RsmNode, parent: THREE.Matrix4, depth: number) => {
    const m = parent.clone().multiply(new THREE.Matrix4().makeTranslation(...node.pos));
    if (node.rotKey) {
      const [x, y, z, w] = node.rotKey;
      m.multiply(new THREE.Matrix4().makeRotationFromQuaternion(new THREE.Quaternion(x, y, z, w).normalize()));
    } else {
      const axis = new THREE.Vector3(...node.rotAxis);
      if (axis.lengthSq() > 1e-8) m.multiply(new THREE.Matrix4().makeRotationAxis(axis.normalize(), node.rotAngle));
    }
    m.multiply(new THREE.Matrix4().makeScale(...node.scale));

    const l = m.clone();
    if (!only) l.multiply(new THREE.Matrix4().makeTranslation(...node.offset));
    const k = node.mat3; // column-major 3x3
    l.multiply(new THREE.Matrix4().set(k[0], k[3], k[6], 0, k[1], k[4], k[7], 0, k[2], k[5], k[8], 0, 0, 0, 0, 1));
    local.set(node, l);
    for (const p of node.vertices) box.expandByPoint(v.set(...p).applyMatrix4(l));

    if (depth > 32) return; // guard against malformed cycles
    for (const child of rsm.nodes) {
      if (child !== node && child.parent === node.name) visit(child, m, depth + 1);
    }
  };
  const main = rsm.nodes.find((n) => n.name === rsm.mainNode) ?? rsm.nodes[0];
  if (!main) return [];
  visit(main, new THREE.Matrix4(), 0);

  // Centre on x/z and stand the model's lowest point (max y, since y is down) at 0.
  const center = box.getCenter(new THREE.Vector3());
  const shift = new THREE.Matrix4().makeTranslation(-center.x, -box.max.y, -center.z);

  const buckets = new Map<number, { pos: number[]; uv: number[] }>();
  for (const [node, l] of local) {
    const m = shift.clone().multiply(l);
    for (const f of node.faces) {
      const tex = node.textures[f.tex] ?? 0;
      let b = buckets.get(tex);
      if (!b) buckets.set(tex, (b = { pos: [], uv: [] }));
      for (let i = 0; i < 3; i++) {
        const p = node.vertices[f.vert[i]];
        const t = node.tverts[f.tvert[i]] ?? [0, 0];
        if (!p) continue;
        v.set(...p).applyMatrix4(m);
        b.pos.push(v.x, v.y, v.z);
        b.uv.push(t[0], t[1]);
      }
    }
  }

  return Promise.all(
    [...buckets].map(async ([texIndex, b]) => {
      const tex = await loadTexture(`texture/${rsm.textures[texIndex] ?? ""}`);
      const geometry = new THREE.BufferGeometry();
      geometry.setAttribute("position", new THREE.Float32BufferAttribute(b.pos, 3));
      geometry.setAttribute("uv", new THREE.Float32BufferAttribute(b.uv, 2));
      const count = b.pos.length / 3;
      geometry.setAttribute("tint", new THREE.Float32BufferAttribute(new Float32Array(count * 3).fill(1), 3));
      geometry.setAttribute("lmuv", new THREE.Float32BufferAttribute(new Float32Array(count * 2), 2));
      geometry.computeVertexNormals(); // non-indexed, so these are flat face normals
      geometry.computeBoundingSphere();
      return { geometry, material: createMaterial(tex, light, { opacity: rsm.alpha }) };
    }),
  );
}

/** Places an RSW model instance. Positions are relative to the map centre. */
export async function createInstance(
  inst: RswModel,
  mapCenter: [number, number],
  light: SceneLight,
): Promise<THREE.Object3D | null> {
  const parts = await loadModel(inst.file, light);
  if (parts.length === 0) return null;
  const obj = new THREE.Group();
  obj.name = inst.name;
  const deg = Math.PI / 180;
  obj.matrixAutoUpdate = false;
  obj.matrix
    .makeTranslation(inst.position[0] + mapCenter[0], inst.position[1], inst.position[2] + mapCenter[1])
    .multiply(new THREE.Matrix4().makeRotationZ(inst.rotation[2] * deg))
    .multiply(new THREE.Matrix4().makeRotationX(inst.rotation[0] * deg))
    .multiply(new THREE.Matrix4().makeRotationY(inst.rotation[1] * deg))
    .multiply(new THREE.Matrix4().makeScale(...inst.scale));
  for (const part of parts) obj.add(new THREE.Mesh(part.geometry, part.material));
  return obj;
}
