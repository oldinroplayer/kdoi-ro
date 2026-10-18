// WebSocket connection speaking the server's binary protocol (see server/README.md).

export const LoginCode = {
  OK: 0,
  Created: 1,
  WrongPassword: 2,
  InvalidName: 3,
  InvalidPassword: 4,
  AlreadyOnline: 5,
  ServerError: 6,
} as const;

export const loginMessages: Record<number, string> = {
  [LoginCode.WrongPassword]: "Wrong password for that character.",
  [LoginCode.InvalidName]: "Names are 4–23 letters, digits or underscores.",
  [LoginCode.InvalidPassword]: "Passwords are 4–72 characters.",
  [LoginCode.AlreadyOnline]: "That character is already online.",
  [LoginCode.ServerError]: "Server error, try again.",
};

export type ServerPacket =
  | { type: "loginResult"; code: number }
  | { type: "enterMap"; selfId: number; map: string; x: number; y: number; dir: number; job: number; speed: number }
  | { type: "appear"; id: number; name: string; job: number; x: number; y: number; dir: number; speed: number }
  | { type: "disappear"; id: number }
  | { type: "move"; id: number; fromX: number; fromY: number; steps: [number, number][] }
  | { type: "stop"; id: number; x: number; y: number };

const textDecoder = new TextDecoder();
const textEncoder = new TextEncoder();

function decode(buf: ArrayBuffer): ServerPacket | null {
  const v = new DataView(buf);
  let o = 0;
  const u8 = () => v.getUint8(o++);
  const u16 = () => ((o += 2), v.getUint16(o - 2, true));
  const u32 = () => ((o += 4), v.getUint32(o - 4, true));
  const str = () => {
    const n = u8();
    o += n;
    return textDecoder.decode(new Uint8Array(buf, o - n, n));
  };

  switch (u16()) {
    case 0x8001:
      return { type: "loginResult", code: u8() };
    case 0x8002:
      return { type: "enterMap", selfId: u32(), map: str(), x: u16(), y: u16(), dir: u8(), job: u16(), speed: u16() };
    case 0x8003:
      return { type: "appear", id: u32(), name: str(), job: u16(), x: u16(), y: u16(), dir: u8(), speed: u16() };
    case 0x8004:
      return { type: "disappear", id: u32() };
    case 0x8005: {
      const id = u32(), fromX = u16(), fromY = u16(), n = u16();
      const steps: [number, number][] = [];
      for (let i = 0; i < n; i++) steps.push([u16(), u16()]);
      return { type: "move", id, fromX, fromY, steps };
    }
    case 0x8006:
      return { type: "stop", id: u32(), x: u16(), y: u16() };
    default:
      return null;
  }
}

export class Connection {
  private ws: WebSocket | null = null;
  onPacket: (p: ServerPacket) => void = () => {};
  onClose: (reason: string) => void = () => {};

  connect(url: string): Promise<void> {
    return new Promise((resolve, reject) => {
      const ws = new WebSocket(url);
      ws.binaryType = "arraybuffer";
      let opened = false;
      ws.onopen = () => {
        opened = true;
        resolve();
      };
      ws.onerror = () => {
        if (!opened) reject(new Error(`Can't reach the server at ${url}.`));
      };
      ws.onclose = (e) => {
        this.ws = null;
        if (opened) this.onClose(e.reason || "Disconnected from server.");
      };
      ws.onmessage = (e) => {
        if (!(e.data instanceof ArrayBuffer)) return;
        try {
          const p = decode(e.data);
          if (p) this.onPacket(p);
        } catch (err) {
          console.error("bad packet", err);
        }
      };
      this.ws = ws;
    });
  }

  get connected(): boolean {
    return this.ws?.readyState === WebSocket.OPEN;
  }

  close(): void {
    this.ws?.close();
  }

  login(name: string, password: string): void {
    const n = textEncoder.encode(name).slice(0, 255);
    const p = textEncoder.encode(password).slice(0, 255);
    const b = new Uint8Array(2 + 1 + n.length + 1 + p.length);
    const v = new DataView(b.buffer);
    v.setUint16(0, 0x0001, true);
    b[2] = n.length;
    b.set(n, 3);
    b[3 + n.length] = p.length;
    b.set(p, 4 + n.length);
    this.ws?.send(b);
  }

  move(x: number, y: number): void {
    const b = new DataView(new ArrayBuffer(6));
    b.setUint16(0, 0x0002, true);
    b.setUint16(2, x, true);
    b.setUint16(4, y, true);
    this.ws?.send(b.buffer);
  }
}
