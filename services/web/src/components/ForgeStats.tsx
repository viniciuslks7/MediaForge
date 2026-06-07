import type { Artifact } from '../types';
import { humanSize } from '../utils';

const num = (v: unknown): number | undefined => (typeof v === 'number' ? v : undefined);

function pct(delta: number): string {
  const p = Math.round(Math.abs(delta) * 100);
  if (p === 0) return '±0%';
  return delta < 0 ? `−${p}%` : `+${p}%`;
}

/** "Forge economy": how each produced variant compares in size to the original
 *  upload — the compression win MediaForge delivers, drawn as proportional bars. */
export function ForgeStats({
  artifacts,
  originalBytes,
}: {
  artifacts: Artifact[];
  originalBytes: number | null;
}) {
  const sized = artifacts.filter((a) => (num(a.size_bytes) ?? 0) > 0);
  if (!originalBytes || sized.length === 0) return null;

  // Bars are scaled against the largest of {original, any artifact}.
  const max = Math.max(originalBytes, ...sized.map((a) => a.size_bytes ?? 0));

  const rows = [
    { name: 'original', bytes: originalBytes, delta: 0, base: true },
    ...sized.map((a) => ({
      name: a.name,
      bytes: a.size_bytes ?? 0,
      delta: ((a.size_bytes ?? 0) - originalBytes) / originalBytes,
      base: false,
    })),
  ];

  // Headline = the biggest saving across the produced variants.
  const best = sized.reduce(
    (acc, a) => {
      const d = ((a.size_bytes ?? 0) - originalBytes) / originalBytes;
      return d < acc.delta ? { name: a.name, delta: d } : acc;
    },
    { name: '', delta: 0 },
  );

  return (
    <section className="stats" aria-label="Forge economy">
      <div className="stats-head">
        <span className="stats-title">forge economy</span>
        {best.delta < 0 && (
          <span className="stats-headline">
            best win: <b>{best.name}</b> {pct(best.delta)} smaller
          </span>
        )}
      </div>
      <div className="stats-rows">
        {rows.map((r) => (
          <div key={r.name} className={`stat-row${r.base ? ' is-original' : ''}`}>
            <span className="stat-name">{r.name}</span>
            <span className="stat-bar">
              <span
                className={`stat-fill ${r.base ? 'base' : r.delta <= 0 ? 'good' : 'up'}`}
                style={{ width: `${Math.max(2, (r.bytes / max) * 100)}%` }}
              />
            </span>
            <span className="stat-size">{humanSize(r.bytes)}</span>
            <span className={`stat-delta ${r.base ? '' : r.delta <= 0 ? 'good' : 'up'}`}>
              {r.base ? 'source' : pct(r.delta)}
            </span>
          </div>
        ))}
      </div>
    </section>
  );
}
