// .rsm (v1.x): static 3D model made of a node hierarchy.
import { Reader, ver } from "./reader";

export interface RsmFace {
  vert: [number, number, number];
  tvert: [number, number, number];
  tex: number; // index into node.textures
  twoSide: boolean;
}

export interface RsmNode {
  name: string;
  parent: string;
  textures: number[]; // indices into Rsm.textures
  mat3: number[]; // 3x3, column-major as stored
  offset: [number, number, number];
  pos: [number, number, number];
  rotAngle: number; // radians
  rotAxis: [number, number, number];
  scale: [number, number, number];
  vertices: [number, number, number][];
  tverts: [number, number][];
  faces: RsmFace[];
  rotKey: [number, number, number, number] | null; // first rotation keyframe (quaternion)
}

export interface Rsm {
  alpha: number;
  textures: string[];
  mainNode: string;
  nodes: RsmNode[];
}

export function parseRsm(buf: ArrayBuffer): Rsm {
  const r = new Reader(buf);
  r.magic("GRSM");
  const v = r.version();
  if (v >= ver(2, 0)) throw new Error(`unsupported RSM version ${v.toString(16)}`);

  r.i32(); // anim length
  r.i32(); // shade type
  const alpha = v >= ver(1, 4) ? r.u8() / 255 : 1;
  r.skip(16);

  const textures: string[] = [];
  const texCount = r.i32();
  for (let i = 0; i < texCount; i++) textures.push(r.str(40));

  const mainNode = r.str(40);
  const nodeCount = r.i32();
  const nodes: RsmNode[] = [];
  for (let n = 0; n < nodeCount; n++) {
    const name = r.str(40);
    const parent = r.str(40);
    const nodeTex: number[] = [];
    const ntc = r.i32();
    for (let i = 0; i < ntc; i++) nodeTex.push(r.i32());
    const mat3: number[] = [];
    for (let i = 0; i < 9; i++) mat3.push(r.f32());
    const offset = r.vec3();
    const pos = r.vec3();
    const rotAngle = r.f32();
    const rotAxis = r.vec3();
    const scale = r.vec3();

    const vertices: RsmNode["vertices"] = [];
    const vc = r.i32();
    for (let i = 0; i < vc; i++) vertices.push(r.vec3());

    const tverts: RsmNode["tverts"] = [];
    const tc = r.i32();
    for (let i = 0; i < tc; i++) {
      if (v >= ver(1, 2)) r.u32(); // color
      tverts.push([r.f32(), r.f32()]);
    }

    const faces: RsmFace[] = [];
    const fc = r.i32();
    for (let i = 0; i < fc; i++) {
      const vert: RsmFace["vert"] = [r.u16(), r.u16(), r.u16()];
      const tvert: RsmFace["tvert"] = [r.u16(), r.u16(), r.u16()];
      const tex = r.u16();
      r.u16(); // padding
      const twoSide = r.i32() !== 0;
      if (v >= ver(1, 2)) r.i32(); // smoothing group
      faces.push({ vert, tvert, tex, twoSide });
    }

    if (v >= ver(1, 5)) {
      const pk = r.i32();
      r.skip(pk * 16);
    }
    let rotKey: RsmNode["rotKey"] = null;
    const rk = r.i32();
    for (let i = 0; i < rk; i++) {
      r.i32(); // frame
      const q: [number, number, number, number] = [r.f32(), r.f32(), r.f32(), r.f32()];
      if (i === 0) rotKey = q;
    }

    nodes.push({ name, parent, textures: nodeTex, mat3, offset, pos, rotAngle, rotAxis, scale, vertices, tverts, faces, rotKey });
  }
  return { alpha, textures, mainNode, nodes };
}
