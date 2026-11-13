package store

import (
	"context"
	"testing"
	"time"
)

func TestLastAttachments(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	must := func(m Message) {
		if err := s.UpsertMessage(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	must(Message{ID: "a1", ThreadID: 1, SenderID: 5, Text: "hej", Timestamp: time.UnixMilli(1000)})
	must(Message{ID: "a2", ThreadID: 1, SenderID: 5, Timestamp: time.UnixMilli(2000), MediaKind: "file", MediaName: "umowa.pdf"})
	must(Message{ID: "b1", ThreadID: 2, SenderID: 5, Timestamp: time.UnixMilli(1000), MediaKind: "image", MediaName: "image"})
	must(Message{ID: "b2", ThreadID: 2, SenderID: 5, Text: "tekst na koniec", Timestamp: time.UnixMilli(3000)})
	got, err := s.LastAttachments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got[1] != [2]string{"file", "umowa.pdf"} {
		t.Fatalf("thread 1: %v", got[1])
	}
	if _, has := got[2]; has {
		t.Fatalf("thread 2 ends with text, got %v", got[2])
	}
}
