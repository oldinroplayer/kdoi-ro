// Fetches extracted client files from /data (public/data -> ro-client/data).
//
// RO references use backslashes and mixed case; extracted files are lowercase.

export function dataUrl(path: string): string {
  const clean = path.replace(/\\/g, "/").replace(/^\/+/, "").toLowerCase();
  return encodeURI(`/data/${clean}`);
}

export async function fetchData(path: string): Promise<ArrayBuffer> {
  const res = await fetch(dataUrl(path));
  if (!res.ok) throw new Error(`${path}: HTTP ${res.status}`);
  return res.arrayBuffer();
}
