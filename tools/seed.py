# Rebuilds the fake conversations used for screenshots. Usage: python -I seed.py <data-dir>
import sqlite3, sys, time, os

db = os.path.join(sys.argv[1], "messages.db")
c = sqlite3.connect(db)
for t in ("messages", "threads", "contacts", "reactions", "participant_reads", "hybrid_threads"):
    c.execute("delete from " + t)

now = int(time.time() * 1000)
MIN, HOUR, DAY = 60_000, 3_600_000, 86_400_000
ME = 999
people = {
    101: "Anna Testowa", 102: "Marek Przykładowy", 104: "Zażółć Gęślą-Jaźń",
    105: "Kasia Nowak", 106: "Tomek Zieliński", 108: "Ola Wiśniewska",
    109: "Piotr Lewandowski", 110: "Magda Kamińska", ME: "Ja Testowy",
}
for i, n in people.items():
    c.execute("insert into contacts(id,name) values(?,?)", (i, n))

n = [0]
def msg(thread, sender, ago, text="", **kw):
    n[0] += 1
    mid = kw.pop("id", "m%d" % n[0])
    cols = dict(id=mid, thread_id=thread, sender_id=sender, text=text, timestamp_ms=now - ago)
    cols.update(kw)
    c.execute("insert into messages(%s) values(%s)" % (",".join(cols), ",".join("?" * len(cols))), list(cols.values()))
    return mid

def thread(tid, title, ttype, preview, ago, read=True, mute=0):
    c.execute("insert into threads(id,title,last_preview,thread_type,updated_at_ms,last_read_ms,mute_until_ms) values(?,?,?,?,?,?,?)",
              (tid, title, preview, ttype, now - ago, now if read else now - ago - 30 * MIN, mute))

# 101: plain one-to-one, with read marks, a reply, a reaction and a photo
msg(101, 101, 26 * HOUR, "Hej, wracasz jutro z Krakowa?")
msg(101, ME, 26 * HOUR - 2 * MIN, "Tak, pociąg mam o 16:40.")
msg(101, 101, 3 * HOUR + 10 * MIN, "Cześć! Jak tam weekend?")
a = msg(101, ME, 3 * HOUR + 8 * MIN, "Hej, całkiem dobrze. Byłem w górach.")
b = msg(101, ME, 3 * HOUR + 7 * MIN, "Pogoda dopisała, widać było Tatry.")
c.execute("insert into reactions(message_id,actor_id,emoji) values(?,?,?)", (b, 101, "❤"))
msg(101, 101, 2 * HOUR, "Super! Zażółć gęślą jaźń — test polskich znaków: ąćęłńóśźż ĄĆĘŁŃÓŚŹŻ")
msg(101, 101, 2 * HOUR - MIN, "To jest dłuższa wiadomość, która powinna się zawinąć do kolejnej linii w oknie rozmowy, żeby sprawdzić zawijanie tekstu w widoku wiadomości bez poziomego paska przewijania.")
msg(101, 101, 2 * HOUR - 2 * MIN, "", id="img-landscape", media_kind="image", media_name="tatry.jpg",
    media_mime="image/jpeg", media_url="https://pictures.invalid/m/landscape.jpg")
msg(101, ME, 2 * HOUR - 3 * MIN, "A tak wyglądał szlak pod koniec dnia.", id="img-portrait", media_kind="image",
    media_name="szlak.jpg", media_mime="image/jpeg", media_url="https://pictures.invalid/m/portrait.jpg")
msg(101, 101, 2 * HOUR - 4 * MIN, "", id="img-missing", media_kind="image", media_name="brak.jpg",
    media_mime="image/jpeg", media_url="https://pictures.invalid/m/missing.jpg")
msg(101, ME, 40 * MIN, "Widzimy się jutro?", reply_text="Hej, wracasz jutro z Krakowa?", reply_sender_id=101)
msg(101, 101, 12 * MIN, "Do zobaczenia jutro o 18.")
msg(101, ME, 11 * MIN, "Będę. Wezmę zdjęcia z wyjazdu.", edited=1)
c.execute("insert into participant_reads(thread_id,contact_id,read_ms) values(?,?,?)", (101, 101, now - 30 * MIN))
thread(101, "Anna Testowa", 1, "Będę. Wezmę zdjęcia z wyjazdu.", 11 * MIN)

