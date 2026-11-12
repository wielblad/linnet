# Unofficial / reverse-engineered Messenger libraries for PERSONAL accounts, other than mautrix-meta/messagix (state on 12 Nov 2026)

Method note (read first): every repository figure below (stars, archived flag, last push, licence, open issues) was read from the GitHub REST API, the npm registry, the npm downloads API and the PyPI JSON API on **2026-11-12**. README and issue quotes were fetched the same day. **Nothing was executed or logged in**: every "works / partially / dead" verdict is a reading of repository evidence (maintenance, README statements, issue threads), not a live test. "pushed" is GitHub's `pushed_at` (any branch); where I also read the default-branch commit log I give that date separately.

## 1. Comparison table: which candidates work in late 2026?

### Takeaway
Every candidate that can reach end-to-end encrypted (E2EE) 1:1 chats in 2026 does so by embedding mautrix-meta (messagix + whatsmeow) as a Go bridge, sidecar or FFI binary; no independent E2EE implementation was found. The classic libraries (fbchat, facebook-chat-api, purple-facebook, bitlbee-facebook, mautrix-facebook, libfb-js, fbchat-sharp, Messer, fb-messenger-cli) are unmaintained or archived, and the maintained non-E2EE ones (fca forks, fbchat-muqit) are cookie-only web-MQTT bot libraries that cannot message ordinary friends in E2EE threads.

### Cited Findings

Verdict key: WORKS = maintained in H2 2026 and claims E2EE; PARTIAL = maintained or recently maintained but no E2EE or with a stated breakage; DEAD = archived/unmaintained with reported login or send failures.

