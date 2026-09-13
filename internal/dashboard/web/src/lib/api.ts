export async function getJSON(path: string) {
  const res = await fetch(path);
  if (!res.ok) throw new Error(`${path}: ${res.status}`);
  return res.json();
}

export function buildUrl(path: string, params: Record<string, string>) {
  const query = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v) query.set(k, v);
  }
  const suffix = query.toString();
  return suffix ? `${path}?${suffix}` : path;
}
