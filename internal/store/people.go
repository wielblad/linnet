package store

import (
	"context"
	"fmt"
	"time"
)

// Receipt is how far one participant has received and read a thread.
type Receipt struct {
	ContactID int64
	Read      time.Time
	Delivered time.Time
}

// migratePeople adds what the window needs beyond the terminal client:
// picture addresses, per-thread nicknames and delivery marks.
func (s *Store) migratePeople(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS pictures (
	id INTEGER PRIMARY KEY,
	url TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS nicknames (
	thread_id INTEGER NOT NULL,
	contact_id INTEGER NOT NULL,
	nickname TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (thread_id, contact_id)
);
`
	if _, err := s.q.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrate people: %w", err)
	}
	if _, err := s.q.ExecContext(ctx, `ALTER TABLE participant_reads ADD COLUMN delivered_ms INTEGER NOT NULL DEFAULT 0`); err != nil && !isDuplicateColumn(err) {
		return fmt.Errorf("migrate delivery marks: %w", err)
	}
	return nil
}

// SetPicture remembers the picture address of a contact or of a group thread.
func (s *Store) SetPicture(ctx context.Context, id int64, url string) error {
	if id == 0 || url == "" {
		return nil
	}
	_, err := s.q.ExecContext(ctx, `
INSERT INTO pictures (id, url) VALUES (?, ?)
ON CONFLICT(id) DO UPDATE SET url=excluded.url
`, id, url)
	if err != nil {
		return fmt.Errorf("set picture: %w", err)
	}
	return nil
}

// Pictures returns every known picture address by contact or thread id.
func (s *Store) Pictures(ctx context.Context) (map[int64]string, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT id, url FROM pictures WHERE url != ''`)
	if err != nil {
		return nil, fmt.Errorf("pictures: %w", err)
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var url string
		if err := rows.Scan(&id, &url); err != nil {
			return nil, fmt.Errorf("scan picture: %w", err)
		}
		out[id] = url
	}
	return out, rows.Err()
}

// SetNickname stores the name a participant carries inside one thread.
// An empty nickname is ignored: participant rows without one must not erase a known nickname.
func (s *Store) SetNickname(ctx context.Context, threadID, contactID int64, nickname string) error {
	if threadID == 0 || contactID == 0 || nickname == "" {
		return nil
	}
	_, err := s.q.ExecContext(ctx, `
INSERT INTO nicknames (thread_id, contact_id, nickname) VALUES (?, ?, ?)
ON CONFLICT(thread_id, contact_id) DO UPDATE SET nickname=excluded.nickname
`, threadID, contactID, nickname)
	if err != nil {
		return fmt.Errorf("set nickname: %w", err)
	}
	return nil
}

// Nicknames returns the nicknames used inside one thread by contact id.
func (s *Store) Nicknames(ctx context.Context, threadID int64) (map[int64]string, error) {
	return s.nicknameMap(ctx, `SELECT contact_id, nickname FROM nicknames WHERE thread_id = ? AND nickname != ''`, threadID)
}

// TitleNicknames returns, by thread id, the nickname of the other person in one-to-one threads.
func (s *Store) TitleNicknames(ctx context.Context) (map[int64]string, error) {
	return s.nicknameMap(ctx, `SELECT thread_id, nickname FROM nicknames WHERE contact_id = thread_id AND nickname != ''`)
}

func (s *Store) nicknameMap(ctx context.Context, query string, args ...any) (map[int64]string, error) {
	rows, err := s.q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("nicknames: %w", err)
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scan nickname: %w", err)
		}
		out[id] = name
	}
	return out, rows.Err()
}

// SetParticipantDelivered records that everything up to the given time reached the participant.
func (s *Store) SetParticipantDelivered(ctx context.Context, threadID, contactID int64, at time.Time) error {
	if threadID == 0 || contactID == 0 || at.UnixMilli() <= 0 {
		return nil
	}
	_, err := s.q.ExecContext(ctx, `
INSERT INTO participant_reads (thread_id, contact_id, delivered_ms) VALUES (?, ?, ?)
ON CONFLICT(thread_id, contact_id) DO UPDATE SET delivered_ms = MAX(delivered_ms, excluded.delivered_ms)
`, threadID, contactID, at.UnixMilli())
	if err != nil {
		return fmt.Errorf("set participant delivered: %w", err)
	}
	return nil
}

