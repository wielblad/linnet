// mdiag: read-only diagnostic. It connects to Messenger's plain (LightSpeed) channel
// with the saved session, collects the tables the server sends, and prints counts only:
// participant nicknames, presence, delivery and read receipts. It sends no messages,
// marks nothing as read, registers no encryption device and writes nothing to the session.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	"go.mau.fi/mautrix-meta/pkg/messagix"
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"

	"lazyfmess/internal/session"
)

type presence struct {
	status     int64
	lastActive int64
}

type collector struct {
	mu          sync.Mutex
	kinds       map[string]map[string]int // source -> table name -> rows
	threadType  map[int64]int64
	hybrid      map[int64]int64 // thread key -> jid
	contactName map[int64]string
	parts       int
	partsDeliv  int
	partsRead   int
	nickRows    int
	nickByType  map[int64]int
	nickValues  map[string]bool
	nickThreads map[int64]bool
	targetHits  []string
	presence    map[int64]presence
	delivRows   int
	contactRows int
	contactPics int
	threadPics  int
	xmaRows     int
	xmaPreview  int
	xmaTitle    int
	xmaPlayable int
	blobRows    int
	blobPreview int
	blobPlay    int
	readRows    int
	target      string
}

func newCollector(target string) *collector {
	return &collector{
		kinds: map[string]map[string]int{}, threadType: map[int64]int64{}, hybrid: map[int64]int64{},
		contactName: map[int64]string{}, nickByType: map[int64]int{}, nickValues: map[string]bool{},
		nickThreads: map[int64]bool{}, presence: map[int64]presence{}, target: strings.ToLower(target),
	}
}

func (c *collector) add(source string, tbl *table.LSTable) {
	if tbl == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if raw, err := json.Marshal(tbl); err == nil {
		var m map[string]json.RawMessage
		if json.Unmarshal(raw, &m) == nil {
			if c.kinds[source] == nil {
				c.kinds[source] = map[string]int{}
			}
			for name, v := range m {
				var rows []json.RawMessage
				if json.Unmarshal(v, &rows) == nil {
					c.kinds[source][name] += len(rows)
				}
			}
		}
	}
	for _, t := range tbl.LSDeleteThenInsertThread {
		if t != nil {
			c.threadType[t.ThreadKey] = int64(t.ThreadType)
		}
	}
	for _, t := range tbl.LSVerifyThreadExists {
		if t != nil {
			c.threadType[t.ThreadKey] = int64(t.ThreadType)
		}
	}
	for _, t := range tbl.LSUpdateOrInsertThread {
		if t != nil {
			c.threadType[t.ThreadKey] = int64(t.ThreadType)
		}
	}
	for _, h := range tbl.LSVerifyHybridThreadExists {
		if h != nil {
			c.hybrid[h.ThreadKey] = h.ThreadJID
			c.threadType[h.ThreadKey] = int64(h.ThreadType)
		}
	}
	for _, ct := range tbl.LSVerifyContactParticipantExist {
		if ct != nil && ct.Name != "" {
			c.contactName[ct.ContactID] = ct.Name
		}
	}
	for _, ct := range tbl.LSDeleteThenInsertContact {
		if ct != nil && ct.GetName() != "" {
			c.contactName[ct.GetFBID()] = ct.GetName()
		}
	}
	for _, ct := range tbl.LSVerifyContactRowExists {
		if ct != nil && ct.GetName() != "" {
			c.contactName[ct.GetFBID()] = ct.GetName()
		}
		if ct != nil {
			c.contactRows++
			if ct.ProfilePictureUrl != "" {
				c.contactPics++
			}
		}
	}
	for _, t := range tbl.LSDeleteThenInsertThread {
		if t != nil && t.ThreadPictureUrl != "" {
			c.threadPics++
		}
	}
	for _, x := range tbl.LSInsertXmaAttachment {
		if x == nil {
			continue
		}
		c.xmaRows++
		if x.PreviewUrl != "" {
			c.xmaPreview++
		}
		if x.TitleText != "" {
			c.xmaTitle++
		}
		if x.PlayableUrl != "" {
			c.xmaPlayable++
		}
	}
	for _, b := range tbl.LSInsertBlobAttachment {
		if b == nil {
			continue
		}
		c.blobRows++
		if b.PreviewUrl != "" {
			c.blobPreview++
		}
		if b.PlayableUrl != "" {
			c.blobPlay++
		}
	}
	for _, p := range tbl.LSAddParticipantIdToGroupThread {
		if p == nil {
			continue
		}
		c.parts++
		if p.DeliveredWatermarkTimestampMs > 0 {
			c.partsDeliv++
		}
		if p.ReadWatermarkTimestampMs > 0 {
			c.partsRead++
		}
		if p.Nickname == "" {
			continue
		}
		c.nickRows++
		c.nickValues[p.Nickname] = true
		c.nickThreads[p.ThreadKey] = true
		tt, known := c.threadType[p.ThreadKey]
		if !known {
			tt = -1
		}
		c.nickByType[tt]++
		if c.target != "" && strings.ToLower(p.Nickname) == c.target {
			c.targetHits = append(c.targetHits, fmt.Sprintf("source=%s thread_type=%d", source, tt))
		}
	}
	for _, pr := range tbl.LSDeleteThenInsertContactPresence {
		if pr != nil {
			c.presence[pr.ContactId] = presence{status: pr.Status, lastActive: pr.LastActiveTimestampMs}
		}
	}
	c.delivRows += len(tbl.LSUpdateDeliveryReceipt)
	c.readRows += len(tbl.LSUpdateReadReceipt)
}

