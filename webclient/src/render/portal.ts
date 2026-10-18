// Warp portal marker: a pulsing blue ring with a soft light column.
import * as THREE from "three";

export class Portal {
  readonly object = new THREE.Group();
  private readonly ring: THREE.Mesh;
  private readonly column: THREE.Mesh;

  constructor(radius: number) {
    const ringMat = new THREE.MeshBasicMaterial({
      color: 0x5fb8ff,
      transparent: true,
      opacity: 0.8,
      blending: THREE.AdditiveBlending,
      depthWrite: false,
      side: THREE.DoubleSide,
    });
    const ringGeo = new THREE.RingGeometry(radius * 0.6, radius, 48);
    ringGeo.rotateX(-Math.PI / 2);
    this.ring = new THREE.Mesh(ringGeo, ringMat);

    const colGeo = new THREE.CylinderGeometry(radius * 0.8, radius, radius * 3, 32, 1, true);
    colGeo.translate(0, radius * 1.5, 0);
    // Fade the column out towards the top using vertex colours.
    const pos = colGeo.getAttribute("position");
    const colors = new Float32Array(pos.count * 3);
    for (let i = 0; i < pos.count; i++) {
      const k = 1 - pos.getY(i) / (radius * 3);
      colors.set([0.35 * k, 0.7 * k, 1.0 * k], i * 3);
    }
    colGeo.setAttribute("color", new THREE.BufferAttribute(colors, 3));
    this.column = new THREE.Mesh(
      colGeo,
      new THREE.MeshBasicMaterial({
        vertexColors: true,
        transparent: true,
        opacity: 0.45,
        blending: THREE.AdditiveBlending,
        depthWrite: false,
        side: THREE.DoubleSide,
      }),
    );

    this.object.add(this.ring, this.column);
    this.object.renderOrder = 1;
  }

  update(timeMs: number): void {
    const t = timeMs / 1000;
    const pulse = 0.5 + 0.5 * Math.sin(t * 3);
    this.ring.scale.setScalar(0.9 + 0.15 * pulse);
    (this.ring.material as THREE.MeshBasicMaterial).opacity = 0.55 + 0.35 * pulse;
    this.column.rotation.y = t * 0.8;
  }

  dispose(): void {
    for (const m of [this.ring, this.column]) {
      m.geometry.dispose();
      (m.material as THREE.Material).dispose();
    }
  }
}
