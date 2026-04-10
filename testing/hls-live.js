/**
 * hls-live: espectadores concurrentes sobre live (manifest variable + segmentos).
 *
 * Requiere STREAM_ID en live y publisher RTMP activo.
 *
 * Ejemplo:
 *   $env:BASE_URL="http://localhost:8080"
 *   $env:STREAM_ID="uuid-del-stream"
 *   $env:VUS="10"; $env:DURATION="15m"
 *   k6 run testing/hls-live.js
 *
 * Reportes generados en testing/reports/hls-live-<timestamp>.{html,json}
 */
import http from 'k6/http';
import { check, sleep } from 'k6';
import { htmlReport } from 'https://raw.githubusercontent.com/benc-uk/k6-reporter/main/dist/bundle.js';
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.0.1/index.js';

const base = (__ENV.BASE_URL || 'http://localhost:8080').replace(/\/$/, '');

export function setup() {
  const streamId = (__ENV.STREAM_ID || '').trim();
  if (!streamId) {
    throw new Error('STREAM_ID es obligatorio (env STREAM_ID)');
  }
  const url = `${base}/hls/live/${streamId}/index.m3u8`;
  const res = http.get(url);
  if (res.status !== 200) {
    throw new Error(`Playlist live no disponible: GET ${url} -> ${res.status}`);
  }
  return { streamId };
}

function segmentNamesFromM3U8(body) {
  if (!body) return [];
  const lines = String(body).split(/\r?\n/);
  const segs = [];
  for (const line of lines) {
    const t = line.trim();
    if (!t || t.startsWith('#')) continue;
    if (t.startsWith('http://') || t.startsWith('https://')) continue;
    segs.push(t);
  }
  return segs;
}

export const options = {
  vus: Number(__ENV.VUS || 10),
  duration: __ENV.DURATION || '15m',
  thresholds: {
    http_req_failed: [__ENV.THRESHOLD_FAILED || 'rate<0.015'],
    'http_req_duration{name:hls_live_playlist}': [__ENV.THRESHOLD_P95_PLAYLIST || 'p(95)<500'],
    checks: ['rate>0.98'],
  },
};

export default function (data) {
  const playlistUrl = `${base}/hls/live/${data.streamId}/index.m3u8`;

  if (Math.random() < 0.25) {
    const plRes = http.get(playlistUrl, { tags: { name: 'hls_live_playlist' } });
    check(plRes, {
      'playlist 200': (r) => r.status === 200,
      'playlist body': (r) => (r.body || '').length > 0,
    });
    sleep(0.5 + Math.random() * 1.0);
    return;
  }

  const plRes = http.get(playlistUrl, { tags: { name: 'hls_live_playlist' } });
  check(plRes, {
    'playlist 200': (r) => r.status === 200,
    'playlist body': (r) => (r.body || '').length > 0,
  });

  if (plRes.status !== 200) {
    sleep(0.5);
    return;
  }

  const segs = segmentNamesFromM3U8(plRes.body);
  if (!segs.length) {
    sleep(0.3);
    return;
  }

  const k = 2 + Math.floor(Math.random() * 3);
  const start = Math.max(0, segs.length - k);
  for (let i = start; i < segs.length; i++) {
    const segUrl = `${base}/hls/live/${data.streamId}/${segs[i]}`;
    const segRes = http.get(segUrl, { tags: { name: 'hls_live_segment' } });
    check(segRes, { 'segment 200': (r) => r.status === 200 });
  }

  sleep(0.4 + Math.random() * 0.9);
}

export function handleSummary(data) {
  const ts = new Date().toISOString().slice(0, 16).replace(/:/g, '-');
  const base = `testing/reports/hls-live-${ts}`;
  return {
    [`${base}.html`]: htmlReport(data),
    [`${base}.json`]: JSON.stringify(data, null, 2),
    stdout: textSummary(data, { indent: ' ', enableColors: true }),
  };
}
