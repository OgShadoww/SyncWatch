package room

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"sync"
	"testing"

	"syncwatch/internal/playback"
)

// A single-room store for exercising the manager without a database.
type memoryStore struct {
	id      string
	saved   Saved
	history []Message
	saveErr error
}

func (m *memoryStore) Create(_ context.Context, id string, hash []byte) (bool, error) {
	m.id = id
	m.saved.HostHash = hash
	return true, nil
}
func (m *memoryStore) Load(_ context.Context, id string) (Saved, []Message, error) {
	if id != m.id {
		return Saved{}, nil, ErrNotFound
	}
	return m.saved, m.history, nil
}
func (m *memoryStore) SavePlayback(_ context.Context, _ string, s playback.State) error {
	if m.saveErr == nil {
		m.saved.State = s
	}
	return m.saveErr
}
func (m *memoryStore) AddMessage(_ context.Context, _ string, name, body string) (Message, error) {
	msg := Message{ID: int64(len(m.history) + 1), Name: name, Text: body}
	m.history = append(m.history, msg)
	return msg, nil
}

func TestMembershipAuthorizationAndRejoin(t *testing.T) {
	ctx := t.Context()
	store := &memoryStore{}
	m := New(store)
	id, secret, err := m.Create(ctx)
	hash := sha256.Sum256([]byte(secret))
	if err != nil || !IDPattern.MatchString(id) || !bytes.Equal(hash[:], store.saved.HostHash) {
		t.Fatal("invalid room credentials", err)
	}
	join := func(name, key string) *Session {
		t.Helper()
		s, err := m.Join(ctx, id, name, key, func() {})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	host := join("Host", secret)
	if snap := <-host.Events(); !snap.Self.Host || len(snap.People) != 1 {
		t.Fatal(snap)
	}
	guest := join("Host", "forged")
	if snap := <-guest.Events(); snap.Self.Host || len(snap.People) != 2 {
		t.Fatal(snap)
	}
	if e := <-host.Events(); e.Type != "people" || len(e.People) != 2 {
		t.Fatal(e)
	}
	version := int64(0)
	cmd := playback.Command{Action: "video", VideoID: "M7lc1UVf-VE", Version: &version}
	if err := guest.Playback(ctx, cmd); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
	if err := host.Playback(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	if e := <-guest.Events(); e.State == nil || e.State.Version != 1 {
		t.Fatal(e)
	}
	if err := guest.Chat(ctx, " hello "); err != nil {
		t.Fatal(err)
	}
	guest.Leave()
	host.Leave()
	if len(m.rooms) != 0 {
		t.Fatal("empty room retained")
	}
	host = join("Returning host", secret)
	defer host.Leave()
	snap := <-host.Events()
	if !snap.Self.Host || snap.State.VideoID != cmd.VideoID || len(snap.History) != 1 || snap.History[0].Text != "hello" {
		t.Fatal(snap)
	}
	if _, err := m.Join(ctx, id, "  ", "", func() {}); !errors.Is(err, ErrName) {
		t.Fatal(err)
	}
	other := "0000000000"
	if id == other {
		other = "1111111111"
	}
	if _, err := m.Join(ctx, other, "Guest", "", func() {}); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if len(m.rooms) != 1 {
		t.Fatal("unknown room retained")
	}
	if err := host.Chat(ctx, strings.Repeat("é", 501)); !errors.Is(err, ErrChat) {
		t.Fatal(err)
	}
}

func TestConcurrentPlaybackAndFailedCommit(t *testing.T) {
	ctx := t.Context()
	store := &memoryStore{}
	m := New(store)
	id, secret, err := m.Create(ctx)
	if err != nil {
		t.Fatal(err)
	}
	host, err := m.Join(ctx, id, "Host", secret, func() {})
	if err != nil {
		t.Fatal(err)
	}
	defer host.Leave()
	<-host.Events()
	var version int64
	cmd := playback.Command{Action: "video", VideoID: "M7lc1UVf-VE", Version: &version}
	store.saveErr = errors.New("offline")
	if err := host.Playback(ctx, cmd); !errors.Is(err, store.saveErr) {
		t.Fatal(err)
	}
	select {
	case <-host.Events():
		t.Fatal("broadcast uncommitted state")
	default:
	}
	store.saveErr = nil
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() { results <- host.Playback(ctx, cmd) })
	}
	wg.Wait()
	close(results)
	commits, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			commits++
		case errors.Is(err, playback.ErrStale):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if commits != 1 || conflicts != 1 || store.saved.State.Version != 1 {
		t.Fatalf("commits=%d conflicts=%d state=%+v", commits, conflicts, store.saved.State)
	}
}
