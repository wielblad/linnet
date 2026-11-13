//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	signallog "go.mau.fi/libsignal/logger"

	"lazyfmess/internal/meta"
	"lazyfmess/internal/session"
	"lazyfmess/internal/store"
)

type quietSignalLog struct{}

func (quietSignalLog) Debug(string, string)   {}
func (quietSignalLog) Info(string, string)    {}
func (quietSignalLog) Warning(string, string) {}
func (quietSignalLog) Error(caller, message string) {
	log.Error().Str("caller", caller).Msg(message)
}
func (quietSignalLog) Configure(string) {}

type backend struct {
	ctx    context.Context
	cancel context.CancelFunc
	dir    string
	db     *store.Store
	logW   io.Writer
	logF   *os.File

	mu     sync.Mutex
	client *meta.Client
	status string
	// note is a short message shown instead of the connection state until noteUntil.
	note      string
	noteUntil time.Time

	people    []meta.Person
	peopleFor string
	searching string

	hwnd           atomic.Uintptr
	selfOverride   int64
	noRead         bool // never tell Messenger that something was read (for checks)
	refreshPending atomic.Bool
	mediaRetry     atomic.Bool // pictures that failed may be tried again (the connection came back)
	armed          atomic.Bool
	pinNeeded      atomic.Bool // the encrypted history waits for Messenger's PIN
	done           chan struct{}
}

func newBackend(dir string) (*backend, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, logName), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	zerolog.SetGlobalLevel(zerolog.WarnLevel)
	log.Logger = zerolog.New(f).With().Timestamp().Logger()
	var sl signallog.Loggable = quietSignalLog{}
	signallog.Setup(&sl)

	ctx, cancel := context.WithCancel(context.Background())
	db, err := store.Open(ctx, dir)
	if err != nil {
		cancel()
		f.Close()
		return nil, err
	}
	_ = db.RepairThreadActivity(ctx)
	return &backend{ctx: ctx, cancel: cancel, dir: dir, db: db, logW: f, logF: f, done: make(chan struct{}), status: "Uruchamianie…"}, nil
}

func (b *backend) start(hwnd uintptr) {
	b.hwnd.Store(hwnd)
	go func() {
		defer close(b.done)
		b.run()
	}()
}

func (b *backend) stop() {
	b.cancel()
	select {
	case <-b.done:
	case <-time.After(5 * time.Second):
	}
	b.mu.Lock()
	client := b.client
	b.client = nil
	b.mu.Unlock()
	if client != nil {
		client.Disconnect()
	}
	b.db.Close()
	b.logF.Close()
}

// historyLocked reports that the older messages of the encrypted conversations are still
// behind Messenger's PIN: without it this computer sees only what arrives from now on.
func (b *backend) historyLocked() bool { return b.pinNeeded.Load() }

// checkHistoryPIN asks, once connected, whether this account keeps a PIN-locked history.
func (b *backend) checkHistoryPIN() {
	client := b.current()
	if client == nil {
		return
	}
	need, err := client.NeedsHistoryPIN(b.ctx)
	if err != nil {
		log.Warn().Err(err).Msg("history pin")
		return
	}
	b.pinNeeded.Store(need)
	b.requestRefresh()
}

// unlockHistory hands Messenger's PIN over once, to fetch the key to the stored history. The
// PIN goes to Meta and is kept nowhere on this computer.
func (b *backend) unlockHistory(pin string) {
	client := b.current()
	if client == nil || pin == "" {
		return
	}
	go func() {
		if err := client.UnlockHistory(b.ctx, pin); err != nil {
			log.Warn().Err(err).Msg("unlock history")
			b.notice("Nie udało się odblokować historii. Sprawdź PIN i spróbuj jeszcze raz.")
			return
		}
		b.pinNeeded.Store(false)
		b.notice("Historia odblokowana. Pobieram starsze wiadomości…")
		if err := client.SyncRecentEncrypted(b.ctx, b.db); err != nil {
			log.Warn().Err(err).Msg("sync recent encrypted after unlock")
		}
		_ = b.db.RepairThreadActivity(b.ctx)
		b.requestRefresh()
	}()
}

