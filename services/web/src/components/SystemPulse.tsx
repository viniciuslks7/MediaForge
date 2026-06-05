import { useEffect, useState } from 'react';

interface Stats {
  connected_clients: number;
  tracked_jobs: number;
}

/**
 * SystemPulse polls the realtime-gateway's /stats endpoint for live, per-replica
 * counts (connected WebSocket clients and tracked jobs). Renders nothing until
 * the first successful poll, so it stays quiet when the backend is unreachable.
 */
export function SystemPulse() {
  const [stats, setStats] = useState<Stats | null>(null);

  useEffect(() => {
    let alive = true;
    const tick = () => {
      fetch('/stats')
        .then((r) => (r.ok ? (r.json() as Promise<Stats>) : null))
        .then((s) => {
          if (alive) setStats(s);
        })
        .catch(() => {
          if (alive) setStats(null);
        });
    };
    tick();
    const id = setInterval(tick, 3000);
    return () => {
      alive = false;
      clearInterval(id);
    };
  }, []);

  if (!stats) return null;
  return (
    <div className="pulse" aria-live="polite" title="Live counts from the realtime gateway">
      <span className="pulse-dot" aria-hidden />
      <span>
        <b>{stats.connected_clients}</b> client{stats.connected_clients === 1 ? '' : 's'}
      </span>
      <span className="pulse-sep">·</span>
      <span>
        <b>{stats.tracked_jobs}</b> tracked
      </span>
    </div>
  );
}
