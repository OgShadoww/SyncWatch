"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { ClientEvent, Clock, Message, Person, ServerEvent } from "./room";

export function useRoom(id: string, name: string) {
  const socket = useRef<WebSocket | null>(null);
  const [status, setStatus] = useState("Connecting");
  const [error, setError] = useState("");
  const [self, setSelf] = useState<Person>();
  const [people, setPeople] = useState<Person[]>([]);
  const [messages, setMessages] = useState<Message[]>([]);
  const [clock, setClock] = useState<Clock>();

  useEffect(() => {
    let disposed = false;
    let terminal = false;
    let attempts = 0;
    let timer: ReturnType<typeof setTimeout>;
    function connect() {
      if (disposed || terminal) return;
      const url = new URL("/api/rooms/" + id + "/ws", window.location.href);
      url.protocol = location.protocol === "https:" ? "wss:" : "ws:";
      const ws = new WebSocket(url);
      socket.current = ws;
      ws.onopen = () => ws.send(JSON.stringify({ type: "join", name }));
      ws.onmessage = (e) => {
        if (disposed || socket.current !== ws) return;
        const msg: ServerEvent = JSON.parse(e.data);
        if (msg.type === "state" || msg.type === "snapshot") {
          const next = { ...msg.state, received: performance.now() };
          setClock((prev) =>
            msg.type === "snapshot" || !prev || prev.version <= next.version ? next : prev,
          );
        }
        switch (msg.type) {
          case "snapshot":
            attempts = 0;
            setSelf(msg.self);
            setPeople(msg.people);
            setMessages(msg.history || []);
            setStatus("Connected");
            setError("");
            break;
          case "people":
            setPeople(msg.people);
            break;
          case "chat":
            setMessages((prev) => [...prev.slice(-49), msg.message]);
            break;
          case "error":
            setError(msg.error);
            break;
        }
      };
      ws.onclose = (event) => {
        if (disposed || socket.current !== ws) return;
        socket.current = null;
        if ([4400, 4404].includes(event.code)) {
          terminal = true;
          setStatus("Unable to join");
          setError(event.reason);
          return;
        }
        setStatus("Reconnecting");
        timer = setTimeout(connect, Math.min(10000, 500 * 2 ** Math.min(attempts++, 5)));
      };
    }
    function resume() {
      if (document.hidden) return;
      if (socket.current?.readyState === WebSocket.OPEN)
        socket.current.send(JSON.stringify({ type: "sync" }));
      else if (!socket.current) {
        clearTimeout(timer);
        connect();
      }
    }
    connect();
    document.addEventListener("visibilitychange", resume);
    window.addEventListener("online", resume);
    return () => {
      disposed = true;
      clearTimeout(timer);
      document.removeEventListener("visibilitychange", resume);
      window.removeEventListener("online", resume);
      socket.current?.close();
      socket.current = null;
    };
  }, [id, name]);

  const send = useCallback((event: ClientEvent) => {
    if (socket.current?.readyState !== WebSocket.OPEN) {
      setError("Reconnecting. Try again in a moment.");
      return false;
    }
    socket.current.send(JSON.stringify(event));
    setError("");
    return true;
  }, []);

  return { status, error, self, people, messages, clock, send };
}