// Receipts returns the read and delivery marks of everyone in the thread except the account itself.
func (s *Store) Receipts(ctx context.Context, threadID, selfID int64) ([]Receipt, error) {
	rows, err := s.q.QueryContext(ctx, `
SELECT contact_id, read_ms, delivered_ms FROM participant_reads
WHERE thread_id = ? AND contact_id != ? ORDER BY contact_id`, threadID, selfID)
	if err != nil {
		return nil, fmt.Errorf("receipts: %w", err)
	}
	defer rows.Close()
	var out []Receipt
	for rows.Next() {
		var id, readMS, deliveredMS int64
		if err := rows.Scan(&id, &readMS, &deliveredMS); err != nil {
			return nil, fmt.Errorf("scan receipt: %w", err)
		}
		out = append(out, Receipt{ContactID: id, Read: time.UnixMilli(readMS), Delivered: time.UnixMilli(deliveredMS)})
	}
	return out, rows.Err()
}

// RepairThreadActivity makes every thread at least as recent as its newest stored message,
// with that message as the preview. The server's own "last activity" of an encrypted chat
// can be days behind, which would sort the chat far too low.
func (s *Store) RepairThreadActivity(ctx context.Context) error {
	_, err := s.q.ExecContext(ctx, `
UPDATE threads SET
	last_preview = COALESCE((
		SELECT CASE
			WHEN m.text != '' THEN m.text
			WHEN m.media_kind != '' THEN '[' || m.media_kind || ']'
			ELSE threads.last_preview END
		FROM messages m WHERE m.thread_id = threads.id AND m.pending = 0
		ORDER BY m.timestamp_ms DESC, m.id DESC LIMIT 1), last_preview),
	updated_at_ms = (SELECT MAX(m.timestamp_ms) FROM messages m WHERE m.thread_id = threads.id AND m.pending = 0)
WHERE updated_at_ms < (SELECT MAX(m.timestamp_ms) FROM messages m WHERE m.thread_id = threads.id AND m.pending = 0)
`)
	if err != nil {
		return fmt.Errorf("repair thread activity: %w", err)
	}
	return nil
}

// AllReceipts returns the read and delivery marks of every thread, by thread id.
// The rows also tell who takes part in a thread.
func (s *Store) AllReceipts(ctx context.Context) (map[int64][]Receipt, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT thread_id, contact_id, read_ms, delivered_ms FROM participant_reads ORDER BY thread_id, read_ms DESC`)
	if err != nil {
		return nil, fmt.Errorf("all receipts: %w", err)
	}
	defer rows.Close()
	out := map[int64][]Receipt{}
	for rows.Next() {
		var thread, id, readMS, deliveredMS int64
		if err := rows.Scan(&thread, &id, &readMS, &deliveredMS); err != nil {
			return nil, fmt.Errorf("scan receipt: %w", err)
		}
		out[thread] = append(out[thread], Receipt{ContactID: id, Read: time.UnixMilli(readMS), Delivered: time.UnixMilli(deliveredMS)})
	}
	return out, rows.Err()
}

// LastSenders returns who sent the newest stored message of each thread.
func (s *Store) LastSenders(ctx context.Context) (map[int64]int64, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT thread_id, sender_id, MAX(timestamp_ms) FROM messages WHERE pending = 0 GROUP BY thread_id`)
	if err != nil {
		return nil, fmt.Errorf("last senders: %w", err)
	}
	defer rows.Close()
	out := map[int64]int64{}
	for rows.Next() {
		var thread, sender, ts int64
		if err := rows.Scan(&thread, &sender, &ts); err != nil {
			return nil, fmt.Errorf("scan last sender: %w", err)
		}
		out[thread] = sender
	}
	return out, rows.Err()
}
