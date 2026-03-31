import { ChangeDetectionStrategy, Component, DestroyRef, OnInit, inject, signal } from '@angular/core';
import { ActivatedRoute } from '@angular/router';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { interval } from 'rxjs';
import { VideoPlayerComponent } from '../../components/video-player/video-player.component';
import { StreamItem } from '../../models/stream.model';
import { StreamApiService } from '../../services/stream-api.service';

@Component({
  selector: 'app-watch-live-page',
  imports: [VideoPlayerComponent, MatIconModule, MatProgressSpinnerModule],
  templateUrl: './watch-live-page.component.html',
  styleUrl: './watch-live-page.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class WatchLivePageComponent implements OnInit {
  private readonly streamApi = inject(StreamApiService);
  private readonly route = inject(ActivatedRoute);
  private readonly destroyRef = inject(DestroyRef);

  stream = signal<StreamItem | null>(null);
  error = signal<string | null>(null);

  ngOnInit(): void {
    const id = this.route.snapshot.paramMap.get('id')!;
    this.loadStream(id);
    interval(3000)
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe(() => this.loadStream(id));
  }

  playbackUrl(): string | null {
    return this.streamApi.resolvePlaybackUrl(this.stream()?.hls_path);
  }

  private loadStream(id: string): void {
    this.streamApi
      .getStream(id)
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (stream) => {
          this.stream.set(stream);
          this.error.set(null);
        },
        error: () => this.error.set('No se encontró el stream.'),
      });
  }
}