func (c *collector) contactByName(name string) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	want := strings.ToLower(strings.TrimSpace(name))
	for id, n := range c.contactName {
		if strings.ToLower(n) == want {
			return id
		}
	}
	return 0
}

func (c *collector) threadKeysFor(contact int64) []int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	seen := map[int64]bool{contact: true}
	keys := []int64{contact}
	for key, jid := range c.hybrid {
		if (key == contact || jid == contact) && !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
		if key == contact && jid != 0 && !seen[jid] {
			seen[jid] = true
			keys = append(keys, jid)
		}
	}
	return keys
}

func main() {
	dir := flag.String("data-dir", "", "directory with cookies.json")
	nick := flag.String("nick", "", "nickname to look for (case-insensitive)")
	name := flag.String("name", "", "real name of the person with that nickname, for a targeted fetch")
	wait := flag.Int("wait", 25, "seconds to listen for live tables")
	logPath := flag.String("log", "mdiag.log", "where to write the library log")
	flag.Parse()

	creds, err := session.LoadCookies(*dir)
	if err != nil {
		fmt.Println("ERROR load cookies:", err)
		os.Exit(1)
	}
	platform := types.PlatformFromString(creds.Platform)
	values := make(map[cookies.MetaCookieName]string, len(creds.Values))
	for k, v := range creds.Values {
		values[cookies.MetaCookieName(k)] = v
	}
	ck := &cookies.Cookies{Platform: platform}
	ck.UpdateValues(values)

	logFile, err := os.Create(*logPath)
	if err != nil {
		fmt.Println("ERROR open log:", err)
		os.Exit(1)
	}
	defer logFile.Close()
	zerolog.SetGlobalLevel(zerolog.WarnLevel)
	logger := zerolog.New(logFile).With().Timestamp().Logger()
	zerolog.DefaultContextLogger = &logger
	zlog.Logger = logger

	col := newCollector(*nick)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	mx := messagix.NewClient(ck, logger, &messagix.Config{})
	mx.SetEventHandler(func(_ context.Context, evt any) {
		if tbl, ok := evt.(*table.LSTable); ok {
			col.add("live", tbl)
		}
	})

	fmt.Println("platform:", creds.Platform)
	_, initial, err := mx.LoadMessagesPage(ctx)
	if err != nil {
		fmt.Println("ERROR load messages page:", err)
		os.Exit(2)
	}
	col.add("initial", initial)
	fmt.Println("initial page loaded")

	if err := mx.Connect(ctx); err != nil {
		fmt.Println("ERROR connect socket:", err)
		os.Exit(3)
	}
	fmt.Println("socket connected")
	time.Sleep(2 * time.Second)

	for batch := 0; batch < 3; batch++ {
		ks, tbl, err := mx.FetchMoreThreads(ctx, 1)
		if err != nil {
			fmt.Println("fetch more threads error:", err)
			break
		}
		col.add("inbox", tbl)
		if ks == nil || !ks.HasMoreBefore {
			break
		}
		time.Sleep(1500 * time.Millisecond)
	}
	fmt.Println("inbox batches fetched")

	targeted := "not attempted"
	if *name != "" && len(col.targetHits) == 0 {
		contact := col.contactByName(*name)
		if contact == 0 {
			targeted = "person with that name not found among received contacts"
		} else {
			keys := col.threadKeysFor(contact)
			done := 0
			for _, key := range keys {
				if done >= 3 {
					break
				}
				tbl, err := mx.ExecuteTasks(ctx, &socket.FetchAdditionalThreadData{ThreadKey: key})
				if err != nil {
					fmt.Println("fetch additional thread data error:", err)
					continue
				}
				col.add("thread", tbl)
				done++
				time.Sleep(time.Second)
			}
			targeted = fmt.Sprintf("person found; fetched extra data for %d thread key(s)", done)
		}
	}

	fmt.Printf("listening %d s for live tables...\n", *wait)
	time.Sleep(time.Duration(*wait) * time.Second)
	mx.Disconnect()
	time.Sleep(500 * time.Millisecond)

	col.mu.Lock()
	defer col.mu.Unlock()
	fmt.Println()
	fmt.Println("== tables received (rows) ==")
	for _, src := range []string{"initial", "inbox", "thread", "live"} {
		names := make([]string, 0, len(col.kinds[src]))
		for n, cnt := range col.kinds[src] {
			if cnt > 0 {
				names = append(names, fmt.Sprintf("%s=%d", strings.TrimPrefix(n, "LS"), cnt))
			}
		}
		sort.Strings(names)
		fmt.Printf("[%s] %s\n", src, strings.Join(names, " "))
	}
	fmt.Println()
	fmt.Println("== participants ==")
	fmt.Println("participant rows:", col.parts, "| with delivered watermark:", col.partsDeliv, "| with read watermark:", col.partsRead)
	fmt.Println("rows with a nickname:", col.nickRows, "| distinct nicknames:", len(col.nickValues), "| threads with nicknames:", len(col.nickThreads))
	fmt.Println("nickname rows by thread type (-1 = type unknown):", col.nickByType)
	typeCount := map[int64]int{}
	for _, tt := range col.threadType {
		typeCount[tt]++
	}
	fmt.Println("threads seen by type:", typeCount, "| hybrid mappings:", len(col.hybrid))
	if *nick != "" {
		fmt.Printf("target nickname %q found: %v %v\n", *nick, len(col.targetHits) > 0, col.targetHits)
		fmt.Println("targeted fetch:", targeted)
	}
	fmt.Println()
	fmt.Println("== pictures, link cards, attachments ==")
	fmt.Println("contact rows:", col.contactRows, "| with profile picture URL:", col.contactPics, "| threads with picture URL:", col.threadPics)
	fmt.Println("link-card (XMA) rows:", col.xmaRows, "| with preview image:", col.xmaPreview, "| with title:", col.xmaTitle, "| with playable URL:", col.xmaPlayable)
	fmt.Println("attachment rows:", col.blobRows, "| with preview:", col.blobPreview, "| with playable URL:", col.blobPlay)
	fmt.Println()
	fmt.Println("== presence ==")
	statuses := map[int64]int{}
	recent := 0
	now := time.Now().UnixMilli()
	for _, p := range col.presence {
		statuses[p.status]++
		if p.lastActive > 0 && now-p.lastActive < 10*60*1000 {
			recent++
		}
	}
	fmt.Println("contacts with presence rows:", len(col.presence), "| by status:", statuses, "| active in last 10 min:", recent)
	fmt.Println()
	fmt.Println("== receipts ==")
	fmt.Println("delivery receipt rows:", col.delivRows, "| read receipt rows:", col.readRows)

	fmt.Println()
	fmt.Println("== procedures the library skipped ==")
	logFile.Sync()
	if f, err := os.Open(*logPath); err == nil {
		defer f.Close()
		counts := map[string]int{}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 1<<20), 8<<20)
		for sc.Scan() {
			var line struct {
				Ref string `json:"reference_name"`
			}
			if json.Unmarshal(sc.Bytes(), &line) == nil && line.Ref != "" {
				counts[line.Ref]++
			}
		}
		names := make([]string, 0, len(counts))
		for n, cnt := range counts {
			names = append(names, fmt.Sprintf("%s=%d", n, cnt))
		}
		sort.Strings(names)
		fmt.Println(strings.Join(names, " "))
	}
}
