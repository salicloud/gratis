import type { Server } from "./types";

const BASE = "/api/v1";

async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(BASE + path, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`${res.status}: ${text}`);
  }
  return res.json() as Promise<T>;
}

export const api = {
  servers: {
    list: () => apiFetch<Server[]>("/servers"),
    get: (id: string) => apiFetch<Server>(`/servers/${id}`),
    createVhost: (id: string, body: {
      domain: string;
      username?: string;
      docroot?: string;
      php_version?: string;
      aliases?: string[];
    }) => apiFetch(`/servers/${id}/vhosts`, { method: "POST", body: JSON.stringify(body) }),
    deleteVhost: (id: string, domain: string) =>
      apiFetch(`/servers/${id}/vhosts/${domain}`, { method: "DELETE" }),
  },
};
