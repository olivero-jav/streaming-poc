import { spawn, type ChildProcess } from 'node:child_process';
import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { measure } from './measure.js';
import type { ScenarioName } from './scenarios.js';
import { generateReport } from './reporter.js';

interface RunConfig {
  apiBase: string;
  frontendBase: string;
  rtmpBase: string;
  outputRoot: string;
  scenarios: ScenarioName[];
  baselineMs: number;
  stressMs: number;
  recoveryMs: number;
  sampleEveryMs: number;
  headless: boolean;
  firstSegmentTimeoutMs: number;
}

const CONFIG: RunConfig = {
  apiBase: process.env.API_BASE ?? 'http://localhost:8080',
  frontendBase: process.env.FRONTEND_BASE ?? 'http://localhost:4200',
  rtmpBase: process.env.RTMP_BASE ?? 'rtmp://localhost:1935/live',
  outputRoot: join(process.cwd(), 'results', new Date().toISOString().replace(/[:.]/g, '-')),
  scenarios: (process.env.SCENARIOS?.split(',') as ScenarioName[]) ?? ['baseline', 'throttle', 'offline', 'cpu', 'pause'],
  baselineMs: Number(process.env.BASELINE_MS ?? 30_000),
  stressMs: Number(process.env.STRESS_MS ?? 30_000),
  recoveryMs: Number(process.env.RECOVERY_MS ?? 60_000),
  sampleEveryMs: Number(process.env.SAMPLE_MS ?? 1000),
  headless: process.env.HEADFUL !== '1',
  firstSegmentTimeoutMs: 60_000,
};

const sleep = (ms: number) => new Promise<void>((r) => setTimeout(r, ms));

interface CreatedStream {
  id: string;
  stream_key: string;
}

async function createStream(apiBase: string, title: string): Promise<CreatedStream> {
  const res = await fetch(`${apiBase}/streams`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ title }),
  });
  if (!res.ok) {
    throw new Error(`createStream failed: ${res.status} ${await res.text()}`);
  }
  const body = (await res.json()) as CreatedStream;
  console.log(`created stream id=${body.id} key=${body.stream_key}`);
  return body;
}

async function waitForLive(apiBase: string, id: string, timeoutMs: number): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const res = await fetch(`${apiBase}/streams/${id}`);
    if (res.ok) {
      const body = (await res.json()) as { status: string; hls_path?: string };
      if (body.status === 'live' && body.hls_path) {
        console.log(`stream ${id} is live`);
        return;
      }
    }
    await sleep(1000);
  }
  throw new Error(`stream ${id} did not reach 'live' within ${timeoutMs}ms`);
}

function spawnFfmpeg(rtmpUrl: string): ChildProcess {
  const args = [
    '-re',
    '-f', 'lavfi',
    '-i', 'testsrc=size=1280x720:rate=30',
    '-f', 'lavfi',
    '-i', 'sine=frequency=440:sample_rate=44100',
    '-c:v', 'libx264',
    '-preset', 'veryfast',
    '-tune', 'zerolatency',
    '-pix_fmt', 'yuv420p',
    '-g', '60',
    '-c:a', 'aac',
    '-ar', '44100',
    '-b:a', '128k',
    '-f', 'flv',
    rtmpUrl,
  ];
  console.log(`spawning ffmpeg → ${rtmpUrl}`);
  const proc = spawn('ffmpeg', args, { stdio: ['ignore', 'ignore', 'pipe'] });
  proc.stderr?.on('data', () => {});
  proc.on('exit', (code, sig) => console.log(`ffmpeg exited code=${code} sig=${sig}`));
  return proc;
}

function killFfmpeg(proc: ChildProcess): Promise<void> {
  return new Promise((resolve) => {
    if (proc.exitCode !== null) return resolve();
    proc.once('exit', () => resolve());
    proc.kill('SIGTERM');
    setTimeout(() => {
      if (proc.exitCode === null) proc.kill('SIGKILL');
    }, 3000);
  });
}

async function main() {
  console.log('hls-latency run starting with config:', CONFIG);

  await mkdir(CONFIG.outputRoot, { recursive: true });

  for (const scenario of CONFIG.scenarios) {
    console.log(`\n=== scenario: ${scenario} ===`);

    const stream = await createStream(CONFIG.apiBase, `latency-probe-${scenario}-${Date.now()}`);
    const rtmpUrl = `${CONFIG.rtmpBase}/${stream.stream_key}`;
    const ffmpeg = spawnFfmpeg(rtmpUrl);

    try {
      await waitForLive(CONFIG.apiBase, stream.id, CONFIG.firstSegmentTimeoutMs);
      // Let the live edge settle a couple of segments before sampling.
      await sleep(5000);

      const watchUrl = `${CONFIG.frontendBase}/watch/live/${stream.id}?debug=hls`;
      const outputCsv = join(CONFIG.outputRoot, `${scenario}.csv`);

      await measure({
        watchUrl,
        scenario,
        outputCsv,
        baselineMs: CONFIG.baselineMs,
        stressMs: CONFIG.stressMs,
        recoveryMs: CONFIG.recoveryMs,
        sampleEveryMs: CONFIG.sampleEveryMs,
        headless: CONFIG.headless,
      });
    } finally {
      console.log('stopping ffmpeg…');
      await killFfmpeg(ffmpeg);
      // Allow backend to recognize unpublish before next scenario.
      await sleep(5000);
    }
  }

  await generateReport(CONFIG.outputRoot);
}

main().catch((err) => {
  console.error('run failed:', err);
  process.exit(1);
});
