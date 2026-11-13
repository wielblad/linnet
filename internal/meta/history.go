package meta

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
	"go.mau.fi/mautrix-meta/pkg/messagix/securehistory"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waArmadilloBackupMessage"
	"go.mau.fi/whatsmeow/proto/waConsumerApplication"
	"go.mau.fi/whatsmeow/proto/waMsgApplication"
	"google.golang.org/protobuf/proto"

	"lazyfmess/internal/session"
	"lazyfmess/internal/store"
)

const (
	historyPageSize   = 50
	historyMaxPages   = 20
	threadBatchDelay  = 2 * time.Second
	threadBatchLimit  = 40
	encryptedMaxPages = 10
	// olderEmptyPages is how many pages in a row may bring nothing to show before the reader
	// is handed back the wheel: records the program does not show still fill a page.
	olderEmptyPages = 5
)

// ErrHistoryLocked says that the older messages of an encrypted chat cannot be asked for:
// the copy that keeps them has not been unlocked with its PIN on this computer. It is not
// the beginning of the conversation, and must not be shown as one.
var ErrHistoryLocked = errors.New("encrypted history is not unlocked")

// historyDebug writes one line for every page of encrypted history, to find out what the
// server answers for a chat that will not page back.
var historyDebug = os.Getenv("MLITE_HISTORY_DEBUG") != ""

func debugHistoryPage(what string, thread store.Thread, asked int64, page *securehistory.MessageRange, decrypted, stored int) {
	if !historyDebug || page == nil {
		return
	}
	more, next := false, int64(0)
	if page.Range != nil {
		more = page.Range.HasMoreBefore != nil && *page.Range.HasMoreBefore
		next = page.Range.Before
	}
	epochs := 0
	if page.Epochs != nil {
		epochs = len(page.Epochs.Edges)
	}
	log.Warn().Str("what", what).Int64("thread", thread.ID).Int64("type", int64(thread.ThreadType)).
		Int64("asked_before_ms", asked).Int("records", len(page.Messages)).Int("decrypted", decrypted).Int("stored", stored).
		Bool("has_more_before", more).Int64("next_before_ms", next).Int("epoch_edges", epochs).Msg("history page")
}

func (c *Client) loadHistoryDevice() error {
	device, err := session.LoadHistoryDevice(c.dataDir)
	if err != nil {
		return err
	}
	c.historyMu.Lock()
	c.historyDevice = device
	c.historyMu.Unlock()
	return nil
}

func (c *Client) HasHistoryDevice() bool {
	c.historyMu.Lock()
	defer c.historyMu.Unlock()
	return c.historyDevice != nil && c.historyDevice.ID != ""
}

func (c *Client) HistoryDevice() *securehistory.Device {
	c.historyMu.Lock()
	defer c.historyMu.Unlock()
	return c.historyDevice
}

