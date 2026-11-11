// Command loadtest measures one room's WebSocket broadcasts through the public API.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"syncwatch/internal/room"
)

type config struct {
	URL                                   string
	Clients                               string
	Events, ChatMessages                  int
	Interval, ChatInterval, Ramp, Timeout time.Duration
}

type result struct {
	RoomID                                                                    string
	Clients, PeakConnected                                                    int
	Started, Finished                                                         time.Time
	PlaybackSent, PlaybackReceived, PlaybackExpected                          int
	DeliveryPercent, P50MS, P95MS, P99MS                                      float64
	ChatSent, ChatCommitted, ChatReceived, ChatExpected                       int
	ChatSeconds, ChatPerSecond                                                float64
	ApplicationErrors, ClientErrors, FailedRequests, Disconnections, Timeouts int
	Failure                                                                   string `json:",omitempty"`
}

type received struct {
	client int
	event  room.Event
	at     time.Time
	err    error
}

type trial struct {
	roomID, cookie string
	cfg            config
	ctx            context.Context
	conns          []*websocket.Conn
	inbox          chan received
	readers        sync.WaitGroup
	result         result
	version        int64
	latencies      []float64
	alive          int
}

func (t *trial) failure(err error, application bool) {
	t.result.FailedRequests++
	if application {
		t.result.ApplicationErrors++
	} else {
		t.result.ClientErrors++
	}
	var timeout interface{ Timeout() bool }
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
		t.result.Timeouts++
	}
}

