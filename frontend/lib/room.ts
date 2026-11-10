export type Person = { id: number; name: string; host: boolean };
export type Message = { id: number; name: string; text: string; sentAt: string };
export type Playback = {
  videoId: string;
  position: number;
  duration: number;
  playing: boolean;
  version: number;
};
export type Clock = Playback & { received: number };
export type Control =
  | { action: "video"; videoId: string }
  | { action: "play"; duration: number }
  | { action: "pause" }
  | { action: "seek"; position: number };
export type ClientEvent = { type: "chat"; text: string } | ({ type: "playback"; version: number } & Control);
export type ServerEvent =
  | { type: "snapshot"; state: Playback; self: Person; people: Person[]; history?: Message[] }
  | { type: "state"; state: Playback }
  | { type: "people"; people: Person[] }
  | { type: "chat"; message: Message }
  | { type: "error"; error: string };

export const nameKey = (id: string) => "syncwatch.name." + id;
export const roomPath = (id: string) => "/room/?id=" + encodeURIComponent(id);

export function roomCode(value: string): string | null {
  let code = value.trim();
  if (!/^[a-f0-9]{10}$/i.test(code)) {
    try {
      code = new URL(code).searchParams.get("id") || "";
    } catch {
      return null;
    }
  }
  return /^[a-f0-9]{10}$/i.test(code) ? code.toUpperCase() : null;
}

export function videoID(input: string): string | null {
  let id = input.trim();
  if (!/^[a-zA-Z0-9_-]{11}$/.test(id)) {
    try {
      const url = new URL(id.includes("://") ? id : "https://" + id);
      const host = url.hostname.toLowerCase().replace(/^www\./, "");
      const [, path, video] = url.pathname.split("/");
      if (host === "youtu.be") id = path;
      else if (host === "youtube.com" || host === "m.youtube.com" || host === "youtube-nocookie.com") {
        id =
          path === "watch"
            ? url.searchParams.get("v") || ""
            : ["embed", "shorts", "live"].includes(path)
              ? video || ""
              : "";
      } else return null;
    } catch {
      return null;
    }
  }
  return /^[a-zA-Z0-9_-]{11}$/.test(id) ? id : null;
}

export function position(s: Clock) {
  const elapsed = s.playing ? Math.max(0, performance.now() - s.received) / 1000 : 0;
  return Math.min(s.duration || Infinity, s.position + elapsed);
}

export function timeLabel(seconds: number) {
  const n = Math.floor(seconds);
  return Math.floor(n / 60) + ":" + String(n % 60).padStart(2, "0");
}
