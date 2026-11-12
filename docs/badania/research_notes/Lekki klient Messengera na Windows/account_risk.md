# Account risk for a personal Facebook account when using unofficial Messenger clients / bridges (2024 – 12 Nov 2026)

Method note for the report writer: pages were read through a fetch tool that summarises content with a small model. Quotes below are as returned by that tool; short quotes are reliable, but long passages may be truncated. Anything I could not open directly is marked NOT VERIFIED. All access dates: 2026-11-12. "Maintainer statement" = official docs/help pages; "anecdote" = individual user report.

## 1. What do maintainers say?

### Takeaway
Every maintainer source found admits that Meta may flag the session and demand a captcha, phone number, password reset or login approval; none promises safety, and none of them publishes a rate. Beeper frames it as "typically a security checkpoint, not an account ban"; mautrix says only that Meta may "block you" until tasks are completed.

### Cited Findings
- MAINTAINER (mautrix-meta official docs, page undated, read 2026-11-12): "In some cases, Meta may decide your account has suspicious activity and block you" ... "until you do some tasks like completing a captcha, adding a phone number or resetting your password." and "It is recommended to have two-factor authentication enabled to reduce the risk of such blocks." (the two halves of the first quote were returned separately; the joining words were not captured) — [docs.mau.fi authentication](https://docs.mau.fi/bridges/go/meta/authentication.html)
- MAINTAINER (same page): login procedure is to open facebook.com / messenger.com / instagram.com "in a private window" and copy cookies from a `graphql` XHR request; login modes are `facebook` or `messenger` (cookies) or `messenger-lite` (username/password); Instagram has `instagram` (cookies) or `android` (username/password, v26.10+). The page, as read, does NOT mention IP addresses, proxies or E2EE as risk factors. — [docs.mau.fi authentication](https://docs.mau.fi/bridges/go/meta/authentication.html)
- MAINTAINER (Beeper help centre, Messenger article, undated, footer "© 2026 Automattic Inc."): "When you connect Facebook to Beeper, it may appear as a new device, browser, or location."; "Meta may prompt you to complete additional verification steps" (described as "a normal part of their login security process"); "Facebook may also flag login attempts as suspicious."; "Disconnections can happen if Facebook logs you out due to security checks, changes to your password or 2FA settings"; "Enabling 2FA helps prevent Facebook from flagging your login as suspicious—especially when signing in from a new device". The page does not mention bans or permanent disabling. — [help.beeper.com Messenger](https://help.beeper.com/chat-networks/messenger)
- MAINTAINER (Beeper help centre, Instagram article, undated): on the "We suspect automated behavior" warning — "When Instagram detects activity from a new device or location, it may flag your account with this warning."; "This is typically a security checkpoint, not an account ban."; "As long as you're using your account normally and not engaging in spam or harmful behavior, you'll be fine."; "Instagram sometimes resets login sessions as part of its security measures."; "Beeper will show up as an Android phone, usually a Pixel 6 or a Linux computer." — [help.beeper.com Instagram](https://help.beeper.com/instagram)
- MAINTAINER, older bridge (deprecated mautrix-facebook, as documented in matrix-docker-ansible-deploy; seen only as a search snippet, NOT opened): if the server is in a very different location from where the account is normally used, login attempts may be rejected and Facebook may force a password change. — [matrix-docker-ansible-deploy docs mirror](https://git.cqre.net/cqrenet/matrix-docker-ansible-deploy/src/branch/master/docs/configuring-playbook-bridge-mautrix-facebook.md)
- MAINTAINER-ish, fca-unofficial family (search snippets only, NOT opened; these are forks of varying quality): a fork README says third-party Messenger automation always carries some risk and suggests a secondary account; fbchat-muqit says maintainers are not responsible if the account is banned and recommends a dummy account. — [@dongdev/fca-unofficial README](https://app.unpkg.com/@dongdev/fca-unofficial@4.0.3/files/README.md); [fca-sifu README](https://cdn.jsdelivr.net/npm/fca-sifu@2.0.0/README.md); [fbchat-muqit listing](https://awesome.ecosyste.ms/projects/github.com%2Ftogashigreat%2Ffbchat-muqit)
- The mautrix/meta tracker shows continuing work on checkpoint handling right up to Nov 2026 (titles only, bodies not read; mostly Instagram): "ig/login: add support for checkpoints in native login" (#347, 2026-09-08), "instameow: handle IG checkpoints/captcha URLs better" (#358, 2026-09-17), "ig/login: hand off checkpoint redirects after web login to the webview" (#441, 2026-10-21), "igconnector: disconnect on account notices and recheck them" (#498, 2026-11-01), "Switch to bad credentials when we get an IG challenge or suspended er…" (#91, 2024-07-26). — [mautrix/meta issues & PRs](https://github.com/mautrix/meta/issues?q=checkpoint)

### Inferences
- Maintainers treat checkpoints as an expected, recurring operating condition (they write code to detect and route them), not as a rare accident.
- Beeper's "you'll be fine" is a vendor reassurance, not data; it is contradicted by at least a few anecdotes in section 2.

### Gaps
- mautrix-meta README, slidge/messlidger docs, ws3-fca README and the old fbchat README were NOT opened (search returned nothing for slidge/messlidger). No claim about their wording should be made.
- No maintainer publishes any frequency/rate of checkpoints or bans.
- No statement by the mautrix lead maintainer was found in issue #44 (all 38 comments have author_association NONE).

## 2. Concrete user reports 2024–2026 (how often, were accounts recovered)

### Takeaway
The single best primary source is mautrix/meta issue #44 (open since 2024-03-22, 38 comments, still active Aug 2026): roughly 17 personal users report warnings, forced logouts or locks; most are session/token blocks or checkpoints, and two users report something stronger (one "banned instantly", one "Your account is disabled"). No rate can be derived — this is self-selected reporting with an unknown user base.

### Cited Findings
(All from one thread unless noted; all ANECDOTES.) — [mautrix/meta #44](https://github.com/mautrix/meta/issues/44), comment JSON at [GitHub API](https://api.github.com/repos/mautrix/meta/issues/44/comments?per_page=100)
- Thread facts: title "Message from Facebook: 'We suspect automated behaviour on your account.'", opened 2024-03-22, state open, 38 comments, 13 reactions on the opening post; no maintainer reply.
- 2024-03-24/25: one user disconnected "about weekly", bridge hosted in a different country from residence; opener had backfill disabled, bridge in a different country; another user got the notice with server in the SAME country and backfill enabled.
- 2024-03-25: a user with 2FA enabled says the old bridge was fine but the new one produced an account-disabled-style message twice (later asks others "Like banned banned, or locked?" — i.e. theirs was a lock).
- 2024-03-28 → 2024-04-20 (Instagram mode): first warning the day after switching, backfill off, 2FA on, captcha required; later: "Following up on this, just updated to the newest version and my account got banned instantly." No follow-up on recovery.
- 2024-04-09: logged out two days in a row in `messenger` mode after months of use; switched to `facebook` mode, no logouts for ~1 week.
- 2024-04-20: a user with ~1 year of bridging, self-hosted abroad over VPN, "no issues so far"; the same user on 2024-07-03: "I've also received notification about 'unusual activity'", then routed the bridge through a proxy on their home address; on 2024-07-26 says Instagram automated-behaviour warnings stopped after disabling backfill ("Now it won't complain").
- 2024-06-12: Instagram shows "Help us confirm that it's you" with temporary restrictions.
- 2024-07-05: ran the bridge at home without the warning; after moving it to the cloud it lasted "a couple hours" before the warning.
- 2024-07-18 / 2024-08-09: brand-new Facebook account + bridge → 24-hour soft-lock (only one browser session allowed) and another warning next day; after converting to a Messenger-only (DEMA) account and using `messenger` mode it ran "without harassment" for a couple of weeks.
- 2024-10-29: "Backfilling is disabled but I'm having the issue".
- 2025-02-11: "Still the same problem quite regularly with Instagram".
- 2025-04-25/26: bridge ran from Sept 2024 with no problem until an "Unusual Behavior" message on 2025-04-23; stable VPN IP; user explains the message appears on web login after Meta blocks the bridge's token, not the whole account; a replacement token was blocked in under 24 hours; account not disabled, user stopped.
- 2025-04-26: one user reports bridge error BAD_CREDENTIALS, "Your account is disabled, please review your account" — full disablement; says they did not use Facebook anyway; no recovery info, no usage details.
- 2025-08-06: one user gets the error "about once every 1–2 months"; on 2026-08-25 the same user reports no logout in ~3 months on the stock version.
- 2025-12-26 (Instagram): warning after about one hour, backfill off, HOME IP, 2FA on; user quit.
- 2026-08-25: self-hosted Messenger bridge, cookie session dies after ~7 days ("Logged out, please relogin to continue", BAD_CREDENTIALS / meta-user-id-is-zero) while the source browser session stays logged in.
- Counter-examples in the same thread: one user 12 days without any message (2024-03-24); one ~1 year clean before a first notice.
- r/beeper thread "Is using Beeper on Meta apps allowed or not" (2025-05-04) — seen ONLY via search snippets, page returned HTTP 403, NOT VERIFIED: one user reports a restriction and will not reconnect; another says Facebook "prompted to reset my password and locked my FB messenger until I acknowledged the notification in Facebook about suspicious activity"; a commenter theorises the bridge re-authenticates with expired/revoked cookies. — [reddit mirror](https://redlib.hbubli.cc/r/beeper/comments/1kedikt/is_using_beeper_on_meta_apps_allowed_or_not)
- Context: independent of bridges, press reported waves of unexplained Meta account bans/lockouts hitting ordinary users in 2025 (search snippets only, not opened). — [Search Engine Land](https://searchengineland.com/why-facebook-account-lockouts-are-rising-and-whats-driving-them-476546)

### Inferences
- Dominant outcome in the evidence: warning/checkpoint + session invalidation, cleared by captcha / identity confirmation / re-login; recurring for some users (weekly to every 1–2 months), absent for months for others.
- Instagram reports look more frequent and harsher than Facebook/Messenger ones in this thread (the "banned instantly" and the 1-hour home-IP case are both Instagram).
- Because background Meta false-positive bans exist, a lone "account disabled" report cannot be causally attributed to the bridge.

### Gaps
- No denominator: number of mautrix-meta / Beeper Messenger users is unknown, so NO rate can be stated.
- Recovery outcome is unknown for the two strongest reports (2024-04-20 "banned", 2025-04-26 "disabled").
- r/beeper, r/selfhosted, Hacker News and Matrix room logs could not be read directly (Reddit blocked; no HN hits surfaced). My user count (~17) is from tool summaries of the thread and is approximate; one further commenter runs ~300 accounts and was excluded as non-personal.

## 3. Reported triggers

### Takeaway
Users report many suspected triggers, but the thread shows a counter-example for nearly every one; nothing is confirmed by Meta or by maintainers except "new device, browser, or location".

### Cited Findings
- New device / browser / location is the only trigger stated by a maintainer: "Meta may treat Beeper as a new device, browser, or location." — [help.beeper.com Instagram](https://help.beeper.com/instagram); [help.beeper.com Messenger](https://help.beeper.com/chat-networks/messenger)
- Datacenter vs home (anecdote, 2024-07-05): fine at home, warning within "a couple hours" after moving to cloud. Counter-anecdotes: warning on HOME IP after ~1 hour (Instagram, 2025-12-26); warning with server in same country (2024-03-25); a year clean while hosted abroad on a VPN (2024-04-20). — [mautrix/meta #44](https://github.com/mautrix/meta/issues/44)
- Backfill (anecdote): disabling it stopped Instagram warnings for one user (2024-07-26); others got warnings with backfill already disabled (2024-03-25, 2024-03-28, 2024-10-29, 2025-12-26). — [mautrix/meta #44](https://github.com/mautrix/meta/issues/44)
- New account age (anecdote, 2024-07/08): days-old account got a 24-hour soft-lock. — [mautrix/meta #44](https://github.com/mautrix/meta/issues/44)
- User-agent / client-hint mismatch between the cookie-origin browser and the bridge's hard-coded UA (Chrome/141 Linux vs Chrome 151 macOS) — a user HYPOTHESIS (2026-08-25, self-declared AI-assisted), unconfirmed. Related: maintainer commit "login: store user agent when using provisioning API" by tulir (date not captured) stores the login UA. — [mautrix/meta #44](https://github.com/mautrix/meta/issues/44); [commit 5f853f8](https://github.com/mautrix/meta/commit/5f853f8)
- `messenger` vs `facebook` mode: contradictory anecdotes — one user fixed logouts by switching messenger→facebook (2024-04-09); another had success with messenger mode + Messenger-only account (2024-08-09). — [mautrix/meta #44](https://github.com/mautrix/meta/issues/44)
- Re-authenticating with expired/revoked cookies — a Reddit commenter's theory (2025-05), NOT VERIFIED. — [reddit mirror](https://redlib.hbubli.cc/r/beeper/comments/1kedikt/is_using_beeper_on_meta_apps_allowed_or_not)

### Inferences
- For the end user's scenario (home residential IP, same machine as their browser, human-rate chatting) the location-mismatch trigger is removed, but the evidence shows home IP alone does not guarantee no warning.

### Gaps
- No evidence found specifically on: E2EE new-device registration as a trigger, 24/7 uptime, multiple simultaneous sessions, or sending rate for personal use.
- Meta publishes nothing on what its detection keys on.

## 4. Mitigations documented

### Takeaway
Officially documented mitigations are few: enable 2FA, log in via a private window, approve login alerts, and (Beeper) mark the session as recognised. Everything else (home-IP proxy, disable backfill, mode switching, UA matching) is user folklore with mixed results.

### Cited Findings
- 2FA: "It is recommended to have two-factor authentication enabled to reduce the risk of such blocks." — [docs.mau.fi](https://docs.mau.fi/bridges/go/meta/authentication.html); same advice from Beeper — [help.beeper.com Messenger](https://help.beeper.com/chat-networks/messenger). Anecdotes show warnings still occur with 2FA on (2024-03-25, 2024-03-28, 2025-12-26). — [mautrix/meta #44](https://github.com/mautrix/meta/issues/44)
- Private-window cookie extraction is the documented login path. — [docs.mau.fi](https://docs.mau.fi/bridges/go/meta/authentication.html)
- Approve login alerts in the Facebook app / email; reconnect; "verify Beeper as a recognized app in your Instagram login activity". — [help.beeper.com Messenger](https://help.beeper.com/chat-networks/messenger); [help.beeper.com Instagram](https://help.beeper.com/instagram)
- Proxy support exists in the bridge (PR title "client: set proxy for whatsmeow", #231, 2026-02-25; body not read); users route via home-address proxy (anecdote, 2024-07-03, outcome not reported). — [mautrix/meta #231](https://github.com/mautrix/meta/pull/231); [mautrix/meta #44](https://github.com/mautrix/meta/issues/44)
- Disable backfill (anecdote, mixed), switch login mode (anecdote, contradictory), log in via the bridge's webview rather than pasting cookies so the UA matches (user suggestion, 2026-08, unverified). — [mautrix/meta #44](https://github.com/mautrix/meta/issues/44)

### Inferences
- A self-written client running on the same PC and IP as the user's browser, sending a consistent browser-like fingerprint, matches the mitigations users converge on — but no source demonstrates that this prevents checkpoints.

### Gaps
- "Close the private window without logging out" and "run on the same IP as the login browser" were NOT found verbatim in the docs page as read; do not attribute them to maintainers.

## 5. Broader Meta enforcement / platform changes 2024–2026

### Takeaway
No cease-and-desist, lawsuit or public enforcement action by Meta against Beeper, mautrix or other personal Messenger clients was found. The relevant platform change is Meta ending its own native desktop Messenger apps on 15 Dec 2025.

### Cited Findings
- Meta shut down the standalone Messenger desktop apps for Windows and macOS effective 2025-12-15, directing users to messenger.com / facebook.com (Windows users also to the Facebook desktop app); sources differ on whether a 60-day grace period applied. — [CP24, 2025-12-16](https://www.cp24.com/news/canada/2025/12/16/metas-messenger-desktop-apps-shut-down/); [Techloy](https://www.techloy.com/meta-shuts-down-messenger-desktop-app-on-windows-and-macos/); [Gulf News](https://gulfnews.com/technology/media/messenger-desktop-apps-on-mac-windows-are-ending-heres-what-you-must-do-1.500310786) (search summaries; articles not opened individually)
- A web search for Meta cease-and-desist / legal action against Beeper or third-party Messenger clients (2025) returned no such report. — [search result context: r/beeper thread](https://redlib.hbubli.cc/r/beeper/comments/1kedikt/is_using_beeper_on_meta_apps_allowed_or_not)
- Different product, context only: Meta excluded third-party AI assistants from the WhatsApp Business API in Oct 2025, leading to an EU antitrust order reported June 2026. — [Morningstar/Dow Jones, 2026-06-09](https://www.morningstar.com/news/dow-jones/202606095919/meta-gets-eu-antitrust-order-to-open-whatsapp-to-rival-ai-chatbots-update)
- Context only: Apple actively blocked Beeper Mini's iMessage access in Dec 2023 — a different company and mechanism. — [The Register, 2023-12-18](https://www.theregister.com/2023/12/18/beeper_blocked_imessage_whacamole_continues/)

### Inferences
- Absence of found reports is not proof of absence; Meta's enforcement appears to be automated per-account/session, not legal action against client authors.

### Gaps
- No verified information on new web attestation / "browser integrity" checks on messenger.com in 2024–2026.
- Historical Meta actions before 2024 were out of scope and not researched.

## 6. Meta terms and EU law

### Takeaway
Meta's Terms (effective 1 Jan 2025) prohibit accessing or collecting data "using automated means (without our prior permission)" and allow suspension or permanent disabling; whether a hand-driven unofficial client counts as "automated means" is not settled by any source found. The DMA's messaging-interoperability duty is addressed to Meta vis-à-vis other providers and gives no documented right for an individual to use a self-made client.

### Cited Findings
- Terms of Service, "Effective January 1, 2025" (no separate revision date shown): "You may not access or collect data from our Products using automated means (without our prior permission)" (the Polish version continues: "ani próbować uzyskiwać dostępu do danych, do których nie pozyskał zgody na dostęp"). — [facebook.com/legal/terms](https://www.facebook.com/legal/terms?locale=en_US)
- Same Terms: "we may suspend or permanently disable your access to Meta Company Products"; "we may permanently disable or delete your account"; "Where we disable or delete your account, we will also explain any options you have to request a review". — [facebook.com/legal/terms](https://www.facebook.com/legal/terms?locale=en_US)
- DMA Article 7: Meta has had to comply with the interoperability obligation since 2024-03-07 for WhatsApp and 2024-09-06 for Facebook Messenger. — [BEREC Opinion BoR (25) 21, March 2025](https://www.berec.europa.eu/system/files/2025-03/BoR%20(25)%2021%20BEREC%20Opinion%20on%20Meta's%20reference%20offers.pdf) (via search summary)
- In practice interoperability is delivered to partner messaging services under a reference offer; first WhatsApp partners (BirdyChat, Haiket) were announced Nov 2025, opt-in for users, mobile only. No Messenger partner was found. — [Meta newsroom, Nov 2025](https://about.fb.com/news/2025/11/messaging-interoperability-whatsapp-enables-third-party-chats-for-users-in-europe/)
- User-level opinion only (r/beeper, 2025-05, NOT VERIFIED): commenters disagreed whether the terms forbid third-party clients. — [reddit mirror](https://redlib.hbubli.cc/r/beeper/comments/1kedikt/is_using_beeper_on_meta_apps_allowed_or_not)

### Inferences
- The clause is broad enough that Meta can invoke it against any non-official client; a personal user should assume the terms give Meta discretion, with a contractual right to request review.

### Gaps
- No separate Messenger-specific terms were retrieved; no credible legal source was found on whether the EU Software Directive interoperability exception or the DMA protects an individual user of a self-made client. Not legal advice; unresolved.
- The remainder of the Terms sentence about automated means was not captured in full.

## 7. Evidence of permanent disabling solely for personal use of a bridge/unofficial client?

### Takeaway
No verified, documented case was found of a personal account being permanently disabled where the bridge was demonstrably the sole cause. Two unverified self-reports in mautrix/meta #44 claim a ban/disablement (one Instagram, one unspecified), with no detail on recovery or other factors.

### Cited Findings
- "just updated to the newest version and my account got banned instantly" (Instagram mode, 2024-04-20; another user asked whether it was a ban or a lock; no answer captured). — [mautrix/meta #44](https://github.com/mautrix/meta/issues/44)
- "Your account is disabled, please review your account" (BAD_CREDENTIALS), 2025-04-26; reporter says they do not use Facebook anyway; "please review" wording implies a review path was offered. — [mautrix/meta #44](https://github.com/mautrix/meta/issues/44)
- Another user in the same thread (2025-04-26) states that what gets blocked is the bridge's token/session, not the whole account. — [mautrix/meta #44](https://github.com/mautrix/meta/issues/44)
- Beeper's position: "This is typically a security checkpoint, not an account ban." — [help.beeper.com Instagram](https://help.beeper.com/instagram)

### Inferences
- The documented risk for normal personal chatting is predominantly nuisance-level (checkpoints, forced re-logins, password resets, short locks), with a small, unquantifiable tail risk of disablement that the evidence can neither confirm nor exclude.

### Gaps
- No follow-up from either "banned/disabled" reporter; no way to separate bridge-caused action from Meta's general false-positive bans.
- No data at all on fully custom native clients (as opposed to mautrix-meta/Beeper); the user's own client would have its own fingerprint and its risk is not covered by any source found.
