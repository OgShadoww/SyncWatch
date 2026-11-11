"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Clock, Control, position, roomPath, timeLabel, videoID } from "../lib/room";
import { useRoom } from "../lib/useRoom";
import { Player } from "./Player";
import { VideoSuggest } from "./VideoSuggest";

function Transport({
  clock,
  host,
  online,
  duration,
  control,
}: {
  clock: Clock;
  host: boolean;
  online: boolean;
  duration: number;
  control: (c: Control) => void;
}) {
  const [now, setNow] = useState(clock.position);
  const [seek, setSeek] = useState("");
  useEffect(() => {
    setNow(position(clock));
    if (!clock.playing) return;
    const timer = window.setInterval(() => setNow(position(clock)), 500);
    return () => clearInterval(timer);
  }, [clock]);
  const playing = clock.playing && (!clock.duration || now < clock.duration);
  return (
    <div className="flex flex-wrap items-center gap-3 border-t border-line p-4">
      {host ? (
        <button
          className="primary"
          disabled={!online}
          onClick={() => control(playing ? { action: "pause" } : { action: "play", duration })}
        >
          {playing ? "Pause" : "Play"}
        </button>
      ) : (
        <span className="text-sm text-muted">{playing ? "Playing" : "Paused"}</span>
      )}
      <span className="font-mono text-sm tabular-nums">
        {timeLabel(now)}
        {duration > 0 && <span className="text-muted"> / {timeLabel(duration)}</span>}
      </span>
      {host && (
        <form
          className="ml-auto flex items-center gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            if (seek !== "") {
              control({ action: "seek", position: Number(seek) });
              setSeek("");
            }
          }}
        >
          <input
            aria-label="Seek position in seconds"
            className="field w-24"
            type="number"
            min="0"
            max={duration || 604800}
            step="any"
            value={seek}
            onChange={(e) => setSeek(e.target.value)}
            placeholder="Seconds"
            required
          />
          <button className="secondary" disabled={!online}>
            Seek
          </button>
        </form>
      )}
    </div>
  );
}

