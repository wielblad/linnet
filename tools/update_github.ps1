# Checks the update path against the real published release: installs a build whose version
# is lower than the published one under the name "Linnet Test", then lets it find, fetch and
# install the release from GitHub. The real installation is not touched.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File tools\update_github.ps1
$ErrorActionPreference = "Stop"
$root = Split-Path $PSScriptRoot
$name = "Linnet Test"
$slug = "linnet-test"
$dir = Join-Path $env:LOCALAPPDATA "Programs\$name"
$data = Join-Path $env:APPDATA $name
$shots = Join-Path $root ".local\shots"
$sh = "C:\Program Files\Git\bin\sh.exe"

Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class UG {
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool MoveWindow(IntPtr h, int x, int y, int w, int hh, bool r);
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr dc, uint f);
  [DllImport("user32.dll")] public static extern bool ScreenToClient(IntPtr h, ref POINT p);
  [DllImport("user32.dll")] public static extern IntPtr ChildWindowFromPointEx(IntPtr h, POINT p, uint f);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  public struct RECT { public int L, T, R, B; }
  public struct POINT { public int X, Y; }
}
"@
[UG]::SetProcessDPIAware() | Out-Null

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
    $w = [UG]::FindWindowEx([IntPtr]::Zero, $w, "MliteMain", [NullString]::Value)
    if ($w -eq [IntPtr]::Zero) { return [IntPtr]::Zero }
    $wpid = 0; [UG]::GetWindowThreadProcessId($w, [ref]$wpid) | Out-Null
    if ($wpid -eq $procId) { return $w }
  }
  return [IntPtr]::Zero
}
function Click($h, $x, $y) {
  $r = New-Object UG+RECT; [UG]::GetWindowRect($h, [ref]$r) | Out-Null
  $win = $h
  for ($d = 0; $d -lt 4; $d++) {
    $pt = New-Object UG+POINT; $pt.X = $r.L + $x; $pt.Y = $r.T + $y
    [UG]::ScreenToClient($win, [ref]$pt) | Out-Null
    $child = [UG]::ChildWindowFromPointEx($win, $pt, 1)
    if ($child -eq [IntPtr]::Zero -or $child -eq $win) { break }
    $win = $child
  }
  $pt = New-Object UG+POINT; $pt.X = $r.L + $x; $pt.Y = $r.T + $y
  [UG]::ScreenToClient($win, [ref]$pt) | Out-Null
  $lp = [IntPtr](($pt.Y -shl 16) -bor ($pt.X -band 0xFFFF))
  [UG]::PostMessage($win, 0x0200, [IntPtr]::Zero, $lp) | Out-Null; Start-Sleep -Milliseconds 150
  [UG]::PostMessage($win, 0x0201, [IntPtr]1, $lp) | Out-Null
  [UG]::PostMessage($win, 0x0202, [IntPtr]::Zero, $lp) | Out-Null
  Start-Sleep -Milliseconds 500
}
function Shot($h, $file) {
  $r = New-Object UG+RECT; [UG]::GetWindowRect($h, [ref]$r) | Out-Null
  if ($r.R - $r.L -lt 10) { "zrzut {0}: okna juz nie ma" -f $file; return }
  $bmp = New-Object System.Drawing.Bitmap ($r.R - $r.L), ($r.B - $r.T)
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  $dc = $g.GetHdc(); [UG]::PrintWindow($h, $dc, 2) | Out-Null; $g.ReleaseHdc($dc); $g.Dispose()
  $bmp.Save((Join-Path $shots $file), [System.Drawing.Imaging.ImageFormat]::Png); $bmp.Dispose()
}
function FindButton($bmp) {
  for ($y = $bmp.Height - 30; $y -gt 60; $y -= 2) {
    $run = 0
    for ($x = 560; $x -lt $bmp.Width - 40; $x++) {
      $c = $bmp.GetPixel($x, $y)
      $up = $bmp.GetPixel($x, $y - 10)   # a button is tall; the outline of a focused row is a thin line
      if ($c.B -gt 140 -and $c.B - $c.R -gt 60 -and $up.B -gt 140 -and $up.B - $up.R -gt 60) {
        $run++
        if ($run -ge 60) { return @(($x - 30), $y) }
      } else { $run = 0 }
    }
  }
  return $null
}
function Version { (Get-ItemProperty "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\LinnetTest" -ErrorAction SilentlyContinue).DisplayVersion }

