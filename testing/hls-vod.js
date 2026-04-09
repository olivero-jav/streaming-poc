/**
 * hls-vod: espectadores concurrentes sobre VOD (playlist + segmentos).
 *
 * Requiere VIDEO_ID en estado ready.
 *
 * Ejemplo:
 *   $env:BASE_URL="http://localhost:8080"
 *   $env:VIDEO_ID="uuid-del-video"
 *   $env:VUS="10"; $env:DURATION="15m"
 *   k6 run testing/hls-vod.js
 */
import http from 'k6/http';
import { check, sleep } from 'k6';

const base = (__ENV.BASE_URL || 'http://localhost:8080').replace(/\/$/, '');

export function setup() {
  const videoId = (__ENV.VIDEO_ID || '').trim();
  if (!videoId) {
    throw new Error('VIDEO_ID es obligatorio (env VIDEO_ID)');
  }
  const url = `${base}/hls/vod/${videoId}/index.m3u8`;
  const res = http.get(url);
  if (res.status !== 200) {
    throw new Error(`Playlist VOD no disponible: GET ${url} -> ${res.status}`);
  }
  return { videoId };
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
    http_req_failed: [__ENV.THRESHOLD_FAILED || 'rate<0.01'],
    'http_req_duration{name:hls_vod_playlist}': [__ENV.THRESHOLD_P95_PLAYLIST || 'p(95)<400'],
    checks: ['rate>0.99'],
  },
};

export default function (data) {
  const playlistUrl = `${base}/hls/vod/${data.videoId}/index.m3u8`;
  const plRes = http.get(playlistUrl, { tags: { name: 'hls_vod_playlist' } });
  check(plRes, {
    'playlist 200': (r) => r.status === 200,
    'playlist body': (r) => (r.body || '').length > 0,
  });

  if (plRes.status !== 200) {
    sleep(1);
    return;
  }

  if (Math.random() < 0.15) {
    sleep(0.4 + Math.random() * 0.6);
    return;
  }

  const segs = segmentNamesFromM3U8(plRes.body);
  if (!segs.length) {
    sleep(1);
    return;
  }

  const n = 3 + Math.floor(Math.random() * 4);
  const start = Math.max(0, segs.length - n);
  for (let i = start; i < segs.length; i++) {
    const segUrl = `${base}/hls/vod/${data.videoId}/${segs[i]}`;
    const segRes = http.get(segUrl, { tags: { name: 'hls_vod_segment' } });
    check(segRes, { 'segment 200': (r) => r.status === 200 });
  }

  sleep(0.5 + Math.random() * 1.0);
}
