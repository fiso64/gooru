from pathlib import Path

p = Path("internal/serve/browse.go")
text = p.read_text()
old = '''func mediaTypeForPath(path string) string {
\tif typ := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); typ != "" {
\t\treturn typ
\t}
\treturn "application/octet-stream"
}'''
new = '''func mediaTypeForPath(path string) string {
\textension := strings.ToLower(filepath.Ext(path))
\tif extension == ".cbz" {
\t\treturn "application/vnd.comicbook+zip"
\t}
\tif typ := mime.TypeByExtension(extension); typ != "" {
\t\treturn typ
\t}
\treturn "application/octet-stream"
}'''
if old not in text:
    raise SystemExit("missing mediaTypeForPath")
p.write_text(text.replace(old, new, 1))

p = Path("internal/serve/comic_kind_test.go")
text = p.read_text()
text += '''
func TestMediaTypeForComicBookZipDoesNotDependOnSystemMimeDatabase(t *testing.T) {
\tif got := mediaTypeForPath("/library/book.CBZ"); got != "application/vnd.comicbook+zip" {
\t\tt.Fatalf("mediaTypeForPath(CBZ) = %q", got)
\t}
}
'''
p.write_text(text)
