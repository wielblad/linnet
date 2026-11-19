# Someone is writing: opens a conversation from a head, tells the program the way the
# connection does that someone writes in it and in a conversation whose head is closed,
# takes a picture of the heads and the conversation, waits until the news has grown old
# and takes another. Says for each moment whether the conversation is taller by the dots.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File tools\typing_test.ps1 -Exe build\linnet.exe -DataDir .local\data-ui-test -OutDir .local\shots [-Mode cols|one] [-Open 102] [-Other 101]
param([string]$Exe, [string]$DataDir, [string]$OutDir, [string]$Mode = "cols", [long]$Open = 102, [long]$Other = 101, [string]$Theme = "")
$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Windows.Forms, System.Drawing
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class TT {
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  public struct RECT { public int L, T, R, B; }
}
"@
[TT]::SetProcessDPIAware() | Out-Null
$settings = '{"toasts":false,"body":true,"background":true,"show_mem":true,"dnd":false,"head_preview":false,"sound":false,"theme":"' + $Theme + '","head_mode":"' + $Mode + '","heads":[{"id":' + $Open + ',"x":1500,"y":300},{"id":' + $Other + ',"x":1500,"y":380}]}'
[IO.File]::WriteAllText((Join-Path $DataDir "settings.json"), $settings)
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline") -PassThru
try {
  Start-Sleep -Seconds 4
  function All($cls) {
    $out = @(); $w = [IntPtr]::Zero
    for ($i = 0; $i -lt 300; $i++) {
      $w = [TT]::FindWindowEx([IntPtr]::Zero, $w, $cls, [NullString]::Value)
      if ($w -eq [IntPtr]::Zero) { break }
      $wpid = 0; [TT]::GetWindowThreadProcessId($w, [ref]$wpid) | Out-Null
      if ($wpid -eq $p.Id) { $out += $w }
    }
    return $out
  }
  function Rect($h) { $r = New-Object TT+RECT; [TT]::GetWindowRect($h, [ref]$r) | Out-Null; return $r }
  function Shot($name) {
    $l = 100000; $t = 100000; $r = -100000; $b = -100000
    foreach ($c in @("MliteHead", "MlitePane", "MliteMain")) {
      foreach ($w in (All $c)) {
        if (-not [TT]::IsWindowVisible($w)) { continue }
        $q = Rect $w
        if ($q.L -lt $l) { $l = $q.L }; if ($q.T -lt $t) { $t = $q.T }
        if ($q.R -gt $r) { $r = $q.R }; if ($q.B -gt $b) { $b = $q.B }
      }
    }
    $bmp = New-Object System.Drawing.Bitmap ($r - $l), ($b - $t)
    $g = [System.Drawing.Graphics]::FromImage($bmp); $g.CopyFromScreen($l, $t, 0, 0, $bmp.Size)
    $g.Dispose(); $bmp.Save((Join-Path $OutDir "$name.png"), [System.Drawing.Imaging.ImageFormat]::Png); $bmp.Dispose()
    "zrzut $name.png"
  }
  function Typing($id, $on) { [TT]::PostMessage($main, 0x800E, [IntPtr]$on, [IntPtr]$id) | Out-Null }   # wmAppTyping
  $main = (All "MliteMain")[0]
  $multi = ($Mode -eq "cols" -or $Mode -eq "dock")
  if ($multi) { [TT]::PostMessage($main, 0x0010, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null; Start-Sleep -Milliseconds 1000 }
  $head = (All "MliteHead" | Sort-Object { (Rect $_).T })[0]
  [TT]::PostMessage($head, 0x0201, [IntPtr]1, [IntPtr]0x00140014) | Out-Null
  [TT]::PostMessage($head, 0x0202, [IntPtr]::Zero, [IntPtr]0x00140014) | Out-Null
  Start-Sleep -Milliseconds 1800
  Shot "pisze-$Mode-0-przed"
  Typing $Open 1; Typing $Other 1
  Start-Sleep -Milliseconds 900
  Shot "pisze-$Mode-1-pisze"
  Start-Sleep -Milliseconds 400
  Shot "pisze-$Mode-2-pisze-dalej"
  Typing $Other 0                      # that one stopped; the open one is left to grow old
  Start-Sleep -Milliseconds 800
  Shot "pisze-$Mode-3-jedna-przestala"
  Start-Sleep -Seconds 9
  Shot "pisze-$Mode-4-po-czasie"
  $p.Refresh(); "program dziala: {0}" -f (-not $p.HasExited)
} finally {
  if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force }
  Remove-Item (Join-Path $DataDir "settings.json") -ErrorAction SilentlyContinue
}
