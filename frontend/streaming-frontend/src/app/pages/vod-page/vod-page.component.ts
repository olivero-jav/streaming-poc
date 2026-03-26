import { ChangeDetectionStrategy, Component, DestroyRef, OnInit, ViewChild, inject, signal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatCardModule } from '@angular/material/card';
import { MatChipsModule } from '@angular/material/chips';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { MatSidenavModule } from '@angular/material/sidenav';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { interval } from 'rxjs';
import { NavComponent } from '../../components/nav/nav.component';
import { UploadDrawerComponent } from '../../components/upload-drawer/upload-drawer.component';
import { VideoPlayerComponent } from '../../components/video-player/video-player.component';
import { VideoItem } from '../../models/video.model';
import { VideoApiService } from '../../services/video-api.service';

@Component({
  selector: 'app-vod-page',
  imports: [
    MatButtonModule,
    MatCardModule,
    MatChipsModule,
    MatIconModule,
    MatProgressSpinnerModule,
    MatSidenavModule,
    NavComponent,
    UploadDrawerComponent,
    VideoPlayerComponent,
  ],
  templateUrl: './vod-page.component.html',
  styleUrl: './vod-page.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class VodPageComponent implements OnInit {
  private readonly videoApi = inject(VideoApiService);
  private readonly destroyRef = inject(DestroyRef);

  videos = signal<VideoItem[]>([]);
  selectedVideo = signal<VideoItem | null>(null);
  loading = signal(false);
  drawerOpen = signal(false);
  error = signal<string | null>(null);

  @ViewChild(UploadDrawerComponent) private uploadDrawer?: UploadDrawerComponent;

  ngOnInit(): void {
    this.loadVideos();
    interval(5000).pipe(takeUntilDestroyed(this.destroyRef)).subscribe(() => this.loadVideos());
  }

  loadVideos(): void {
    this.loading.set(true);
    this.videoApi
      .listVideos()
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: ({ items }) => {
          this.videos.set(items);
          this.syncSelectedVideo(items);
          this.error.set(null);
          this.loading.set(false);
        },
        error: () => {
          this.error.set('No se pudo cargar la lista de videos.');
          this.loading.set(false);
        },
      });
  }

  isPlayable(video: VideoItem): boolean {
    return (video.status === 'ready' || video.status === 'processing') && !!video.hls_path;
  }

  selectVideo(video: VideoItem): void {
    if (!this.isPlayable(video)) {
      return;
    }
    this.selectedVideo.set(video);
  }

  openDrawer(): void {
    this.drawerOpen.set(true);
  }

  closeDrawer(): void {
    this.drawerOpen.set(false);
    this.uploadDrawer?.resetForm();
  }

  uploadVideo(payload: { title: string; description: string; file: File }): void {
    this.videoApi
      .uploadVideo(payload)
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: () => {
          this.closeDrawer();
          this.loadVideos();
        },
        error: () => {
          this.error.set('No se pudo subir el video.');
        },
      });
  }

  isSelected(video: VideoItem): boolean {
    return this.selectedVideo()?.id === video.id;
  }

  statusChipClass(status: VideoItem['status']): string {
    switch (status) {
      case 'ready':
        return 'status-ready';
      case 'pending':
        return 'status-pending';
      case 'processing':
        return 'status-processing';
      default:
        return 'status-error';
    }
  }

  statusLabel(status: VideoItem['status']): string {
    switch (status) {
      case 'pending':
        return 'pendiente';
      case 'processing':
        return 'procesando';
      case 'ready':
        return 'listo';
      default:
        return 'error';
    }
  }

  playbackUrl(video: VideoItem | null): string | null {
    return this.videoApi.resolvePlaybackUrl(video?.hls_path);
  }

  private syncSelectedVideo(videos: VideoItem[]): void {
    const current = this.selectedVideo();
    if (current) {
      const refreshed = videos.find((video) => video.id === current.id);
      if (refreshed) {
        this.selectedVideo.set(refreshed);
        return;
      }
    }

    const firstReady = videos.find((video) => video.status === 'ready') ?? null;
    this.selectedVideo.set(firstReady);
  }
}
