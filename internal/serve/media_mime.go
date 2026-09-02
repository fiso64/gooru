package serve

import "mime"

func init() {
	// mime.TypeByExtension may inherit host-specific /etc/mime.types entries.
	// Keep browser media/archive extensions deterministic across hosts.
	_ = mime.AddExtensionType(".webm", "video/webm")
	_ = mime.AddExtensionType(".weba", "audio/webm")
	_ = mime.AddExtensionType(".cbz", "application/vnd.comicbook+zip")
}
