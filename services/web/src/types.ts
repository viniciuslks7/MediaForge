// Mirrors the canonical JSON Schemas in specs/schemas — the single source of
// truth shared by the Go, Python and TypeScript services.

export type Kind = 'image' | 'ocr';
export type Status = 'pending' | 'processing' | 'completed' | 'failed';
export type Operation = 'resize' | 'thumbnail' | 'webp';
export type ResizeFormat = 'jpeg' | 'png' | 'webp';

// Optional per-job overrides for the image pipeline (the "params" object in
// specs/schemas/job.schema.json). Absent fields fall back to worker defaults.
export interface JobParams {
  resize_max_dim?: number;
  thumbnail_size?: number;
  resize_format?: ResizeFormat;
  quality?: number;
}

export interface Job {
  job_id: string;
  kind: Kind;
  status: Status;
  source_key: string;
  source_mime?: string;
  size_bytes?: number;
  operations?: Operation[];
  params?: JobParams;
  attempt?: number;
  created_at?: string;
  updated_at?: string;
}

export interface Artifact {
  job_id?: string;
  name: string;
  object_key: string;
  content_type: string;
  size_bytes?: number;
  metadata?: Record<string, unknown>;
  created_at?: string;
  url?: string;
}

export interface JobView {
  job: Job;
  artifacts: Artifact[];
}

export interface SubmitResponse {
  job_id: string;
  kind: Kind;
  status: 'pending';
}

// Lifecycle event fanned out by realtime-gateway over the WebSocket.
export interface JobEvent {
  job_id: string;
  kind: Kind;
  status: Status;
  message?: string;
  progress: number;
  artifact?: string;
  ts: number;
}
