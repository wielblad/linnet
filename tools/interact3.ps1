param([string]$Exe, [string]$DataDir, [string]$OutDir, [int]$X = 373, [int]$Y = 518)
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class W7 {
  [DllImport("user32.dll")] public static extern IntPtr SendMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr dc, uint f);
  [DllImport("user32.dll")] public static extern IntPtr GetDlgItem(IntPtr h, int id);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr h, IntPtr after, int x, int y, int cx, int cy, uint flags);
  public struct RECT { public int L, T, R, B; }
}
"@
[W7]::SetProcessDPIAware() | Out-Null
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline") -PassThru
Start-Sleep -Seconds 3
$p.Refresh()
$h = $p.MainWindowHandle
if ($h -eq [IntPtr]::Zero) { "no main window"; exit 1 }
$list = [W7]::GetDlgItem($h, 311)
$view = [W7]::FindWindowEx($h, [IntPtr]::Zero, "MliteView", [NullString]::Value)
function Shot($name, [switch]$Screen) {
  Start-Sleep -Milliseconds 600
  $r = New-Object W7+RECT
  [W7]::GetWindowRect($h, [ref]$r) | Out-Null
  $bmp = New-Object System.Drawing.Bitmap ($r.R - $r.L), ($r.B - $r.T)
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  if ($Screen) { $g.CopyFromScreen($r.L, $r.T, 0, 0, $bmp.Size) }
  else { $dc = $g.GetHdc(); [W7]::PrintWindow($h, $dc, 2) | Out-Null; $g.ReleaseHdc($dc) }
  $g.Dispose()
  $bmp.Save((Join-Path $OutDir "$name.png"), [System.Drawing.Imaging.ImageFormat]::Png)
  "saved $name"
}
[W7]::SetWindowPos($h, [IntPtr](-1), 0, 0, 0, 0, 0x0043) | Out-Null
[W7]::SendMessage($list, 0x0186, [IntPtr]1, [IntPtr]::Zero) | Out-Null
[W7]::SendMessage($h, 0x0111, [IntPtr]((1 -shl 16) -bor 311), $list) | Out-Null
Start-Sleep -Seconds 1
$lp = [IntPtr](($Y -shl 16) -bor $X)
# 1. menu of an own message
[W7]::PostMessage($view, 0x0205, [IntPtr]::Zero, $lp) | Out-Null
Shot "m1" -Screen
[W7]::PostMessage($h, 0x001F, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null
Start-Sleep -Milliseconds 400
# 2. reply
[W7]::SendMessage($h, 0x0111, [IntPtr]400, [IntPtr]::Zero) | Out-Null
Shot "m2"
# 3. edit
[W7]::SendMessage($h, 0x0111, [IntPtr]402, [IntPtr]::Zero) | Out-Null
Shot "m3"
# 4. react without a connection: a notice, and the edit bar stays
[W7]::SendMessage($h, 0x0111, [IntPtr]410, [IntPtr]::Zero) | Out-Null
Shot "m4"
$p.Refresh()
"alive={0} WS={1:N1}MB" -f (-not $p.HasExited), ($p.WorkingSet64/1MB)
Stop-Process -Id $p.Id -Force
