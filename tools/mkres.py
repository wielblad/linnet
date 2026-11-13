# Builds cmd/mlite/rsrc_windows_amd64.syso: the Windows resources linked into linnet.exe.
# Group 1 is the application icon (the first icon group is what Explorer shows for the
# file), group 2 the light tray mark (for a dark taskbar), group 3 the dark tray mark.
# Usage: python tools/mkres.py   (from the project root; no outside tools needed)
import struct, os, sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
GROUPS = [
    (1, os.path.join(ROOT, "brand", "02-windows", "win32", "Linnet.ico")),
    (2, os.path.join(ROOT, "brand", "02-windows", "tray", "Linnet-tray-light.ico")),
    (3, os.path.join(ROOT, "brand", "02-windows", "tray", "Linnet-tray-dark.ico")),
]
OUT = os.path.join(ROOT, "cmd", "mlite", "rsrc_windows_amd64.syso")
RT_ICON, RT_GROUP_ICON, LANG = 3, 14, 0x0409


def read_ico(path):
    data = open(path, "rb").read()
    reserved, kind, count = struct.unpack_from("<HHH", data, 0)
    assert reserved == 0 and kind == 1 and count > 0, path
    frames = []
    for i in range(count):
        w, h, colors, res, planes, bits, size, offset = struct.unpack_from("<BBBBHHII", data, 6 + 16 * i)
        frames.append((w, h, colors, res, planes, bits, data[offset:offset + size]))
    return frames


icons = {}   # icon id -> image bytes
groups = {}  # group id -> GRPICONDIR bytes
next_id = 1
for gid, path in GROUPS:
    frames = read_ico(path)
    blob = struct.pack("<HHH", 0, 1, len(frames))
    for w, h, colors, res, planes, bits, image in frames:
        blob += struct.pack("<BBBBHHIH", w, h, colors, res, planes, bits, len(image), next_id)
        icons[next_id] = image
        next_id += 1
    groups[gid] = blob

# resource tree: type -> id -> language -> data
tree = [(RT_ICON, sorted(icons.items())), (RT_GROUP_ICON, sorted(groups.items()))]
n_res = len(icons) + len(groups)
size_dirs = 16 + 8 * len(tree)                     # root
size_dirs += sum(16 + 8 * len(items) for _, items in tree)  # one directory per type
size_dirs += n_res * (16 + 8)                      # one language directory per resource
entries_at = size_dirs                             # data entries follow the directories
data_at = entries_at + 16 * n_res

def directory(n_ids):
    return struct.pack("<IIHHHH", 0, 0, 0, 0, 0, n_ids)

root = directory(len(tree))
type_dirs = b""
lang_dirs = b""
entries = b""
blobs = b""
relocs = []
type_dir_at = 16 + 8 * len(tree)
lang_dir_at = type_dir_at + sum(16 + 8 * len(items) for _, items in tree)
entry_i = 0
for rtype, items in tree:
    root += struct.pack("<II", rtype, 0x80000000 | (type_dir_at + len(type_dirs)))
    block = directory(len(items))
    for rid, payload in items:
        block += struct.pack("<II", rid, 0x80000000 | (lang_dir_at + len(lang_dirs)))
        entry_off = entries_at + 16 * entry_i
        lang_dirs += directory(1) + struct.pack("<II", LANG, entry_off)
        while len(blobs) % 8:
            blobs += b"\0"
        relocs.append(entry_off)  # the data address is relative to the image: the linker fixes it
        entries += struct.pack("<IIII", data_at + len(blobs), len(payload), 0, 0)
        blobs += payload
        entry_i += 1
    type_dirs += block
section = root + type_dirs + lang_dirs + entries + blobs
assert len(root + type_dirs + lang_dirs) == size_dirs
while len(section) % 8:
    section += b"\0"

# COFF object: one .rsrc section, its relocations, one section symbol
header_size = 20 + 40
reloc_at = header_size + len(section)
symtab_at = reloc_at + 10 * len(relocs)
out = struct.pack("<HHIIIHH", 0x8664, 1, 0, symtab_at, 1, 0, 0)
out += struct.pack("<8sIIIIIIHHI", b".rsrc", 0, 0, len(section), header_size, reloc_at, 0, len(relocs), 0, 0x40000040)
out += section
for off in relocs:
    out += struct.pack("<IIH", off, 0, 3)          # IMAGE_REL_AMD64_ADDR32NB against symbol 0
out += struct.pack("<8sIhHBB", b".rsrc", 0, 1, 0, 3, 0)  # static symbol for the section
out += struct.pack("<I", 4)                        # empty string table
open(OUT, "wb").write(out)
print("zapisano %s: %d ikon w %d grupach, %d bajtow" % (os.path.relpath(OUT, ROOT), len(icons), len(groups), len(out)))
