param([string]$Exe, [string]$DataDir, [string]$Out, [int]$Select = 2, [string]$Clicks = "", [int]$Width = 1020, [int]$Height = 760, [string]$Extra = "", [switch]$Real)
# Starts the program on test data with MLITE_DRYOPEN (nothing is really opened), selects a
# conversation, clicks at the given points and saves a picture of the window.
# -Clicks "x,y;x,y" are coordinates in the saved picture (window-relative).
# A point may carry a verb: "move:x,y", "down:x,y", "up:x,y", "right:x,y", "key:VK", "ctrl:VK", "wait:ms", "shot:name".
Add-Type -AssemblyName System.Windows.Forms
$keepText = $null; if ([Windows.Forms.Clipboard]::ContainsText()) { $keepText = [Windows.Forms.Clipboard]::GetText() }
$keepImage = $null; if ($keepText -eq $null -and [Windows.Forms.Clipboard]::ContainsImage()) { $keepImage = [Windows.Forms.Clipboard]::GetImage() }
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class CK {
  [DllImport("user32.dll")] public static extern IntPtr SendMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll", CharSet=CharSet.Unicode, EntryPoint="SendMessageW")] public static extern IntPtr SendText(IntPtr h, uint m, IntPtr w, string l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool MoveWindow(IntPtr h, int x, int y, int w, int hh, bool r);
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr dc, uint f);
  [DllImport("user32.dll")] public static extern IntPtr GetDlgItem(IntPtr h, int id);
  [DllImport("user32.dll")] public static extern bool ScreenToClient(IntPtr h, ref POINT p);
  [DllImport("user32.dll")] public static extern IntPtr WindowFromPoint(POINT p);
  [DllImport("user32.dll")] public static extern IntPtr ChildWindowFromPointEx(IntPtr h, POINT p, uint f);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool AttachThreadInput(uint a, uint b, bool f);
  [DllImport("user32.dll")] public static extern bool GetKeyboardState(byte[] s);
  [DllImport("user32.dll")] public static extern bool SetKeyboardState(byte[] s);
  [DllImport("user32.dll")] public static extern IntPtr GetFocus();
  [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr h, IntPtr after, int x, int y, int cx, int cy, uint flags);
  [DllImport("kernel32.dll")] public static extern uint GetCurrentThreadId();
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  public struct RECT { public int L, T, R, B; }
  public struct POINT { public int X, Y; }
}
"@
[CK]::SetProcessDPIAware() | Out-Null
if (-not $Real) { $env:MLITE_DRYOPEN = "1" }; $env:MLITE_KEEPHOVER = "1"
$argList = @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline")
if ($Extra -ne "") { $argList += $Extra.Split(" ") }
$p = Start-Process -FilePath $Exe -ArgumentList $argList -PassThru
$env:MLITE_DRYOPEN = $null; $env:MLITE_KEEPHOVER = $null
try {
  Start-Sleep -Seconds 3
  $h = [IntPtr]::Zero; $w = [IntPtr]::Zero; $uiThread = 0
  for ($i = 0; $i -lt 300; $i++) {
    $w = [CK]::FindWindowEx([IntPtr]::Zero, $w, "MliteMain", [NullString]::Value)
    if ($w -eq [IntPtr]::Zero) { break }
    $wpid = 0; $tid = [CK]::GetWindowThreadProcessId($w, [ref]$wpid)
    if ($wpid -eq $p.Id) { $h = $w; $uiThread = $tid; break }
  }
  if ($h -eq [IntPtr]::Zero) { "brak okna"; return }
  $r0 = New-Object CK+RECT; [CK]::GetWindowRect($h, [ref]$r0) | Out-Null
  [CK]::MoveWindow($h, $r0.L, $r0.T, $Width, $Height, $true) | Out-Null
  Start-Sleep -Milliseconds 400
  if ($Select -ge 0) {
    $lb = [CK]::GetDlgItem($h, 311)
    [CK]::SendMessage($lb, 0x0186, [IntPtr]$Select, [IntPtr]::Zero) | Out-Null
    [CK]::SendMessage($h, 0x0111, [IntPtr]((1 -shl 16) -bor 311), $lb) | Out-Null
    Start-Sleep -Seconds 1
  }
  function Target($x, $y) {
    # the deepest child window of this program under the picture point, and the point in its coordinates
    $r = New-Object CK+RECT; [CK]::GetWindowRect($h, [ref]$r) | Out-Null
    $win = $h
    for ($d = 0; $d -lt 4; $d++) {
      $pt = New-Object CK+POINT; $pt.X = $r.L + $x; $pt.Y = $r.T + $y
      [CK]::ScreenToClient($win, [ref]$pt) | Out-Null
      $child = [CK]::ChildWindowFromPointEx($win, $pt, 1)
      if ($child -eq [IntPtr]::Zero -or $child -eq $win) { break }
      $win = $child
    }
    $pt = New-Object CK+POINT; $pt.X = $r.L + $x; $pt.Y = $r.T + $y
    [CK]::ScreenToClient($win, [ref]$pt) | Out-Null
    return @($win, [IntPtr](($pt.Y -shl 16) -bor ($pt.X -band 0xFFFF)))
  }
  function Shot($file) {
    $r = New-Object CK+RECT; [CK]::GetWindowRect($h, [ref]$r) | Out-Null
    $bmp = New-Object System.Drawing.Bitmap ($r.R - $r.L), ($r.B - $r.T)
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $dc = $g.GetHdc(); [CK]::PrintWindow($h, $dc, 2) | Out-Null; $g.ReleaseHdc($dc); $g.Dispose()
    $bmp.Save($file, [System.Drawing.Imaging.ImageFormat]::Png); $bmp.Dispose()
  }
  function Key($vk, $ctrl, $cls) {
    $me = [CK]::GetCurrentThreadId()
    [CK]::AttachThreadInput($me, $uiThread, $true) | Out-Null
    $focus = [CK]::GetFocus(); if ($focus -eq [IntPtr]::Zero) { $focus = $h }
    if ($cls) { $focus = [CK]::FindWindowEx($h, [IntPtr]::Zero, $cls, [NullString]::Value) }
    $st = New-Object byte[] 256
    [CK]::GetKeyboardState($st) | Out-Null
    if ($ctrl) { $st[0x11] = 0x80 } else { $st[0x11] = 0 }
    [CK]::SetKeyboardState($st) | Out-Null
    [CK]::PostMessage($focus, 0x0100, [IntPtr]$vk, [IntPtr]1) | Out-Null
    Start-Sleep -Milliseconds 500
    $st[0x11] = 0
    [CK]::SetKeyboardState($st) | Out-Null
    [CK]::AttachThreadInput($me, $uiThread, $false) | Out-Null
  }
  foreach ($step in ($Clicks -split ";" | Where-Object { $_ -ne "" })) {
    $verb = "click"; $arg = $step
    if ($step -match "^(\w+):(.*)$") { $verb = $matches[1]; $arg = $matches[2] }
    switch ($verb) {
      "wait" { Start-Sleep -Milliseconds ([int]$arg) }
      "menukey" { [CK]::PostMessage($h, 0x0100, [IntPtr][int][char]$arg.ToUpper(), [IntPtr]1) | Out-Null; Start-Sleep -Milliseconds 700 }
      "screen" { [CK]::SetWindowPos($h, [IntPtr](-1), 0, 0, 0, 0, 0x0043) | Out-Null; Start-Sleep -Milliseconds 400; $r = New-Object CK+RECT; [CK]::GetWindowRect($h, [ref]$r) | Out-Null; $bmp = New-Object System.Drawing.Bitmap ($r.R - $r.L), ($r.B - $r.T); $g = [System.Drawing.Graphics]::FromImage($bmp); $g.CopyFromScreen($r.L, $r.T, 0, 0, $bmp.Size); $g.Dispose(); $bmp.Save((Join-Path (Split-Path $Out) "$arg.png"), [System.Drawing.Imaging.ImageFormat]::Png); $bmp.Dispose(); [CK]::SetWindowPos($h, [IntPtr](-2), 0, 0, 0, 0, 0x0013) | Out-Null }
      "procs" { $n = @(Get-Process msedgewebview2 -ErrorAction SilentlyContinue).Count; $p.Refresh(); "procesy przegladarki [{0}]: {1}, pamiec programu {2:N1} MB" -f $arg, $n, ((Get-CimInstance Win32_PerfFormattedData_PerfProc_Process -Filter "IDProcess=$($p.Id)").WorkingSetPrivate/1MB) }
      "type" { $pan = [CK]::FindWindowEx($h, [IntPtr]::Zero, "MlitePanel", [NullString]::Value); $ed = [CK]::FindWindowEx($pan, [IntPtr]::Zero, "EDIT", [NullString]::Value); [CK]::SendText($ed, 0x00C2, [IntPtr]1, $arg) | Out-Null; Start-Sleep -Milliseconds 400 }
      "clip" { "schowek [{0}]: {1}" -f $arg, [Windows.Forms.Clipboard]::GetText() }
      "shot" { Shot (Join-Path (Split-Path $Out) "$arg.png") }
      "key" { $a = $arg -split "@"; Key ([Convert]::ToInt32($a[0], 16)) $false $a[1] }
      "ctrl" { $a = $arg -split "@"; Key ([Convert]::ToInt32($a[0], 16)) $true $a[1] }
      default {
        $xy = $arg -split ","; $t = Target ([int]$xy[0]) ([int]$xy[1])
        switch ($verb) {
          "move" { [CK]::PostMessage($t[0], 0x0200, [IntPtr]::Zero, $t[1]) | Out-Null }
          "down" { [CK]::PostMessage($t[0], 0x0200, [IntPtr]::Zero, $t[1]) | Out-Null; [CK]::PostMessage($t[0], 0x0201, [IntPtr]1, $t[1]) | Out-Null }
          "drag" { [CK]::PostMessage($t[0], 0x0200, [IntPtr]1, $t[1]) | Out-Null }
          "up" { [CK]::PostMessage($t[0], 0x0202, [IntPtr]::Zero, $t[1]) | Out-Null }
          "right" { [CK]::PostMessage($t[0], 0x0205, [IntPtr]::Zero, $t[1]) | Out-Null }
          default {
            [CK]::PostMessage($t[0], 0x0200, [IntPtr]::Zero, $t[1]) | Out-Null
            Start-Sleep -Milliseconds 150
            [CK]::PostMessage($t[0], 0x0201, [IntPtr]1, $t[1]) | Out-Null
            [CK]::PostMessage($t[0], 0x0202, [IntPtr]::Zero, $t[1]) | Out-Null
          }
        }
        Start-Sleep -Milliseconds 350
      }
    }
  }
  Start-Sleep -Milliseconds 500
  Shot $Out
  $p.Refresh()
  "alive={0}" -f (-not $p.HasExited)
} finally {
  if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force }
  if ($keepText -ne $null) { [Windows.Forms.Clipboard]::SetText($keepText) } elseif ($keepImage -ne $null) { [Windows.Forms.Clipboard]::SetImage($keepImage) }
}
