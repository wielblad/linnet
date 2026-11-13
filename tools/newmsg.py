# Adds one incoming message to the test conversation 102, as if it had just arrived.
import sqlite3, sys, os, time

c = sqlite3.connect(os.path.join(sys.argv[1], "messages.db"), timeout=5)
now = int(time.time() * 1000)
text = "Jestem już pod dworcem, zobacz trasę https://mapy.example.org/trasa/dworzec \U0001F44B"
if len(sys.argv) > 2 and sys.argv[2] == "plain":
    text = "Jestem już pod dworcem, czekam przy wejściu głównym \U0001F44B"
c.execute("insert into messages(id,thread_id,sender_id,text,timestamp_ms) values(?,?,?,?,?)", ("live-%d" % now, 102, 102, text, now))
c.execute("update threads set last_preview=?, updated_at_ms=? where id=102", (text, now))
c.commit()
