# The ordinary window of the program comes before the heads and the conversations beside
# them: opens a conversation from a head that stands inside the window and checks that both
# are behind the window, then puts the window away and checks that both are on top again.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File tools\zorder_test.ps1 -Exe build\linnet.exe -DataDir .local\data-ui-test -OutDir .local\shots
param([string]$Exe, [string]$DataDir, [string]$OutDir)
$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Windows.Forms, System.Drawing
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class ZT {
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr h, IntPtr after, int x, int y, int w, int ht, uint flags);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] public static extern IntPtr GetTopWindow(IntPtr h);
  [DllImport("user32.dll")] public static extern IntPtr GetWindow(IntPtr h, uint cmd);
  [DllImport("user32.dll", EntryPoint="GetWindowLongPtrW")] public static extern IntPtr GetWindowLongPtr(IntPtr h, int i);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  public struct RECT { public int L, T, R, B; }
}
"@
[ZT]::SetProcessDPIAware() | Out-Null
$settings = '{"toasts":false,"body":true,"background":true,"show_mem":true,"dnd":false,"head_preview":false,"sound":false,"head_mode":"cols","heads":[{"id":106,"x":400,"y":300}]}'
[IO.File]::WriteAllText((Join-Path $DataDir "settings.json"), $settings)
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline") -PassThru
try {
  Start-Sleep -Seconds 4
  function One($cls) {
    $w = [IntPtr]::Zero
    for ($i = 0; $i -lt 300; $i++) {
      $w = [ZT]::FindWindowEx([IntPtr]::Zero, $w, $cls, [NullString]::Value)
      if ($w -eq [IntPtr]::Zero) { break }
      $wpid = 0; [ZT]::GetWindowThreadProcessId($w, [ref]$wpid) | Out-Null
      if ($wpid -eq $p.Id) { return $w }
    }
    return [IntPtr]::Zero
  }
  function Depth($h) {   # how far down from the top of the screen a window is
    $w = [ZT]::GetTopWindow([IntPtr]::Zero); $n = 0
    while ($w -ne [IntPtr]::Zero -and $w -ne $h -and $n -lt 5000) { $w = [ZT]::GetWindow($w, 2); $n++ }
    return $n
  }
  function Topmost($h) { return (([ZT]::GetWindowLongPtr($h, -20).ToInt64() -band 8) -ne 0) }
  function Say($what) {
    $m = Depth $main; $hd = Depth $head; $pn = Depth $pane
    "{0}: okno programu na {1}, glowka na {2} (zawsze na wierzchu: {3}), rozmowa na {4} (zawsze na wierzchu: {5})" -f $what, $m, $hd, (Topmost $head), $pn, (Topmost $pane)
    "   glowka pod oknem programu: {0}, rozmowa pod oknem programu: {1}" -f ($hd -gt $m), ($pn -gt $m)
  }
  $main = One "MliteMain"; $head = One "MliteHead"
  [ZT]::SetWindowPos($main, [IntPtr]::Zero, 0, 0, 0, 0, 0x0013) | Out-Null   # to the top of the ordinary windows, without taking the keyboard
  [ZT]::PostMessage($head, 0x0201, [IntPtr]1, [IntPtr]0x00140014) | Out-Null
  [ZT]::PostMessage($head, 0x0202, [IntPtr]::Zero, [IntPtr]0x00140014) | Out-Null
  Start-Sleep -Milliseconds 1500
  $pane = One "MlitePane"
  "okno rozmowy otwarte: {0}" -f ($pane -ne [IntPtr]::Zero)
  [ZT]::SetWindowPos($main, [IntPtr]::Zero, 0, 0, 0, 0, 0x0013) | Out-Null
  Start-Sleep -Milliseconds 2500     # the program looks again every second
  Say "okno programu widoczne"
  $q = New-Object ZT+RECT; [ZT]::GetWindowRect($main, [ref]$q) | Out-Null
  $bmp = New-Object System.Drawing.Bitmap ($q.R - $q.L), ($q.B - $q.T)
  $g = [System.Drawing.Graphics]::FromImage($bmp); $g.CopyFromScreen($q.L, $q.T, 0, 0, $bmp.Size)
  $g.Dispose(); $bmp.Save((Join-Path $OutDir "kolejnosc-okno-programu.png"), [System.Drawing.Imaging.ImageFormat]::Png); $bmp.Dispose()
  [ZT]::PostMessage($main, 0x0010, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null   # the window is closed: it goes to the tray
  Start-Sleep -Milliseconds 2500
  "okno programu schowane: {0}" -f (-not [ZT]::IsWindowVisible($main))
  Say "po schowaniu okna programu"
  $p.Refresh(); "program dziala: {0}" -f (-not $p.HasExited)
} finally {
  if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force }
  Remove-Item (Join-Path $DataDir "settings.json") -ErrorAction SilentlyContinue
}
