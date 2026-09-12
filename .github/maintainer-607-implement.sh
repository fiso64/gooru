#!/usr/bin/env bash
set -euo pipefail

git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
git fetch origin develop
git rebase origin/develop

cat > internal/buildinfo/buildinfo.go <<'EOF'
package buildinfo

import (
	"fmt"
	"runtime/debug"
	"strings"
)

// Version, Revision, and Dirty are overridden with -ldflags by release and
// package builds. Source builds intentionally identify themselves as dev.
var (
	Version  = "0.0.0-dev"
	Revision = ""
	Dirty    = ""
)

type Info struct {
	Version     string `json:"version"`
	Revision    string `json:"revision"`
	Dirty       bool   `json:"dirty"`
	Development bool   `json:"development"`
}

func Current() Info {
	version := strings.TrimSpace(Version)
	if version == "" { version = "0.0.0-dev" }
	revision := strings.TrimSpace(Revision)
	dirtyText := strings.TrimSpace(Dirty)
	dirty := strings.EqualFold(dirtyText, "true")
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range bi.Settings {
			switch setting.Key {
			case "vcs.revision":
				if revision == "" && setting.Value != "" { revision = setting.Value }
			case "vcs.modified":
				if dirtyText == "" { dirty = setting.Value == "true" }
			}
		}
	}
	if revision == "" { revision = "unknown" }
	return Info{Version: version, Revision: revision, Dirty: dirty, Development: strings.Contains(version, "dev") || dirty || revision == "unknown"}
}

func ShortRevision(revision string) string {
	revision = strings.TrimSpace(revision)
	if len(revision) <= 12 { return revision }
	return revision[:12]
}

func Summary() string {
	info := Current()
	state := ""
	if info.Dirty { state = ", dirty" } else if info.Development { state = ", development" }
	return fmt.Sprintf("gooru v%s (revision %s%s)", info.Version, ShortRevision(info.Revision), state)
}
EOF

cat > internal/buildinfo/buildinfo_test.go <<'EOF'
package buildinfo

import "testing"

func TestCurrentUsesInjectedIdentity(t *testing.T) {
	oldVersion, oldRevision, oldDirty := Version, Revision, Dirty
	t.Cleanup(func() { Version, Revision, Dirty = oldVersion, oldRevision, oldDirty })
	Version, Revision, Dirty = "1.2.3", "0123456789abcdef", "false"
	got := Current()
	if got.Version != "1.2.3" || got.Revision != "0123456789abcdef" || got.Dirty || got.Development { t.Fatalf("unexpected build info: %+v", got) }
	if ShortRevision(got.Revision) != "0123456789ab" { t.Fatalf("unexpected short revision: %q", ShortRevision(got.Revision)) }
}
EOF

cat > cmd/gooru/cmd/version.go <<'EOF'
package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"gooru.local/internal/buildinfo"
)

var versionCmd = &cobra.Command{
	Use: "version",
	Short: "Print Gooru build identity",
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) { fmt.Fprintln(cmd.OutOrStdout(), buildinfo.Summary()) },
}

func init() {
	rootCmd.Version = buildinfo.Summary()
	rootCmd.SetVersionTemplate("{{.Version}}\n")
	rootCmd.AddCommand(versionCmd)
}
EOF

python3 - <<'PY'
from pathlib import Path
p=Path('cmd/gooru/cmd/root.go'); s=p.read_text()
s=s.replace('cmd.Name() == "init" || cmd.Name() == "serve" || cmd.CommandPath() == "gooru user create-admin"','cmd.Name() == "init" || cmd.Name() == "serve" || cmd.Name() == "version" || cmd.CommandPath() == "gooru user create-admin"')
p.write_text(s)
PY

cat > internal/serve/build_info.go <<'EOF'
package serve

import (
	"net/http"
	"gooru.local/internal/buildinfo"
)

func (s *Server) handleBuildInfo(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, buildinfo.Current()) }
EOF

cat > internal/serve/build_info_test.go <<'EOF'
package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"gooru.local/internal/buildinfo"
)

