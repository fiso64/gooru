package serve

import "testing"

func TestThumbnailSupportDeclaresProtectedAccessForEverySupportedKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		path   string
		kind   string
		access protectedThumbnailAccess
	}{
		{name: "jpeg", path: "photo.jpg", kind: "photo", access: protectedThumbnailSource},
		{name: "gif", path: "animation.gif", kind: "gif", access: protectedThumbnailSource},
		{name: "mp4", path: "video.mp4", kind: "video", access: protectedThumbnailSeekableVideo},
		{name: "webm", path: "video.webm", kind: "video", access: protectedThumbnailSeekableVideo},
		{name: "mkv", path: "video.mkv", kind: "video", access: protectedThumbnailSeekableVideo},
		{name: "cbz", path: "comic.cbz", kind: "comic", access: protectedThumbnailComicArchive},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			support, ok := thumbnailSupportForPath(tt.path)
			if !ok {
				t.Fatalf("thumbnail support for %q is undeclared", tt.path)
			}
			if support.kind != tt.kind {
				t.Fatalf("kind = %q, want %q", support.kind, tt.kind)
			}
			if support.protectedAccess != tt.access {
				t.Fatalf("protected access = %q, want %q", support.protectedAccess, tt.access)
			}
		})
	}
}

func TestThumbnailSupportRejectsUndeclaredMediaKind(t *testing.T) {
	t.Parallel()

	if support, ok := thumbnailSupportForPath("document.pdf"); ok {
		t.Fatalf("unexpected thumbnail support declaration: %+v", support)
	}
}
