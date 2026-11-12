param([string]$Exe, [string]$DataDir, [string]$Out, [long]$Thread, [int]$Connect = 14, [int]$Wait = 25, [string]$Extra = "-no-read")
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class WL {
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr dc, uint f);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  public struct RECT { public int L, T, R, B; }
}
"@
[WL]::SetProcessDPIAware() | Out-Null
$argList = @("-data-dir", "`"$DataDir`"")
if ($Extra -ne "") { $argList += $Extra.Split(" ") }
$p = Start-Process -FilePath $Exe -ArgumentList $argList -PassThru
Start-Sleep -Seconds $Connect
$h = [IntPtr]::Zero; $scan = [IntPtr]::Zero
for ($i = 0; $i -lt 300; $i++) {
  $scan = [WL]::FindWindowEx([IntPtr]::Zero, $scan, "MliteMain", [NullString]::Value)
  if ($scan -eq [IntPtr]::Zero) { break }
  $spid = 0; [WL]::GetWindowThreadProcessId($scan, [ref]$spid) | Out-Null
  if ($spid -eq $p.Id) { $h = $scan; break }
}
if ($h -eq [IntPtr]::Zero) { "no main window"; exit 1 }
if ($Thread -ne 0) {
  [WL]::PostMessage($h, 0x8004, [IntPtr]::Zero, [IntPtr]$Thread) | Out-Null
  Start-Sleep -Seconds $Wait
}
$r = New-Object WL+RECT
[WL]::GetWindowRect($h, [ref]$r) | Out-Null
$bmp = New-Object System.Drawing.Bitmap ($r.R - $r.L), ($r.B - $r.T)
$g = [System.Drawing.Graphics]::FromImage($bmp)
$dc = $g.GetHdc(); [WL]::PrintWindow($h, $dc, 2) | Out-Null; $g.ReleaseHdc($dc); $g.Dispose()
$bmp.Save($Out, [System.Drawing.Imaging.ImageFormat]::Png)
$c = Get-CimInstance Win32_PerfFormattedData_PerfProc_Process -Filter "IDProcess=$($p.Id)"
"pid {0}, pamiec prywatna {1:N1} MB" -f $p.Id, ($c.WorkingSetPrivate/1MB)
