import { ChangeDetectionStrategy, Component, DestroyRef, OnInit, inject, signal } from '@angular/core';
import { ActivatedRoute } from '@angular/router';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { VideoPlayerComponent } from '../../components/video-player/video-player.component';
import { VideoItem } from '../../models/video.model';
import { VideoApiService } from '../../services/video-api.service';

@Component({
  selector: 'app-watch-vod-page',
  imports: [VideoPlayerComponent],
  templateUrl: './watch-vod-page.component.html',
  styleUrl: './watch-vod-page.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class WatchVodPageComponent implements OnInit {
  private readonly route = inject(ActivatedRoute);
  private readonly videoApi = inject(VideoApiService);
  private readonly destroyRef = inject(DestroyRef);

  video = signal<VideoItem | null>(null);
  error = signal<string | null>(null);

  ngOnInit(): void {
    const videoId = this.route.snapshot.paramMap.get('id');
    if (!videoId) {
      this.error.set('No se encontro el video solicitado.');
      return;
    }

    this.videoApi
      .getVideo(videoId)
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (video) => {
          this.video.set(video);
          this.error.set(null);
        },
        error: () => {
          this.error.set('No se pudo cargar el video.');
        },
      });
  }

  playbackUrl(hlsPath?: string): string | null {
    return this.videoApi.resolvePlaybackUrl(hlsPath);
  }
}
