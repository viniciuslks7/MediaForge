import { useCallback, useEffect, useRef, useState } from 'react';
import { Uploader, type ForgeRequest } from './components/Uploader';
import { Pipeline, type Stage } from './components/Pipeline';
import { EventLog } from './components/EventLog';
import { Artifacts } from './components/Artifacts';
import { Compare } from './components/Compare';
import { History } from './components/History';
import { SystemPulse } from './components/SystemPulse';
import { getJob, submitMedia, proxiedArtifactUrl } from './api';
import { RealtimeClient } from './ws';
import { readHistory, pushHistory, type HistoryEntry } from './history';
import type { Artifact, JobEvent, Status } from './types';

/** Pick the artifact best suited as the "after" image in the comparison. */
function afterImage(artifacts: Artifact[]): string | undefined {
  const pick =
    artifacts.find((a) => a.name === 'resized') ??
    artifacts.find((a) => a.content_type.startsWith('image/') && a.name !== 'thumbnail') ??
    artifacts.find((a) => a.content_type.startsWith('image/'));
  return proxiedArtifactUrl(pick?.url);
}

const RANK: Record<Stage, number> = { idle: 0, pending: 1, processing: 2, completed: 3, failed: 3 };
const advance = (a: Stage, b: Status): Stage => (RANK[b] >= RANK[a] ? b : a);

// Base URL of the Jaeger UI, used to deep-link a job to its distributed trace.
const JAEGER_URL = (import.meta.env.VITE_JAEGER_URL as string | undefined) ?? 'http://localhost:16686';

