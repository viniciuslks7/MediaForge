import type { JobParams, Kind, Operation } from '../types';

export type QueueState = 'queued' | 'active' | 'done' | 'error';

/** One unit of work in the forge queue. The File is carried until the job is
 *  submitted; the rest is what the strip and the main panel need to render. */
export interface QueueItem {
  id: string;
  file: File;
  fileName: string;
  kind: Kind;
  operations: Operation[];
  params?: JobParams;
  status: QueueState;
  jobId?: string;
  error?: string;
}

const GLYPH: Record<QueueState, string> = {
  queued: '○',
  active: '◐',
  done: '✓',
  error: '✕',
};

/** A strip of queued / processing / finished jobs. Finished cards reload their
 *  result into the main panel on click; the focused job is highlighted. */
export function ForgeQueue({
  items,
  activeId,
  onSelect,
  onClear,
}: {
  items: QueueItem[];
  activeId: string | null;
  onSelect: (item: QueueItem) => void;
  onClear: () => void;
}) {
  if (items.length === 0) return null;

  const finished = items.filter((i) => i.status === 'done' || i.status === 'error').length;
  const allDone = finished === items.length;

  return (
    <section className="queue" aria-label="Forge queue">
      <div className="queue-head">
        <span className="queue-title">
          forge queue{' '}
          <span className="queue-count">
            {finished}/{items.length}
          </span>
        </span>
        {allDone && (
          <button type="button" className="queue-clear" onClick={onClear}>
            clear
          </button>
        )}
      </div>
      <div className="queue-strip">
        {items.map((it) => {
          const clickable = Boolean(it.jobId) && (it.status === 'done' || it.status === 'error');
          return (
            <button
              key={it.id}
              type="button"
              className={`q-card ${it.status}${it.jobId && it.jobId === activeId ? ' on' : ''}`}
              onClick={() => clickable && onSelect(it)}
              disabled={!clickable}
              title={it.error ? `${it.fileName} — ${it.error}` : it.fileName}
            >
              <span className={`q-glyph ${it.status}`} aria-hidden>
                {GLYPH[it.status]}
              </span>
              <span className="q-name">{it.fileName}</span>
              <span className="q-kind">{it.kind}</span>
            </button>
          );
        })}
      </div>
    </section>
  );
}
