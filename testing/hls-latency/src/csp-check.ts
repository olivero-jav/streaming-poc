import { chromium, type ConsoleMessage } from 'playwright';

interface Finding {
  url: string;
  type: 'console' | 'pageerror';
  level: string;
  text: string;
}

const ROUTES = ['/', '/app/vod', '/app/live'];
const BASE = process.env.BASE_URL ?? 'http://localhost:8080';

const CSP_PATTERNS = [
  /content security policy/i,
  /refused to (load|execute|apply)/i,
  /blocked by CSP/i,
  /violates the following content security policy/i,
];

function isCspViolation(text: string): boolean {
  return CSP_PATTERNS.some((re) => re.test(text));
}

async function visit(route: string): Promise<Finding[]> {
  const findings: Finding[] = [];
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();

  page.on('console', (msg: ConsoleMessage) => {
    const text = msg.text();
    if (msg.type() === 'error' || isCspViolation(text)) {
      findings.push({ url: route, type: 'console', level: msg.type(), text });
    }
  });
  page.on('pageerror', (err) => {
    findings.push({ url: route, type: 'pageerror', level: 'error', text: err.message });
  });

  const url = `${BASE}${route}`;
  try {
    await page.goto(url, { waitUntil: 'networkidle', timeout: 15_000 });
    // Linger briefly so async loads (Angular bootstrap, lazy chunks) settle.
    await page.waitForTimeout(2000);
  } catch (err) {
    findings.push({ url: route, type: 'pageerror', level: 'error', text: `goto failed: ${(err as Error).message}` });
  } finally {
    await browser.close();
  }
  return findings;
}

async function main() {
  console.log(`csp-check against ${BASE}`);
  const all: Finding[] = [];
  for (const route of ROUTES) {
    console.log(`\n--- ${route} ---`);
    const findings = await visit(route);
    if (findings.length === 0) {
      console.log('  clean');
    } else {
      for (const f of findings) {
        console.log(`  [${f.type}/${f.level}] ${f.text}`);
      }
    }
    all.push(...findings);
  }
  const cspViolations = all.filter((f) => isCspViolation(f.text));
  console.log(`\n=== summary ===`);
  console.log(`routes checked: ${ROUTES.length}`);
  console.log(`total findings: ${all.length}`);
  console.log(`csp violations: ${cspViolations.length}`);
  process.exit(cspViolations.length > 0 ? 1 : 0);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