# 102: encrypted one-to-one with two unread messages
msg(102, ME, 5 * HOUR, "Masz chwilę po południu?")
msg(102, 102, 50 * MIN, "Mam, dzwoń po 15.")
msg(102, 102, 49 * MIN, "Albo napisz, jak wolisz.")
thread(102, "Marek Przykładowy", 15, "Albo napisz, jak wolisz.", 49 * MIN, read=False)

# 103: group with several senders, a reply, an unsent message and reactions
msg(103, 105, 4 * HOUR, "Kto bierze namiot?")
g1 = msg(103, 106, 4 * HOUR - 2 * MIN, "Ja mogę wziąć dwuosobowy.")
msg(103, 106, 4 * HOUR - 3 * MIN, "Ale śpiwory każdy swój.")
msg(103, ME, 3 * HOUR + 30 * MIN, "Biorę kuchenkę i gaz.")
g2 = msg(103, 105, 3 * HOUR + 20 * MIN, "Świetnie, to mamy komplet.", reply_text="Biorę kuchenkę i gaz.", reply_sender_id=ME)
for actor, e in ((106, "👍"), (ME, "👍"), (108, "❤")):
    c.execute("insert into reactions(message_id,actor_id,emoji) values(?,?,?)", (g2, actor, e))
msg(103, 108, 95 * MIN, "", unsent=1)
msg(103, 108, 94 * MIN, "Wyjazd w piątek o 7:00 spod dworca.")
thread(103, "Wyjazd w góry", 2, "Ola: Wyjazd w piątek o 7:00 spod dworca.", 94 * MIN)

# 104: muted, with a message being sent and one that failed
msg(104, 104, 6 * HOUR, "łódź, źdźbło, pięść")
msg(104, ME, 5 * HOUR, "Wysłane wcześniej.")
msg(104, ME, 3 * MIN, "Ta wiadomość się nie wysłała.", error="timeout")
msg(104, ME, 1 * MIN, "A ta jest w drodze.", pending=1)
thread(104, "Zażółć Gęślą-Jaźń", 1, "A ta jest w drodze.", 1 * MIN, mute=-1)

# older conversations to fill the list
for tid, title, ttype, prev, ago in (
    (105, "Kasia Nowak", 15, "Dzięki, do usłyszenia!", 27 * HOUR),
    (106, "Tomek Zieliński", 15, "Wyślę Ci to wieczorem.", 2 * DAY + 3 * HOUR),
    (107, "Rodzina", 2, "Mama: Obiad w niedzielę o 14.", 3 * DAY),
    (108, "Ola Wiśniewska", 15, "Ty: Jasne, pasuje mi.", 4 * DAY + 2 * HOUR),
    (109, "Piotr Lewandowski", 1, "[image]", 9 * DAY),
    (110, "Magda Kamińska", 15, "[file]", 20 * DAY),
):
    msg(tid, tid if ttype != 2 else 105, ago, prev)
    thread(tid, title, ttype, prev, ago)

# ---- stage 2: pictures, nicknames, delivery marks
c.executescript("""
CREATE TABLE IF NOT EXISTS pictures (id INTEGER PRIMARY KEY, url TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS nicknames (thread_id INTEGER NOT NULL, contact_id INTEGER NOT NULL,
  nickname TEXT NOT NULL DEFAULT '', PRIMARY KEY (thread_id, contact_id));
""")
try:
    c.execute("ALTER TABLE participant_reads ADD COLUMN delivered_ms INTEGER NOT NULL DEFAULT 0")
except sqlite3.OperationalError:
    pass
c.execute("delete from pictures")
c.execute("delete from nicknames")

def fnv32(data):
    h = 2166136261
    for b in data:
        h = ((h ^ b) * 16777619) & 0xFFFFFFFF
    return h

def portrait(seed):
    from PIL import Image, ImageDraw
    import colorsys
    S = 240
    hue = (seed * 0.137) % 1.0
    top = tuple(int(v * 255) for v in colorsys.hsv_to_rgb(hue, 0.35, 0.95))
    bottom = tuple(int(v * 255) for v in colorsys.hsv_to_rgb((hue + 0.08) % 1, 0.55, 0.70))
    img = Image.new("RGB", (S, S))
    d = ImageDraw.Draw(img)
    for y in range(S):
        t = y / (S - 1)
        d.line([(0, y), (S, y)], fill=tuple(int(top[i] * (1 - t) + bottom[i] * t) for i in range(3)))
    skin = [(236, 200, 170), (214, 170, 135), (180, 130, 100), (245, 215, 190)][seed % 4]
    hair = [(60, 40, 30), (25, 25, 30), (150, 100, 50), (200, 170, 110), (110, 60, 40)][seed % 5]
    shirt = tuple(int(v * 255) for v in colorsys.hsv_to_rgb((hue + 0.5) % 1, 0.5, 0.55))
    d.ellipse([40, 170, 200, 330], fill=shirt)
    d.rectangle([104, 140, 136, 185], fill=skin)
    d.ellipse([70, 50, 170, 160], fill=skin)
    d.pieslice([64, 40, 176, 150], 180, 360, fill=hair)
    if seed % 2:
        d.rectangle([64, 92, 78, 150], fill=hair)
        d.rectangle([162, 92, 176, 150], fill=hair)
    return img

