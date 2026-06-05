import { useEffect, useRef } from 'react';
import type { JobEvent } from '../types';

function clock(ts: number): string {
  const d = new Date(ts);
  return d.toLocaleTimeString('en-GB', { hour12: false }) + '.' + String(d.getMilliseconds()).padStart(3, '0');
}

export function EventLog({ events }: { events: JobEvent[] }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    ref.current?.scrollTo({ top: ref.current.scrollHeight, behavior: 'smooth' });
  }, [events]);

  return (
    <div className="panel">
      <div className="panel-head">
        <span>event stream · /ws</span>
        <span className="dots">
          <i /><i /><i />
        </span>
      </div>
      <div className="term" ref={ref} aria-live="polite" aria-label="Job event stream">
        {events.length === 0 ? (
          <div className="empty">// awaiting job — events fan out here in real time</div>
        ) : (
          events.map((e, i) => (
            <div className="row" key={i} style={{ animationDelay: `${Math.min(i * 0.02, 0.2)}s` }}>
              <span className="t">{clock(e.ts)}</span>
              <span className={`st ${e.status}`}>{e.status.toUpperCase()}</span>
              <span className="msg">
                {e.message ?? e.artifact ?? `progress ${e.progress}%`}
              </span>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
