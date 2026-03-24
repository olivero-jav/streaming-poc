import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';
import { VideoItem, VideoListResponse } from '../models/video.model';

export const API_BASE_URL = 'http://localhost:8080';

@Injectable({ providedIn: 'root' })
export class VideoApiService {
  private readonly http = inject(HttpClient);

  listVideos(): Observable<VideoListResponse> {
    return this.http.get<VideoListResponse>(`${API_BASE_URL}/videos`);
  }

  getVideo(id: string): Observable<VideoItem> {
    return this.http.get<VideoItem>(`${API_BASE_URL}/videos/${id}`);
  }

  uploadVideo(payload: { title: string; description?: string; file: File }): Observable<VideoItem> {
    const formData = new FormData();
    formData.append('title', payload.title);
    if (payload.description) {
      formData.append('description', payload.description);
    }
    formData.append('file', payload.file);

    return this.http.post<VideoItem>(`${API_BASE_URL}/videos`, formData);
  }

  resolvePlaybackUrl(hlsPath?: string): string | null {
    if (!hlsPath) {
      return null;
    }
    if (hlsPath.startsWith('http://') || hlsPath.startsWith('https://')) {
      return hlsPath;
    }
    return `${API_BASE_URL}${hlsPath.startsWith('/') ? '' : '/'}${hlsPath}`;
  }
}
