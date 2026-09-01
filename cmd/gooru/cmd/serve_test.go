package cmd

import (
	"testing"

	"gooru.local/internal/serve"
)

func TestStartupURL(t *testing.T) {
	tests := []struct {
		name string
		cfg  serve.Config
		want string
	}{
		{
			name: "loopback listen",
			cfg:  serve.Config{Server: serve.ServerConfig{Listen: "127.0.0.1:5678"}},
			want: "http://127.0.0.1:5678",
		},
		{
			name: "wildcard listen stays clickable",
			cfg:  serve.Config{Server: serve.ServerConfig{Listen: ":5678"}},
			want: "http://127.0.0.1:5678",
		},
		{
			name: "ipv6 wildcard stays clickable",
			cfg:  serve.Config{Server: serve.ServerConfig{Listen: "[::]:5678"}},
			want: "http://127.0.0.1:5678",
		},
		{
			name: "public URL wins",
			cfg: serve.Config{Server: serve.ServerConfig{
				Listen:    "127.0.0.1:5678",
				PublicURL: "https://gooru.example.test/",
			}},
			want: "https://gooru.example.test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := startupURL(tt.cfg); got != tt.want {
				t.Fatalf("startupURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
