import type { JobView, Operation, SubmitResponse } from './types';

// Same-origin: nginx (prod) / vite proxy (dev) forward these to the gateway.
const BASE = '';

// Write endpoints are guarded by API_AUTH_TOKEN. This is the local dev token
// (matches .env.example); a real deployment would mint per-user tokens.
const TOKEN = (import.meta.env.VITE_API_TOKEN as string | undefined) ?? 'dev-local-token';

/** Turn a presigned MinIO URL (Host minio:9000) into a same-origin /s3 path. */
export function proxiedArtifactUrl(url?: string): string | undefined {
  if (!url) return undefined;
  return url.replace(/^https?:\/\/[^/]+\//, '/s3/');
}

export async function submitMedia(
  file: File,
  opts: { kind?: 'image' | 'ocr'; operations?: Operation[] },
): Promise<SubmitResponse> {
  const form = new FormData();
  form.append('file', file);
  if (opts.kind) form.append('kind', opts.kind);
  if (opts.operations?.length) form.append('operations', opts.operations.join(','));

  const res = await fetch(`${BASE}/v1/media`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${TOKEN}` },
    body: form,
  });
  if (!res.ok) {
    const detail = await res.json().catch(() => ({}));
    throw new Error((detail as { error?: string }).error ?? `submit failed (${res.status})`);
  }
  return res.json() as Promise<SubmitResponse>;
}

export async function getJob(id: string): Promise<JobView> {
  const res = await fetch(`${BASE}/v1/media/${id}`);
  if (!res.ok) throw new Error(`job lookup failed (${res.status})`);
  return res.json() as Promise<JobView>;
}
