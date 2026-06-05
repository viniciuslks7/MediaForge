import { useEffect, useState } from 'react';
import { motion } from 'framer-motion';
import type { Artifact } from '../types';
import { proxiedArtifactUrl } from '../api';
import { humanSize } from '../utils';

/** OCR artifacts are plain text — fetch and render them inline like a terminal. */
function OcrText({ url }: { url?: string }) {
  const [text, setText] = useState('loading…');
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
  return <div className="ocr-text">{text}</div>;
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
