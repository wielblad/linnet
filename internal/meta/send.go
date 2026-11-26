package meta

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/methods"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waConsumerApplication"
	"go.mau.fi/whatsmeow/proto/waMsgApplication"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"lazyfmess/internal/store"
)

type SendResult struct {
	MessageID string
	Timestamp time.Time
}

func (c *Client) SendText(ctx context.Context, thread store.Thread, text string, replyTo store.Message) (SendResult, error) {
	if err := c.waitReady(ctx); err != nil {
		return SendResult{}, err
	}
	text = trimMessage(text)
	if text == "" {
		return SendResult{}, fmt.Errorf("message is empty")
	}
	if table.ThreadType(thread.ThreadType).IsWhatsApp() {
		return c.sendE2EEText(ctx, thread, text, replyTo)
	}
	return c.sendLightspeedText(ctx, thread.ID, text, replyTo)
}

// ThumbEmoji is the emoji of a conversation's button until its people choose another.
const ThumbEmoji = "\U0001F44D"

// The thumb is not sent as text: Messenger has three stickers for it, one for each size.
var thumbStickers = [3]int64{369239263222822, 369239343222814, 369239383222810}

// thumbSize tells which size of the thumb a sticker is, 0 when it is some other sticker.
func thumbSize(sticker int64) int64 {
	for i, id := range thumbStickers {
		if id == sticker {
			return int64(i + 1)
		}
	}
	return 0
}

// HotSizes reports whether an emoji sent with the button can be given a size in this
// conversation. Encrypted chats carry it as plain text: their messages have no place for
// a size.
func HotSizes(thread store.Thread) bool {
	return !table.ThreadType(thread.ThreadType).IsWhatsApp()
}

// SendEmoji sends the emoji of the conversation's button in one of three sizes, 1 to 3.
func (c *Client) SendEmoji(ctx context.Context, thread store.Thread, emoji string, size int64) (SendResult, error) {
	if err := c.waitReady(ctx); err != nil {
		return SendResult{}, err
	}
	if emoji == "" {
		emoji = ThumbEmoji
	}
	size = min(max(size, 1), 3)
	if !HotSizes(thread) {
		return c.sendE2EEText(ctx, thread, emoji, store.Message{})
	}
	if emoji == ThumbEmoji {
		return c.sendLightspeed(ctx, &socket.SendMessageTask{
			ThreadId:  thread.ID,
			SendType:  table.STICKER,
			StickerId: thumbStickers[size-1],
		})
	}
	return c.sendLightspeed(ctx, &socket.SendMessageTask{
		ThreadId:     thread.ID,
		SendType:     table.TEXT,
		Text:         emoji,
		HotEmojiSize: int32(size),
	})
}

func (c *Client) sendLightspeedText(ctx context.Context, threadID int64, text string, replyTo store.Message) (SendResult, error) {
	task := &socket.SendMessageTask{
		ThreadId: threadID,
		SendType: table.TEXT,
		Text:     text,
	}
	if replyTo.ID != "" {
		task.ReplyMetaData = &socket.ReplyMetaData{
			ReplyMessageId:  replyTo.ID,
			ReplySourceType: 1,
			ReplySender:     replyTo.SenderID,
		}
	}
	return c.sendLightspeed(ctx, task)
}

func (c *Client) sendLightspeed(ctx context.Context, task *socket.SendMessageTask) (SendResult, error) {
	if err := c.mx.WaitUntilCanSendMessages(ctx, 15*time.Second); err != nil {
		return SendResult{}, fmt.Errorf("wait to send: %w", err)
	}
	otid := methods.GenerateEpochID()
	task.Otid = otid
	task.Source = table.MESSENGER_INBOX_IN_THREAD
	task.InitiatingSource = table.FACEBOOK_INBOX
	task.SyncGroup = 1
	resp, err := c.mx.ExecuteTasks(ctx, task)
	if err != nil {
		return SendResult{}, fmt.Errorf("execute send task: %w", err)
	}
	msgID, ts := extractSendResult(resp, otid)
	if msgID == "" {
		for _, failed := range resp.LSMarkOptimisticMessageFailed {
			return SendResult{}, fmt.Errorf("send rejected: %s", failed.Message)
		}
		for _, failed := range resp.LSHandleFailedTask {
			return SendResult{}, fmt.Errorf("send rejected: %s", failed.Message)
		}
		return SendResult{}, fmt.Errorf("send response missing message id")
	}
	if ts.IsZero() {
		ts = time.Now()
	}
	return SendResult{MessageID: msgID, Timestamp: ts}, nil
}

func (c *Client) sendE2EEText(ctx context.Context, thread store.Thread, text string, replyTo store.Message) (SendResult, error) {
	if c.e2ee == nil {
		return SendResult{}, fmt.Errorf("e2ee client is not connected")
	}
	otid := methods.GenerateEpochID()
	msgID := strconv.FormatInt(otid, 10)
	chat := chatJID(thread)
	payload := consumerContent(&waConsumerApplication.ConsumerApplication_Content{
		Content: &waConsumerApplication.ConsumerApplication_Content_MessageText{
			MessageText: &waCommon.MessageText{Text: proto.String(text)},
		},
	})
	var metadata *waMsgApplication.MessageApplication_Metadata
	if replyTo.ID != "" {
		participant := types.JID{User: strconv.FormatInt(replyTo.SenderID, 10), Server: types.MessengerServer}
		metadata = &waMsgApplication.MessageApplication_Metadata{
			QuotedMessage: &waMsgApplication.MessageApplication_Metadata_QuotedMessage{
				StanzaID:    proto.String(replyTo.ID),
				Participant: proto.String(participant.String()),
			},
		}
	}
	resp, err := c.e2ee.SendFBMessage(ctx, chat, payload, metadata, whatsmeow.SendRequestExtra{ID: msgID})
	if err != nil {
		return SendResult{}, fmt.Errorf("send e2ee message: %w", err)
	}
	ts := resp.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	return SendResult{MessageID: msgID, Timestamp: ts}, nil
}

func extractSendResult(resp *table.LSTable, otid int64) (string, time.Time) {
	if resp == nil {
		return "", time.Time{}
	}
	otidStr := strconv.FormatInt(otid, 10)
	for _, replace := range resp.LSReplaceOptimsiticMessage {
		if replace == nil {
			continue
		}
		if replace.OfflineThreadingId == otidStr && replace.MessageId != "" {
			if ts, err := methods.ParseMessageID(replace.MessageId); err == nil {
				return replace.MessageId, time.UnixMilli(ts)
			}
			return replace.MessageId, time.Time{}
		}
	}
	return "", time.Time{}
}

func trimMessage(text string) string {
	for len(text) > 0 && (text[0] == ' ' || text[0] == '\n' || text[0] == '\t' || text[0] == '\r') {
		text = text[1:]
	}
	for len(text) > 0 {
		last := text[len(text)-1]
		if last != ' ' && last != '\n' && last != '\t' && last != '\r' {
			break
		}
		text = text[:len(text)-1]
	}
	return text
}
