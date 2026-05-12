import { ChangeDetectionStrategy, Component, ElementRef, OnChanges, OnDestroy, SimpleChanges, ViewChild, input } from '@angular/core';
import Hls, { type HlsConfig } from 'hls.js';

export type PlayerMode = 'vod' | 'live';

// Live keeps the playhead near the edge and avoids unbounded buffer growth
// during long sessions. VOD trims back-buffer so a 2h film does not pin every
// past segment in memory.
const HLS_CONFIG: Record<PlayerMode, Partial<HlsConfig>> = {
  vod: {
    enableWorker: true,
    maxBufferLength: 30,
    backBufferLength: 30,
  },
  live: {
    enableWorker: true,
    lowLatencyMode: true,
    maxBufferLength: 10,
    backBufferLength: 0,
    liveSyncDurationCount: 3,
    liveMaxLatencyDurationCount: 6,
  },
};

@Component({
  selector: 'app-video-player',
  templateUrl: './video-player.component.html',
  styleUrl: './video-player.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class VideoPlayerComponent implements OnChanges, OnDestroy {
  src = input<string | null>(null);
  poster = input<string>('');
  mode = input<PlayerMode>('vod');

  @ViewChild('videoElement', { static: true }) private videoElement!: ElementRef<HTMLVideoElement>;

  private hls?: Hls;

  ngOnChanges(changes: SimpleChanges): void {
    if (changes['src'] || changes['mode']) {
      this.attachSource();
    }
  }

  ngOnDestroy(): void {
    this.destroyHls();
  }

  private attachSource(): void {
    const source = this.src();
    const video = this.videoElement.nativeElement;

    this.destroyHls();
    video.removeAttribute('src');
    video.load();

    if (!source) {
      return;
    }

    if (Hls.isSupported()) {
      this.hls = new Hls(HLS_CONFIG[this.mode()]);
      this.hls.loadSource(source);
      this.hls.attachMedia(video);
      if (location.search.includes('debug=hls')) {
        (window as unknown as { __hls?: Hls }).__hls = this.hls;
      }
      return;
    }

    if (video.canPlayType('application/vnd.apple.mpegurl')) {
      video.src = source;
    }
  }

  private destroyHls(): void {
    if (this.hls) {
      this.hls.destroy();
      this.hls = undefined;
    }
  }
}
