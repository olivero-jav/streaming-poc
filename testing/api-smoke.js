/**
 * api-smoke: carga ligera sobre la API de metadatos.
 *
 * Ejemplo (PowerShell):
 *   $env:BASE_URL="http://localhost:8080"
 *   $env:VUS="10"; $env:DURATION="15m"
 *   k6 run testing/api-smoke.js
 */
import http from 'k6/http';
import { check, sleep } from 'k6';

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
