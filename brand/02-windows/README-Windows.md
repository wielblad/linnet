# Linnet — zasoby graficzne dla Windows

Wersja 1.0 · 12 listopada 2026

Zestaw zawiera gotowe pliki graficzne do integracji w aplikacji Windows. Znak aplikacji to białe „l” na niebieskim kole; poza kołem płótno pozostaje przezroczyste. Wszystkie wersje kolorowe zachowują te same kolory i proporcje.

## Win32

`win32/Linnet.ico` zawiera osobne obrazy 16, 20, 24, 32, 40, 48, 64, 128 i 256 px. Każda klatka ma 32 bity z kanałem alfa. Plik można osadzić jako ikonę EXE, wykorzystać w skrócie oraz podłączyć do okna aplikacji zgodnie z używanym frameworkiem. Obok znajdują się źródłowe PNG.

Nie zastępuj całego ICO pojedynczym PNG przemianowanym na `.ico`. Nie generuj mniejszych obrazów przez skalowanie 16 px w górę.

## Ikony zasobnika systemowego

`tray/Linnet-tray-light.ico` i `tray/Linnet-tray-dark.ico` zawierają 16, 20, 24, 32, 40 i 48 px. Odpowiednie pojedyncze PNG i źródła SVG są w tym samym katalogu. `light` oznacza jasny znak przeznaczony na ciemne tło, a `dark` — ciemny znak przeznaczony na jasne tło. Są to uproszczone monochromatyczne litery bez niebieskiego koła.

Wybieraj wariant względem rzeczywistego koloru tła zasobnika. Tryb jasny/ciemny aplikacji może różnić się od motywu powłoki Windows. Udostępnij etykietę „Linnet” oraz tooltip ze stanem, np. „Linnet — 3 nieprzeczytane wiadomości”. Zapewnij wybór pozostawiania aplikacji w zasobniku i możliwość jej zamknięcia. Nie przekazuj statusu wyłącznie kolorem.

## MSIX — 57 PNG

Katalog `msix/` zawiera:

| Rodzina | Skale lub dokładne rozmiary |
|---|---|
| `Square44x44Logo.targetsize-{n}.png` | 16, 20, 24, 30, 32, 36, 40, 48, 60, 64, 72, 80, 96, 256 px |
| `Square44x44Logo.targetsize-{n}_altform-unplated.png` | te same 14 rozmiarów |
| `Square44x44Logo.targetsize-{n}_altform-lightunplated.png` | te same 14 rozmiarów |
| `Square44x44Logo.scale-{s}.png` | 100 / 125 / 150 / 200 / 400% → 44 / 55 / 66 / 88 / 176 px |
| `Square150x150Logo.scale-{s}.png` | 100 / 125 / 150 / 200 / 400% → 150 / 188 / 225 / 300 / 600 px |
| `StoreLogo.scale-{s}.png` | 100 / 125 / 150 / 200 / 400% → 50 / 63 / 75 / 100 / 200 px |

Pliki `targetsize` mają ikonę w pełnym kadrze; przezroczyste pozostają narożniki wokół niebieskiego koła. Wersje `_altform-unplated` i `_altform-lightunplated` zachowują koło marki — kwalifikator dotyczy dodatkowej systemowej płytki, nie usunięcia niebieskiego koła z samego znaku.

Pliki `scale` mają znak wyśrodkowany, z około 16% przezroczystego pola na stronę. Raster wymaga zaokrąglenia do pełnych pikseli. Nie zastosowano dodatkowego tła kafla. Nie rozciągnięto ani nie przycięto znaku.

W konkretnym manifeście bazowe odwołania powinny odpowiadać użytemu prefiksowi, np. `Assets\Square44x44Logo.png`, `Assets\Square150x150Logo.png` i `Assets\StoreLogo.png`, po skopiowaniu zawartości katalogu `msix/` do `Assets/` projektu. System zasobów dobiera odmiany według kwalifikatorów. Bazowe odwołanie nie oznacza konieczności wpisywania każdej odmiany jako osobnego atrybutu manifestu. Sprawdź sposób kompilowania zasobów we właściwym systemie budowania projektu.

## Zakres integracji

Win32 opisuje technologię aplikacji; MSIX — sposób jej pakietowania. Program Win32 może być dystrybuowany także jako MSIX i wtedy wykorzystywać oba zestawy zasobów.

Ten zestaw nie zawiera manifestu, instalatora ani podpisanego pakietu MSIX. Dodatkowe grafiki kafli, splash screen i materiały oferty Microsoft Store dobiera się do faktycznego manifestu, obsługiwanych wersji Windows i kanału dystrybucji. Gotowość publikacji wymaga walidacji konkretnego projektu i pakietu.

## Kontrast i dostępność

Motyw kontrastowy jest osobnym trybem względem zwykłego jasnego i ciemnego. Kolory interfejsu powinny być mapowane na dynamiczne zasoby systemowe, np. `SystemColorWindowColor` + `SystemColorWindowTextColor` oraz `SystemColorHighlightColor` + `SystemColorHighlightTextColor`. W HTML/WebView analogiczną rolę pełnią `forced-colors` i systemowe kolory CSS.

Monochromatyczne zasoby tray są wariantami graficznymi, nie automatyczną obsługą dowolnej palety kontrastowej. Aplikacja powinna wykrywać ustawienia systemu i dobierać lub renderować czytelny znak względem rzeczywistego tła. Zachowuj tekstowe etykiety, widoczny fokus klawiatury i informację o stanie niezależną od barwy. Sprawdź podgląd w rzeczywistej wielkości, powiększeniu DPI i trybie kontrastowym.

Powiadomienia powinny używać nazwy i ikony Linnet. Zaprojektuj wyciszenie i wybór wyświetlania treści wiadomości; uwzględniaj preferencje systemowe użytkownika.

## Weryfikacja plików

Sprawdzono nagłówki trzech plików ICO, liczbę i rozmiary wszystkich zawartych obrazów, poprawność odczytu przez Pillow oraz wymiary, RGBA i przezroczystość narożników wszystkich 57 PNG MSIX. Warianty `scale` sprawdzono również pod kątem przezroczystego pola ochronnego. Jest to weryfikacja plików, a nie test działania aplikacji w Windows.

## Oficjalne źródła

- [Microsoft Learn: konstrukcja ikon aplikacji Windows](https://learn.microsoft.com/en-us/windows/apps/design/iconography/app-icon-construction) — rozmiary, kwalifikatory skali, `targetsize`, warianty motywu.
- [Microsoft Learn: schemat uap:VisualElements](https://learn.microsoft.com/en-us/uwp/schemas/appxpackage/uapmanifestschema/element-uap-visualelements) — odwołania manifestu do grafik.
- [Microsoft Learn: obszar powiadomień Win32](https://learn.microsoft.com/en-us/windows/win32/shell/notification-area) — ikona zasobnika i DPI.
- [Microsoft Learn: motywy kontrastowe](https://learn.microsoft.com/en-us/windows/apps/design/accessibility/high-contrast-themes) — dynamiczne kolory systemowe.
- [Microsoft Learn: dostępne aplikacje Windows](https://learn.microsoft.com/en-us/windows/apps/develop/accessibility) — klawiatura, etykiety i kontrast.

Źródła sprawdzono 12 listopada 2026. Zalecenia Microsoft dotyczące ikon nie zastępują wymagań wybranego schematu manifestu i aktualnej walidacji publikacji.
