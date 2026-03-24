import { ChangeDetectionStrategy, Component, ElementRef, OnChanges, OnDestroy, SimpleChanges, ViewChild, input } from '@angular/core';
import Hls from 'hls.js';

@Component({
  selector: 'app-video-player',
  templateUrl: './video-player.component.html',
  styleUrl: './video-player.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class VideoPlayerComponent implements OnChanges, OnDestroy {
  src = input<string | null>(null);
  poster = input<string>('');

  @ViewChild('videoElement', { static: true }) private videoElement!: ElementRef<HTMLVideoElement>;

  private hls?: Hls;

  ngOnChanges(changes: SimpleChanges): void {
    if (changes['src']) {
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
      this.hls = new Hls();
      this.hls.loadSource(source);
      this.hls.attachMedia(video);
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
