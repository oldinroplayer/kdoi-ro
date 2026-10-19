// Hit effect: the white shockwave ring RO draws when a melee attack lands.
// The 2004 client builds it in code from texture/effect/ring_white.tga, a strip
// that is solid at the bottom and spiky at the top: wrapped into a ring with the
// solid side inward, it gives the jagged white ellipse around the target.
import * as THREE from "three";
import { SPRITE_SCALE } from "./character";
import { loadTexture } from "./textures";

/** A short-lived effect in the scene. */
export interface Effect {
  readonly object: THREE.Object3D;
  /** Advances the effect; returns false once it has finished. */
  update(now: number, camera: THREE.Camera): boolean;
  dispose(): void;
}

const DURATION = 300; // ms
const START_RADIUS = 10; // sprite pixels
const END_RADIUS = 46;
const INNER = 0.55; // inner edge as a fraction of the outer radius
const SQUASH = 0.45; // the ring is seen at an angle, so it's an ellipse
const SEGMENTS = 40;
const TEXTURE_REPEATS = 5; // times the spiky strip wraps around the ring

let ringGeometry: THREE.BufferGeometry | null = null;

/** A unit annulus: v = 0 on the outer (spiky) edge, 1 on the inner (solid) edge. */
function ring(): THREE.BufferGeometry {
  if (ringGeometry) return ringGeometry;
  const pos: number[] = [];
  const uv: number[] = [];
  const index: number[] = [];
  for (let i = 0; i <= SEGMENTS; i++) {
    const a = (i / SEGMENTS) * Math.PI * 2;
    const c = Math.cos(a), s = Math.sin(a);
    const u = (i / SEGMENTS) * TEXTURE_REPEATS;
    pos.push(c, s, 0, c * INNER, s * INNER, 0);
    uv.push(u, 0, u, 1);
    if (i < SEGMENTS) {
      const o = i * 2;
      index.push(o, o + 1, o + 2, o + 2, o + 1, o + 3);
    }
  }
  ringGeometry = new THREE.BufferGeometry();
  ringGeometry.setAttribute("position", new THREE.Float32BufferAttribute(pos, 3));
  ringGeometry.setAttribute("uv", new THREE.Float32BufferAttribute(uv, 2));
  ringGeometry.setIndex(index);
  return ringGeometry;
}

export const loadRingTexture = () => loadTexture("texture/effect/ring_white.tga");

export class HitRing implements Effect {
  readonly object: THREE.Mesh;
  private readonly start = performance.now();
  /** Tilt of the ellipse on screen, so repeated hits don't look identical. */
  private readonly tilt: number;
  private readonly spin = new THREE.Quaternion();

  constructor(
    texture: THREE.Texture,
    /** Where the ring is centred (re-read every frame so it follows the target). */
    private readonly anchor: () => THREE.Vector3,
  ) {
    const mat = new THREE.MeshBasicMaterial({
      map: texture,
      transparent: true,
      depthWrite: false,
      depthTest: false,
      side: THREE.DoubleSide,
      blending: THREE.AdditiveBlending,
    });
    this.object = new THREE.Mesh(ring(), mat);
    this.object.renderOrder = 5;
    this.object.frustumCulled = false;
    const sign = Math.random() < 0.5 ? -1 : 1;
    this.tilt = sign * THREE.MathUtils.degToRad(30 + Math.random() * 30);
    this.spin.setFromAxisAngle(new THREE.Vector3(0, 0, 1), this.tilt);
  }

  update(now: number, camera: THREE.Camera): boolean {
    const t = (now - this.start) / DURATION;
    if (t >= 1) return false;
    const ease = 1 - (1 - t) * (1 - t); // fast out, slow in
    const r = (START_RADIUS + (END_RADIUS - START_RADIUS) * ease) * SPRITE_SCALE;
    this.object.position.copy(this.anchor());
    // Face the camera, then tilt the ellipse in screen space.
    this.object.quaternion.copy(camera.quaternion).multiply(this.spin);
    this.object.scale.set(r, r * SQUASH, 1);
    (this.object.material as THREE.MeshBasicMaterial).opacity = t < 0.15 ? 1 : 1 - (t - 0.15) / 0.85;
    return true;
  }

  dispose(): void {
    (this.object.material as THREE.Material).dispose(); // geometry and texture are shared
  }
}
