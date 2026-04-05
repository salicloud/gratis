"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

const nav = [
  { label: "Servers",  href: "/servers",  icon: "⬡" },
  { label: "Domains",  href: "/domains",  icon: "◈" },
  { label: "Databases",href: "/databases",icon: "◫" },
  { label: "Email",    href: "/email",    icon: "◻" },
  { label: "DNS",      href: "/dns",      icon: "◌" },
];

export function Sidebar() {
  const path = usePathname();

  return (
    <aside className="w-56 shrink-0 flex flex-col border-r border-surface-border bg-surface-card">
      <div className="px-5 py-5 border-b border-surface-border">
        <span className="text-brand font-bold text-lg tracking-tight">gratis</span>
        <span className="text-slate-500 text-xs ml-2">by sali.cloud</span>
      </div>

      <nav className="flex-1 px-3 py-4 space-y-0.5">
        {nav.map(({ label, href, icon }) => {
          const active = path.startsWith(href);
          return (
            <Link
              key={href}
              href={href}
              className={`flex items-center gap-3 px-3 py-2 rounded-md text-sm transition-colors ${
                active
                  ? "bg-brand/10 text-brand font-medium"
                  : "text-slate-400 hover:text-slate-100 hover:bg-surface-border/50"
              }`}
            >
              <span className="text-base leading-none">{icon}</span>
              {label}
            </Link>
          );
        })}
      </nav>

      <div className="px-5 py-4 border-t border-surface-border">
        <p className="text-xs text-slate-500">Gratis v0.1.0</p>
      </div>
    </aside>
  );
}
