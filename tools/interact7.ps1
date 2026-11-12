param([string]$Exe, [string]$DataDir, [string]$OutDir, [int]$X = 480, [int]$Y = 395, [int]$Select = 3)
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class WH {
  [DllImport("user32.dll")] public static extern IntPtr SendMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern IntPtr GetDlgItem(IntPtr h, int id);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr h, IntPtr after, int x, int y, int cx, int cy, uint flags);
  public struct RECT { public int L, T, R, B; }
}
"@
[WH]::SetProcessDPIAware() | Out-Null
$env:MLITE_KEEPHOVER = "1"
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline") -PassThru
Start-Sleep -Seconds 3
$h = [WH]::FindWindowEx([IntPtr]::Zero, [IntPtr]::Zero, "MliteMain", [NullString]::Value)
if ($h -eq [IntPtr]::Zero) { "no main window"; exit 1 }
$list = [WH]::GetDlgItem($h, 311)
$view = [WH]::FindWindowEx($h, [IntPtr]::Zero, "MliteView", [NullString]::Value)
function Shot($name) {
  Start-Sleep -Milliseconds 600
  $r = New-Object WH+RECT
  [WH]::GetWindowRect($h, [ref]$r) | Out-Null
  $bmp = New-Object System.Drawing.Bitmap ($r.R - $r.L - 18), ($r.B - $r.T - 10)
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  $g.CopyFromScreen($r.L + 9, $r.T + 1, 0, 0, $bmp.Size); $g.Dispose()
  $bmp.Save((Join-Path $OutDir "$name.png"), [System.Drawing.Imaging.ImageFormat]::Png)
  "saved $name"
}
[WH]::SetWindowPos($h, [IntPtr](-1), 0, 0, 0, 0, 0x0043) | Out-Null
[WH]::SendMessage($list, 0x0186, [IntPtr]$Select, [IntPtr]::Zero) | Out-Null
[WH]::SendMessage($h, 0x0111, [IntPtr]((1 -shl 16) -bor 311), $list) | Out-Null
Start-Sleep -Seconds 1
# pointer over a message: the actions appear beside the bubble
[WH]::PostMessage($view, 0x0200, [IntPtr]::Zero, [IntPtr](($Y -shl 16) -bor $X)) | Out-Null
Shot "r1"
# pointer over the first action, then a click: the reaction bar
$ax = 265
[WH]::PostMessage($view, 0x0200, [IntPtr]::Zero, [IntPtr](($Y -shl 16) -bor $ax)) | Out-Null
Start-Sleep -Milliseconds 300
[WH]::PostMessage($view, 0x0202, [IntPtr]::Zero, [IntPtr](($Y -shl 16) -bor $ax)) | Out-Null
Shot "r2"
# wheel in the list and in the conversation: bars appear over the content
[WH]::PostMessage([WH]::FindWindowEx([IntPtr]::Zero, [IntPtr]::Zero, "MliteReact", [NullString]::Value), 0x0201, [IntPtr]::Zero, [IntPtr]((500 -shl 16) -bor 900)) | Out-Null
[WH]::PostMessage($list, 0x020A, [IntPtr](-360 -shl 16), [IntPtr]::Zero) | Out-Null
Start-Sleep -Milliseconds 500
Shot "r3"
$p.Refresh()
"alive={0}" -f (-not $p.HasExited)
Stop-Process -Id $p.Id -Force
