# Official Messenger on Windows and Meta's official APIs / interoperability routes (state as of 12 Nov 2026)

Method note: research done 12 Nov 2026 with web search + page fetch. Pages were read through a summarising fetch tool, so "quotes" below are the fragments the tool returned verbatim; long clauses are partial. Items I could not open or confirm are listed under Gaps in each section. Nothing here is from memory alone.

## 1. What is the official Messenger experience on Windows today?

### Takeaway
There is no standalone official Messenger program for Windows any more. The desktop apps (Windows + macOS) were shut down in mid-December 2025, and the standalone website messenger.com was retired in April 2026 and redirects to facebook.com/messages. What remains on a Windows PC is Facebook in a browser (facebook.com/messages) or the Facebook app from the Microsoft Store. I found no reliable measured RAM figures for Messenger-on-Windows from 2025-2026.

### Cited Findings
- Windows Latest (17 Oct 2025) reported Meta's in-app notice "Messenger desktop app is going away" and that the app "will no longer be available after December 14, 2025"; scope: Windows 11 and macOS; mobile apps unaffected. — [Windows Latest, 2025-10-17](https://www.windowslatest.com/2025/10/17/meta-removes-messenger-app-for-windows-11-wants-you-to-use-the-web-app)
- Other outlets give the cut-off as 15 December 2025 (a one-day discrepancy with Windows Latest's "after December 14"). Reports say Meta confirmed the plan to TechCrunch in October 2025, and that opening the dead app redirects the user (on Windows to the Facebook desktop app, on Mac to the web). — [Techloy](https://www.techloy.com/meta-shuts-down-messenger-desktop-app-on-windows-and-macos/); [CyberInsider](https://cyberinsider.com/meta-to-shut-down-messenger-desktop-apps-on-december-15-redirects-users-to-web/); [Windows Report](https://windowsreport.com/facebook-officially-shuts-down-messenger-desktop-apps-for-windows-and-mac/) (search-result summaries only; articles not opened individually)
- Meta's notice pointed Windows users to the Facebook app; Windows Latest's author could not find Messenger in Microsoft Store search and the Store web page's download button was disabled (Oct 2025). The Facebook desktop app shows Messenger chats only if the Messenger account is connected to a Facebook account. — [Windows Latest, 2025-10-17](https://www.windowslatest.com/2025/10/17/meta-removes-messenger-app-for-windows-11-wants-you-to-use-the-web-app)
- Users were told to set a PIN to keep (end-to-end encrypted) chat history: "You need to create a PIN to avoid losing chat history." — [Windows Latest, 2025-10-17](https://www.windowslatest.com/2025/10/17/meta-removes-messenger-app-for-windows-11-wants-you-to-use-the-web-app)
- Earlier history: the Windows Messenger client had already been switched from a native app to a progressive web app (PWA) in September 2024, per press and a Reddit r/Windows11 thread from 27 Sep 2024. — [TechIssuesToday](https://techissuestoday.com/facebook-messenger-ditches-native-app-for-a-slimmer-pwa-on-windows); [Reddit mirror](https://reddit.birdcat.cafe/r/Windows11/comments/1fqaxtp/the_facebook_messenger_app_on_ms_store_has_been) (search-result summaries only)
- messenger.com retirement: Meta "will stop supporting messaging from April 2026"; visitors are redirected to https://www.facebook.com/messages, usable "while logged into their Facebook account". Android/iOS apps remain. The fact-check cites Meta Help Center article https://www.facebook.com/help/messenger-app/804132271957789. — [Fact Crescendo, 2026-03-04](https://english.factcrescendo.com/2026/03/04/meta-not-shutting-down-messenger-only-website/)
- Secondary reports give 16 April 2026 as the day messenger.com stopped working for messaging, and say people without a Facebook account lose web access and must use the mobile app. — [IBTimes AU](https://www.ibtimes.com.au/facebook-messenger-down-web-version-shuts-today-meta-redirects-users-facebook-chat-april-2026-1866690); [Gulf News](https://gulfnews.com/technology/meta-to-shut-messenger-website-in-april-2026-redirect-users-to-facebook-1.500447134); [Wikipedia](https://en.wikipedia.org/wiki/Facebook_Messenger) (search-result summaries only)
- RAM context (NOT Messenger): Windows Latest measured Meta's WebView2-based WhatsApp for Windows at roughly 600 MB idle and up to about 1.2 GB after scrolling chats (article dated 13 Jun 2026). — [Windows Latest, 2026-06-13](https://windowslatest.com/2026/06/13/whatsapp-is-eating-1-2gb-ram-on-windows-11-even-as-microsoft-kills-web-app-slop) (search-result summary only)
- Low-quality blog claims for the Facebook Store app: "240MB of RAM versus 410MB for a Chrome tab"; sources contradict each other on whether the Facebook Windows app is a thin web wrapper (~5 MB download) or "Electron-based". — [winprotips.blog](https://winprotips.blog/facebook-app-windows-11-10-review); [totaltech.blog](https://totaltech.blog/facebook-lite-download-pc-windows); [FileHorse](https://www.filehorse.com/download-facebook-windows-10/) (unreliable; do not present as fact)

### Inferences
- Because messenger.com now redirects to facebook.com/messages, a "webview wrapper around messenger.com" is no longer even the same thing it was in 2025; any wrapper or web-protocol client today effectively targets facebook.com.
- The official Windows path in Nov 2026 is web technology in every variant (browser tab or the Facebook Store app), which is consistent with the user's complaint about weight, but I have no measured Messenger number to prove it.

### Gaps
- Technology of the current Facebook app in the Microsoft Store (PWA vs WebView2 vs other): NOT verified. I did not open the Store listing; the blogs found contradict each other.
- Whether a "Messenger" listing still exists in the Microsoft Store in Nov 2026: NOT verified (last data point is Oct 2025: not findable / download disabled).
- No trustworthy 2025-2026 RAM measurement of Messenger (old PWA, facebook.com/messages tab, or Facebook Store app) was found. Only the WhatsApp figures above, which are a different app.
- TechCrunch's original Oct 2025 article (the claimed Meta confirmation and "60 days" notice wording) could not be fetched (HTTP 404 on the URL I tried); Dataconomy's Feb 2026 article returned HTTP 403. Meta's own Help Center pages were not opened directly.
- Exact shutdown day: 14 vs 15 December 2025 conflict between sources is unresolved.

## 2. Messenger Platform / Graph API: personal accounts or only Pages / business?

### Takeaway
Meta's official Messenger Platform is a business-messaging API: it works for a Facebook Page or an Instagram Professional account, authenticated with a Page access token. The official docs describe no way to send or read messages as a personal profile, and the FAQ explicitly says Facebook Login user IDs do not work with the Send/Receive API.

### Cited Findings
- Official overview: "Messenger from Meta lets a business' Facebook Page or Instagram Professional account respond to people". — [Meta for Developers, Messenger Platform overview (fetched 2026-11-12; no last-updated date shown)](https://developers.facebook.com/docs/messenger-platform/overview)
- Page access tokens are "Required to send and receive messages through the Messenger Platform"; the `pages_messaging` permission is requested via Facebook Login for Business. — [Messenger Platform overview](https://developers.facebook.com/docs/messenger-platform/overview)
- Standard messaging window: "For most user actions, it lasts 24 hours." — [Messenger Platform overview](https://developers.facebook.com/docs/messenger-platform/overview)
- "Instagram Messaging is available for any Instagram Professional account for a business or creator" (i.e. the "Messenger API for Instagram" is also for professional accounts, not personal ones). — [Messenger Platform overview](https://developers.facebook.com/docs/messenger-platform/overview)
- Official FAQ: "User IDs from Facebook Login are not intended to work with the Send/Receive API." / "Only user IDs obtained through authentication with the Messenger Platform will work with the Messenger Platform." / "User IDs for the Messenger Platform are scoped to a page and, therefore, are page specific." / "Apps cannot send messages outside the standard messaging window." — [Meta for Developers, Messenger Platform FAQ (fetched 2026-11-12)](https://developers.facebook.com/docs/messenger-platform/faq)
- The docs now live under a "business-messaging" path (e.g. developers.facebook.com/documentation/business-messaging/messenger-platform/...). — [Meta for Developers, error codes](https://developers.facebook.com/documentation/business-messaging/messenger-platform/error-codes)
- Third-party integrators state the same limit: Missive: "Facebook only allows third-party apps to manage messages sent to Facebook Pages. Personal messages aren't accessible."; Pipedrive: only Messenger for Business can be integrated; Zapier staff: only Pages can be connected. — [Missive](https://missiveapp.com/docs/core-features/connected-accounts/other-channels/social/faq); [Pipedrive](https://support.pipedrive.com/article/facebook-messenger-integration-faq); [Zapier community](https://community.zapier.com/how-do-i-3/facebook-messenger-schedule-message-from-personal-account-42520) (search-result summaries; undated)

### Inferences
- A personal user could technically create a Page and a bot, but that bot talks as the Page to people who message the Page, inside a 24-hour window; it cannot read or send the user's own friend-to-friend chats. So the Messenger Platform does not solve the user's goal.

### Gaps
- The official docs do not contain an explicit sentence "personal accounts are not supported"; the conclusion rests on the positive scope statement (Page / Instagram Professional) plus the FAQ. No dated statement from Meta saying so in those words was found.
- I did not check the Graph API changelog for 2026; no evidence of any new personal-inbox permission was found in search, but absence was not exhaustively verified.

## 3. EU DMA messaging interoperability for Messenger: status in 2026

### Takeaway
DMA Article 7 interop exists for Messenger on paper since September 2024 (1:1), with groups added in September 2025 and calls due by 7 September 2027, but it is a route for other messaging SERVICES whose users then chat with Messenger users. It is not a way to log into your own Messenger account from another client. The only named live partners (BirdyChat, Haiket, Nov 2025) are confirmed for WhatsApp; I could not confirm any service live with Messenger.

### Cited Findings
- European Commission portal: "Any provider of messaging services already operating or planning to operate in the Union may request interoperability"; the gatekeeper must fulfil "any reasonable interoperability request" within three months and "must publish a reference offer, setting out the technical specifications and general terms and conditions for interoperability." — [European Commission, DMA messaging interoperability page (fetched 2026-11-12)](https://digital-markets-act.ec.europa.eu/businesses-portal/messaging-interoperability_en)
- Compliance dates for Meta (designated 6 Sep 2023): 1:1 messaging: WhatsApp 7 Mar 2024, Messenger 7 Sep 2024; group chats: 7 Sep 2025 (both); audio/video calls: 7 Sep 2027 (both). — [European Commission, same page](https://digital-markets-act.ec.europa.eu/businesses-portal/messaging-interoperability_en)
- Same page: in November 2025 "the two first third-party NIICS interoperable with WhatsApp were launched." (No Messenger launch is mentioned.) — [European Commission, same page](https://digital-markets-act.ec.europa.eu/businesses-portal/messaging-interoperability_en)
- Meta's developer page: purpose is to "Enable users on your app in Europe to exchange messages" with Messenger/WhatsApp users; requester "must be able to comply with the terms of the Messenger and/or WhatsApp Reference Offer"; the app must provide messaging services to end users in Europe. Process: review Reference Offer + Developer Documentation Overview + Application Guidelines, apply via form, receive detailed docs under NDA if eligible, sign an Interoperability Agreement, build, complete milestones, joint launch. "Separate documentation and applications are required" for Messenger and WhatsApp. — [Meta for Developers, Messaging Interoperability (fetched 2026-11-12; undated)](https://developers.facebook.com/m/messaging-interoperability/)
- Meta engineering (6 Mar 2024, updated 6 Sep 2024): "third-party providers will sign an agreement with Messenger and/or WhatsApp"; "we would prefer third-party providers to use the Signal Protocol" (equivalent protocols allowed if same security); third-party clients connect to Meta's servers ("third-party clients will connect to WhatsApp servers using our protocol", XMPP-based for WhatsApp); interop applies to "users of third-party providers who choose to enable interoperability". — [Engineering at Meta, 2024-03-06](https://engineering.fb.com/2024/03/06/security/whatsapp-messenger-messaging-interoperability-eu/)
- Meta newsroom (6 Sep 2024): feature is opt-in "third-party chats" inside WhatsApp and Messenger, separate folder or combined inbox; reactions, direct replies, typing indicators, read receipts; "In accordance with the DMA, in 2025 we will include the option to create groups, and voice / video calling in 2027."; "Users will start to see the third-party chat option when a third-party messaging service has built, tested and launched" the technology. — [Meta Newsroom, 2024-09-06](https://about.fb.com/news/2024/09/an-update-on-how-were-building-safe-and-secure-third-party-chats-for-users-in-europe/)
- Meta's DMA compliance report submitted 6 Mar 2026 (as summarised by PPC Land, 10 Mar 2026): Messenger interop launched 7 Sep 2024; an interoperable group messaging solution for WhatsApp and Messenger was launched during the period; as of Nov 2025 BirdyChat "fully launched" and Haiket "in advanced beta" — described in relation to WhatsApp; two technical models (client-to-server, and a proxy model added later); encryption must be Signal-Protocol-equivalent. — [PPC Land, 2026-03-10](https://ppc.land/metas-2026-dma-report-reveals-whatsapp-ads-a-eu200m-fine-and-a-defiant-stance-on-personalized-advertising/); primary PDF (not readable by my tool): [Meta 2026 non-confidential summary](https://ppc.land/content/files/2026/03/Non-confidential-Public-Summary-of-Meta-s-DMA-Compliance-Report--2026.pdf)
- Meta's own WhatsApp announcement of BirdyChat and Haiket (Nov 2025). — [Meta Newsroom, 2025-11](https://about.fb.com/news/2025/11/messaging-interoperability-whatsapp-enables-third-party-chats-for-users-in-europe/) (title/summary only, not opened)
- Conflicting secondary claim: a Wolters Kluwer competition-law blog is summarised as saying BirdyChat and Haiket are interoperable "with WhatsApp and Messenger"; and one summary of Meta's report dates Messenger interop to "November 2024" rather than 7 Sep 2024. — [Kluwer Competition Law Blog](https://legalblogs.wolterskluwer.com/competition-blog/metas-second-dma-compliance-workshop-a-half-an-egg-omelette/); contradicted on both points by [European Commission page](https://digital-markets-act.ec.europa.eu/businesses-portal/messaging-interoperability_en) and [PPC Land](https://ppc.land/metas-2026-dma-report-reveals-whatsapp-ads-a-eu200m-fine-and-a-defiant-stance-on-personalized-advertising/)
- Commission DMA review (April 2026) is summarised as noting that alternative services can now interoperate with WhatsApp and Messenger and that this "led to new providers, including a European SME". — [European Commission, DMA Review Report COM(2026) 178](https://digital-markets-act.ec.europa.eu/system/files/2026-04/DMA%20Review%20Report_COM_2026_178_1_EN.pdf) (search-result summary only)
- Signal and Threema reportedly declined to interoperate over privacy concerns; Galaxus reviewers could not actually test the feature (Haiket waiting list). — [Galaxus](https://www.galaxus.at/en/page/opening-up-whatsapp-like-this-achieves-nothing-43002) (search-result summary only)
- BEREC published an opinion on Meta's Messenger and WhatsApp reference offers (reported as March 2025). — [BEREC](https://www.berec.europa.eu/en/all-documents/berec/opinions/berec-opinion-on-metas-reference-offers-to-facilitate-messenger-and-whatsapp-interoperability-under-article-7-of-the-digital-markets-act) (not opened)

### Inferences
- DMA interop is service-to-service: you would have to BE a messaging service provider (your own user base/identity system, Signal-grade E2EE, NDA, signed Interoperability Agreement, joint launch with Meta). Your users would appear to Messenger users as "third-party chats" which each Messenger friend must opt into. You would not see your existing Messenger account, friend list or history.
- The Commission's wording ("any provider of messaging services ... operating or planning to operate in the Union") does not on its face exclude a small provider, but nothing found says a private individual with a one-user hobby client qualifies; Meta decides eligibility before releasing the documentation. Realistically this is not a route for a personal client.
- Messenger-side uptake appears to be nil or unconfirmed as of the sources found; all concrete launches are WhatsApp.

### Gaps
- The Messenger Reference Offer itself, the Application Guidelines and the Developer Documentation Overview were NOT opened; exact eligibility criteria (e.g. legal-entity or EEA-establishment requirement, minimum size, security audit) are unverified.
- Whether any third-party service is live with Messenger (as opposed to WhatsApp) as of Nov 2026: NOT confirmed. Sources conflict (see above).
- Meta's 2025 and 2026 compliance report PDFs could not be parsed by my tool (binary / over size limit); statements about them are secondhand via PPC Land and search summaries.
- No source newer than April 2026 on Article 7 status was found; anything between May and Nov 2026 is unknown.

## 4. Any other official channel (Messenger Lite desktop, MQTT, Business Suite, data portability, Instagram API, 2025-2026 announcements)

### Takeaway
I found no official alternative that gives a self-written client live access to a personal Messenger inbox. Everything official is either business-side (Messenger Platform, Instagram messaging for professional accounts) or export-style (data download / DMA portability), not a chat API.

### Cited Findings
- Meta's direction in 2025-2026 has been to remove standalone Messenger surfaces on desktop (apps Dec 2025, messenger.com Apr 2026), not add new ones. — [Windows Latest, 2025-10-17](https://www.windowslatest.com/2025/10/17/meta-removes-messenger-app-for-windows-11-wants-you-to-use-the-web-app); [Fact Crescendo, 2026-03-04](https://english.factcrescendo.com/2026/03/04/meta-not-shutting-down-messenger-only-website/)
- Instagram messaging via the Messenger Platform is limited to Instagram Professional (business or creator) accounts. — [Messenger Platform overview](https://developers.facebook.com/docs/messenger-platform/overview)
- DMA Article 6(9) data portability: gatekeepers must give end users and third parties they authorise portability of their data, including continuous real-time access; the Commission's developer portal lists Meta as offering a "Data Portability API for end users" and "for authorised third parties". — [European Commission, end-user data portability](https://digital-markets-act.ec.europa.eu/developer-portal/end-user-data-portability_pl) (search-result summary only)
- Chat history can be downloaded through Facebook's data-download tools (mentioned in shutdown coverage). — [Gulf News](https://gulfnews.com/technology/media/messenger-desktop-apps-on-mac-windows-are-ending-heres-what-you-must-do-1.500310786) (search-result summary only)

### Inferences
- Portability tools are export/transfer mechanisms and are one-directional; even if Messenger messages are in scope, they do not let you send messages, so they cannot back a chat client.

### Gaps
- Whether Meta's DMA data-portability API covers Messenger message content, and whether an individual can register as an "authorised third party": NOT verified.
- No official documentation of public MQTT/real-time endpoints for personal accounts was found; I did not find a Meta statement either way. (The absence of documentation is the only evidence.)
- "Messenger Lite" for desktop: no evidence found that such a product exists; not exhaustively verified.
- Meta Business Suite inbox: not researched in detail; it is a Page/business inbox by design (no source fetched).
- 2025-2026 Meta event announcements (Connect, Conversations): not searched specifically; nothing relevant surfaced in the searches run.

## 5. Meta's Terms of Service wording about automated access / unofficial clients

### Takeaway
The Facebook Terms of Service shown to a Polish user (effective date displayed: 1 January 2025) forbid accessing or collecting data from Meta products by automated means without Meta's prior permission, and forbid circumventing technical measures. A self-written client that speaks Meta's private web/mobile protocols on a personal account falls under these clauses, so it is not an authorised route.

### Cited Findings
- Section 3.2, point 3 (Polish text as served): "Użytkownik nie może uzyskiwać dostępu ani gromadzić danych z naszych Produktów sposobem zautomatyzowanym" [(bez naszej uprzedniej zgody)] "ani próbować uzyskiwać dostępu do danych, do których nie pozyskał zgody na dostęp". Plus: "Zastrzegamy też wszelkie nasze prawa przeciwko bezprawnemu pozyskiwaniu tekstu i danych." — [Facebook Terms of Service, "Data wejścia w życie: 1 stycznia 2025" (fetched 2026-11-12)](https://www.facebook.com/legal/terms)
- Section 3.2 also prohibits acting "w celu obejścia, pominięcia lub wyłączenia jakichkolwiek środków technologicznych" (circumventing technological measures) and collecting other users' login names/credentials. — [Facebook Terms of Service](https://www.facebook.com/legal/terms)
- The DMA interoperability agreements "supersede any other developer terms" on Meta's developer site, including Platform Terms and Developer Policies, i.e. the only sanctioned third-party-client path has its own contract. — [Meta for Developers, Messaging Interoperability](https://developers.facebook.com/m/messaging-interoperability/)

### Inferences
- Legally/contractually: official route for a personal-account third-party client = none found. Unofficial protocol clients are a ToS breach risk (account restriction is the practical consequence), independent of whether they are technically feasible.

### Gaps
- The quotes are fragments returned by the fetch tool; the bracketed "bez naszej uprzedniej zgody" (without our prior permission) is the tool's paraphrase of the clause's condition, not a confirmed verbatim string. The writer should re-read the page before quoting the full clause.
- Whether the terms were revised after 1 January 2025 for other locales, or whether separate Messenger-specific terms add anything: NOT verified.
- Enforcement practice (how often Meta bans accounts using unofficial clients) was out of scope and not researched here.
