param([string]$Exe, [string]$DataDir, [string]$Out)
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class WM {
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr h, int c);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr dc, uint f);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  public struct RECT { public int L, T, R, B; }
}
"@
[WM]::SetProcessDPIAware() | Out-Null
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"") -PassThru
function Sample($what) {
  $c = Get-CimInstance Win32_PerfFormattedData_PerfProc_Process -Filter "IDProcess=$($p.Id)"
  "{0}: pamiec prywatna {1:N1} MB" -f $what, ($c.WorkingSetPrivate/1MB)
}
Start-Sleep -Seconds 55
Sample "polaczony, okno widoczne"
$h = [WM]::FindWindowEx([IntPtr]::Zero, [IntPtr]::Zero, "MliteMain", [NullString]::Value)
$r = New-Object WM+RECT; [WM]::GetWindowRect($h, [ref]$r) | Out-Null
$bmp = New-Object System.Drawing.Bitmap ($r.R - $r.L), ($r.B - $r.T)
$g = [System.Drawing.Graphics]::FromImage($bmp)
$dc = $g.GetHdc(); [WM]::PrintWindow($h, $dc, 2) | Out-Null; $g.ReleaseHdc($dc); $g.Dispose()
$bmp.Save($Out, [System.Drawing.Imaging.ImageFormat]::Png)
[WM]::PostMessage($h, 0x0010, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null
Start-Sleep -Seconds 12
Sample "polaczony, schowany w zasobniku"
[WM]::ShowWindow($h, 5) | Out-Null
Start-Sleep -Seconds 5
Sample "pokazany ponownie"
"pid " + $p.Id
