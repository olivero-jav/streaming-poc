const ALLOWED_PROTOCOLS = new Set(['http:', 'https:']);
const ALLOWED_PATH_PREFIX = '/hls/';

export function resolvePlaybackUrl(hlsPath: string | undefined | null, apiBaseUrl: string): string | null {
  if (!hlsPath) {
    return null;
  }

  const trimmed = hlsPath.trim();
  if (!trimmed) {
    return null;
  }

  if (trimmed.startsWith('//')) {
    console.warn('[playback-url] rejected: protocol-relative URL', hlsPath);
    return null;
  }

  let allowedHost: string;
  try {
    allowedHost = new URL(apiBaseUrl).host;
  } catch {
    console.warn('[playback-url] rejected: invalid api base url', apiBaseUrl);
    return null;
  }

  const isAbsolute = trimmed.startsWith('http://') || trimmed.startsWith('https://');
  let url: URL;
  try {
    if (isAbsolute) {
      url = new URL(trimmed);
    } else {
      if (trimmed.includes('..')) {
        console.warn('[playback-url] rejected: path traversal', hlsPath);
        return null;
      }
      const withSlash = trimmed.startsWith('/') ? trimmed : `/${trimmed}`;
      url = new URL(withSlash, apiBaseUrl);
    }
  } catch {
    console.warn('[playback-url] rejected: invalid url', hlsPath);
    return null;
  }

  if (!ALLOWED_PROTOCOLS.has(url.protocol)) {
    console.warn('[playback-url] rejected: unexpected protocol', url.protocol, hlsPath);
    return null;
  }

  if (url.host !== allowedHost) {
    console.warn('[playback-url] rejected: host mismatch', url.host, 'expected', allowedHost);
    return null;
  }

  if (!url.pathname.startsWith(ALLOWED_PATH_PREFIX)) {
    console.warn('[playback-url] rejected: path outside /hls/ scope', url.pathname);
    return null;
  }

  return url.toString();
}
