param([string]$Exe, [string]$DataDir, [string]$Label = "")
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class W5 {
  [DllImport("user32.dll")] public static extern IntPtr SendMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern IntPtr GetDlgItem(IntPtr h, int id);
  [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr h, int c);
}
"@
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline") -PassThru
function Sample($what) {
  $p.Refresh()
  $c = Get-CimInstance Win32_PerfFormattedData_PerfProc_Process -Filter "IDProcess=$($p.Id)"
  "{0} | {1}: PrivateWS={2:N1}MB WS={3:N1}MB Commit={4:N1}MB" -f $Label, $what, ($c.WorkingSetPrivate/1MB), ($p.WorkingSet64/1MB), ($p.PrivateMemorySize64/1MB)
}
Start-Sleep -Seconds 3
$p.Refresh(); $h = $p.MainWindowHandle
$lb = [W5]::GetDlgItem($h, 311)
foreach ($i in 1, 3, 0, 1) {
  [W5]::SendMessage($lb, 0x0186, [IntPtr]$i, [IntPtr]::Zero) | Out-Null
  [W5]::SendMessage($h, 0x0111, [IntPtr]((1 -shl 16) -bor 311), $lb) | Out-Null
  Start-Sleep -Milliseconds 700
}
Start-Sleep -Seconds 4
Sample "okno widoczne"
[W5]::PostMessage($h, 0x0010, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null
Start-Sleep -Seconds 6
Sample "schowane w zasobniku"
[W5]::ShowWindow($h, 5) | Out-Null
Start-Sleep -Seconds 4
Sample "pokazane ponownie"
Stop-Process -Id $p.Id -Force
