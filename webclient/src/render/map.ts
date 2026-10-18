// Loads a whole map (ground, water, models) into a group that converts RO space to three.js.
import * as THREE from "three";
import { fetchData } from "../assets";
import { parseGat, type Gat } from "../formats/gat";
import { parseGnd } from "../formats/gnd";
import { parseRsw, type Rsw } from "../formats/rsw";
import { buildGround } from "./ground";
import type { SceneLight } from "./material";
import { clearModelCache, createInstance } from "./models";
import { loadTexture } from "./textures";

/** Size of one GAT cell in RO units (a GND tile is 2x2 cells). */
export const CELL = 5;

export interface MapView {
  name: string;
  gat: Gat;
  /** RO space, rotated 180° about X: RO (x, y-down, z-north) -> three (x, y-up, -z). */
  root: THREE.Group;
  ground: THREE.Mesh[];
  update(timeMs: number): void;
  dispose(): void;
}

/** RO-space point to three.js world space (matches MapView.root's rotation). */
export function roToWorld(x: number, y: number, z: number, out = new THREE.Vector3()): THREE.Vector3 {
  return out.set(x, -y, -z);
}

export function worldToRo(p: THREE.Vector3): [number, number, number] {
  return [p.x, -p.y, -p.z];
}

export async function loadMap(name: string, onProgress?: (msg: string) => void): Promise<MapView> {
  onProgress?.("Loading map data…");
  const [gatBuf, gndBuf, rswBuf] = await Promise.all([
    fetchData(`${name}.gat`),
    fetchData(`${name}.gnd`),
    fetchData(`${name}.rsw`),
  ]);
  const gat = parseGat(gatBuf);
  const gnd = parseGnd(gndBuf);
  const rsw = parseRsw(rswBuf);
  const light = sceneLight(rsw);

  const root = new THREE.Group();
  root.name = `map:${name}`;
  root.rotation.x = Math.PI;

  onProgress?.("Building ground…");
  const ground = await buildGround(gnd, light);
  for (const m of ground) root.add(m);

  onProgress?.(`Placing ${rsw.models.length} models…`);
  const center: [number, number] = [(gnd.width * gnd.zoom) / 2, (gnd.height * gnd.zoom) / 2];
  const instances = await Promise.all(rsw.models.map((m) => createInstance(m, center, light)));
  const models = new THREE.Group();
  models.name = "models";
  for (const obj of instances) if (obj) models.add(obj);
  root.add(models);

  const water = await buildWater(rsw, gnd.width * gnd.zoom, gnd.height * gnd.zoom);
  root.add(water.mesh);

  return {
    name,
    gat,
    root,
    ground,
    update: water.update,
    dispose() {
      root.traverse((o) => {
        if (o instanceof THREE.Mesh) o.geometry.dispose();
      });
      clearModelCache();
    },
  };
}

function sceneLight(rsw: Rsw): SceneLight {
  const lon = (rsw.light.longitude * Math.PI) / 180;
  const lat = (rsw.light.latitude * Math.PI) / 180;
  // Direction towards the light in RO space, converted to three.js space.
  const ro: [number, number, number] = [-Math.cos(lon) * Math.sin(lat), -Math.cos(lat), -Math.sin(lon) * Math.sin(lat)];
  return {
    direction: roToWorld(...ro).normalize(),
    ambient: new THREE.Color(...rsw.light.ambient),
    diffuse: new THREE.Color(...rsw.light.diffuse),
  };
}

/** Animated water plane; terrain above the water level hides it via the depth test. */
async function buildWater(rsw: Rsw, width: number, depth: number) {
  const frames = await Promise.all(
    Array.from({ length: 32 }, (_, i) =>
      loadTexture(`texture/워터/water${rsw.water.type}${String(i).padStart(2, "0")}.jpg`),
    ),
  );
  const tilesPerTexture = 4; // one water texture spans 4 GND tiles
  const geo = new THREE.PlaneGeometry(width, depth);
  geo.rotateX(Math.PI / 2); // into the RO x/z plane
  geo.translate(width / 2, rsw.water.level, depth / 2);
  const uv = geo.getAttribute("uv");
  for (let i = 0; i < uv.count; i++) {
    uv.setXY(i, (uv.getX(i) * width) / (10 * tilesPerTexture), (uv.getY(i) * depth) / (10 * tilesPerTexture));
  }
  const mat = new THREE.MeshBasicMaterial({ map: frames[0], transparent: true, opacity: 0.6, depthWrite: false, side: THREE.DoubleSide });
  const mesh = new THREE.Mesh(geo, mat);
  mesh.name = "water";
  mesh.renderOrder = 1;
  const frameMs = Math.max(1, rsw.water.animSpeed) * (1000 / 60);
  return {
    mesh,
    update(timeMs: number) {
      const tex = frames[Math.floor(timeMs / frameMs) % frames.length];
      if (mat.map !== tex) mat.map = tex;
    },
  };
}
