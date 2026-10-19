// Loads a whole map (ground, water, models) into a group that converts RO space to three.js.
import * as THREE from "three";
import { fetchData } from "../assets";
import { parseGat, type Gat } from "../formats/gat";
import { parseGnd, type Gnd } from "../formats/gnd";
import { parseRsw, type Rsw, type RswSound } from "../formats/rsw";
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
  /** Ambient sounds with positions in RO space (same units as cells × CELL). */
  sounds: (RswSound & { x: number; z: number })[];
  /** Placed effects (torch fire...) with positions in RO space. */
  effects: { id: number; x: number; y: number; z: number }[];
  update(timeMs: number): void;
  /** The scene light every map material uses. */
  light: SceneLight;
  /** The map's fog colour and strength from data/fogparametertable.txt, or null. */
  fog: MapFog | null;
  /** The light falling on a cell (map light, the ground's baked shadow and
   * coloured light there), for shading sprites standing on it. */
  lightAt(x: number, y: number, out: THREE.Color): THREE.Color;
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
  const fog = (await loadFogTable()).get(name.toLowerCase()) ?? null;
  const cellLight = buildCellLight(gnd, light);

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

  const sounds = rsw.sounds.map((s) => ({ ...s, x: s.position[0] + center[0], z: s.position[2] + center[1] }));
  const effects = rsw.effects.map((e) => ({
    id: e.id,
    x: e.position[0] + center[0],
    y: e.position[1],
    z: e.position[2] + center[1],
  }));

  return {
    name,
    gat,
    root,
    ground,
    sounds,
    effects,
    update: water.update,
    light,
    fog,
    lightAt: cellLight,
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

/** A map's entry in data/fogparametertable.txt. */
export interface MapFog {
  color: THREE.Color;
  /** How strongly the colour covers the scene (0.3 in Payon Cave). */
  strength: number;
}

let fogTable: Promise<Map<string, MapFog>> | null = null;

/** Parses the client's fog table: "map.rsw#", start#, end#, 0xAARRGGBB#, strength# per map. */
function loadFogTable(): Promise<Map<string, MapFog>> {
  fogTable ??= fetchData("fogparametertable.txt")
    .then((buf) => {
      const fields = new TextDecoder("euc-kr").decode(buf).split("#").map((f) => f.trim()).filter(Boolean);
      const out = new Map<string, MapFog>();
      for (let i = 0; i + 4 < fields.length; i += 5) {
        const argb = parseInt(fields[i + 3], 16);
        out.set(fields[i].toLowerCase().replace(/\.rsw$/, ""), {
          color: new THREE.Color().setHex(argb & 0xffffff, THREE.SRGBColorSpace),
          strength: Number(fields[i + 4]),
        });
      }
      return out;
    })
    .catch((err) => {
      console.warn("fog table missing", err);
      return new Map();
    });
  return fogTable;
}

/**
 * Light per GAT cell for sprites, as RO shades characters: the map's ambient
 * plus some diffuse light, dimmed by the ground's baked shadow under them and
 * brightened by its baked coloured light. Each GND cube covers 2x2 cells.
 */
function buildCellLight(gnd: Gnd, light: SceneLight) {
  const { cellW, cellH, data } = gnd.lightmap;
  const per = cellW * cellH;
  const shadow = new Float32Array(gnd.width * gnd.height).fill(1);
  const glow = new Float32Array(gnd.width * gnd.height * 3);
  gnd.cubes.forEach((cube, i) => {
    if (cube.top < 0) return;
    const lm = gnd.surfaces[cube.top].lightmap;
    if (lm < 0 || lm >= gnd.lightmap.count) return;
    // Average the inner texels (the outer ring is padding).
    let a = 0, r = 0, g = 0, b = 0, n = 0;
    for (let ty = 1; ty < cellH - 1; ty++) {
      for (let tx = 1; tx < cellW - 1; tx++) {
        const j = ty * cellW + tx;
        a += data[lm * per * 4 + j];
        r += data[lm * per * 4 + per + j * 3];
        g += data[lm * per * 4 + per + j * 3 + 1];
        b += data[lm * per * 4 + per + j * 3 + 2];
        n++;
      }
    }
    shadow[i] = a / n / 255;
    glow.set([r / n / 255, g / n / 255, b / n / 255], i * 3);
  });
  // Sprites face the camera, so they get about half the directional light.
  const base = light.ambient.clone().add(light.diffuse.clone().multiplyScalar(0.5));
  base.r = Math.min(base.r, 1);
  base.g = Math.min(base.g, 1);
  base.b = Math.min(base.b, 1);
  return (x: number, y: number, out: THREE.Color): THREE.Color => {
    const cx = Math.min(Math.max(Math.floor(x / 2), 0), gnd.width - 1);
    const cy = Math.min(Math.max(Math.floor(y / 2), 0), gnd.height - 1);
    const i = cy * gnd.width + cx;
    const s = shadow[i];
    return out.setRGB(
      Math.min(base.r * s + glow[i * 3], 1),
      Math.min(base.g * s + glow[i * 3 + 1], 1),
      Math.min(base.b * s + glow[i * 3 + 2], 1),
      THREE.LinearSRGBColorSpace,
    );
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
