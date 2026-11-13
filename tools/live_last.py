# Prints the state of the newest messages of one conversation of the real account, without
# their text: who sent them (me / other), when, and the flags that tell whether a send worked.
# Usage: python -I tools/live_last.py <thread id> [count]
import sqlite3, os, sys, datetime

tid = int(sys.argv[1])
n = int(sys.argv[2]) if len(sys.argv) > 2 else 6
p = os.path.join(os.environ["APPDATA"], "Linnet", "messages.db")
c = sqlite3.connect("file:" + p.replace("\\", "/") + "?mode=ro", uri=True, timeout=10)
me = c.execute("select sender_id from messages where thread_id = ? and sender_id != ? order by timestamp_ms desc limit 1", (tid, tid)).fetchone()
me = me[0] if me else 0
rows = c.execute("""select id, sender_id, timestamp_ms, pending, error, length(text), text like '%[TEST Linnet]%', media_kind, edited, unsent,
    reply_to_id != '', (select group_concat(emoji, '') from reactions r where r.message_id = m.id)
    from messages m where thread_id = ? order by timestamp_ms desc, id desc limit ?""", (tid, n)).fetchall()
for (mid, sender, ts, pending, error, tlen, is_test, kind, edited, unsent, reply, rx) in reversed(rows):
    when = datetime.datetime.fromtimestamp(ts / 1000).strftime("%H:%M:%S")
    who = "ja  " if sender == me else "inny"
    flags = []
    if is_test: flags.append("TEST")
    if pending: flags.append("w toku")
    if error: flags.append("blad: " + error[:60])
    if kind: flags.append("zalacznik " + kind)
    if edited: flags.append("edytowana")
    if unsent: flags.append("cofnieta")
    if reply: flags.append("odpowiedz")
    if rx: flags.append("reakcja " + rx)
    local = "lokalny" if not mid.startswith("mid.") and len(mid) < 24 else "z serwera"
    print("%s %s tekst %3d zn. id %-9s %s" % (when, who, tlen, local, ", ".join(flags)))