| Candidate | Lang | Stars | Archived | Last push / last default-branch commit | Latest release | Licence | Login | E2EE | Transport | Verdict (evidence-based, untested) |
|---|---|---|---|---|---|---|---|---|---|---|
| fbchat-dev/fbchat | Python | 1214 | no | pushed 2024-02-16; last master commit 2020-09-23 "Add unmaintained notice" | PyPI 1.9.7, 2020-06-08 | BSD-3 | email/password (broken per issue #678) | none | web HTTP + MQTT | DEAD |
| tulir/fbchat-asyncio | Python | 41 | yes | 2020-12-15 | PyPI 0.6.21, 2020-12-01 | BSD-3 | n/a | none | web | DEAD |
| togashigreat/fbchat-muqit | Python | 83 | no | 2026-09-22 | PyPI 1.2.2, 2026-04-30 | GPL-3.0 (+BSD parts) | cookies only | none; README says user-to-user sending unsupported | web MQTT (aiomqtt), JSON `/t_ms` | PARTIAL (groups/pages only) |
| m008v/fbchat-v2 | Python + Go bridge | 144 | no | 2026-10-26 | v2.3.4, 2026-10-21 (GitHub and PyPI) | Apache-2.0 | username/password or cookie; 2FA; checkpoint issues reported | yes, via Go bridge built on mautrix/meta submodule | web MQTT (paho-mqtt, edge-chat.facebook.com) + Go bridge | WORKS (but E2EE = mautrix-meta inside) |
| Schmavery/facebook-chat-api | Node | 1941 | yes | 2021-12-24 | npm 1.8.0, 2020-04-03 | MIT | email/password or appState | none | web MQTT | DEAD |
| fca-unofficial (npm, original fork) | Node | n/a | n/a | n/a | npm 1.3.10, 2020-07-20 | MIT | appState | none | web MQTT | DEAD (no release since 2020) |
| @xaviabot/fca-unofficial (XaviaTeam) | Node | 25 | no | 2025-02-18 | npm 1.4.0, 2024-03-20 | MIT | appState | none found | web MQTT | PARTIAL/stale |
| @dongdev/fca-unofficial (repo now donyst/fca-unofficial) | TypeScript | 30 | **yes** | 2026-04-11 | npm 4.0.3, 2026-04-11 | Apache-2.0 | appState recommended; email/password "prone to checkpoints and CAPTCHAs" | README does not mention E2EE | web HTTP/GraphQL + MQTT over WebSocket | PARTIAL (archived Apr 2026; no E2EE) |
| ws3-fca (nethgraves/ws3-fca) | Node | 8 | no | 2026-04-16 | npm 3.5.2, 2025-08-31 | MIT | appState | README does not mention E2EE | web MQTT | PARTIAL (no E2EE) |
| fca-delta (hmhung1) | Node | 4 | **yes** | 2025-12-07 | npm 3.9.0, 2025-12-07 | MIT | appState | not found | web MQTT | DEAD (archived) |
| nexus-fca (Nexus-016/Nexus-fCA) | Node | 12 | no | 2026-11-11 | npm 3.4.0, 2026-01-17 | MIT | appState, or email/password/TOTP | opt-in "E2EE Bridge (BETA)" needing a native binary | web MQTT ("MQIsdp/3", mqtt 5.16.1) with HTTP fallback | PARTIAL (E2EE is beta, native bridge) |
| stfca (sheikhtamimlover/ST-FCA) | Node | 4 | no | 2026-06-07 | npm 1.2.28, 2026-06-07 | MIT (npm) | credentials or appState | README claims "full E2EE" via `connectE2EE` bridge | web MQTT + E2EE bridge | PARTIAL (claim unverified, tiny project) |
| meta-messenger.js (yumi-team; original aiko-chan-ai) | Go + Node FFI | 32 (+7) | **yes** (both) | 2026-03-15 (last commit 2026-03-09) | v1.1.3, 2026-02-17 | AGPL-3.0 | cookies | yes, "using the mautrix-meta library (Go) through FFI binding" | whatever mautrix-meta uses | PARTIAL (worked Feb-Mar 2026, now archived) |
| dequis/purple-facebook | C (libpurple) | 950 | no | pushed 2026-08-23; last master commits 2026-03-04 are housekeeping (history merge, remove Travis) | v0.9.6, 2019-01-13 | GPL-2.0 | email/password; errors 405/406/5711 reported | none; E2EE chats unreachable (issue #547) | Android-app MQTT + Thrift (see section 4) | DEAD for practical use |
| bitlbee/bitlbee-facebook | C (BitlBee) | 283 | no | pushed 2023-04-01; last master commit 2021-02-25 | 1.2.2, 2021-02-13 | GPL-2.0 | email/password | none; E2EE contacts' messages do not show (issue #222) | same code family as purple-facebook | DEAD |
| mautrix/facebook (incl. `maufbapi`) | Python | 390 | **yes** | 2024-03-02 | n/a (not checked) | AGPL-3.0 | n/a | n/a | Android API + Android MQTT (`AndroidAPI`, `AndroidMQTT`) | DEAD ("This bridge is deprecated") |
| messlidger (slidge, Codeberg) | Python | n/a | not stated | last commit 2025-10-23 (lockfile); last substantive 2025-04-06 | PyPI 0.2.1, 2025-04-06 | n/a | n/a | not mentioned | built on mautrix-facebook | DEAD/PARTIAL (receive-only per README) |
| ChatPlug/libfb-js (npm `libfb`) | TypeScript | 62 | no | 2022-05-15 | npm 2.3.5, 2019-09-24 | GPL-2.0 (repo) / MIT (npm) | username/password | none | Android MQTT ("based on bitlbee-facebook") | DEAD |
| gave92/fbchat-sharp | C# | 64 | no | 2022-06-23 | n/a | unclear | email/password | none | port of fbchat | DEAD |
| mjkaufer/Messer (CLI) | Node | 451 | no | 2022-12-30 | npm 0.7.3, 2020-06-19 | ISC | n/a | none | facebook-chat-api family | DEAD |
| Alex-Rose/fb-messenger-cli | Node | 765 | no | 2022-12-07 | npm 2.1.1, 2018-10-05 | none stated | n/a | none | n/a | DEAD |
| erictran308/tuimeta (TUI) | Rust | 1 | no | 2026-11-09 | n/a (README mentions 0.5.0) | MIT | n/a | via mautrix-meta + whatsmeow | mautrix-meta | WORKS on paper (new, 1 star, untested) |

- Repository metadata for fbchat, fbchat-v2, fbchat-muqit, fbchat-asyncio, fbchat-sharp, facebook-chat-api, purple-facebook, bitlbee-facebook, mautrix/facebook, meta-messenger.js: read 2026-11-12 from the GitHub API — [fbchat-dev/fbchat](https://github.com/fbchat-dev/fbchat), [m008v/fbchat-v2](https://github.com/m008v/fbchat-v2), [togashigreat/fbchat-muqit](https://github.com/togashigreat/fbchat-muqit), [tulir/fbchat-asyncio](https://github.com/tulir/fbchat-asyncio), [gave92/fbchat-sharp](https://github.com/gave92/fbchat-sharp), [Schmavery/facebook-chat-api](https://github.com/Schmavery/facebook-chat-api), [dequis/purple-facebook](https://github.com/dequis/purple-facebook), [bitlbee/bitlbee-facebook](https://github.com/bitlbee/bitlbee-facebook), [mautrix/facebook](https://github.com/mautrix/facebook), [yumi-team/meta-messenger.js](https://github.com/yumi-team/meta-messenger.js), [aiko-chan-ai/meta-messenger.js](https://github.com/aiko-chan-ai/meta-messenger.js)
- fca-family repository metadata (2026-11-12): [donyst/fca-unofficial](https://github.com/donyst/fca-unofficial) (the npm package still points to github.com/dongp06/fca-unofficial, which the API resolves to donyst/fca-unofficial), [nethgraves/ws3-fca](https://github.com/nethgraves/ws3-fca) (npm points to NethWs3Dev/ws3-fca), [Nexus-016/Nexus-fCA](https://github.com/Nexus-016/Nexus-fCA), [sheikhtamimlover/ST-FCA](https://github.com/sheikhtamimlover/ST-FCA), [XaviaTeam/fca-unofficial](https://github.com/XaviaTeam/fca-unofficial), [hmhung1/fca-delta](https://github.com/hmhung1/fca-delta)
- npm versions, publish dates and last-week downloads (read 2026-11-12): facebook-chat-api 1.8.0 (2020-04-03), 298/week — [npm](https://www.npmjs.com/package/facebook-chat-api); fca-unofficial 1.3.10 (2020-07-20), 1,218/week — [npm](https://www.npmjs.com/package/fca-unofficial); ws3-fca 3.5.2 (2025-08-31), 1,236/week — [npm](https://www.npmjs.com/package/ws3-fca); @dongdev/fca-unofficial 4.0.3 (2026-04-11), 1,523/week — [npm](https://www.npmjs.com/package/@dongdev/fca-unofficial); @xaviabot/fca-unofficial 1.4.0 (2024-03-20), 167/week — [npm](https://www.npmjs.com/package/@xaviabot/fca-unofficial); fca-delta 3.9.0 (2025-12-07), 88/week — [npm](https://www.npmjs.com/package/fca-delta); nexus-fca 3.4.0 (2026-01-17), 74/week — [npm](https://www.npmjs.com/package/nexus-fca); stfca 1.2.28 (2026-06-07), 591/week — [npm](https://www.npmjs.com/package/stfca); meta-messenger.js 1.1.3 (2026-02-17), 146/week — [npm](https://www.npmjs.com/package/meta-messenger.js); @neoaz07/nkxfca 1.0.10 (2026-04-03), 62/week — [npm](https://www.npmjs.com/package/@neoaz07/nkxfca); libfb 2.3.5 (2019-09-24), 435/week — [npm](https://www.npmjs.com/package/libfb); messer 0.7.3 (2020-06-19), 6/week — [npm](https://www.npmjs.com/package/messer); fb-messenger-cli 2.1.1 (2018-10-05), 33/week — [npm](https://www.npmjs.com/package/fb-messenger-cli)
- PyPI versions (read 2026-11-12): fbchat 1.9.7 uploaded 2020-06-08 — [PyPI](https://pypi.org/project/fbchat/); fbchat-muqit 1.2.2 uploaded 2026-04-30 — [PyPI](https://pypi.org/project/fbchat-muqit/); fbchat-v2 2.3.4 uploaded 2026-10-21 — [PyPI](https://pypi.org/project/fbchat-v2/); fbchat-asyncio 0.6.21 uploaded 2020-12-01 — [PyPI](https://pypi.org/project/fbchat-asyncio/); messlidger 0.2.1 uploaded 2025-04-06 — [PyPI](https://pypi.org/project/messlidger/); there is no `maufbapi` package on PyPI (HTTP 404) — [PyPI](https://pypi.org/project/maufbapi/)
- fbchat's last default-branch commit is "Add unmaintained notice" dated 2020-09-23 — [fbchat commits](https://github.com/fbchat-dev/fbchat/commits/master)
- fbchat issue #678 (opened 2025-01-03): "Currently, This library is Unmaintained and Due to Facebook recent changes It doesn't work anymore"; "Login into Facebook account through Email and password is not working anymore. Even if you try your account will get Locked." — [fbchat #678](https://github.com/fbchat-dev/fbchat/issues/678)
- mautrix-facebook README: "This bridge is deprecated. mautrix-meta is recommended instead." Repository archived, last push 2024-03-02 — [mautrix/facebook](https://github.com/mautrix/facebook)
- messlidger README warns that it "cannot send messages from XMPP to Messenger anymore because the library used to communicate with Facebook Messenger is not maintained anymore"; receiving is said to still work; it is "based on slidge and mautrix-facebook"; latest commit 2025-10-23 is an automatic lockfile update — [codeberg.org/slidge/messlidger](https://codeberg.org/slidge/messlidger)
- meta-messenger.js README: "Using the mautrix-meta library (Go) through FFI binding, this library supports (almost) all features that the original library has implemented"; requirements "Node.js >= 22" and "Cookies from a logged-in Facebook/Messenger account"; it has `e2eeMessage` events and E2EE device-state persistence — [yumi-team/meta-messenger.js README](https://github.com/yumi-team/meta-messenger.js)
- fbchat-v2 README (English): "It uses async/await, with a Go bridge for E2EE"; "Receive E2EE personal messages (Secret Conversations / Labyrinth) through the Go bridge"; the bridge directory contains "meta/ # mautrix-meta Git submodule" and `go.mod` "uses a replace directive pointing to ./meta"; Go toolchain "Only required for E2EE"; the bridge "has a binary build and distribution workflow for Windows, Linux, and macOS"; bridge RAM "about 80-150 MB while running" — [fbchat-v2 README_EN](https://github.com/m008v/fbchat-v2/blob/main/README_EN.md)
- matterbridge has no native Messenger support; its README only links third-party helpers fbridge and fbridge-asyncio — [42wim/matterbridge](https://github.com/42wim/matterbridge)
- Context that affects every web-protocol library: Meta announced in February 2026 that the standalone messenger.com website would stop being available in April 2026, with users sent to facebook.com/messages — [TechCrunch, 2026-02-19](https://www.techcrunch.com/2026/02/19/meta-is-shutting-down-messengers-standalone-website/); [Engadget](https://www.engadget.com/apps/meta-is-shuttering-messengers-standalone-website-which-is-a-thing-that-exists-191808134.html)

### Inferences
- The only actively maintained, non-archived library in this scope that claims E2EE and shipped a release in the last 30 days is fbchat-v2 (v2.3.4, 2026-10-21), and its E2EE path is mautrix-meta compiled into a helper binary. Choosing it means depending on mautrix-meta anyway, plus a Python layer.
- Three of the more serious 2026 Node efforts were archived within months (meta-messenger.js in spring 2026, @dongdev/fca-unofficial after 2026-04-11, fca-delta after 2025-12-07). No reason for archiving was found in the READMEs; the pattern suggests poor longevity for single-maintainer projects in this space.
- A licence consequence follows from the dependency: anything that links mautrix-meta inherits AGPL-3.0 obligations (meta-messenger.js is AGPL-3.0 for that reason; matpb/meta-mcp-rs says so explicitly, see section 6).

### Gaps
- No live login or message test was run for any library; verdicts are documentary.
- Why donyst/fca-unofficial, yumi-team/meta-messenger.js and hmhung1/fca-delta were archived: not stated in the material read.
- Whether the messenger.com shutdown actually took effect on the announced date and what it broke in these libraries: only pre-shutdown announcements were found, plus one third-party README saying "As of April 2026, standalone Messenger-only accounts are no longer accessible" ([rozsazoltan/messenger](https://github.com/rozsazoltan/messenger)).
- Latest tagged release of mautrix/facebook and of messlidger on Codeberg: not read.
- Stars for the original `fca-unofficial/fca-unofficial` repo: not read.
- Beeper's stack was not researched here (it is built on mautrix-meta, which another researcher covers).

## 2. Does login work in 2026 (password vs cookie import, 2FA, checkpoints)?

### Takeaway
Cookie/appState import from a browser session is the only login path the maintained libraries recommend; email/password login is described as broken, lock-inducing or checkpoint-prone by the projects themselves. The Android-API password login used by purple-facebook has fresh 2026 failure reports with no fix.

### Cited Findings
- fbchat-muqit README: "To login in Facebook you will need Facebook account cookies. Since login via email and password is no longer supported." — [fbchat-muqit README](https://github.com/togashigreat/fbchat-muqit)
- @dongdev/fca-unofficial README, authentication table: "`email` + `password` | Web login credentials. Prone to checkpoints and CAPTCHAs; not recommended for long-running bots." — [donyst/fca-unofficial README](https://github.com/donyst/fca-unofficial)
- nexus-fca README lists typed lock patterns detected at login: "`checkpoint`, `unusual_activity`, `account_locked`, `temporarily_blocked`, `verification_required`, `login_approval`, `cookie_invalid`", and offers "Email/Password/TOTP-2FA" login that generates an appState — [Nexus-fCA README](https://github.com/Nexus-016/Nexus-fCA)
- fbchat-v2 supports "username/password or session cookie"; open issue #30 (2026-05-25) reports correct username, password and 2FA still failing with "code=401 subcode=1348131"; issue #27 (2026-04-16, closed) reports a login checkpoint, "OAuthException code 405, subcode 1348023" — [fbchat-v2 #30](https://github.com/m008v/fbchat-v2/issues/30), [fbchat-v2 #27](https://github.com/m008v/fbchat-v2/issues/27)
- purple-facebook issue #565 (2026-07-09): the plugin "writes only this: Go to the app store on your device and update the Messenger app to log in. (5711)"; closed 2026-07-13 by the author with no fix, who wrote "I'm using Caprine." — [purple-facebook #565](https://github.com/dequis/purple-facebook/issues/565)
- Older purple-facebook login failures still open: #550 "Return of the dreaded 405 error" (2024-06-30, 10 comments), #552 "Facebook login Issue" with a Thrift assertion failure (2025-02-27), #546 "Login Approvals" (406, 2023-12-01), #544 "You're temporarily blocked" (2023-11-27) — [#550](https://github.com/dequis/purple-facebook/issues/550), [#552](https://github.com/dequis/purple-facebook/issues/552), [#546](https://github.com/dequis/purple-facebook/issues/546), [#544](https://github.com/dequis/purple-facebook/issues/544)
- purple-facebook has had no maintainer since at least 2021: bitlbee-facebook #214 "Looking for new maintainer" (2021-05-18) forwards to purple-facebook #518; a user wrote on 2024-03-31 "This plugin has no maintainer though." — [bitlbee-facebook #214](https://github.com/bitlbee/bitlbee-facebook/issues/214), [purple-facebook #547](https://github.com/dequis/purple-facebook/issues/547)
- meta-messenger.js issue #11 (2026-02-10, open): login error although the reporter is "absolutely certain that the cookies are still valid" — [meta-messenger.js #11](https://github.com/yumi-team/meta-messenger.js/issues/11)

### Inferences
- For a self-written client, the realistic login design in 2026 is "user logs in with a real browser, client imports the session cookies". Implementing password + 2FA login directly is the part every project reports as fragile.
- The 2026 purple-facebook activity (history merge by Pidgin developers in March 2026, per [pidgin/purple-facebook-converge](https://github.com/pidgin/purple-facebook-converge) and [PR #557](https://github.com/dequis/purple-facebook/pull/557)) is repository housekeeping, not a protocol revival; the July 2026 login error postdates it.

### Gaps
- No 2026 issue was found that confirms a successful purple-facebook or bitlbee-facebook login; absence of success reports is not proof of total failure.
- Cookie lifetime in practice (how often a session must be re-imported) is not quantified by any source read.

## 3. E2EE: do these libraries see ordinary friend chats at all?

### Takeaway
Libraries without E2EE cannot interact with E2EE 1:1 threads: messages from those contacts do not arrive and sends fail or drop the connection, as documented for purple-facebook, bitlbee-facebook and fbchat-muqit. Groups and pages remain reachable through the plain web MQTT path.

### Cited Findings
- purple-facebook issue #547 "End-to-End Encryption and purple-facebook" (opened 2024-02-03, 60 comments, still open; latest activity 2025-08-21): when E2EE is on for a conversation "you can no longer interact with it from your pidgin. you no longer receive messages concerning it"; sending gives "Failed to send message"; 2024-03-28 comment: "While someone with E2E encryption enable[d], this is just useless."; 2025-04-18 comment: "Facebook is removing non-encrypted messages as a whole."; no fork is confirmed to support E2EE — [purple-facebook #547](https://github.com/dequis/purple-facebook/issues/547)
- purple-facebook #549 "can't send messages anymore" (2024-04-07, 15 comments): "doing so kicks me off. Receiving messages works fine" — [purple-facebook #549](https://github.com/dequis/purple-facebook/issues/549)
- bitlbee-facebook issue #222 (2024-04-17, open): "Message from contacts that have end-to-end encryption enabled don't show up. Sending them a message with `/msg` crashes the connection and bitlbee reconnects." — [bitlbee-facebook #222](https://github.com/bitlbee/bitlbee-facebook/issues/222)
- fbchat-muqit README warning: "Due to end-to-end encryption sending messages to other Users is not supported anymore. You can send messages to Group Chat, Room Chat and to pages." It adds that sending may still work to accounts "inactive for many years". Its quick-start tells users to test through a group chat — [fbchat-muqit README](https://github.com/togashigreat/fbchat-muqit)
- tas33n/facebook-api-mcp (an fca-style Node library plus MCP server, created 2026-11-05): "Native E2EE support is not implemented. Cookie sessions may expire." — [tas33n/facebook-api-mcp](https://github.com/tas33n/facebook-api-mcp)
- nexus-fca: E2EE is "opt-in ... (BETA, needs native deps)", "requires: npm i koffi yumi-json-bigint (+ Labyrinth native binary)" — [Nexus-fCA README](https://github.com/Nexus-016/Nexus-fCA)
- ST-FCA README claims "full End-to-End Encryption support" with `enableE2EE: true`, `api.connectE2EE`, and E2EE thread IDs in JID form ("61568577897207:69@msgr") — [ST-FCA README](https://github.com/sheikhtamimlover/ST-FCA)
- E2EE history is not retrievable server-side: fukata/messenger-mcp states "Past end-to-end encrypted messages cannot be fetched through the API"; matpb/meta-mcp-rs states "Encrypted Messenger history from before the device was registered is not available." — [fukata/messenger-mcp](https://github.com/fukata/messenger-mcp), [matpb/meta-mcp-rs](https://github.com/matpb/meta-mcp-rs)
- A browser-driving CLI describes the same reality from the web side: "Most Messenger chats are end-to-end encrypted. Their history needs your Messenger PIN, because a new browser profile has no keys." — [hieu-lee/facebook-cli README](https://github.com/hieu-lee/facebook-cli)

### Inferences
- The `@msgr` JID thread format and the `koffi` + `yumi-json-bigint` dependencies in nexus-fca and ST-FCA indicate their "E2EE bridge" is the meta-messenger.js native binary (itself mautrix-meta via FFI), not an independent implementation. This is an inference from naming; neither README names its bridge source.
- For the stated goal (chatting with Facebook friends 1:1), any library lacking E2EE is functionally unusable in 2026, regardless of how actively it is maintained.

### Gaps
- No issue thread inside the fca forks (dongdev, ws3-fca) was found that explicitly describes E2EE 1:1 chats being invisible; their READMEs simply do not mention E2EE. ws3-fca issue #14 "Error While Sending DM msgs" (2025-07-27) has no text error and no replies — [ws3-fca #14](https://github.com/nethgraves/ws3-fca/issues/14).
- The share of a typical user's 1:1 threads that are E2EE in 2026 is not quantified by any source read.
- ST-FCA's and nexus-fca's E2EE claims are unverified.

## 4. Transport and push delivery

### Takeaway
The living libraries use the web client's MQTT-over-WebSocket channel (push-based) plus HTTP/GraphQL; the Android-native MQTT implementations (purple-facebook, bitlbee-facebook, libfb-js, maufbapi) are all unmaintained. No maintained 2025-2026 library implementing the Android/Messenger Lite MQTT protocol or Bloks login for Messenger was found outside mautrix-meta.

### Cited Findings
- @dongdev/fca-unofficial: "It speaks the same HTTP/GraphQL and MQTT protocols the browser client uses"; realtime is a "Persistent WebSocket connection to Facebook's MQTT broker" delivering messages, reactions, typing, presence and read receipts — [donyst/fca-unofficial README](https://github.com/donyst/fca-unofficial)
- nexus-fca: "Runs on mqtt 5.16.1 with Facebook's exact `MQIsdp/3` protocol and custom `wsOptions` headers"; send fallback chain "MQTT publish -> HTTP send -> direct -> OldMessage" — [Nexus-fCA README](https://github.com/Nexus-016/Nexus-fCA)
- fbchat-v2 depends on "paho-mqtt>=1.6.1" and requires the network not be "blocked from facebook.com and edge-chat.facebook.com" — [fbchat-v2 README_EN](https://github.com/m008v/fbchat-v2/blob/main/README_EN.md)
- fbchat-muqit issue #39 (2026-02-06): a user observed that browser WebSocket traffic on messenger.com and facebook.com showed only Lightspeed's binary protocol on `/ls_resp`, while fbchat-muqit expects JSON on `/t_ms`, and asked whether the JSON MQTT path is legacy. No maintainer answered; the reporter wrote on 2026-03-02 "No, given it up." — [fbchat-muqit #39](https://github.com/togashigreat/fbchat-muqit/issues/39)
- mautrix-facebook's `maufbapi` exports `AndroidAPI`, `AndroidMQTT`, `AndroidState` (Android-app protocol, paho MQTT v3.1) and lives only inside the archived repository — [maufbapi/__init__.py](https://github.com/mautrix/facebook/blob/master/maufbapi/__init__.py), [maufbapi/mqtt/conn.py](https://github.com/mautrix/facebook/blob/master/maufbapi/mqtt/conn.py)
- libfb-js: "Facebook chat MQTT library for Node.js"; "MQTT connection logic was based on bitlbee-facebook"; last push 2022-05-15 — [ChatPlug/libfb-js](https://github.com/ChatPlug/libfb-js)
- purple-facebook's protocol layer uses Thrift over MQTT (error text "Failed to read thrift: api.c:1897 fb_api_cb_publish_pt") and presents itself to Facebook as the Android Messenger app (the 5711 error asks to "update the Messenger app") — [purple-facebook #552](https://github.com/dequis/purple-facebook/issues/552), [purple-facebook #565](https://github.com/dequis/purple-facebook/issues/565)

### Inferences
- The fca/fbchat family relies on a legacy JSON MQTT topic set that the official web client reportedly no longer uses (issue #39). That is a single unanswered user observation, but if correct it is a structural risk: Meta could retire that path without affecting its own clients.
- Receiving is push-based in every living candidate (MQTT over WebSocket); none relies on HTTP polling.

### Gaps
- Exact WebSocket endpoint and topic list used by each fca fork were not read from source code.
- No dedicated search hit for a 2025-2026 standalone Messenger "Bloks login" or `mqtt-mini.facebook.com` library; GitHub search was limited by unauthenticated rate limits, so this is "not found", not "does not exist".

## 5. Real-world use in 2026 and account-ban reports

### Takeaway
There is a live but small-scale bot scene around fca forks and fbchat-v2 (Vietnamese and Bangladeshi/Filipino authors, weekly npm downloads in the low thousands at most), and every maintained project warns of locks, checkpoints or bans; several tell users to use a throwaway account.

### Cited Findings
- Download scale (last week, read 2026-11-12): @dongdev/fca-unofficial 1,523; ws3-fca 1,236; fca-unofficial 1,218; stfca 591; meta-messenger.js 146; nexus-fca 74 — [npm @dongdev/fca-unofficial](https://www.npmjs.com/package/@dongdev/fca-unofficial), [npm ws3-fca](https://www.npmjs.com/package/ws3-fca), [npm fca-unofficial](https://www.npmjs.com/package/fca-unofficial), [npm stfca](https://www.npmjs.com/package/stfca), [npm meta-messenger.js](https://www.npmjs.com/package/meta-messenger.js), [npm nexus-fca](https://www.npmjs.com/package/nexus-fca)
- New fca-style repositories keep appearing in H2 2026 (e.g. alaminnna/al-fca pushed 2026-11-01, frnAlt/Floppa-Chatbot pushed 2026-10-28, yohannzapata/fca-unofficial created 2026-09-30, GoatBot-style bots advertising a "custom anti-ban FCA engine") — [al-fca](https://github.com/alaminnna/al-fca), [Floppa-Chatbot](https://github.com/frnAlt/Floppa-Chatbot), [yohannzapata/fca-unofficial](https://github.com/yohannzapata/fca-unofficial), [frnAlt/facebookChatBot](https://github.com/frnAlt/facebookChatBot)
- fbchat-v2's README and issue tracker are primarily Vietnamese, with a Telegram support channel; recent issues are dated 2026-04 to 2026-10 — [fbchat-v2](https://github.com/m008v/fbchat-v2), [fbchat-v2 #35](https://github.com/m008v/fbchat-v2/issues/35)
- fbchat-muqit: "we are not responsible if you get banned by Facebook. We recommend to use a dummy Facebook account." — [fbchat-muqit README](https://github.com/togashigreat/fbchat-muqit)
- meta-messenger.js: "Using this feature on user accounts is prohibited under Meta's Terms of Service and may result in account suspension." — [meta-messenger.js README](https://github.com/yumi-team/meta-messenger.js)
- tuimeta: "an account can be locked, checkpointed or banned for it ... try it with an account you can afford to lose first." — [tuimeta README](https://github.com/erictran308/tuimeta)
- matpb/meta-mcp-rs: "users of the Messenger library report checkpoints and 'suspect automated..." behaviour locks; "Will personal mode get my account banned? Nobody can promise it will not." — [meta-mcp-rs README](https://github.com/matpb/meta-mcp-rs)
- fbchat issue #675 (2024-09-03): "everytime i logged in using this tool, they always restrict me when the bot receive 2 or more messages." — [fbchat #675](https://github.com/fbchat-dev/fbchat/issues/675)
- A trust warning specific to this ecosystem: ws3-fca issue #10 "Remove the backdoor." (2025-06-01, closed) accuses the package of making users' accounts "auto follow / share your accounts / posts without consent"; fbchat-v2 PR #31 (2026-05-31) removed a live session cookie that had been committed to the repo — [ws3-fca #10](https://github.com/nethgraves/ws3-fca/issues/10), [fbchat-v2 PR #31](https://github.com/m008v/fbchat-v2/pull/31)

### Inferences
- The fca ecosystem is bot-oriented (group-chat command bots), which matches what still works without E2EE (groups). It is not evidence that personal 1:1 chatting works through those libraries.
- Supply-chain risk is real in this scene: packages run with full account cookies, at least one was accused of hidden account actions, and fbchat-v2 auto-downloads a prebuilt bridge binary (a user asked how to verify it in issue #35). Code from these projects should be audited or built from source before being given a real session.

### Gaps
- No quantified ban rate exists; all ban evidence is anecdotal or disclaimer text.
- Reddit / Hacker News threads from 2025-2026 were not searched (tool-call budget went to primary repositories).

## 6. Native (non-Electron, non-webview) desktop/TUI clients and MCP servers for personal Messenger

### Takeaway
No established native Windows Messenger client built on these libraries was found; the old CLIs (Messer, fb-messenger-cli) are dead, and the one new protocol-level TUI (tuimeta, Rust, October 2026, 1 star) sits on mautrix-meta. A cluster of personal-account MCP servers appeared in October-November 2026, all with 0-1 stars, and every one that handles E2EE uses mautrix-meta (directly, as a sidecar, or through fbchat-v2's bridge).

### Cited Findings
- Messer: 451 stars, last push 2022-12-30, npm 0.7.3 from 2020-06-19, 6 downloads/week — [mjkaufer/Messer](https://github.com/mjkaufer/Messer), [npm messer](https://www.npmjs.com/package/messer)
- fb-messenger-cli: 765 stars, last push 2022-12-07, npm 2.1.1 from 2018-10-05, 75 open issues — [Alex-Rose/fb-messenger-cli](https://github.com/Alex-Rose/fb-messenger-cli)
- tuimeta: Rust terminal client for Messenger, Instagram and WhatsApp; "it speaks Messenger's, Instagram's, WhatsApp's and Threads' own protocols, through mautrix-meta and whatsmeow"; created 2026-10-08, pushed 2026-11-09, MIT, 1 star — [erictran308/tuimeta](https://github.com/erictran308/tuimeta)
- an920107/messenger-flatpak calls itself "native GTK4 / Libadwaita" but is "Written in Rust with WebKitGTK 6.0", i.e. a webview wrapper, Linux only — [messenger-flatpak README](https://github.com/an920107/messenger-flatpak)
- Other 2026 "desktop client" repositories found are Tauri/webview wrappers (NeedsAdjustment/mesa "built with Tauri", wickedtech/messenger-desktop "powered by Tauri 2.x", rozsazoltan/messenger at 98 stars) and so fall under the excluded category — [mesa](https://github.com/NeedsAdjustment/mesa), [messenger-desktop](https://github.com/wickedtech/messenger-desktop), [rozsazoltan/messenger](https://github.com/rozsazoltan/messenger)
- MCP, personal account, mautrix-meta based: 0xjesus/messenger-mcp ("MCP server for personal Facebook Messenger via a local mautrix-meta bridge (read, search, send)", Go, created 2026-10-13, 0 stars; uses mautrix-meta as a separate binary) — [0xjesus/messenger-mcp](https://github.com/0xjesus/messenger-mcp)
- O-chr/messenger-mcp ("Personal Facebook Messenger MCP with per-chat read/write permissions, confirmed E2EE sends, and secure history reads", Go backend wrapping `mautrix/meta`, created 2026-10-20, no licence, 0 stars) — [O-chr/messenger-mcp](https://github.com/O-chr/messenger-mcp)
- matpb/meta-mcp-rs (Rust, MIT, created 2026-10-24, pushed 2026-11-10, 0 stars): personal mode uses a Messenger sidecar "built on the `messagix` library of the mautrix-meta bridge and on `whatsmeow` for encrypted chats"; the daemon listens "on Windows, a named pipe"; limits include "encrypted chats: images only, no captions"; it also has a Graph API business mode for Page inboxes — [matpb/meta-mcp-rs](https://github.com/matpb/meta-mcp-rs)
- fukata/messenger-mcp (Python, MIT, created 2026-10-12, 0 stars) is built on fbchat-v2 and notes: "The prebuilt E2EE bridge that fbchat-v2 2.3.2 downloads is built against an old mautrix-meta and can no longer connect"; "Regular (non-E2EE) media upload is disabled in this build" — [fukata/messenger-mcp](https://github.com/fukata/messenger-mcp)
- tas33n/facebook-api-mcp (Node, created 2026-11-05, 0 stars): fca-style library plus MCP server, no E2EE — [tas33n/facebook-api-mcp](https://github.com/tas33n/facebook-api-mcp)
- Browser-automation / scraping route: hieu-lee/facebook-cli ("fast, token-efficient Facebook + Messenger CLI for LLMs, driving your own logged-in Chrome", Rust, 56 stars, created 2026-10-25) and ishan-parihar/facebook-lyr ("Facebook / Messenger MCP server for AI agents (cookie-based web scraping)", 1 star) — [hieu-lee/facebook-cli](https://github.com/hieu-lee/facebook-cli), [ishan-parihar/facebook-lyr](https://github.com/ishan-parihar/facebook-lyr)
- The Claude connector registry returned no Messenger personal-chat connector for the keywords "messenger", "facebook messenger", "facebook", "meta" on 2026-11-12; the hits were Meta Ads and unrelated tools (registry search via the in-session MCP registry tool; no public URL).

### Inferences
- The 2026 MCP projects are useful mainly as worked examples of wrapping mautrix-meta from Go/Rust/Python on Windows (matpb/meta-mcp-rs explicitly supports Windows named pipes; fbchat-v2 ships a Windows bridge build). None has adoption (0-1 stars, weeks old), so none is a dependable product.
- The fukata note shows that a pinned mautrix-meta build stops connecting within months when Meta changes the protocol; any self-written client on this stack needs a routine for updating the mautrix-meta dependency.
- For a lightweight native Windows client, no reusable non-mautrix protocol library exists in C#/.NET, Rust, Dart or C that reaches E2EE chats: fbchat-sharp (C#) stopped in 2022, and searches for Rust and Dart libraries returned only Page-API SDKs, webview wrappers and tuimeta.

### Gaps
- kamarouski/messenger-client (C#, 29 stars, pushed 2026-01-27) could not be characterised: its README fetch failed, so it is unknown whether it targets personal accounts or the Page API — [repo](https://github.com/kamarouski/messenger-client).
- thedantez/cmu ("universal TUI client for messengers", Rust) README had no Messenger-related text matched; its Messenger support is unverified — [repo](https://github.com/thedantez/cmu).
- TimkaLIM/Messenger-MCP (Go, created 2026-10-30) has no descriptive README; backend unknown — [repo](https://github.com/TimkaLIM/Messenger-MCP).
- Other MCP registries (Smithery, mcp.so, Glama, the official modelcontextprotocol registry) were not searched; only GitHub and the in-session registry were.
- EionRobb has no Facebook/Messenger/Meta plugin repository according to a GitHub user-scoped search on 2026-11-12 (0 results); no "purple-meta" plugin was found, and purple-gowhatsapp's README does not mention Messenger. A broader search could still miss a project with an unexpected name.