// signOut ends the connection but leaves the program running, so that the sign-in screen can
// come next. The conversations already on this computer stay where they are.
func (b *backend) signOut() {
	b.mu.Lock()
	client := b.client
	b.client = nil
	b.mu.Unlock()
	if client != nil {
		client.Disconnect()
	}
	b.armed.Store(false)
	b.setStatus("Wymagane logowanie")
}

func (b *backend) setStatus(s string) {
	b.mu.Lock()
	b.status = s
	b.mu.Unlock()
	if h := b.hwnd.Load(); h != 0 {
		postMsg(h, wmAppStatus, 0, 0)
	}
}

// askLogin tells the window to show the sign-in screen.
func (b *backend) askLogin(expired bool) {
	if h := b.hwnd.Load(); h != 0 {
		reason := uintptr(0)
		if expired {
			reason = loginExpired
		}
		postMsg(h, wmAppLogin, reason, 0)
	}
}

// notice shows a message about one failed action for a few seconds.
func (b *backend) notice(s string) {
	b.mu.Lock()
	b.note, b.noteUntil = s, time.Now().Add(8*time.Second)
	b.mu.Unlock()
	if h := b.hwnd.Load(); h != 0 {
		postMsg(h, wmAppStatus, 0, 0)
	}
}

// act runs one action on the connection in the background and reports a failure as a notice.
func (b *backend) act(failed string, fn func(c *meta.Client) error) bool {
	client := b.current()
	if client == nil {
		b.notice(failed + ": brak połączenia.")
		return false
	}
	go func() {
		if err := fn(client); err != nil {
			log.Warn().Err(err).Msg(failed)
			b.notice(failed + ": " + err.Error())
		}
		b.requestRefresh()
	}()
	return true
}

// react sets the account's reaction on a message; an empty emoji takes it back.
func (b *backend) react(thread store.Thread, m store.Message, emoji string) bool {
	return b.act("Nie udało się zmienić reakcji", func(c *meta.Client) error { return c.React(b.ctx, b.db, thread, m, emoji) })
}

func (b *backend) edit(thread store.Thread, m store.Message, text string) bool {
	return b.act("Nie udało się zmienić wiadomości", func(c *meta.Client) error { return c.Edit(b.ctx, b.db, thread, m, text) })
}

// unsend removes a message for everyone in the conversation.
func (b *backend) unsend(thread store.Thread, m store.Message) bool {
	return b.act("Nie udało się usunąć wiadomości", func(c *meta.Client) error { return c.Unsend(b.ctx, b.db, thread, m) })
}

