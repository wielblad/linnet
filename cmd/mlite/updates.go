//go:build windows

package main

// Keeping the program up to date: it asks the publisher's manifest whether a newer release
// exists, and on one click fetches the installer, checks it against the signature and runs
// it. The installer closes this program, replaces its file and starts the new one.

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"lazyfmess/internal/update"
)

// updateManifestURL is where the newest release is announced. Empty means no updates.
const updateManifestURL = "https://raw.githubusercontent.com/wieladek/linnet/main/latest.json"

const (
	upIdle = iota
	upChecking
	upCurrent  // nothing newer
	upReady    // a newer release is waiting
	upFetching // the installer is being fetched
	upInstall  // the installer is running
	upFailed
)

var upd struct {
	mu       sync.Mutex
	state    int
	note     string // what went wrong, or what is new
	manifest update.Manifest
	done     int64
	total    int64
	checked  time.Time
}

// updatesOn reports whether this program can update itself: only an installed copy can,
// since it is the installed file that would be replaced.
func updatesOn() bool {
	if updateSource().ManifestURL == "" {
		return false
	}
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	return strings.EqualFold(filepath.Clean(exe), filepath.Clean(installedExe()))
}

func updateSource() update.Source {
	src := update.Source{ManifestURL: updateManifestURL}
	if u := os.Getenv("MLITE_UPDATE_URL"); u != "" { // for checks of the whole path
		src.ManifestURL = u
		src.AllowLoopback = true
	}
	key, err := hex.DecodeString(updatePublicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return update.Source{}
	}
	src.PublicKey = key
	return src
}

func updateState() (int, string) {
	upd.mu.Lock()
	defer upd.mu.Unlock()
	return upd.state, upd.note
}

func updateSet(state int, note string) {
	upd.mu.Lock()
	upd.state, upd.note = state, note
	upd.mu.Unlock()
	postMsg(hMain, wmAppUpdate, 0, 0)
}

// updateLine is what the settings row says about updates.
func updateLine() string {
	state, note := updateState()
	switch state {
	case upChecking:
		return "Sprawdzanie…"
	case upCurrent:
		return "Masz najnowszą wersję."
	case upReady:
		return note
	case upFetching:
		upd.mu.Lock()
		done, total := upd.done, upd.total
		upd.mu.Unlock()
		pct := 0
		if total > 0 {
			pct = int(done * 100 / total)
		}
		return fmt.Sprintf("Pobieranie: %d%%", pct)
	case upInstall:
		return "Instalowanie. Program zaraz uruchomi się ponownie."
	case upFailed:
		return note
	}
	if !updatesOn() {
		return "Dostępne po zainstalowaniu programu."
	}
	return "Sprawdź, czy jest nowsza wersja."
}

// updateButton is the label of the button beside that row, "" when there is none.
func updateButton() string {
	if !updatesOn() {
		return ""
	}
	switch state, _ := updateState(); state {
	case upChecking, upFetching, upInstall:
		return ""
	case upReady:
		return "Aktualizuj"
	}
	return "Sprawdź"
}

func updateAction() {
	switch state, _ := updateState(); state {
	case upReady:
		go updateFetch()
	case upChecking, upFetching, upInstall:
	default:
		go updateCheck(false)
	}
}

// updateCheck asks for the manifest. quiet means the result is only shown when something
// newer is there: that is the look taken after start.
func updateCheck(quiet bool) {
	if !updatesOn() {
		return
	}
	if !quiet {
		updateSet(upChecking, "")
	}
	ctx, cancel := context.WithTimeout(be.ctx, 30*time.Second)
	defer cancel()
	m, err := updateSource().Check(ctx)
	upd.mu.Lock()
	upd.checked = time.Now()
	upd.mu.Unlock()
	switch {
	case err != nil:
		log.Warn().Err(err).Msg("check for updates")
		if !quiet {
			updateSet(upFailed, "Nie udało się sprawdzić: "+err.Error())
		}
	case update.Newer(m.Version, appVersion):
		upd.mu.Lock()
		upd.manifest = m
		upd.mu.Unlock()
		note := "Jest nowsza wersja: " + m.Version + "."
		if n := oneLine(m.Notes, 160); n != "" {
			note += " " + n
		}
		updateSet(upReady, note)
	case !quiet:
		updateSet(upCurrent, "")
	}
}

// updateFetch brings the installer in and runs it. The program ends as the installer
// replaces its file; the installer starts the new one.
func updateFetch() {
	upd.mu.Lock()
	m := upd.manifest
	upd.done, upd.total = 0, m.Size
	upd.mu.Unlock()
	if m.Version == "" {
		return
	}
	updateSet(upFetching, "")
	dst := filepath.Join(dataDir, "update", "Linnet-Setup.exe")
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		updateSet(upFailed, "Nie udało się pobrać: "+err.Error())
		return
	}
	last := time.Now()
	ctx, cancel := context.WithTimeout(be.ctx, 20*time.Minute)
	defer cancel()
	err := updateSource().Download(ctx, m, dst, func(done, total int64) {
		upd.mu.Lock()
		upd.done, upd.total = done, total
		upd.mu.Unlock()
		if time.Since(last) > 200*time.Millisecond || done == total {
			last = time.Now()
			postMsg(hMain, wmAppUpdate, 0, 0)
		}
	})
	if err != nil {
		log.Warn().Err(err).Msg("fetch update")
		os.Remove(dst)
		updateSet(upFailed, "Nie udało się pobrać: "+err.Error())
		return
	}
	updateSet(upInstall, "")
	args := "-install -quiet -launch"
	if appName != "Linnet" { // a test installation keeps its own name
		args += ` -name "` + appName + `"`
	}
	if !shellOpen(dst, args) {
		updateSet(upFailed, "Nie udało się uruchomić instalatora.")
		return
	}
	// The installer asks this program to end; should it not, the state stays visible.
}

// updateStart looks for a newer release a short while after the program starts, and then
// once a day while it runs.
func updateStart() {
	if !updatesOn() {
		return
	}
	go func() {
		select {
		case <-be.ctx.Done():
			return
		case <-time.After(90 * time.Second):
		}
		for {
			updateCheck(true)
			select {
			case <-be.ctx.Done():
				return
			case <-time.After(24 * time.Hour):
			}
		}
	}()
}