func TestBuildInfoEndpoint(t *testing.T) {
	oldVersion, oldRevision, oldDirty := buildinfo.Version, buildinfo.Revision, buildinfo.Dirty
	t.Cleanup(func() { buildinfo.Version, buildinfo.Revision, buildinfo.Dirty = oldVersion, oldRevision, oldDirty })
	buildinfo.Version, buildinfo.Revision, buildinfo.Dirty = "1.2.3", "0123456789abcdef", "false"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/build", nil); rec := httptest.NewRecorder()
	NewServer(Config{}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK { t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String()) }
	var got buildinfo.Info
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil { t.Fatal(err) }
	if got.Version != "1.2.3" || got.Revision != "0123456789abcdef" || got.Development { t.Fatalf("unexpected build info: %+v", got) }
}
EOF

python3 - <<'PY'
from pathlib import Path
p=Path('internal/serve/server.go'); s=p.read_text(); marker='\tmux.HandleFunc("/api/v1/health", methodHandler(http.MethodGet, s.handleHealth))\n'
if marker not in s: raise SystemExit('health route marker missing')
s=s.replace(marker,'\tmux.HandleFunc("/api/v1/build", methodHandler(http.MethodGet, s.handleBuildInfo))\n'+marker,1); p.write_text(s)
PY

cat > frontend/src/lib/api/buildInfo.ts <<'EOF'
export type BuildInfo = { version: string; revision: string; dirty: boolean; development: boolean };

export async function getBuildInfo(): Promise<BuildInfo> {
  const response = await fetch('/api/v1/build', { headers: { Accept: 'application/json' } });
  if (!response.ok) throw new Error(`build info request failed: ${response.status}`);
  return response.json() as Promise<BuildInfo>;
}
EOF

python3 - <<'PY'
from pathlib import Path
p=Path('frontend/src/lib/components/AuthPanel.svelte'); s=p.read_text()
s=s.replace("  import { onMount } from 'svelte';\n", "  import { onMount } from 'svelte';\n  import { getBuildInfo, type BuildInfo } from '$lib/api/buildInfo';\n")
s=s.replace('  let usernameInput: HTMLInputElement;\n\n  onMount(() => {\n    usernameInput?.focus();\n  });','''  let usernameInput: HTMLInputElement;
  let buildInfo: BuildInfo | null = $state(null);

  function versionLabel(): string { return buildInfo ? `v${buildInfo.version}` : 'v…'; }
  function buildRef(): string {
    if (!buildInfo) return 'develop';
    if (!buildInfo.development) return `v${buildInfo.version}`;
    return buildInfo.revision && buildInfo.revision !== 'unknown' ? buildInfo.revision : 'develop';
  }
  function sourceHref(): string {
    if (!buildInfo || !buildInfo.revision || buildInfo.revision === 'unknown') return 'https://github.com/fiso64/gooru';
    return `https://github.com/fiso64/gooru/tree/${buildInfo.revision}`;
  }
  function docsHref(): string { return `https://github.com/fiso64/gooru/tree/${buildRef()}/docs`; }

  onMount(() => {
    usernameInput?.focus();
    void getBuildInfo().then((info) => { buildInfo = info; }).catch(() => {});
  });''')
s=s.replace('<div class="login-v2-id-meta"><span>agpl-3.0</span></div>','<div class="login-v2-id-meta"><span>{versionLabel()}</span><span>agpl-3.0</span></div>')
s=s.replace('<a href="https://github.com/fiso64/gooru/tree/develop/docs" target="_blank" rel="noreferrer">docs</a>','<a href={docsHref()} target="_blank" rel="noreferrer">docs</a>')
s=s.replace('<a href="https://github.com/fiso64/gooru" target="_blank" rel="noreferrer">source</a>','<a href={sourceHref()} target="_blank" rel="noreferrer">source</a>')
p.write_text(s)
PY

python3 - <<'PY'
from pathlib import Path
p=Path('flake.nix'); s=p.read_text()
s=s.replace('      supportedSystems = [ "x86_64-linux" "aarch64-linux" ];\n      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;','''      supportedSystems = [ "x86_64-linux" "aarch64-linux" ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
      version = builtins.replaceStrings [ "\\n" ] [ "" ] (builtins.readFile ./VERSION);
      revision = if self ? rev then self.rev else if self ? dirtyRev then builtins.replaceStrings [ "-dirty" ] [ "" ] self.dirtyRev else "unknown";
      dirty = if self ? dirtyRev then "true" else "false";''')
