#!/usr/bin/env python3
"""Build user installers. Publishing is handled separately by GitHub Actions."""
import argparse
import hashlib
import ipaddress
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tomllib
from urllib.parse import urlsplit

ROOT = Path(__file__).resolve().parents[2]
TARGETS = {
    "android": ("mobile", "aarch64-linux-android", (".apk",)),
    "linux": ("desktop", "x86_64-unknown-linux-gnu", (".AppImage", ".deb")),
    "windows": ("desktop", "x86_64-pc-windows-msvc", (".exe",)),
}


def validate_endpoint(endpoint):
    url = urlsplit(endpoint)
    host = (url.hostname or "").lower().rstrip(".")
    if url.scheme != "https" or not host or url.username or url.password or url.query or url.fragment:
        raise ValueError("VITE_API_ENDPOINT must be an HTTPS URL without credentials/query/fragment")
    if host == "localhost" or host.endswith(".localhost"):
        raise ValueError("Release endpoint cannot be localhost")
    try:
        address = ipaddress.ip_address(host)
    except ValueError:
        return
    if address.is_loopback or address.is_unspecified or address.is_link_local:
        raise ValueError("Release endpoint cannot be a loopback/unspecified/link-local address")


def validate_version(root, version):
    if not re.fullmatch(r"\d+\.\d+\.\d+", version):
        raise ValueError("Use a release version such as 0.1.0")
    files = [(root / "client/package.json", "json")]
    for workspace in ("mobile", "desktop"):
        base = root / "client" / workspace
        files += [(base / "package.json", "json"), (base / "src-tauri/tauri.conf.json", "json"),
                  (base / "src-tauri/Cargo.toml", "toml")]
    for path, kind in files:
        data = tomllib.loads(path.read_text()) if kind == "toml" else json.loads(path.read_text())
        actual = data["package"]["version"] if kind == "toml" else data["version"]
        if actual != version:
            raise ValueError(f"{path.relative_to(root)}: version {actual} does not match {version}")


def validate_android(env):
    required = ("ANDROID_KEYSTORE_PATH", "ANDROID_STORE_PASSWORD", "ANDROID_KEY_ALIAS", "ANDROID_KEY_PASSWORD")
    missing = [name for name in required if not env.get(name)]
    if missing:
        raise ValueError("Missing Android signing variables: " + ", ".join(missing))
    if not Path(env["ANDROID_KEYSTORE_PATH"]).is_file():
        raise ValueError("ANDROID_KEYSTORE_PATH must point to an existing keystore")
    code = env.get("ANDROID_VERSION_CODE", "")
    if not code.isdecimal() or not 1 <= int(code) <= 2100000000:
        raise ValueError("ANDROID_VERSION_CODE must be an integer between 1 and 2100000000")
    return int(code)


def collect(source, output, extensions):
    selected = []
    for extension in extensions:
        matches = sorted(path for path in source.rglob("*" + extension)
                         if "unsigned" not in path.name and path.is_file()
                         and (extension != ".apk" or path.parent.name == "release"))
        if len(matches) != 1:
            raise ValueError(f"Expected exactly one {extension} installer in {source}; found {len(matches)}")
        selected += matches
    output.mkdir(parents=True, exist_ok=True)
    for path in selected:
        shutil.copy2(path, output / path.name)
    return [output / path.name for path in selected]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--platform", choices=TARGETS, required=True)
    parser.add_argument("--version", default=os.environ.get("VERSION", "0.1.0"))
    parser.add_argument("--check-only", action="store_true")
    args = parser.parse_args()
    validate_endpoint(os.environ.get("VITE_API_ENDPOINT", ""))
    validate_version(ROOT, args.version)
    config = {}
    if args.platform == "android":
        config = {"bundle": {"android": {"versionCode": validate_android(os.environ)}}}
    if args.check_only:
        print(f"Validated {args.platform} installer inputs for {args.version}")
        return
    workspace, target, extensions = TARGETS[args.platform]
    if args.platform == "windows" and sys.platform != "win32":
        raise ValueError("Windows installer requires a native Windows MSVC builder")
    if args.platform == "linux" and not sys.platform.startswith("linux"):
        raise ValueError("Linux installer requires a Linux builder")
    base = ROOT / "client" / workspace / "src-tauri"
    output = ROOT / "dist/installers" / args.platform
    if output.exists() and any(output.iterdir()):
        raise ValueError(f"Output directory is not empty: {output}. Choose a clean checkout for a new build.")
    env = os.environ.copy()
    # Separate Cargo outputs from development and other platform builds.
    env["CARGO_TARGET_DIR"] = str(ROOT / "dist/app-targets" / args.platform)
    cli = ROOT / "client/node_modules/@tauri-apps/cli/tauri.js"
    command = ["node", str(cli)]
    if args.platform == "android":
        command += ["android", "build", "--ci", "--target", "aarch64", "--apk", "--config", json.dumps(config)]
        source = base / "gen/android/app/build/outputs/apk"
    else:
        bundles = "appimage,deb" if args.platform == "linux" else "nsis"
        command += ["build", "--ci", "--target", target, "--bundles", bundles, "--", "--locked"]
        source = Path(env["CARGO_TARGET_DIR"]) / target / "release/bundle"
    subprocess.run(command, cwd=base.parent, env=env, check=True)
    if args.platform == "android":
        # Verify the actual artifact before copying/uploading it.
        sdk = Path(env.get("ANDROID_HOME") or env["ANDROID_SDK_ROOT"])
        apksigner = sdk / "build-tools/35.0.0/apksigner"
        for apk in source.rglob("*.apk"):
            if "unsigned" not in apk.name and apk.parent.name == "release":
                subprocess.run([str(apksigner), "verify", "--verbose", "--print-certs", str(apk)], check=True)
    artifacts = collect(source, output, extensions)
    checksums = "".join(f"{hashlib.sha256(path.read_bytes()).hexdigest()}  {path.name}\n" for path in artifacts)
    (output / f"SHA256SUMS-{args.platform}.txt").write_text(checksums, newline="\n")
    commit = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()
    manifest = {"version": args.version, "commit": commit, "platform": args.platform,
                "apiEndpoint": env["VITE_API_ENDPOINT"], "artifacts": [p.name for p in artifacts],
                "signing": "verified APK signature" if args.platform == "android" else "unsigned",
                "androidVersionCode": config.get("bundle", {}).get("android", {}).get("versionCode")}
    (output / f"manifest-{args.platform}.json").write_text(json.dumps(manifest, indent=2) + "\n")
    print(f"Installers saved to {output}")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, subprocess.CalledProcessError) as error:
        sys.exit(str(error))
