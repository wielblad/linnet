# Product

<!-- impeccable:product-schema 1 -->

## Platform

windows (natywna aplikacja desktopowa Win32; nie web, nie iOS, nie Android)

## Users

Właściciel projektu (Windows 11, Polska) oraz kilku jego nietechnicznych znajomych. Każdy używa własnego, prywatnego konta Facebook/Messenger. Piszą ze znajomymi w ciągu dnia, zwykle przy okazji innej pracy na komputerze. Zadanie: zauważyć, że ktoś napisał, przeczytać, szybko odpisać i wrócić do swoich spraw.

## Product Purpose

Lekki zamiennik oficjalnego Messengera na Windows, który po wyłączeniu aplikacji desktopowej (grudzień 2025) i messenger.com (kwiecień 2026) działa tylko jako strona w przeglądarce. Sukces: program działa cały dzień w tle przy małym zużyciu pamięci, otwiera się natychmiast i niezawodnie wysyła oraz odbiera wiadomości.

## Positioning

Okno rysowane bezpośrednio przez API Windows, bez silnika przeglądarki, na nieoficjalnej bibliotece protokołu Messengera (messagix i whatsmeow, przez rdzeń lazyfmess). Jeden plik exe i dziesiątki MB pamięci zamiast setek.

## Operating Context

- Program nasłuchuje przez cały dzień, także gdy okna nie widać.
- Użytkownik chce wybierać formę obecności na ekranie: ikona w zasobniku z liczbą nieprzeczytanych; praca w tle bez okna; wąskie, wysokie okno przy krawędzi ekranu; połowa ekranu; pełne okno; przypięte „główki" wybranych osób (kółko zawsze na wierzchu z liczbą nowych wiadomości i opcjonalnym krótkim podglądem przychodzącej wiadomości).
- Nowe wiadomości mają się też pojawiać jako powiadomienia Windows z podglądem treści.
- Sesja pochodzi z zalogowanej przeglądarki (ciasteczka). Meta może zażądać weryfikacji konta, którą kończy się w przeglądarce.

## Capabilities and Constraints

Potwierdzony zakres pierwszej używalnej wersji:

- rozmowy tekstowe 1:1 i grupowe z nazwami osób, reakcje, odpowiedzi na konkretną wiadomość;
- zdjęcia widoczne w rozmowie i wysyłanie plików;
- praca w tle, autostart z Windows, ikona w zasobniku, powiadomienia;
- szukanie na liście rozmów i rozpoczynanie nowej rozmowy;
- prosty ekran logowania w oknie i zrozumiałe komunikaty błędów (dla znajomych).
- podział okna na Rozmowy, Osoby i Ustawienia;
- linki z podglądem (miniatura, tytuł, adres) i odtwarzanie filmów z YouTube i Facebooka w oknie programu;
- podgląd zdjęć w oknie programu;
- pseudonimy osób z Messengera pokazywane zamiast imienia i nazwiska (tylko odczyt; potwierdzone testem na żywo).

Ograniczenia:

- bez webview i bez silnika przeglądarki; jak dotąd bez zewnętrznych bibliotek interfejsu;
- protokół jest nieoficjalny: łamie regulamin Mety, konto może dostać weryfikację, a zmiana po stronie Mety potrafi zepsuć połączenie do czasu aktualizacji biblioteki;
- rdzeń jest na licencji AGPL-3.0, więc przekazanie programu znajomym wymaga udostępnienia im źródeł;
- przywracanie zaszyfrowanej historii PIN-em jest pomijane (ryzyko blokady konta);
- interfejs po polsku;
- tekst rysowany przez GDI pokazuje emoji jednokolorowo;
- rdzeń nie potrafi jeszcze pobierać mediów z rozmów szyfrowanych.