func (c *Client) NeedsHistoryPIN(ctx context.Context) (bool, error) {
	if err := c.waitReady(ctx); err != nil {
		return false, err
	}
	if c.HasHistoryDevice() {
		return false, nil
	}
	_, err := c.mx.GetSecureHistoryClient().GetBackup(ctx)
	if errors.Is(err, securehistory.ErrNoPINBackup) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (c *Client) UnlockHistory(ctx context.Context, pin string) error {
	if err := c.waitReady(ctx); err != nil {
		return err
	}
	if c == nil || c.mx == nil {
		return fmt.Errorf("unlock history: client is nil")
	}
	if c.device == nil || c.device.IdentityKey == nil {
		return fmt.Errorf("unlock history: e2ee device is not ready")
	}
	if c.HasHistoryDevice() {
		return nil
	}
	history := c.mx.GetSecureHistoryClient()
	backup, err := history.GetBackup(ctx)
	if err != nil {
		return fmt.Errorf("get history backup: %w", err)
	}
	recovery, err := history.RecoverPIN(ctx, backup, []byte(pin))
	if err != nil {
		return fmt.Errorf("recover history pin: %w", err)
	}
	virtual, err := history.GetVirtualDevice(ctx, backup, c.AccountID(), recovery.Code)
	if err != nil {
		return fmt.Errorf("get virtual history device: %w", err)
	}
	epochs, err := virtual.DeriveEpochs()
	if err != nil {
		return fmt.Errorf("derive history epochs: %w", err)
	}
	enrollment, err := virtual.PrepareDevice(backup.ID, history.RegistrationID, c.device.IdentityKey, epochs)
	if err != nil {
		return fmt.Errorf("prepare history device: %w", err)
	}
	device, err := history.AddDevice(ctx, enrollment)
	if err != nil {
		return fmt.Errorf("add history device: %w", err)
	}
	if err := session.SaveHistoryDevice(c.dataDir, device); err != nil {
		return err
	}
	c.historyMu.Lock()
	c.historyDevice = device
	c.historyMu.Unlock()
	return nil
}

func (c *Client) saveHistoryDevice(device *securehistory.Device) error {
	if err := session.SaveHistoryDevice(c.dataDir, device); err != nil {
		return err
	}
	c.historyMu.Lock()
	c.historyDevice = device
	c.historyMu.Unlock()
	return nil
}

func (c *Client) SyncInboxThreads(ctx context.Context, s *store.Store) error {
	if err := c.waitReady(ctx); err != nil {
		return err
	}
	if c == nil || c.mx == nil || s == nil {
		return nil
	}
	var prevMin int64
	for batch := 0; batch < threadBatchLimit; batch++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		keyStore, tbl, err := c.mx.FetchMoreThreads(ctx, 1)
		if err != nil {
			return fmt.Errorf("fetch more threads: %w", err)
		}
		if tbl == nil {
			return nil
		}
		if err := ApplyTable(ctx, s, tbl); err != nil {
			return err
		}
		if keyStore == nil || !keyStore.HasMoreBefore || keyStore.MinThreadKey == prevMin {
			return nil
		}
		prevMin = keyStore.MinThreadKey
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(threadBatchDelay):
		}
	}
	return nil
}

func (c *Client) SyncThreadHistory(ctx context.Context, s *store.Store, thread store.Thread) error {
	if err := c.waitReady(ctx); err != nil {
		return err
	}
	if c == nil || s == nil || thread.ID == 0 {
		return nil
	}
	if table.ThreadType(thread.ThreadType).IsWhatsApp() {
		return c.syncEncryptedThread(ctx, s, thread)
	}
	return c.syncSocketThread(ctx, s, thread)
}

// SyncThreadNewest brings a conversation up to date when it is opened: the newest page of
// an encrypted chat, or the first page of a chat the store knows nothing about yet. Older
// messages are fetched page by page with FetchOlder, as the reader scrolls back.
func (c *Client) SyncThreadNewest(ctx context.Context, s *store.Store, thread store.Thread) error {
	if err := c.waitReady(ctx); err != nil {
		return err
	}
	if c == nil || s == nil || thread.ID == 0 {
		return nil
	}
	if table.ThreadType(thread.ThreadType).IsWhatsApp() {
		return c.syncEncryptedPages(ctx, s, thread, 1)
	}
	if _, err := s.OldestMessage(ctx, thread.ID); errors.Is(err, sql.ErrNoRows) {
		_, err := c.fetchOlderSocket(ctx, s, thread)
		return err
	} else if err != nil {
		return err
	}
	return nil
}

// FetchOlder fetches one page of messages older than the oldest one stored for the thread
// and reports whether the server has more before it.
func (c *Client) FetchOlder(ctx context.Context, s *store.Store, thread store.Thread) (bool, error) {
	if err := c.waitReady(ctx); err != nil {
		return false, err
	}
	if c == nil || s == nil || thread.ID == 0 {
		return false, nil
	}
	if table.ThreadType(thread.ThreadType).IsWhatsApp() {
		return c.fetchOlderEncrypted(ctx, s, thread)
	}
	return c.fetchOlderSocket(ctx, s, thread)
}

