import type { Server } from "@/lib/types";
import Link from "next/link";

function Gauge({ value, max, label }: { value: number; max: number; label: string }) {
  const pct = max > 0 ? Math.min(100, (value / max) * 100) : 0;
  const color = pct > 85 ? "bg-red-500" : pct > 65 ? "bg-amber-400" : "bg-brand";
  return (
    <div className="space-y-1">
      <div className="flex justify-between text-xs text-slate-400">
        <span>{label}</span>
        <span>{pct.toFixed(0)}%</span>
      </div>
      <div className="h-1.5 rounded-full bg-surface-border overflow-hidden">
        <div className={`h-full rounded-full transition-all ${color}`} style={{ width: `${pct}%` }} />
      </div>
    </div>
  );
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
}

export function ServerCard({ server }: { server: Server }) {
  const m = server.metrics;
  return (
    <Link href={`/servers/${server.server_id}`}>
      <div className="rounded-lg border border-surface-border bg-surface-card p-4 hover:border-brand/40 transition-colors cursor-pointer space-y-4">
        <div className="flex items-start justify-between">
          <div>
            <p className="font-medium text-slate-100 text-sm">{server.hostname}</p>
            <p className="text-xs text-slate-500 mt-0.5 font-mono">{server.server_id}</p>
          </div>
          <span className={`inline-flex items-center gap-1.5 text-xs px-2 py-0.5 rounded-full ${
            server.online
              ? "bg-emerald-500/10 text-emerald-400"
              : "bg-slate-500/10 text-slate-400"
          }`}>
            <span className={`w-1.5 h-1.5 rounded-full ${server.online ? "bg-emerald-400" : "bg-slate-400"}`} />
            {server.online ? "online" : "offline"}
          </span>
        </div>

        {m ? (
          <div className="space-y-2.5">
            <Gauge value={m.cpu_percent} max={100} label="CPU" />
            <Gauge value={m.mem_used} max={m.mem_total} label="Memory" />
            <Gauge value={m.disk_used} max={m.disk_total} label="Disk" />
            <div className="flex justify-between text-xs text-slate-500 pt-0.5">
              <span>Load: {m.load_1.toFixed(2)} / {m.load_5.toFixed(2)} / {m.load_15.toFixed(2)}</span>
              <span>{formatBytes(m.mem_total - m.mem_used)} free</span>
            </div>
          </div>
        ) : (
          <p className="text-xs text-slate-500">Waiting for metrics…</p>
        )}
      </div>
    </Link>
  );
}
