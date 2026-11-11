package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/go-chi/chi/v5"

	"syncwatch/internal/playback"
	"syncwatch/internal/room"
)

const hostCookie = "syncwatch_host"
const timeout = 5 * time.Second

type API struct {
	rooms   *room.Manager
	root    context.Context
	sockets sync.WaitGroup
	accept  websocket.AcceptOptions
}

func New(ctx context.Context, rooms *room.Manager, assets, devOrigin string) (http.Handler, func()) {
	api := &API{rooms: rooms, root: ctx}
	if devOrigin != "" {
		api.accept.OriginPatterns = []string{devOrigin}
	}
	r := chi.NewRouter()
	r.Use(http.NewCrossOriginProtection().Handler)
	r.Post("/api/rooms", api.create)
	r.Post("/api/ai/suggest", suggestHandler())
	r.Get("/api/rooms/{id}/ws", api.socket)
	r.Handle("/*", http.FileServer(http.Dir(assets)))
	return r, api.sockets.Wait
}

func (a *API) create(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	id, secret, err := a.rooms.Create(ctx)
	if err != nil {
		slog.Error("create room", "error", err)
		http.Error(w, "Could not create room", http.StatusServiceUnavailable)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: hostCookie, Value: secret, Path: "/api/rooms/" + id + "/",
		HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: 60 * 60 * 24 * 365})
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

type command struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
	Text string `json:"text,omitempty"`
	playback.Command
}

func read(ctx context.Context, conn *websocket.Conn) (command, error) {
	var c command
	kind, data, err := conn.Read(ctx)
	if err != nil {
		return c, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if kind != websocket.MessageText || !utf8.Valid(data) || dec.Decode(&c) != nil || dec.Decode(new(any)) != io.EOF {
		conn.Close(websocket.StatusInvalidFramePayloadData, "Expected a JSON text command")
		return c, errors.New("invalid payload")
	}
	return c, nil
}

func (a *API) socket(w http.ResponseWriter, req *http.Request) {
	a.sockets.Add(1)
	defer a.sockets.Done()
	conn, err := websocket.Accept(w, req, &a.accept)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(4096)
	ctx, cancel := context.WithCancel(a.root)
	defer cancel()
	joinCtx, finish := context.WithTimeout(ctx, 10*time.Second)
	defer finish()
	c, err := read(joinCtx, conn)
	if err != nil {
		return
	}
	if c.Type != "join" {
		conn.Close(4400, "Join with a display name first")
		return
	}
	var secret string
	if cookie, err := req.Cookie(hostCookie); err == nil {
		secret = cookie.Value
	}
	s, err := a.rooms.Join(joinCtx, chi.URLParam(req, "id"), c.Name, secret, cancel)
	finish()
	if err != nil {
		switch {
		case errors.Is(err, room.ErrNotFound):
			conn.Close(4404, "Room not found")
		case errors.Is(err, room.ErrName):
			conn.Close(4400, err.Error())
		default:
			slog.Error("join room", "error", err)
			conn.Close(websocket.StatusTryAgainLater, "Room temporarily unavailable")
		}
		return
	}
	defer s.Leave()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer cancel()
		writeLoop(ctx, conn, s.Events())
	}()
	defer func() { cancel(); <-done }()
	for {
		c, err := read(ctx, conn)
		if err != nil {
			return
		}
		actionCtx, end := context.WithTimeout(ctx, timeout)
		switch c.Type {
		case "sync":
			s.Sync()
		case "chat":
			err = s.Chat(actionCtx, c.Text)
		case "playback":
			err = s.Playback(actionCtx, c.Command)
		default:
			err = playback.ErrInvalid
		}
		end()
		if err == nil {
			continue
		}
		msg := "Could not save the change. Try again."
		if errors.Is(err, room.ErrForbidden) || errors.Is(err, room.ErrChat) ||
			errors.Is(err, playback.ErrInvalid) || errors.Is(err, playback.ErrStale) {
			msg = err.Error()
		} else {
			slog.Error("room action", "error", err)
		}
		if write(ctx, conn, room.Event{Type: "error", Error: msg}) != nil {
			return
		}
	}
}

func write(ctx context.Context, conn *websocket.Conn, event room.Event) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return wsjson.Write(ctx, conn, event)
}

func writeLoop(ctx context.Context, conn *websocket.Conn, events <-chan room.Event) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-events:
			if write(ctx, conn, event) != nil {
				return
			}
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, timeout)
			err := conn.Ping(pingCtx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}
