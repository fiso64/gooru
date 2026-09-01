package serve

import "mime"

func init() {
	// mime.TypeByExtension may inherit host-specific /etc/mime.types entries.
	// Keep the extension used by browser video files deterministic across hosts;
	// audio-only WebM should use the conventional .weba extension instead.
	_ = mime.AddExtensionType(".webm", "video/webm")
	_ = mime.AddExtensionType(".weba", "audio/webm")
}
