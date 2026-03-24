export type VideoStatus = 'pending' | 'processing' | 'ready' | 'error';

export interface VideoItem {
  id: string;
  title: string;
  description?: string;
  status: VideoStatus;
  source_path?: string;
  hls_path?: string;
  duration_seconds: number;
  created_at: string;
  updated_at: string;
}

export interface VideoListResponse {
  items: VideoItem[];
}