def landscape():
    from PIL import Image, ImageDraw
    S = 240
    img = Image.new("RGB", (S, S))
    d = ImageDraw.Draw(img)
    for y in range(S):
        t = y / (S - 1)
        d.line([(0, y), (S, y)], fill=(int(120 + 100 * t), int(170 + 50 * t), int(230 - 20 * t)))
    d.ellipse([160, 30, 200, 70], fill=(255, 225, 140))
    d.polygon([(0, 240), (70, 90), (140, 240)], fill=(90, 110, 130))
    d.polygon([(60, 240), (150, 60), (240, 240)], fill=(70, 90, 110))
    d.polygon([(132, 96), (150, 60), (168, 96), (150, 88)], fill=(240, 244, 248))
    d.rectangle([0, 205, 240, 240], fill=(70, 120, 80))
    return img

avdir = os.path.join(sys.argv[1], "avatars")
os.makedirs(avdir, exist_ok=True)
for f in os.listdir(avdir):
    os.remove(os.path.join(avdir, f))
for pid in (101, 102, 103, 105, 106, 108, 110):
    path = "/demo/%d.jpg" % pid
    c.execute("insert into pictures(id,url) values(?,?)", (pid, "https://pictures.invalid" + path))
    img = landscape() if pid == 103 else portrait(pid)
    img.save(os.path.join(avdir, "%d-%08x.img" % (pid, fnv32(path.encode()))), "JPEG", quality=88)

# pictures inside conversations: two present in the cache, one missing on purpose
def fnv64(data):
    h = 14695981039346656037
    for b in data:
        h = ((h ^ b) * 1099511628211) & 0xFFFFFFFFFFFFFFFF
    return h

def scene(w, h, seed):
    from PIL import Image, ImageDraw
    img = Image.new("RGB", (w, h))
    d = ImageDraw.Draw(img)
    for y in range(h):
        t = y / (h - 1)
        d.line([(0, y), (w, y)], fill=(int(250 - 120 * t), int(190 - 60 * t + 20 * seed), int(140 + 90 * t)))
    d.ellipse([w * 0.62, h * 0.12, w * 0.62 + w * 0.14, h * 0.12 + w * 0.14], fill=(255, 240, 200))
    d.polygon([(0, h), (w * 0.3, h * 0.42), (w * 0.62, h)], fill=(70, 80, 110))
    d.polygon([(w * 0.25, h), (w * 0.68, h * 0.34), (w, h * 0.9), (w, h)], fill=(50, 60, 90))
    d.polygon([(w * 0.62, h * 0.43), (w * 0.68, h * 0.34), (w * 0.75, h * 0.45), (w * 0.68, h * 0.42)], fill=(240, 244, 250))
    d.rectangle([0, h * 0.88, w, h], fill=(40, 70, 60))
    return img

mdir = os.path.join(sys.argv[1], "media")
os.makedirs(mdir, exist_ok=True)
for f in os.listdir(mdir):
    os.remove(os.path.join(mdir, f))
# attachments and links: thread 106 gets one of each kind
c.execute("delete from messages where thread_id=106")
msg(106, 106, 50 * MIN, "Zobacz to: https://www.youtube.com/watch?v=dQw4w9WgXcQ")
msg(106, 106, 48 * MIN, "", id="att-video", media_kind="video", media_mime="video/mp4", media_name="wideo.mp4",
    media_url="https://pictures.invalid/m/clip.mp4", media_duration_ms=83000)
msg(106, ME, 46 * MIN, "", id="att-voice", media_kind="audio", media_mime="audio/mp4", media_name="audio",
    media_url="https://pictures.invalid/m/voice.m4a", media_duration_ms=12000)
msg(106, 106, 44 * MIN, "Tu masz regulamin wyjazdu.", id="att-file", media_kind="file", media_mime="application/pdf",
    media_name="regulamin-wyjazdu.pdf", media_url="https://pictures.invalid/m/doc.pdf")
