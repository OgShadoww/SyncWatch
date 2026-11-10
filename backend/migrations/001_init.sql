CREATE TABLE IF NOT EXISTS rooms (
    id text PRIMARY KEY,
    host_hash bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    video_id text NOT NULL DEFAULT '',
    position double precision NOT NULL DEFAULT 0 CHECK (position >= 0),
    duration double precision NOT NULL DEFAULT 0 CHECK (duration >= 0),
    playing boolean NOT NULL DEFAULT false,
    version bigint NOT NULL DEFAULT 0,
    anchor timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS messages (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    room_id text NOT NULL REFERENCES rooms(id),
    name text NOT NULL,
    body text NOT NULL,
    sent_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS messages_by_room ON messages (room_id, id DESC);
