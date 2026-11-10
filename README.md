# SyncWatch

A small watch-party app for YouTube. Create a room, share its link, and watch with synchronized host controls, a participant list, and persistent chat.

## Run locally

```sh
cp .env.example .env
docker compose up --build -d
```

Open [SyncWatch](http://localhost:8080). Create a room with a display name and open its invitation in a private browser window to join as a guest. Paste a YouTube link, then use the host's **Set video**, **Play**, **Pause**, and **Seek** controls. For guests on another device, open the app through the host's reachable network address before copying the link.

Use `docker compose down` to stop. The PostgreSQL volume retains rooms and chat. Set `APP_PORT` in `.env` to change the port.

## Architecture

- The Go/Chi server serves the exported Next.js/TypeScript/Tailwind frontend, a room creation endpoint, and a same-origin WebSocket per participant. The creator receives a room-scoped HttpOnly cookie. PostgreSQL stores its SHA-256 digest.
- Each active room serializes its playback changes and chat writes under a room-local lock. Host commands include the last observed version. A successful database write precedes the broadcast. The final disconnect releases the in-memory room.
- A playback snapshot includes the video, position, duration when known, playing flag, and version. The server projects position from its time anchor. Clients correct drift locally every four seconds and request a fresh snapshot on returning to the tab.
- On join or reconnect, the server sends participants, current playback, and the latest 50 messages. An embedded SQL migration initializes PostgreSQL. Restarting the single Go instance restores playback paused at the last saved control position.

## Development and checks

For Next.js hot reload, start the stack on port 8080 with `DEV_ORIGIN=http://localhost:3000 docker compose up --build -d`, then run `npm ci && npm run dev` in `frontend/`. Open [the development server](http://localhost:3000).

```sh
(cd backend && go test -race ./... && go build ./cmd/server)
(cd frontend && npm ci && npm run typecheck && npm run build)
```

Use Go 1.26.4+ and Node 24 for native development. An externally managed PostgreSQL instance can be supplied with `DATABASE_URL` when running `go run ./cmd/server` from `backend/`.