export function WatchRoom({ id, name }: { id: string; name: string }) {
  const { status, self, people, messages, clock, send, error } = useRoom(id, name);
  const [video, setVideo] = useState("");
  const [videoError, setVideoError] = useState("");
  const [text, setText] = useState("");
  const [share, setShare] = useState("");
  const [meta, setMeta] = useState({ id: "", duration: 0 });
  const chat = useRef<HTMLDivElement>(null);
  const atBottom = useRef(true);
  const online = status === "Connected";
  const host = self?.host === true;
  const duration = clock?.duration || (meta.id === clock?.videoId ? meta.duration : 0);
  const onDuration = useCallback((id: string, duration: number) => {
    if (Number.isFinite(duration) && duration > 0)
      setMeta((prev) => (prev.id === id && prev.duration === duration ? prev : { id, duration }));
  }, []);
  // Metadata can arrive only after the first play. Commit the length once it is known.
  useEffect(() => {
    if (online && host && clock?.playing && !clock.duration && duration) {
      send({ type: "playback", action: "play", version: clock.version, duration });
    }
  }, [online, host, clock, duration, send]);
  useEffect(() => {
    if (chat.current && atBottom.current) chat.current.scrollTop = chat.current.scrollHeight;
  }, [messages]);
  function control(c: Control) {
    if (clock && online) send({ type: "playback", version: clock.version, ...c });
  }
  const link = typeof window === "undefined" ? "" : new URL(roomPath(id), location.origin).href;
  if (status === "Unable to join")
    return (
      <main className="mx-auto max-w-md px-5 py-12">
        <h1 className="text-2xl font-semibold">{error}</h1>
        <a className="secondary mt-5 inline-block" href="/">
          Back to lobby
        </a>
      </main>
    );

  return (
    <main className="mx-auto max-w-6xl px-5 py-7 sm:px-8 sm:py-10">
      <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Watch room</h1>
          <p className="mt-1 font-mono text-sm text-muted">Room {id}</p>
        </div>
        <div className="flex items-center gap-4">
          <span role="status" className="flex items-center gap-2 text-sm text-muted">
            <span className={`size-2 rounded-full ${online ? "bg-emerald-400" : "bg-amber-300"}`} />
            {status}
          </span>
          <button
            className="secondary"
            onClick={async () => {
              try {
                await navigator.clipboard.writeText(link);
                setShare("Link copied");
              } catch {
                setShare(link);
              }
            }}
          >
            Copy room link
          </button>
        </div>
      </div>
      {share && (
        <p role="status" className="mb-4 break-all text-sm text-muted">
          {share}
        </p>
      )}
      {error && (
        <p role="alert" className="feedback mb-4">
          {error}
        </p>
      )}
      <div className="grid items-start gap-5 lg:grid-cols-[minmax(0,2.2fr)_minmax(17rem,1fr)]">
        <section
          aria-label="Shared video"
          className="min-w-0 overflow-hidden rounded-xl border border-line bg-panel"
        >
          {clock?.videoId ? (
            <>
              <Player key={clock.videoId} clock={clock} onDuration={onDuration} />
              <Transport clock={clock} host={host} online={online} duration={duration} control={control} />
            </>
          ) : (
            <div className="grid aspect-video min-h-[200px] place-content-center gap-2 bg-black/40 p-5 text-center">
              <h2 className="font-semibold">{online ? "No video selected" : "Connecting to room…"}</h2>
              {online && (
                <p className="text-sm text-muted">
                  {host
                    ? "Add a YouTube video to start watching."
                    : "Waiting for the host to choose a video."}
                </p>
              )}
            </div>
          )}
          {host && (
            <div className="border-t border-line p-4 sm:p-5">
              <div className="mb-2 flex items-center justify-between gap-2">
                <label className="text-sm font-medium" htmlFor="video">
                  YouTube link or video ID
                </label>
                <VideoSuggest />
              </div>
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  const id = videoID(video);
                  if (!id) {
                    setVideoError("Enter a valid YouTube link or video ID.");
                    return;
                  }
                  control({ action: "video", videoId: id });
                  setVideo("");
                  setVideoError("");
                }}
              >
                <div className="flex gap-2">
                  <input
                    id="video"
                    className="field"
                    value={video}
                    onChange={(e) => setVideo(e.target.value)}
                    placeholder="Paste a YouTube link"
                    required
                  />
                  <button className="secondary shrink-0" disabled={!online}>
                    Set video
                  </button>
                </div>
                {videoError && (
                  <p className="feedback mt-2" role="alert">
                    {videoError}
                  </p>
                )}
              </form>
            </div>
          )}
        </section>
        <aside className="grid min-w-0 gap-5">
          <section className="panel">
            <h2 className="font-semibold">
              Participants <span className="ml-1 text-sm text-muted">{online ? people.length : "—"}</span>
            </h2>
            <ul className="mt-3 space-y-2">
              {people.map((p) => (
                <li key={p.id} className="flex items-baseline justify-between gap-3 text-sm">
                  <span className="break-all">
                    {p.name}
                    {p.id === self?.id && <span className="text-muted"> (you)</span>}
                  </span>
                  {p.host && <span className="text-xs font-medium text-accent">Host</span>}
                </li>
              ))}
            </ul>
          </section>
          <section className="panel">
            <h2 className="font-semibold">Room chat</h2>
            <div
              ref={chat}
              role="log"
              aria-label="Chat messages"
              className="my-3 h-56 overflow-y-auto border-y border-line py-3"
              onScroll={(e) => {
                const el = e.currentTarget;
                atBottom.current = el.scrollHeight - el.clientHeight - el.scrollTop < 32;
              }}
            >
              {messages.length === 0 && <p className="text-sm text-muted">No messages yet.</p>}
              {messages.map((m) => (
                <div key={m.id} className="mb-3 break-words text-sm">
                  <span className="font-semibold">{m.name}</span>
                  <time dateTime={m.sentAt} className="ml-2 text-xs text-muted">
                    {new Date(m.sentAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}
                  </time>
                  <p className="mt-0.5">{m.text}</p>
                </div>
              ))}
            </div>
            <form
              className="flex gap-2"
              onSubmit={(e) => {
                e.preventDefault();
                if (text.trim() && send({ type: "chat", text: text.trim() })) {
                  atBottom.current = true;
                  setText("");
                }
              }}
            >
              <input
                className="field"
                aria-label="Chat message"
                placeholder="Message the room"
                maxLength={500}
                required
                value={text}
                onChange={(e) => setText(e.target.value)}
              />
              <button className="secondary" disabled={!online || !text.trim()}>
                Send
              </button>
            </form>
          </section>
        </aside>
      </div>
    </main>
  );
}
