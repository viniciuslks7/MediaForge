import { useCallback, useRef, useState } from 'react';

/**
 * Compare renders a draggable before/after slider: the original upload on the
 * left, the forged result on the right, revealed by sweeping the divider. The
 * orange handle (or anywhere on the frame) drags directly; arrow keys nudge it
 * when the handle is focused.
 */
export function Compare({ before, after }: { before: string; after: string }) {
  const [pos, setPos] = useState(50);
  const frameRef = useRef<HTMLDivElement>(null);
  const dragging = useRef(false);

  const posFromX = useCallback((clientX: number) => {
    const rect = frameRef.current?.getBoundingClientRect();
    if (!rect || rect.width === 0) return;
    const pct = ((clientX - rect.left) / rect.width) * 100;
    setPos(Math.min(100, Math.max(0, pct)));
  }, []);

  const onPointerDown = (e: React.PointerEvent<HTMLDivElement>) => {
    dragging.current = true;
    e.currentTarget.setPointerCapture(e.pointerId);
    posFromX(e.clientX);
  };
  const onPointerMove = (e: React.PointerEvent<HTMLDivElement>) => {
    if (dragging.current) posFromX(e.clientX);
  };
  const endDrag = () => {
    dragging.current = false;
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    const step = e.shiftKey ? 10 : 2;
    if (e.key === 'ArrowLeft') setPos((p) => Math.max(0, p - step));
    else if (e.key === 'ArrowRight') setPos((p) => Math.min(100, p + step));
    else if (e.key === 'Home') setPos(0);
    else if (e.key === 'End') setPos(100);
    else return;
    e.preventDefault();
  };

  return (
    <section className="compare">
      <div className="eyebrow">Before · after</div>
      <div
        ref={frameRef}
        className="cmp-frame"
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={endDrag}
        onPointerCancel={endDrag}
      >
        <img className="cmp-img" src={before} alt="original upload" draggable={false} />
        <div className="cmp-after" style={{ clipPath: `inset(0 0 0 ${pos}%)` }}>
          <img className="cmp-img" src={after} alt="forged result" draggable={false} />
        </div>
        <div className="cmp-divider" style={{ left: `${pos}%` }}>
          <span
            className="cmp-handle"
            role="slider"
            tabIndex={0}
            aria-label="Before/after comparison position"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={Math.round(pos)}
            onKeyDown={onKeyDown}
          />
        </div>
        <span className="cmp-tag left">original</span>
        <span className="cmp-tag right">forged</span>
      </div>
    </section>
  );
}
