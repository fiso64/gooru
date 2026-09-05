from pathlib import Path

path = Path('internal/serve/browse.go')
text = path.read_text()
old = '\t"path/filepath"\n\t"strings"\n'
new = '\t"path/filepath"\n\t"strconv"\n\t"strings"\n'
if old not in text:
    raise SystemExit('browse import anchor missing')
path.write_text(text.replace(old, new, 1))
