/** Environment-driven configuration (12-factor). */
export interface Config {
  rabbitUrl: string;
  httpPort: number;
  metricsPath: string;
  otelEnabled: boolean;
  otelEndpoint: string;
  otelSampleRatio: number;
  serviceName: string;
  serviceVersion: string;
}

function req(key: string): string {
  const v = process.env[key]?.trim();
  if (!v) throw new Error(`missing required env: ${key}`);
  return v;
}

export function loadConfig(): Config {
  return {
    rabbitUrl: req('RABBITMQ_URL'),
    httpPort: Number(process.env.RT_HTTP_PORT ?? '8090'),
    metricsPath: process.env.RT_METRICS_PATH ?? '/metrics',
    otelEnabled: (process.env.OTEL_TRACES_ENABLED ?? 'false').toLowerCase() === 'true',
    otelEndpoint: process.env.OTEL_EXPORTER_OTLP_ENDPOINT ?? 'http://otel-collector:4318',
    otelSampleRatio: Number(process.env.OTEL_TRACES_SAMPLE_RATIO ?? '1.0'),
    serviceName: process.env.OTEL_SERVICE_NAME ?? 'realtime-gateway',
    serviceVersion: process.env.SERVICE_VERSION ?? 'dev',
  };
}
