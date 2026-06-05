import { NodeTracerProvider } from '@opentelemetry/sdk-trace-node';
import {
  BatchSpanProcessor,
  ParentBasedSampler,
  TraceIdRatioBasedSampler,
} from '@opentelemetry/sdk-trace-base';
import { resourceFromAttributes } from '@opentelemetry/resources';
import { OTLPTraceExporter } from '@opentelemetry/exporter-trace-otlp-http';
import type { Config } from './config.js';

/**
 * initTracing installs a global tracer provider exporting spans over OTLP/HTTP
 * to the collector. `provider.register()` also installs the default W3C
 * trace-context propagator, so context extracted from RabbitMQ event headers
 * continues the trace the gateway and workers started. Returns the provider
 * (call `.shutdown()` on exit) or undefined when tracing is disabled.
 */
export function initTracing(cfg: Config): NodeTracerProvider | undefined {
  if (!cfg.otelEnabled || !cfg.otelEndpoint) return undefined;

  const base = cfg.otelEndpoint.replace(/\/$/, '');
  const exporter = new OTLPTraceExporter({ url: `${base}/v1/traces` });

  const provider = new NodeTracerProvider({
    resource: resourceFromAttributes({
      'service.name': cfg.serviceName,
      'service.version': cfg.serviceVersion,
    }),
    sampler: new ParentBasedSampler({
      root: new TraceIdRatioBasedSampler(cfg.otelSampleRatio || 1),
    }),
    spanProcessors: [new BatchSpanProcessor(exporter)],
  });

  provider.register();
  return provider;
}
