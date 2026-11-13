//go:build windows

package main

// A link to Facebook or Instagram that arrived without the preview Messenger usually
// attaches: the picture and title are read from the page itself. Only for these two sites,
// which belong to the company the messages already come through, so no third party learns
// anything; no cookies are sent. Links to other sites are never fetched.

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"lazyfmess/internal/store"
)

const (
	ogBusy = iota + 1
	ogHaveIt
	ogFailed
)

type ogDone struct {
	url, title string
	ok         bool
}

var (
	og struct {
		titles map[string]string
		state  map[string]int
		done   chan ogDone
		loaded bool
	}
	ogClient = &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || req.URL.Scheme != "https" || !metaHost(req.URL.String()) {
				return fmt.Errorf("redirect outside the site")
			}
			return nil
		},
	}
	ogImage = regexp.MustCompile(`<meta[^>]+property="og:image"[^>]+content="([^"]+)"`)
	ogTitle = regexp.MustCompile(`<meta[^>]+property="og:title"[^>]+content="([^"]*)"`)
)

// metaHost reports whether an address is on Facebook or Instagram.
func metaHost(raw string) bool {
	host := linkHost(raw)
	for _, d := range []string{"facebook.com", "fb.watch", "instagram.com"} {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

func ogMessage(u string) store.Message {
	return store.Message{ID: "og-" + u, MediaKind: "image", MediaMime: "image/jpeg"}
}

func ogTitlesPath() string { return filepath.Join(dataDir, "media", "podglady.json") }

func ogLoad() {
	if og.loaded {
		return
	}
	og.loaded = true
	og.titles, og.state = map[string]string{}, map[string]int{}
	og.done = make(chan ogDone, 32)
	if raw, err := os.ReadFile(ogTitlesPath()); err == nil {
		_ = json.Unmarshal(raw, &og.titles)
	}
}

// ogHave returns the preview fetched earlier for an address, if there is one.
func ogHave(u string) (store.Message, string, bool) {
	ogLoad()
	m := ogMessage(u)
	if og.state[u] == 0 {
		if _, err := os.Stat(mediaPath(m)); err == nil {
			og.state[u] = ogHaveIt
		}
	}
	return m, og.titles[u], og.state[u] == ogHaveIt
}

// ogKick starts fetching the preview of an address, once; the view is laid out again when
// it arrives.
func ogKick(u string) {
	ogLoad()
	if og.state[u] != 0 || av.offline || !metaHost(u) {
		return
	}
	og.state[u] = ogBusy
	path := mediaPath(ogMessage(u))
	go func() {
		av.sem <- struct{}{}
		title, err := ogFetch(u, path)
		<-av.sem
		og.done <- ogDone{u, title, err == nil}
		postMsg(hMain, wmAppMedia, 0, 0)
	}()
}

func ogFetch(u, path string) (string, error) {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil || req.URL.Scheme != "https" {
		return "", fmt.Errorf("address not usable")
	}
	req.Header.Set("User-Agent", "Linnet/"+appVersion)
	req.Header.Set("Accept-Language", "pl,en;q=0.8")
	resp, err := ogClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	page, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	found := ogImage.FindSubmatch(page)
	if found == nil {
		return "", fmt.Errorf("no preview picture on the page")
	}
	img := html.UnescapeString(string(found[1]))
	pu, err := url.Parse(img)
	if err != nil || pu.Scheme != "https" || !(strings.HasSuffix(pu.Hostname(), ".fbcdn.net") || strings.HasSuffix(pu.Hostname(), ".cdninstagram.com")) {
		return "", fmt.Errorf("preview picture is not on the site's own servers")
	}
	if err := fetchTo(img, path, 8<<20); err != nil {
		return "", err
	}
	title := ""
	if t := ogTitle.FindSubmatch(page); t != nil {
		title = strings.TrimSpace(html.UnescapeString(string(t[1])))
		if r := []rune(title); len(r) > 160 {
			title = string(r[:160])
		}
	}
	return title, nil
}

// ogArrived notes the previews fetched since the last call; true means the view should be
// laid out again.
func ogArrived() bool {
	if !og.loaded {
		return false
	}
	changed := false
	for {
		select {
		case d := <-og.done:
			if d.ok {
				og.state[d.url] = ogHaveIt
				if d.title != "" {
					og.titles[d.url] = d.title
				}
				changed = true
			} else {
				og.state[d.url] = ogFailed
			}
			continue
		default:
		}
		break
	}
	if changed {
		if raw, err := json.Marshal(og.titles); err == nil {
			_ = os.WriteFile(ogTitlesPath(), raw, 0o600)
		}
	}
	return changed
}
