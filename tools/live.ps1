param([string]$Steps = "", [string]$OutDir = "")
# Drives the INSTALLED, running Linnet on the real account through its own window, the way
# a person would: used only for checks the user asked for. Steps are separated by ";":
#   open:<thread id>   shot:<name>   wait:<ms>   input:<text>   enter
#   click:x,y  move:x,y  right:x,y   (coordinates in the saved picture of the window)
#   menukey:<letter>   yes   pastefile:<path>   key:<hex vk>@<window class>
Add-Type -AssemblyName System.Windows.Forms, System.Drawing
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class LV {
  [DllImport("user32.dll")] public static extern IntPtr SendMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll", CharSet=CharSet.Unicode, EntryPoint="SendMessageW")] public static extern IntPtr SendText(IntPtr h, uint m, IntPtr w, string l);
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr dc, uint f);
  [DllImport("user32.dll")] public static extern IntPtr GetDlgItem(IntPtr h, int id);
  [DllImport("user32.dll")] public static extern bool ScreenToClient(IntPtr h, ref POINT p);
  [DllImport("user32.dll")] public static extern IntPtr ChildWindowFromPointEx(IntPtr h, POINT p, uint f);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool AttachThreadInput(uint a, uint b, bool f);
  [DllImport("user32.dll")] public static extern bool GetKeyboardState(byte[] s);
  [DllImport("user32.dll")] public static extern bool SetKeyboardState(byte[] s);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("kernel32.dll")] public static extern uint GetCurrentThreadId();
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  public struct RECT { public int L, T, R, B; }
  public struct POINT { public int X, Y; }
}
"@
[LV]::SetProcessDPIAware() | Out-Null
$exe = "$env:LOCALAPPDATA\Programs\Linnet\linnet.exe"
$p = Get-Process linnet -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq $exe } | Select-Object -First 1
if (-not $p) { "Linnet nie dziala"; exit 1 }
$h = [IntPtr]::Zero; $w = [IntPtr]::Zero; $uiThread = 0
for ($i = 0; $i -lt 300; $i++) {
  $w = [LV]::FindWindowEx([IntPtr]::Zero, $w, "MliteMain", [NullString]::Value)
  if ($w -eq [IntPtr]::Zero) { break }
  $wpid = 0; $tid = [LV]::GetWindowThreadProcessId($w, [ref]$wpid)
  if ($wpid -eq $p.Id) { $h = $w; $uiThread = $tid; break }
}
if ($h -eq [IntPtr]::Zero) { "brak okna"; exit 1 }
if (-not [LV]::IsWindowVisible($h)) { [LV]::PostMessage($h, 0x8003, [IntPtr]::Zero, [IntPtr]0x0202) | Out-Null; Start-Sleep -Seconds 1 }
$keepText = $null; $keepImage = $null; $touched = $false
function Target($x, $y) {
  $r = New-Object LV+RECT; [LV]::GetWindowRect($h, [ref]$r) | Out-Null
  $win = $h
  for ($d = 0; $d -lt 4; $d++) {
    $pt = New-Object LV+POINT; $pt.X = $r.L + $x; $pt.Y = $r.T + $y
    [LV]::ScreenToClient($win, [ref]$pt) | Out-Null
    $child = [LV]::ChildWindowFromPointEx($win, $pt, 1)
    if ($child -eq [IntPtr]::Zero -or $child -eq $win) { break }
    $win = $child
  }
  $pt = New-Object LV+POINT; $pt.X = $r.L + $x; $pt.Y = $r.T + $y
  [LV]::ScreenToClient($win, [ref]$pt) | Out-Null
  return @($win, [IntPtr](($pt.Y -shl 16) -bor ($pt.X -band 0xFFFF)))
}
function Shot($name) {
  $r = New-Object LV+RECT; [LV]::GetWindowRect($h, [ref]$r) | Out-Null
  $bmp = New-Object System.Drawing.Bitmap ($r.R - $r.L), ($r.B - $r.T)
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  $dc = $g.GetHdc(); [LV]::PrintWindow($h, $dc, 2) | Out-Null; $g.ReleaseHdc($dc); $g.Dispose()
  $bmp.Save((Join-Path $OutDir "$name.png"), [System.Drawing.Imaging.ImageFormat]::Png); $bmp.Dispose()
  "zrzut {0}: {1}x{2}" -f $name, ($r.R - $r.L), ($r.B - $r.T)
}
function Key($target, $vk, $ctrl) {
  $me = [LV]::GetCurrentThreadId()
  [LV]::AttachThreadInput($me, $uiThread, $true) | Out-Null
  $st = New-Object byte[] 256
  [LV]::GetKeyboardState($st) | Out-Null
  if ($ctrl) { $st[0x11] = 0x80 } else { $st[0x11] = 0 }
  [LV]::SetKeyboardState($st) | Out-Null
  [LV]::PostMessage($target, 0x0100, [IntPtr]$vk, [IntPtr]1) | Out-Null
  Start-Sleep -Milliseconds 600
  $st[0x11] = 0
  [LV]::SetKeyboardState($st) | Out-Null
  [LV]::AttachThreadInput($me, $uiThread, $false) | Out-Null
}
$edit = [LV]::GetDlgItem($h, 330)
try {
  foreach ($step in ($Steps -split ";" | Where-Object { $_ -ne "" })) {
    $verb = $step; $arg = ""
    if ($step -match "^(\w+):(.*)$") { $verb = $matches[1]; $arg = $matches[2] }
    switch ($verb) {
      "open" { [LV]::PostMessage($h, 0x8004, [IntPtr]::Zero, [IntPtr][long]$arg) | Out-Null; Start-Sleep -Seconds 4 }
      "wait" { Start-Sleep -Milliseconds ([int]$arg) }
      "shot" { Shot $arg }
      "input" { [LV]::SendText($edit, 0x00C2, [IntPtr]1, $arg) | Out-Null; Start-Sleep -Milliseconds 300 }
      "enter" { Key $edit 0x0D $false }
      "key" { $a = $arg -split "@"; $t = [LV]::FindWindowEx($h, [IntPtr]::Zero, $a[1], [NullString]::Value); Key $t ([Convert]::ToInt32($a[0], 16)) $false }
      "pick" {
        $pw = [IntPtr]::Zero; $hit = $false
        for ($n = 0; $n -lt 50 -and -not $hit; $n++) {
          $pw = [LV]::FindWindowEx([IntPtr]::Zero, $pw, "MliteReact", [NullString]::Value)
          if ($pw -eq [IntPtr]::Zero) { break }
          $ppid = 0; [LV]::GetWindowThreadProcessId($pw, [ref]$ppid) | Out-Null
          if ($ppid -eq $p.Id -and [LV]::IsWindowVisible($pw)) {
            $pr = New-Object LV+RECT; [LV]::GetWindowRect($pw, [ref]$pr) | Out-Null
            $cell = ($pr.R - $pr.L) / 6
            $lp = [IntPtr]((([int](($pr.B - $pr.T) / 2)) -shl 16) -bor ([int]($cell * ([int]$arg + 0.5)) -band 0xFFFF))
            [LV]::PostMessage($pw, 0x0200, [IntPtr]::Zero, $lp) | Out-Null; Start-Sleep -Milliseconds 150
            [LV]::PostMessage($pw, 0x0201, [IntPtr]1, $lp) | Out-Null; [LV]::PostMessage($pw, 0x0202, [IntPtr]::Zero, $lp) | Out-Null
            $hit = $true
          }
        }
        "pasek reakcji: {0}" -f $hit; Start-Sleep -Milliseconds 800
      }
      "menukey" { [LV]::PostMessage($h, 0x0100, [IntPtr][int][char]$arg.ToUpper(), [IntPtr]1) | Out-Null; Start-Sleep -Milliseconds 600 }
      "yes" {
        $dlg = [IntPtr]::Zero; $found = $false
        for ($n = 0; $n -lt 50 -and -not $found; $n++) {
          $dlg = [LV]::FindWindowEx([IntPtr]::Zero, $dlg, "#32770", [NullString]::Value)
          if ($dlg -eq [IntPtr]::Zero) { break }
          $dpid = 0; [LV]::GetWindowThreadProcessId($dlg, [ref]$dpid) | Out-Null
          if ($dpid -eq $p.Id) { [LV]::PostMessage($dlg, 0x0111, [IntPtr]6, [IntPtr]::Zero) | Out-Null; $found = $true }
        }
        "okno potwierdzenia: {0}" -f $found; Start-Sleep -Milliseconds 800
      }
      "pastefile" {
        if (-not $touched) {
          if ([Windows.Forms.Clipboard]::ContainsText()) { $keepText = [Windows.Forms.Clipboard]::GetText() }
          elseif ([Windows.Forms.Clipboard]::ContainsImage()) { $keepImage = [Windows.Forms.Clipboard]::GetImage() }
          $touched = $true
        }
        $list = New-Object System.Collections.Specialized.StringCollection
        $list.Add($arg) | Out-Null
        [Windows.Forms.Clipboard]::SetFileDropList($list)
        Key $edit 0x56 $true
      }
      default {
        $xy = $arg -split ","; $t = Target ([int]$xy[0]) ([int]$xy[1])
        switch ($verb) {
          "move" { [LV]::PostMessage($t[0], 0x0200, [IntPtr]::Zero, $t[1]) | Out-Null }
          "right" { [LV]::PostMessage($t[0], 0x0200, [IntPtr]::Zero, $t[1]) | Out-Null; Start-Sleep -Milliseconds 150; [LV]::PostMessage($t[0], 0x0205, [IntPtr]::Zero, $t[1]) | Out-Null }
          "click" {
            [LV]::PostMessage($t[0], 0x0200, [IntPtr]::Zero, $t[1]) | Out-Null
            Start-Sleep -Milliseconds 200
            [LV]::PostMessage($t[0], 0x0201, [IntPtr]1, $t[1]) | Out-Null
            [LV]::PostMessage($t[0], 0x0202, [IntPtr]::Zero, $t[1]) | Out-Null
          }
        }
        Start-Sleep -Milliseconds 400
      }
    }
  }
} finally {
  if ($touched) {
    if ($keepText -ne $null) { [Windows.Forms.Clipboard]::SetText($keepText) } elseif ($keepImage -ne $null) { [Windows.Forms.Clipboard]::SetImage($keepImage) } else { [Windows.Forms.Clipboard]::Clear() }
  }
}
