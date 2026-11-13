# Shows the bar about a new version on a test installation: installs an older build under
# the name "Linnet Test", lets it find the published release, and looks at the main window.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File tools\banner_test.ps1
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
public class BN {
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
[BN]::SetProcessDPIAware() | Out-Null

function Clean {
  Get-Process $slug -ErrorAction SilentlyContinue | Stop-Process -Force
  Start-Sleep -Milliseconds 800
  Remove-Item $dir, $data -Recurse -Force -ErrorAction SilentlyContinue
  Remove-Item (Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\$name.lnk") -Force -ErrorAction SilentlyContinue
  Remove-Item "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\LinnetTest" -Recurse -Force -ErrorAction SilentlyContinue
}
function Click($h, $x, $y) {
  $r = New-Object BN+RECT; [BN]::GetWindowRect($h, [ref]$r) | Out-Null
  $win = $h
  for ($d = 0; $d -lt 4; $d++) {
    $pt = New-Object BN+POINT; $pt.X = $r.L + $x; $pt.Y = $r.T + $y
    [BN]::ScreenToClient($win, [ref]$pt) | Out-Null
    $child = [BN]::ChildWindowFromPointEx($win, $pt, 1)
    if ($child -eq [IntPtr]::Zero -or $child -eq $win) { break }
    $win = $child
  }
  $pt = New-Object BN+POINT; $pt.X = $r.L + $x; $pt.Y = $r.T + $y
  [BN]::ScreenToClient($win, [ref]$pt) | Out-Null
  $lp = [IntPtr](($pt.Y -shl 16) -bor ($pt.X -band 0xFFFF))
  [BN]::PostMessage($win, 0x0200, [IntPtr]::Zero, $lp) | Out-Null; Start-Sleep -Milliseconds 150
  [BN]::PostMessage($win, 0x0201, [IntPtr]1, $lp) | Out-Null
  [BN]::PostMessage($win, 0x0202, [IntPtr]::Zero, $lp) | Out-Null
  Start-Sleep -Milliseconds 500
}
function Shot($h, $file) {
  $r = New-Object BN+RECT; [BN]::GetWindowRect($h, [ref]$r) | Out-Null
  if ($r.R - $r.L -lt 10) { "zrzut {0}: brak okna" -f $file; return }
  $bmp = New-Object System.Drawing.Bitmap ($r.R - $r.L), ($r.B - $r.T)
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  $dc = $g.GetHdc(); [BN]::PrintWindow($h, $dc, 2) | Out-Null; $g.ReleaseHdc($dc); $g.Dispose()
  $bmp.Save((Join-Path $shots $file), [System.Drawing.Imaging.ImageFormat]::Png); $bmp.Dispose()
  "zrzut {0}" -f $file
}

Clean
$install = Join-Path $root "cmd\mlite\install.go"
$orig = [IO.File]::ReadAllText($install)
try {
  [IO.File]::WriteAllText($install, ($orig -replace 'appVersion = "0\.3"', 'appVersion = "0.2"'), (New-Object Text.UTF8Encoding $false))
  & $sh "$root/tools/build.sh" | Out-Null
  Copy-Item (Join-Path $root "build\Linnet-Setup.exe") (Join-Path $env:TEMP "Linnet-Setup-02.exe") -Force
} finally {
  [IO.File]::WriteAllText($install, $orig, (New-Object Text.UTF8Encoding $false))
}
& $sh "$root/tools/build.sh" | Out-Null
$i = Start-Process -FilePath (Join-Path $env:TEMP "Linnet-Setup-02.exe") -ArgumentList @("-install", "-quiet", "-name", "`"$name`"") -PassThru -Wait
"zainstalowano 0.2 jako $name (kod {0}); opublikowana jest 0.3" -f $i.ExitCode

$env:MLITE_THEME = "dark"
$p = Start-Process -FilePath (Join-Path $dir "$slug.exe") -ArgumentList @("-name", "`"$name`"", "-offline", "-self", "999", "-data-dir", "`"$(Join-Path $root ".local\data-ui-test")`"") -PassThru
$env:MLITE_THEME = $null
try {
  Start-Sleep -Seconds 5
  $h = [IntPtr]::Zero; $w = [IntPtr]::Zero
  for ($k = 0; $k -lt 300; $k++) {
    $w = [BN]::FindWindowEx([IntPtr]::Zero, $w, "MliteMain", [NullString]::Value)
    if ($w -eq [IntPtr]::Zero) { break }
    $wpid = 0; [BN]::GetWindowThreadProcessId($w, [ref]$wpid) | Out-Null
    if ($wpid -eq $p.Id) { $h = $w; break }
  }
  if ($h -eq [IntPtr]::Zero) { throw "brak okna" }
  $r0 = New-Object BN+RECT; [BN]::GetWindowRect($h, [ref]$r0) | Out-Null
  [BN]::MoveWindow($h, $r0.L, $r0.T, 1020, 760, $true) | Out-Null
  Start-Sleep -Milliseconds 600
  # ask for a check in the settings, then come back to the conversations
  Click $h 44 696
  Start-Sleep -Milliseconds 700
  $set = [BN]::FindWindowEx($h, [IntPtr]::Zero, "MliteSettings", [NullString]::Value)
  for ($k = 0; $k -lt 30; $k++) { [BN]::PostMessage($set, 0x0115, [IntPtr]1, [IntPtr]::Zero) | Out-Null }
  Start-Sleep -Milliseconds 800
  Shot $h "pasek-0-ustawienia.png"
  Add-Type -AssemblyName System.Drawing
  $bmp = [System.Drawing.Bitmap]::FromFile((Join-Path $shots "pasek-0-ustawienia.png"))
  $btn = $null
  for ($y = $bmp.Height - 30; $y -gt 60 -and -not $btn; $y -= 2) {
    $run = 0
    for ($x = 560; $x -lt $bmp.Width - 40; $x++) {
      $c = $bmp.GetPixel($x, $y)
      if ($c.B -gt 140 -and $c.B - $c.R -gt 60) { $run++; if ($run -ge 60) { $btn = @(($x - 30), $y); break } } else { $run = 0 }
    }
  }
  $bmp.Dispose()
  if (-not $btn) { throw "brak przycisku Sprawdz" }
  function Size($label) {
    $r = New-Object BN+RECT; [BN]::GetWindowRect($h, [ref]$r) | Out-Null
    "  {0}: okno {1}x{2} w ({3},{4})" -f $label, ($r.R - $r.L), ($r.B - $r.T), $r.L, $r.T
  }
  Size "przed Sprawdz"
  Click $h $btn[0] $btn[1]
  Start-Sleep -Seconds 8
  Size "po Sprawdz"
  Click $h 44 64            # back to Rozmowy
  Start-Sleep -Milliseconds 900
  Size "po powrocie na liste"
  Shot $h "pasek-1-lista.png"
  # the bar sits under the search box; a click on it should open the settings
  Click $h 220 150
  Start-Sleep -Seconds 1
  Shot $h "pasek-2-po-kliknieciu.png"
  $p.Refresh()
  "program dziala: {0}" -f (-not $p.HasExited)
} finally {
  if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force }
  Clean
  "instalacja testowa usunieta"
}
