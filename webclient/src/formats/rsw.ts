// .rsw: map scene description — water, global light, and placed objects.
import { Reader, ver } from "./reader";

export interface RswModel {
  name: string;
  file: string;
  position: [number, number, number];
  rotation: [number, number, number]; // degrees
  scale: [number, number, number];
}

/** An ambient sound placed on the map, replayed every `cycle` seconds in range. */
export interface RswSound {
  file: string;
  position: [number, number, number];
  volume: number;
  range: number;
  cycle: number;
}

/** An effect placed on the map, by the client's effect ID (47 = torch fire). */
export interface RswEffect {
  id: number;
  position: [number, number, number];
}

export interface Rsw {
  water: { level: number; type: number; waveHeight: number; waveSpeed: number; wavePitch: number; animSpeed: number };
  light: { longitude: number; latitude: number; diffuse: [number, number, number]; ambient: [number, number, number]; opacity: number };
  models: RswModel[];
  sounds: RswSound[];
  effects: RswEffect[];
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
  const sounds: RswSound[] = [];
  const effects: RswEffect[] = [];
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
      case 3: {
        r.str(80); // name
        const file = r.str(80);
        const position = r.vec3();
        const volume = r.f32();
        r.skip(8); // width, height
        const range = r.f32();
        const cycle = v >= ver(2, 0) ? r.f32() : 4;
        sounds.push({ file, position, volume, range, cycle });
        break;
      }
      case 4: {
        // Placed effect (torch fire, chimney smoke...): name, pos, id, delay, params.
        r.str(80); // name
        const position = r.vec3();
        const id = r.i32();
        r.skip(4 + 16); // delay, params
        effects.push({ id, position });
        break;
      }
      default:
        throw new Error(`unknown RSW object type ${type} at index ${i}`);
    }
  }
  return { water, light, models, sounds, effects };
}
