"use client";

import { FormEvent, useRef, useState } from "react";
import { nameKey, roomCode, roomPath } from "../lib/room";

export default function Home() {
  const dialog = useRef<HTMLDialogElement>(null);
  const [mode, setMode] = useState<"create" | "join">("create");
  const [name, setName] = useState("");
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  function openRoom(action: "create" | "join") {
    setMode(action);
    setError("");
    dialog.current?.showModal();
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (busy) return;
    setError("");
    if (!name.trim()) {
      setError("Enter your display name.");
      return;
    }
    let id = roomCode(code);
    if (mode === "join" && !id) {
      setError("Enter a 10-character room code or an invitation link.");
      return;
    }
    setBusy(true);
    try {
      if (mode === "create") {
        const res = await fetch("/api/rooms", { method: "POST" });
        if (!res.ok) throw new Error();
        id = (await res.json()).id;
      }
      if (!id) throw new Error();
      sessionStorage.setItem(nameKey(id), name.trim());
      window.location.assign(roomPath(id));
    } catch {
      setError("Could not open the room. Please try again.");
      setBusy(false);
    }
  }

  return (
    <div className="mx-auto flex min-h-[calc(100svh-4rem)] max-w-6xl flex-col px-5 sm:px-8">
      <main className="flex-1">
        <section className="py-16 sm:py-24">
          <h1 className="text-5xl font-semibold leading-[1.03] tracking-[-0.05em] sm:text-7xl">
            Watch together, <span className="block text-muted">anywhere.</span>
          </h1>
          <p className="mt-6 max-w-lg text-lg leading-relaxed text-muted">
            Share a room to watch YouTube in sync with friends.
          </p>
          <div className="mt-8 flex flex-wrap gap-3">
            <button className="primary px-5" onClick={() => openRoom("create")}>
              Create Room
            </button>
            <button className="secondary px-5" onClick={() => openRoom("join")}>
              Join Room
            </button>
          </div>
        </section>
        <section
          aria-label="Room features"
          className="grid gap-6 border-t border-line py-8 sm:grid-cols-3 sm:gap-8 sm:py-10"
        >
          {[
            ["Synchronized playback", "Play, pause, or seek for the whole room."],
            ["Real-time chat", "Share reactions with friends as you watch."],
            ["AI Video Finder", "Describe a video and get a YouTube search to explore."],
          ].map(([title, detail]) => (
            <div key={title} className="sm:border-l sm:border-line sm:pl-6 sm:first:border-0 sm:first:pl-0">
              <h2 className="font-semibold">{title}</h2>
              <p className="mt-1.5 max-w-xs text-sm leading-relaxed text-muted">{detail}</p>
            </div>
          ))}
        </section>
      </main>
      <footer className="border-t border-line py-5 text-sm text-muted">
        <a className="hover:text-zinc-100" href="https://github.com/OgShadoww/SyncWatch">
          GitHub ↗
        </a>
      </footer>
      <dialog
        ref={dialog}
        className="panel modal"
        aria-labelledby="room-action"
        onCancel={(e) => {
          if (busy) e.preventDefault();
        }}
      >
        <h2 id="room-action" className="mb-5 text-xl font-semibold tracking-tight">
          {mode === "create" ? "Create a room" : "Join a room"}
        </h2>
        <form className="space-y-5" onSubmit={submit}>
          <div>
            <label className="label" htmlFor="name">
              Display name
            </label>
            <input
              className="field"
              id="name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Your name"
              autoComplete="nickname"
              autoFocus
              maxLength={32}
              required
            />
          </div>
          {mode === "join" && (
            <div>
              <label className="label" htmlFor="code">
                Room code or link
              </label>
              <input
                className="field"
                id="code"
                value={code}
                onChange={(e) => setCode(e.target.value)}
                placeholder="Paste an invitation"
                required
              />
            </div>
          )}
          {error && (
            <p role="alert" className="feedback">
              {error}
            </p>
          )}
          <div className="flex gap-2">
            <button className="primary" disabled={busy}>
              {busy ? "Opening room…" : mode === "create" ? "Create Room" : "Join Room"}
            </button>
            <button
              type="button"
              className="secondary"
              disabled={busy}
              onClick={() => dialog.current?.close()}
            >
              Cancel
            </button>
          </div>
        </form>
      </dialog>
    </div>
  );
}
