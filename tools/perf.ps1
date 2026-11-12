param([string]$Exe, [string]$DataDir, [string]$Label = "")
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class W6 {
  [DllImport("user32.dll")] public static extern IntPtr SendMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern IntPtr GetDlgItem(IntPtr h, int id);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern bool RedrawWindow(IntPtr h, IntPtr r, IntPtr rgn, uint f);
}
"@
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline") -PassThru
Start-Sleep -Seconds 3
$p.Refresh(); $h = $p.MainWindowHandle
$lb = [W6]::GetDlgItem($h, 311)
[W6]::SendMessage($lb, 0x0186, [IntPtr]1, [IntPtr]::Zero) | Out-Null
[W6]::SendMessage($h, 0x0111, [IntPtr]((1 -shl 16) -bor 311), $lb) | Out-Null
Start-Sleep -Seconds 1
$view = [W6]::FindWindowEx($h, [IntPtr]::Zero, "MliteView", [NullString]::Value); "view handle: $view"
$n = 60
$sw = [Diagnostics.Stopwatch]::StartNew()
for ($i = 0; $i -lt $n; $i++) { [W6]::RedrawWindow($view, [IntPtr]::Zero, [IntPtr]::Zero, 0x0101) | Out-Null }
$a = $sw.Elapsed.TotalMilliseconds / $n
$sw.Restart()
for ($i = 0; $i -lt $n; $i++) { [W6]::RedrawWindow($lb, [IntPtr]::Zero, [IntPtr]::Zero, 0x0101) | Out-Null }
$b = $sw.Elapsed.TotalMilliseconds / $n
"{0}: rozmowa {1:N1} ms na odrysowanie, lista {2:N1} ms" -f $Label, $a, $b
Stop-Process -Id $p.Id -Force
