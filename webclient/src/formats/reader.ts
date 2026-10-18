// Little-endian cursor over an ArrayBuffer, with CP949 (Korean) fixed-size strings.

const cp949 = new TextDecoder("euc-kr"); // the Encoding Standard maps this label to windows-949

export class Reader {
  readonly view: DataView;
  offset = 0;

  constructor(buffer: ArrayBuffer, offset = 0) {
    this.view = new DataView(buffer);
    this.offset = offset;
  }

  get remaining(): number {
    return this.view.byteLength - this.offset;
  }

  u8(): number {
    return this.view.getUint8(this.offset++);
  }
  u16(): number {
    const v = this.view.getUint16(this.offset, true);
    this.offset += 2;
    return v;
  }
  i16(): number {
    const v = this.view.getInt16(this.offset, true);
    this.offset += 2;
    return v;
  }
  u32(): number {
    const v = this.view.getUint32(this.offset, true);
    this.offset += 4;
    return v;
  }
  i32(): number {
    const v = this.view.getInt32(this.offset, true);
    this.offset += 4;
    return v;
  }
  f32(): number {
    const v = this.view.getFloat32(this.offset, true);
    this.offset += 4;
    return v;
  }
  vec3(): [number, number, number] {
    return [this.f32(), this.f32(), this.f32()];
  }
  bytes(n: number): Uint8Array {
    const b = new Uint8Array(this.view.buffer, this.view.byteOffset + this.offset, n);
    this.offset += n;
    return b;
  }
  skip(n: number): void {
    this.offset += n;
  }

  /** Fixed-size, zero-padded CP949 string. */
  str(n: number): string {
    const b = this.bytes(n);
    const end = b.indexOf(0);
    return cp949.decode(end < 0 ? b : b.subarray(0, end));
  }

  magic(expected: string): void {
    const got = String.fromCharCode(...this.bytes(expected.length));
    if (got !== expected) throw new Error(`bad magic: expected ${expected}, got ${got}`);
  }

  /** RO files store version as major then minor byte; returns e.g. 2.1 -> 0x0201. */
  version(): number {
    const major = this.u8();
    const minor = this.u8();
    return (major << 8) | minor;
  }
}

export const ver = (major: number, minor: number) => (major << 8) | minor;
