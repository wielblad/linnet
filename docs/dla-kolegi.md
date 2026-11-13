# Linnet — jak zacząć

Linnet to lekki klient Messengera na Windows: osobne okno zamiast karty w przeglądarce,
około 30 MB pamięci w tle.

## Instalacja

1. Pobierz instalator: <https://github.com/wielblad/linnet/raw/main/release/Linnet-Setup.exe>
2. Uruchom go. Windows pokaże ostrzeżenie „Nie można zweryfikować wydawcy” — plik nie ma
   certyfikatu, bo to program robiony prywatnie. Kliknij **Więcej informacji** →
   **Uruchom mimo to**.
3. Instalacja nie pyta o nic i nie wymaga uprawnień administratora. Program trafia do
   Twojego profilu, a skrót „Linnet” do menu Start.

## Pierwsze uruchomienie

Otworzy się ekran logowania do Messengera. Wpisujesz swoje dane tak jak na stronie
Facebooka — trafiają prosto do Mety, program ich nie zapisuje. Na komputerze zostaje tylko
sesja (to samo, co ciasteczko w przeglądarce), w folderze `%APPDATA%\Linnet`.

Po zalogowaniu Linnet pobiera listę rozmów. Pierwsze wejście w starszą rozmowę może chwilę
potrwać, bo doczytuje historię.

## Co warto wiedzieć

- **Aktualizacje**: program sam sprawdza nowe wersje. W Ustawieniach jest przycisk
  „Aktualizuj” — jedno kliknięcie i program podmienia się sam.
- **W tle**: zamknięcie okna zostawia program w zasobniku, żeby dalej odbierał wiadomości.
  Można to wyłączyć w Ustawieniach.
- **Główki na pulpicie**: prawy przycisk na rozmowie na liście → „Przypnij jako główkę”.
- **Powiadomienia**: można odpisać prosto z powiadomienia Windows.
- **Filmy**: rolki z Facebooka i filmy z YouTube grają w oknie programu; zwykłe linki
  otwierają się w Twojej przeglądarce.

## Zanim się zalogujesz

To nieoficjalny klient. Meta nie udostępnia API dla zwykłych kont, więc program łączy się
tak jak aplikacja Messenger Lite. Meta może potraktować to jako nietypową aktywność:
w najgorszym razie poprosi o potwierdzenie tożsamości albo wyloguje. Takie zgłoszenia
zdarzały się użytkownikom podobnych narzędzi. Jeśli nie chcesz tego ryzykować na swoim
koncie — nie instaluj.

Program jest na licencji AGPL v3, kod jest w całości tutaj:
<https://github.com/wielblad/linnet>
