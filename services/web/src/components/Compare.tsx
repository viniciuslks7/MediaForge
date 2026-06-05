import { useState } from 'react';

/**
 * Compare renders a draggable before/after slider: the original upload on the
 * left, the forged result on the right, revealed by sweeping the handle. Both
 * images share the source aspect ratio (the resize preserves it), so they align.
 */
export function Compare({ before, after }: { before: string; after: string }) {
  const [pos, setPos] = useState(50);

  return (
    <section className="compare">
      <div className="eyebrow">Before · after</div>
      <div className="cmp-frame">
        <img className="cmp-img" src={before} alt="original upload" draggable={false} />
        <div className="cmp-after" style={{ clipPath: `inset(0 0 0 ${pos}%)` }}>
          <img className="cmp-img" src={after} alt="forged result" draggable={false} />
        </div>
        <div className="cmp-divider" style={{ left: `${pos}%` }} aria-hidden>
          <span className="cmp-handle" />
        </div>
        <span className="cmp-tag left">original</span>
        <span className="cmp-tag right">forged</span>
      </div>
      <input
        className="cmp-range"
        type="range"
        min={0}
        max={100}
        value={pos}
        onChange={(e) => setPos(Number(e.target.value))}
        aria-label="Before/after comparison position"
      />
    </section>
  );
}
