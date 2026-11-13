# Walks the whole update path on a test installation called "Linnet Test", without
# touching the real one: builds a newer version, signs it, publishes it on this computer,
# then has the installed test program find it, fetch it and replace itself.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File tools\update_test.ps1
$ErrorActionPreference = "Stop"
$root = Split-Path $PSScriptRoot
$name = "Linnet Test"
$slug = "linnet-test"
$dir = Join-Path $env:LOCALAPPDATA "Programs\$name"
$data = Join-Path $env:APPDATA $name
$dist = Join-Path $root "dist-test"
$shots = Join-Path $root ".local\shots"
$sh = "C:\Program Files\Git\bin\sh.exe"
$port = 8787

Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class UT {
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern IntPtr SendMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool MoveWindow(IntPtr h, int x, int y, int w, int hh, bool r);
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr dc, uint f);
  [DllImport("user32.dll")] public static extern IntPtr GetDlgItem(IntPtr h, int id);
  [DllImport("user32.dll")] public static extern bool ScreenToClient(IntPtr h, ref POINT p);
  [DllImport("user32.dll")] public static extern IntPtr ChildWindowFromPointEx(IntPtr h, POINT p, uint f);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  public struct RECT { public int L, T, R, B; }
  public struct POINT { public int X, Y; }
}
"@
[UT]::SetProcessDPIAware() | Out-Null

