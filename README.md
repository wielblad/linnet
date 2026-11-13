# Linnet

Lekki klient Messengera na Windows. Osobne okno, natywny interfejs, bez przeglądarki w środku.

> [!WARNING]
> Linnet jest niezależną aplikacją i nie jest powiązana z Meta Platforms, Inc. Messenger
> i Facebook są znakami ich właścicieli. Meta może ograniczyć lub wylogować konto, które
> korzysta z nieoficjalnego klienta — przeczytaj [Ryzyko](#ryzyko) przed zalogowaniem.

## Co potrafi

- Lista rozmów ze zdjęciami, licznikiem nieprzeczytanych i podglądem ostatniej wiadomości.
- Rozmowa: dymki, odpowiedzi z cytatem, reakcje, edycja i cofanie wiadomości, stany
  „wysłano / dostarczono / przeczytano”, doczytywanie starszych wiadomości przy przewijaniu.
- Zdjęcia i naklejki w rozmowie, podgląd na całe okno z pobieraniem, załączniki i pliki.
- Podgląd linków (obrazek i tytuł), filmy z YouTube i Facebooka odtwarzane w oknie programu.
- Szukanie w rozmowie i panel z jej zdjęciami, plikami i linkami.
- Główki rozmów na pulpicie z dymkiem nowej wiadomości.
- Powiadomienia Windows z polem szybkiej odpowiedzi, ikona w zasobniku z licznikiem,
  tryb „Nie przeszkadzać”, dźwięk nowej wiadomości.
- Jasny i ciemny motyw zgodny z Windows, skalowanie tekstu, wysoki kontrast.
- Zużycie pamięci w tle: około 30 MB.

## Instalacja

1. Pobierz [`release/Linnet-Setup.exe`](release/Linnet-Setup.exe).
2. Uruchom plik. Instalacja nie wymaga uprawnień administratora: program trafia do
   `%LOCALAPPDATA%\Programs\Linnet`, skrót do menu Start, wpis do „Zainstalowane aplikacje”.
3. Przy pierwszym uruchomieniu zaloguj się do Messengera w oknie programu. Dane logowania
   trafiają wyłącznie do Mety; na tym komputerze zapisywana jest tylko sesja
   (`%APPDATA%\Linnet`).

Windows SmartScreen pokaże ostrzeżenie o nieznanym wydawcy, bo plik nie jest podpisany
certyfikatem: „Więcej informacji” → „Uruchom mimo to”.

## Aktualizacje

Program sam sprawdza, czy jest nowsza wersja (przy starcie i raz dziennie), a w
Ustawieniach pokazuje przycisk **Aktualizuj**. Aktualizacja pobiera instalator z tego
repozytorium, sprawdza jego sumę kontrolną i podpis wydawcy (Ed25519), po czym podmienia
program i uruchamia go ponownie. Plik niezgodny z podpisem jest odrzucany.

Krótka instrukcja do przesłania komuś innemu: [docs/dla-kolegi.md](docs/dla-kolegi.md).

## Ryzyko

- **Konto.** Meta wykrywa nietypową aktywność. Każda metoda logowania może skończyć się
  prośbą o potwierdzenie tożsamości, wylogowaniem albo czasowym ograniczeniem konta;
  użytkownicy [mautrix-meta](https://github.com/mautrix/meta/issues/44) zgłaszali też
  zablokowane konta. Ryzyko zmniejsza logowanie z domowego łącza (nie przez VPN) i unikanie
  częstych ponownych logowań.
- **Brak gwarancji.** Program powstaje na własny użytek i jest udostępniany bez gwarancji.

## Budowanie ze źródeł

Wymagane: Go 1.27 na Windows (w repozytorium są skrypty, które używają kopii Go z
`.local/toolchain`, jeśli jest).

```sh
sh tools/build.sh     # build/linnet.exe oraz build/Linnet-Setup.exe
sh tools/go.sh test ./...
```

`tools/mkres.py` wbudowuje ikony z `brand/` w plik programu. Narzędzia testowe
(`tools/*.ps1`) sterują zbudowanym programem i robią zrzuty ekranu.

### Wydanie nowej wersji

```sh
sh tools/go.sh run ./cmd/release keygen                        # raz, tworzy klucz wydawcy
sh tools/build.sh
sh tools/go.sh run ./cmd/release sign -version 0.2 -notes "Co nowego."
git add latest.json release/Linnet-Setup.exe && git commit && git push
```

Klucz prywatny leży w `.local/keys/` i nigdy nie trafia do repozytorium. Jego publiczna
połowa jest wkompilowana w program (`cmd/mlite/updatekey.go`).

## Licencja i podziękowania

Linnet jest na licencji [GNU AGPL v3](LICENSE). Powstał na rdzeniu
[lazyfmess](https://github.com/samjoshuadud/lazyfmess) (AGPL v3) — z niego pochodzą pakiety
`internal/meta`, `internal/store` i `internal/session`, tutaj rozwijane dalej; interfejs
Windows (`cmd/mlite`, `internal/webview`, `internal/toast`, `internal/update`) jest nowy.

Pod spodem pracują [mautrix-meta](https://github.com/mautrix/meta) (AGPL v3),
[whatsmeow](https://github.com/tulir/whatsmeow) i [mautrix-go](https://github.com/mautrix/go)
(MPL 2.0) oraz [libsignal-protocol-go](https://github.com/tulir/libsignal-protocol-go)
(GPL v3), a także biblioteki na licencjach MIT, BSD, ISC i Apache 2.0.

Identyfikacja graficzna (`brand/`) jest częścią tego projektu. Font
[Manrope](https://github.com/google/fonts/tree/main/ofl/manrope) jest na licencji SIL Open
Font License.