- biblioteka nie podaje dziś obecności osób („aktywny teraz");
- odtwarzanie filmów z YouTube i Facebooka wymaga silnika przeglądarki; do rozstrzygnięcia, czy włączać go tylko na czas odtwarzania.

Decyzje otwarte: nazwa produktu („mlite" jest nazwą roboczą), sposób logowania dla znajomych (hasło w programie albo prowadzony import ciasteczek), kolorowe emoji (wymagają DirectWrite), docelowy budżet pamięci.

## Brand Commitments

- Wybrany kierunek wizualny (12.11.2026): standard kategorii, czyli wygląd komunikatora na Windows 11, wykonany na poziomie wzorców wskazanych przez właściciela: Telegram Desktop, aplikacje systemowe Windows 11, WhatsApp.
- Kolor akcentu, motyw jasny lub ciemny i wielkość tekstu pochodzą z ustawień Windows; program nie ma własnego przełącznika motywu.
- Zdjęcia profilowe zamiast inicjałów; warstwy i głębia zamiast płaskich szarości; szczegóły dojrzałego komunikatora (potwierdzenia odczytu, akcje przy wiadomościach, stany).
- Nazwa robocza: mlite.

## Evidence on Hand

- Pomiary z tego komputera (12.11.2026): lazyfmess w terminalu 26 MB pamięci roboczej bez logowania i 54 MB po 13 minutach połączenia; pierwsza wersja okna Win32 31 MB bez połączenia.
- Punkt odniesienia z prasy (inna aplikacja tej samej klasy): WhatsApp na WebView2 około 600 MB bezczynnie (Windows Latest, 2.10.2026).
- Brak pomiaru oficjalnego Messengera. Brak zrzutów prawdziwych rozmów: dane pokazowe są sztuczne i tak mają być oznaczane.

## Stan backendu sprawdzony w kodzie i w danych (12.11.2026)

Źródła: mautrix-meta v0.2610.0 (messagix, msgconv, connector), whatsmeow, rdzeń lazyfmess, lokalna baza i log z sesji właściciela (same liczby i nazwy procedur).

- Działa już w rdzeniu: tekst 1:1 i w grupach, odpowiedzi, reakcje dowolnym emoji, edycja i cofnięcie wiadomości, „pisze…", potwierdzenia odczytu z pozycją każdej osoby (100 znaczników w bazie), wyciszenie, wyszukiwanie osób i nowa rozmowa, wysyłanie plików, historia rozmów szyfrowanych jako sam tekst (3839 wiadomości od maja 2024 w 30 rozmowach szyfrowanych).
- Biblioteka to podaje, rdzeń jeszcze nie używa: stan „dostarczona" (whatsmeow `ReceiptTypeDelivered`, tabela `LSUpdateDeliveryReceipt`), zdjęcia profilowe (`ProfilePictureUrl`, `ThreadPictureUrl`), karty linków odebranych (tabela XMA; w szyfrowanych `MSG_EXTERNAL_LINK_SHARE`), media z rozmów szyfrowanych (`DownloadFB`), lista osób, które zareagowały (`FetchReactionsV2UserList`), zdjęcia, odpowiedzi i reakcje w odtwarzanej historii szyfrowanej.
- Potwierdzone testem na żywo (program `cmd/mdiag`, dwa przebiegi, tylko odczyt, 12.11.2026): pseudonimy przychodzą w tabeli uczestników `LSAddParticipantIdToGroupThread`, także dla rozmowy szyfrowanej 1:1 (pseudonim właściciela znaleziony; 3 wiersze z pseudonimem na 233); zdjęcia profilowe (152 z 152 kontaktów ma adres zdjęcia, 21 rozmów ma własne zdjęcie); znaczniki dostarczenia i odczytu na uczestnikach (86 i 137 z 233 wierszy) oraz 71 potwierdzeń odczytu; załączniki z adresem podglądu i pliku (8 z 8).
- Test na żywo dał wynik negatywny: obecność „aktywny teraz" (zero wierszy `LSDeleteThenInsertContactPresence` w obu przebiegach), osobna tabela `LSUpdateDeliveryReceipt` (zero wierszy; stan „dostarczona" trzeba brać ze znacznika na uczestniku).
- Nie zaobserwowane w teście, znane tylko z kodu: karty linków (zero wierszy XMA w pobranym oknie rozmów).
- Uwaga o bibliotece: wersja v0.2610.0 źle czyta dalsze pola tabeli `LSUpdateOrInsertThread` (392 ostrzeżenia o typie pola w jednym przebiegu), m.in. kolor i emoji rozmowy. Pola używane przez program (nazwa, zdjęcie, wyciszenie) leżą wcześniej i są poprawne.
- Brak w bibliotece: zmiana pseudonimu (nie ma takiego zadania), wyszukiwanie w treści wiadomości po stronie serwera, podgląd przy własnym linku w rozmowie szyfrowanej, rolki i filmy Facebooka udostępnione w rozmowie szyfrowanej (przychodzą jako „Unsupported message"), zakończenie sesji po stronie serwera (`LogoutRemote` czyści tylko dane lokalne).
- Stany sesji i logowania, które biblioteka faktycznie zgłasza (kod `httpclient/http.go`, `socket.go`, `bloks`): `ErrTokenInvalidated` (sesja wygasła), `ErrCheckpointRequired` i `ErrChallengeRequired` (weryfikacja konta), `ErrConsentRequired`, `ErrAccountSuspended`; zdarzenia połączenia `Connected`, `Reconnected`, `TransientDisconnect`, `PermanentError`. Logowanie hasłem ma kroki: dane logowania, wybór metody potwierdzenia, kod (`otp_code`), oczekiwanie na zatwierdzenie na innym urządzeniu, obrazek captcha; kroki wymagające strony WWW nie są obsługiwane bez przeglądarki. Według README lazyfmess wpisanie kodu 2FA nie działa w v0.2610.0.
- Poza backendem (zależy tylko od programu): zasobnik, główki, powiadomienia, autostart, ustawienia, podgląd zdjęć, odtwarzanie filmów z YouTube (wymaga silnika przeglądarki).

## Product Principles

1. Lekkość jest produktem: każda funkcja musi uzasadnić swój koszt pamięci i procesora.
2. Nie wchodzi w drogę: żyje w tle, pojawia się, gdy ktoś pisze, pozwala odpisać i zniknąć.
3. Uczciwie nieoficjalny: jasne stany połączenia, sesji i weryfikacji konta; nie udaje aplikacji Mety.
4. Użytkownik wybiera formę obecności: zasobnik, wąskie okno, pełne okno i „główki" są równorzędne.
5. Prywatnie: sesja i wiadomości zostają na komputerze użytkownika.
