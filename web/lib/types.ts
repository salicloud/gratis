export interface Server {
  server_id: string;
  hostname: string;
  online: boolean;
  last_seen: string;
  metrics?: ServerMetrics;
}

export interface ServerMetrics {
  cpu_percent: number;
  mem_total: number;
  mem_used: number;
  disk_total: number;
  disk_used: number;
  load_1: number;
  load_5: number;
  load_15: number;
}
