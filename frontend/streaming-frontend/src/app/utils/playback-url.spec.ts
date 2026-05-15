import { resolvePlaybackUrl } from './playback-url';

const BASE = 'http://localhost:8080';

describe('resolvePlaybackUrl', () => {
  let warnSpy: jasmine.Spy;

  beforeEach(() => {
    warnSpy = spyOn(console, 'warn');
  });

  describe('empty inputs', () => {
    it('returns null for undefined', () => {
      expect(resolvePlaybackUrl(undefined, BASE)).toBeNull();
      expect(warnSpy).not.toHaveBeenCalled();
    });

    it('returns null for null', () => {
      expect(resolvePlaybackUrl(null, BASE)).toBeNull();
      expect(warnSpy).not.toHaveBeenCalled();
    });

    it('returns null for empty string', () => {
      expect(resolvePlaybackUrl('', BASE)).toBeNull();
      expect(warnSpy).not.toHaveBeenCalled();
    });

    it('returns null for whitespace-only string', () => {
      expect(resolvePlaybackUrl('   ', BASE)).toBeNull();
      expect(warnSpy).not.toHaveBeenCalled();
    });
  });

  describe('valid relative paths', () => {
    it('resolves a path under /hls/ against the api base', () => {
      expect(resolvePlaybackUrl('/hls/vod/abc/index.m3u8', BASE)).toBe(
        'http://localhost:8080/hls/vod/abc/index.m3u8',
      );
    });

    it('prepends a leading slash if missing', () => {
      expect(resolvePlaybackUrl('hls/vod/abc/index.m3u8', BASE)).toBe(
        'http://localhost:8080/hls/vod/abc/index.m3u8',
      );
    });

    it('preserves query strings', () => {
      expect(resolvePlaybackUrl('/hls/live/key/index.m3u8?token=xyz', BASE)).toBe(
        'http://localhost:8080/hls/live/key/index.m3u8?token=xyz',
      );
    });
  });

  describe('valid absolute paths', () => {
    it('allows an absolute URL on the same host', () => {
      expect(
        resolvePlaybackUrl('http://localhost:8080/hls/vod/abc/index.m3u8', BASE),
      ).toBe('http://localhost:8080/hls/vod/abc/index.m3u8');
    });
  });

  describe('rejections', () => {
    it('rejects a different host', () => {
      expect(resolvePlaybackUrl('http://evil.com/hls/x.m3u8', BASE)).toBeNull();
      expect(warnSpy).toHaveBeenCalled();
    });

    it('rejects javascript: protocol', () => {
      expect(resolvePlaybackUrl('javascript:alert(1)', BASE)).toBeNull();
      expect(warnSpy).toHaveBeenCalled();
    });

    it('rejects data: protocol', () => {
      expect(resolvePlaybackUrl('data:text/html,<script>1</script>', BASE)).toBeNull();
      expect(warnSpy).toHaveBeenCalled();
    });

    it('rejects protocol-relative URLs', () => {
      expect(resolvePlaybackUrl('//evil.com/hls/x.m3u8', BASE)).toBeNull();
      expect(warnSpy).toHaveBeenCalled();
    });

    it('rejects path traversal', () => {
      expect(resolvePlaybackUrl('/hls/../etc/passwd', BASE)).toBeNull();
      expect(warnSpy).toHaveBeenCalled();
    });

    it('rejects paths outside the /hls/ scope', () => {
      expect(resolvePlaybackUrl('/videos/abc.m3u8', BASE)).toBeNull();
      expect(warnSpy).toHaveBeenCalled();
    });

    it('rejects an invalid api base url', () => {
      expect(resolvePlaybackUrl('/hls/x.m3u8', 'not a url')).toBeNull();
      expect(warnSpy).toHaveBeenCalled();
    });
  });
});
