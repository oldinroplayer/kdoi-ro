// Skill database: rules from the server's /skills.json, plus names,
// descriptions and icons from the client's skillnametable/skilldesctable and
// texture/유저인터페이스/item/<name>.bmp, keyed by RO's internal skill name.
import { fetchData } from "./assets";
import { keyedImage } from "./items";

export interface SkillInfo {
  id: number;
  name: string; // internal name, e.g. NV_BASIC
  displayName: string;
  description: string; // may contain RO colour codes
  job: number;
  maxLevel: number;
  target: "passive" | "self" | "enemy" | "friend" | "ground";
  /** 0 neutral, 1 water, 3 fire, 4 wind, 5 poison, 6 holy. */
  element: number;
  sp: number[];
  castTime: number[];
  requires: { id: number; level: number }[];
  quest: boolean;
}

const cp949 = new TextDecoder("euc-kr");

export class SkillDb {
  private constructor(private readonly skills: Map<number, SkillInfo>) {}

  static async load(wsUrl: string): Promise<SkillDb> {
    const http = new URL(wsUrl);
    http.protocol = http.protocol === "wss:" ? "https:" : "http:";
    http.pathname = "/skills.json";
    http.search = "";

    const [defs, names, descs] = await Promise.all([
      fetch(http).then((r) => {
        if (!r.ok) throw new Error(`skills.json: HTTP ${r.status}`);
        return r.json() as Promise<Partial<SkillInfo>[]>;
      }),
      fetchData("skillnametable.txt").then(parseNames),
      fetchData("skilldesctable.txt").then(parseDescs),
    ]);
    const skills = new Map<number, SkillInfo>();
    for (const d of defs) {
      const name = d.name!;
      skills.set(d.id!, {
        id: d.id!,
        name,
        displayName: (names.get(name) ?? name).replace(/_/g, " "),
        description: descs.get(name) ?? "",
        job: d.job ?? 0,
        maxLevel: d.maxLevel ?? 1,
        target: d.target ?? "passive",
        element: d.element ?? 0,
        sp: d.sp ?? [],
        castTime: d.castTime ?? [],
        requires: d.requires ?? [],
        quest: d.quest ?? false,
      });
    }
    return new SkillDb(skills);
  }

  get(id: number): SkillInfo | undefined {
    return this.skills.get(id);
  }
}

/** Skill IDs the client draws effects for. */
export const Skill = {
  Bash: 5,
  MagnumBreak: 7,
  ColdBolt: 14,
  FireBolt: 19,
  LightningBolt: 20,
  Heal: 28,
  IncAgi: 29,
  Blessing: 34,
  Mammonite: 42,
  DoubleStrafe: 46,
  Envenom: 52,
  FirstAid: 142,
} as const;

/** SP cost at a level. */
export function skillSp(s: SkillInfo, level: number): number {
  if (level < 1 || s.sp.length === 0) return 0;
  return s.sp[Math.min(level, s.sp.length) - 1];
}

export function skillIcon(s: SkillInfo): Promise<string> {
  return keyedImage(`texture/유저인터페이스/item/${s.name.toLowerCase()}.bmp`);
}

/** "NV_BASIC#Basic_Skill#" lines. */
function parseNames(buf: ArrayBuffer): Map<string, string> {
  const out = new Map<string, string>();
  for (const line of cp949.decode(buf).split(/\r?\n/)) {
    const m = /^([A-Z0-9_]+)#([^#]*)#/.exec(line.trim());
    if (m) out.set(m[1], m[2]);
  }
  return out;
}

/** Blocks of "NV_BASIC#", description lines, "#". */
function parseDescs(buf: ArrayBuffer): Map<string, string> {
  const out = new Map<string, string>();
  const re = /^([A-Z0-9_]+)#\r?\n([\s\S]*?)^#/gm;
  const text = cp949.decode(buf);
  for (let m = re.exec(text); m; m = re.exec(text)) out.set(m[1], m[2].trim());
  return out;
}
