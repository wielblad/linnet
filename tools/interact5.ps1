param([string]$Exe, [string]$DataDir, [string]$OutDir, [string]$Python = "python")
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class W8 {
  [DllImport("user32.dll")] public static extern IntPtr SendMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern IntPtr GetDlgItem(IntPtr h, int id);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr h, IntPtr after, int x, int y, int cx, int cy, uint flags);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  public struct RECT { public int L, T, R, B; }
}
"@
[W8]::SetProcessDPIAware() | Out-Null
$settings = '{"toasts":true,"body":true,"background":true,"show_mem":true,"dnd":false,"head_preview":true,"heads":[{"id":102,"x":820,"y":250},{"id":104,"x":820,"y":330},{"id":103,"x":820,"y":410}]}'
[IO.File]::WriteAllText((Join-Path $DataDir "settings.json"), $settings)
$env:MLITE_ICONDUMP = (Join-Path $OutDir "icons")
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline") -PassThru
Start-Sleep -Seconds 3
$p.Refresh()
$h = [IntPtr]::Zero
$scan = [IntPtr]::Zero
for ($i = 0; $i -lt 200; $i++) {
  $scan = [W8]::FindWindowEx([IntPtr]::Zero, $scan, "MliteMain", [NullString]::Value)
  if ($scan -eq [IntPtr]::Zero) { break }
  $spid = 0; [W8]::GetWindowThreadProcessId($scan, [ref]$spid) | Out-Null
  if ($spid -eq $p.Id) { $h = $scan; break }
}
if ($h -eq [IntPtr]::Zero) { "no main window"; Stop-Process -Id $p.Id -Force; exit 1 }
function Raise() {
  [W8]::SetWindowPos($h, [IntPtr](-1), 0, 0, 0, 0, 0x0043) | Out-Null
  $found = @()
  foreach ($cls in "MliteHead", "MlitePeek") {
    $w = [IntPtr]::Zero
    for ($i = 0; $i -lt 200; $i++) {
      $w = [W8]::FindWindowEx([IntPtr]::Zero, $w, $cls, [NullString]::Value)
      if ($w -eq [IntPtr]::Zero) { break }
      $wpid = 0; [W8]::GetWindowThreadProcessId($w, [ref]$wpid) | Out-Null
      if ($wpid -eq $p.Id) { $found += $w }
    }
  }
  foreach ($w in $found) { [W8]::SetWindowPos($w, [IntPtr](-1), 0, 0, 0, 0, 0x0013) | Out-Null }
}
function Shot($name) {
  Raise
  Start-Sleep -Milliseconds 700
  $r = New-Object W8+RECT
  [W8]::GetWindowRect($h, [ref]$r) | Out-Null
  $bmp = New-Object System.Drawing.Bitmap ($r.R - $r.L - 18), ($r.B - $r.T - 10)
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  $g.CopyFromScreen($r.L + 9, $r.T + 1, 0, 0, $bmp.Size)
  $g.Dispose()
  $bmp.Save((Join-Path $OutDir "$name.png"), [System.Drawing.Imaging.ImageFormat]::Png)
  "saved $name"
}
$list = [W8]::GetDlgItem($h, 311)
[W8]::SendMessage($list, 0x0186, [IntPtr]1, [IntPtr]::Zero) | Out-Null
[W8]::SendMessage($h, 0x0111, [IntPtr]((1 -shl 16) -bor 311), $list) | Out-Null
Start-Sleep -Milliseconds 800
Shot "k1"
# a new message arrives in a pinned conversation
& $Python -I (Join-Path $OutDir "newmsg.py") $DataDir | Out-Null
[W8]::PostMessage($h, 0x8001, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null
Start-Sleep -Milliseconds 900
Shot "k2"
$p.Refresh()
"alive={0} PrivateWS~ WS={1:N1}MB" -f (-not $p.HasExited), ($p.WorkingSet64/1MB)
Stop-Process -Id $p.Id -Force
Remove-Item (Join-Path $DataDir "settings.json") -ErrorAction SilentlyContinue
