import { useEffect, useState, type MouseEvent } from 'react';
import { motion } from 'framer-motion';
import type { Artifact } from '../types';
import { proxiedArtifactUrl } from '../api';
import { humanSize } from '../utils';

const num = (v: unknown): number | undefined => (typeof v === 'number' ? v : undefined);

/** OCR artifacts are plain text — fetch and render them inline like a terminal. */
function OcrText({ url }: { url?: string }) {
  const [text, setText] = useState('loading…');
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    if (!url) return;
    const ac = new AbortController();
    fetch(url, { signal: ac.signal })
      .then((r) => r.text())
      .then((t) => setText(t.trim() || '(no text detected)'))
      .catch((e) => {
        if (e.name !== 'AbortError') setText('(failed to load text)');
      });
    return () => ac.abort();
  }, [url]);

  const copy = (e: MouseEvent) => {
    e.preventDefault();
    void navigator.clipboard?.writeText(text).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  };

  return (
    <div className="ocr-wrap">
      <button type="button" className="copy-btn" onClick={copy} aria-label="Copy extracted text">
        {copied ? 'copied ✓' : 'copy'}
      </button>
      <div className="ocr-text">{text}</div>
    </div>
  );
}

/** Per-artifact insight badges drawn from the worker-produced metadata. */
function ArtifactBadges({ art, isImage }: { art: Artifact; isImage: boolean }) {
  const meta = art.metadata ?? {};
  const badges: string[] = [];

  if (isImage) {
    const w = num(meta.width);
    const h = num(meta.height);
    if (w && h) badges.push(`${w}×${h}`);
  } else {
    const words = num(meta.word_count);
    const conf = num(meta.mean_confidence);
    if (words !== undefined) badges.push(`${words} word${words === 1 ? '' : 's'}`);
    if (conf !== undefined) badges.push(`conf ${conf.toFixed(0)}%`);
  }

  if (badges.length === 0) return null;
  return (
    <span className="badges">
      {badges.map((b) => (
        <span key={b} className="badge">
          {b}
        </span>
      ))}
    </span>
  );
}

function ArtifactCard({ art }: { art: Artifact }) {
  const url = proxiedArtifactUrl(art.url);
  const isImage = art.content_type.startsWith('image/');
  return (
    <motion.div
      className="art"
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.4, ease: 'easeOut' }}
    >
      <a href={url} target="_blank" rel="noreferrer">
        {isImage ? (
          <img className="thumb" src={url} alt={art.name} loading="lazy" />
        ) : (
          <OcrText url={url} />
        )}
        <div className="cap">
          <span className="nm">{art.name}</span>
          <ArtifactBadges art={art} isImage={isImage} />
          <span className="sz">{humanSize(art.size_bytes)}</span>
        </div>
      </a>
    </motion.div>
  );
}

export function Artifacts({ artifacts }: { artifacts: Artifact[] }) {
  return (
    <div className="panel">
      <div className="panel-head">
        <span>artifacts · minio</span>
        <span className="dots">
          <i /><i /><i />
        </span>
      </div>
      <div className={`arts${artifacts.length === 0 ? ' empty-state' : ''}`}>
        {artifacts.length === 0 ? (
          <div className="empty">
            forged outputs land here —<br />
            thumbnails, webp, extracted text
          </div>
        ) : (
          artifacts.map((a) => <ArtifactCard key={a.name} art={a} />)
        )}
      </div>
    </div>
  );
}
