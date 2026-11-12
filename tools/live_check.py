# Reads only counts and states from the live data folder: no names, no message text.
import json, os, sqlite3, sys, collections

d = sys.argv[1]
c = sqlite3.connect("file:" + os.path.join(d, "messages.db").replace("\\", "/") + "?mode=ro", uri=True)
one = lambda q: c.execute(q).fetchone()[0]
tables = [r[0] for r in c.execute("select name from sqlite_master where type='table'")]
print("rozmowy:", one("select count(*) from threads"), "| wiadomosci:", one("select count(*) from messages"))
print("rozmowy nieprzeczytane:", one("select count(*) from threads where updated_at_ms > last_read_ms"))
if "pictures" in tables:
    print("adresy zdjec:", one("select count(*) from pictures"))
    print("pseudonimy:", one("select count(*) from nicknames"),
          "| w tym jako nazwa rozmowy 1:1:", one("select count(*) from nicknames where thread_id = contact_id"))
    print("znaczniki odczytu:", one("select count(*) from participant_reads where read_ms > 0"),
          "| dostarczenia:", one("select count(*) from participant_reads where delivered_ms > 0"))
print("wiadomosci ze zdjeciem do pobrania:", one("select count(*) from messages where media_kind='image' and media_url != ''"))
for sub in ("avatars", "media"):
    p = os.path.join(d, sub)
    print("pliki w", sub + ":", len(os.listdir(p)) if os.path.isdir(p) else 0)

log = os.path.join(d, "mlite.log")
levels, msgs = collections.Counter(), collections.Counter()
if os.path.exists(log):
    for line in open(log, encoding="utf-8", errors="replace"):
        try:
            e = json.loads(line)
        except Exception:
            continue
        levels[e.get("level", "?")] += 1
        msgs[(e.get("level", "?"), str(e.get("message", ""))[:60])] += 1
print("dziennik wg poziomu:", dict(levels))
for (lvl, m), n in msgs.most_common(8):
    print("  %4d x [%s] %s" % (n, lvl, m))
