import type { Kind } from './types';

/** A recently submitted job, persisted client-side so it survives reloads. */
export interface HistoryEntry {
  job_id: string;
  kind: Kind;
  trace_id?: string;
  ts: number;
}

const KEY = 'mediaforge.history';
const MAX = 8;

export function readHistory(): HistoryEntry[] {
  try {
    const raw = localStorage.getItem(KEY);
    return raw ? (JSON.parse(raw) as HistoryEntry[]) : [];
  } catch {
    return [];
  }
}

/** Prepend an entry (de-duplicated by job_id), cap the list, and persist it. */
export function pushHistory(entry: HistoryEntry): HistoryEntry[] {
  const next = [entry, ...readHistory().filter((e) => e.job_id !== entry.job_id)].slice(0, MAX);
  try {
    localStorage.setItem(KEY, JSON.stringify(next));
  } catch {
    /* storage unavailable (private mode / quota) — history is best-effort */
  }
  return next;
}
