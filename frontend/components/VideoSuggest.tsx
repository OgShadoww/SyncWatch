"use client";

import { FormEvent, useEffect, useRef, useState } from "react";

export function VideoSuggest() {
  const dialog = useRef<HTMLDialogElement>(null);
  const pending = useRef<AbortController | null>(null);
  const [description, setDescription] = useState("");
  const [query, setQuery] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => () => pending.current?.abort(), []);

  async function suggest(e: FormEvent) {
    e.preventDefault();
    if (!description.trim() || pending.current) return;
    const controller = new AbortController();
    pending.current = controller;
    setBusy(true);
    setQuery("");
    setError("");
    try {
      const response = await fetch("/api/ai/suggest", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ description: description.trim() }),
        signal: controller.signal,
      });
      if (!response.ok) throw new Error();
      const result = await response.json();
      if (!controller.signal.aborted) setQuery(result.query);
    } catch {
      if (!controller.signal.aborted)
        setError("AI suggestions are unavailable. Try again or paste a YouTube link.");
    } finally {
      if (pending.current === controller) {
        pending.current = null;
        setBusy(false);
      }
    }
  }

  return (
    <>
      <button type="button" className="secondary shrink-0" onClick={() => dialog.current?.showModal()}>
        AI Suggest
      </button>
      <dialog
        ref={dialog}
        aria-labelledby="finder-title"
        className="panel modal"
        onClose={() => {
          pending.current?.abort();
          pending.current = null;
          setBusy(false);
          setError("");
        }}
      >
        <h2 id="finder-title" className="text-xl font-semibold tracking-tight">
          AI Video Finder
        </h2>
        <form onSubmit={suggest} className="mt-5 space-y-4">
          <div>
            <label className="label" htmlFor="description">
              What would you like to watch?
            </label>
            <textarea
              id="description"
              className="field resize-y"
              rows={3}
              maxLength={400}
              required
              autoFocus
              placeholder="A quiet train ride through the Alps"
              disabled={busy}
              value={description}
              onChange={(e) => {
                setDescription(e.target.value);
                setQuery("");
                setError("");
              }}
            />
          </div>
          <div aria-live="polite">
            {error && <p className="feedback">{error}</p>}
            {query && (
              <div className="border-l-2 border-accent pl-3">
                <p className="break-words font-medium">{query}</p>
                <a
                  className="mt-1 inline-block text-sm text-accent underline underline-offset-4"
                  href={"https://www.youtube.com/results?search_query=" + encodeURIComponent(query)}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  Search on YouTube ↗
                </a>
              </div>
            )}
          </div>
          <div className="flex items-center gap-2">
            <button className="primary" disabled={busy || !description.trim()}>
              {busy ? "Finding a query…" : "Suggest a search"}
            </button>
            <button className="secondary" type="button" onClick={() => dialog.current?.close()}>
              Close
            </button>
          </div>
        </form>
      </dialog>
    </>
  );
}
