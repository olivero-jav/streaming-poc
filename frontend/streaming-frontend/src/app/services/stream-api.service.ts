import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';
import { StreamItem, StreamListResponse } from '../models/stream.model';
import { resolvePlaybackUrl as resolvePlaybackUrlUtil } from '../utils/playback-url';
import { API_BASE_URL } from './video-api.service';

@Injectable({ providedIn: 'root' })
export class StreamApiService {
  private readonly http = inject(HttpClient);

  listStreams(): Observable<StreamListResponse> {
    return this.http.get<StreamListResponse>(`${API_BASE_URL}/streams`);
  }

  getStream(id: string): Observable<StreamItem> {
    return this.http.get<StreamItem>(`${API_BASE_URL}/streams/${id}`);
  }

  createStream(title: string): Observable<StreamItem> {
    return this.http.post<StreamItem>(`${API_BASE_URL}/streams`, { title });
  }

  resolvePlaybackUrl(hlsPath?: string): string | null {
    return resolvePlaybackUrlUtil(hlsPath, API_BASE_URL);
  }
}