// fetchEncrypted downloads and decrypts an attachment of an encrypted chat into a file.
func (b *backend) fetchEncrypted(ref, path string) error {
	client := b.current()
	if client == nil {
		return errors.New("no connection")
	}
	ctx, cancel := context.WithTimeout(b.ctx, 60*time.Second)
	defer cancel()
	data, err := client.DownloadEncryptedMedia(ctx, ref)
	if err != nil {
		log.Warn().Err(err).Msg("encrypted media")
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// dropLocal discards a message that never left this computer.
func (b *backend) dropLocal(m store.Message) {
	_ = b.db.DeleteMessage(b.ctx, m.ID)
	b.requestRefresh()
}

func (b *backend) getStatus() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.note != "" && time.Now().Before(b.noteUntil) {
		return b.note
	}
	return b.status
}

func (b *backend) requestRefresh() {
	if b.refreshPending.CompareAndSwap(false, true) {
		if h := b.hwnd.Load(); h != 0 {
			postMsg(h, wmAppRefresh, 0, 0)
		}
	}
}

func (b *backend) current() *meta.Client {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.client
}

func (b *backend) accountID() int64 {
	if c := b.current(); c != nil {
		return c.AccountID()
	}
	return b.selfOverride
}

// retry sends a failed message again and drops the failed copy.
func (b *backend) retry(thread store.Thread, m store.Message) {
	_ = b.db.DeleteMessage(b.ctx, m.ID)
	b.send(thread, m.Text, store.Message{ID: m.ReplyToID, Text: m.ReplyText, SenderID: m.ReplySenderID})
}

func (b *backend) run() {
	creds, err := session.LoadCookies(b.dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		b.setStatus("Wymagane logowanie")
		b.askLogin(false)
		return
	case err != nil:
		b.setStatus("Nie można odczytać sesji: " + err.Error())
		return
	}
	client, err := meta.New(creds, b.dir, b.logW, b.onEvent)
	if err != nil {
		b.setStatus("Sesja nie nadaje się do użycia: " + err.Error())
		return
	}
	b.mu.Lock()
	b.client = client
	b.mu.Unlock()

	b.setStatus("Łączenie z Messengerem…")
	if err := client.Connect(b.ctx); err != nil {
		if meta.IsLoggedOut(err) {
			b.setStatus("Sesja wygasła")
			b.askLogin(true)
		} else {
			b.setStatus("Błąd połączenia: " + err.Error())
		}
		log.Error().Err(err).Msg("connect")
		return
	}
	if err := meta.ApplyTable(b.ctx, b.db, client.InitialTable()); err != nil {
		log.Error().Err(err).Msg("apply initial table")
	}
	b.requestRefresh()
	b.setStatus("Połączono · synchronizacja rozmów…")
	if err := client.SyncInboxThreads(b.ctx, b.db); err != nil {
		log.Error().Err(err).Msg("sync inbox")
	}
	_ = b.db.RepairThreadActivity(b.ctx)
	b.armed.Store(true)
	b.requestRefresh()
	b.setStatus("Połączono")
	b.checkHistoryPIN() // without the PIN the encrypted conversations start empty here
	// Encrypted chats: the server does not know their last message, so fetch the newest
	// page of each in the background and let the list reorder itself.
	if err := client.SyncRecentEncrypted(b.ctx, b.db); err != nil {
		log.Warn().Err(err).Msg("sync recent encrypted")
	}
	_ = b.db.RepairThreadActivity(b.ctx)
	b.requestRefresh()
	debug.FreeOSMemory() // the first synchronisation is over; hand its working memory back
}

func (b *backend) onEvent(ctx context.Context, evt meta.Event) {
	switch {
	case evt.Type == "permanent_error" && meta.IsLoggedOut(evt.Err):
		b.setStatus("Sesja wygasła")
		b.askLogin(true)
		return
	case evt.Type == "permanent_error" && evt.Err != nil:
		b.setStatus("Błąd: " + evt.Err.Error())
	case evt.Type == "transient_disconnect":
		b.setStatus("Rozłączono, ponawiam…")
	case evt.Type == "e2ee_connected":
		// Encrypted chats are back after a break: catch up on what arrived meanwhile and let
		// pictures that could not be fetched be tried again.
		log.Warn().Msg("encrypted chats reconnected")
		b.mediaRetry.Store(true)
		if client := b.current(); client != nil {
			go func() {
				if err := client.SyncRecentEncrypted(b.ctx, b.db); err != nil {
					log.Warn().Err(err).Msg("catch up after reconnect")
				}
				b.requestRefresh()
			}()
		}
		b.requestRefresh()
		return
	case evt.Type == "reconnected" || evt.Type == "connected":
		if b.armed.Load() {
			b.setStatus("Połączono")
		}
	}
	changed, err := meta.ApplyEvent(ctx, b.db, evt)
	if err != nil {
		log.Error().Err(err).Str("event", evt.Type).Msg("apply event")
		return
	}
	if changed {
		b.requestRefresh()
	}
}

// openThread loads older history for a thread and marks it read.
func (b *backend) openThread(thread store.Thread, markRead bool) {
	client := b.current()
	if client == nil {
		return
	}
	go func() {
		if err := client.SyncThreadNewest(b.ctx, b.db, thread); err != nil {
			log.Warn().Err(err).Msg("sync thread history")
		}
		if markRead && !b.noRead {
			if fresh, err := b.db.GetThread(b.ctx, thread.ID); err == nil {
				if err := client.MarkRead(b.ctx, b.db, fresh); err != nil {
					log.Warn().Err(err).Msg("mark read")
				}
			}
		}
		b.requestRefresh()
	}()
}

// searchPeople asks Messenger for people and groups matching the query.
func (b *backend) searchPeople(q string) {
	client := b.current()
	if client == nil || !b.armed.Load() {
		return
	}
	b.mu.Lock()
	b.searching = q
	b.mu.Unlock()
	b.requestRefresh()
	go func() {
		ctx, cancel := context.WithTimeout(b.ctx, 20*time.Second)
		defer cancel()
		found, err := client.SearchPeople(ctx, q)
		if err != nil {
			log.Warn().Err(err).Msg("search people")
		}
		b.mu.Lock()
		if b.searching == q {
			b.people, b.peopleFor, b.searching = found, q, ""
		}
		b.mu.Unlock()
		b.requestRefresh()
	}()
}

// peopleResults returns the server results for q and whether a search for q is still running.
func (b *backend) peopleResults(q string) ([]meta.Person, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.peopleFor == q {
		return b.people, b.searching == q
	}
	return nil, b.searching == q
}

// openPerson creates or finds the conversation with a search result and asks the window to show it.
func (b *backend) openPerson(p meta.Person) {
	client := b.current()
	if client == nil {
		return
	}
	go func() {
		thread, err := client.OpenPerson(b.ctx, b.db, p)
		if err != nil {
			log.Warn().Err(err).Msg("open person")
			if thread.ID == 0 {
				b.notice("Nie udało się otworzyć rozmowy: " + err.Error())
				return
			}
		}
		if h := b.hwnd.Load(); h != 0 {
			postMsg(h, wmAppOpen, 0, uintptr(thread.ID))
		}
	}()
}

// send queues a text message; it reports false when nothing was sent or stored.
// replyTo is the message being answered; its zero value means a plain message.
func (b *backend) send(thread store.Thread, text string, replyTo store.Message) bool {
	client := b.current()
	if client == nil {
		b.notice("Brak połączenia. Wiadomość nie została wysłana.")
		return false
	}
	pending := store.Message{
		ID:        fmt.Sprintf("pending-%d", time.Now().UnixNano()),
		ThreadID:  thread.ID,
		SenderID:  client.AccountID(),
		Text:      text,
		Timestamp: time.Now(),
		Pending:   true,

		ReplyToID:     replyTo.ID,
		ReplyText:     replyTo.Text,
		ReplySenderID: replyTo.SenderID,
		}
	if err := b.db.UpsertMessage(b.ctx, pending); err != nil {
		b.notice("Błąd zapisu: " + err.Error())
		return false
	}
	b.requestRefresh()
	go func() {
		result, err := client.SendText(b.ctx, thread, text, replyTo)
		if err != nil {
			pending.Pending = false
			pending.Error = err.Error()
			_ = b.db.UpsertMessage(b.ctx, pending)
			b.notice("Nie wysłano: " + err.Error())
			b.requestRefresh()
			return
		}
		_ = b.db.DeleteMessage(b.ctx, pending.ID)
		sent := pending
		sent.ID, sent.Timestamp, sent.Pending = result.MessageID, result.Timestamp, false
		_ = b.db.UpsertMessage(b.ctx, sent)
		thread.LastPreview = text
		thread.UpdatedAt = result.Timestamp
		_ = b.db.UpsertThread(b.ctx, thread)
		_ = b.db.SetLastRead(b.ctx, thread.ID, result.Timestamp)
		b.requestRefresh()
	}()
	return true
}
