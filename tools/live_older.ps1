# Checks, on the real account, that scrolling to the top of a conversation fetches older
# messages from Messenger. The program is started with -no-read, so nothing is marked as
# read; afterwards it is restarted the normal way. Only counts are printed.
param([string]$Kind = "encrypted")
Add-Type @"
using System; using System.Runtime.InteropServices;
public class LO {
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
}
"@
$exe = "$env:LOCALAPPDATA\Programs\Linnet\linnet.exe"
$py = @'
import sqlite3, os, sys
p = os.path.join(os.environ["APPDATA"], "Linnet", "messages.db")
c = sqlite3.connect("file:" + p.replace("\\", "/") + "?mode=ro", uri=True, timeout=10)
if len(sys.argv) > 1 and sys.argv[1] == "pick":
    # an encrypted one-to-one conversation with a middling number of stored messages
    if len(sys.argv) > 2 and sys.argv[2] == "group":
        row = c.execute("select t.id, count(m.id) n from threads t join messages m on m.thread_id = t.id where t.thread_type != 15 group by t.id order by n desc limit 1").fetchone()
    else:
        row = c.execute("select t.id, count(m.id) n from threads t join messages m on m.thread_id = t.id where t.thread_type = 15 group by t.id order by abs(n - 150) limit 1").fetchone()
    print(row[0])
else:
    tid = int(sys.argv[1])
    n, oldest = c.execute("select count(*), min(timestamp_ms) from messages where thread_id = ?", (tid,)).fetchone()
    import datetime
    print("%d wiadomosci, najstarsza z %s" % (n, datetime.datetime.fromtimestamp(oldest / 1000).strftime("%Y-%m-%d")))
'@
$script = Join-Path $env:TEMP "linnet_older_check.py"
[IO.File]::WriteAllText($script, $py)
function QuitApp { $h = [LO]::FindWindowEx([IntPtr]::Zero, [IntPtr]::Zero, "MliteMain", [NullString]::Value); if ($h -ne [IntPtr]::Zero) { [LO]::PostMessage($h, 0x8009, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null; Start-Sleep -Seconds 3 } }
try {
  QuitApp
  $tid = (python -I $script pick $Kind).Trim()
  "przed: " + (python -I $script $tid)
  $env:MLITE_TIMING = "1"
  $p = Start-Process -FilePath $exe -ArgumentList @("-no-read") -PassThru
  $env:MLITE_TIMING = $null
  Start-Sleep -Seconds 35
  $h = [LO]::FindWindowEx([IntPtr]::Zero, [IntPtr]::Zero, "MliteMain", [NullString]::Value)
  [LO]::PostMessage($h, 0x8004, [IntPtr]::Zero, [IntPtr][long]$tid) | Out-Null
  Start-Sleep -Seconds 6
  "po otwarciu: " + (python -I $script $tid)
  $view = [LO]::FindWindowEx($h, [IntPtr]::Zero, "MliteView", [NullString]::Value)
  foreach ($round in 1..6) {
    [LO]::PostMessage($view, 0x0100, [IntPtr]0x24, [IntPtr]1) | Out-Null   # Home: to the top of what is loaded
    Start-Sleep -Seconds 5
    "po przewinieciu {0}: {1}" -f $round, (python -I $script $tid)
  }
  $m = Get-CimInstance Win32_PerfFormattedData_PerfProc_Process -Filter "IDProcess=$($p.Id)"
  "pamiec prywatna: {0:N1} MB" -f ($m.WorkingSetPrivate/1MB)
  $log = Get-Content "$env:APPDATA\Linnet\linnet.log" -Tail 400
  "bledy pobierania starszych: {0}" -f @($log | Select-String "fetch older messages").Count
  $t = $log | Select-String '"message":"timing"' | ForEach-Object { if ($_.Line -match '"messages":(\d+),"layout":([0-9.]+)') { "{0} wiadomosci: {1:N0} ms" -f $matches[1], [double]$matches[2] } }
  "czasy ukladu: " + (($t | Select-Object -Unique | Select-Object -Last 6) -join "; ")
} finally {
  QuitApp
  Start-Process -FilePath $exe
  Remove-Item $script -ErrorAction SilentlyContinue
  "uruchomiono ponownie w zwyklym trybie"
}
