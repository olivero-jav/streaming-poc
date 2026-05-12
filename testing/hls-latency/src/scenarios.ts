import type { CDPSession, Page } from 'playwright';

export type ScenarioName = 'baseline' | 'throttle' | 'offline' | 'cpu' | 'pause';

export interface Scenario {
  name: ScenarioName;
  apply(ctx: { page: Page; cdp: CDPSession }): Promise<void>;
  revert(ctx: { page: Page; cdp: CDPSession }): Promise<void>;
}

const noop = async () => {};

export const SCENARIOS: Record<ScenarioName, Scenario> = {
  baseline: {
    name: 'baseline',
    apply: noop,
    revert: noop,
  },
  throttle: {
    name: 'throttle',
    apply: async ({ cdp }) => {
      // ~500 kbps down, 200 ms RTT — bandwidth-limited but not dead.
      await cdp.send('Network.emulateNetworkConditions', {
        offline: false,
        downloadThroughput: (500 * 1024) / 8,
        uploadThroughput: (500 * 1024) / 8,
        latency: 200,
      });
    },
    revert: async ({ cdp }) => {
      await cdp.send('Network.emulateNetworkConditions', {
        offline: false,
        downloadThroughput: -1,
        uploadThroughput: -1,
        latency: 0,
      });
    },
  },
  offline: {
    name: 'offline',
    apply: async ({ cdp }) => {
      await cdp.send('Network.emulateNetworkConditions', {
        offline: true,
        downloadThroughput: 0,
        uploadThroughput: 0,
        latency: 0,
      });
    },
    revert: async ({ cdp }) => {
      await cdp.send('Network.emulateNetworkConditions', {
        offline: false,
        downloadThroughput: -1,
        uploadThroughput: -1,
        latency: 0,
      });
    },
  },
  cpu: {
    name: 'cpu',
    apply: async ({ cdp }) => {
      await cdp.send('Emulation.setCPUThrottlingRate', { rate: 4 });
    },
    revert: async ({ cdp }) => {
      await cdp.send('Emulation.setCPUThrottlingRate', { rate: 1 });
    },
  },
  pause: {
    name: 'pause',
    apply: async ({ page }) => {
      await page.evaluate(() => {
        const v = document.querySelector('video');
        v?.pause();
      });
    },
    revert: async ({ page }) => {
      await page.evaluate(() => {
        const v = document.querySelector('video');
        v?.play().catch(() => {});
      });
    },
  },
};
