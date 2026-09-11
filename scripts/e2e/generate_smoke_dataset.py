#!/usr/bin/env python3
"""Generate a deterministic, reusable mixed-media dataset for the E2E smoke test."""

from __future__ import annotations

import argparse
import hashlib
import json
import random
import shutil
import subprocess
import tempfile
import zipfile
from pathlib import Path

from PIL import Image, ImageDraw

FIXTURE_VERSION = 1
DEFAULT_SEED = 0x600D64
RATIOS = {
    "png": 40,
    "jpg": 40,
    "cbz": 1,
    "gif": 9,
    "webm": 5,
    "mp4": 5,
}
SIZES = [(320, 240), (384, 288), (400, 300), (480, 320), (512, 384), (640, 360)]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--count", type=int, default=2000)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--seed", type=int, default=DEFAULT_SEED)
    parser.add_argument("--force", action="store_true")
    return parser.parse_args()


def expected_counts(total: int) -> dict[str, int]:
    if total <= 0 or total % 100:
        raise SystemExit("--count must be a positive multiple of 100 so the media mix is exact")
    return {kind: total * pct // 100 for kind, pct in RATIOS.items()}


def palette(index: int) -> tuple[tuple[int, int, int], ...]:
    rng = random.Random(DEFAULT_SEED + index * 7919)
    return tuple((rng.randrange(24, 232), rng.randrange(24, 232), rng.randrange(24, 232)) for _ in range(5))


def render_image(index: int, width: int, height: int, frame: int = 0) -> Image.Image:
    rng = random.Random(DEFAULT_SEED * 17 + index * 104729 + frame * 1543)
    colors = palette(index + frame)
    img = Image.new("RGB", (width, height), colors[0])
    draw = ImageDraw.Draw(img)
    stripe = max(12, min(width, height) // 10)
    for n, x in enumerate(range(-height, width + height, stripe * 2)):
        draw.polygon(
            [(x, 0), (x + stripe, 0), (x + height + stripe, height), (x + height, height)],
            fill=colors[(n + 1) % len(colors)],
        )
    for n in range(10):
        radius = rng.randint(max(8, min(width, height) // 28), max(16, min(width, height) // 8))
        x = rng.randint(0, width)
        y = rng.randint(0, height)
        draw.ellipse((x - radius, y - radius, x + radius, y + radius), outline=colors[(n + 2) % len(colors)], width=max(2, radius // 8))
    inset = 4 + (index % 9)
    draw.rectangle((inset, inset, width - inset - 1, height - inset - 1), outline=colors[4], width=3)
    # Encode the index visually without relying on fonts.
    bits = f"{index:016b}"
    cell = max(3, width // 100)
    y0 = height - cell * 4
    for bit_index, bit in enumerate(bits):
        if bit == "1":
            x0 = cell * 2 + bit_index * cell * 2
            draw.rectangle((x0, y0, x0 + cell, y0 + cell * 2), fill=colors[3])
    return img


def save_png(path: Path, index: int) -> None:
    w, h = SIZES[index % len(SIZES)]
    render_image(index, w, h).save(path, "PNG", optimize=False)


def save_jpg(path: Path, index: int) -> None:
    w, h = SIZES[index % len(SIZES)]
    render_image(index, w, h).save(path, "JPEG", quality=82, subsampling=1, optimize=False, progressive=False)


def save_gif(path: Path, index: int) -> None:
    w, h = SIZES[index % 3]
    frames = [render_image(index, w, h, frame).quantize(colors=128) for frame in range(6)]
    frames[0].save(path, "GIF", save_all=True, append_images=frames[1:], duration=90, loop=0, optimize=False)


def run_ffmpeg(path: Path, index: int, codec: str) -> None:
    hue = (index * 47) % 360
    source = f"testsrc2=size=480x270:rate=24:duration=1.25,format=yuv420p,hue=h={hue}"
    common = ["ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", source, "-an"]
    if codec == "mp4":
        cmd = common + ["-c:v", "libx264", "-preset", "veryfast", "-crf", "24", "-movflags", "+faststart", str(path)]
    else:
        cmd = common + ["-c:v", "libvpx-vp9", "-deadline", "realtime", "-cpu-used", "5", "-crf", "34", "-b:v", "0", str(path)]
    subprocess.run(cmd, check=True)


def save_cbz(path: Path, index: int) -> None:
    page_count = 12 + (index % 89)  # deterministic 12..100 pages
    with tempfile.TemporaryDirectory(prefix="gooru-cbz-") as tmp:
        tmpdir = Path(tmp)
        pages: list[Path] = []
        for page in range(page_count):
            page_path = tmpdir / f"page-{page + 1:03d}.jpg"
            w, h = SIZES[(index + page) % len(SIZES)]
            render_image(index * 1000 + page, w, h).save(page_path, "JPEG", quality=78, subsampling=1)
            pages.append(page_path)
        with zipfile.ZipFile(path, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=6) as archive:
            for page_path in pages:
                archive.write(page_path, arcname=page_path.name)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as fh:
        for chunk in iter(lambda: fh.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def existing_fixture_ok(output: Path, count: int, seed: int) -> bool:
    manifest_path = output / "manifest.json"
    if not manifest_path.is_file():
        return False
    try:
        manifest = json.loads(manifest_path.read_text())
    except (OSError, json.JSONDecodeError):
        return False
    if manifest.get("fixture_version") != FIXTURE_VERSION or manifest.get("count") != count or manifest.get("seed") != seed:
        return False
    files = manifest.get("files")
    if not isinstance(files, list) or len(files) != count:
        return False
    return all((output / item.get("name", "")).is_file() for item in files if isinstance(item, dict))


def main() -> None:
    args = parse_args()
    counts = expected_counts(args.count)
    output = args.output.expanduser().resolve()
    if not args.force and existing_fixture_ok(output, args.count, args.seed):
        print(f"Reusing stable smoke dataset: {output}")
        return

    if output.exists():
        shutil.rmtree(output)
    output.mkdir(parents=True)

    builders = {
        "png": save_png,
        "jpg": save_jpg,
        "gif": save_gif,
        "cbz": save_cbz,
    }
    files: list[dict[str, object]] = []
    serial = 0
    for kind in ("png", "jpg", "cbz", "gif", "webm", "mp4"):
        for local_index in range(counts[kind]):
            serial += 1
            name = f"smoke-{serial:04d}-{kind}-{local_index + 1:04d}.{kind}"
            path = output / name
            visual_index = args.seed + serial
            if kind in builders:
                builders[kind](path, visual_index)
            else:
                run_ffmpeg(path, visual_index, kind)
            size = path.stat().st_size
            if size > 20 * 1024 * 1024:
                raise RuntimeError(f"fixture exceeds 20 MiB: {name} ({size} bytes)")
            files.append({"name": name, "kind": kind, "bytes": size, "sha256": sha256(path)})
            if serial % 100 == 0 or serial == args.count:
                print(f"generated {serial}/{args.count}", flush=True)

    manifest = {
        "fixture_version": FIXTURE_VERSION,
        "seed": args.seed,
        "count": args.count,
        "ratios_percent": RATIOS,
        "counts": counts,
        "files": files,
    }
    (output / "manifest.json").write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n")
    print(f"Generated stable smoke dataset: {output}")


if __name__ == "__main__":
    main()
