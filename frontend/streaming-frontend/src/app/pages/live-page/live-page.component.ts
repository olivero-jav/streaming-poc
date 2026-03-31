import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  OnInit,
  inject,
  signal,
} from '@angular/core';
import { FormControl, ReactiveFormsModule, Validators } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatCardModule } from '@angular/material/card';
import { MatChipsModule } from '@angular/material/chips';
import { MatDialog, MatDialogModule, MatDialogRef } from '@angular/material/dialog';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { interval } from 'rxjs';
import { NavComponent } from '../../components/nav/nav.component';
import { VideoPlayerComponent } from '../../components/video-player/video-player.component';
import { StreamItem } from '../../models/stream.model';
import { StreamApiService } from '../../services/stream-api.service';

@Component({
  selector: 'app-create-stream-dialog',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [ReactiveFormsModule, MatButtonModule, MatDialogModule, MatFormFieldModule, MatInputModule],
  template: `
    <h2 mat-dialog-title>Nueva transmisión</h2>
    <mat-dialog-content>
      <mat-form-field class="full-width">
        <mat-label>Título</mat-label>
        <input matInput [formControl]="titleControl" (keydown.enter)="confirm()" />
      </mat-form-field>
    </mat-dialog-content>
    <mat-dialog-actions align="end">
      <button mat-button mat-dialog-close>Cancelar</button>
      <button mat-flat-button color="primary" [disabled]="titleControl.invalid" (click)="confirm()">
        Crear
      </button>
    </mat-dialog-actions>
  `,
  styles: ['.full-width { width: 100%; min-width: 280px; }'],
})
export class CreateStreamDialogComponent {
  private readonly dialogRef = inject(MatDialogRef<CreateStreamDialogComponent>);
  titleControl = new FormControl('', { nonNullable: true, validators: [Validators.required] });

  confirm(): void {
    const title = this.titleControl.value.trim();
    if (title) this.dialogRef.close(title);
  }
}

@Component({
  selector: 'app-live-page',
  imports: [
    NavComponent,
    VideoPlayerComponent,
    MatButtonModule,
    MatCardModule,
    MatChipsModule,
    MatDialogModule,
    MatIconModule,
    MatProgressSpinnerModule,
  ],
  templateUrl: './live-page.component.html',
  styleUrl: './live-page.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class LivePageComponent implements OnInit {
  private readonly streamApi = inject(StreamApiService);
  private readonly dialog = inject(MatDialog);
  private readonly destroyRef = inject(DestroyRef);

  streams = signal<StreamItem[]>([]);
  selectedStream = signal<StreamItem | null>(null);
  loading = signal(false);
  error = signal<string | null>(null);

  ngOnInit(): void {
    this.loadStreams();
    interval(5000).pipe(takeUntilDestroyed(this.destroyRef)).subscribe(() => this.loadStreams());
  }

  loadStreams(): void {
    this.loading.set(true);
    this.streamApi
      .listStreams()
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: ({ items }) => {
          this.streams.set(items);
          this.syncSelectedStream(items);
          this.loading.set(false);
          this.error.set(null);
        },
        error: () => {
          this.error.set('No se pudo cargar la lista de streams.');
          this.loading.set(false);
        },
      });
  }

  openCreateDialog(): void {
    this.dialog
      .open(CreateStreamDialogComponent, { width: '360px' })
      .afterClosed()
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe((title: string | undefined) => {
        if (!title) return;
        this.streamApi
          .createStream(title)
          .pipe(takeUntilDestroyed(this.destroyRef))
          .subscribe({
            next: (stream) => {
              this.streams.update((list) => [stream, ...list]);
              this.selectedStream.set(stream);
              this.error.set(null);
            },
            error: () => this.error.set('No se pudo crear el stream.'),
          });
      });
  }

  selectStream(stream: StreamItem): void {
    this.selectedStream.set(stream);
  }

  isSelected(stream: StreamItem): boolean {
    return this.selectedStream()?.id === stream.id;
  }

  playbackUrl(stream: StreamItem | null): string | null {
    return this.streamApi.resolvePlaybackUrl(stream?.hls_path);
  }

  copyStreamKey(key: string): void {
    navigator.clipboard.writeText(key);
  }

  statusChipClass(status: StreamItem['status']): string {
    switch (status) {
      case 'live':
        return 'status-ready';
      case 'pending':
        return 'status-pending';
      default:
        return 'status-error';
    }
  }

  statusLabel(status: StreamItem['status']): string {
    switch (status) {
      case 'pending':
        return 'pendiente';
      case 'live':
        return 'en vivo';
      default:
        return 'finalizado';
    }
  }

  private syncSelectedStream(streams: StreamItem[]): void {
    const current = this.selectedStream();
    if (current) {
      const refreshed = streams.find((s) => s.id === current.id);
      if (refreshed) {
        this.selectedStream.set(refreshed);
        return;
      }
    }
    const firstLive = streams.find((s) => s.status === 'live' && s.hls_path) ?? null;
    this.selectedStream.set(firstLive);
  }
}
