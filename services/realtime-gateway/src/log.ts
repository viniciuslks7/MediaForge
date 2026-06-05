import { trace } from '@opentelemetry/api';

type Level = 'INFO' | 'WARN' | 'ERROR';

/**
 * emit writes a single JSON log line. When called within an active span it tags
 * the line with trace_id/span_id, so logs and traces are cross-navigable (the
 * third observability pillar). Outside a span the trace fields are omitted.
 */
function emit(level: Level, msg: string, extra?: Record<string, unknown>): void {
  const sc = trace.getActiveSpan()?.spanContext();
  const fields: Record<string, unknown> = { level, msg, ...extra };
  if (sc) {
    fields.trace_id = sc.traceId;
    fields.span_id = sc.spanId;
  }
  const line = JSON.stringify(fields);
  if (level === 'ERROR') console.error(line);
  else if (level === 'WARN') console.warn(line);
  else console.log(line);
}

export const log = {
  info: (msg: string, extra?: Record<string, unknown>) => emit('INFO', msg, extra),
  warn: (msg: string, extra?: Record<string, unknown>) => emit('WARN', msg, extra),
  error: (msg: string, extra?: Record<string, unknown>) => emit('ERROR', msg, extra),
};
