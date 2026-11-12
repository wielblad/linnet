package meta

import (
	"context"
	"testing"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"lazyfmess/internal/store"
)

// A nickname belongs to the thread, the contact keeps its real name, and the picture
// address, read mark and delivery mark of a participant row are all kept.
func TestApplyTablePeople(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	tbl := &table.LSTable{
		LSVerifyContactRowExists: []*table.LSVerifyContactRowExists{
			{ContactId: 7, Name: "Marek Przykładowy", ProfilePictureUrl: "https://example.invalid/p/7.jpg?sig=1"},
		},
		LSDeleteThenInsertThread: []*table.LSDeleteThenInsertThread{
			{ThreadKey: 7, ThreadType: table.ONE_TO_ONE, LastActivityTimestampMs: 1_700_000_000_000},
			{ThreadKey: 500, ThreadName: "Grupa", ThreadType: table.GROUP_THREAD, ThreadPictureUrl: "https://example.invalid/t/500.jpg"},
		},
		LSAddParticipantIdToGroupThread: []*table.LSAddParticipantIdToGroupThread{
			{ThreadKey: 7, ContactId: 7, Nickname: "Maro", ReadWatermarkTimestampMs: 1_700_000_000_100, DeliveredWatermarkTimestampMs: 1_700_000_000_200},
			{ThreadKey: 500, ContactId: 7, Nickname: ""},
		},
		LSUpdateDeliveryReceipt: []*table.LSUpdateDeliveryReceipt{
			{ThreadKey: 7, ContactId: 7, DeliveredWatermarkTimestampMs: 1_700_000_000_300},
		},
	}
	if err := ApplyTable(ctx, s, tbl); err != nil {
		t.Fatalf("ApplyTable: %v", err)
	}
	// a later participant row without a nickname must not erase it
	if err := ApplyTable(ctx, s, &table.LSTable{LSAddParticipantIdToGroupThread: []*table.LSAddParticipantIdToGroupThread{{ThreadKey: 7, ContactId: 7}}}); err != nil {
		t.Fatalf("ApplyTable again: %v", err)
	}

	if name, _ := s.ContactName(ctx, 7); name != "Marek Przykładowy" {
		t.Fatalf("contact name = %q, want the real name", name)
	}
	titles, err := s.TitleNicknames(ctx)
	if err != nil || titles[7] != "Maro" {
		t.Fatalf("title nicknames = %v, %v", titles, err)
	}
	if group, _ := s.Nicknames(ctx, 500); len(group) != 0 {
		t.Fatalf("group nicknames = %v, want none", group)
	}
	pics, err := s.Pictures(ctx)
	if err != nil || pics[7] == "" || pics[500] == "" {
		t.Fatalf("pictures = %v, %v", pics, err)
	}
	rs, err := s.Receipts(ctx, 7, 99)
	if err != nil || len(rs) != 1 {
		t.Fatalf("receipts = %v, %v", rs, err)
	}
	if rs[0].Read.UnixMilli() != 1_700_000_000_100 || rs[0].Delivered.UnixMilli() != 1_700_000_000_300 {
		t.Fatalf("receipt = read %d delivered %d", rs[0].Read.UnixMilli(), rs[0].Delivered.UnixMilli())
	}
}

// A delivery receipt from an encrypted chat moves the delivery mark and nothing else.
func TestDeliveryReceiptEvent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	at := time.UnixMilli(1_700_000_005_000)
	chat := types.NewJID("7", types.MessengerServer)
	receipt, ok := decodeReceipt(&events.Receipt{
		MessageSource: types.MessageSource{Chat: chat, Sender: chat},
		Timestamp:     at,
		Type:          types.ReceiptTypeDelivered,
	})
	if !ok || !receipt.delivered {
		t.Fatalf("decodeReceipt = %+v, %v", receipt, ok)
	}
	if _, err := ApplyEvent(ctx, s, Event{Type: "e2ee_receipt", receipt: &receipt}); err != nil {
		t.Fatalf("ApplyEvent: %v", err)
	}
	rs, err := s.Receipts(ctx, 7, 99)
	if err != nil || len(rs) != 1 || !rs[0].Delivered.Equal(at) || rs[0].Read.UnixMilli() != 0 {
		t.Fatalf("receipts = %+v, %v", rs, err)
	}
}
