# mautrix-meta / messagix as a foundation for a lightweight native Messenger client (state as of 2026-11-12)

Method note: repository facts were read directly from github.com/mautrix/meta `main` via the GitHub API on 2026-11-12 (files, commit list, issue search). docs.mau.fi, GitHub HTML pages and pkg.go.dev were read through a fetch tool that summarises pages, so wording from those is paraphrased unless quoted. Anything not confirmed is listed under "Gaps".

## 1. Project health (release, commits, issues, maintainers, licence, Go version)

### Takeaway
The project is very actively maintained: latest release v26.10 (tag `v0.2610.0`) on 2026-10-16, commits daily up to 2026-11-11, monthly release cadence. Licence is AGPL-3.0-or-later; minimum Go is 1.26.

### Cited Findings
- Latest release: name `v26.10`, tag `v0.2610.0`, published 2026-10-16T15:46:44Z — [GitHub API releases/latest](https://api.github.com/repos/mautrix/meta/releases/latest) (read 2026-11-12)
- Release assets for v26.10: `mautrix-meta-amd64`, `-arm`, `-arm64`, `-darwin-arm64` (same four for `mautrix-instagram`) plus `sha256sums.txt`. No Windows asset. — [GitHub API releases/latest](https://api.github.com/repos/mautrix/meta/releases/latest)
- 2026 release cadence (from CHANGELOG and releases page): v26.02 (16 Feb), v26.04 (16 Apr), v26.05 (16 May), v26.05.1 (29 May), v26.06 (16 Jun), v26.07 (16 Jul), v26.08 (16 Aug), v26.08.1 (10 Sep), v26.09 (16 Sep), v26.10 (16 Oct). — [CHANGELOG.md](https://github.com/mautrix/meta/blob/main/CHANGELOG.md), [Releases](https://github.com/mautrix/meta/releases)
- Versioning switched to calendar versioning in v25.10; previous scheme ended at v0.5.3 (2025-08-16). — [CHANGELOG.md](https://github.com/mautrix/meta/blob/main/CHANGELOG.md)
- Most recent commit on `main`: f5d5c35a, 2026-11-11T11:51:13Z, "messagix/dgw: stop waiting on a full frame queue once the socket is closed (#551)". The 30 most recent commits span only 2026-11-03 to 2026-11-11. — [commits](https://github.com/mautrix/meta/commits/main)
- Commit authors in those 30 commits: Gerardo Rodriguez (most), `indent[bot]` (automated PRs), Tulir Asokan, Nick Mills-Barrett, Lucas Lima, Toni Spets, "Highest". Source file copyright headers name Tulir Asokan. — [commits](https://github.com/mautrix/meta/commits/main), [e2ee-client.go](https://github.com/mautrix/meta/blob/main/pkg/messagix/e2ee-client.go)
- Repo metadata on 2026-11-12: 477 stars, 99 forks, created 2024-01-16, not archived, licence SPDX `AGPL-3.0`, last push 2026-11-12T03:49:02Z. `open_issues_count` = 97 (this API field counts issues + PRs). — [GitHub API repo](https://api.github.com/repos/mautrix/meta)
- Open issues (issues only): 38; open pull requests: 59. — [issue search `is:issue is:open`](https://github.com/mautrix/meta/issues?q=is%3Aissue+is%3Aopen) and [Releases page header](https://github.com/mautrix/meta/releases) (both read 2026-11-12)
- Licence text in source headers: "GNU Affero General Public License ... either version 3 of the License, or (at your option) any later version". — [e2ee-client.go](https://github.com/mautrix/meta/blob/main/pkg/messagix/e2ee-client.go)
- `LICENSE.exceptions` grants exceptions only to Beeper (embedding in Beeper clients/servers) and Element (binaries in Element Server Suite), conditional on modifications staying public under AGPLv3+. No general exception for third-party apps. — [LICENSE.exceptions](https://github.com/mautrix/meta/blob/main/LICENSE.exceptions)
- `go.mod`: module `go.mau.fi/mautrix-meta`, `go 1.26.0`, `toolchain go1.27.3`. Minimum Go bumped to 1.26 in v26.09, 1.25 in v26.02. — [go.mod](https://github.com/mautrix/meta/blob/main/go.mod), [CHANGELOG.md](https://github.com/mautrix/meta/blob/main/CHANGELOG.md)
- Since v26.07/v26.08 the repo holds two bridges: `mautrix-meta` (Messenger only since v26.08) and `mautrix-instagram`. v26.06 changelog: "it appears that Meta has abandoned their plans to unify their messengers and are now moving to different protocols again." — [README.md](https://github.com/mautrix/meta/blob/main/README.md), [CHANGELOG.md](https://github.com/mautrix/meta/blob/main/CHANGELOG.md)

### Inferences
- AGPL implication for a personal tool: linking `pkg/messagix` into your own program makes the combined work AGPL-covered. For a program you only run yourself and do not distribute or offer to others over a network, the source-disclosure obligations are not triggered in practice; they apply as soon as you hand the binary to anyone else. (My reading of AGPLv3, not a statement from the project; not legal advice.)
- A large share of recent commits come from an automated bot account and one Beeper-affiliated contributor, which suggests the bridge is maintained as part of Beeper's product rather than as a hobby project. Affiliation of individual contributors was not verified.

### Gaps
- An explicit "maintainers" list was not found; inferred only from commit authors and copyright headers.
- Exact dates with year for releases before v26.10 were taken from the CHANGELOG order plus day/month on the releases page; only v26.10's full timestamp was verified via the API.

## 2. Meta surfaces / login modes available today

### Takeaway
For Messenger there are four login flows in code today: cookie flows for facebook.com and messenger.com, and two username/password "native" flows that imitate the Messenger iOS and Messenger Android (Lite) apps. A Tor onion variant of the facebook.com flow exists behind a config flag. Instagram has moved to a separate bridge binary.

### Cited Findings
- Flow IDs in `pkg/connector/login.go`: `facebook` ("Login using cookies from facebook.com"), `messenger` ("Login using cookies from messenger.com"), `messenger-lite` ("Login with username/password using Messenger iOS API"), `messenger-lite-android` ("Login with username/password using Messenger Android API"). — [login.go](https://github.com/mautrix/meta/blob/main/pkg/connector/login.go)
- Platform types handled by `messagix.Client`: `Facebook`, `FacebookTor`, `Messenger`, `MessengerLiteIOS`, `MessengerLiteAndroid`. — [client.go](https://github.com/mautrix/meta/blob/main/pkg/messagix/client.go)
- Tor: config `tor: false`; when true with a Tor proxy the bridge uses `facebookwkhpilnemxj7asaniu7vnjjbiltxjqhye3mhbshg7kx5tfyd.onion` instead of facebook.com. — [example-config.yaml](https://github.com/mautrix/meta/blob/main/pkg/connector/example-config.yaml), [endpoints/facebook.go](https://github.com/mautrix/meta/blob/main/pkg/messagix/endpoints/facebook.go)
- History of native modes: "experimental iOS Messenger login mode, which doesn't require using a browser" added v26.02; bug fixes in v26.04, v26.05, v26.06; "added Android Messenger as an option" v26.08; "Improved Messenger mobile login modes" v26.09 and v26.10. — [CHANGELOG.md](https://github.com/mautrix/meta/blob/main/CHANGELOG.md)
- docs.mau.fi authentication page (read 2026-11-12) lists for Facebook/Messenger: `facebook` or `messenger` use cookies, `messenger-lite` uses username and password. — [docs.mau.fi authentication](https://docs.mau.fi/bridges/go/meta/authentication.html)
- Instagram support was removed from mautrix-meta in v26.08; encrypted chats on Instagram were removed in v26.04. — [CHANGELOG.md](https://github.com/mautrix/meta/blob/main/CHANGELOG.md)

### Inferences
- The native "Messenger Lite" flows only replace the login step: after login the code calls the same `loginWithCookies` / `LoadMessagesPage` path as cookie logins, so the message transport afterwards is the same web-style LightSpeed sync (see section 6).

### Gaps
- The docs do not state which mode is "recommended" in 2026. The fetched docs page describes the cookie flow step by step and mentions `messenger-lite` as the password alternative; no explicit recommendation was found.
- The docs page fetched did not mention `messenger-lite-android`, although the code has it; docs may lag (an open issue from 2025-03-19, #125, is titled "Authentication documentation is outdated" — [issue list](https://github.com/mautrix/meta/issues?q=is%3Aissue+is%3Aopen)).

## 3. Login mechanics, cookies, 2FA, session lifetime, logouts

### Takeaway
Cookie login needs `xs`, `c_user` and `datr` (docs also list `sb`); password login exists through the Messenger iOS/Android flows and had 2FA bugs fixed as recently as October 2026. Session lifetime is not documented; user reports range from about 7 days to 3+ months, and Meta's "automated behaviour" checkpoint is a long-standing open problem.

### Cited Findings
- Required Facebook cookies in code: `FBRequiredCookies = {xs, c_user, datr}`. Other known cookie names in code: `sb`, `fr`, `wd`, `presence`, `oo`, `dpr`. Code comments: `xs` "is the main session cookie for Facebook", `c_user` "contains the user ID", `datr` "seems to be a session ID that's displayed in security settings". — [cookies.go](https://github.com/mautrix/meta/blob/main/pkg/messagix/cookies/cookies.go)
- docs.mau.fi lists Facebook cookies to collect as `datr`, `c_user`, `sb`, `xs`. — [docs.mau.fi authentication](https://docs.mau.fi/bridges/go/meta/authentication.html)
- "Reduced number of required cookies" in v0.2.0 (2024-03-16). — [CHANGELOG.md](https://github.com/mautrix/meta/blob/main/CHANGELOG.md)
- Cookie login procedure per docs: send `login <mode>` to the bridge bot, log in to the site in a private browser window, copy a `graphql` XHR request as cURL (or build a JSON object of cookies) and paste it to the bot. Code instruction string: "Enter a JSON object with your cookies, or a cURL command copied from browser devtools." — [docs.mau.fi authentication](https://docs.mau.fi/bridges/go/meta/authentication.html), [login.go](https://github.com/mautrix/meta/blob/main/pkg/connector/login.go)
- Cookie login validates by loading the messages page; mapped failure types include `Challenge`, `Checkpoint`, `Consent`, `TokenInvalidated`, `RateLimited`, `InboxLoadFailed`. — [login.go](https://github.com/mautrix/meta/blob/main/pkg/connector/login.go)
- Password login: `MetaNativeLogin` drives `MessengerLite.DoLoginSteps` (a multi-step flow using an interpreter for Meta's "Bloks" UI payloads), then creates a normal cookie-based client. Endpoints used include `b-graph.facebook.com` / `graph.facebook.com` `pwd_key_fetch` and `auth/create_session_for_app`. — [login.go](https://github.com/mautrix/meta/blob/main/pkg/connector/login.go), [endpoints/facebook.go](https://github.com/mautrix/meta/blob/main/pkg/messagix/endpoints/facebook.go)
- 2FA in the password flow: issue #375 (opened 2026-10-01, closed 2026-10-07) "Facebook messenger-lite login: unimplemented function bk.action.core.While at MFA method step"; issue #373 (2026-09-30, closed 2026-10-07) "messenger-lite and messenger-lite-android login commands crashing". — [#375](https://github.com/mautrix/meta/issues/375), [#373](https://github.com/mautrix/meta/issues/373)
- docs.mau.fi warns that Meta may flag the account as suspicious and require a captcha, phone number or password reset, and recommends enabling two-factor authentication to reduce the risk. — [docs.mau.fi authentication](https://docs.mau.fi/bridges/go/meta/authentication.html)
- Issue #44 "Message from Facebook: 'We suspect automated behaviour on your account.'" — open since 2024-03-22, 38 comments, last updated 2026-08-26. Reports include logouts, blocked sessions and, in a few cases, disabled accounts (Apr 2024, Apr 2025). No maintainer reply in the thread. — [#44](https://github.com/mautrix/meta/issues/44)
- Session lifetime, user reports in #44 (anecdotal): one user (2026-08-25) saw a cookie session expire after about seven days and hypothesised a browser-fingerprint mismatch; another user said they had not been logged out in about three months; a 2025-08-06 report gives a warning roughly every one to two months. — [#44](https://github.com/mautrix/meta/issues/44)
- Connection-level settings: `force_refresh_interval_seconds: 72000` (full reconnect every 20 h by default), `min_full_reconnect_interval_seconds: 3600`; cached connection state is rejected after 24 h (`MaxCachedStateAge`). — [example-config.yaml](https://github.com/mautrix/meta/blob/main/pkg/connector/example-config.yaml), [client.go](https://github.com/mautrix/meta/blob/main/pkg/messagix/client.go)
- "Fixed detecting some types of logouts" (v0.5.0, 2025-06-16); a DGW close with status "unauthorized" is treated as a permanent error by `Client.Connect`. — [CHANGELOG.md](https://github.com/mautrix/meta/blob/main/CHANGELOG.md), [client.go](https://github.com/mautrix/meta/blob/main/pkg/messagix/client.go)

### Inferences
- The client sends a fixed user agent (`useragent.UserAgent`) regardless of which browser the cookies came from; the fingerprint-mismatch theory in #44 is plausible but unconfirmed by maintainers.

### Gaps
- No official statement on session lifetime or on what triggers logouts was found in the docs.
- Whether the iOS/Android password flows reduce checkpoint frequency versus cookie logins: no data found.
- Rate of account restrictions is unknown; #44 is self-selected anecdote.

## 4. End-to-end encryption (E2EE) support

### Takeaway
Encrypted Messenger chats are supported and are the default for new DMs since v26.02; the E2EE leg runs through whatsmeow over a second WebSocket. Encrypted history backfill arrived only in v26.10 (October 2026), needs the user's backup PIN, and is off by default.

### Cited Findings
- ROADMAP "Misc": `[x] Messenger encryption`. — [ROADMAP.md](https://github.com/mautrix/meta/blob/main/ROADMAP.md)
- Implementation: `PrepareE2EEClient` builds a `whatsmeow.Client` with `MessengerConfig` (WebSocket `wss://web-chat-e2ee.facebook.com/ws/chat`), client payload `Product: MESSENGER`, platform `BLUE_WEB`, authenticated with a "crypto auth token" (CAT) refreshed through GraphQL `MAWCatQuery`. — [e2ee-client.go](https://github.com/mautrix/meta/blob/main/pkg/messagix/e2ee-client.go), [endpoints/facebook.go](https://github.com/mautrix/meta/blob/main/pkg/messagix/endpoints/facebook.go)
- Device registration endpoints: `https://reg-e2ee.facebook.com/v2/fb_icdc_fetch` and `/v2/fb_register_v2` (web modes); `https://v.whatsapp.net/v2/fb_icdc_fetch` and `fb_register_v2` (Messenger Lite modes). A dedicated file `e2ee-register.go` exists. — [endpoints/facebook.go](https://github.com/mautrix/meta/blob/main/pkg/messagix/endpoints/facebook.go), [pkg/messagix tree](https://github.com/mautrix/meta/tree/main/pkg/messagix)
- Dependencies: `go.mau.fi/whatsmeow` and `go.mau.fi/libsignal` (pure-Go Signal protocol). — [go.mod](https://github.com/mautrix/meta/blob/main/go.mod)
- v26.02: "Changed starting chats to default to encrypted DMs on Messenger"; "Added auto-reconnect for WhatsApp connection used for encrypted chats". — [CHANGELOG.md](https://github.com/mautrix/meta/blob/main/CHANGELOG.md)
- v0.3.0 (2024-04-16): automatic detection "when Messenger force-enables encryption in a chat". — [CHANGELOG.md](https://github.com/mautrix/meta/blob/main/CHANGELOG.md)
- Encrypted history: v26.10 "Added support for backfilling encrypted Messenger chat history. Note: older unencrypted history will no longer be backfilled, but may be re-added in a future release." — [CHANGELOG.md](https://github.com/mautrix/meta/blob/main/CHANGELOG.md)
- Release post (2026-10-16): after login the Messenger bridge prompts for the backup PIN; this can be skipped if history is not wanted. — [mau.fi blog 2026-10](https://mau.fi/blog/2026-10-mautrix-release/)
- Config: `e2ee_backfill: false` — "Should the bridge prompt for the encrypted backup PIN on login and upload sent messages to encrypted history?" A `securehistory` package exists in messagix. — [example-config.yaml](https://github.com/mautrix/meta/blob/main/pkg/connector/example-config.yaml), [pkg/messagix tree](https://github.com/mautrix/meta/tree/main/pkg/messagix)
- Commit 2026-11-03: "securehistory: only upload sent messages to secure history when e2ee_backfill is enabled (#505)". — [commits](https://github.com/mautrix/meta/commits/main)
- ROADMAP, encrypted section, supported both directions: text, images, videos, files, voice messages, stickers, GIFs, formatting, replies, mentions, reactions, edits, unsend/redaction, typing, read receipts; incoming locations; group invite/kick, add/remove/leave. — [ROADMAP.md](https://github.com/mautrix/meta/blob/main/ROADMAP.md)
- ROADMAP, encrypted section, not supported (unchecked): sending locations, polls (both directions), "Writing to chat backup", "Message history/Reading chat backup", presence, changing room name/avatar from Matrix, live location, story/reel/profile/product shares. — [ROADMAP.md](https://github.com/mautrix/meta/blob/main/ROADMAP.md)
- Conflict: ROADMAP still shows encrypted history/backup as unchecked, while CHANGELOG v26.10 and the example config document it as implemented. ROADMAP appears stale on this point. — [ROADMAP.md](https://github.com/mautrix/meta/blob/main/ROADMAP.md) vs [CHANGELOG.md](https://github.com/mautrix/meta/blob/main/CHANGELOG.md)
- Other encrypted-chat changelog items: disappearing messages and story replies (v25.10), animated webp (v25.11), "more encrypted message types" and GIF sending fix (v26.04), push registrations for encrypted chats with the Messenger Android flow (v26.09), fallback for unsupported encrypted messages (v0.4.5). — [CHANGELOG.md](https://github.com/mautrix/meta/blob/main/CHANGELOG.md)
- Open E2EE-related issues: #105 stickers in encrypted chats fail "Failed to transfer media" (open since 2024-09-30, updated 2026-07-23); #106 "Cannot bridge view once media" (updated 2026-10-15); #379 link-share note dropped in encrypted chats (2026-10-06); #46 "Allow verifying encryption keys" (2024-03-31). — [open issues](https://github.com/mautrix/meta/issues?q=is%3Aissue+is%3Aopen)
- Commit 2026-11-06: "handlewhatsapp: bridge undecryptable e2ee messages as notices (#503)". — [commits](https://github.com/mautrix/meta/commits/main)

### Inferences
- A standalone client built on messagix must persist a whatsmeow device store (Signal keys, sessions) in addition to cookies; `PrepareE2EEClient` fails without a `store.Device`. That store in whatsmeow is SQL-backed, so some database is needed even without Matrix.
- The bridge registers itself as an additional E2EE device on the account (ICDC registration); the bridge name for the device as shown to the user was not checked.

### Gaps
- Calls (voice/video): not mentioned in ROADMAP for Messenger; only "notices for calls from Instagram" (v26.08). No evidence of Messenger call support; not confirmed either way beyond absence.
- The term "Labyrinth" does not appear in the files read; the feature is referred to as "encrypted backup PIN" / "secure history". Whether recovery-code or other non-PIN restore methods work was not verified.
- Docs at docs.mau.fi contain no E2EE section for mautrix-meta (the meta docs have only an Authentication sub-page). — [docs index](https://docs.mau.fi/bridges/go/meta/index.html)

## 5. Feature matrix (unencrypted Messenger, from ROADMAP.md)

### Takeaway
Core messaging is complete in both directions: text, images, video, files, voice messages, replies, mentions, reactions, edits, unsend, typing, read receipts, groups, history. Missing: presence, sending locations, per-chat nicknames.

### Cited Findings
- To Messenger (supported): text, files, voice messages, videos, images, formatting, replies, mentions, polls (groups), redactions, reactions, edits, typing notifications, read receipts, invite, kick, room name and avatar. Not supported: locations, presence, power levels, leave (ROADMAP unchecked), per-room nick. — [ROADMAP.md](https://github.com/mautrix/meta/blob/main/ROADMAP.md)
- From Messenger (supported): text, images, videos, GIFs, stickers, files, voice messages, locations, story/reel/clip shares, profile shares, formatting, replies, mentions, polls (groups), unsend, reactions, edits, message history, typing, read receipts, admin status, member add/remove/leave, chat metadata, user name and avatar. Not supported: live location, product shares, presence, per-chat nickname. — [ROADMAP.md](https://github.com/mautrix/meta/blob/main/ROADMAP.md)
- Newer than ROADMAP: leaving Messenger groups and blocking/unblocking users (v26.10); text formatting bridging (v26.07); marketplace chat backfill (v26.04); option to backfill all chats instead of the most recent 20, add/remove group participants (v26.02); typing in all directions, creating unencrypted groups (v25.10). — [CHANGELOG.md](https://github.com/mautrix/meta/blob/main/CHANGELOG.md)
- Polls in both directions merged to `main` on 2026-11-05 (#403), after v26.10. — [commits](https://github.com/mautrix/meta/commits/main)
- Presence: config comment "Full presence bridging is not supported"; `send_presence_on_typing` only affects E2EE chats. — [example-config.yaml](https://github.com/mautrix/meta/blob/main/pkg/connector/example-config.yaml)
- Open feature gaps in issues: "Messenger send image as HD" (#151), waveform for audio (#265), Meta AI (#280), additional profiles (#113), community group chats (#41). — [open issues](https://github.com/mautrix/meta/issues?q=is%3Aissue+is%3Aopen)

### Inferences
- The ROADMAP describes the bridge (Matrix↔Messenger conversion). The underlying messagix capabilities are at least as broad, but a custom client must implement its own conversion from LightSpeed tables / whatsmeow events to UI models.

### Gaps
- ROADMAP is partly stale (see section 4 conflict), so individual checkboxes may not match `main`.

## 6. Transport protocol and suitability for idle background use

### Takeaway
Current messagix does not use MQTT: it holds a persistent WebSocket to Meta's "DGW" gateway (`gateway.facebook.com/ws/lightspeed`) carrying LightSpeed sync payloads, plus HTTP GraphQL, plus a second WebSocket (whatsmeow) for E2EE. It is push-based, but the client sends a ping every 10 seconds.

### Cited Findings
- `NewClient` creates one `dgw.Socket` with `WSURL: cli.GetEndpoint("dgw_lightspeed")`; endpoint is `https://gateway.<host>/ws/lightspeed` where host is facebook.com or messenger.com. — [client.go](https://github.com/mautrix/meta/blob/main/pkg/messagix/client.go), [endpoints/facebook.go](https://github.com/mautrix/meta/blob/main/pkg/messagix/endpoints/facebook.go)
- DGW socket: binary WebSocket frames with stream multiplexing (ping/pong, establish-stream, data, ack, end-of-data, drain, deauth); URL parameters `x-dgw-appid`, `x-dgw-authtype`, `x-dgw-version=5`, `x-dgw-uuid`, `x-dgw-deviceid`; authenticated with the cookie header. — [dgw/socket.go](https://github.com/mautrix/meta/blob/main/pkg/messagix/dgw/socket.go)
- Timers: `PingInterval = 10 * time.Second`, `PongTimeout = 30 * time.Second`, `AckTimeout = 5s`, `ResponseTimeout = 30s`. — [dgw/socket.go](https://github.com/mautrix/meta/blob/main/pkg/messagix/dgw/socket.go)
- Incoming frames are JSON `PublishResponseData` whose payload is LightSpeed "steps" decoded into an `LSTable` and delivered to a single `EventHandler func(ctx, evt any)`; requests are LightSpeed task payloads (`SocketLSRequestPayload`). — [socket.go](https://github.com/mautrix/meta/blob/main/pkg/messagix/socket.go), [client.go](https://github.com/mautrix/meta/blob/main/pkg/messagix/client.go)
- Initial state comes from an HTTP load of the messages page (`LoadMessagesPage` parses page modules); GraphQL at `/api/graphql/`; media upload at `/ajax/mercury/upload.php`; native modes use the rupload API for media (commit 2026-11-05). — [client.go](https://github.com/mautrix/meta/blob/main/pkg/messagix/client.go), [endpoints/facebook.go](https://github.com/mautrix/meta/blob/main/pkg/messagix/endpoints/facebook.go), [commits](https://github.com/mautrix/meta/commits/main)
- Reconnect logic: exponential backoff capped at 5 minutes; immediate reconnect after a connection that lasted over 2 minutes. — [client.go](https://github.com/mautrix/meta/blob/main/pkg/messagix/client.go)
- v26.08.1 (10 Sep 2026): "Fixed connecting after a breaking change on the Messenger servers." — [CHANGELOG.md](https://github.com/mautrix/meta/blob/main/CHANGELOG.md)
- Web push / native push registration code exists (`web_push` endpoint, `nativepush.go`, `pushcrypto`); v26.09 added push registrations for encrypted chats with the Messenger Android flow. — [pkg/messagix tree](https://github.com/mautrix/meta/tree/main/pkg/messagix), [CHANGELOG.md](https://github.com/mautrix/meta/blob/main/CHANGELOG.md)
- Extra low-level deps suggest non-browser transports for native modes: `github.com/quic-go/quic-go`, `github.com/refraction-networking/utls`, `github.com/apache/thrift`, `github.com/google/flatbuffers`. — [go.mod](https://github.com/mautrix/meta/blob/main/go.mod)

### Inferences
- The file dates (dgw/socket.go copyright 2026) plus the v26.08.1 note suggest the move from the older MQTT-over-WebSocket (`edge-chat`) transport to DGW happened during 2026, probably forced by Meta. The exact commit/date of the switch was not traced.
- Idle cost: two long-lived TLS WebSockets with a 10-second ping on the main one means the process wakes at least every 10 s; CPU cost is negligible but it is not a zero-wake design. Go runtime baseline memory applies.

### Gaps
- No measurement of idle CPU or network traffic was found.
- Whether any MQTT code path remains for some mode was not exhaustively checked (only the top-level client wiring was read).

## 7. Reusability of pkg/messagix outside Matrix

### Takeaway
`pkg/messagix` is an importable Go package with no Matrix homeserver dependency in its client constructor, but it is undocumented, has no stable API, and maintainers have not answered the two-year-old request to support it as a library. A few third-party projects do embed it, usually by vendoring the whole repo.

### Cited Findings
- Constructor and core API: `messagix.NewClient(cookies *cookies.Cookies, logger zerolog.Logger, cfg *Config) *Client`; `LoadMessagesPage(ctx)`, `SetEventHandler`, `Connect(ctx)`, `Disconnect()`, `DumpState()/LoadState()`, `PrepareE2EEClient() (*whatsmeow.Client, error)`, `FetchMoreThreads`. Imports are zerolog, coder/websocket, go.mau.fi/util, whatsmeow and messagix sub-packages — no `maunium.net/go/mautrix` import in client.go. — [client.go](https://github.com/mautrix/meta/blob/main/pkg/messagix/client.go), [e2ee-client.go](https://github.com/mautrix/meta/blob/main/pkg/messagix/e2ee-client.go)
- The bridge layer (`pkg/connector`) is where bridgev2/Matrix types appear; it calls `messagix.NewClient(c, log, conn.getMessagixConfig())`. — [login.go](https://github.com/mautrix/meta/blob/main/pkg/connector/login.go)
- Issue #104 "mautrix-meta/messagix usable as a library" opened 2024-09-29 (label: enhancement), still open; asks for whatsmeow-style library support or docs, calls messagix documentation "very little to none"; one follow-up user comment 2025-04-05; no maintainer response. — [#104](https://github.com/mautrix/meta/issues/104)
- Example programs in the repo: only `cmd/lscli` (a debugging tool that decodes a LightSpeed payload from clipboard/stdin to JSON) and `cmd/dgwparse`; no example chat client. — [cmd/](https://github.com/mautrix/meta/tree/main/cmd), [lscli.go](https://github.com/mautrix/meta/blob/main/cmd/lscli/lscli.go)
- pkg.go.dev "Imported by" for `go.mau.fi/mautrix-meta/pkg/messagix` (module version shown v0.2610.0): one external package, `github.com/onelegdave/omachat/internal/messenger`. — [pkg.go.dev importedby](https://pkg.go.dev/go.mau.fi/mautrix-meta/pkg/messagix?tab=importedby)
- onelegdave/omachat: a desktop chat panel for the Omarchy Linux desktop (QML UI + Go helper) covering Google Messages, WhatsApp, Telegram and Messenger; README credits mautrix-meta for Messenger; Messenger login is "Pair from browser" using a Chromium-family browser session; "New encrypted Messenger chats must be started in Messenger and then refreshed in OmaChat"; AGPL-3.0-or-later from v0.4.3; created 2026-09-14, last push 2026-10-26, 9 stars. Linux/Wayland only. — [onelegdave/omachat](https://github.com/onelegdave/omachat)
- m008v/fbchat-v2: Python Messenger library for personal accounts; its `bridge-e2ee/` is a Go JSON-RPC helper (stdin/stdout) that uses mautrix-meta as a git submodule with a `replace` directive to receive/send E2EE messages; README states Windows, Linux and macOS support, Go 1.26.10+ for the helper; cookie or username/password login; Apache-2.0 for its own code; 144 stars. — [m008v/fbchat-v2](https://github.com/m008v/fbchat-v2)
- babyvibe/deplao-builder: desktop multi-account CRM-style app (Zalo, Facebook, Telegram) that vendors mautrix-meta under `src/bridge-e2ee/login/meta/` (48 matching files). — [GitHub code search](https://github.com/search?q=%22go.mau.fi%2Fmautrix-meta%2Fpkg%2Fmessagix%22+language%3AGo+-repo%3Amautrix%2Fmeta&type=code), [babyvibe/deplao-builder](https://github.com/babyvibe/deplao-builder)
- GitHub code search for the import path outside mautrix/meta returned 652 file hits; the first 50 results belonged to just two repos (deplao-builder, fbchat-v2). — [GitHub code search](https://github.com/search?q=%22go.mau.fi%2Fmautrix-meta%2Fpkg%2Fmessagix%22+language%3AGo+-repo%3Amautrix%2Fmeta&type=code)

### Inferences
- A sidecar pattern (small Go process exposing JSON-RPC, UI in another language) is the approach both fbchat-v2 and omachat chose; it is the most proven way to reuse messagix today.
- Because Meta changed transport in 2026 and the package has no API stability promise, a custom client would need to track upstream `main` closely (monthly at minimum) and adapt to breaking API changes.
- fbchat-v2's claim of Windows support for its Go helper is indirect evidence that messagix compiles and runs on Windows; not verified by building.

### Gaps
- Only the first 50 of 652 code-search hits were inspected; other importers (forks, vendored copies) were not enumerated.
- No third-party TUI/GUI Messenger client for Windows built on messagix was found.
- Quality/maintenance of the third-party projects was not audited; their source was not read beyond README/search hits.

## 8. Running the bridge itself (memory, database, Windows, homeserver, bbctl)

### Takeaway
No published memory figure for mautrix-meta was found. Running the full bridge on Windows is not a supported path: there are no Windows binaries, it needs a Matrix homeserver with appservice support (docs name Synapse) or a Beeper account via `bbctl`, and `bbctl` itself does not run natively on Windows.

### Cited Findings
- Prebuilt binaries: linux amd64/arm64/arm and macOS arm64 only (releases and CI config). — [GitHub API releases/latest](https://api.github.com/repos/mautrix/meta/releases/latest), [.gitlab-ci.yml](https://github.com/mautrix/meta/blob/main/.gitlab-ci.yml)
- Docs requirements (Go bridges setup page, read 2026-11-12): a Matrix homeserver that supports application services "such as Synapse"; PostgreSQL v16+; for manual compile Go 1.26+, and libolm3 with dev headers plus a C/C++ compiler for end-to-bridge encryption. — [docs.mau.fi Go bridge setup](https://docs.mau.fi/bridges/go/setup.html?bridge=meta)
- Build options per docs: `./build.sh`; `-tags nocrypto` to build without (Matrix) encryption; experimental `-tags goolm` uses a pure-Go libolm reimplementation so no C compiler or Olm headers are needed. — [docs.mau.fi Go bridge setup](https://docs.mau.fi/bridges/go/setup.html?bridge=meta)
- Troubleshooting docs: setting `CGO_ENABLED=0` "will also disable SQLite support". `go.mod` lists `github.com/mattn/go-sqlite3` (cgo) and `github.com/lib/pq` as indirect deps. — [docs.mau.fi troubleshooting](https://docs.mau.fi/bridges/general/troubleshooting.html), [go.mod](https://github.com/mautrix/meta/blob/main/go.mod)
- Neither the setup page nor the troubleshooting page mentions Windows; troubleshooting mentions only Synapse among homeservers. — [docs.mau.fi Go bridge setup](https://docs.mau.fi/bridges/go/setup.html?bridge=meta), [docs.mau.fi troubleshooting](https://docs.mau.fi/bridges/general/troubleshooting.html)
- bbctl (Beeper Bridge Manager): connects self-hosted bridges to the beeper.com Matrix server "without having to self-host a whole Matrix homeserver"; cannot be used with any other homeserver; official bridge list includes mautrix-meta (identifiers `meta, instagram, facebook`); bridges run in the foreground; "bbctl supports amd64 and arm64 on Linux and macOS. Windows is not supported natively, please use WSL."; requires a Beeper account. — [beeper/bridge-manager README](https://github.com/beeper/bridge-manager/blob/main/README.md)
- All bridgev2 bridges support appservice websockets (so no inbound port/TLS is needed with Beeper). — [beeper/bridge-manager README](https://github.com/beeper/bridge-manager/blob/main/README.md)
- Homeserver RAM, secondary/anecdotal only: one comparison site gives idle Conduit 70–100 MB vs idle Synapse 800 MB–1 GB without citing measurements; a forum user reports ~100 MB for a 3-user Conduit instance. — [selfhosting.sh Conduit vs Synapse](https://selfhosting.sh/compare/conduit-vs-synapse/) (undated, unsourced figures; low reliability)

### Inferences
- Bridge + homeserver + a Matrix client is three processes; even with the lightest homeserver this is architecturally heavier than a single small client embedding messagix, and does not meet a "tiny native client" goal on its own. This follows from the documented requirements, not from measured numbers.
- Using Beeper (cloud bridge or bbctl under WSL) removes the homeserver but replaces it with the Beeper desktop app, which is not a lightweight native client; Beeper client footprint was not researched here.
- For a custom client, only `pkg/messagix` + whatsmeow + a small store are needed; libolm/goolm are relevant only to Matrix end-to-bridge encryption and would not be required.

### Gaps
- Memory footprint of the mautrix-meta process: no figure found in docs, issues, or blog posts. Must be measured.
- Whether mautrix-meta builds and runs natively on Windows (with or without cgo/SQLite): not documented and not tested.
- Whether the docs still allow SQLite for the bridge database: the fetched summary listed only PostgreSQL v16+; SQLite driver is present in go.mod, but an explicit statement was not confirmed.
- Appservice compatibility and RAM of Conduit / conduwuit / continuwuity / tuwunel / Dendrite with mautrix bridges: no primary-source statement found in this pass; RAM figures above are unsourced secondary claims.
- A bridgev2 "standalone" mode that runs with no homeserver at all: none found in the docs or release post read; not confirmed to exist or not exist.
- The docs URL `docs.mau.fi/bridges/general/self-hosting-with-beeper.html` returned 404; the bbctl facts come from the GitHub README instead.

## 9. Known breakages 2025–2026 and fix speed

### Takeaway
Meta-side changes broke the bridge several times in 2026 (Messenger connection in September, Instagram in May and June, inbox loading in March); fixes typically shipped as point releases or on `main` within days to about three weeks. The "automated behaviour" checkpoint problem has been open since March 2024 with no fix.

### Cited Findings
- v26.08.1 (published 10 Sep 2026): "Fixed connecting after a breaking change on the Messenger servers." Released as an out-of-cycle cherry-pick six days before v26.09. — [CHANGELOG.md](https://github.com/mautrix/meta/blob/main/CHANGELOG.md), [Releases](https://github.com/mautrix/meta/releases)
- v26.05.1 (29 May 2026): "Fixed connecting to Instagram after an app ID change broke it"; v26.06 (16 Jun 2026): "Fixed connecting to Instagram again." — [CHANGELOG.md](https://github.com/mautrix/meta/blob/main/CHANGELOG.md)
- Issue #245 (opened 2026-03-10, closed 2026-03-20): Messenger bridge fails to load inbox with "version ID not found". Issue #233 (opened 2026-03-02, closed 2026-03-20): "Thread discovery fails: LSUpsertFolder and 8 other new Meta SPs unrecognised" on fresh login. — [#245](https://github.com/mautrix/meta/issues/245), [#233](https://github.com/mautrix/meta/issues/233)
- Issue #352 (opened 2026-09-14, closed 2026-09-15): bridge reports CONNECTED with healthy keepalives but silently stops receiving message events. — [#352](https://github.com/mautrix/meta/issues/352)
- Issue #346 (opened 2026-09-06, closed 2026-10-07): a single DGW "Unauthorized" close treated as permanent logout (Instagram bridge). — [#346](https://github.com/mautrix/meta/issues/346)
- Issues #373 and #375 (opened 2026-09-30 / 2026-10-01, both closed 2026-10-07): messenger-lite login crashes, including at the MFA step. — [#373](https://github.com/mautrix/meta/issues/373), [#375](https://github.com/mautrix/meta/issues/375)
- Earlier fixes: v0.5.2 (2025-07-16) "Fixed LSVersion finding error on some accounts"; v25.11 "Fixed bridge connection failing with certain FB/IG accounts"; v26.04 "Fixed bridge getting stuck instead of reporting an error for certain sync errors returned by Facebook servers"; v26.02 "Added retrying for 5xx responses from Meta". — [CHANGELOG.md](https://github.com/mautrix/meta/blob/main/CHANGELOG.md)
- Unresolved: #44 "We suspect automated behaviour on your account" open since 2024-03-22 (38 comments, latest 2026-08-26); #327 "No backfill on FB: Can't backfill with non-FB message ID" open since 2026-08-22. — [#44](https://github.com/mautrix/meta/issues/44), [open issues](https://github.com/mautrix/meta/issues?q=is%3Aissue+is%3Aopen)
- Recent hardening on `main` (Nov 2026): "messagix: give lightspeed sync more time and fall back when the page lacks it (#504)", "httpclient: switch proxy and retry once on 429 (#519)". — [commits](https://github.com/mautrix/meta/commits/main)

### Inferences
- Breakages cluster around Meta protocol/version changes and are fixed quickly upstream, but a custom client that pins a messagix version would stop working until it is rebuilt against the fix; an update mechanism is a practical necessity.
- Issue close dates are a proxy for fix dates; the fixing commits were not individually traced.

### Gaps
- Only 10 closed 2026 issues matching login/checkpoint/disconnect keywords were reviewed; discussions in the Matrix room #meta:maunium.net were not accessible.
- "This Week in Matrix" and Beeper blog posts were not searched in this pass.