function Clean {
  Get-Process $slug -ErrorAction SilentlyContinue | Stop-Process -Force
  Start-Sleep -Milliseconds 800
  Remove-Item $dir, $data -Recurse -Force -ErrorAction SilentlyContinue
  Remove-Item (Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\$name.lnk") -Force -ErrorAction SilentlyContinue
  Remove-Item "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\LinnetTest" -Recurse -Force -ErrorAction SilentlyContinue
}
function MainWindow($procId) {
  $w = [IntPtr]::Zero
  for ($i = 0; $i -lt 300; $i++) {
    $w = [UT]::FindWindowEx([IntPtr]::Zero, $w, "MliteMain", [NullString]::Value)
    if ($w -eq [IntPtr]::Zero) { return [IntPtr]::Zero }
    $wpid = 0; [UT]::GetWindowThreadProcessId($w, [ref]$wpid) | Out-Null
    if ($wpid -eq $procId) { return $w }
  }
  return [IntPtr]::Zero
}
function Click($h, $x, $y) {
  $r = New-Object UT+RECT; [UT]::GetWindowRect($h, [ref]$r) | Out-Null
  $win = $h
  for ($d = 0; $d -lt 4; $d++) {
    $pt = New-Object UT+POINT; $pt.X = $r.L + $x; $pt.Y = $r.T + $y
    [UT]::ScreenToClient($win, [ref]$pt) | Out-Null
    $child = [UT]::ChildWindowFromPointEx($win, $pt, 1)
    if ($child -eq [IntPtr]::Zero -or $child -eq $win) { break }
    $win = $child
  }
  $pt = New-Object UT+POINT; $pt.X = $r.L + $x; $pt.Y = $r.T + $y
  [UT]::ScreenToClient($win, [ref]$pt) | Out-Null
  $lp = [IntPtr](($pt.Y -shl 16) -bor ($pt.X -band 0xFFFF))
  [UT]::PostMessage($win, 0x0200, [IntPtr]::Zero, $lp) | Out-Null; Start-Sleep -Milliseconds 150
  [UT]::PostMessage($win, 0x0201, [IntPtr]1, $lp) | Out-Null
  [UT]::PostMessage($win, 0x0202, [IntPtr]::Zero, $lp) | Out-Null
  Start-Sleep -Milliseconds 500
}
function Shot($h, $file) {
  $r = New-Object UT+RECT; [UT]::GetWindowRect($h, [ref]$r) | Out-Null
  if ($r.R - $r.L -lt 10) { "zrzut {0}: okna juz nie ma" -f $file; return }
  $bmp = New-Object System.Drawing.Bitmap ($r.R - $r.L), ($r.B - $r.T)
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  $dc = $g.GetHdc(); [UT]::PrintWindow($h, $dc, 2) | Out-Null; $g.ReleaseHdc($dc); $g.Dispose()
  $bmp.Save((Join-Path $shots $file), [System.Drawing.Imaging.ImageFormat]::Png); $bmp.Dispose()
}
function FindButton($bmp) {
  # the only wide blue area on the settings page is the button of the update row; the
  # switches are blue too, so a run of at least 60 blue pixels tells them apart
  for ($y = $bmp.Height - 30; $y -gt 60; $y -= 2) {
    $run = 0
    for ($x = 560; $x -lt $bmp.Width - 40; $x++) {
      $c = $bmp.GetPixel($x, $y)
      if ($c.B -gt 140 -and $c.B - $c.R -gt 60) {
        $run++
        if ($run -ge 60) { return @(($x - 30), $y) }
      } else { $run = 0 }
    }
  }
  return $null
}
function Version { (Get-ItemProperty "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\LinnetTest" -ErrorAction SilentlyContinue).DisplayVersion }

Clean
Remove-Item $dist -Recurse -Force -ErrorAction SilentlyContinue

# 1. the version that will be offered as the update, signed and put in dist-test
$install = Join-Path $root "cmd\mlite\install.go"
$orig = [IO.File]::ReadAllText($install)
try {
  [IO.File]::WriteAllText($install, ($orig -replace 'appVersion = "0\.1"', 'appVersion = "0.2"'), (New-Object Text.UTF8Encoding $false))
  & $sh "$root/tools/build.sh" | Out-Null
  & $sh "$root/tools/go.sh" run ./cmd/release sign -version 0.2 -url "http://127.0.0.1:$port/Linnet-Setup.exe" -notes "Wersja testowa." -out $dist | Select-Object -Last 2
} finally {
  [IO.File]::WriteAllText($install, $orig, (New-Object Text.UTF8Encoding $false))
}
& $sh "$root/tools/build.sh" | Out-Null
"zbudowano: 0.1 do instalacji, 0.2 do pobrania"

$serve = Start-Process -FilePath "python" -ArgumentList @("-I", "-m", "http.server", $port, "--bind", "127.0.0.1", "--directory", $dist) -PassThru -WindowStyle Hidden
try {
  Start-Sleep -Seconds 2
  # 2. install the older version under the test name
  $i = Start-Process -FilePath (Join-Path $root "build\Linnet-Setup.exe") -ArgumentList @("-install", "-quiet", "-name", "`"$name`"") -PassThru -Wait
  "instalacja 0.1: kod {0}, wersja w zainstalowanych aplikacjach: {1}" -f $i.ExitCode, (Version)

  # 3. run it, open the settings and ask for an update
  $env:MLITE_UPDATE_URL = "http://127.0.0.1:$port/latest.json"
  $p = Start-Process -FilePath (Join-Path $dir "$slug.exe") -ArgumentList @("-name", "`"$name`"", "-offline", "-self", "999") -PassThru
  $env:MLITE_UPDATE_URL = $null
  Start-Sleep -Seconds 5
  $h = MainWindow $p.Id
  if ($h -eq [IntPtr]::Zero) { throw "brak okna programu testowego" }
  $r0 = New-Object UT+RECT; [UT]::GetWindowRect($h, [ref]$r0) | Out-Null
  [UT]::MoveWindow($h, $r0.L, $r0.T, 1020, 900, $true) | Out-Null
  Start-Sleep -Milliseconds 600
  Click $h 44 836                      # Ustawienia on the rail
  Start-Sleep -Milliseconds 800
  $set = [UT]::FindWindowEx($h, [IntPtr]::Zero, "MliteSettings", [NullString]::Value)
  for ($k = 0; $k -lt 30; $k++) { [UT]::PostMessage($set, 0x0115, [IntPtr]1, [IntPtr]::Zero) | Out-Null }  # SB_LINEDOWN
  Start-Sleep -Milliseconds 900
  Shot $h "update-1-settings.png"

  # the update row is the one with a button: find it by its blue button in the picture
  Add-Type -AssemblyName System.Drawing
  $bmp = [System.Drawing.Bitmap]::FromFile((Join-Path $shots "update-1-settings.png"))
  $btn = FindButton $bmp
  $bmp.Dispose()
  if (-not $btn) { throw "nie znalazlem przycisku aktualizacji na zrzucie" }
  "przycisk aktualizacji w ({0},{1})" -f $btn[0], $btn[1]

  # 4. "Sprawdź"
  Click $h $btn[0] $btn[1]
  Start-Sleep -Seconds 4
  Shot $h "update-2-znaleziono.png"

  # 5. "Aktualizuj" (the same place; the row may have grown, so look again)
  $bmp = [System.Drawing.Bitmap]::FromFile((Join-Path $shots "update-2-znaleziono.png"))
  $btn2 = FindButton $bmp
  $bmp.Dispose()
  if (-not $btn2) { throw "nie ma przycisku Aktualizuj - prawdopodobnie nie znalazl nowej wersji" }
  Click $h $btn2[0] $btn2[1]
  Start-Sleep -Milliseconds 1200
  Shot $h "update-3-pobieranie.png"

  # 6. wait for the program to be replaced and started again
  $restarted = $false
  for ($k = 0; $k -lt 60 -and -not $restarted; $k++) {
    Start-Sleep -Seconds 1
    $now = Get-Process $slug -ErrorAction SilentlyContinue | Where-Object { $_.Id -ne $p.Id }
    if ($now -and (Version) -eq "0.2") { $restarted = $true }
  }
  $p.Refresh()
  "stara wersja zakonczona: {0} | wersja w zainstalowanych aplikacjach po: {1} | program dziala znowu: {2}" -f $p.HasExited, (Version), [bool](Get-Process $slug -ErrorAction SilentlyContinue)
  $new = Get-Process $slug -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($new) {
    $hn = MainWindow $new.Id
    if ($hn -ne [IntPtr]::Zero) {
      $r2 = New-Object UT+RECT; [UT]::GetWindowRect($hn, [ref]$r2) | Out-Null
      [UT]::MoveWindow($hn, $r2.L, $r2.T, 1020, 900, $true) | Out-Null
      Start-Sleep -Milliseconds 600
      Click $hn 44 836
      Start-Sleep -Milliseconds 800
      $set2 = [UT]::FindWindowEx($hn, [IntPtr]::Zero, "MliteSettings", [NullString]::Value)
      for ($k = 0; $k -lt 30; $k++) { [UT]::PostMessage($set2, 0x0115, [IntPtr]1, [IntPtr]::Zero) | Out-Null }
      Start-Sleep -Milliseconds 900
      Shot $hn "update-4-po-aktualizacji.png"
    }
  }
  "plik programu: {0:N0} bajtow" -f (Get-Item (Join-Path $dir "$slug.exe")).Length
} finally {
  if ($serve -and -not $serve.HasExited) { Stop-Process -Id $serve.Id -Force }
}
