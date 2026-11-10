"use client";

import { useEffect, useState } from "react";
import { nameKey, roomCode } from "../../lib/room";
import { WatchRoom } from "../../components/WatchRoom";

export default function RoomPage() {
  const [id, setID] = useState<string | null>();
  const [name, setName] = useState("");
  const [draft, setDraft] = useState("");

  useEffect(() => {
    const id = roomCode(new URLSearchParams(location.search).get("id") || "");
    setID(id);
    if (id) setName(sessionStorage.getItem(nameKey(id)) || "");
  }, []);

  if (id === undefined) return <main className="mx-auto max-w-md p-8 text-muted">Opening room…</main>;
  if (id && name) return <WatchRoom id={id} name={name} />;

  return (
    <main className="mx-auto max-w-md px-5 py-12 sm:py-20">
      <h1 className="text-3xl font-semibold tracking-tight">{id ? "Join room" : "Invalid invitation"}</h1>
      {id ? (
        <>
          <p className="mt-2 font-mono text-sm text-muted">{id}</p>
          <form
            className="panel mt-7 space-y-5"
            onSubmit={(e) => {
              e.preventDefault();
              if (draft.trim()) {
                sessionStorage.setItem(nameKey(id), draft.trim());
                setName(draft.trim());
              }
            }}
          >
            <div>
              <label className="label" htmlFor="name">
                Display name
              </label>
              <input
                className="field"
                id="name"
                placeholder="Your name"
                maxLength={32}
                required
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                autoComplete="nickname"
              />
            </div>
            <button className="primary w-full" disabled={!draft.trim()}>
              Enter room
            </button>
          </form>
        </>
      ) : (
        <a className="secondary mt-5 inline-block" href="/">
          Back to lobby
        </a>
      )}
    </main>
  );
}
