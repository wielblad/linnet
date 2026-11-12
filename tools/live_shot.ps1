param([string]$Out, [int]$PagesUp = 0, [long]$Thread = 0, [int]$Wait = 2)
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class WS {
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr dc, uint f);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  public struct RECT { public int L, T, R, B; }
}
"@
[WS]::SetProcessDPIAware() | Out-Null
$h = [WS]::FindWindowEx([IntPtr]::Zero, [IntPtr]::Zero, "MliteMain", [NullString]::Value)
if ($h -eq [IntPtr]::Zero) { "no window"; exit 1 }
if ($Thread -ne 0) { [WS]::PostMessage($h, 0x8004, [IntPtr]::Zero, [IntPtr]$Thread) | Out-Null; Start-Sleep -Seconds $Wait }
$view = [WS]::FindWindowEx($h, [IntPtr]::Zero, "MliteView", [NullString]::Value)
for ($i = 0; $i -lt $PagesUp; $i++) { [WS]::PostMessage($view, 0x0115, [IntPtr]2, [IntPtr]::Zero) | Out-Null; Start-Sleep -Milliseconds 150 }
Start-Sleep -Seconds $Wait
$r = New-Object WS+RECT
[WS]::GetWindowRect($h, [ref]$r) | Out-Null
$bmp = New-Object System.Drawing.Bitmap ($r.R - $r.L), ($r.B - $r.T)
$g = [System.Drawing.Graphics]::FromImage($bmp)
$dc = $g.GetHdc(); [WS]::PrintWindow($h, $dc, 2) | Out-Null; $g.ReleaseHdc($dc); $g.Dispose()
$bmp.Save($Out, [System.Drawing.Imaging.ImageFormat]::Png)
"saved"
