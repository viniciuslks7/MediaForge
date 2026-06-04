import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import Ajv2020 from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';
import { describe, expect, it } from 'vitest';
import type { JobEvent } from './ws/hub.js';

// Resolve the canonical schemas at the repo root, relative to this file.
function loadSchema(name: string): object {
  const url = new URL(`../../../specs/schemas/${name}`, import.meta.url);
  return JSON.parse(readFileSync(fileURLToPath(url), 'utf-8'));
}

const ajv = new Ajv2020({ strict: false });
addFormats(ajv);

describe('event contract', () => {
  const validate = ajv.compile(loadSchema('event.schema.json'));

  it('a JobEvent the gateway broadcasts satisfies the Event contract', () => {
    const event: JobEvent = {
      job_id: '9c1f0c4a-2b3d-4e5f-8a9b-0c1d2e3f4a5b',
      kind: 'image',
      status: 'processing',
      message: 'transforming',
      progress: 40,
      artifact: 'thumbnail',
      ts: Date.now(),
    };
    expect(validate(event)).toBe(true);
  });

  it('rejects an event with an out-of-range progress', () => {
    const bad = {
      job_id: 'x',
      kind: 'image',
      status: 'processing',
      progress: 150,
      ts: Date.now(),
    };
    expect(validate(bad)).toBe(false);
  });

  it('rejects an event with an unknown status', () => {
    const bad = {
      job_id: 'x',
      kind: 'image',
      status: 'exploded',
      progress: 10,
      ts: Date.now(),
    };
    expect(validate(bad)).toBe(false);
  });
});
