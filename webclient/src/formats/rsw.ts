// .rsw: map scene description — water, global light, and placed objects.
import { Reader, ver } from "./reader";

export interface RswModel {
  name: string;
  file: string;
  position: [number, number, number];
  rotation: [number, number, number]; // degrees
  scale: [number, number, number];
}

export interface Rsw {
  water: { level: number; type: number; waveHeight: number; waveSpeed: number; wavePitch: number; animSpeed: number };
  light: { longitude: number; latitude: number; diffuse: [number, number, number]; ambient: [number, number, number]; opacity: number };
  models: RswModel[];
}

export function parseRsw(buf: ArrayBuffer): Rsw {
  const r = new Reader(buf);
  r.magic("GRSW");
  const v = r.version();
  if (v < ver(1, 9) || v > ver(2, 1)) throw new Error(`unsupported RSW version ${v.toString(16)}`);

  r.str(40); // ini
  r.str(40); // gnd
  r.str(40); // gat
  r.str(40); // src

  const water = {
    level: r.f32(),
    type: r.i32(),
    waveHeight: r.f32(),
    waveSpeed: r.f32(),
    wavePitch: r.f32(),
    animSpeed: r.i32(),
  };
  const light = {
    longitude: r.i32(),
    latitude: r.i32(),
    diffuse: r.vec3(),
    ambient: r.vec3(),
    opacity: r.f32(),
  };
  r.skip(16); // ground bounds

  const models: RswModel[] = [];
  const count = r.i32();
  for (let i = 0; i < count; i++) {
    const type = r.i32();
    switch (type) {
      case 1: {
        const name = r.str(40);
        r.i32(); // anim type
        r.f32(); // anim speed
        r.i32(); // block type
        const file = r.str(80);
        r.str(80); // node name
        models.push({ name, file, position: r.vec3(), rotation: r.vec3(), scale: r.vec3() });
        break;
      }
      case 2: // light: name, pos, color, range
        r.skip(80 + 12 + 12 + 4);
        break;
      case 3: // sound: name, file, pos, vol, w, h, range, cycle (2.0+)
        r.skip(80 + 80 + 12 + 4 + 4 + 4 + 4 + (v >= ver(2, 0) ? 4 : 0));
        break;
      case 4: // effect: name, pos, id, delay, params
        r.skip(80 + 12 + 4 + 4 + 16);
        break;
      default:
        throw new Error(`unknown RSW object type ${type} at index ${i}`);
    }
  }
  return { water, light, models };
}
