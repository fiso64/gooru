package serve

import "testing"

func TestMediaKindForComicBookZip(t *testing.T) {
	for _, mediaType := range []string{
		"application/vnd.comicbook+zip",
		"application/vnd.comicbook+zip; charset=binary",
	} {
		if got := mediaKindForType(mediaType); got != "comic" {
			t.Fatalf("mediaKindForType(%q) = %q, want comic", mediaType, got)
		}
	}
}

func TestMediaTypeForComicBookZipDoesNotDependOnSystemMimeDatabase(t *testing.T) {
	if got := mediaTypeForPath("/library/book.CBZ"); got != "application/vnd.comicbook+zip" {
		t.Fatalf("mediaTypeForPath(CBZ) = %q", got)
	}
}