s=s.replace('version = "0-unstable";', 'version = version;')
needle='            tags = [ "govips" ];\n'
if needle not in s: raise SystemExit('flake tags marker missing')
s=s.replace(needle,needle+'''            ldflags = [
              "-X=gooru.local/internal/buildinfo.Version=${version}"
              "-X=gooru.local/internal/buildinfo.Revision=${revision}"
              "-X=gooru.local/internal/buildinfo.Dirty=${dirty}"
            ];
''',1)
p.write_text(s)
PY

python3 - <<'PY'
from pathlib import Path
p=Path('.github/workflows/nix-package.yml'); s=p.read_text()
s=s.replace("      - 'flake.nix'\n", "      - 'flake.nix'\n      - 'VERSION'\n      - 'internal/buildinfo/**'\n")
p.write_text(s)
PY

cat > .github/workflows/release.yml <<'EOF'
name: Release

on:
  push:
    tags:
      - 'v*'

permissions:
  contents: write

jobs:
  release:
    runs-on: [self-hosted, gooru]
    env:
      GOTOOLCHAIN: local
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-node@v4
        with:
          node-version: 24
          cache: npm
          cache-dependency-path: frontend/package-lock.json
      - name: Validate tag and canonical version
        shell: bash
        run: |
          set -euo pipefail
          version="$(tr -d '\n' < VERSION)"
          [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([+-][0-9A-Za-z.-]+)?$ ]]
          [[ "$GITHUB_REF_NAME" == "v$version" ]]
      - name: Test Go
        run: go test -count=1 ./...
      - name: Build frontend
        working-directory: frontend
        run: |
          npm ci --prefer-offline --no-audit
          npm run check
          npm run test:unit
          npm run build
      - name: Validate Nix package
        run: nix build .#packages.x86_64-linux.default --print-build-logs
      - name: Build release binary
        shell: bash
        run: |
          set -euo pipefail
          version="$(tr -d '\n' < VERSION)"
          mkdir -p dist
          go build -trimpath -ldflags "-X=gooru.local/internal/buildinfo.Version=$version -X=gooru.local/internal/buildinfo.Revision=$GITHUB_SHA -X=gooru.local/internal/buildinfo.Dirty=false" -o dist/gooru-linux-amd64 ./cmd/gooru
          dist/gooru-linux-amd64 version
          tar -C dist -czf "dist/gooru_${version}_linux_amd64.tar.gz" gooru-linux-amd64
          sha256sum "dist/gooru_${version}_linux_amd64.tar.gz" > "dist/gooru_${version}_linux_amd64.tar.gz.sha256"
      - name: Create GitHub Release
        env:
          GH_TOKEN: ${{ github.token }}
        shell: bash
        run: |
          version="$(tr -d '\n' < VERSION)"
          gh release create "$GITHUB_REF_NAME" "dist/gooru_${version}_linux_amd64.tar.gz" "dist/gooru_${version}_linux_amd64.tar.gz.sha256" --verify-tag --generate-notes --title "$GITHUB_REF_NAME"
EOF

python3 - <<'PY'
from pathlib import Path
p=Path('docs/openapi.yaml'); s=p.read_text(); marker='  /health:\n'
if marker not in s: raise SystemExit('OpenAPI health marker missing')
block='''  /build:
    get:
      summary: Get running build identity.
      responses:
        "200":
          description: Running server version and source revision.
          content:
            application/json:
              schema:
                type: object
                required: [version, revision, dirty, development]
                properties:
                  version:
                    type: string
                  revision:
                    type: string
                  dirty:
                    type: boolean
                  development:
                    type: boolean
'''
s=s.replace(marker,block+marker,1); p.write_text(s)
PY

rm -f .github/workflows/maintainer-607-implement.yml .github/maintainer-607-implement.sh
gofmt -w internal/buildinfo cmd/gooru/cmd/version.go internal/serve/build_info.go internal/serve/build_info_test.go
git add -A
git diff --cached --check
git commit -m "feat: add canonical build version identity"

go test -count=1 ./internal/buildinfo ./internal/serve ./cmd/gooru/cmd
go build -o /tmp/gooru-607 ./cmd/gooru
/tmp/gooru-607 version
/tmp/gooru-607 --version
npm --prefix frontend ci --prefer-offline --no-audit
npm --prefix frontend run check
npm --prefix frontend run test:unit
npm --prefix frontend run build
nix build .#packages.x86_64-linux.default --print-build-logs

git push --force-with-lease origin HEAD:maintainer/607-versioning
