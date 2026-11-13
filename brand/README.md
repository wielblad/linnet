# Linnet — identyfikacja wizualna

Wersja 1.2 · 13 listopada 2026

## Docelowy znak

Biała litera „l” w niebieskim kole. **Górny koniec pionowego trzonu dochodzi do górnej krawędzi koła.** Pozioma stopka jest gruba i wyraźna, ale kończy się wewnątrz koła. Trzon i stopka mają ok. 23% średnicy; cała litera ma szerokość ok. 42% średnicy.

Wzorzec: `01-logo/linnet-symbol-primary.svg`. Wersja do 16–32 px: `01-logo/linnet-symbol-small.svg`. Pliki SVG zawierają krzywe; logotyp nie wymaga zainstalowanego fontu. PNG mają przezroczyste tło poza kołem.

## Autorski logotyp słowny

Zatwierdzony napis odpowiada dużemu logotypowi z dostarczonej grafiki „Linnet / et”: zachowuje krótszą stopkę „L”, indywidualne kontury obu „n”, ukośną górę „t” i górne połączenie „e–t”. SVG zawiera kontury przeniesione z tego wzorca. Manrope służy tekstom marki.

- Sam napis: `01-logo/linnet-wordmark-dark.svg` (także white i blue).
- Pełny znak: `01-logo/linnet-lockup-dark.svg` (także white i blue).
- Detal ligatury: `01-logo/linnet-wordmark-et-detail.svg`.
- Kontury i ich pochodzenie: `04-system/wordmark-geometry.json`.
- Dokładna zaakceptowana grafika: `04-system/approved-wordmark-reference.png`.
- Duży napis wycięty ze wzorca: `04-system/approved-wordmark-hero.png`.

Przy sygnecie 256 jednostek napis ma szerokość około 430; przerwa to 48; pełny znak około 734 × 256. Nie rozdzielaj pary „e–t”, nie zmieniaj odstępów ani nie ściskaj liter. Zasoby ikony aplikacji zachowują odrębny wzorzec małej litery „l”.

## Zacznij tutaj

- `Linnet-raport-identyfikacji.pdf` — pełny, 20-stronicowy raport: marka, znak, paleta, typografia, dostępność, obecny UI i koncepcje rozwoju, zasoby Windows.
- `Linnet-raport-identyfikacji.html` — samodzielny raport z osadzonymi fontami i obrazami; działa bez internetu.
- `Linnet-plansza-marki.png` — plansza z finalnym znakiem.
- `Linnet-raport-tekstowy.md` — uzupełniający tekst raportu.

## Katalogi

| Katalog | Zawartość |
|---|---|
| 01-logo | Sygnet, logotyp i pełne zestawienie. SVG i PNG, kolor/mono. |
| 02-windows/win32 | Linnet.ico z 9 rozmiarami i pojedyncze PNG. |
| 02-windows/tray | Dwa monochromatyczne ICO, PNG i SVG. |
| 02-windows/msix | 57 PNG: targetsize, altform i skale. |
| 03-makiety | Jasny i ciemny UI: PNG 1440 × 960 oraz edytowalne HTML. |
| 04-system | design-tokens.json, CSS, geometria napisu i manifest weryfikacji. |
| 05-fonty | Manrope 400/500/600/700 i licencja OFL. |

Makiety HTML korzystają z sąsiednich katalogów logo/fontów; zachowaj strukturę folderów. Przeglądarka z blokadą lokalnych plików może wymagać uruchomienia prostego serwera HTTP. PNG i PDF nie wymagają serwera.

## UI obecny i proponowany

Raport uwzględnia dostarczony ekran obecnej aplikacji: wąska nawigacja, lista rozmów i panel czatu. Makiety pokazują przyszły kierunek rozwoju. Zawierają wyłącznie fikcyjne dane i nie są działającą aplikacją.

## Windows

Instrukcja integracji i źródła: `02-windows/README-Windows.md`. Pliki graficzne są gotowe do użycia w projekcie. Paczka nie jest instalatorem i nie zawiera manifestu ani podpisanego MSIX. Dobierz zasoby do technologii i kanału dystrybucji.

## Fonty i nazwy

Manrope: SIL Open Font License, plik licencji w 05-fonty. Segoe UI / Segoe UI Variable to zalecany font systemowego UI Windows; nie jest dołączony. Nazwy Messenger i Facebook opisują obsługiwany serwis. Przed publikacją sprawdź dostępność nazwy i znaku Linnet.

## Zgodność z zaakceptowaną grafiką

Kontur pochodzi wyłącznie z dużego napisu na wzorcu. Zachowano różnice między dwoma „n” i lekko owalny kształt kropki „i”. Sprawdzenie maski obrazu i wyrenderowanego SVG wykazało zgodność pola konturu na poziomie ponad 99,6% przy rozdzielczości źródłowej. W produkcyjnych wariantach stosowane są płaskie kolory palety Linnet.

## Sprawdzenie

Zweryfikowano wymiary i kanał alfa PNG, klatki trzech ICO, odwołania fontów i logo w makietach oraz skład wszystkich 20 stron PDF. Nie wykonywano testów uruchomieniowych w Windows ani testów działającej aplikacji.
