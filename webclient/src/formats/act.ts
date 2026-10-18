// .act: animations for a sprite. Actions are grouped by 8 directions
// (action = state * 8 + direction, direction 0 = facing the camera, then clockwise).
import { Reader, ver } from "./reader";

export interface ActLayer {
  x: number;
  y: number;
  sprite: number; // -1 = none
  mirror: boolean;
  color: [number, number, number, number]; // RGBA 0-255
  scaleX: number;
  scaleY: number;
  rotation: number; // degrees
  type: number; // 0 indexed, 1 rgba
}

export interface ActFrame {
  layers: ActLayer[];
  anchors: { x: number; y: number }[];
}

export interface ActAction {
  frames: ActFrame[];
  delay: number; // ms per frame
}

export interface Act {
  actions: ActAction[];
}

export function parseAct(buf: ArrayBuffer): Act {
  const r = new Reader(buf);
  r.magic("AC");
  const raw = r.version();
  const v = ((raw & 0xff) << 8) | (raw >> 8); // minor byte first, like SPR
  const actionCount = r.u16();
  r.skip(10);

  const actions: ActAction[] = [];
  for (let a = 0; a < actionCount; a++) {
    const frameCount = r.u32();
    const frames: ActFrame[] = [];
    for (let f = 0; f < frameCount; f++) {
      r.skip(32); // unused bounding ranges
      const layerCount = r.u32();
      const layers: ActLayer[] = [];
      for (let l = 0; l < layerCount; l++) {
        const x = r.i32();
        const y = r.i32();
        const sprite = r.i32();
        const mirror = r.i32() !== 0;
        let color: ActLayer["color"] = [255, 255, 255, 255];
        let scaleX = 1, scaleY = 1, rotation = 0, type = 0;
        if (v >= ver(2, 0)) {
          color = [r.u8(), r.u8(), r.u8(), r.u8()];
          scaleX = r.f32();
          scaleY = v >= ver(2, 4) ? r.f32() : scaleX;
          rotation = r.i32();
          type = r.i32();
          if (v >= ver(2, 5)) r.skip(8); // width, height
        }
        layers.push({ x, y, sprite, mirror, color, scaleX, scaleY, rotation, type });
      }
      if (v >= ver(2, 0)) r.i32(); // sound event id
      const anchors: ActFrame["anchors"] = [];
      if (v >= ver(2, 3)) {
        const n = r.i32();
        for (let i = 0; i < n; i++) {
          r.skip(4);
          anchors.push({ x: r.i32(), y: r.i32() });
          r.skip(4);
        }
      }
      frames.push({ layers, anchors });
    }
    actions.push({ frames, delay: 150 });
  }

  if (v >= ver(2, 1)) {
    const sounds = r.i32();
    r.skip(sounds * 40);
  }
  if (v >= ver(2, 2)) {
    // Frame interval per action, in units of 25 ms.
    for (const a of actions) a.delay = r.f32() * 25;
  }
  return { actions };
}
