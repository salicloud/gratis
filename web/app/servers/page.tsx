import { ServerCard } from "@/components/server-card";
import type { Server } from "@/lib/types";

async function getServers(): Promise<Server[]> {
  try {
    const res = await fetch(
      `${process.env.GRATIS_API_URL ?? "http://localhost:8080"}/api/v1/servers`,
      { next: { revalidate: 10 } }
    );
    if (!res.ok) return [];
    return res.json();
  } catch {
    return [];
  }
}

export default async function ServersPage() {
  const servers = await getServers();

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-xl font-semibold text-slate-100">Servers</h1>
          <p className="text-sm text-slate-400 mt-0.5">
            {servers.length} connected
          </p>
        </div>
      </div>

      {servers.length === 0 ? (
        <div className="rounded-lg border border-surface-border bg-surface-card p-12 text-center">
          <p className="text-slate-400 text-sm">No servers connected.</p>
          <p className="text-slate-500 text-xs mt-1">
            Start the Gratis agent on a server to see it here.
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {servers.map((s) => (
            <ServerCard key={s.server_id} server={s} />
          ))}
        </div>
      )}
    </div>
  );
}
