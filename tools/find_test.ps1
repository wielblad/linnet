# The picture of a conversation beside a head, shown large and then on the whole screen:
# takes pictures of the viewer in the window of the conversation, of the whole screen with
# its buttons and after they went away, and checks the way back with Esc.
# For a few seconds the picture covers the whole screen.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File tools\full_test.ps1 -Exe build\linnet.exe -DataDir .local\data-ui-test -OutDir .local\shots
param([string]$Exe, [string]$DataDir, [string]$OutDir)
$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Windows.Forms, System.Drawing
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class FT {
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  public struct RECT { public int L, T, R, B; }
}
"@
[FT]::SetProcessDPIAware() | Out-Null
$settings = '{"toasts":false,"body":true,"background":true,"show_mem":true,"dnd":false,"head_preview":false,"sound":false,"head_mode":"cols","heads":[{"id":101,"x":1500,"y":300}]}'
[IO.File]::WriteAllText((Join-Path $DataDir "settings.json"), $settings)
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline") -PassThru
try {
  Start-Sleep -Seconds 4
  function Top($cls) {
    $out = @(); $w = [IntPtr]::Zero
    for ($i = 0; $i -lt 300; $i++) {
      $w = [FT]::FindWindowEx([IntPtr]::Zero, $w, $cls, [NullString]::Value)
      if ($w -eq [IntPtr]::Zero) { break }
      $wpid = 0; [FT]::GetWindowThreadProcessId($w, [ref]$wpid) | Out-Null
      if ($wpid -eq $p.Id -and [FT]::IsWindowVisible($w)) { $out += $w }
    }
    return $out
  }
  function Rect($h) { $r = New-Object FT+RECT; [FT]::GetWindowRect($h, [ref]$r) | Out-Null; return $r }
  function Click($h, $x, $y) {
    $lp = [IntPtr](($y -shl 16) -bor ($x -band 0xFFFF))
    [FT]::PostMessage($h, 0x0201, [IntPtr]1, $lp) | Out-Null
    [FT]::PostMessage($h, 0x0202, [IntPtr]::Zero, $lp) | Out-Null
    Start-Sleep -Milliseconds 1200
  }
  function Key($h, $vk) { [FT]::PostMessage($h, 0x0100, [IntPtr]$vk, [IntPtr]1) | Out-Null; Start-Sleep -Milliseconds 700 }
  function Shot($h, $file) {
    $q = Rect $h
    $bmp = New-Object System.Drawing.Bitmap ($q.R - $q.L), ($q.B - $q.T)
    $g = [System.Drawing.Graphics]::FromImage($bmp); $g.CopyFromScreen($q.L, $q.T, 0, 0, $bmp.Size)
    $g.Dispose(); $bmp.Save((Join-Path $OutDir "$file.png"), [System.Drawing.Imaging.ImageFormat]::Png); $bmp.Dispose()
    "  {0}: ({1},{2}) {3}x{4}" -f $file, $q.L, $q.T, ($q.R - $q.L), ($q.B - $q.T)
  }
  Click (Top "MliteHead")[0] 20 20
  $pane = (Top "MlitePane")[0]; $q = Rect $pane
  Click $pane (($q.R - $q.L) - 108) 28          # the magnifier in the header
  $panel = [FT]::FindWindowEx($pane, [IntPtr]::Zero, "MlitePanel", [NullString]::Value)
  "panel wyszukiwania w oknie rozmowy: {0}, widoczny: {1}" -f ($panel -ne [IntPtr]::Zero), [FT]::IsWindowVisible($panel)
  $edit = [FT]::FindWindowEx($panel, [IntPtr]::Zero, "Edit", [NullString]::Value)
  foreach ($ch in "jutro".ToCharArray()) { [FT]::PostMessage($edit, 0x0102, [IntPtr][int]$ch, [IntPtr]::Zero) | Out-Null }
  Start-Sleep -Milliseconds 900
  Shot $pane "szukaj-1-wyniki"
  Click $panel 120 150                          # the first result: the conversation goes to that message
  "po kliknieciu wyniku: panel w oknie rozmowy {0}" -f ([FT]::FindWindowEx($pane, [IntPtr]::Zero, "MlitePanel", [NullString]::Value) -ne [IntPtr]::Zero)
  Shot $pane "szukaj-2-po-wyniku"
  Click $pane (($q.R - $q.L) - 108) 28
  Key ([FT]::FindWindowEx([FT]::FindWindowEx($pane, [IntPtr]::Zero, "MlitePanel", [NullString]::Value), [IntPtr]::Zero, "Edit", [NullString]::Value)) 0x1B
  "po Esc: panel w oknie rozmowy {0}" -f ([FT]::FindWindowEx($pane, [IntPtr]::Zero, "MlitePanel", [NullString]::Value) -ne [IntPtr]::Zero)
  Click $pane (($q.R - $q.L) - 108) 28          # open again, then close the pane with the search in it
  Click $pane (($q.R - $q.L) - 28) 28
  "okien rozmow po zamknieciu: {0}" -f @(Top "MlitePane").Count
  $main = [FT]::FindWindowEx([IntPtr]::Zero, [IntPtr]::Zero, "MliteMain", [NullString]::Value)
  "panel wrocil do okna programu: {0}" -f ([FT]::FindWindowEx($main, [IntPtr]::Zero, "MlitePanel", [NullString]::Value) -ne [IntPtr]::Zero)
  $p.Refresh(); "program dziala: {0}" -f (-not $p.HasExited)
} finally {
  if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force }
  Remove-Item (Join-Path $DataDir "settings.json") -ErrorAction SilentlyContinue
}