// Readers timestamp decoded events before handing them to the collector.
func (t *trial) read(index int, conn *websocket.Conn) {
	defer t.readers.Done()
	for {
		var event room.Event
		err := wsjson.Read(t.ctx, conn, &event)
		at := time.Now()
		if t.ctx.Err() != nil {
			return
		}
		if err != nil || event.Type == "state" || event.Type == "chat" || event.Type == "error" {
			select {
			case t.inbox <- received{index, event, at, err}:
			case <-t.ctx.Done():
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (t *trial) eventError(e received) error {
	if e.err != nil {
		t.result.Disconnections++
		t.alive--
		var syntax *json.SyntaxError
		var typeErr *json.UnmarshalTypeError
		if errors.As(e.err, &syntax) || errors.As(e.err, &typeErr) {
			t.result.ClientErrors++
		}
		return fmt.Errorf("client %d disconnected: %w", e.client, e.err)
	}
	if e.event.Type == "error" {
		t.result.ApplicationErrors++
		return fmt.Errorf("application rejected client %d: %s", e.client, e.event.Error)
	}
	return nil
}

func (t *trial) join(endpoint, cookie string, index int) error {
	ctx, cancel := context.WithTimeout(t.ctx, t.cfg.Timeout)
	defer cancel()
	headers := http.Header{}
	if index == 0 {
		headers.Set("Cookie", cookie)
	}
	conn, res, err := websocket.Dial(ctx, endpoint, &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		t.failure(err, res != nil && res.StatusCode != http.StatusSwitchingProtocols)
		return fmt.Errorf("connect client %d: %w", index, err)
	}
	conn.SetReadLimit(1 << 20)
	t.conns = append(t.conns, conn)
	if err := wsjson.Write(ctx, conn, map[string]string{"type": "join", "name": fmt.Sprintf("loadtest-%d", index)}); err != nil {
		t.failure(err, false)
		return err
	}
	var snapshot room.Event
	if err := wsjson.Read(ctx, conn, &snapshot); err != nil {
		t.failure(err, false)
		return err
	}
	if snapshot.Type != "snapshot" || snapshot.Self == nil || snapshot.State == nil || snapshot.Self.Host != (index == 0) {
		err := errors.New("invalid snapshot or host authentication")
		t.failure(err, false)
		return err
	}
	if index == 0 {
		t.version = snapshot.State.Version
	}
	// Drain disconnects during ramp-up before counting another live participant.
	for len(t.inbox) > 0 {
		if err := t.eventError(<-t.inbox); err != nil {
			return err
		}
	}
	t.alive++
	t.result.PeakConnected = max(t.result.PeakConnected, t.alive)
	t.readers.Add(1)
	go t.read(index, conn)
	return nil
}

// Each operation waits for all recipients. A missing host acknowledgement aborts
// the level instead of guessing the next version or retrying a committed write.
func (t *trial) exchange(sender int, command map[string]any, measured bool) (time.Time, error) {
	playback := command["type"] == "playback"
	start := time.Now()
	ctx, cancel := context.WithTimeout(t.ctx, t.cfg.Timeout)
	defer cancel()
	if measured {
		if playback {
			t.result.PlaybackSent++
			t.result.PlaybackExpected += t.result.Clients - 1
		} else {
			t.result.ChatSent++
			t.result.ChatExpected += t.result.Clients
		}
	}
	if err := wsjson.Write(ctx, t.conns[sender], command); err != nil {
		t.failure(err, false)
		return start, err
	}
	seen := make([]bool, len(t.conns))
	remaining := len(seen)
	for remaining > 0 {
		select {
		case <-ctx.Done():
			t.result.FailedRequests++
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				t.result.Timeouts++
			}
			return start, ctx.Err()
		case e := <-t.inbox:
			if err := t.eventError(e); err != nil {
				t.result.FailedRequests++
				return start, err
			}
			match := e.event.Type == "chat" && e.event.Message != nil && e.event.Message.Text == command["text"]
			if playback {
				match = e.event.Type == "state" && e.event.State != nil && e.event.State.Version == t.version+1
			}
			if !match || seen[e.client] {
				continue
			}
			seen[e.client] = true
			remaining--
			if measured {
				if playback && e.client != 0 {
					t.result.PlaybackReceived++
					t.latencies = append(t.latencies, e.at.Sub(start).Seconds()*1000)
				} else if !playback {
					t.result.ChatReceived++
					if e.client == sender {
						t.result.ChatCommitted++
					}
				}
			}
		}
	}
	if playback {
		t.version++
	}
	return start, nil
}

func wait(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(max(duration, 0))
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (t *trial) createRoom() error {
	ctx, cancel := context.WithTimeout(t.ctx, t.cfg.Timeout)
	req, err := http.NewRequestWithContext(ctx, "POST", t.cfg.URL+"/api/rooms", nil)
	if err != nil {
		cancel()
		t.failure(err, false)
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.failure(err, false)
		return err
	}
	var created struct{ ID string }
	err = json.NewDecoder(res.Body).Decode(&created)
	res.Body.Close()
	cancel()
	if res.StatusCode != http.StatusCreated {
		err := fmt.Errorf("create room: HTTP %d", res.StatusCode)
		t.failure(err, true)
		return err
	}
	if err != nil || created.ID == "" {
		err := errors.New("invalid create-room response")
		t.failure(err, false)
		return err
	}
	for _, c := range res.Cookies() {
		if c.Name == "syncwatch_host" {
			t.cookie = c.Name + "=" + c.Value
		}
	}
	t.roomID = created.ID
	return nil
}

func (t *trial) run() error {
	fmt.Fprintf(os.Stderr, "clients=%d ramping\n", t.result.Clients)
	if t.roomID == "" {
		if err := t.createRoom(); err != nil {
			return err
		}
	}
	t.result.RoomID = t.roomID
	endpoint := strings.Replace(t.cfg.URL, "http", "ws", 1) + "/api/rooms/" + t.roomID + "/ws"
	for i := len(t.conns); i < t.result.Clients; i++ {
		if err := t.join(endpoint, t.cookie, i); err != nil {
			return err
		}
		if err := wait(t.ctx, t.cfg.Ramp); err != nil {
			return err
		}
	}
	_, err := t.exchange(0, map[string]any{"type": "playback", "action": "video", "videoId": "M7lc1UVf-VE", "version": t.version}, false)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "clients=%d playback (6 warm-up + %d measured)\n", t.result.Clients, t.cfg.Events)
	for i := range t.cfg.Events + 6 {
		command := map[string]any{"type": "playback", "version": t.version}
		switch i % 3 {
		case 0:
			command["action"], command["duration"] = "play", 600
		case 1:
			command["action"], command["position"] = "seek", float64(i%300)
		case 2:
			command["action"] = "pause"
		}
		start, err := t.exchange(0, command, i >= 6)
		if err != nil {
			return err
		}
		if err := wait(t.ctx, time.Until(start.Add(t.cfg.Interval))); err != nil {
			return err
		}
	}
	fmt.Fprintf(os.Stderr, "clients=%d chat (5 warm-up + %d measured)\n", t.result.Clients, t.cfg.ChatMessages)
	var chatStart time.Time
	defer func() {
		if !chatStart.IsZero() {
			t.result.ChatSeconds = time.Since(chatStart).Seconds()
			t.result.ChatPerSecond = float64(t.result.ChatCommitted) / t.result.ChatSeconds
		}
	}()
	for i := range t.cfg.ChatMessages + 5 {
		if i == 5 {
			chatStart = time.Now()
		}
		command := map[string]any{"type": "chat", "text": fmt.Sprintf("loadtest-%d-%d", t.result.Clients, i)}
		start, err := t.exchange(i%len(t.conns), command, i >= 5)
		if err != nil {
			return err
		}
		if err := wait(t.ctx, time.Until(start.Add(t.cfg.ChatInterval))); err != nil {
			return err
		}
	}
	return nil
}

func (t *trial) runLevel(clients int) result {
	t.result = result{Clients: clients, PeakConnected: len(t.conns), Started: time.Now().UTC()}
	t.latencies = nil
	if err := t.run(); err != nil {
		t.result.Failure = err.Error()
	}
	for len(t.inbox) > 0 {
		if err := t.eventError(<-t.inbox); err != nil {
			t.result.Failure = err.Error()
		}
	}
	t.result.Finished = time.Now().UTC()
	if t.result.PlaybackExpected > 0 {
		t.result.DeliveryPercent = 100 * float64(t.result.PlaybackReceived) / float64(t.result.PlaybackExpected)
	}
	slices.Sort(t.latencies)
	percentile := func(p float64) float64 {
		if len(t.latencies) == 0 {
			return 0
		}
		return t.latencies[int(math.Ceil(p*float64(len(t.latencies))))-1]
	}
	t.result.P50MS, t.result.P95MS, t.result.P99MS = percentile(.50), percentile(.95), percentile(.99)
	return t.result
}

func run() int {
	cfg := config{}
	flag.StringVar(&cfg.URL, "url", "http://localhost:8080", "SyncWatch base URL")
	flag.StringVar(&cfg.Clients, "clients", "25,50,100,200", "total clients per level, including one host")
	flag.IntVar(&cfg.Events, "events", 60, "measured playback commands per level (minimum 30)")
	flag.IntVar(&cfg.ChatMessages, "chat-messages", 100, "measured chat messages per level")
	flag.DurationVar(&cfg.Interval, "interval", 500*time.Millisecond, "minimum time between playback sends")
	flag.DurationVar(&cfg.ChatInterval, "chat-interval", 100*time.Millisecond, "minimum time between chat sends")
	flag.DurationVar(&cfg.Ramp, "ramp", 50*time.Millisecond, "pause after each connection")
	flag.DurationVar(&cfg.Timeout, "timeout", 10*time.Second, "timeout per connect, request or broadcast")
	flag.Parse()
	cfg.URL = strings.TrimRight(cfg.URL, "/")
	base, err := url.Parse(cfg.URL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" ||
		cfg.Events < 30 || cfg.ChatMessages < 1 || cfg.Interval <= 0 || cfg.ChatInterval <= 0 || cfg.Ramp <= 0 || cfg.Timeout <= 0 {
		fmt.Fprintln(os.Stderr, "invalid URL or benchmark settings")
		return 2
	}
	var levels []int
	for _, value := range strings.Split(cfg.Clients, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || n < 2 || (len(levels) > 0 && n <= levels[len(levels)-1]) {
			fmt.Fprintln(os.Stderr, "client levels must increase and each must be at least 2")
			return 2
		}
		levels = append(levels, n)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	t := &trial{cfg: cfg, ctx: ctx, inbox: make(chan received, levels[len(levels)-1]*2)}
	defer func() {
		// Intentional teardown is excluded from disconnect counts.
		cancel()
		for _, conn := range t.conns {
			conn.CloseNow()
		}
		t.readers.Wait()
	}()
	out := json.NewEncoder(os.Stdout)
	out.Encode(map[string]any{"Config": cfg, "Go": runtime.Version(), "OS": runtime.GOOS, "Arch": runtime.GOARCH, "Started": time.Now().UTC()})
	failed := false
	for _, clients := range levels {
		res := t.runLevel(clients)
		out.Encode(res)
		if res.Failure != "" {
			failed = true
			fmt.Fprintln(os.Stderr, res.Failure)
			break
		}
		if ctx.Err() != nil {
			break
		}
	}
	if failed {
		return 1
	}
	return 0
}

func main() { os.Exit(run()) }
