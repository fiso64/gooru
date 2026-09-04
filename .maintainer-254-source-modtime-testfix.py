from pathlib import Path

p = Path('internal/serve/upload_encryption_e2e_test.go')
text = p.read_text()
old = '''\tif !file.ModTime.Equal(sourceModTime) {
\t\tt.Fatalf("registered modtime=%v want source=%v", file.ModTime, sourceModTime)
\t}'''
new = '''\tif file.ModTime != sourceModTime.Unix() {
\t\tt.Fatalf("registered modtime=%d want source=%d", file.ModTime, sourceModTime.Unix())
\t}'''
if text.count(old) != 1:
    raise SystemExit('encrypted upload modtime assertion pattern mismatch')
p.write_text(text.replace(old, new))
