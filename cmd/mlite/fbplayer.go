//go:build windows

package main

// Facebook films that Facebook's own player for other sites refuses.
//
// That player (plugins/video.php) shows only films anyone may see, and does not follow the
// short links people share (facebook.com/share/r/…): for those it says the film is not
// there. The film itself plays on its page, for the person signed in. So the page is what
// the embedded browser opens then, and a script run in it leaves nothing of the page on
// the screen but the film, filling the player of this program. It reads nothing from the
// page and sends nothing anywhere: it only changes what is shown.

import (
	"encoding/json"
	"net/url"
	"strings"
)

// fbShareLink reports whether an address is one of Facebook's short links to a film, which
// its player for other sites does not follow.
func fbShareLink(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := linkHost(raw)
	if host == "fb.watch" {
		return true
	}
	if host != "facebook.com" && !strings.HasSuffix(host, ".facebook.com") {
		return false
	}
	return strings.HasPrefix(u.Path, "/share/")
}

// filmOnlyScript is the script the embedded browser runs in every page it opens while a
// Facebook film is played. In Facebook's player for other sites it waits for the film and,
// when the player says there is none, goes to the page of the film (page is its address).
// On that page it shows the film alone.
func filmOnlyScript(page string) string {
	addr, _ := json.Marshal(page)
	return `(() => {
  if (window.top !== window) return;
  const host = location.hostname;
  if (host !== 'fb.watch' && host !== 'facebook.com' && !host.endsWith('.facebook.com')) return;
  const page = ` + string(addr) + `;
  const start = Date.now();

  if (location.pathname.startsWith('/plugins/video')) {
    // The player for other sites: no film in it after a moment, or its own "not available",
    // means the page of the film is the way.
    const look = setInterval(() => {
      const film = document.querySelector('video');
      const text = document.body ? document.body.innerText || '' : '';
      const refused = /niedost|unavailable|nie masz uprawnie|isn.t available/i.test(text);
      if (film && !refused) { clearInterval(look); return; }
      if (refused || Date.now() - start > 7000) { clearInterval(look); location.replace(page); }
    }, 300);
  }

  // In that player and on the page of a film alike, only the film is shown, whole and as
  // large as the player of this program: the player for other sites cuts an upright film
  // down to its middle.
  const css = document.createElement('style');
  css.textContent =
    'html.linnet-film, html.linnet-film body { background: #0E0F12 !important; overflow: hidden !important; }' +
    'html.linnet-film body * { visibility: hidden !important; }' +
    'html.linnet-film video.linnet-film { visibility: visible !important; position: fixed !important; left: 0 !important; top: 0 !important;' +
    ' width: 100vw !important; height: 100vh !important; max-width: none !important; max-height: none !important;' +
    ' object-fit: contain !important; background: #0E0F12 !important; z-index: 2147483647 !important; }';
  let shown = null;
  const largest = () => {
    let best = null, area = 0;
    for (const v of document.querySelectorAll('video')) {
      const r = v === shown ? { width: v.videoWidth || 1, height: v.videoHeight || 1 } : v.getBoundingClientRect();
      const a = r.width * r.height;
      if (a > area && (v.currentSrc || v.src || v.readyState > 0)) { best = v; area = a; }
    }
    return best;
  };
  const free = el => {
    // "fixed" is counted from the nearest ancestor that is transformed or clipped: none may be.
    for (let p = el.parentElement; p && p !== document.documentElement; p = p.parentElement) {
      for (const k of ['transform', 'filter', 'perspective', 'contain', 'will-change', 'clip-path'])
        p.style.setProperty(k, 'none', 'important');
      p.style.setProperty('overflow', 'visible', 'important');
    }
  };
  const tend = () => {
    const film = largest();
    if (!film) return;
    if (!css.isConnected) (document.head || document.documentElement).appendChild(css);
    if (film !== shown) {
      if (shown) shown.classList.remove('linnet-film');
      shown = film;
      film.classList.add('linnet-film');
      film.controls = true;
      free(film);
      const go = film.play(); if (go && go.catch) go.catch(() => {});
    }
    document.documentElement.classList.add('linnet-film');
  };
  // The page builds itself bit by bit and may replace its film: keep looking for a while.
  const watch = setInterval(() => { tend(); if (Date.now() - start > 120000) clearInterval(watch); }, 400);
  document.addEventListener('DOMContentLoaded', tend);
})();`
}
