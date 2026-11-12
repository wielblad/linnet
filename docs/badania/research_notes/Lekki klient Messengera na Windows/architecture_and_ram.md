# Architectures and RAM footprints for a lightweight non-webview Messenger client on Windows 11 (state: 12 Nov 2026)

Research date: 2026-11-12. All "retrieved" dates are 2026-11-12 unless stated. Source quality is flagged per item: [PRIMARY] = project repo/docs/vendor page, [PRESS] = tech press, [SECONDARY] = blog/aggregator with weak or no methodology. Where no measurement was found this is stated explicitly; nothing below is estimated by me unless it sits under "Inferences".

Important premise change found during research: Meta discontinued the Messenger desktop app for Windows/macOS (Dec 2025) and messenger.com (Apr 2026). The "official Messenger on Windows" is now facebook.com/messages in a browser. See section A.

---

## A. Ready-made routes (no or little coding)

### Takeaway
Two brand-new open-source clients built directly on messagix already exist: `lazyfmess` (Go + Bubble Tea TUI, ships a `lazyfmess.exe`, but Windows build is "compiled but not tested", repo created 2026-11-07) and `Hermes` (native AppKit, macOS only). No tested, native, non-webview Messenger GUI for Windows was found. Beeper Desktop is Electron (no RAM figure found); the Matrix route works but no measured RAM for the full stack was found; bitlbee/Pidgin/messlidger are effectively dead ends (messlidger cannot send).

### Cited Findings

