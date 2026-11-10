package playback

import (
	"errors"
	"math"
	"regexp"
	"time"
)

var (
	videoID    = regexp.MustCompile("^[a-zA-Z0-9_-]{11}$")
	ErrStale   = errors.New("playback changed; try again")
	ErrInvalid = errors.New("invalid playback command")
)

type State struct {
	VideoID  string    `json:"videoId"`
	Playing  bool      `json:"playing"`
	Position float64   `json:"position"`
	Duration float64   `json:"duration"`
	Version  int64     `json:"version"`
	Anchor   time.Time `json:"-"`
}

type Command struct {
	Action   string   `json:"action"`
	Version  *int64   `json:"version"`
	VideoID  string   `json:"videoId,omitempty"`
	Position *float64 `json:"position,omitempty"`
	Duration float64  `json:"duration,omitempty"`
}

// At projects the last committed position without changing its version.
func (s State) At(now time.Time) State {
	if s.Playing {
		s.Position += math.Max(0, now.Sub(s.Anchor).Seconds())
	}
	if s.Duration > 0 && s.Position >= s.Duration {
		s.Position, s.Playing = s.Duration, false
	}
	s.Anchor = now
	return s
}

func validSeconds(n float64) bool {
	return n >= 0 && n <= 7*24*3600 && !math.IsNaN(n)
}

func (s State) Apply(c Command, now time.Time) (State, error) {
	if c.Version == nil {
		return s, ErrInvalid
	}
	if *c.Version != s.Version {
		return s, ErrStale
	}
	next := s.At(now)
	if c.Action != "video" && next.VideoID == "" {
		return s, ErrInvalid
	}
	switch c.Action {
	case "video":
		if !videoID.MatchString(c.VideoID) {
			return s, ErrInvalid
		}
		next.VideoID, next.Position, next.Duration, next.Playing = c.VideoID, 0, 0, false
	case "play":
		if !validSeconds(c.Duration) {
			return s, ErrInvalid
		}
		if c.Duration > 0 {
			next.Duration = c.Duration
		}
		if next.Duration > 0 && next.Position >= next.Duration {
			next.Position = 0
		}
		next.Playing = true
	case "pause":
		next.Playing = false
	case "seek":
		if c.Position == nil || !validSeconds(*c.Position) {
			return s, ErrInvalid
		}
		next.Position = *c.Position
	default:
		return s, ErrInvalid
	}
	next.Version++
	return next.At(now), nil
}
