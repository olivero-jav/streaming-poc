/**
 * api-smoke: carga ligera sobre la API de metadatos.
 *
 * Ejemplo (PowerShell):
 *   $env:BASE_URL="http://localhost:8080"
 *   $env:VUS="10"; $env:DURATION="15m"
 *   k6 run testing/api-smoke.js
 *
 * Reportes: REPORT_DIR (run.ps1), ./reports con cwd testing/, o testing/reports desde la raíz del repo.
 */
import http from 'k6/http';
import { check, sleep } from 'k6';
import { htmlReport } from 'https://raw.githubusercontent.com/benc-uk/k6-reporter/main/dist/bundle.js';
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.0.1/index.js';

const base = __ENV.BASE_URL || 'http://localhost:8080';

export const options = {
  vus: Number(__ENV.VUS || 10),
  duration: __ENV.DURATION || '15m',
  thresholds: {
    http_req_failed: [__ENV.THRESHOLD_FAILED || 'rate<0.01'],
    http_req_duration: [__ENV.THRESHOLD_P95 || 'p(95)<400'],
    checks: ['rate>0.99'],
  },
};

export function setup() {
  let backendCommit = '';
  let redisUp = null;
  try {
    const h = http.get(`${base}/health`);
    if (h.status === 200) {
      const body = h.json();
      backendCommit = body.commit || '';
      if (typeof body.redis_up === 'boolean') redisUp = body.redis_up;
    }
  } catch (_) {}
  return {
    gitCommit: __ENV.GIT_COMMIT || '',
    hostname: __ENV.HOSTNAME || '',
    backendCommit,
    redisUp,
    startedAt: new Date().toISOString(),
  };
}

function pickVideoId() {
  const res = http.get(`${base}/videos`);
  if (res.status !== 200) return null;
  let data;
  try {
    data = res.json();
  } catch {
    return null;
  }
  const items = data.items || [];
  if (!items.length) return null;
  return items[Math.floor(Math.random() * items.length)].id;
}

function pickStreamId() {
  const res = http.get(`${base}/streams`);
  if (res.status !== 200) return null;
  let data;
  try {
    data = res.json();
  } catch {
    return null;
  }
  const items = data.items || [];
  if (!items.length) return null;
  return items[Math.floor(Math.random() * items.length)].id;
}

export default function () {
  const r = Math.random() * 100;

  if (r < 10) {
    const res = http.get(`${base}/health`, { tags: { name: 'api_health' } });
    check(res, { 'health 200': (x) => x.status === 200 });
  } else if (r < 45) {
    const res = http.get(`${base}/videos`, { tags: { name: 'api_videos_list' } });
    check(res, { 'videos list 200': (x) => x.status === 200 });
  } else if (r < 70) {
    const res = http.get(`${base}/streams`, { tags: { name: 'api_streams_list' } });
    check(res, { 'streams list 200': (x) => x.status === 200 });
  } else if (r < 85) {
    const id = pickVideoId();
    if (!id) {
      sleep(0.3);
      return;
    }
    const res = http.get(`${base}/videos/${id}`, { tags: { name: 'api_video_get' } });
    check(res, { 'video get 200': (x) => x.status === 200 });
  } else {
    const id = pickStreamId();
    if (!id) {
      sleep(0.3);
      return;
    }
    const res = http.get(`${base}/streams/${id}`, { tags: { name: 'api_stream_get' } });
    check(res, { 'stream get 200': (x) => x.status === 200 });
  }

  sleep(0.3 + Math.random() * 0.7);
}

function reportDir() {
  return (__ENV.REPORT_DIR || 'reports').replace(/\\/g, '/').replace(/\/+$/, '');
}

export function handleSummary(data) {
  const ts = new Date().toISOString().slice(0, 16).replace(/:/g, '-');
  const base = `${reportDir()}/api-smoke-${ts}`;
  return {
    [`${base}.html`]: htmlReport(data),
    [`${base}.json`]: JSON.stringify(data, null, 2),
    stdout: textSummary(data, { indent: ' ', enableColors: true }),
  };
}
