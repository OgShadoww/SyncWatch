import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "SyncWatch",
  description: "Watch YouTube together in a shared room.",
};

export default function Layout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>
        <header className="border-b border-line">
          <div className="mx-auto flex h-16 max-w-6xl items-center px-5 sm:px-8">
            <a
              href="/"
              aria-label="SyncWatch home"
              className="inline-flex items-center gap-2 text-xl font-bold tracking-tight"
            >
              <span
                aria-hidden="true"
                className="grid size-7 place-items-center rounded-md bg-accent text-sm text-base-dark"
              >
                ▶
              </span>
              SyncWatch
            </a>
          </div>
        </header>
        {children}
      </body>
    </html>
  );
}