msg(106, ME, 42 * MIN, "A to z Facebooka https://www.facebook.com/reel/1234567890 :D", id="reel",
    link_url="https://www.facebook.com/reel/1234567890", link_title="Zachód słońca nad Morskim Okiem",
    link_image="https://pictures.invalid/p/reel.jpg")
msg(106, 106, 41 * MIN, "Publiczne wideo z FB: https://www.facebook.com/facebook/videos/10153231379946729/", id="fbvid")
msg(106, 106, 40 * MIN, "Mapa trasy: https://mapy.example.org/trasa/tatry-2026 <3", id="mapa",
    link_url="https://mapy.example.org/trasa/tatry-2026", link_title="Trasa: Dolina Pięciu Stawów – Zawrat",
    link_image="https://pictures.invalid/p/mapa.jpg")
c.execute("update threads set last_preview=?, updated_at_ms=?, last_read_ms=? where id=106",
          ("Mapa trasy: https://mapy.example.org/trasa/tatry-2026 <3", now - 40 * MIN, now))
scene(320, 180, 2).save(os.path.join(mdir, "%016x.jpg" % fnv64(b"yt-dQw4w9WgXcQ")), "JPEG", quality=88)
scene(360, 640, 1).save(os.path.join(mdir, "%016x.jpg" % fnv64(b"lnk-reel")), "JPEG", quality=88)
scene(600, 315, 0).save(os.path.join(mdir, "%016x.jpg" % fnv64(b"lnk-mapa")), "JPEG", quality=88)
msg(106, 106, 38 * MIN, "", id="img-sticker", media_kind="image", media_mime="image/webp", media_name="sticker",
    media_url="https://pictures.invalid/m/sticker.webp")
scene(360, 360, 3).save(os.path.join(mdir, "%016x.webp" % fnv64(b"img-sticker")), "WEBP", quality=85)
scene(1200, 800, 0).save(os.path.join(mdir, "%016x.jpg" % fnv64(b"img-landscape")), "JPEG", quality=88)
scene(800, 1200, 1).save(os.path.join(mdir, "%016x.jpg" % fnv64(b"img-portrait")), "JPEG", quality=88)

c.execute("insert into nicknames(thread_id,contact_id,nickname) values(102,102,'Maro')")
c.execute("insert into nicknames(thread_id,contact_id,nickname) values(103,106,'Tomek Namiot')")

def receipt(thread, contact, read_ago=None, delivered_ago=None):
    c.execute("""insert into participant_reads(thread_id,contact_id,read_ms,delivered_ms) values(?,?,?,?)
                 on conflict(thread_id,contact_id) do update set read_ms=excluded.read_ms, delivered_ms=excluded.delivered_ms""",
              (thread, contact, now - read_ago if read_ago else 0, now - delivered_ago if delivered_ago else 0))

receipt(101, 101, read_ago=30 * MIN, delivered_ago=5 * MIN)   # last own message delivered, the one before read
receipt(104, 104, delivered_ago=4 * HOUR)                      # older own message delivered, not read
receipt(103, 105, read_ago=90 * MIN)                           # has seen everything
receipt(103, 106, read_ago=3 * HOUR + 25 * MIN)                # stopped at my message
receipt(102, 102, read_ago=4 * HOUR)

# 120: a long conversation, to try scrolling back through the history
c.execute("insert into contacts(id,name) values(?,?)", (120, "Historia Długa"))
for i in range(1, 701):
    extra = " To jest trochę dłuższa wiadomość, żeby zawijała się w dymku na dwie linijki albo więcej." if i % 7 == 0 else ""
    msg(120, 120 if i % 3 else ME, 4 * DAY + (700 - i) * 7 * MIN, "Wiadomość %d.%s" % (i, extra))
thread(120, "Historia Długa", 1, "Wiadomość 700.", 4 * DAY)

msg(109, 109, 9 * DAY, "", media_kind="image", media_mime="image/jpeg", media_name="image", media_url="https://pictures.invalid/m/p109.jpg")
msg(110, ME, 20 * DAY - MIN, "", media_kind="file", media_mime="application/pdf", media_name="faktura-2026-11.pdf", media_url="https://pictures.invalid/m/f110.pdf")

c.commit()
print("threads:", c.execute("select count(*) from threads").fetchone()[0], "messages:", c.execute("select count(*) from messages").fetchone()[0])
