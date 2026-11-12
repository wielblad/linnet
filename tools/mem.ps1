param([string]$Exe, [string]$DataDir, [string]$Extra = "", [int]$Wait = 12, [int]$Select = -1)
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class W3 {
  [DllImport("user32.dll")] public static extern IntPtr SendMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern IntPtr GetDlgItem(IntPtr h, int id);
}
"@
$argList = @("-data-dir", "`"$DataDir`"")
if ($Extra -ne "") { $argList += $Extra.Split(" ") }
$p = Start-Process -FilePath $Exe -ArgumentList $argList -PassThru
function Sample($label) {
  $p.Refresh()
  $c = Get-CimInstance Win32_PerfFormattedData_PerfProc_Process -Filter "IDProcess=$($p.Id)"
  "{0}: WS={1:N1}MB PrivateWS={2:N1}MB Commit={3:N1}MB" -f $label, ($p.WorkingSet64/1MB), ($c.WorkingSetPrivate/1MB), ($p.PrivateMemorySize64/1MB)
}
Start-Sleep -Seconds 3
Sample "po 3 s"
if ($Select -ge 0) {
  $p.Refresh(); $h = $p.MainWindowHandle
  $lb = [W3]::GetDlgItem($h, 311)
  [W3]::SendMessage($lb, 0x0186, [IntPtr]$Select, [IntPtr]::Zero) | Out-Null
  [W3]::SendMessage($h, 0x0111, [IntPtr]((1 -shl 16) -bor 311), $lb) | Out-Null
  Start-Sleep -Seconds 2
  Sample "po otwarciu rozmowy"
}
Start-Sleep -Seconds $Wait
Sample "po $Wait s"
Stop-Process -Id $p.Id -Force
