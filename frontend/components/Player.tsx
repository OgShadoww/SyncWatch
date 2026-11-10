"use client";

import { useEffect, useRef, useState } from "react";
import { Clock, position } from "../lib/room";

declare global {
  interface Window {
    onYouTubeIframeAPIReady?: () => void;
  }
}

let api: Promise<void> | undefined;
function loadAPI() {
  if (window.YT?.Player) return Promise.resolve();
  if (api) return api;
  api = new Promise<void>((resolve, reject) => {
    const script = document.createElement("script");
    function fail() {
      clearTimeout(timer);
      script.remove();
      api = undefined;
      reject(new Error("Could not load YouTube."));
    }
    const timer = window.setTimeout(fail, 15000);
    window.onYouTubeIframeAPIReady = () => {
      clearTimeout(timer);
      resolve();
    };
    script.onerror = fail;
    script.src = "https://www.youtube.com/iframe_api";
    document.head.append(script);
  });
  return api;
}

export function Player({
  clock,
  onDuration,
}: {
  clock: Clock;
  onDuration: (id: string, duration: number) => void;
}) {
  const mount = useRef<HTMLDivElement>(null);
  const current = useRef(clock);
  const sync = useRef<(force: boolean) => void>(() => {});
  const [notice, setNotice] = useState("");
  const [blocked, setBlocked] = useState(false);
  const [retry, setRetry] = useState(0);
  const { videoId } = clock;

  useEffect(() => {
    current.current = clock;
    sync.current(true);
    if (!clock.playing) setBlocked(false);
  }, [clock]);

  useEffect(() => {
    let disposed = false;
    let player: YT.Player | undefined;
    let ready = false;
    let autoplayBlocked = false;
    let failed = false;
    setNotice("");
    setBlocked(false);
    function reconcile(force: boolean) {
      if (!ready || !player || failed || (autoplayBlocked && !force)) return;
      const state = current.current;
      const target = position(state);
      const mode = player.getPlayerState();
      const playing = state.playing && (!state.duration || target < state.duration);
      if (playing) {
        if (Math.abs(player.getCurrentTime() - target) > (force ? 0.35 : 1.5)) player.seekTo(target, true);
        if (mode !== YT.PlayerState.PLAYING && mode !== YT.PlayerState.BUFFERING) player.playVideo();
      } else if (mode === YT.PlayerState.CUED || mode === YT.PlayerState.UNSTARTED) {
        if (force) player.cueVideoById({ videoId, startSeconds: target });
      } else {
        if (mode === YT.PlayerState.PLAYING || mode === YT.PlayerState.BUFFERING) player.pauseVideo();
        if (Math.abs(player.getCurrentTime() - target) > (force ? 0.35 : 1.5)) player.seekTo(target, true);
      }
    }
    sync.current = reconcile;
    const readyTimer = window.setTimeout(() => {
      if (!ready) setNotice("YouTube is taking too long to respond.");
    }, 18000);
    loadAPI()
      .then(() => {
        if (disposed || !mount.current) return;
        const element = document.createElement("div");
        mount.current.replaceChildren(element);
        player = new YT.Player(element, {
          width: "100%",
          height: "100%",
          videoId,
          playerVars: { controls: 0, disablekb: 1, playsinline: 1, origin: location.origin },
          events: {
            onReady: (e) => {
              if (disposed) return;
              ready = true;
              clearTimeout(readyTimer);
              setNotice("");
              e.target.getIframe().title = "Shared YouTube player";
              onDuration(videoId, e.target.getDuration());
              reconcile(true);
            },
            onStateChange: (e) => {
              if (disposed || !ready) return;
              if (e.data === YT.PlayerState.PLAYING) {
                autoplayBlocked = false;
                setBlocked(false);
                onDuration(videoId, e.target.getDuration());
              }
            },
            onAutoplayBlocked: () => {
              if (disposed) return;
              autoplayBlocked = true;
              setBlocked(true);
            },
            onError: () => {
              if (disposed) return;
              failed = true;
              setNotice("YouTube could not play this video. Check its availability and embedding settings.");
            },
          },
        });
      })
      .catch(() => {
        if (!disposed) {
          clearTimeout(readyTimer);
          setNotice("Could not load the YouTube player.");
        }
      });
    const interval = window.setInterval(() => reconcile(false), 4000);
    return () => {
      disposed = true;
      clearInterval(interval);
      clearTimeout(readyTimer);
      sync.current = () => {};
      player?.destroy();
    };
  }, [videoId, retry, onDuration]);

  return (
    <>
      <div className="aspect-video min-h-[200px] w-full bg-black" ref={mount} />
      {blocked && !notice && (
        <button className="secondary m-3" onClick={() => sync.current(true)}>
          Enable playback on this device
        </button>
      )}
      {notice && (
        <div role="status" className="flex items-center justify-between gap-3 p-3 text-sm text-muted">
          {notice}
          <button className="secondary" onClick={() => setRetry((n) => n + 1)}>
            Retry
          </button>
        </div>
      )}
    </>
  );
}
