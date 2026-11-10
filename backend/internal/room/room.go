package room

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"syncwatch/internal/playback"
)

const HistoryLimit = 50

var (
	IDPattern    = regexp.MustCompile("^[A-F0-9]{10}$")
	ErrNotFound  = errors.New("room not found")
	ErrForbidden = errors.New("only the host can control playback")
	ErrName      = errors.New("enter a display name of 1–32 characters")
	ErrChat      = errors.New("messages must contain 1–500 characters")
)

type Message struct {
	ID     int64     `json:"id"`
	Name   string    `json:"name"`
	Text   string    `json:"text"`
	SentAt time.Time `json:"sentAt"`
}

type Saved struct {
	HostHash []byte
	State    playback.State
}

type Store interface {
	Create(context.Context, string, []byte) (bool, error)
	Load(context.Context, string) (Saved, []Message, error)
	SavePlayback(context.Context, string, playback.State) error
	AddMessage(context.Context, string, string, string) (Message, error)
}

type Person struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Host bool   `json:"host"`
}

type Event struct {
	Type    string          `json:"type"`
	Self    *Person         `json:"self,omitempty"`
	People  []Person        `json:"people,omitempty"`
	State   *playback.State `json:"state,omitempty"`
	History []Message       `json:"history,omitempty"`
	Message *Message        `json:"message,omitempty"`
	Error   string          `json:"error,omitempty"`
}

type Manager struct {
	store Store
	mu    sync.Mutex
	rooms map[string]*liveRoom
}

type liveRoom struct {
	mu      sync.Mutex
	refs    int // guarded by Manager.mu, includes joins in progress
	loaded  bool
	saved   Saved
	history []Message
	members map[*Session]bool
	nextID  int
}

type Session struct {
	room    *liveRoom
	manager *Manager
	roomID  string
	person  Person
	events  chan Event
	cancel  context.CancelFunc
}

func New(store Store) *Manager { return &Manager{store: store, rooms: make(map[string]*liveRoom)} }

func randomHex(n int) string {
	data := make([]byte, n)
	rand.Read(data)
	return hex.EncodeToString(data)
}

func (m *Manager) Create(ctx context.Context) (id, secret string, err error) {
	secret = randomHex(32)
	hash := sha256.Sum256([]byte(secret))
	for {
		id = strings.ToUpper(randomHex(5))
		inserted, err := m.store.Create(ctx, id, hash[:])
		if err != nil {
			return "", "", err
		}
		if inserted {
			return id, secret, nil
		}
	}
}

func (m *Manager) acquire(id string) *liveRoom {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.rooms[id]
	if r == nil {
		r = &liveRoom{members: make(map[*Session]bool)}
		m.rooms[id] = r
	}
	r.refs++
	return r
}

func (m *Manager) release(id string, r *liveRoom) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r.refs--
	if r.refs == 0 {
		delete(m.rooms, id)
	}
}

func clean(raw string, limit int) (string, bool) {
	value := strings.TrimSpace(raw)
	valid := value != "" && utf8.ValidString(value) && utf8.RuneCountInString(value) <= limit &&
		strings.IndexFunc(value, unicode.IsControl) == -1
	return value, valid
}

func (m *Manager) Join(ctx context.Context, id, rawName, secret string, cancel context.CancelFunc) (*Session, error) {
	if !IDPattern.MatchString(id) {
		return nil, ErrNotFound
	}
	name, ok := clean(rawName, 32)
	if !ok {
		return nil, ErrName
	}
	r := m.acquire(id)
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.loaded {
		saved, history, err := m.store.Load(ctx, id)
		if err != nil {
			m.release(id, r)
			return nil, err
		}
		r.saved, r.history, r.loaded = saved, history, true
	}
	hash := sha256.Sum256([]byte(secret))
	r.nextID++
	person := Person{ID: r.nextID, Name: name, Host: secret != "" && subtle.ConstantTimeCompare(hash[:], r.saved.HostHash) == 1}
	s := &Session{room: r, manager: m, roomID: id, person: person, events: make(chan Event, 32), cancel: cancel}
	r.members[s] = true
	people := r.people()
	state := r.saved.State.At(time.Now())
	s.send(Event{Type: "snapshot", Self: &person, People: people, State: &state, History: slices.Clone(r.history)})
	for other := range r.members {
		if other != s {
			other.send(Event{Type: "people", People: people})
		}
	}
	return s, nil
}

func (r *liveRoom) people() []Person {
	people := make([]Person, 0, len(r.members))
	for s := range r.members {
		people = append(people, s.person)
	}
	slices.SortFunc(people, func(a, b Person) int { return a.ID - b.ID })
	return people
}

func (s *Session) Events() <-chan Event { return s.events }

func (s *Session) Sync() {
	s.room.mu.Lock()
	defer s.room.mu.Unlock()
	state := s.room.saved.State.At(time.Now())
	s.send(Event{Type: "state", State: &state})
}

func (s *Session) send(e Event) {
	select {
	case s.events <- e:
	default:
		s.cancel() // a slow reader reconnects for a fresh snapshot
	}
}

func (r *liveRoom) broadcast(e Event) {
	for s := range r.members {
		s.send(e)
	}
}

func (s *Session) Leave() {
	r := s.room
	r.mu.Lock()
	delete(r.members, s)
	r.broadcast(Event{Type: "people", People: r.people()})
	r.mu.Unlock()
	s.manager.release(s.roomID, r)
}

func (s *Session) Playback(ctx context.Context, c playback.Command) error {
	if !s.person.Host {
		return ErrForbidden
	}
	r := s.room
	r.mu.Lock()
	defer r.mu.Unlock()
	next, err := r.saved.State.Apply(c, time.Now())
	if err != nil {
		return err
	}
	if err := s.manager.store.SavePlayback(ctx, s.roomID, next); err != nil {
		return fmt.Errorf("save playback: %w", err)
	}
	r.saved.State = next
	state := next.At(time.Now())
	r.broadcast(Event{Type: "state", State: &state})
	return nil
}

func (s *Session) Chat(ctx context.Context, raw string) error {
	body, ok := clean(raw, 500)
	if !ok {
		return ErrChat
	}
	r := s.room
	r.mu.Lock()
	defer r.mu.Unlock()
	msg, err := s.manager.store.AddMessage(ctx, s.roomID, s.person.Name, body)
	if err != nil {
		return err
	}
	if len(r.history) == HistoryLimit {
		r.history = r.history[1:]
	}
	r.history = append(r.history, msg)
	r.broadcast(Event{Type: "chat", Message: &msg})
	return nil
}
