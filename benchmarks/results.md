# SyncWatch backend benchmark

Measured 10 November 2026, 20:48:37–20:51:43 UTC. One local trial, one room (`F0B51CA4D7`), gradually increased to 200 simultaneous WebSocket clients.

## Playback propagation

Clients includes one authenticated host. Every target level reached its stated peak connected count. Each row measures 60 sequential host commands and every guest's corresponding versioned state event.

| Clients | Delivery | p50 | p95 | p99 | Errors |
| ---: | ---: | ---: | ---: | ---: | ---: |
| 25 | 100% (1,440/1,440) | 3.98 ms | 6.38 ms | 8.06 ms | 0 |
| 50 | 100% (2,940/2,940) | 4.55 ms | 5.81 ms | 7.48 ms | 0 |
| 100 | 100% (5,940/5,940) | 5.28 ms | 6.77 ms | 7.66 ms | 0 |
| 200 | 100% (11,940/11,940) | 6.74 ms | 9.60 ms | 11.08 ms | 0 |

Every level recorded zero failed requests, application errors, client transport/protocol errors, unexpected disconnections, and timeouts.

## Chat and resources

Chat ran separately after playback, with round-robin senders, unique short messages, and a maximum offered rate of 10 messages/second. A sender's matching server broadcast confirms the PostgreSQL commit. Every recipient was also checked for delivery.

| Clients | Committed | Measured duration | Messages/second | Go RSS after level |
| ---: | ---: | ---: | ---: | ---: |
| 25 | 100/100 | 10.104 s | 9.897 | 21.85 MiB |
| 50 | 100/100 | 10.097 s | 9.904 | 21.51 MiB |
| 100 | 100/100 | 10.099 s | 9.902 | 25.77 MiB |
| 200 | 100/100 | 10.099 s | 9.902 | 31.84 MiB |

All 37,500 measured chat deliveries arrived. A direct SQL check found 420 stored messages, including 20 warm-up messages, and playback version 268.

During the complete 200-client level, 16 periodic Docker snapshots reported mean app-container CPU of 4.99% and peak 17.20%, where 100% represents one CPU core. These approximate samples cover ramp-up, warm-up, playback, and chat. RSS comes from `/proc/1/status` after each level. PostgreSQL runs in its own container.

## Environment

- Apple M4, 10 physical/logical CPU cores, 16 GiB RAM, macOS 26.5.1, ARM64.
- Docker Desktop 4.65.0, Engine 29.2.1, Compose 5.1.0. Linux VM 6.12.76-linuxkit, 10 CPUs, 7.65 GiB RAM. Compose uses default container CPU/memory limits.
- Go server compiled with Go 1.26.11 for linux/arm64, CGO disabled. Native load generator compiled with Go 1.26.4 for darwin/arm64, without race instrumentation.
- PostgreSQL 17.11, named local volume, `fsync=on`, `synchronous_commit=on`. The existing warm application and database volume were used.
- Base revision `5e47723baf0567cc36105f1b1bb0ee9fbc140fa9` with the existing AI/homepage working-tree changes. Server binary SHA-256 `5e6502af1d12d53f345b9cb647e3d9a8beac31febefafcbb5beea0e577855a52`.

## Method

The tool creates one room through `POST /api/rooms`, retains the server-issued host cookie, and connects guests without that cookie. Each connection must receive its initial snapshot. Clients remain connected as the room grows through 25, 50, 100 and 200 clients, with 50 ms between additions.

Each level selects a video, warms up with six playback commands, then measures 60 alternating play/seek/pause commands at a maximum of two commands/second. Commands use the last acknowledged version. The collector waits for the host and every guest before proceeding.

Latency uses monotonic Go timestamps immediately before the host write and immediately after each guest decodes the matching event. It includes serialization, local networking, server processing, PostgreSQL playback-state persistence, and guest decoding. Startup, room creation, and warm-up are excluded. Percentiles use nearest rank across all guest-event samples, which share commands and are correlated.

Chat has five warm-up messages per level followed by 100 measured messages. Throughput divides acknowledged commits by the full paced measurement window, including the final interval. Every request/broadcast has a 10-second deadline. A failed level ends the run and emits partial results with a nonzero exit status. Intentional teardown is excluded from disconnects.

These measurements establish delivery at the specified rates and duration on a shared local machine. Higher concurrency, sustained saturation throughput, WAN latency, and browser/media performance require further measurements.

## Reproduce

From the repository root, with Docker running:

```sh
docker compose up --build -d
(cd backend && go build -o /tmp/syncwatch-loadtest ./cmd/loadtest)
set -o pipefail
/tmp/syncwatch-loadtest \
  -url http://localhost:8080 -clients 25,50,100,200 \
  -events 60 -interval 500ms -chat-messages 100 -chat-interval 100ms \
  -ramp 50ms -timeout 10s | tee benchmarks/loadtest.jsonl
```

The tool creates a room and persists its chat messages. Raw output contains exact counts and unrounded percentiles. In another terminal, use `docker stats --no-stream --format '{{json .}}' "$(docker compose ps -q app)"` periodically for resource snapshots, and `docker compose exec -T app cat /proc/1/status` for RSS.

Raw measurements are in [loadtest.jsonl](loadtest.jsonl) and [resources.jsonl](resources.jsonl). Early streaming resource collection encountered Docker terminal-format parsing errors. One preliminary run was stopped and the reported run completed with periodic snapshot collection covering the full 200-client level.

Validation passed with `go test -race ./...`, `go vet ./...`, a two-level smoke run, and a race-instrumented load-generator smoke run. An intentional refused-port check exited 1 and correctly recorded one failed request and one client transport error.

## CV wording

Benchmarked a Go/WebSocket watch-party backend with 200 concurrent clients in one room, achieving 100% delivery across 11,940 playback updates with 9.60 ms p95 propagation latency in a local Docker test.