func (c *Client) fetchOlderEncrypted(ctx context.Context, s *store.Store, thread store.Thread) (bool, error) {
	device := c.HistoryDevice()
	if device == nil || device.ID == "" {
		return false, ErrHistoryLocked
	}
	oldestMS := time.Now().Add(time.Minute).UnixMilli()
	oldest, err := s.OldestMessage(ctx, thread.ID)
	switch {
	case err == nil:
		oldestMS = oldest.Timestamp.UnixMilli()
	case errors.Is(err, sql.ErrNoRows):
	default:
		return false, err
	}
	// Pages are followed by the place the server names for the next one, as the library
	// this is built on does. Asking "before the oldest stored message" every time stood
	// still whenever a page held nothing the program shows, and that was taken for the
	// beginning of the conversation.
	timestamp := oldestMS
	c.historyMu.Lock()
	if cur, ok := c.historyCursor[thread.ID]; ok && cur > 0 && cur < timestamp {
		timestamp = cur
	}
	c.historyMu.Unlock()
	remember := func(at int64) {
		c.historyMu.Lock()
		if c.historyCursor == nil {
			c.historyCursor = map[int64]int64{}
		}
		if at > 0 {
			c.historyCursor[thread.ID] = at
		} else {
			delete(c.historyCursor, thread.ID)
		}
		c.historyMu.Unlock()
	}
	threadID := strconv.FormatInt(thread.ID, 10)
	isGroup := table.ThreadType(thread.ThreadType) == table.ENCRYPTED_OVER_WA_GROUP
	for page := 0; page < olderEmptyPages; page++ {
		if page > 0 {
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case <-time.After(300 * time.Millisecond):
			}
		}
		rangeResp, err := c.mx.GetSecureHistoryClient().FetchMessages(ctx, device, threadID, true, timestamp, historyPageSize)
		if err != nil {
			return false, fmt.Errorf("fetch encrypted history %d: %w", thread.ID, err)
		}
		if rangeResp.Epochs != nil && len(rangeResp.Epochs.Edges) > 0 {
			updated, err := device.WithEpochs(rangeResp.Epochs)
			if err != nil {
				return false, fmt.Errorf("apply history epochs: %w", err)
			}
			if err := c.saveHistoryDevice(updated); err != nil {
				return false, err
			}
			device = updated
		}
		messages, err := rangeResp.Decrypt(device, threadID, c.AccountID(), isGroup)
		if err != nil {
			return false, fmt.Errorf("decrypt encrypted history %d: %w", thread.ID, err)
		}
		stored := 0
		if err := s.InTx(ctx, func(s *store.Store) error {
			n, err := storeArchivePage(ctx, s, thread, messages)
			stored = n
			return err
		}); err != nil {
			return false, err
		}
		debugHistoryPage("older", thread, timestamp, rangeResp, len(messages), stored)
		more, next := false, int64(0)
		if rangeResp.Range != nil {
			more = rangeResp.Range.HasMoreBefore != nil && *rangeResp.Range.HasMoreBefore
			next = rangeResp.Range.Before
		}
		moved := false
		if after, err := s.OldestMessage(ctx, thread.ID); err == nil && after.Timestamp.UnixMilli() < oldestMS {
			moved = true
		}
		verdict, cursor, again := olderVerdict(more, next, timestamp, moved)
		remember(cursor)
		if !again {
			return verdict, nil
		}
		timestamp = next
	}
	return true, nil // pages of nothing to show, and more behind them: the reader may go on
}

// olderVerdict decides what one page of older encrypted messages means: whether the
// conversation has more before it, where the next page starts (0 for "ask before the oldest
// stored message"), and whether the next page should be fetched at once because this one
// brought nothing to show.
func olderVerdict(more bool, next, asked int64, moved bool) (verdict bool, cursor int64, again bool) {
	switch {
	case !more: // the server says this was the first page of the conversation
		return false, 0, false
	case next <= 0 || next >= asked: // the server did not move back: the same page would come again
		return moved, 0, false
	case moved:
		return true, next, false
	}
	return true, next, true
}

