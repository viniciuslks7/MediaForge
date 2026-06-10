import { useCallback, useEffect, useRef, useState } from 'react';
import { Uploader, type ForgeRequest } from './components/Uploader';
import { Pipeline, type Stage } from './components/Pipeline';
import { EventLog } from './components/EventLog';
import { Artifacts } from './components/Artifacts';
import { Compare } from './components/Compare';
import { History } from './components/History';
import { SystemPulse } from './components/SystemPulse';
import { ForgeQueue, type QueueItem } from './components/ForgeQueue';
import { ForgeStats } from './components/ForgeStats';
import { Gallery } from './components/Gallery';
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

// Monotonic local id for queue items (distinct from the server's job_id).
let seq = 0;
const nextId = () => `q${Date.now().toString(36)}-${seq++}`;

export default function App() {
  const [connected, setConnected] = useState(false);
  const [status, setStatus] = useState<Stage>('idle');
  const [events, setEvents] = useState<JobEvent[]>([]);
  const [artifacts, setArtifacts] = useState<Artifact[]>([]);
  const [jobId, setJobId] = useState<string | null>(null);
  const [traceId, setTraceId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [originalUrl, setOriginalUrl] = useState<string | null>(null);
  const [originalBytes, setOriginalBytes] = useState<number | null>(null);
  const [history, setHistory] = useState<HistoryEntry[]>(() => readHistory());
  const [queue, setQueue] = useState<QueueItem[]>([]);
  // Bumped whenever a job reaches a terminal state, so the gallery re-fetches.
  const [galleryTick, setGalleryTick] = useState(0);

  const rt = useRef<RealtimeClient | null>(null);
  const currentJob = useRef<string | null>(null);
  const originalUrlRef = useRef<string | null>(null);
  const queueRef = useRef<QueueItem[]>([]);
  const running = useRef(false);

  // Single long-lived WebSocket; feeds the live event stream and advances the
  // pipeline for whichever job is currently in focus.
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

  // Mutate the queue through one helper so the ref (read by the async runner)
  // and the rendered state never drift apart.
  const setQ = useCallback((updater: (q: QueueItem[]) => QueueItem[]) => {
    queueRef.current = updater(queueRef.current);
    setQueue(queueRef.current);
  }, []);

  const patchItem = useCallback(
    (id: string, patch: Partial<QueueItem>) => {
      setQ((q) => q.map((it) => (it.id === id ? { ...it, ...patch } : it)));
    },
    [setQ],
  );

  // Poll REST until the job reaches a terminal state, resolving with that
  // status. It keeps polling regardless of focus (so a queue item still
  // finishes if the user clicks away) but only drives the visible panel while
  // the job is the focused one.
  const pollUntilDone = useCallback((id: string): Promise<Status> => {
    return new Promise((resolve) => {
      const tick = (attempt = 0) => {
        getJob(id)
          .then(({ job, artifacts: arts }) => {
            if (currentJob.current === id) {
              setStatus((s) => advance(s, job.status));
              if (arts.length) setArtifacts(arts);
            }
            if (job.status === 'completed' || job.status === 'failed') {
              if (currentJob.current === id) setArtifacts(arts);
              return resolve(job.status);
            }
            if (attempt < 90) setTimeout(() => tick(attempt + 1), 1000);
            else resolve('failed');
          })
          .catch(() => {
            if (attempt < 90) setTimeout(() => tick(attempt + 1), 1200);
            else resolve('failed');
          });
      };
      tick();
    });
  }, []);

  // Process one queue item end-to-end. It becomes the focused job: the pipeline,
  // event log, artifacts and before/after slider all reflect it.
  const processItem = useCallback(
    async (item: QueueItem) => {
      patchItem(item.id, { status: 'active' });
      setError(null);
      setEvents([]);
      setArtifacts([]);
      setTraceId(null);
      setStatus('pending');

      if (originalUrlRef.current) URL.revokeObjectURL(originalUrlRef.current);
      const preview =
        item.kind === 'image' && item.file.type.startsWith('image/')
          ? URL.createObjectURL(item.file)
          : null;
      originalUrlRef.current = preview;
      setOriginalUrl(preview);
      // Original size drives the "forge economy" bars; only meaningful for the
      // image pipeline, where the artifacts are re-encodings of this upload.
      setOriginalBytes(item.kind === 'image' ? item.file.size : null);

      try {
        const { job_id, trace_id } = await submitMedia(item.file, {
          kind: item.kind,
          operations: item.operations,
          params: item.params,
        });
        currentJob.current = job_id;
        setJobId(job_id);
        setTraceId(trace_id ?? null);
        patchItem(item.id, { jobId: job_id });
        setHistory(pushHistory({ job_id, kind: item.kind, trace_id, ts: Date.now() }));
        rt.current?.subscribe(job_id);
        const final = await pollUntilDone(job_id);
        patchItem(item.id, { status: final === 'completed' ? 'done' : 'error' });
        setGalleryTick((n) => n + 1);
      } catch (e) {
        const msg = e instanceof Error ? e.message : 'submission failed';
        setError(msg);
        setStatus((s) => (s === 'pending' ? 'idle' : s));
        patchItem(item.id, { status: 'error', error: msg });
      }
    },
    [patchItem, pollUntilDone],
  );

  // Drain the queue one item at a time. Re-entrancy is guarded by `running`, so
  // enqueuing more files while a batch is in flight just extends the run.
  const runNext = useCallback(async () => {
    if (running.current) return;
    const next = queueRef.current.find((it) => it.status === 'queued');
    if (!next) return;
    running.current = true;
    await processItem(next);
    running.current = false;
    if (queueRef.current.some((it) => it.status === 'queued')) void runNext();
  }, [processItem]);

  const enqueue = useCallback(
    (req: ForgeRequest) => {
      const items = req.files.map((file): QueueItem => {
        // PDFs can't be decoded by the image worker — route them to the OCR
        // pipeline regardless of the selected mode (the gateway rejects the
        // mismatch anyway; this keeps mixed batches flowing).
        const kind = file.type === 'application/pdf' ? 'ocr' : req.kind;
        return {
          id: nextId(),
          file,
          fileName: file.name,
          kind,
          operations: kind === 'image' ? req.operations : [],
          params: kind === 'image' ? req.params : undefined,
          status: 'queued',
        };
      });
      setQ((q) => [...q, ...items]);
      void runNext();
    },
    [setQ, runNext],
  );

  const clearFinished = useCallback(() => {
    setQ((q) => q.filter((it) => it.status === 'queued' || it.status === 'active'));
  }, [setQ]);

  // Reload a job (from history, a queue card or the gallery) into the main
  // panel. No original preview survives a reload, so the before/after slider
  // is skipped. Gallery jobs can still be in flight (even another client's),
  // so a non-terminal job keeps polling until it settles.
  const loadJob = useCallback(
    (entry: { job_id: string; trace_id?: string }) => {
      currentJob.current = entry.job_id;
      setJobId(entry.job_id);
      setTraceId(entry.trace_id ?? null);
      if (originalUrlRef.current) URL.revokeObjectURL(originalUrlRef.current);
      originalUrlRef.current = null;
      setOriginalUrl(null);
      setOriginalBytes(null);
      setError(null);
      setEvents([]);
      setArtifacts([]);
      setStatus('pending');
      getJob(entry.job_id)
        .then(({ job, artifacts: arts }) => {
          setStatus((s) => advance(s, job.status));
          setArtifacts(arts);
          if (job.status !== 'completed' && job.status !== 'failed') {
            rt.current?.subscribe(entry.job_id);
            void pollUntilDone(entry.job_id);
          }
        })
        .catch(() => setError('failed to load job from history'));
    },
    [pollUntilDone],
  );

  const onSelectQueue = useCallback(
    (item: QueueItem) => {
      if (item.jobId) loadJob({ job_id: item.jobId });
    },
    [loadJob],
  );

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

      <Uploader onForge={enqueue} />

      <ForgeQueue items={queue} activeId={jobId} onSelect={onSelectQueue} onClear={clearFinished} />

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

      {status === 'completed' && <ForgeStats artifacts={artifacts} originalBytes={originalBytes} />}

      <div className="grid2" style={{ marginTop: 30 }}>
        <EventLog events={events} />
        <Artifacts artifacts={artifacts} />
      </div>

      <Gallery
        activeId={jobId}
        onSelect={(id) => loadJob({ job_id: id })}
        refreshKey={galleryTick}
      />

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
