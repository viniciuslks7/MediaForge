import { useCallback, useRef, useState } from 'react';
import type { JobParams, Operation, ResizeFormat } from '../types';
import { humanSize } from '../utils';

export interface ForgeRequest {
  file: File;
  kind: 'image' | 'ocr';
  operations: Operation[];
  params?: JobParams;
}

const ALL_OPS: Operation[] = ['resize', 'thumbnail', 'webp', 'grayscale'];
const FORMATS: ResizeFormat[] = ['jpeg', 'png', 'webp'];

export function Uploader({
  busy,
  onForge,
}: {
  busy: boolean;
  onForge: (req: ForgeRequest) => void;
}) {
  const [file, setFile] = useState<File | null>(null);
  const [preview, setPreview] = useState<string | null>(null);
  const [kind, setKind] = useState<'image' | 'ocr'>('image');
  const [ops, setOps] = useState<Operation[]>(['resize', 'thumbnail', 'webp']);
  const [drag, setDrag] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  // Optional per-job overrides. Empty string = "use worker default".
  const [advanced, setAdvanced] = useState(false);
  const [resizeMaxDim, setResizeMaxDim] = useState('');
  const [thumbnailSize, setThumbnailSize] = useState('');
  const [resizeFormat, setResizeFormat] = useState<ResizeFormat | ''>('');
  const [quality, setQuality] = useState('');

  const buildParams = (): JobParams | undefined => {
    const p: JobParams = {};
    if (resizeMaxDim) p.resize_max_dim = Number(resizeMaxDim);
    if (thumbnailSize) p.thumbnail_size = Number(thumbnailSize);
    if (resizeFormat) p.resize_format = resizeFormat;
    if (quality) p.quality = Number(quality);
    return Object.keys(p).length ? p : undefined;
  };

  const accept = useCallback((f: File | undefined) => {
    if (!f) return;
    setFile(f);
    setPreview((prev) => {
      if (prev) URL.revokeObjectURL(prev);
      return f.type.startsWith('image/') ? URL.createObjectURL(f) : null;
    });
  }, []);

  const toggleOp = (op: Operation) =>
    setOps((cur) => (cur.includes(op) ? cur.filter((o) => o !== op) : [...cur, op]));

  return (
    <div className="hero">
      <div className="eyebrow">Ingest</div>
      <p className="hero-lead">
        Drop a file. Watch the <em>forge</em> work it.
      </p>
      <p className="hero-sub">
        MediaForge accepts your media at the gateway, fans it out over RabbitMQ to a
        fleet of workers, and streams every state change back here in real time — image
        transforms in Go, OCR in Python.
      </p>

      <div
        className={`dropzone${drag ? ' drag' : ''}${file ? ' has-file' : ''}`}
        role="button"
        tabIndex={0}
        onClick={() => inputRef.current?.click()}
        onKeyDown={(e) => (e.key === 'Enter' || e.key === ' ') && inputRef.current?.click()}
        onDragOver={(e) => {
          e.preventDefault();
          setDrag(true);
        }}
        onDragLeave={() => setDrag(false)}
        onDrop={(e) => {
          e.preventDefault();
          setDrag(false);
          accept(e.dataTransfer.files[0]);
        }}
      >
        <input
          ref={inputRef}
          type="file"
          accept="image/*,application/pdf"
          hidden
          onChange={(e) => accept(e.target.files?.[0])}
        />
        {!file ? (
          <>
            <div className="dz-anvil" aria-hidden>⚒</div>
            <div className="dz-title">Arraste sua mídia, ou clique para escolher</div>
            <div className="dz-hint">PNG · JPG · WEBP · PDF — até 25&nbsp;MiB</div>
          </>
        ) : (
          <div className="dz-preview">
            {preview ? (
              <img src={preview} alt="preview" />
            ) : (
              <div className="dz-anvil" style={{ margin: 0 }}>
                ◫
              </div>
            )}
            <div className="dz-meta">
              <div className="n">{file.name}</div>
              <div className="s">
                {humanSize(file.size)} · {file.type || 'unknown'}
              </div>
            </div>
          </div>
        )}
      </div>

      <div className="controls">
        <div className="mode-switch" role="tablist" aria-label="Pipeline">
          <button
            role="tab"
            aria-selected={kind === 'image'}
            className={kind === 'image' ? 'on' : ''}
            onClick={() => setKind('image')}
          >
            Image
          </button>
          <button
            role="tab"
            aria-selected={kind === 'ocr'}
            className={kind === 'ocr' ? 'on' : ''}
            onClick={() => setKind('ocr')}
          >
            OCR
          </button>
        </div>

        {kind === 'image' && (
          <div className="ops" role="group" aria-label="Operations">
            {ALL_OPS.map((op) => (
              <span
                key={op}
                role="checkbox"
                aria-checked={ops.includes(op)}
                tabIndex={0}
                className={`chip${ops.includes(op) ? ' on' : ''}`}
                onClick={() => toggleOp(op)}
                onKeyDown={(e) => (e.key === 'Enter' || e.key === ' ') && toggleOp(op)}
              >
                {op}
              </span>
            ))}
          </div>
        )}

        {kind === 'image' && (
          <div className="adv">
            <button
              type="button"
              className="adv-toggle"
              aria-expanded={advanced}
              onClick={() => setAdvanced((v) => !v)}
            >
              {advanced ? '▾' : '▸'} Advanced
            </button>

            {advanced && (
              <div className="adv-panel">
                <label className="adv-field">
                  <span>Resize max edge (px)</span>
                  <input
                    type="number"
                    min={16}
                    max={8000}
                    placeholder="1600"
                    value={resizeMaxDim}
                    onChange={(e) => setResizeMaxDim(e.target.value)}
                  />
                </label>
                <label className="adv-field">
                  <span>Thumbnail size (px)</span>
                  <input
                    type="number"
                    min={16}
                    max={2000}
                    placeholder="256"
                    value={thumbnailSize}
                    onChange={(e) => setThumbnailSize(e.target.value)}
                  />
                </label>
                <label className="adv-field">
                  <span>Resize format</span>
                  <select
                    value={resizeFormat}
                    onChange={(e) => setResizeFormat(e.target.value as ResizeFormat | '')}
                  >
                    <option value="">default (jpeg)</option>
                    {FORMATS.map((f) => (
                      <option key={f} value={f}>
                        {f}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="adv-field">
                  <span>Quality (1–100)</span>
                  <input
                    type="number"
                    min={1}
                    max={100}
                    placeholder="85"
                    value={quality}
                    onChange={(e) => setQuality(e.target.value)}
                  />
                </label>
              </div>
            )}
          </div>
        )}

        <button
          className="forge-btn"
          disabled={!file || busy || (kind === 'image' && ops.length === 0)}
          onClick={() =>
            file &&
            onForge({
              file,
              kind,
              operations: ops,
              params: kind === 'image' ? buildParams() : undefined,
            })
          }
        >
          {busy ? 'Forging…' : 'Forge ▸'}
        </button>
      </div>
    </div>
  );
}
