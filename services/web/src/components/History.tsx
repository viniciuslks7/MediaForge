import type { HistoryEntry } from '../history';

/** A compact strip of recent jobs; clicking one reloads its result + trace. */
export function History({
  entries,
  activeId,
  onSelect,
}: {
  entries: HistoryEntry[];
  activeId: string | null;
  onSelect: (entry: HistoryEntry) => void;
}) {
  if (entries.length === 0) return null;
  return (
    <div className="history" aria-label="Recent jobs">
      <span className="history-label">recent</span>
      <div className="history-list">
        {entries.map((e) => (
          <button
            key={e.job_id}
            type="button"
            className={`history-chip${e.job_id === activeId ? ' on' : ''}`}
            onClick={() => onSelect(e)}
            title={new Date(e.ts).toLocaleString()}
          >
            <span className="hk">{e.kind}</span>
            {e.job_id.slice(0, 8)}
          </button>
        ))}
      </div>
    </div>
  );
}
