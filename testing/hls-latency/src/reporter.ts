import { readFile, readdir, writeFile } from 'node:fs/promises';
import { join, basename } from 'node:path';

interface Row {
  t_ms: number;
  phase: 'baseline' | 'stress' | 'recovery';
  latency: number | null;
  buffered_ahead: number | null;
}

interface PhaseStats {
  count: number;
  p50: number | null;
  p95: number | null;
  max: number | null;
  mean: number | null;
}

interface ScenarioReport {
  scenario: string;
  samples: number;
  latency_by_phase: Record<string, PhaseStats>;
  buffered_by_phase: Record<string, PhaseStats>;
}

function quantile(sorted: number[], q: number): number | null {
  if (sorted.length === 0) return null;
  const idx = Math.min(sorted.length - 1, Math.floor(sorted.length * q));
  return sorted[idx] ?? null;
}

function stats(values: (number | null)[]): PhaseStats {
  const clean = values.filter((v): v is number => v !== null && Number.isFinite(v) && v >= 0);
  if (clean.length === 0) {
    return { count: 0, p50: null, p95: null, max: null, mean: null };
  }
  const sorted = [...clean].sort((a, b) => a - b);
  const sum = clean.reduce((acc, v) => acc + v, 0);
  return {
    count: clean.length,
    p50: quantile(sorted, 0.5),
    p95: quantile(sorted, 0.95),
    max: sorted[sorted.length - 1] ?? null,
    mean: sum / clean.length,
  };
}

function parseCsv(text: string): Row[] {
  const lines = text.trim().split('\n');
  const rows: Row[] = [];
  for (const line of lines.slice(1)) {
    const cols = line.split(',');
    const phase = cols[1] as Row['phase'];
    if (phase !== 'baseline' && phase !== 'stress' && phase !== 'recovery') continue;
    rows.push({
      t_ms: Number(cols[0]),
      phase,
      latency: cols[2] === '' || cols[2] === undefined ? null : Number(cols[2]),
      buffered_ahead: cols[4] === '' || cols[4] === undefined ? null : Number(cols[4]),
    });
  }
  return rows;
}

function buildReport(scenarioName: string, rows: Row[]): ScenarioReport {
  const phases: Row['phase'][] = ['baseline', 'stress', 'recovery'];
  const latency_by_phase: Record<string, PhaseStats> = {};
  const buffered_by_phase: Record<string, PhaseStats> = {};
  for (const p of phases) {
    const inPhase = rows.filter((r) => r.phase === p);
    latency_by_phase[p] = stats(inPhase.map((r) => r.latency));
    buffered_by_phase[p] = stats(inPhase.map((r) => r.buffered_ahead));
  }
  return { scenario: scenarioName, samples: rows.length, latency_by_phase, buffered_by_phase };
}

function fmt(n: number | null): string {
  return n === null ? '   --   ' : n.toFixed(2).padStart(8, ' ');
}

function printTable(reports: ScenarioReport[]): void {
  console.log('\n=== Latency (hls.latency, seconds) ===');
  console.log('scenario   phase      n      p50      p95      max     mean');
  for (const r of reports) {
    for (const p of ['baseline', 'stress', 'recovery'] as const) {
      const s = r.latency_by_phase[p];
      if (!s) continue;
      console.log(
        `${r.scenario.padEnd(10)} ${p.padEnd(9)} ${String(s.count).padStart(4)} ${fmt(s.p50)} ${fmt(s.p95)} ${fmt(s.max)} ${fmt(s.mean)}`,
      );
    }
  }
  console.log('\n=== Buffered ahead (seconds) ===');
  console.log('scenario   phase      n      p50      p95      max     mean');
  for (const r of reports) {
    for (const p of ['baseline', 'stress', 'recovery'] as const) {
      const s = r.buffered_by_phase[p];
      if (!s) continue;
      console.log(
        `${r.scenario.padEnd(10)} ${p.padEnd(9)} ${String(s.count).padStart(4)} ${fmt(s.p50)} ${fmt(s.p95)} ${fmt(s.max)} ${fmt(s.mean)}`,
      );
    }
  }
}

export async function generateReport(dir: string): Promise<void> {
  const entries = await readdir(dir);
  const csvFiles = entries.filter((f) => f.endsWith('.csv')).sort();
  const reports: ScenarioReport[] = [];
  for (const f of csvFiles) {
    const text = await readFile(join(dir, f), 'utf8');
    const rows = parseCsv(text);
    const scenarioName = basename(f, '.csv');
    reports.push(buildReport(scenarioName, rows));
  }
  await writeFile(join(dir, 'summary.json'), JSON.stringify(reports, null, 2), 'utf8');
  printTable(reports);
  console.log(`\nwrote ${join(dir, 'summary.json')}`);
}

const isDirect = (() => {
  try {
    const argvUrl = new URL(`file://${process.argv[1]}`).href;
    return import.meta.url === argvUrl;
  } catch {
    return false;
  }
})();

if (isDirect) {
  const target = process.argv[2];
  if (!target) {
    console.error('usage: tsx reporter.ts <results-dir>');
    process.exit(1);
  }
  generateReport(target).catch((err) => {
    console.error(err);
    process.exit(1);
  });
}
