#!/usr/bin/env bash
set -u -o pipefail

count="${1:?file count is required}"
retain_logs="${2:-false}"

: "${GITHUB_WORKSPACE:?GITHUB_WORKSPACE is required}"
: "${RUNNER_TEMP:?RUNNER_TEMP is required}"
: "${GITHUB_RUN_ID:?GITHUB_RUN_ID is required}"
: "${GITHUB_RUN_ATTEMPT:?GITHUB_RUN_ATTEMPT is required}"
: "${GITHUB_SHA:?GITHUB_SHA is required}"
: "${RUNNER_NAME:?RUNNER_NAME is required}"
: "${GOORU_SMOKE_ARTIFACT_ROOT:?GOORU_SMOKE_ARTIFACT_ROOT is required}"
: "${GOORU_SMOKE_PASSWORD:?GOORU_SMOKE_PASSWORD is required}"
: "${GOORU_SMOKE_USERNAME:?GOORU_SMOKE_USERNAME is required}"

stamp="$(date -u +%Y%m%dT%H%M%SZ)"
artifact_dir="$GOORU_SMOKE_ARTIFACT_ROOT/$stamp"
dataset_dir="$HOME/.cache/gooru/e2e-smoke/v1-$count"
run_dir="$RUNNER_TEMP/gooru-e2e-smoke-${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}-${count}"
upload_dir="$run_dir/uploads"
media_cache="$run_dir/media-cache"
database="$run_dir/gooru.db"
config="$run_dir/serve.yaml"
readme="$GOORU_SMOKE_ARTIFACT_ROOT/README.md"

mkdir -p "$artifact_dir" "$upload_dir" "$media_cache"

if [[ "$retain_logs" == "true" ]]; then
  daemon_log="$artifact_dir/daemon.log"
  playwright_output="$artifact_dir/playwright"
  logs_label="retained"
else
  daemon_log="$run_dir/daemon.log"
  playwright_output="$run_dir/playwright"
  logs_label="not retained"
fi

printf 'run_id=%s\nrun_attempt=%s\ncommit=%s\nrunner=%s\nfile_count=%s\nretain_logs=%s\n' \
  "$GITHUB_RUN_ID" "$GITHUB_RUN_ATTEMPT" "$GITHUB_SHA" "$RUNNER_NAME" "$count" "$retain_logs" \
  > "$artifact_dir/run.txt"

cat > "$config" <<EOF
server:
  listen: 127.0.0.1:5678
  frontend_dir: $GITHUB_WORKSPACE/frontend/build
database:
  path: $database
uploads:
  enabled: true
  targets:
    - id: smoke
      name: Smoke target
      path: $upload_dir
      added_at_strategy: queue
  max_file_size_bytes: 20971520
  preserve_modtime: true
media:
  cache_dir: $media_cache
  thumbnail_sizes: [256, 512]
  thumbnail_format: jpeg
  preview_size: 1280
  preview_enabled: true
tools:
  ffmpeg_path: ffmpeg
  ffprobe_path: ffprobe
logging:
  level: debug
ui:
  pagination_mode: paged
  items_per_page: 100
EOF

stage_status=0
daemon_pid=""

cleanup() {
  if [[ -n "$daemon_pid" ]]; then
    kill "$daemon_pid" 2>/dev/null || true
    wait "$daemon_pid" 2>/dev/null || true
  fi
  rm -rf "$run_dir"
}
trap cleanup EXIT

if ! GOORU_ADMIN_PASSWORD="$GOORU_SMOKE_PASSWORD" \
  "$GITHUB_WORKSPACE/.e2e/bin/gooru" user create-admin --username "$GOORU_SMOKE_USERNAME" --config "$config"; then
  stage_status=1
else
  ffmpeg_out="$(nix build --no-link --print-out-paths nixpkgs#ffmpeg)" || stage_status=1
  if (( stage_status == 0 )); then
    PATH="$ffmpeg_out/bin:$PATH" "$GITHUB_WORKSPACE/.e2e/bin/gooru" serve --config "$config" >"$daemon_log" 2>&1 &
    daemon_pid=$!

    ready=0
    for _ in $(seq 1 120); do
      if node -e "fetch('http://127.0.0.1:5678/api/v1/ui-config').then(r=>{if(!r.ok)process.exit(1)}).catch(()=>process.exit(1))"; then
        ready=1
        break
      fi
      sleep 1
    done

    if (( ready == 0 )); then
      echo "daemon did not become ready for $count-file smoke stage" >&2
      stage_status=1
    else
      (
        cd "$GITHUB_WORKSPACE/frontend"
        GOORU_SMOKE_FILE_COUNT="$count" \
        GOORU_SMOKE_DATASET_DIR="$dataset_dir" \
        GOORU_SMOKE_ARTIFACT_DIR="$artifact_dir" \
        GOORU_SMOKE_RUN_DIR="$run_dir" \
        GOORU_SMOKE_TAG="e2e:smoke-${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}-${count}" \
        GOORU_SMOKE_PLAYWRIGHT_OUTPUT="$playwright_output" \
        npx playwright test --config playwright.smoke.config.ts
      ) || stage_status=$?
    fi
  fi
fi

if (( stage_status != 0 )) && [[ "$retain_logs" != "true" ]]; then
  if [[ -f "$daemon_log" ]]; then
    cp "$daemon_log" "$artifact_dir/daemon.log"
  fi
  if [[ -d "$playwright_output" ]]; then
    cp -a "$playwright_output" "$artifact_dir/playwright"
  fi
  logs_label="retained after failure"
fi

if (( stage_status != 0 )) && [[ -s "$daemon_log" ]]; then
  echo "::group::Daemon log tail ($count-file smoke stage)"
  tail -n 200 "$daemon_log"
  echo "::endgroup::"
fi

upload_ms="not-completed"
delete_ms="not-completed"
if [[ -f "$artifact_dir/timings.json" ]]; then
  read -r upload_ms delete_ms < <(node - "$artifact_dir/timings.json" <<'NODE'
const fs = require('fs');
const report = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
const fmt = (timing) => timing ? timing.duration_ms.toFixed(3) : 'not-completed';
process.stdout.write(`${fmt(report.upload)}\t${fmt(report.delete_from_disk)}\n`);
NODE
)
  [[ "$upload_ms" == "not-completed" ]] || upload_ms="$upload_ms ms"
  [[ "$delete_ms" == "not-completed" ]] || delete_ms="$delete_ms ms"
fi

result="pass"
if (( stage_status != 0 )); then
  result="FAIL"
fi

printf '| %s | %s | %s | `%s` | `%s` | %s | %s | %s | %s |\n' \
  "$stamp" "$GITHUB_RUN_ID" "$GITHUB_RUN_ATTEMPT" "${GITHUB_SHA:0:12}" "$RUNNER_NAME" \
  "$count" "$upload_ms" "$delete_ms" "$result ($logs_label)" >> "$readme"

printf '### %s files — %s\n\n- Upload: %s\n- Delete from disk: %s\n- Logs: %s\n- Artifact directory: `%s`\n\n' \
  "$count" "$result" "$upload_ms" "$delete_ms" "$logs_label" "$stamp" >> "$GITHUB_STEP_SUMMARY"

exit "$stage_status"
