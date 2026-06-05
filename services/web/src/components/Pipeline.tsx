import { motion } from 'framer-motion';
import type { Status } from '../types';

export type Stage = Status | 'idle';

interface Node {
  key: string;
  label: string;
  svc: string;
  icon: string;
}

// The five stages a job flows through — each maps to a real service in the stack.
const NODES: Node[] = [
  { key: 'ingest',  label: 'Ingest',  svc: 'api-gateway',      icon: '⬡' },
  { key: 'queue',   label: 'Queue',   svc: 'rabbitmq',         icon: '≋' },
  { key: 'forge',   label: 'Forge',   svc: 'worker',           icon: '⚒' },
  { key: 'store',   label: 'Store',   svc: 'minio',            icon: '◫' },
  { key: 'deliver', label: 'Deliver', svc: 'realtime-gateway', icon: '➤' },
];

// How far down the rail each status has reached (node index, 0..4).
const REACH: Record<Stage, number> = {
  idle: -1,
  pending: 1,
  processing: 2,
  completed: 4,
  failed: 2,
};

export function Pipeline({ status }: { status: Stage }) {
  const reach = REACH[status];
  const fill =
    status === 'completed' ? 100 : reach < 0 ? 0 : (reach / (NODES.length - 1)) * 100;

  return (
    <section className="pipeline">
      <div className="eyebrow">Distributed pipeline · live</div>
      <div className="rail">
        <div className="track">
          <motion.div
            className="fill"
            animate={{ width: `${fill}%` }}
            transition={{ duration: 0.7, ease: [0.4, 0, 0.2, 1] }}
          />
        </div>
        {NODES.map((n, i) => {
          let cls = 'node';
          if (status === 'failed' && i === REACH.failed) cls += ' fail';
          else if (status === 'completed') cls += ' done';
          else if (i < reach) cls += ' done';
          else if (i === reach) cls += status === 'processing' ? ' active pulse' : ' active';
          return (
            <div key={n.key} className={cls}>
              <div className="disc" aria-hidden>{n.icon}</div>
              <div className="lbl">{n.label}</div>
              <div className="svc">{n.svc}</div>
            </div>
          );
        })}
      </div>
    </section>
  );
}
