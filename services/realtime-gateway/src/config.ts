/** Environment-driven configuration (12-factor). */
export interface Config {
  rabbitUrl: string;
  httpPort: number;
  metricsPath: string;
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
  };
}
