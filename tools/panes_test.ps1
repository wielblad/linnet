# Opens three conversations from heads with "Kolumny" or "Dok" and takes pictures of the
# panel and the panes beside it: all three open, one written in, one folded or closed, and
# the panel put away.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File tools\panes_test.ps1 -Exe build\linnet.exe -DataDir .local\data-ui-test -OutDir .local\shots -Mode cols
param([string]$Exe, [string]$DataDir, [string]$OutDir, [string]$Mode = "cols", [string]$Theme = "", [switch]$Full)
$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Windows.Forms, System.Drawing
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class PT {
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  public struct RECT { public int L, T, R, B; }
}
"@
[PT]::SetProcessDPIAware() | Out-Null
$settings = '{"toasts":false,"body":true,"background":true,"show_mem":true,"dnd":false,"head_preview":false,"sound":false,"theme":"' + $Theme + '","head_mode":"' + $Mode + '","heads":[{"id":102,"x":1500,"y":260},{"id":101,"x":1500,"y":340},{"id":106,"x":1500,"y":420}]}'
[IO.File]::WriteAllText((Join-Path $DataDir "settings.json"), $settings)
$env:MLITE_KEEPHOVER = "1"   # the pointer is not really there: what it was moved over stays marked
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline") -PassThru
try {
  Start-Sleep -Seconds 4
  function All($cls) {
    $out = @(); $w = [IntPtr]::Zero
    for ($i = 0; $i -lt 300; $i++) {
      $w = [PT]::FindWindowEx([IntPtr]::Zero, $w, $cls, [NullString]::Value)
      if ($w -eq [IntPtr]::Zero) { break }
      $wpid = 0; [PT]::GetWindowThreadProcessId($w, [ref]$wpid) | Out-Null
      if ($wpid -eq $p.Id) { $out += $w }
    }
    return $out
  }
  function Rect($h) { $r = New-Object PT+RECT; [PT]::GetWindowRect($h, [ref]$r) | Out-Null; return $r }
  $main = (All "MliteMain")[0]
  $heads = All "MliteHead" | Sort-Object { (Rect $_).T }
  "glowek: {0}" -f $heads.Count
  function Click($h, $x, $y) {
    $lp = [IntPtr](($y -shl 16) -bor ($x -band 0xFFFF))
    [PT]::PostMessage($h, 0x0201, [IntPtr]1, $lp) | Out-Null
    [PT]::PostMessage($h, 0x0202, [IntPtr]::Zero, $lp) | Out-Null
    Start-Sleep -Milliseconds 1500
  }
  function Frame($name) {
    $wins = @()
    if ([PT]::IsWindowVisible($main)) { $wins += $main }
    $wins += @(All "MlitePane" | Where-Object { [PT]::IsWindowVisible($_) })
    $l = 100000; $t = 100000; $r = -100000; $b = -100000
    foreach ($w in ($wins + $heads)) {
      $q = Rect $w
      if ($q.L -lt $l) { $l = $q.L }; if ($q.T -lt $t) { $t = $q.T }
      if ($q.R -gt $r) { $r = $q.R }; if ($q.B -gt $b) { $b = $q.B }
    }
    $pad = 12
    $bmp = New-Object System.Drawing.Bitmap (($r - $l) + 2*$pad), (($b - $t) + 2*$pad)
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $g.CopyFromScreen(($l - $pad), ($t - $pad), 0, 0, $bmp.Size)
    $g.Dispose(); $bmp.Save((Join-Path $OutDir "$name.png"), [System.Drawing.Imaging.ImageFormat]::Png); $bmp.Dispose()
    "  {0}: okien {1}" -f $name, $wins.Count
    foreach ($w in $wins) { $q = Rect $w; "    ({0},{1}) {2}x{3}" -f $q.L, $q.T, ($q.R - $q.L), ($q.B - $q.T) }
  }
  Click $heads[0] 20 20; Frame "$Mode-1"
  Click $heads[1] 20 20; Click $heads[2] 20 20; Frame "$Mode-3"
  $panes = @(All "MlitePane" | Sort-Object { (Rect $_).L })
  "paneli: {0}" -f $panes.Count
  if ($panes.Count -gt 0) {
    $pane = $panes[0]
    $edit = [PT]::FindWindowEx($pane, [IntPtr]::Zero, "Edit", [NullString]::Value)
    foreach ($ch in "Test z drugiej rozmowy".ToCharArray()) { [PT]::PostMessage($edit, 0x0102, [IntPtr][int]$ch, [IntPtr]::Zero) | Out-Null }
    Start-Sleep -Milliseconds 400
    [PT]::PostMessage($edit, 0x0100, [IntPtr]0x0D, [IntPtr]1) | Out-Null   # Enter
    Start-Sleep -Milliseconds 1500
    Frame "$Mode-wyslane"
    $q = Rect $pane
    if ($Mode -eq "dock") {   # fold the pane with its "–" button
      Click $pane (($q.R - $q.L) - 68) 26; Frame "$Mode-zwiniety"
    }
  }
  if ($Full -and $panes.Count -gt 1) {
    # What a pane can do with a message, the same as the panel: the actions beside it, a
    # reply, the reactions, the large picture, the menu and the question before deleting.
    # The places are those of the test conversations at 100% scale.
    function Kid($parent, $cls) { [PT]::FindWindowEx($parent, [IntPtr]::Zero, $cls, [NullString]::Value) }
    function Hover($h, $x, $y) { [PT]::PostMessage($h, 0x0200, [IntPtr]::Zero, [IntPtr](($y -shl 16) -bor ($x -band 0xFFFF))) | Out-Null; Start-Sleep -Milliseconds 500 }
    function Key($h, $vk) { [PT]::PostMessage($h, 0x0100, [IntPtr]$vk, [IntPtr]1) | Out-Null; Start-Sleep -Milliseconds 700 }
    $anna = $panes[1]; $tomek = $panes[0]
    $av = Kid $anna "MliteView"; $ae = Kid $anna "Edit"
    Hover $av 60 357; Frame "$Mode-pelne-akcje"
    Hover $av 289 357; Click $av 289 357; Frame "$Mode-pelne-odpowiedz"
    foreach ($ch in "Odpowiedz z dodatkowego okna".ToCharArray()) { [PT]::PostMessage($ae, 0x0102, [IntPtr][int]$ch, [IntPtr]::Zero) | Out-Null }
    Start-Sleep -Milliseconds 500; Frame "$Mode-pelne-odpowiedz-tekst"
    [PT]::PostMessage($ae, 0x000C, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null   # the field is emptied
    Key $ae 0x1B                      # Esc in the field: the reply is given up
    Frame "$Mode-pelne-po-esc"
    Hover $av 60 357; Hover $av 255 357; Click $av 255 357; Frame "$Mode-pelne-reakcje"
    Key $ae 0x1B
    Hover $av 250 428; Hover $av 21 428; Click $av 21 428; Frame "$Mode-pelne-menu"
    Key $anna 0x26; Key $anna 0x0D    # Up (the last item), Enter: "Usuń u wszystkich" asks first
    Frame "$Mode-pelne-pytanie"
    $q = Kid $anna "MliteDialog"
    "  pytanie w oknie rozmowy: {0}" -f ($q -ne [IntPtr]::Zero)
    if ($q -ne [IntPtr]::Zero) { Key $q 0x1B }
    $tv = Kid $tomek "MliteView"
    Click $tv 150 300; Frame "$Mode-pelne-zdjecie"
    $big = Kid $tomek "MliteViewer"
    "  duze zdjecie w oknie rozmowy: {0}" -f ($big -ne [IntPtr]::Zero)
    if ($big -ne [IntPtr]::Zero) { Key $big 0x1B }
    Frame "$Mode-pelne-koniec"
    # and the panel itself still does the same, with its own field
    $mview = Kid $main "MliteView"
    Hover $mview 60 220; Hover $mview 249 220; Click $mview 249 220; Frame "$Mode-pelne-panel-odpowiedz"
    Key (Kid $main "Edit") 0x1B
    $p.Refresh(); "program dziala: {0}" -f (-not $p.HasExited)
  }
  Click $heads[0] 20 20; Frame "$Mode-bez-panelu"   # the head of the panel puts the panel away
  Click $heads[2] 20 20; Frame "$Mode-zamknieta"    # the head of a pane closes its pane
  $p.Refresh()
  "program dziala: {0}" -f (-not $p.HasExited)
} finally {
  if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force }
  Remove-Item (Join-Path $DataDir "settings.json") -ErrorAction SilentlyContinue
}