func (c *Client) fetchOlderSocket(ctx context.Context, s *store.Store, thread store.Thread) (bool, error) {
	var task socket.Task
	refID := ""
	oldest, err := s.OldestMessage(ctx, thread.ID)
	switch {
	case err == nil:
		refID = oldest.ID
		task = &socket.FetchMessagesTask{
			ThreadKey:            thread.ID,
			Direction:            0,
			ReferenceTimestampMs: oldest.Timestamp.UnixMilli(),
			ReferenceMessageId:   refID,
			SyncGroup:            1,
			Cursor:               c.mx.GetCursor(1),
		}
	case errors.Is(err, sql.ErrNoRows):
		task = &socket.CreateThreadTask{ThreadFBID: thread.ID, SyncGroup: 1}
	default:
		return false, err
	}
	tbl, err := c.mx.ExecuteTasks(ctx, task)
	if err != nil {
		return false, fmt.Errorf("fetch socket history %d: %w", thread.ID, err)
	}
	if err := ApplyTable(ctx, s, tbl); err != nil {
		return false, err
	}
	// The answer does not always say whether more is left, and part of a page can arrive a
	// moment later over the socket. So the page counts as "there may be more" when it moved
	// the oldest stored message back, and as the beginning when it brought nothing older.
	for try := 0; try < 4; try++ {
		after, err := s.OldestMessage(ctx, thread.ID)
		if err == nil && after.ID != refID {
			return true, nil
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(600 * time.Millisecond):
		}
	}
	return false, nil
}

