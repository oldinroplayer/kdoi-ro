// Effects the 2004 client draws in code rather than from an STR file, built
// from the textures it uses for them (texture/effect/, per roBrowser's effect
// table notes): the arrow streak of bow shots (allow_a.bmp), Heal's light
// (alpha_down.tga), Bash's flash (alpha_center.tga), Increase AGI's streaks
// (ac_center2.tga) and Magnum Break's blast (대폭발.tga).
import * as THREE from "three";
import { SPRITE_SCALE } from "./character";
import type { Effect } from "./effect";
import { CELL } from "./map";
import type { Anchor } from "./skilleffects";

const additive = (map: THREE.Texture, color: THREE.ColorRepresentation = 0xffffff) =>
  new THREE.MeshBasicMaterial({
    map,
    color,
    transparent: true,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
    side: THREE.DoubleSide,
  });

/** Eases 0..1 quickly out (fast start, slow finish). */
const easeOut = (t: number) => 1 - Math.pow(1 - Math.min(Math.max(t, 0), 1), 3);

/**
 * A bow shot: the glowing arrow streak (allow_a.bmp, bright head to the right)
 * flying from the archer to the target, turned along its path on screen.
 */
export class ArrowStreak implements Effect {
  readonly object: THREE.Mesh;
  private readonly start = performance.now();
  private readonly a = new THREE.Vector3();
  private readonly b = new THREE.Vector3();
  private readonly spin = new THREE.Quaternion();
  private readonly axis = new THREE.Vector3(0, 0, 1);

  constructor(
    texture: THREE.Texture,
    private readonly from: Anchor,
    private readonly to: Anchor,
    private readonly ms: number,
    private readonly delay = 0,
  ) {
    const geo = new THREE.PlaneGeometry(128 * SPRITE_SCALE * 0.7, 24 * SPRITE_SCALE * 0.7);
    // The head (right end of the texture) leads: put it at the mesh origin.
    geo.translate((-128 * SPRITE_SCALE * 0.7) / 2, 0, 0);
    this.object = new THREE.Mesh(geo, additive(texture));
    this.object.renderOrder = 6;
    this.object.visible = false;
  }

  update(now: number, camera: THREE.Camera): boolean {
    const t = (now - this.start - this.delay) / this.ms;
    if (t >= 1) return false;
    if (t < 0) return true;
    this.object.visible = true;
    this.a.copy(this.from());
    this.b.copy(this.to());
    this.object.position.lerpVectors(this.a, this.b, t);
    // Angle of the flight on screen.
    const pa = this.a.clone().project(camera);
    const pb = this.b.clone().project(camera);
    const aspect = (camera as THREE.PerspectiveCamera).aspect ?? 1;
    const angle = Math.atan2(pb.y - pa.y, (pb.x - pa.x) * aspect);
    this.object.quaternion.copy(camera.quaternion).multiply(this.spin.setFromAxisAngle(this.axis, angle));
    return true;
  }

  dispose(): void {
    this.object.geometry.dispose();
    (this.object.material as THREE.Material).dispose();
  }
}

/**
 * Heal: a column of light on the target from alpha_down.tga (bright at the
 * bottom, fading upward), shooting up and fading while it turns.
 */
export class FadeColumn implements Effect {
  readonly object = new THREE.Group();
  private readonly start = performance.now();
  private readonly meshes: THREE.Mesh[] = [];

  constructor(
    texture: THREE.Texture,
    private readonly anchor: Anchor,
    color: THREE.ColorRepresentation,
    private readonly ms = 1000,
  ) {
    const height = 110 * SPRITE_SCALE;
    for (const radius of [CELL * 0.85, CELL * 0.5]) {
      const geo = new THREE.CylinderGeometry(radius, radius, height, 28, 1, true);
      geo.translate(0, height / 2, 0);
      const mesh = new THREE.Mesh(geo, additive(texture, color));
      mesh.renderOrder = 4;
      this.meshes.push(mesh);
      this.object.add(mesh);
    }
  }

  update(now: number): boolean {
    const t = (now - this.start) / this.ms;
    if (t >= 1) return false;
    this.object.position.copy(this.anchor());
    const rise = easeOut(t / 0.25);
    const fade = t < 0.5 ? 1 : 1 - (t - 0.5) / 0.5;
    this.meshes.forEach((m, i) => {
      m.scale.set(1, Math.max(rise, 0.01), 1);
      m.rotation.y = (i ? -1 : 1) * t * 2;
      (m.material as THREE.MeshBasicMaterial).opacity = fade;
    });
    return true;
  }

  dispose(): void {
    for (const m of this.meshes) {
      m.geometry.dispose();
      (m.material as THREE.Material).dispose();
    }
  }
}

/**
 * Increase AGI: thin bands of light (ac_center2.tga, bright down the middle)
 * shooting up around the target one after another, then fading.
 */
export class RisingStreaks implements Effect {
  readonly object = new THREE.Group();
  private readonly start = performance.now();
  private readonly streaks: { mesh: THREE.Mesh; at: number; angle: number }[] = [];

  constructor(
    texture: THREE.Texture,
    private readonly anchor: Anchor,
    color: THREE.ColorRepresentation,
    private readonly ms = 1100,
    count = 10,
  ) {
    for (let i = 0; i < count; i++) {
      const height = (60 + Math.random() * 50) * SPRITE_SCALE;
      const geo = new THREE.PlaneGeometry(9 * SPRITE_SCALE, height);
      geo.translate(0, height / 2, 0);
      const mesh = new THREE.Mesh(geo, additive(texture, color));
      mesh.renderOrder = 4;
      this.streaks.push({ mesh, at: (i / count) * 0.45, angle: (i / count) * Math.PI * 2 + Math.random() * 0.5 });
      this.object.add(mesh);
    }
  }