Clean
# a build older than what is published, so the published release is the newer one
$install = Join-Path $root "cmd\mlite\install.go"
$orig = [IO.File]::ReadAllText($install)
try {
  [IO.File]::WriteAllText($install, ($orig -replace 'appVersion = "[^"]+"', 'appVersion = "0.0.9"'), (New-Object Text.UTF8Encoding $false))
  & $sh "$root/tools/build.sh" | Out-Null
  Copy-Item (Join-Path $root "build\Linnet-Setup.exe") (Join-Path $env:TEMP "Linnet-Setup-009.exe") -Force
} finally {
  [IO.File]::WriteAllText($install, $orig, (New-Object Text.UTF8Encoding $false))
}
& $sh "$root/tools/build.sh" | Out-Null
"zbudowano wersje 0.0.9 do instalacji; opublikowana jest {0}" -f ((Get-Content (Join-Path $root "latest.json") -Raw | ConvertFrom-Json).version)

$published = (Get-Content (Join-Path $root "latest.json") -Raw | ConvertFrom-Json).version
$i = Start-Process -FilePath (Join-Path $env:TEMP "Linnet-Setup-009.exe") -ArgumentList @("-install", "-quiet", "-name", "`"$name`"") -PassThru -Wait
"instalacja 0.0.9: kod {0}, wersja: {1}" -f $i.ExitCode, (Version)

$p = Start-Process -FilePath (Join-Path $dir "$slug.exe") -ArgumentList @("-name", "`"$name`"", "-offline", "-self", "999") -PassThru
Start-Sleep -Seconds 5
$h = MainWindow $p.Id
if ($h -eq [IntPtr]::Zero) { throw "brak okna" }
$r0 = New-Object UG+RECT; [UG]::GetWindowRect($h, [ref]$r0) | Out-Null
[UG]::MoveWindow($h, $r0.L, $r0.T, 1020, 900, $true) | Out-Null
Start-Sleep -Milliseconds 600
Click $h 44 836
Start-Sleep -Milliseconds 800
$set = [UG]::FindWindowEx($h, [IntPtr]::Zero, "MliteSettings", [NullString]::Value)
for ($k = 0; $k -lt 30; $k++) { [UG]::PostMessage($set, 0x0115, [IntPtr]1, [IntPtr]::Zero) | Out-Null }
Start-Sleep -Milliseconds 900
Shot $h "github-1-przed.png"
Add-Type -AssemblyName System.Drawing
$bmp = [System.Drawing.Bitmap]::FromFile((Join-Path $shots "github-1-przed.png"))
$btn = FindButton $bmp
$bmp.Dispose()
if (-not $btn) { throw "brak przycisku aktualizacji" }
Click $h $btn[0] $btn[1]        # Sprawdź: reaches GitHub
Start-Sleep -Seconds 8
Shot $h "github-2-znaleziono.png"
$bmp = [System.Drawing.Bitmap]::FromFile((Join-Path $shots "github-2-znaleziono.png"))
$btn2 = FindButton $bmp
$bmp.Dispose()
if (-not $btn2) { throw "nie znalazl nowszej wersji na GitHubie" }
Click $h $btn2[0] $btn2[1]      # Aktualizuj: fetches 28 MB from GitHub
Start-Sleep -Seconds 3
Shot $h "github-3-pobieranie.png"
$ok = $false
for ($k = 0; $k -lt 180 -and -not $ok; $k++) {
  Start-Sleep -Seconds 1
  if ((Version) -eq $published) { $ok = $true }
}
$p.Refresh()
"stara wersja zakonczona: {0} | wersja po aktualizacji: {1} | program dziala: {2}" -f $p.HasExited, (Version), [bool](Get-Process $slug -ErrorAction SilentlyContinue)
Start-Sleep -Seconds 3
Clean
"instalacja testowa usunieta"
