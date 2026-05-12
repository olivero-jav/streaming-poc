import { writeFile, mkdir } from 'node:fs/promises';
import { dirname } from 'node:path';
import { chromium, type Browser } from 'playwright';
import { SCENARIOS, type ScenarioName } from './scenarios.js';

export interface Sample {
  t_ms: number;
  phase: 'baseline' | 'stress' | 'recovery';
  latency: number | null;
  current_time: number | null;
  buffered_ahead: number | null;
  target_latency: number | null;
  max_latency: number | null;
}

export interface MeasureOptions {
  watchUrl: string;
  scenario: ScenarioName;
  outputCsv: string;
  baselineMs: number;
  stressMs: number;
  recoveryMs: number;
  sampleEveryMs: number;
  headless: boolean;
}

const sleep = (ms: number) => new Promise<void>((r) => setTimeout(r, ms));

const SAMPLE_FN = () => {
  const w = window as unknown as { __hls?: { latency: number; targetLatency: number; maxLatency: number } };
  const v = document.querySelector('video') as HTMLVideoElement | null;
  let buffered: number | null = null;
  if (v && v.buffered.length > 0) {
    buffered = v.buffered.end(v.buffered.length - 1) - v.currentTime;
  }
  return {
    latency: w.__hls?.latency ?? null,
    target_latency: w.__hls?.targetLatency ?? null,
    max_latency: w.__hls?.maxLatency ?? null,
    current_time: v?.currentTime ?? null,
    buffered_ahead: buffered,
  };
};

export async function measure(opts: MeasureOptions): Promise<Sample[]> {
  const browser: Browser = await chromium.launch({ headless: opts.headless });
  const context = await browser.newContext();
  const page = await context.newPage();
  const cdp = await context.newCDPSession(page);

  // Surface page console errors — invaluable during development.
  page.on('pageerror', (err) => console.error(`[page error] ${err.message}`));

  console.log(`[${opts.scenario}] navigating to ${opts.watchUrl}`);
  await page.goto(opts.watchUrl, { waitUntil: 'domcontentloaded' });

  // Wait for hls.js to attach and start producing latency readings.
  await page.waitForFunction(
    () => {
      const w = window as unknown as { __hls?: { latency?: number } };
      return typeof w.__hls?.latency === 'number';
    },
    null,
    { timeout: 60_000 },
  );
  console.log(`[${opts.scenario}] hls instance ready`);

  // Chrome blocks autoplay; force muted play and wait until the video actually advances.
  await page.evaluate(() => {
    const v = document.querySelector('video') as HTMLVideoElement | null;
    if (v) {
      v.muted = true;
      void v.play().catch(() => {});
    }
  });
  await page.waitForFunction(
    () => {
      const v = document.querySelector('video') as HTMLVideoElement | null;
      return !!v && v.currentTime > 1 && !v.paused;
    },
    null,
    { timeout: 30_000 },
  );
  console.log(`[${opts.scenario}] playback started`);

  const samples: Sample[] = [];
  const start = Date.now();
  const scenario = SCENARIOS[opts.scenario];

  const stressStartAt = opts.baselineMs;
  const stressEndAt = stressStartAt + opts.stressMs;
  const totalMs = stressEndAt + opts.recoveryMs;

  let stressApplied = false;
  let stressReverted = false;

  while (Date.now() - start < totalMs) {
    const t_ms = Date.now() - start;

    if (!stressApplied && t_ms >= stressStartAt) {
      console.log(`[${opts.scenario}] applying stress @ ${t_ms}ms`);
      await scenario.apply({ page, cdp });
      stressApplied = true;
    }
    if (stressApplied && !stressReverted && t_ms >= stressEndAt) {
      console.log(`[${opts.scenario}] reverting stress @ ${t_ms}ms`);
      await scenario.revert({ page, cdp });
      stressReverted = true;
    }

    const phase: Sample['phase'] = t_ms < stressStartAt ? 'baseline' : t_ms < stressEndAt ? 'stress' : 'recovery';

    try {
      const data = await page.evaluate(SAMPLE_FN);
      samples.push({ t_ms, phase, ...data });
    } catch (err) {
      samples.push({ t_ms, phase, latency: null, current_time: null, buffered_ahead: null, target_latency: null, max_latency: null });
    }

    await sleep(opts.sampleEveryMs);
  }

  if (stressApplied && !stressReverted) {
    await scenario.revert({ page, cdp });
  }

  await browser.close();

  await mkdir(dirname(opts.outputCsv), { recursive: true });
  const header = 't_ms,phase,latency,current_time,buffered_ahead,target_latency,max_latency';
  const body = samples
    .map((s) => [s.t_ms, s.phase, s.latency, s.current_time, s.buffered_ahead, s.target_latency, s.max_latency].map((v) => (v === null ? '' : v)).join(','))
    .join('\n');
  await writeFile(opts.outputCsv, `${header}\n${body}\n`, 'utf8');

  console.log(`[${opts.scenario}] wrote ${samples.length} samples → ${opts.outputCsv}`);
  return samples;
}
