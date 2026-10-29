#!/usr/bin/env python3
"""Local/CI image release. Build clean Git snapshots; publish only after all checks/builds.

Requires Python 3.12+, Docker Engine and Buildx. Registry authentication is external
(`docker login`); credentials never pass through this program or the manifest.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tarfile
import tempfile
import tomllib
from datetime import datetime, timezone


class ReleaseError(RuntimeError):
    pass


def run(args, *, cwd=None, capture=False):
    print("+ " + " ".join(str(arg) for arg in args), flush=True)
    result = subprocess.run(
        [str(arg) for arg in args], cwd=cwd, check=True, text=True,
        stdout=subprocess.PIPE if capture else None,
    )
    return result.stdout.strip() if capture else ""


def git(repo, *args):
    return run(["git", "-C", repo, *args], capture=True)


def read_config(repo):
    config = json.loads((repo / "deploy/release/images.json").read_text())
    images = config["images"]
    if config["schemaVersion"] != 1 or not images:
        raise ReleaseError("Unsupported or empty release image manifest")
    for field in ("name", "repository", "imageVariable"):
        if len({item[field] for item in images}) != len(images):
            raise ReleaseError(f"Duplicate image {field}")
    for image in images:
        for field in ("context", "dockerfile"):
            path = Path(image[field])
            if path.is_absolute() or ".." in path.parts:
                raise ReleaseError(f"Image {field} must stay inside the source snapshot")
    return images


def snapshot(repo, revision, destination):
    destination.mkdir(parents=True)
    archive = destination.parent / (destination.name + ".tar")
    run(["git", "-C", repo, "archive", "--format=tar", f"--output={archive}", revision])
    with tarfile.open(archive) as source:
        source.extractall(destination, filter="data")


def bundle_checksums(repo):
    directory = repo / "anomaly-service/models/gatv2-1m-baseline-v1"
    bundle = json.loads((directory / "bundle.json").read_text())
    for name, expected in bundle["files"].items():
        path = directory / name
        if Path(name).name != name or hashlib.sha256(path.read_bytes()).hexdigest() != expected:
            raise ReleaseError(f"ML bundle checksum mismatch: {name}")
    return {
        "modelVersion": bundle["model_version"],
        "featureVersion": bundle["feature_version"],
        "bundleSha256": hashlib.sha256((directory / "bundle.json").read_bytes()).hexdigest(),
    }


def docker_build(source, image, tag, platform, network, *, checks=False, gatv2=None):
    command = ["docker", "buildx", "build", "--load", "--platform", platform,
               "--network", network, "--tag", tag, "--file", source / image["dockerfile"]]
    if network == "host":
        command += ["--allow", "network.host"]
    if checks:
        command += ["--target", image["checksTarget"]]
    if gatv2 is not None and image.get("additionalContext"):
        command += ["--build-context", f"gatv2={gatv2}"]
    command.append(source / image["context"])
    return command


def inspect_image(reference):
    return json.loads(run(["docker", "image", "inspect", reference], capture=True))[0]


def run_container_checks(image, tag):
    """BuildKit checks cannot create sibling containers; run Testcontainers here."""
    if not image.get("testCommand"):
        return
    endpoint = os.environ.get("DOCKER_HOST")
    if not endpoint:
        context = json.loads(run(["docker", "context", "inspect"], capture=True))[0]
        endpoint = context["Endpoints"]["docker"]["Host"]
    if not endpoint.startswith("unix://"):
        raise ReleaseError("Local Testcontainers checks require a Unix Docker socket")
    socket = Path(endpoint.removeprefix("unix://")).resolve()
    if not socket.exists():
        raise ReleaseError(f"Docker socket does not exist: {socket}")
    run(["docker", "run", "--rm", "--network", "host",
         "--mount", f"type=bind,src={socket},dst=/var/run/docker.sock",
         "--env", "DOCKER_HOST=unix:///var/run/docker.sock",
         "--env", "TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1",
         "--env", f"TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE={socket}",
         "--entrypoint", image["testCommand"][0], tag, *image["testCommand"][1:]])


def build_all(source, gatv2, images, *, registry, tag, platform, network, revision, version):
    # No registry mutation before every checks target and every runtime build succeeds.
    for image in images:
        check_tag = f"anomaly-release-checks/{image['name']}:{tag}"
        run(docker_build(source, image, check_tag, platform, network, checks=True, gatv2=gatv2))
        run_container_checks(image, check_tag)
    results = []
    for image in images:
        reference = f"{registry}/{image['repository']}:{tag}"
        command = docker_build(source, image, reference, platform, network, gatv2=gatv2)
        command[-1:-1] = ["--label", f"org.opencontainers.image.revision={revision}",
                          "--label", f"org.opencontainers.image.version={version}",
                          "--label", "org.opencontainers.image.source=https://github.com/dont-wait/anomaly"]
        run(command)
        info = inspect_image(reference)
        if f"{info['Os']}/{info['Architecture']}" != platform:
            raise ReleaseError(f"Unexpected platform for {reference}")
        results.append({"name": image["name"], "imageVariable": image["imageVariable"],
                        "tag": reference, "imageId": info["Id"], "platform": platform})
    # Test imports and bundled model against the actual runtime image, before any push.
    refs = {item["name"]: item["tag"] for item in results}
    run(["docker", "run", "--rm", "--entrypoint", "python", refs["kyc"],
         "-c", "from app.main import app; assert app is not None"])
    run(["docker", "run", "--rm", "--entrypoint", "python", refs["ml"], "-c",
         "import os,sys; sys.path.insert(0,os.environ['GATV2_SRC_PATH']); "
         "from fraud_pipeline.inference.predictor import ReplayPredictor; "
         "ReplayPredictor(os.environ['MODEL_BUNDLE_PATH'],device='cpu')"])
    return results


def publish_all(images):
    published = []
    for image in images:
        info = inspect_image(image["tag"])
        if info["Id"] != image["imageId"]:
            raise ReleaseError(f"Local image changed before push: {image['tag']}")
        run(["docker", "push", image["tag"]])
        repository = image["tag"].rsplit(":", 1)[0]
        digests = [value for value in inspect_image(image["tag"]).get("RepoDigests", [])
                   if value.startswith(repository + "@sha256:")]
        if len(digests) != 1 or not re.fullmatch(re.escape(repository) + r"@sha256:[0-9a-f]{64}", digests[0]):
            raise ReleaseError(f"Cannot identify pushed digest for {image['tag']}")
        published.append({**image, "reference": digests[0]})
    return published


def write_manifest(output, manifest, *, published):
    output.mkdir(parents=True, exist_ok=True)
    filename = "release-manifest.json" if published else "build-manifest.json"
    (output / filename).write_text(json.dumps(manifest, indent=2) + "\n")
    if published:
        content = "".join(f"{item['imageVariable']}={item['reference']}\n" for item in manifest["images"])
        (output / "images.env").write_text(content)
    print(f"Release metadata: {output / filename}")


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", type=Path, default=Path(__file__).resolve().parents[2])
    parser.add_argument("--version", default=os.environ.get("VERSION", "0.1.0"))
    parser.add_argument("--registry", default=os.environ.get("REGISTRY", "ghcr.io/dont-wait"))
    parser.add_argument("--gatv2-dir", type=Path, default=os.environ.get("GATV2_DIR"))
    parser.add_argument("--gatv2-ref", default=os.environ.get("GATV2_REF"))
    parser.add_argument("--network", choices=("default", "host"), default=os.environ.get("BUILD_NETWORK", "default"))
    parser.add_argument("--output", type=Path)
    parser.add_argument("--dry-run", action="store_true", help="Print plan without Docker/build/push")
    parser.add_argument("--build-only", action="store_true", help="Checks/build/smoke, without registry push")
    parser.add_argument("--check-only", action="store_true", help="Run Docker checks targets only")
    args = parser.parse_args(argv)
    repo = args.repo.resolve()
    if not re.fullmatch(r"\d+\.\d+\.\d+(?:-[A-Za-z0-9][A-Za-z0-9.-]*)?", args.version):
        raise ReleaseError("VERSION must be a semantic release version")
    if not re.fullmatch(r"[a-z0-9][a-z0-9.:-]*(?:/[a-z0-9][a-z0-9._-]*)+", args.registry):
        raise ReleaseError("REGISTRY must be a lowercase registry/namespace without a tag or scheme")
    revision = git(repo, "rev-parse", "HEAD")
    images = read_config(repo)
    version_paths = ["client/package.json", "client/mobile/package.json", "client/desktop/package.json",
                     "client/mobile/src-tauri/tauri.conf.json", "client/desktop/src-tauri/tauri.conf.json"]
    # Candidate suffix is image metadata; app source retains the base semantic version.
    version = args.version.split("-", 1)[0]
    for name in version_paths:
        if json.loads((repo / name).read_text())["version"] != version:
            raise ReleaseError(f"VERSION does not match {name}")
    for name in ("client/mobile/src-tauri/Cargo.toml", "client/desktop/src-tauri/Cargo.toml"):
        if tomllib.loads((repo / name).read_text())["package"]["version"] != version:
            raise ReleaseError(f"VERSION does not match {name}")
    for name in ("anomaly-service/pyproject.toml", "kyc-service/pyproject.toml"):
        if tomllib.loads((repo / name).read_text())["project"]["version"] != version:
            raise ReleaseError(f"VERSION does not match {name}")
    build_id = os.environ.get("RELEASE_BUILD_ID") or datetime.now(timezone.utc).strftime("%Y%m%d%H%M%S%f")
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_.-]{0,40}", build_id):
        raise ReleaseError("Invalid RELEASE_BUILD_ID")
    tag = f"{args.version}-{revision[:12]}-{build_id}"
    if len(tag) > 128:
        raise ReleaseError("Generated image tag is too long")
    gatv2_dir = (args.gatv2_dir or repo.parent / "GATv2").resolve()
    if not args.gatv2_ref:
        raise ReleaseError("Set GATV2_REF to the committed revision to package (working changes are never included)")
    gatv2_revision = git(gatv2_dir, "rev-parse", "--verify", args.gatv2_ref + "^{commit}")
    output = (args.output or repo / "dist/release" / tag).resolve()
    plan = {"version": args.version, "commit": revision, "gatv2Commit": gatv2_revision,
            "tag": tag, "platform": "linux/amd64", "registry": args.registry,
            "output": str(output), "images": images}
    if args.dry_run:
        print(json.dumps(plan, indent=2))
        return 0
    if git(repo, "status", "--porcelain", "--untracked-files=normal"):
        raise ReleaseError("Commit/stash source changes and untracked files before release; only committed snapshots are built")
    if (output / "release-manifest.json").exists():
        raise ReleaseError("This output already contains a published release; choose a new candidate/version")
    run(["docker", "info"], capture=True)
    run(["docker", "buildx", "version"])
    with tempfile.TemporaryDirectory(prefix="anomaly-release-") as temporary:
        work = Path(temporary)
        source, gatv2 = work / "anomaly", work / "gatv2"
        snapshot(repo, revision, source)
        snapshot(gatv2_dir, gatv2_revision, gatv2)
        model = bundle_checksums(source)
        if args.check_only:
            for image in images:
                check_tag = f"anomaly-release-checks/{image['name']}:{tag}"
                run(docker_build(source, image, check_tag, "linux/amd64", args.network,
                                 checks=True, gatv2=gatv2))
                run_container_checks(image, check_tag)
            return 0
        built = build_all(source, gatv2, images, registry=args.registry, tag=tag,
                          platform="linux/amd64", network=args.network, revision=revision, version=args.version)
        manifest = {"schemaVersion": 1, "version": args.version, "tag": tag,
                    "commit": revision, "gatv2Commit": gatv2_revision,
                    "buildId": build_id, "createdAt": datetime.now(timezone.utc).isoformat(),
                    "checks": "Docker checks targets + KYC import + bundled ML model load; full integration CI is separate",
                    "model": model, "images": built}
        write_manifest(output, manifest, published=False)
        if not args.build_only:
            manifest["images"] = publish_all(built)
            write_manifest(output, manifest, published=True)
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (ReleaseError, subprocess.CalledProcessError, OSError, ValueError, KeyError) as error:
        print(f"release: {error}", file=sys.stderr)
        raise SystemExit(1)
