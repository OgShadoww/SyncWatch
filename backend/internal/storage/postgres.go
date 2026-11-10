package storage

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"syncwatch/internal/playback"
	"syncwatch/internal/room"
	"syncwatch/migrations"
)

type Postgres struct{ DB *pgxpool.Pool }

func (p Postgres) Init(ctx context.Context) error {
	if _, err := p.DB.Exec(ctx, migrations.Init); err != nil {
		return err
	}
	// A restart resumes from the last committed control, paused.
	_, err := p.DB.Exec(ctx, "UPDATE rooms SET playing=false, version=version+1 WHERE playing")
	return err
}

func (p Postgres) Create(ctx context.Context, id string, hash []byte) (bool, error) {
	tag, err := p.DB.Exec(ctx, "INSERT INTO rooms (id, host_hash) VALUES ($1, $2) ON CONFLICT DO NOTHING", id, hash)
	return tag.RowsAffected() == 1, err
}

func (p Postgres) Load(ctx context.Context, id string) (room.Saved, []room.Message, error) {
	var data room.Saved
	s := &data.State
	err := p.DB.QueryRow(ctx, `SELECT host_hash, video_id, position, duration, playing, version, anchor
		FROM rooms WHERE id=$1`, id).Scan(&data.HostHash, &s.VideoID, &s.Position, &s.Duration, &s.Playing, &s.Version, &s.Anchor)
	if errors.Is(err, pgx.ErrNoRows) {
		err = room.ErrNotFound
	}
	if err != nil {
		return data, nil, err
	}
	rows, err := p.DB.Query(ctx, `SELECT id, name, body, sent_at FROM messages
		WHERE room_id=$1 ORDER BY id DESC LIMIT $2`, id, room.HistoryLimit)
	if err != nil {
		return data, nil, err
	}
	msgs, err := pgx.CollectRows(rows, pgx.RowToStructByPos[room.Message])
	slices.Reverse(msgs)
	return data, msgs, err
}

func (p Postgres) SavePlayback(ctx context.Context, id string, s playback.State) error {
	_, err := p.DB.Exec(ctx, `UPDATE rooms SET video_id=$2, position=$3, duration=$4,
		playing=$5, version=$6, anchor=$7 WHERE id=$1`, id, s.VideoID, s.Position, s.Duration, s.Playing, s.Version, s.Anchor)
	return err
}

func (p Postgres) AddMessage(ctx context.Context, id, name, body string) (room.Message, error) {
	msg := room.Message{Name: name, Text: body}
	err := p.DB.QueryRow(ctx, `INSERT INTO messages (room_id,name,body) VALUES ($1,$2,$3)
		RETURNING id, sent_at`, id, name, body).Scan(&msg.ID, &msg.SentAt)
	return msg, err
}
