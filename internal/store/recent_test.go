package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestRecentMessages(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i := 0; i < 10; i++ {
		m := Message{ID: fmt.Sprintf("m%02d", i), ThreadID: 7, SenderID: 1, Text: fmt.Sprint(i), Timestamp: time.UnixMilli(int64(1000 + i))}
		if err := s.UpsertMessage(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	got, more, err := s.RecentMessages(ctx, 7, 4)
	if err != nil || len(got) != 4 || !more || got[0].ID != "m06" || got[3].ID != "m09" {
		t.Fatalf("newest four: %d messages, more=%v, err=%v", len(got), more, err)
	}
	got, more, err = s.RecentMessages(ctx, 7, 50)
	if err != nil || len(got) != 10 || more || got[0].ID != "m00" {
		t.Fatalf("whole thread: %d messages, more=%v, err=%v", len(got), more, err)
	}
	if all, _ := s.ListMessages(ctx, 7); len(all) != 10 {
		t.Fatalf("ListMessages returned %d", len(all))
	}
}
