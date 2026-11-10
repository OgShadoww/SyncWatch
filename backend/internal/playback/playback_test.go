package playback

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestTimeline(t *testing.T) {
	start := time.Unix(100, 0)
	s := State{}
	apply := func(seconds int, c Command) {
		t.Helper()
		c.Version = &s.Version
		next, err := s.Apply(c, start.Add(time.Duration(seconds)*time.Second))
		if err != nil || next.Version != s.Version+1 {
			t.Fatalf("%s: %+v, %v", c.Action, next, err)
		}
		s = next
	}
	apply(0, Command{Action: "video", VideoID: "M7lc1UVf-VE"})
	apply(1, Command{Action: "play", Duration: 60})
	if got := s.At(start.Add(11 * time.Second)); got.Position != 10 || !got.Playing || got.Version != 2 {
		t.Fatal(got)
	}
	apply(21, Command{Action: "pause"})
	if got := s.At(start.Add(time.Hour)); got.Position != 20 || got.Playing {
		t.Fatal(got)
	}
	pos := 55.0
	apply(22, Command{Action: "seek", Position: &pos})
	apply(23, Command{Action: "play"})
	if got := s.At(start.Add(50 * time.Second)); got.Position != 60 || got.Playing {
		t.Fatal(got)
	}
	apply(50, Command{Action: "play"})
	if s.Position != 0 || !s.Playing {
		t.Fatal(s)
	}
	apply(52, Command{Action: "video", VideoID: "aqz-KE-bpKQ"})
	if s.VideoID != "aqz-KE-bpKQ" || s.Position != 0 || s.Playing || s.Duration != 0 {
		t.Fatal(s)
	}
}

func TestRejectInvalidOrStaleCommands(t *testing.T) {
	version, stale := int64(2), int64(1)
	bad, nan := -1.0, math.NaN()
	state := State{VideoID: "M7lc1UVf-VE", Version: version}
	for _, c := range []Command{
		{Action: "play"}, {Action: "video", Version: &version, VideoID: "invalid"},
		{Action: "seek", Version: &version}, {Action: "seek", Version: &version, Position: &bad},
		{Action: "seek", Version: &version, Position: &nan}, {Action: "invalid", Version: &version},
		{Action: "play", Version: &version, Duration: math.Inf(1)},
	} {
		if next, err := state.Apply(c, time.Now()); !errors.Is(err, ErrInvalid) || next != state {
			t.Fatalf("%+v: %+v %v", c, next, err)
		}
	}
	if next, err := state.Apply(Command{Action: "pause", Version: &stale}, time.Now()); !errors.Is(err, ErrStale) || next != state {
		t.Fatal(next, err)
	}
	var empty State
	if _, err := empty.Apply(Command{Action: "play", Version: &empty.Version}, time.Now()); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}