export default function App() {
  const [connected, setConnected] = useState(false);
  const [status, setStatus] = useState<Stage>('idle');
  const [events, setEvents] = useState<JobEvent[]>([]);
  const [artifacts, setArtifacts] = useState<Artifact[]>([]);
  const [jobId, setJobId] = useState<string | null>(null);
  const [traceId, setTraceId] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [originalUrl, setOriginalUrl] = useState<string | null>(null);
  const [history, setHistory] = useState<HistoryEntry[]>(() => readHistory());

  const rt = useRef<RealtimeClient | null>(null);
  const currentJob = useRef<string | null>(null);
  const originalUrlRef = useRef<string | null>(null);

  // Single long-lived WebSocket; feeds the live event stream + advances the pipeline.
  useEffect(() => {
    const client = new RealtimeClient();
    rt.current = client;
    const offState = client.onState(setConnected);
    const offEvent = client.onEvent((ev) => {
      if (ev.job_id !== currentJob.current) return;
      setEvents((prev) => [...prev, ev]);
      setStatus((s) => advance(s, ev.status));
    });
    client.connect();
    return () => {
      offState();
      offEvent();
      client.close();
    };
  }, []);

  // REST polling is the source of truth for final status + artifacts (events can
  // arrive faster than we subscribe for very short jobs).
  const poll = useCallback((id: string, attempt = 0) => {
    getJob(id)
      .then(({ job, artifacts: arts }) => {
        if (currentJob.current !== id) return;
        setStatus((s) => advance(s, job.status));
        if (arts.length) setArtifacts(arts);
        if (job.status === 'completed' || job.status === 'failed') {
          setArtifacts(arts);
          setBusy(false);
          return;
        }
        if (attempt < 90) setTimeout(() => poll(id, attempt + 1), 1000);
        else {
          setError('Job timed out after 90s — check the worker logs.');
          setBusy(false);
        }
      })
      .catch(() => {
        if (currentJob.current === id && attempt < 90) {
          setTimeout(() => poll(id, attempt + 1), 1200);
        }
      });
  }, []);

  const onForge = useCallback(
    async (req: ForgeRequest) => {
      setBusy(true);
      setError(null);
      setEvents([]);
      setArtifacts([]);
      setTraceId(null);
      setStatus('pending');

      // Keep a local preview of the original to drive the before/after slider.
      if (originalUrlRef.current) URL.revokeObjectURL(originalUrlRef.current);
      const preview =
        req.kind === 'image' && req.file.type.startsWith('image/')
          ? URL.createObjectURL(req.file)
          : null;
      originalUrlRef.current = preview;
      setOriginalUrl(preview);

      try {
        const { job_id, trace_id } = await submitMedia(req.file, {
          kind: req.kind,
          operations: req.operations,
          params: req.params,
        });
        currentJob.current = job_id;
        setJobId(job_id);
        setTraceId(trace_id ?? null);
        setHistory(pushHistory({ job_id, kind: req.kind, trace_id, ts: Date.now() }));
        rt.current?.subscribe(job_id);
        poll(job_id);
      } catch (e) {
        setError(e instanceof Error ? e.message : 'submission failed');
        setStatus('idle');
        setBusy(false);
      }
    },
    [poll],
  );

  // Reload a previously submitted job from history: fetch its current state and
  // artifacts. No original preview survives a reload, so the before/after slider
  // is skipped for historical jobs.
  const loadJob = useCallback((entry: HistoryEntry) => {
    currentJob.current = entry.job_id;
    setJobId(entry.job_id);
    setTraceId(entry.trace_id ?? null);
    if (originalUrlRef.current) URL.revokeObjectURL(originalUrlRef.current);
    originalUrlRef.current = null;
    setOriginalUrl(null);
    setError(null);
    setEvents([]);
    setArtifacts([]);
    setBusy(true);
    setStatus('pending');
    getJob(entry.job_id)
      .then(({ job, artifacts: arts }) => {
        setStatus((s) => advance(s, job.status));
        setArtifacts(arts);
        setBusy(false);
      })
      .catch(() => {
        setError('failed to load job from history');
        setBusy(false);
      });
  }, []);

  return (
    <div className="shell">
      <header className="masthead">
        <div className="brand">
          <h1>
            MEDIA<span className="forge">FORGE</span>
          </h1>
          <span className="tag">control room</span>
        </div>
        <div className="masthead-right">
          <SystemPulse />
          <div className={`conn${connected ? ' live' : ''}`} aria-live="polite">
            <span className="dot" aria-hidden />
            {connected ? 'realtime · live' : 'realtime · offline'}
          </div>
        </div>
      </header>

      <History entries={history} activeId={jobId} onSelect={loadJob} />

      <Uploader busy={busy} onForge={onForge} />

      <Pipeline status={status} />

      {(status === 'completed' || status === 'failed') && (
        <div className={`result-strip ${status}`} role="status" aria-live="polite">
          {status === 'completed'
            ? `✓ forged ${artifacts.length} artifact${artifacts.length === 1 ? '' : 's'}`
            : '✕ job failed — see the event stream'}
          {jobId && <span className="jid">job {jobId.slice(0, 8)}</span>}
          {traceId && (
            <a
              className="trace-link"
              href={`${JAEGER_URL}/trace/${traceId}`}
              target="_blank"
              rel="noreferrer"
              title={`Open trace ${traceId} in Jaeger`}
            >
              trace ↗
            </a>
          )}
        </div>
      )}

      {error && <div className="err">⚠ {error}</div>}

      {status === 'completed' &&
        originalUrl &&
        (() => {
          const after = afterImage(artifacts);
          return after ? <Compare before={originalUrl} after={after} /> : null;
        })()}

      <div className="grid2" style={{ marginTop: 30 }}>
        <EventLog events={events} />
        <Artifacts artifacts={artifacts} />
      </div>

      <footer className="foot">
        <span>MediaForge — Go · Python · TypeScript · RabbitMQ</span>
        <div className="links">
          <a href="http://localhost:3000" target="_blank" rel="noreferrer">
            Grafana ↗
          </a>
          <a href="http://localhost:15672" target="_blank" rel="noreferrer">
            RabbitMQ ↗
          </a>
          <a href="http://localhost:9001" target="_blank" rel="noreferrer">
            MinIO ↗
          </a>
          <a href="http://localhost:9090" target="_blank" rel="noreferrer">
            Prometheus ↗
          </a>
          <a href={JAEGER_URL} target="_blank" rel="noreferrer">
            Jaeger ↗
          </a>
        </div>
      </footer>
    </div>
  );
}
