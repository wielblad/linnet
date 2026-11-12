param([string]$Exe, [string]$DataDir, [string]$OutDir)
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class WC {
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr dc, uint f);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  public struct RECT { public int L, T, R, B; }
}
"@
[WC]::SetProcessDPIAware() | Out-Null
$settings = '{"toasts":true,"body":true,"background":true,"show_mem":true,"dnd":false,"head_preview":true,"heads":[{"id":102,"x":1500,"y":300}]}'
[IO.File]::WriteAllText((Join-Path $DataDir "settings.json"), $settings)
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline") -PassThru
Start-Sleep -Seconds 3
function Find($cls) {
  $w = [IntPtr]::Zero
  for ($i = 0; $i -lt 300; $i++) {
    $w = [WC]::FindWindowEx([IntPtr]::Zero, $w, $cls, [NullString]::Value)
    if ($w -eq [IntPtr]::Zero) { return [IntPtr]::Zero }
    $wpid = 0; [WC]::GetWindowThreadProcessId($w, [ref]$wpid) | Out-Null
    if ($wpid -eq $p.Id) { return $w }
  }
  return [IntPtr]::Zero
}
$h = Find "MliteMain"; $head = Find "MliteHead"
if ($h -eq [IntPtr]::Zero -or $head -eq [IntPtr]::Zero) { "windows not found"; Stop-Process -Id $p.Id -Force; exit 1 }
function Rect($w) { $r = New-Object WC+RECT; [WC]::GetWindowRect($w, [ref]$r) | Out-Null; "{0}x{1} at {2},{3}" -f ($r.R - $r.L), ($r.B - $r.T), $r.L, $r.T }
"przed: okno " + (Rect $h) + " | glowka " + (Rect $head)
$lp = [IntPtr]((30 -shl 16) -bor 30)
[WC]::PostMessage($head, 0x0201, [IntPtr]1, $lp) | Out-Null
[WC]::PostMessage($head, 0x0202, [IntPtr]::Zero, $lp) | Out-Null
Start-Sleep -Seconds 2
"po kliknieciu glowki: okno " + (Rect $h) + ", widoczne=" + [WC]::IsWindowVisible($h)
$r = New-Object WC+RECT; [WC]::GetWindowRect($h, [ref]$r) | Out-Null
$bmp = New-Object System.Drawing.Bitmap ($r.R - $r.L), ($r.B - $r.T)
$g = [System.Drawing.Graphics]::FromImage($bmp)
$dc = $g.GetHdc(); [WC]::PrintWindow($h, $dc, 2) | Out-Null; $g.ReleaseHdc($dc); $g.Dispose()
$bmp.Save((Join-Path $OutDir "c1.png"), [System.Drawing.Imaging.ImageFormat]::Png)
[WC]::PostMessage($head, 0x0201, [IntPtr]1, $lp) | Out-Null
[WC]::PostMessage($head, 0x0202, [IntPtr]::Zero, $lp) | Out-Null
Start-Sleep -Seconds 1
"po drugim kliknieciu: widoczne=" + [WC]::IsWindowVisible($h)
Stop-Process -Id $p.Id -Force
Remove-Item (Join-Path $DataDir "settings.json") -ErrorAction SilentlyContinue