**Official Meta desktop options (baseline situation)**
- Meta shut down the Messenger desktop apps for Windows and Mac; TechCrunch's story on it is dated 2025-12-15. [PRESS] — [TechCrunch 2025-12-15](https://techcrunch.com/2025/12/15/facebook-messengers-desktop-app-is-no-more/)
- messenger.com: "Starting April 2026, the website will no longer be available"; users are "automatically redirected to use facebook.com/messages for messaging on a computer". Article dated 2026-02-19. [PRESS] — [TechCrunch 2026-02-19](https://www.techcrunch.com/2026/02/19/meta-is-shutting-down-messengers-standalone-website/)
- One outlet gives the exact stop date as 16 April 2026 (seen only in a search snippet, not verified by opening the page). [PRESS, UNVERIFIED] — [IBTimes AU](https://ibtimes.com.au/facebook-messenger-down-web-version-shuts-today-meta-redirects-users-facebook-chat-april-2026-1866690)
- mautrix-meta is still the maintained path for Messenger: the August 2026 mautrix release post (v26.08 / v0.2608.0) says mautrix-meta "is still used for Facebook Messenger"; Instagram moved to a separate mautrix-instagram bridge after Meta disabled the old API. [PRIMARY] — [mau.fi blog, Aug 2026](https://mau.fi/blog/2026-08-mautrix-release/)
- mautrix/meta repo last pushed 2026-11-12 (GitHub API `pushed_at: 2026-11-12T03:49:02Z`), i.e. actively maintained. [PRIMARY] — [github.com/mautrix/meta](https://github.com/mautrix/meta)
- matrix.org bridge directory lists mautrix-meta as "Stable" and the old mautrix-facebook as "Obsolete" (from search snippet). [PRIMARY] — [matrix.org bridges: Facebook Messenger](https://matrix.org/ecosystem/bridges/facebook_messenger/)

**Existing standalone clients built directly on messagix (GitHub search 2026-11-12)**
- `samjoshuadud/lazyfmess` — "Unofficial terminal client (TUI) for Facebook Messenger, with end-to-end encrypted chats and a local SQLite cache". Created 2026-11-07, last push 2026-11-09. AGPL-3.0. Features per README: regular and E2EE chats with live delivery, replies, reactions, edit/unsend, read receipts, typing indicators, image/video/file attachments (send up to 25 MB), people search, mute, encrypted-history download with the secure-storage PIN, "instant startup from a local SQLite cache". [PRIMARY] — [lazyfmess README](https://github.com/samjoshuadud/lazyfmess/blob/main/README.md)
  - Windows: release archives include `lazyfmess.exe`, but README states "macOS and Windows builds are compiled but not tested"; browser cookie import "works on Linux only". — same source
  - Stack (go.mod): Go 1.27.3, `charmbracelet/bubbletea v1.3.10` + bubbles + lipgloss, `go.mau.fi/mautrix-meta v0.2610.0`, `go.mau.fi/whatsmeow`, `modernc.org/sqlite v1.61.1` (pure Go, no CGO). [PRIMARY] — [lazyfmess go.mod](https://github.com/samjoshuadud/lazyfmess/blob/main/go.mod)
  - Known limitations per README: 2FA code entry fails with bundled mautrix-meta v0.2610.0 ("expected int64 ... got float64"; fix merged upstream, awaiting next release); "Encrypted media shows as placeholders and cannot be opened"; account-login appears to Meta as a "Google Pixel 7" phone; cookie sessions identify as "Chrome on Linux". — README above
  - Account-risk statement in README: "users of mautrix-meta have occasionally reported disabled accounts" (links mautrix/meta issue #44). — README above
  - No RAM/CPU figure is published in the README.
- `gklka/Hermes` — "An unofficial, free, fully native AppKit macOS Facebook Messenger client". Created 2026-10-12, last push 2026-11-11. macOS 26+ only. Architecture: "AppKit UI ──JSON over C calls──▶ Go core (static lib)" using messagix + whatsmeow; storage in `app.db`/`e2ee.db`. Login uses a WKWebView showing facebook.com's login page; calls and encrypted-history restore run in a hidden/own-window web view of Messenger's web app. AGPL-3.0. Not usable on Windows, but it is a working proof of the "native UI + in-process Go messagix core" design. [PRIMARY] — [Hermes README](https://github.com/gklka/Hermes/blob/main/README.md)
- `leduyphuc1702/chat-space` — repo description says "Native unified messenger: Zalo, Messenger, WhatsApp, Telegram in one app (macOS + Windows)", but the README describes only a macOS native SwiftUI+AppKit app ("không WebView") requiring macOS 14+. Messenger runs in a separate Go sidecar process (`chatspace-messenger`, mautrix-meta/messagix + whatsmeow) talking line-delimited JSON over stdin/stdout. Created 2026-11-02. Conflict between description (Windows) and README (macOS only) — Windows support NOT verified. [PRIMARY] — [chat-space README](https://github.com/leduyphuc1702/chat-space/blob/main/README.md)
  - Notable warning in that README: PIN-based restore of encrypted history was disabled in the project "because Meta locked the account right after one restore" (translated from Vietnamese; single developer anecdote). — same source
  - It also vendors a patched mautrix-meta v0.2610.0 because the upstream Bloks interpreter errored on the login button (`expected int64 or float64 in jmu lhs, got <nil>`). — same source
- Other 2026 repos using messagix as a headless sidecar (descriptions only, not inspected): `kwonsyup/kluro-messagix` ("Messenger connection program used by Kluro… built on mautrix-meta", pushed 2026-11-11), `plutioandco/convolios-meta-bridge` ("on-device Meta bridge sidecar", 2026-06-14), `jonathancheckwise/molebot-bridges` (2026-10-31), `matpb/meta-mcp-rs` ("personal Messenger and WhatsApp through local sidecars", pushed 2026-11-10). [PRIMARY, descriptions only] — [GitHub search "messagix"](https://github.com/search?q=messagix&type=repositories)
- `t2n/wincent-fb-messenger` — small Go Messenger bot on messagix + SQLite (6 Go files: main, config, messenger, ai, store, scheduler); shows the minimal headless integration pattern (cookies `c_user`, `xs`, `datr`). Last push 2026-03-24. [PRIMARY] — [wincent README](https://github.com/t2n/wincent-fb-messenger/blob/master/README.md)
- `rozsazoltan/messenger` — Tauri wrapper ("~5 MB instead of 100-200 MB" installer size claim, Windows/macOS/Linux), i.e. a webview wrapper = the excluded category; listed only for completeness. [PRIMARY] — [github.com/rozsazoltan/messenger](https://github.com/rozsazoltan/messenger)
- Caprine (Electron wrapper) — repo last pushed 2026-01-02. [PRIMARY] — [github.com/sindresorhus/caprine](https://github.com/sindresorhus/caprine)

**Beeper Desktop**
- Still Electron: Beeper's Feb 2025 beta post says the new desktop app was built on the Texts.com desktop foundations; Texts/Beeper desktop are "Electron/React apps" per Daring Fireball (2024-04-11); a third-party tracker lists a Beeper build as Electron v38.3.0, "last verified March 2026" (tracker seen only via search snippet). [PRIMARY for foundation; SECONDARY for Electron] — [Beeper blog 2025-02-24](https://blog.beeper.com/2025/02/24/try-out-the-new-beeper-desktop-and-ios-beta/); [Daring Fireball 2024-04-11](https://daringfireball.net/linked/2024/04/11/automattic-beeper); [desktopinsights.com](https://desktopinsights.com/apps/beepertexts)
- Messenger connection: Beeper's help page lists Facebook Messenger under both On-Device ("Sends directly from your device") and Beeper Cloud ("Routes through Beeper's servers"). An on-device account "remains available only on the device where you set it up". Page undated, footer © 2026. [PRIMARY] — [Beeper help: On-Device vs Cloud](https://help.beeper.com/troubleshooting/beeper-on-device-vs-beeper-cloud-what-goes-to-beeper-servers)
- Free tier: "Beeper is free for up to 5 accounts." Beeper Plus "starting at $9.99/month" (10 accounts), Plus Plus "starting at $49.99/month" (unlimited). Available for Windows, macOS, Linux, iOS, Android, ChromeOS. [PRIMARY] — [beeper.com/faq](https://www.beeper.com/faq)
- RAM: NO measured figure found for Beeper Desktop (2025–2026). Only qualitative: a community comment on Beeper's 2023 blog says the Electron desktop app "shows high memory usage, regardless of the number of Chat Networks". [SECONDARY, old] — [Beeper blog 2023-03-06](https://blog.beeper.com/2023/03/06/state-of-the-app-spring-2023/)
- Beeper changelog mentions fixes for "high CPU usage in certain bridges" and missed incoming messages on on-device Instagram/Facebook bridges (entry dates not captured). [PRIMARY] — [Beeper desktop changelog](https://www.beeper.com/changelog/desktop/page/3)

**Matrix route (mautrix-meta + homeserver + native client)**
- gomuks (2026): web frontend is the mature one ("ready for daily use"); the desktop app is "an Electron-based wrapper of the web frontend"; the terminal frontend is "still experimental" with only basic chatting; legacy TUI is at tag v0.3.1. So gomuks desktop is NOT a non-webview option. [PRIMARY] — [gomuks README](https://raw.githubusercontent.com/gomuks/gomuks/main/README.md)
- Nheko (Qt/C++20): Windows builds distributed as x64 MSIX / appinstaller via GitHub releases (per README, from search snippet). No 2026 release confirmed in my search. [PRIMARY, partially verified] — [github.com/Nheko-Reborn/nheko](https://github.com/Nheko-Reborn/nheko)
- Homeserver idle RAM — only blog/comparison-site estimates found, which conflict with each other (none is a controlled benchmark):
  - Conduit: "20–50 MB" in one table, "70–100 MB with no users" in another guide; default `CONDUIT_DB_CACHE_CAPACITY_MB` is 300. [SECONDARY] — [selfhosting.sh Conduit](https://selfhosting-sh.pages.dev/apps/conduit/); [Conduit vs Synapse](https://selfhosting-sh.pages.dev/compare/conduit-vs-synapse/)
  - Continuwuity: "64–256 MB" for 10–100 users. [SECONDARY] — [pistack.xyz 2026-05-02](https://www.pistack.xyz/posts/2026-05-02-synapse-vs-dendrite-vs-continuwuity-self-hosted-matrix-server-guide/)
  - Dendrite: "50–100 MB idle" vs "256–512 MB" in another guide. [SECONDARY] — [matrixdocs comparison](https://matrixdocs.github.io/docs/servers/comparison); pistack link above
  - Synapse: "300–500 MB" vs "800 MB–1 GB" vs "1–2 GB" depending on source. [SECONDARY] — same sources
  - Tuwunel: no idle figure found.
- mautrix-meta bridge process RAM: no published figure found (docs and release posts give none). — [docs.mau.fi mautrix-meta](https://docs.mau.fi/bridges/go/meta/index.html)

**Legacy gateways**
- slidge "messlidger" (XMPP gateway): README warns it "cannot **send** messages from XMPP to Messenger anymore because the library used to communicate with Facebook Messenger is not maintained anymore" (it is based on the obsolete mautrix-facebook). Last substantive code commit 2025-04-06; last commit (lockfile) 2025-10-23. Receive-only at best. [PRIMARY] — [codeberg.org/slidge/messlidger](https://codeberg.org/slidge/messlidger)
- purple-facebook (Pidgin/libpurple plugin): last push April 2024 per repo metadata (via aggregator snippet). No evidence it works in 2026. [SECONDARY] — [ecosyste.ms: dequis/purple-facebook](https://awesome.ecosyste.ms/projects/github.com%2Fdequis%2Fpurple-facebook)
- bitlbee-facebook: latest packaged version is still 1.2.2 (AUR); no 2025–2026 report found confirming it works or fails. [SECONDARY] — [AUR bitlbee-facebook](https://aur.archlinux.org/pkgbase/bitlbee-facebook)
- nchat (TUI; Telegram/WhatsApp/Signal): officially "for Linux and macOS" (no Windows); project is "feature-complete and in maintenance mode"; SCOPE limits feature requests to ~<100 added lines and to features "Supporting all three protocols" — so Messenger support is out of scope. A search of its issue tracker for "messenger OR facebook" returned no Messenger request (3 unrelated hits). [PRIMARY] — [nchat README](https://github.com/d99kris/nchat/blob/master/README.md); [nchat SCOPE.md](https://github.com/d99kris/nchat/blob/master/doc/SCOPE.md); [issue search](https://github.com/d99kris/nchat/issues?q=messenger+OR+facebook)

**Reference RAM of the webview/Electron class on Windows 11 (for quantifying savings)**
- No Messenger-specific measurement (facebook.com/messages tab, Messenger PWA, Caprine) was found. Closest measured analogues, all from Windows Latest's own Task Manager tests summarized in its 2026-10-02 article: WhatsApp (WebView2) ~600 MB idle, ~1.2 GB while scrolling chats (scrolling test Nov 2025); old native WhatsApp UWP "under 100 MB at idle"; Teams (WebView2) 963 MB idle on a fresh account; New Outlook (WebView2) 490–636 MB idle vs Outlook Classic (Win32) 117–148 MB idle (June 2026). [PRESS] — [Windows Latest 2026-10-02](https://www.windowslatest.com/2026/10/02/windows-11s-winui-just-got-basic-features-and-less-ram-hogging-after-years-of-losing-apps-to-the-web/)
- Blank Electron window: one developer reports "150 and 300 MB RSS depending on platform" at idle. [SECONDARY] — [dev.to post](https://dev.to/namanvyas/5-desktop-app-pain-points-that-pushed-me-to-build-my-own-go-framework-48ic)
- One-window hello-world on Windows 11, private bytes summed over all processes: Tauri (WebView2) 142 MB (median of 5 runs) vs Electron 149 MB (1 run) — supports the user's point that a WebView2 wrapper barely helps. [SECONDARY, method stated] — [dawloom.com Electron alternatives](https://dawloom.com/blog/electron-alternatives/)

### Inferences
- The lowest-effort candidate matching "tiny, non-webview, write/receive messages" is lazyfmess run in Windows Terminal — but it is 5 days old, explicitly untested on Windows, and has no published RAM number; it must be measured locally before being recommended. Its stack (Go + messagix + modernc SQLite + Bubble Tea) is exactly the architecture a self-written client would use, so it doubles as a reference implementation/fork base (AGPL-3.0).
- Beeper Desktop does not meet the goal: it is Electron (same weight class as the things being replaced), even though no exact RAM figure was found.
- The Matrix route needs three long-running pieces (bridge + homeserver + client). Nothing found says they cannot all run on the Windows PC (mautrix bridges and conduit-family servers are single binaries), but no source documenting such an all-local Windows setup or its total RAM was found; by the conflicting estimates above, the homeserver alone could equal or exceed a purpose-built Go client. Its only truly native Windows GUI client with confirmed Windows packages in my search is Nheko (Qt).
- Pidgin/bitlbee/XMPP routes should be treated as non-working for two-way Messenger in 2026 (messlidger explicitly cannot send; the others show no maintenance evidence).

### Gaps
- No measured RAM for: Beeper Desktop, facebook.com/messages in Chromium/Edge, the Messenger PWA, Caprine, mautrix-meta process, lazyfmess, Hermes, Nheko, NeoChat, iamb, gomuks, Fractal, Quaternion. Searches returned nothing citable.
- Windows status in 2026 of iamb, NeoChat, Fractal, Quaternion, Element X desktop, SchildiChat desktop: not verified (searches returned no usable results).
- Whether Beeper's on-device Messenger bridge keeps receiving when Beeper Desktop is closed: not stated in Beeper's docs.
- Whether a Microsoft Store "Messenger" PWA/app still exists after Dec 2025: not verified.
- Current working state of bitlbee-facebook/purple-facebook: no 2025–2026 primary evidence either way.
- Whether lazyfmess actually runs correctly on Windows 11 (input of Polish diacritics in the TUI, notifications): untested by its author and by me.

---

## B. Building blocks for a self-written native Go client on Windows

### Takeaway
Hard, comparable idle-RAM measurements for Go GUI toolkits on Windows essentially do not exist publicly; the few data points found are a Fyne app at ~14 MB (Windows 10, issue report) and a Linux/arm64 comparison where Gio used ~151 MB RSS vs ~13 MB for nucular. The proven architecture patterns in the wild (Nov 2026) are (1) Go core linked in-process behind a native UI (Hermes) and (2) Go messagix sidecar process speaking JSON over stdio to a native UI (chat-space, Kluro, Convolios, meta-mcp-rs), and (3) a single Go binary with a TUI (lazyfmess).

### Cited Findings

**Go GUI toolkit memory (measured data points found)**
- Fyne, Windows 10, Fyne v2.1.4: an issue reports a simple app with an Entry widget at "a stable 14.2 MB", rising to "a stable 18 MB" after typing (memory grows per character typed). [PRIMARY issue report; single user measurement] — [fyne-io/fyne#2969](https://github.com/fyne-io/fyne/issues/2969)
- Fyne regression report: a specific app used "around 40 MB of memory after the application startup" on v2.2.4 vs 400 MB on v2.3.2. Shows Fyne memory is version/app sensitive. [PRIMARY issue report] — [fyne-io/fyne#3762](https://github.com/fyne-io/fyne/issues/3762)
- Fyne image handling has had memory complaints (issue "Memory Usage with Image Viewing"). [PRIMARY] — [fyne-io/fyne#4014](https://github.com/fyne-io/fyne/issues/4014)
- Gio vs go-shirei vs nucular, same small app ("Chelydra"), Linux arm64 phone (Phosh), stripped builds, article dated 2026-08-10. Worst observed values: gioui RSS 154,704 / idle CPU 0.00 % / active CPU 4.30 % / binary 6,867,456; go-shirei RSS 38,112 / idle CPU 0.68 % / binary 7,340,297; nucular RSS 13,652 / idle CPU 0.45 % / binary 4,063,397. Units are not stated by the author (pidstat reports kB by convention, i.e. ~151 MB / ~37 MB / ~13 MB — unit is an assumption). Author's remarks: Gio "had the lowest idle CPU", "did panic under Wayland"; Fyne judged "bloated and unpleasant to work with" (qualitative, no numbers). NOT a Windows measurement. [SECONDARY, method stated] — [ser1.net Go GUI libraries, 2026-08-10](https://ser1.net/post/golang_gui_libraries/)
- Unverified blog claim: "Fyne typically uses 5–15 MB runtime memory" (no methodology). [SECONDARY, UNVERIFIED] — [friday-go.icu Fyne vs Wails](https://friday-go.icu/golang/Fyne-vs-Wails-Deep-Comparison-Choosing-the-Best-Go-Desktop-Framework)
- Wails (WebView2): no direct measurement found; nearest proxy is the Tauri/WebView2 hello-world at 142 MB private bytes on Windows 11 cited in section A. — [dawloom.com](https://dawloom.com/blog/electron-alternatives/)
- No measured figures found for: lxn/walk, tailscale/walk, rodrigocfd/windigo, miqt (Qt), cimgui-go (Dear ImGui), go-flutter, bubbletea, tview.

**Non-Go UI stacks for a split design (daemon + thin UI)**
- Native Win32-class apps on Windows 11 as reference: Outlook Classic (Win32) 117–148 MB idle (June 2026); old native WhatsApp (UWP) and old native Copilot (WinUI) "under/less than 100 MB". These are full apps, not hello-worlds. [PRESS] — [Windows Latest 2026-10-02](https://www.windowslatest.com/2026/10/02/windows-11s-winui-just-got-basic-features-and-less-ram-hogging-after-years-of-losing-apps-to-the-web/)
- Rust: one developer's real app went "from 135 MB to 30 MB" by replacing GPU-rendered egui with Slint's software renderer (article title; details from search snippet, page not opened). [SECONDARY] — [dev.to: 135 MB to 30 MB](https://dev.to/trystan_sarrade/how-i-took-my-rust-gui-from-135-mb-to-30-mb-by-ditching-the-gpu-2i7d)
- iced: a user reports 7.7 MB for a small Minesweeper app (OS/method unclear; snippet only). [SECONDARY, UNVERIFIED] — [feddit.it thread](https://feddit.it/post/862651/1300083)
- egui/iced idle CPU: forum user reports egui ~1–2 % idle CPU and iced "like 8 % instead of 2 %" (anecdote; snippet only). [SECONDARY, UNVERIFIED] — [discourse.iced.rs](https://discourse.iced.rs/t/what-type-of-gui-library-is-it/690)
- Tauri/iced/egui comparison (2023-02-03, Ubuntu 20.04 only): binary sizes Tauri 5 MB, iced 17 MB, egui 18 MB; startup to final render ≈380 / ≈230 / ≈280 ms. It contains NO memory figures and no Windows data. [SECONDARY, method stated] — [lukaskalbertodt 2023-02-03](https://lukaskalbertodt.github.io/2023/02/03/tauri-iced-egui-performance-comparison.html)
- Windows UI frameworks resize test (2024-04-13): WinAPI, WinForms, WPF, UWP pass a live-resize stress test; WinUI 3 fails; web-based (Electron/WebView2/Tauri) fail. The post announces a RAM comparison but publishes no RAM numbers. [SECONDARY] — [vilunov.me 2024-04-13](https://vilunov.me/blog/windows-ui/)
- WinUI 3 with NativeAOT started in ~0.1–0.2 s vs ~0.5 s for .NET 8 WPF in a Windows 11 VM test (startup only, no memory; snippet only). [SECONDARY] — [zenn.dev](https://zenn.dev/suusanex/articles/f642d91df412c9)

**Architecture patterns demonstrated by real messagix-based projects**
- In-process: Hermes builds the Go core as a static library (`libmmcore.a`); Swift calls `MMRequest(json)` and Go answers via one callback with `{"type":"response"|"event", …}`. [PRIMARY] — [Hermes README](https://github.com/gklka/Hermes/blob/main/README.md)
- Sidecar process: chat-space runs each network bridge as a separate process with line-delimited JSON over stdin/stdout, auto-restarting a crashed bridge (max 5 times with backoff), so "a faulty or blocked bridge does not take down the others". [PRIMARY] — [chat-space README](https://github.com/leduyphuc1702/chat-space/blob/main/README.md)
- Single Go binary: lazyfmess = messagix + whatsmeow + modernc SQLite (WAL) + Bubble Tea in one executable, with data files `cookies.json`, `messagix-state.json`, `device.db`, `messages.db`. [PRIMARY] — [lazyfmess README](https://github.com/samjoshuadud/lazyfmess/blob/main/README.md)
- AGPL consequence stated by these projects: "It has to be [AGPL-3.0]: it's built on mautrix-meta". Kluro/Convolios/Molebot publish their bridge sidecar source separately for the same reason. [PRIMARY] — Hermes README above; [GitHub search](https://github.com/search?q=messagix&type=repositories)

**SQLite driver choice (pure Go vs CGO)**
- Speed: older benchmarks show modernc.org/sqlite between ~10 % and 2x slower than mattn/go-sqlite3, the gap largest on writes. [SECONDARY, method stated] — [DataStation 2022-05-12](https://datastation.multiprocess.io/blog/2022-05-12-sqlite-in-go-with-and-without-cgo.html)
- Memory: no controlled RSS comparison found. One project PR reports fewer allocations with mattn ("1.12M vs 1.64M allocs and 21MB vs 30MB" — fragmentary snippet, direction not independently confirmed) and notes that with CGO SQLite, memory allocated in C is outside Go's memory limit/heap metrics. [SECONDARY, UNVERIFIED] — [CoreScope PR #1992](https://github.com/Kpa-clawbot/CoreScope/pull/1992)
- Maintained benchmark suites exist for measuring this locally. [PRIMARY] — [cvilsmeier/go-sqlite-bench](https://github.com/cvilsmeier/go-sqlite-bench); [ncruces/go-sqlite-bench](https://github.com/ncruces/go-sqlite-bench)
- Real-world precedent: lazyfmess uses modernc.org/sqlite (no CGO, simple Windows cross-compilation). — [lazyfmess go.mod](https://github.com/samjoshuadud/lazyfmess/blob/main/go.mod)

### Inferences
- With the evidence available, the only defensible ordering is by class, not by exact MB: TUI or Win32-native UI + Go core (expected tens of MB) << WebView2/Electron wrappers (measured ~140–150 MB for an empty window; ~600 MB–1.2 GB for real chat apps like WhatsApp on WebView2). Exact figures for a messagix client must be measured — the cheapest way is to run `lazyfmess.exe` on the target PC and read Private Working Set in Task Manager.
- Gio is risky for a "low background RAM" goal: the one comparable measurement shows roughly 10x the RSS of an immediate-mode software-rendered toolkit (assuming kB units), although its idle CPU was 0 %. GPU-backed renderers (Gio, egui, Fyne/OpenGL) tend to carry driver/GPU-context memory, which matches the "135 MB → 30 MB after ditching the GPU" Rust report.
- Single Go binary vs split design: single binary is simplest (one process, no IPC, proven by lazyfmess). A split (Go daemon + native Win32/WinUI/Rust UI) lets the UI process exit entirely while a tray daemon keeps the connection, and isolates AGPL Go code as a sidecar (pattern used by Kluro/Convolios/chat-space); cost is two codebases/languages and an IPC protocol to design (chat-space documents its own in docs/PROTOCOL.md).
- Hermes shows that login is the hard UI part: even a "fully native" client uses a web view for facebook.com login, and lazyfmess notes reCAPTCHA/security checkpoints "are web pages that a terminal cannot show". A strictly no-webview client should plan for cookie-paste login as a fallback.

### Gaps
- No measured idle RAM on Windows found for lxn/walk, tailscale/walk, windigo, miqt, cimgui-go, go-flutter, Gio, bubbletea, tview, WinUI 3 hello-world, WPF hello-world, Slint, egui, iced. All would need local measurement.
- No credible modern (Go 1.2x) figure found for the baseline working set of an idle Go process holding one WebSocket/MQTT connection plus SQLite on Windows; search returned only 2013-era Go issues (not usable).
- Maintenance status in 2026 of lxn/walk, tailscale/walk, windigo, miqt, cimgui-go, go-flutter, getlantern/systray, fyne systray, go-toast/WinRT toast bindings: not researched (ran out of budget); do not assume.
- Per-toolkit behaviour for colour emoji rendering, inline image display, IME and Polish diacritics (AltGr) input: no sources collected; unverified for every toolkit.
- Autostart mechanisms (Run registry key / Startup folder / Task Scheduler) from Go: not researched.

---

## C. Effort indicators (comparable hobby projects)

### Takeaway
Two fresh direct comparables exist: lazyfmess (≈308 kB of Go source; full TUI client incl. E2EE, appeared within days of repo creation on 2026-11-07) and Hermes (≈909 kB Swift + ≈231 kB Go; feature-rich native macOS client, repo created 2026-10-12, one month old). Both show a single developer can reach a usable messagix-based client in weeks, but both also document recurring breakage from Meta-side changes and account-risk warnings.

### Cited Findings
- lazyfmess source size: `{"Go":307990,"Nix":636,"Shell":10}` bytes (GitHub languages API, 2026-11-12). Repo created 2026-11-07, last push 2026-11-09. [PRIMARY] — [GitHub API: lazyfmess languages](https://api.github.com/repos/samjoshuadud/lazyfmess/languages)
- Hermes source size: `{"Swift":909136,"Go":230699,"Shell":9953,"HTML":735,"C":102}` bytes. Repo created 2026-10-12, last push 2026-11-11. [PRIMARY] — [GitHub API: Hermes languages](https://api.github.com/repos/gklka/Hermes/languages)
- Minimal headless integration: wincent-fb-messenger is a messagix bot in six Go files (main, config, messenger, ai, store, scheduler) with SQLite. [PRIMARY] — [wincent README](https://github.com/t2n/wincent-fb-messenger/blob/master/README.md)
- Ongoing maintenance burden (evidence): lazyfmess: "Meta changed its code entry screen, and the bundled mautrix-meta v0.2610.0 cannot process it" (2FA codes fail until next upstream release). chat-space vendors and patches mautrix-meta's Bloks interpreter to get login working. nchat states its protocol libraries "are actively developed and need to be updated and integrated into nchat on a regular basis", and had a full outage when whatsmeow was outdated (issue #283, Aug 2024: "Client outdated (405)") and a login break from WhatsApp's new passkey flow (issue #546, July 2026). [PRIMARY] — [lazyfmess README](https://github.com/samjoshuadud/lazyfmess/blob/main/README.md); [chat-space README](https://github.com/leduyphuc1702/chat-space/blob/main/README.md); [nchat README](https://github.com/d99kris/nchat/blob/master/README.md); [nchat #283](https://github.com/d99kris/nchat/issues/283); [nchat #546](https://github.com/d99kris/nchat/issues/546)
- nchat architecture (comparable multi-protocol TUI): "primarily implemented in C++ with some parts in Go"; ncurses UI; bundles tdlib (Telegram), whatsmeow (WhatsApp, Go via cgo wrapper `lib/wmchat/go`), mautrix-signal (Signal). Building needs "~3.5GB RAM … using G++ and ~1.5GB RAM with clang++". Linux and macOS only. Runtime RAM: no figure published. Messenger: not supported, not requested in issues, and out of declared scope. [PRIMARY] — [nchat README](https://github.com/d99kris/nchat/blob/master/README.md); [nchat SCOPE.md](https://github.com/d99kris/nchat/blob/master/doc/SCOPE.md)
- whatsmeow at scale (anecdote): a user running 1,000+ sessions calls its footprint "predictable and low" (no numbers; snippet only). [SECONDARY, UNVERIFIED] — [whatsmeow discussion #979](https://github.com/tulir/whatsmeow/discussions/979)
- Account risk documented by comparables: Hermes — "Meta can restrict, suspend or disable an account it believes breaks its terms"; lazyfmess — any login method "can lead to a security check, a forced logout, or a temporary restriction"; chat-space — account locked right after a PIN history restore (single anecdote). [PRIMARY] — READMEs linked above

### Inferences
- ≈308 kB of Go is on the order of 8–10 thousand lines (my estimate from bytes; not counted) for a complete TUI client with E2EE, cache and attachments; Hermes' native GUI layer is roughly 3x that in Swift on top of a ≈231 kB Go core. A Windows GUI client of similar scope would plausibly sit between the two. The very short calendar time of both repos suggests heavy tooling/AI assistance or prior private work — commit histories were not inspected, so development time in weeks is NOT established.
- The protocol-facing Go core (≈230–310 kB in both projects) is the reusable part; since both are AGPL-3.0, a self-written client can legally start from either core (staying AGPL), which removes most protocol-integration effort and leaves mainly the Windows UI.
- Budget ongoing maintenance: every comparable shows periodic breakage requiring an upstream mautrix-meta bump or local patch.

### Gaps
- No development-time (weeks) data for whatsapp-cli, wuzapi, whatscli, or other whatsmeow clients was collected; LOC for those not gathered.
- Commit counts / contributor counts / real line counts for lazyfmess and Hermes not inspected (only byte totals).
- No runtime RAM figure for nchat or any whatsmeow-based client found.
- No evidence found of Messenger support requests in other multi-protocol native clients (only nchat's tracker was searched).
