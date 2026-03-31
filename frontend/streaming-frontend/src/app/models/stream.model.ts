export type StreamStatus = 'pending' | 'live' | 'ended';

export interface StreamItem {
  id: string;
  title: string;
  stream_key: string;
  status: StreamStatus;
  hls_path?: string;
  started_at?: string;
  ended_at?: string;
  created_at: string;
  updated_at: string;
}

export interface StreamListResponse {
  items: StreamItem[];
}
