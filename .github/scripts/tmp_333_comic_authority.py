from pathlib import Path


def replace(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    if old not in text:
        raise SystemExit(f"missing expected snippet in {path}")
    p.write_text(text.replace(old, new, 1))


replace(
    "internal/serve/browse.go",
    '''func mediaKindForType(mediaType string) string {
\tswitch {
\tcase mediaType == "image/gif":
\t\treturn "gif"''',
    '''func mediaKindForType(mediaType string) string {
\tbaseType := strings.TrimSpace(strings.SplitN(mediaType, ";", 2)[0])
\tswitch {
\tcase strings.EqualFold(baseType, "application/vnd.comicbook+zip"):
\t\treturn "comic"
\tcase mediaType == "image/gif":
\t\treturn "gif"''',
)

replace(
    "internal/database/database.go",
    '''func fileKindExpression() string {
\treturn `coalesce(mm.media_kind, CASE
\t\tWHEN lower(l.extension) = '.cbz' THEN 'comic'
\t\tWHEN lower(l.extension) = '.gif' THEN 'gif'
\t\tWHEN lower(l.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
\t\tWHEN lower(l.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
\t\tELSE 'other'
\tEND)`
}''',
    '''func fileKindExpression() string {
\treturn `CASE
\t\tWHEN lower(l.extension) = '.cbz' THEN 'comic'
\t\tELSE coalesce(mm.media_kind, CASE
\t\t\tWHEN lower(l.extension) = '.gif' THEN 'gif'
\t\t\tWHEN lower(l.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
\t\t\tWHEN lower(l.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
\t\t\tELSE 'other'
\t\tEND)
\tEND`
}''',
)

replace(
    "internal/query/sqlbuilder.go",
    '''func mediaTypeExpression() string {
\treturn `coalesce(mm.media_kind, CASE
\t\tWHEN lower(l.extension) = '.cbz' THEN 'comic'
\t\tWHEN lower(l.extension) = '.gif' THEN 'gif'
\t\tWHEN lower(l.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
\t\tWHEN lower(l.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
\t\tELSE 'other'
\tEND)`
}''',
    '''func mediaTypeExpression() string {
\treturn `CASE
\t\tWHEN lower(l.extension) = '.cbz' THEN 'comic'
\t\tELSE coalesce(mm.media_kind, CASE
\t\t\tWHEN lower(l.extension) = '.gif' THEN 'gif'
\t\t\tWHEN lower(l.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
\t\t\tWHEN lower(l.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
\t\t\tELSE 'other'
\t\tEND)
\tEND`
}''',
)

replace(
    "internal/query/sqlbuilder.go",
    '''\tb.query.WriteString(`SELECT ` + selectColumn + ` FROM media_metadata mm JOIN locations l ON l.id = mm.location_id WHERE lower(mm.media_kind) = lower(?)`)
\tb.args = append(b.args, value)

\tfallback := fallbackMediaTypeCondition(value)''',
    '''\tb.query.WriteString(`SELECT ` + selectColumn + ` FROM media_metadata mm JOIN locations l ON l.id = mm.location_id WHERE lower(l.extension) <> '.cbz' AND lower(mm.media_kind) = lower(?)`)
\tb.args = append(b.args, value)

\tif strings.EqualFold(value, "comic") {
\t\tb.query.WriteString(union + `SELECT ` + selectColumn + ` FROM locations l WHERE lower(l.extension) = '.cbz'`)
\t\treturn
\t}

\tfallback := fallbackMediaTypeCondition(value)''',
)

Path("internal/database/migrations/019_comic_kind_summary.up.sql").write_text('''-- Treat CBZ archives as the existing first-class `comic` media kind so root
-- kind facets can answer comics availability without an extra browse query.
-- The CBZ extension is authoritative even when legacy metadata says `other`.
DROP TRIGGER IF EXISTS kind_counts_location_insert;
DROP TRIGGER IF EXISTS kind_counts_location_delete;
DROP TRIGGER IF EXISTS kind_counts_location_extension_update;
DROP TRIGGER IF EXISTS kind_counts_metadata_insert;
DROP TRIGGER IF EXISTS kind_counts_metadata_update;
DROP TRIGGER IF EXISTS kind_counts_metadata_delete;

DELETE FROM kind_counts;
INSERT INTO kind_counts (kind, files_count)
SELECT CASE
        WHEN lower(l.extension) = '.cbz' THEN 'comic'
        ELSE coalesce(mm.media_kind, CASE
            WHEN lower(l.extension) = '.gif' THEN 'gif'
            WHEN lower(l.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
            WHEN lower(l.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
            ELSE 'other'
        END)
    END AS kind,
    COUNT(*)
FROM locations l
LEFT JOIN media_metadata mm ON mm.location_id = l.id
GROUP BY kind;

CREATE TRIGGER kind_counts_location_insert
AFTER INSERT ON locations
BEGIN
    INSERT INTO kind_counts (kind, files_count)
    VALUES (
        CASE
            WHEN lower(NEW.extension) = '.cbz' THEN 'comic'
            WHEN lower(NEW.extension) = '.gif' THEN 'gif'
            WHEN lower(NEW.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
            WHEN lower(NEW.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
            ELSE 'other'
        END,
        1
    )
    ON CONFLICT(kind) DO UPDATE SET files_count = files_count + 1;
END;

CREATE TRIGGER kind_counts_location_delete
BEFORE DELETE ON locations
BEGIN
    UPDATE kind_counts
    SET files_count = files_count - 1
    WHERE kind = CASE
        WHEN lower(OLD.extension) = '.cbz' THEN 'comic'
        ELSE coalesce(
            (SELECT media_kind FROM media_metadata WHERE location_id = OLD.id),
            CASE
                WHEN lower(OLD.extension) = '.gif' THEN 'gif'
                WHEN lower(OLD.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
                WHEN lower(OLD.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
                ELSE 'other'
            END
        )
    END;
END;

CREATE TRIGGER kind_counts_location_extension_update
AFTER UPDATE OF extension ON locations
WHEN OLD.extension IS NOT NEW.extension
BEGIN
    UPDATE kind_counts
    SET files_count = files_count - 1
    WHERE kind = CASE
        WHEN lower(OLD.extension) = '.cbz' THEN 'comic'
        ELSE coalesce(
            (SELECT media_kind FROM media_metadata WHERE location_id = NEW.id),
            CASE
                WHEN lower(OLD.extension) = '.gif' THEN 'gif'
                WHEN lower(OLD.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
                WHEN lower(OLD.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
                ELSE 'other'
            END
        )
    END;

    INSERT INTO kind_counts (kind, files_count)
    VALUES (
        CASE
            WHEN lower(NEW.extension) = '.cbz' THEN 'comic'
            ELSE coalesce(
                (SELECT media_kind FROM media_metadata WHERE location_id = NEW.id),
                CASE
                    WHEN lower(NEW.extension) = '.gif' THEN 'gif'
                    WHEN lower(NEW.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
                    WHEN lower(NEW.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
                    ELSE 'other'
                END
            )
        END,
        1
    )
    ON CONFLICT(kind) DO UPDATE SET files_count = files_count + 1;
END;

CREATE TRIGGER kind_counts_metadata_insert
AFTER INSERT ON media_metadata
BEGIN
    UPDATE kind_counts
    SET files_count = files_count - 1
    WHERE kind = (
        SELECT CASE
            WHEN lower(extension) = '.cbz' THEN 'comic'
            WHEN lower(extension) = '.gif' THEN 'gif'
            WHEN lower(extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
            WHEN lower(extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
            ELSE 'other'
        END
        FROM locations
        WHERE id = NEW.location_id
    );

    INSERT INTO kind_counts (kind, files_count)
    SELECT CASE WHEN lower(extension) = '.cbz' THEN 'comic' ELSE NEW.media_kind END, 1
    FROM locations
    WHERE id = NEW.location_id
    ON CONFLICT(kind) DO UPDATE SET files_count = files_count + 1;
END;

CREATE TRIGGER kind_counts_metadata_update
AFTER UPDATE OF media_kind ON media_metadata
WHEN OLD.media_kind IS NOT NEW.media_kind
BEGIN
    UPDATE kind_counts
    SET files_count = files_count - 1
    WHERE kind = (
        SELECT CASE WHEN lower(extension) = '.cbz' THEN 'comic' ELSE OLD.media_kind END
        FROM locations
        WHERE id = NEW.location_id
    );
    INSERT INTO kind_counts (kind, files_count)
    SELECT CASE WHEN lower(extension) = '.cbz' THEN 'comic' ELSE NEW.media_kind END, 1
    FROM locations
    WHERE id = NEW.location_id
    ON CONFLICT(kind) DO UPDATE SET files_count = files_count + 1;
END;

CREATE TRIGGER kind_counts_metadata_delete
AFTER DELETE ON media_metadata
WHEN EXISTS (SELECT 1 FROM locations WHERE id = OLD.location_id)
BEGIN
    UPDATE kind_counts
    SET files_count = files_count - 1
    WHERE kind = (
        SELECT CASE WHEN lower(extension) = '.cbz' THEN 'comic' ELSE OLD.media_kind END
        FROM locations
        WHERE id = OLD.location_id
    );
    INSERT INTO kind_counts (kind, files_count)
    SELECT CASE
        WHEN lower(extension) = '.cbz' THEN 'comic'
        WHEN lower(extension) = '.gif' THEN 'gif'
        WHEN lower(extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
        WHEN lower(extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
        ELSE 'other'
    END,
    1
    FROM locations
    WHERE id = OLD.location_id
    ON CONFLICT(kind) DO UPDATE SET files_count = files_count + 1;
END;
''')

p = Path("internal/database/kind_counts_test.go")
text = p.read_text()
old = '''\tif err := RunMigrations(db); err != nil {
\t\tt.Fatalf("RunMigrations through v12: %v", err)
\t}
\tassertKindCountsMatchRecomputed(t, db)

\tif _, err := db.Exec(`
\t\tINSERT INTO media_metadata (location_id, media_kind, mime_type)
\t\tVALUES (?, 'video', 'video/mp4')
\t`, photoID); err != nil {'''
new = '''\tif err := RunMigrations(db); err != nil {
\t\tt.Fatalf("RunMigrations: %v", err)
\t}
\tassertKindCountsMatchRecomputed(t, db)

\tif _, err := db.Exec(`
\t\tINSERT INTO media_metadata (location_id, media_kind, mime_type)
\t\tVALUES (?, 'other', 'application/vnd.comicbook+zip')
\t`, comicID); err != nil {
\t\tt.Fatal(err)
\t}
\tassertKindCountsMatchRecomputed(t, db)

\tif _, err := db.Exec(`UPDATE media_metadata SET media_kind = 'photo' WHERE location_id = ?`, comicID); err != nil {
\t\tt.Fatal(err)
\t}
\tassertKindCountsMatchRecomputed(t, db)

\tif _, err := db.Exec(`UPDATE locations SET extension = '.zip' WHERE id = ?`, comicID); err != nil {
\t\tt.Fatal(err)
\t}
\tassertKindCountsMatchRecomputed(t, db)

\tif _, err := db.Exec(`UPDATE locations SET extension = '.cbz' WHERE id = ?`, comicID); err != nil {
\t\tt.Fatal(err)
\t}
\tassertKindCountsMatchRecomputed(t, db)

\tif _, err := db.Exec(`
\t\tINSERT INTO media_metadata (location_id, media_kind, mime_type)
\t\tVALUES (?, 'video', 'video/mp4')
\t`, photoID); err != nil {'''
if old not in text:
    raise SystemExit("missing kind-count insertion point")
text = text.replace(old, new, 1)
# Remove the old metadata-free comic extension mutation; the new block above
# exercises the harder metadata-backed transition in both directions.
old = '''\tif _, err := db.Exec(`UPDATE locations SET extension = '.zip' WHERE id = ?`, comicID); err != nil {
\t\tt.Fatal(err)
\t}
\tassertKindCountsMatchRecomputed(t, db)

'''
if old not in text:
    raise SystemExit("missing old comic extension mutation")
text = text.replace(old, "", 1)
old = '''\t\tSELECT coalesce(mm.media_kind, CASE
\t\t\tWHEN lower(l.extension) = '.cbz' THEN 'comic'
\t\t\tWHEN lower(l.extension) = '.gif' THEN 'gif'
\t\t\tWHEN lower(l.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
\t\t\tWHEN lower(l.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
\t\t\tELSE 'other'
\t\tEND) AS kind, COUNT(*)'''
new = '''\t\tSELECT CASE
\t\t\tWHEN lower(l.extension) = '.cbz' THEN 'comic'
\t\t\tELSE coalesce(mm.media_kind, CASE
\t\t\t\tWHEN lower(l.extension) = '.gif' THEN 'gif'
\t\t\t\tWHEN lower(l.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
\t\t\t\tWHEN lower(l.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
\t\t\t\tELSE 'other'
\t\t\tEND)
\t\tEND AS kind, COUNT(*)'''
if old not in text:
    raise SystemExit("missing kind-count recompute query")
p.write_text(text.replace(old, new, 1))

Path("internal/serve/comic_kind_test.go").write_text('''package serve

import "testing"

func TestMediaKindForComicBookZip(t *testing.T) {
\tfor _, mediaType := range []string{
\t\t"application/vnd.comicbook+zip",
\t\t"application/vnd.comicbook+zip; charset=binary",
\t} {
\t\tif got := mediaKindForType(mediaType); got != "comic" {
\t\t\tt.Fatalf("mediaKindForType(%q) = %q, want comic", mediaType, got)
\t\t}
\t}
}
''')
