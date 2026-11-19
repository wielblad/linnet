# Issue 8: two heads tied in a stack, both conversations open, one closed with its cross.
# Says where every head is before and after: shown or not, on top or not, its place and
# how far down from the top of the screen it lies, and takes a picture of both moments.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File tools\issue8_test.ps1 -Exe build\linnet.exe -DataDir .local\data-ui-test -OutDir .local\shots [-Mode cols] [-MainUp] [-HeadX 1500]
param([string]$Exe, [string]$DataDir, [string]$OutDir, [string]$Mode = "cols", [switch]$MainUp, [int]$HeadX = 1500, [int]$HeadSize = 48, [int]$Close = 0, [switch]$Real, [switch]$MainBack)
$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Windows.Forms, System.Drawing
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class I8 {
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] public static extern IntPtr GetTopWindow(IntPtr h);
  [DllImport("user32.dll")] public static extern IntPtr GetWindow(IntPtr h, uint cmd);
  [DllImport("user32.dll", EntryPoint="GetWindowLongPtrW")] public static extern IntPtr GetWindowLongPtr(IntPtr h, int i);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr h, IntPtr after, int x, int y, int w, int ht, uint flags);
  [DllImport("user32.dll")] public static extern bool SetCursorPos(int x, int y);
  [DllImport("user32.dll")] public static extern bool GetCursorPos(out POINT p);
  [DllImport("user32.dll")] public static extern void mouse_event(uint f, uint dx, uint dy, uint d, UIntPtr e);
  [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
  public struct POINT { public int X, Y; }
  public struct RECT { public int L, T, R, B; }
}
"@
[I8]::SetProcessDPIAware() | Out-Null
$settings = '{"toasts":false,"body":true,"background":true,"show_mem":true,"dnd":false,"head_preview":false,"sound":false,"head_mode":"' + $Mode + '","head_size":' + $HeadSize +
  ',"groups":[{"id":1,"dir":"v"}],"heads":[{"id":102,"x":' + $HeadX + ',"y":260,"group":1},{"id":101,"x":' + $HeadX + ',"y":314,"group":1}]}'
[IO.File]::WriteAllText((Join-Path $DataDir "settings.json"), $settings)
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline") -PassThru
try {
  Start-Sleep -Seconds 4
  function All($cls) {
    $out = @(); $w = [IntPtr]::Zero
    for ($i = 0; $i -lt 300; $i++) {
      $w = [I8]::FindWindowEx([IntPtr]::Zero, $w, $cls, [NullString]::Value)
      if ($w -eq [IntPtr]::Zero) { break }
      $wpid = 0; [I8]::GetWindowThreadProcessId($w, [ref]$wpid) | Out-Null
      if ($wpid -eq $p.Id) { $out += $w }
    }
    return $out
  }
  function Rect($h) { $r = New-Object I8+RECT; [I8]::GetWindowRect($h, [ref]$r) | Out-Null; return $r }
  function Depth($h) { $w = [I8]::GetTopWindow([IntPtr]::Zero); $n = 0
    while ($w -ne [IntPtr]::Zero -and $w -ne $h -and $n -lt 5000) { $w = [I8]::GetWindow($w, 2); $n++ }; return $n }
  function Click($h, $x, $y) {
    if ($Real) {   # a click of the real mouse: it changes which window is the active one, which a posted message does not
      $q = Rect $h; $was = New-Object I8+POINT; [I8]::GetCursorPos([ref]$was) | Out-Null
      [I8]::SetCursorPos($q.L + $x, $q.T + $y) | Out-Null; Start-Sleep -Milliseconds 120
      [I8]::mouse_event(2, 0, 0, 0, [UIntPtr]::Zero); Start-Sleep -Milliseconds 60; [I8]::mouse_event(4, 0, 0, 0, [UIntPtr]::Zero)
      Start-Sleep -Milliseconds 250; [I8]::SetCursorPos($was.X, $was.Y) | Out-Null
      Start-Sleep -Milliseconds 1200; return
    }
    $lp = [IntPtr](($y -shl 16) -bor ($x -band 0xFFFF))
    [I8]::PostMessage($h, 0x0201, [IntPtr]1, $lp) | Out-Null
    [I8]::PostMessage($h, 0x0202, [IntPtr]::Zero, $lp) | Out-Null
    Start-Sleep -Milliseconds 1500
  }
  function State($what) {
    $fg = [I8]::GetForegroundWindow(); $fpid = 0; [I8]::GetWindowThreadProcessId($fg, [ref]$fpid) | Out-Null
    "--- $what (aktywne okno nalezy do programu: {0})" -f ($fpid -eq $p.Id)
    foreach ($c in "MliteHead", "MlitePane", "MliteMain") {
      foreach ($w in (All $c | Sort-Object { (Rect $_).T })) {
        $q = Rect $w
        "  {0,-5} ({1},{2}) {3}x{4}  widoczne: {5,-5}  na wierzchu: {6,-5}  glebokosc: {7}" -f $c.Substring(5), $q.L, $q.T, ($q.R - $q.L), ($q.B - $q.T),
          [I8]::IsWindowVisible($w), ((([I8]::GetWindowLongPtr($w, -20).ToInt64()) -band 8) -ne 0), (Depth $w)
      }
    }
  }
  function Shot($file) {
    $l = $HeadX - 900; if ($l -lt 0) { $l = 0 }
    $bmp = New-Object System.Drawing.Bitmap 1100, 700
    $g = [System.Drawing.Graphics]::FromImage($bmp); $g.CopyFromScreen($l, 120, 0, 0, $bmp.Size)
    $g.Dispose(); $bmp.Save((Join-Path $OutDir "$file.png"), [System.Drawing.Imaging.ImageFormat]::Png); $bmp.Dispose()
  }
  $main = (All "MliteMain")[0]
  if ($MainBack) {   # the window of the program is open, but behind the windows of other programs
    [I8]::SetWindowPos($main, [IntPtr]1, 0, 0, 0, 0, 0x0013) | Out-Null; Start-Sleep -Milliseconds 2500
  }
  if (-not $MainUp) { [I8]::PostMessage($main, 0x0010, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null; Start-Sleep -Milliseconds 1200 }   # the window goes to the tray
  $heads = All "MliteHead" | Sort-Object { (Rect $_).T }
  foreach ($h in $heads) { Click $h 20 20 }
  State "obie rozmowy otwarte"; Shot "zgl8-przed"
  $panes = @(All "MlitePane" | Where-Object { [I8]::IsWindowVisible($_) } | Sort-Object { (Rect $_).L })
  if ($panes.Count -ge 2) {
    $victim = $panes[$Close]; $q = Rect $victim
    Click $victim (($q.R - $q.L) - 28) 28          # its cross
    State "po zamknieciu jednej krzyzykiem"; Shot "zgl8-po"
    Start-Sleep -Milliseconds 2500
    State "trzy sekundy pozniej"; Shot "zgl8-po-3s"
  } else { "nie udalo sie otworzyc dwoch rozmow: {0}" -f $panes.Count }
  $p.Refresh(); "program dziala: {0}" -f (-not $p.HasExited)
} finally {
  if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force }
  Remove-Item (Join-Path $DataDir "settings.json") -ErrorAction SilentlyContinue
}