  update(now: number, camera: THREE.Camera): boolean {
    const t = (now - this.start) / this.ms;
    if (t >= 1) return false;
    this.object.position.copy(this.anchor());
    const radius = CELL * 0.7;
    for (const s of this.streaks) {
      const local = (t - s.at) / 0.55; // each streak's own 0..1
      s.mesh.visible = local > 0 && local < 1;
      if (!s.mesh.visible) continue;
      s.mesh.position.set(Math.cos(s.angle) * radius, easeOut(local) * 40 * SPRITE_SCALE, Math.sin(s.angle) * radius);
      s.mesh.scale.set(1, 0.3 + easeOut(local / 0.4) * 0.7, 1);
      // Face the camera around the vertical axis.
      s.mesh.rotation.set(0, Math.atan2(camera.position.x - this.object.position.x, camera.position.z - this.object.position.z), 0);
      (s.mesh.material as THREE.MeshBasicMaterial).opacity = local < 0.6 ? 1 : 1 - (local - 0.6) / 0.4;
    }
    return true;
  }

  dispose(): void {
    for (const s of this.streaks) {
      s.mesh.geometry.dispose();
      (s.mesh.material as THREE.Material).dispose();
    }
  }
}

/**
 * Bash: a bright flash across the target (alpha_center.tga, a band bright
 * along its middle) that snaps wide, with a thinner upright one, and fades.
 */
export class FlashCross implements Effect {
  readonly object = new THREE.Group();
  private readonly start = performance.now();
  private readonly wide: THREE.Mesh;
  private readonly tall: THREE.Mesh;

  constructor(
    texture: THREE.Texture,
    private readonly anchor: Anchor,
    color: THREE.ColorRepresentation,
    private readonly ms = 380,
  ) {
    this.wide = new THREE.Mesh(new THREE.PlaneGeometry(120 * SPRITE_SCALE, 26 * SPRITE_SCALE), additive(texture, color));
    const upright = texture.clone();
    upright.center.set(0.5, 0.5);
    upright.rotation = Math.PI / 2;
    upright.needsUpdate = true;
    this.tall = new THREE.Mesh(new THREE.PlaneGeometry(18 * SPRITE_SCALE, 80 * SPRITE_SCALE), additive(upright, color));
    for (const m of [this.wide, this.tall]) {
      m.renderOrder = 6;
      (m.material as THREE.MeshBasicMaterial).depthTest = false;
      this.object.add(m);
    }
  }

  update(now: number, camera: THREE.Camera): boolean {
    const t = (now - this.start) / this.ms;
    if (t >= 1) return false;
    this.object.position.copy(this.anchor());
    this.object.quaternion.copy(camera.quaternion);
    const grow = easeOut(t / 0.35);
    this.wide.scale.set(0.2 + grow * 0.8, 1 - t * 0.5, 1);
    this.tall.scale.set(1 - t * 0.5, 0.3 + grow * 0.7, 1);
    const fade = 1 - t * t;
    for (const m of [this.wide, this.tall]) (m.material as THREE.MeshBasicMaterial).opacity = fade;
    return true;
  }

  dispose(): void {
    for (const m of [this.wide, this.tall]) {
      m.geometry.dispose();
      const mat = m.material as THREE.MeshBasicMaterial;
      if (m === this.tall) mat.map?.dispose(); // the rotated clone
      mat.dispose();
    }
  }
}

/**
 * Magnum Break: a blast of fire (대폭발.tga) bursting outward around the
 * caster, as a flat ring on the ground with a low wall of flame.
 */
export class FireBlast implements Effect {
  readonly object = new THREE.Group();
  private readonly start = performance.now();
  private readonly disc: THREE.Mesh;
  private readonly wall: THREE.Mesh;

  constructor(
    texture: THREE.Texture,
    private readonly anchor: Anchor,
    private readonly ms = 700,
  ) {
    const reach = CELL * 2.5; // Magnum Break hits 2 cells around
    const disc = new THREE.RingGeometry(reach * 0.25, reach, 40, 1);
    disc.rotateX(-Math.PI / 2);
    disc.translate(0, 0.3, 0);
    this.disc = new THREE.Mesh(disc, additive(texture, 0xffb070));
    const wallHeight = 40 * SPRITE_SCALE;
    const wall = new THREE.CylinderGeometry(reach, reach * 0.8, wallHeight, 40, 1, true);
    wall.translate(0, wallHeight / 2, 0);
    this.wall = new THREE.Mesh(wall, additive(texture, 0xff9050));
    for (const m of [this.disc, this.wall]) {
      m.renderOrder = 4;
      this.object.add(m);
    }
  }

  update(now: number): boolean {
    const t = (now - this.start) / this.ms;
    if (t >= 1) return false;
    this.object.position.copy(this.anchor());
    const burst = 0.15 + easeOut(t / 0.5) * 0.85;
    this.disc.scale.set(burst, 1, burst);
    this.wall.scale.set(burst, Math.max(0.05, 1 - t), burst);
    this.object.rotation.y = t * 1.5;
    const fade = t < 0.4 ? 1 : 1 - (t - 0.4) / 0.6;
    for (const m of [this.disc, this.wall]) (m.material as THREE.MeshBasicMaterial).opacity = fade;
    return true;
  }

  dispose(): void {
    for (const m of [this.disc, this.wall]) {
      m.geometry.dispose();
      (m.material as THREE.Material).dispose();
    }
  }
}
