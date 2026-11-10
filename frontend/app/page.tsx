"use client";

import { FormEvent, useState } from "react";
import { nameKey, roomCode, roomPath } from "../lib/room";

export default function Home() {
  const [mode, setMode] = useState<"create" | "join">("create");
  const [name, setName] = useState("");
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

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
    <main className="mx-auto max-w-md px-5 py-12 sm:py-20">
      <h1 className="text-3xl font-semibold tracking-tight">Room lobby</h1>
      <p className="mt-2 text-sm text-muted">Watch YouTube in sync with your friends.</p>
      <div className="panel mt-7">
        <div className="mb-6 flex gap-1 rounded-lg border border-line bg-base-dark p-1">
          {(["create", "join"] as const).map((option) => (
            <button
              key={option}
              type="button"
              aria-pressed={mode === option}
              disabled={busy}
              className={`flex-1 rounded py-2 text-sm font-medium ${mode === option ? "bg-zinc-700 text-white" : "text-muted"}`}
              onClick={() => {
                setMode(option);
                setError("");
              }}
            >
              {option === "create" ? "Create room" : "Join room"}
            </button>
          ))}
        </div>
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
          <button className="primary w-full" disabled={busy}>
            {busy ? "Opening room…" : mode === "create" ? "Create a room" : "Enter room"}
          </button>
        </form>
      </div>
    </main>
  );
}
