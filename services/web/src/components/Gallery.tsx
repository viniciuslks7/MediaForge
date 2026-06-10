import { useEffect, useState } from 'react';
import { listJobs, proxiedArtifactUrl } from '../api';
import { age } from '../utils';
import type { GalleryJob } from '../types';

const PAGE = 12;

/**
 * Server-side gallery of every job the forge has ever run, paginated. Unlike
 * History (localStorage, this browser only), this reads GET /v1/media, so it
 * survives reloads and shows work submitted by anyone.
 */
export function Gallery({
  activeId,
  onSelect,
  refreshKey,
}: {
  activeId: string | null;
  onSelect: (jobId: string) => void;
  /** Bump to re-fetch the current page (e.g. when a queue item finishes). */
  refreshKey: number;
}) {
  const [jobs, setJobs] = useState<GalleryJob[]>([]);
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [error, setError] = useState(false);
  const [retry, setRetry] = useState(0);

  useEffect(() => {
    let alive = true;
    listJobs(PAGE, offset)
      .then((res) => {
        if (!alive) return;
        setJobs(res.jobs);
        setTotal(res.total);
        setError(false);
      })
      .catch(() => alive && setError(true));
    return () => {
      alive = false;
    };
  }, [offset, refreshKey, retry]);

  // Nothing to show and nothing wrong: an empty forge renders no gallery.
  if (!error && total === 0) return null;

  const page = Math.floor(offset / PAGE) + 1;
  const pages = Math.max(1, Math.ceil(total / PAGE));

  return (
    <section className="gallery" aria-label="Job gallery">
      <div className="gallery-head">
        <h2>
          forge gallery <span className="gallery-count">{total} jobs</span>
        </h2>
        {error ? (
          <button type="button" className="gallery-retry" onClick={() => setRetry((n) => n + 1)}>
            offline — retry
          </button>
        ) : (
          <div className="gallery-pager">
            <button
              type="button"
              disabled={offset === 0}
              onClick={() => setOffset(Math.max(0, offset - PAGE))}
              aria-label="Previous page"
            >
              ‹
            </button>
            <span>
              {page}/{pages}
            </span>
            <button
              type="button"
              disabled={offset + PAGE >= total}
              onClick={() => setOffset(offset + PAGE)}
              aria-label="Next page"
            >
              ›
            </button>
          </div>
        )}
      </div>

      <div className="gallery-grid">
        {jobs.map(({ job: j, thumb_url }) => {
          const thumb = proxiedArtifactUrl(thumb_url);
          return (
            <button
              key={j.job_id}
              type="button"
              className={`g-card${j.job_id === activeId ? ' on' : ''}`}
              onClick={() => onSelect(j.job_id)}
              title={`${j.kind} · ${j.status} · ${j.job_id}`}
            >
              {/* The placeholder always renders; the thumbnail covers it and
                  removes itself if the presigned URL has expired (403). */}
              <span className="g-ph" aria-hidden>
                {j.kind === 'ocr' ? '𝐓' : '◳'}
              </span>
              {thumb && (
                <img
                  src={thumb}
                  alt={`thumbnail of job ${j.job_id.slice(0, 8)}`}
                  loading="lazy"
                  onError={(e) => e.currentTarget.remove()}
                />
              )}
              <span className={`g-status ${j.status}`}>{j.status}</span>
              <span className="g-meta">
                <span className="g-kind">{j.kind}</span>
                <span>{age(j.created_at)}</span>
              </span>
            </button>
          );
        })}
      </div>
    </section>
  );
}