func (c *Client) SyncAllEncryptedHistory(ctx context.Context, s *store.Store) error {
	if err := c.waitReady(ctx); err != nil {
		return err
	}
	threads, err := s.ListThreads(ctx)
	if err != nil {
		return err
	}
	for _, thread := range threads {
		if !table.ThreadType(thread.ThreadType).IsWhatsApp() {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := c.syncEncryptedThread(ctx, s, thread); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return nil
}

// SyncRecentEncrypted fetches the newest page of every encrypted chat, so that the list
// shows the real last message of each without opening them one by one.
func (c *Client) SyncRecentEncrypted(ctx context.Context, s *store.Store) error {
	if err := c.waitReady(ctx); err != nil {
		return err
	}
	threads, err := s.ListThreads(ctx)
	if err != nil {
		return err
	}
	failed := 0
	for _, thread := range threads {
		if !table.ThreadType(thread.ThreadType).IsWhatsApp() {
			continue
		}
		if err := c.syncEncryptedPages(ctx, s, thread, 1); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// One chat that cannot be fetched (not in the copy, a key that is missing) used to
			// end the pass, and every chat after it kept an old last message.
			log.Warn().Err(err).Msg("newest page of an encrypted chat")
			failed++
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	if failed > 0 {
		log.Warn().Int("chats", failed).Msg("encrypted chats whose newest page could not be fetched")
	}
	return nil
}

func (c *Client) syncEncryptedThread(ctx context.Context, s *store.Store, thread store.Thread) error {
	return c.syncEncryptedPages(ctx, s, thread, encryptedMaxPages)
}

func (c *Client) syncEncryptedPages(ctx context.Context, s *store.Store, thread store.Thread, maxPages int) error {
	device := c.HistoryDevice()
	if device == nil || device.ID == "" {
		return nil
	}
	threadID := strconv.FormatInt(thread.ID, 10)
	timestamp := time.Now().Add(time.Minute).UnixMilli()
	isGroup := table.ThreadType(thread.ThreadType) == table.ENCRYPTED_OVER_WA_GROUP
	for page := 0; page < maxPages; page++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rangeResp, err := c.mx.GetSecureHistoryClient().FetchMessages(ctx, device, threadID, true, timestamp, historyPageSize)
		if err != nil {
			return fmt.Errorf("fetch encrypted history %d: %w", thread.ID, err)
		}
		if rangeResp.Epochs != nil && len(rangeResp.Epochs.Edges) > 0 {
			updated, err := device.WithEpochs(rangeResp.Epochs)
			if err != nil {
				return fmt.Errorf("apply history epochs: %w", err)
			}
			if err := c.saveHistoryDevice(updated); err != nil {
				return err
			}
			device = updated
		}
		messages, err := rangeResp.Decrypt(device, threadID, c.AccountID(), isGroup)
		if err != nil {
			return fmt.Errorf("decrypt encrypted history %d: %w", thread.ID, err)
		}
		stored := 0
		if err := s.InTx(ctx, func(s *store.Store) error {
			n, err := storeArchivePage(ctx, s, thread, messages)
			stored = n
			return err
		}); err != nil {
			return err
		}
		debugHistoryPage("newest", thread, timestamp, rangeResp, len(messages), stored)
		if rangeResp.Range == nil || rangeResp.Range.HasMoreBefore == nil || !*rangeResp.Range.HasMoreBefore {
			return nil
		}
		next := rangeResp.Range.Before
		if next >= timestamp {
			return nil
		}
		timestamp = next
	}
	return nil
}

func (c *Client) syncSocketThread(ctx context.Context, s *store.Store, thread store.Thread) error {
	var refTS int64
	var refID string
	oldest, err := s.OldestMessage(ctx, thread.ID)
	switch {
	case err == nil:
		refTS = oldest.Timestamp.UnixMilli()
		refID = oldest.ID
	case errors.Is(err, sql.ErrNoRows):
	default:
		return err
	}
	for page := 0; page < historyMaxPages; page++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var task socket.Task
		if refID == "" {
			task = &socket.CreateThreadTask{ThreadFBID: thread.ID, SyncGroup: 1}
		} else {
			task = &socket.FetchMessagesTask{
				ThreadKey:            thread.ID,
				Direction:            0,
				ReferenceTimestampMs: refTS,
				ReferenceMessageId:   refID,
				SyncGroup:            1,
				Cursor:               c.mx.GetCursor(1),
			}
		}
		tbl, err := c.mx.ExecuteTasks(ctx, task)
		if err != nil {
			return fmt.Errorf("fetch socket history %d: %w", thread.ID, err)
		}
		if err := ApplyTable(ctx, s, tbl); err != nil {
			return err
		}
		upsert, _ := tbl.WrapMessages()
		um := upsert[thread.ID]
		if um == nil || um.Range == nil || !um.Range.HasMoreBefore || um.Range.MinMessageId == "" || um.Range.MinMessageId == refID {
			return nil
		}
		refTS = um.Range.MinTimestampMs
		refID = um.Range.MinMessageId
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return nil
}

// storeArchivePage stores the messages of a page that the program can show, and says how
// many those were: a page may be full of records and still bring nothing to read.
func storeArchivePage(ctx context.Context, s *store.Store, thread store.Thread, messages []*securehistory.DecryptedMessage) (int, error) {
	var newestPreview string
	var newestTS time.Time
	stored := 0
	for _, msg := range messages {
		plain, ok := archivePlainMessage(msg)
		if !ok {
			continue
		}
		stored++
		if err := s.EnsureThread(ctx, thread.ID); err != nil {
			return stored, err
		}
		if err := s.UpsertMessage(ctx, store.Message{
			ID:              plain.ID,
			ThreadID:        thread.ID,
			SenderID:        plain.SenderID,
			Text:            plain.Text,
			Timestamp:       time.UnixMilli(plain.TimestampMS),
			MediaKind:       plain.MediaKind,
			MediaMime:       plain.MediaMime,
			MediaName:       plain.MediaName,
			MediaRef:        plain.MediaRef,
			MediaDurationMs: plain.DurationMs,
			LinkURL:         plain.LinkURL,
			LinkTitle:       plain.LinkTitle,
			LinkRef:         plain.LinkRef,
		}); err != nil {
			return stored, err
		}
		if plain.TimestampMS >= newestTS.UnixMilli() {
			newestTS = time.UnixMilli(plain.TimestampMS)
			newestPreview = plain.Text
			if newestPreview == "" && plain.MediaKind != "" {
				newestPreview = "[" + plain.MediaKind + "]"
			}
		}
	}
	if newestPreview == "" {
		return stored, nil
	}
	return stored, s.UpsertThread(ctx, store.Thread{
		ID:          thread.ID,
		Title:       thread.Title,
		LastPreview: newestPreview,
		ThreadType:  thread.ThreadType,
		UpdatedAt:   newestTS,
	})
}

type plainArchiveMessage struct {
	ID          string
	SenderID    int64
	Text        string
	TimestampMS int64

	MediaKind, MediaMime, MediaRef, MediaName string
	DurationMs                                int64

	LinkURL, LinkTitle, LinkRef string
}

func archivePlainMessage(msg *securehistory.DecryptedMessage) (plainArchiveMessage, bool) {
	if msg == nil || msg.Message == nil || msg.Message.Message == nil {
		return plainArchiveMessage{}, false
	}
	meta := msg.Message.Message.GetMetadata()
	if meta == nil || meta.GetMessageID() == "" || meta.GetSenderID() == "" {
		return plainArchiveMessage{}, false
	}
	senderID, err := metaid.ParseIDFromString(meta.GetSenderID())
	if err != nil {
		return plainArchiveMessage{}, false
	}
	// Archived files are fetched from fresh paths delivered next to the message; the picture
	// of a link preview is looked up there the same way as an attachment.
	paths := make(map[string]string, len(msg.Attachments))
	for _, att := range msg.Attachments {
		if att.ObjectID != "" && att.URL != "" {
			paths[att.ObjectID] = att.URL
		}
	}
	body, ok := encryptedContent(archiveContent(msg.Message.Message), paths)
	if !ok {
		return plainArchiveMessage{}, false
	}
	att, link := body.Att, body.Link
	return plainArchiveMessage{
		ID:          meta.GetMessageID(),
		SenderID:    senderID,
		Text:        body.Text,
		TimestampMS: msg.Message.TimestampMS,
		MediaKind:   att.Kind,
		MediaMime:   att.Mime,
		MediaRef:    att.Ref,
		MediaName:   att.Name,
		DurationMs:  att.DurationMs,
		LinkURL:     link.URL,
		LinkTitle:   link.Title,
		LinkRef:     link.Ref,
	}, true
}

func archiveMessageText(message *waArmadilloBackupMessage.BackupMessage) (string, bool) {
	payload, ok := message.GetPayload().(*waArmadilloBackupMessage.BackupMessage_EncryptedTransportMessage)
	if !ok {
		return "", false
	}
	var application waMsgApplication.MessageApplication
	if err := proto.Unmarshal(payload.EncryptedTransportMessage, &application); err != nil {
		return "", false
	}
	decoded, err := whatsmeow.DecodeFBApplication(&application)
	if err != nil {
		return "", false
	}
	return consumerText(decoded.GetConsumerApplication())
}

// archiveContent unpacks what is stored inside an archived record: a chat message
// (ConsumerApplication) or a share (Armadillo), as taken by encryptedContent.
func archiveContent(message *waArmadilloBackupMessage.BackupMessage) any {
	payload, ok := message.GetPayload().(*waArmadilloBackupMessage.BackupMessage_EncryptedTransportMessage)
	if !ok {
		return nil
	}
	var application waMsgApplication.MessageApplication
	if err := proto.Unmarshal(payload.EncryptedTransportMessage, &application); err != nil {
		return nil
	}
	decoded, err := whatsmeow.DecodeFBApplication(&application)
	if err != nil {
		return nil
	}
	return decoded.Message
}

func consumerText(msg *waConsumerApplication.ConsumerApplication) (string, bool) {
	if msg == nil || msg.GetPayload() == nil {
		return "", false
	}
	content := msg.GetPayload().GetContent()
	if content == nil {
		return "", false
	}
	switch typed := content.GetContent().(type) {
	case *waConsumerApplication.ConsumerApplication_Content_MessageText:
		if typed.MessageText == nil {
			return "", false
		}
		text := typed.MessageText.GetText()
		return text, text != ""
	case *waConsumerApplication.ConsumerApplication_Content_ExtendedTextMessage:
		if typed.ExtendedTextMessage == nil || typed.ExtendedTextMessage.GetText() == nil {
			return "", false
		}
		text := typed.ExtendedTextMessage.GetText().GetText()
		return text, text != ""
	case *waConsumerApplication.ConsumerApplication_Content_ImageMessage:
		if typed.ImageMessage != nil && typed.ImageMessage.GetCaption() != nil {
			if text := typed.ImageMessage.GetCaption().GetText(); text != "" {
				return text, true
			}
		}
		return "[image]", true
	case *waConsumerApplication.ConsumerApplication_Content_VideoMessage:
		if typed.VideoMessage != nil && typed.VideoMessage.GetCaption() != nil {
			if text := typed.VideoMessage.GetCaption().GetText(); text != "" {
				return text, true
			}
		}
		return "[video]", true
	case *waConsumerApplication.ConsumerApplication_Content_StickerMessage:
		return "[sticker]", true
	case *waConsumerApplication.ConsumerApplication_Content_AudioMessage:
		return "[audio]", true
	default:
		return "", false
	}
}
