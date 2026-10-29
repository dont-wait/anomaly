"""Regression tests for publishing order, provenance and failure handling."""
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import release


IMAGES = [
    {"name": name, "repository": "anomaly-" + name, "context": name,
     "dockerfile": name + "/Dockerfile", "checksTarget": "checks", "imageVariable": name.upper()}
    for name in ("server", "kyc", "ml", "autoscaler")
]


class ReleaseTests(unittest.TestCase):
    def test_testcontainers_failure_blocks_runtime_images(self):
        images = [{**IMAGES[0], "testCommand": ["go", "test", "./..."]}, *IMAGES[1:]]
        with patch.object(release, "run") as execute, \
             patch.object(release, "run_container_checks", side_effect=release.ReleaseError("tests failed")):
            with self.assertRaisesRegex(release.ReleaseError, "tests failed"):
                release.build_all(Path("/repo"), Path("/gatv2"), images, registry="ghcr.io/test",
                                  tag="0.1.0-test", platform="linux/amd64", network="default",
                                  revision="a" * 40, version="0.1.0")
        self.assertEqual(execute.call_count, 1)
        self.assertIn("--target", execute.call_args.args[0])

    def test_testcontainers_use_the_selected_local_docker_socket(self):
        with tempfile.TemporaryDirectory() as temporary:
            socket = Path(temporary) / "docker.sock"
            socket.touch()
            image = {**IMAGES[0], "testCommand": ["go", "test", "./..."]}
            with patch.dict(release.os.environ, {"DOCKER_HOST": f"unix://{socket}"}), \
                 patch.object(release, "run") as execute:
                release.run_container_checks(image, "checks:1")
            command = execute.call_args.args[0]
            self.assertIn(f"type=bind,src={socket},dst=/var/run/docker.sock", command)
            self.assertIn(f"TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE={socket}", command)
            self.assertEqual(command[-3:], ["checks:1", "test", "./..."])

    def test_checks_must_all_pass_before_runtime_builds(self):
        calls = []

        def execute(args, **kwargs):
            calls.append(args)
            if "kyc/Dockerfile" in str(args):
                raise subprocess.CalledProcessError(1, args)
            return ""

        with patch.object(release, "run", side_effect=execute):
            with self.assertRaises(subprocess.CalledProcessError):
                release.build_all(Path("/repo"), Path("/gatv2"), IMAGES, registry="ghcr.io/test",
                                  tag="0.1.0-test", platform="linux/amd64", network="default",
                                  revision="a" * 40, version="0.1.0")
        self.assertEqual(len(calls), 2)
        self.assertTrue(all("--target" in command for command in calls))
        self.assertFalse(any("push" in command for command in calls))

    def test_runtime_builds_and_smoke_happen_after_all_checks(self):
        calls = []
        with patch.object(release, "run", side_effect=lambda cmd, **kw: calls.append(cmd) or ""), \
             patch.object(release, "inspect_image", return_value={"Os": "linux", "Architecture": "amd64", "Id": "sha256:123"}):
            results = release.build_all(Path("/repo"), Path("/gatv2"), IMAGES, registry="ghcr.io/test",
                                        tag="0.1.0-test", platform="linux/amd64", network="default",
                                        revision="a" * 40, version="0.1.0")
        self.assertEqual(len(results), 4)
        self.assertTrue(all("--target" in command for command in calls[:4]))
        self.assertTrue(all("--target" not in command for command in calls[4:8]))
        self.assertTrue(all(command[1] == "run" for command in calls[8:]))
        self.assertFalse(any("push" in command for command in calls))

    def test_push_refuses_local_image_changed_after_build(self):
        with patch.object(release, "inspect_image", return_value={"Id": "changed"}), \
             patch.object(release, "run") as execute:
            with self.assertRaisesRegex(release.ReleaseError, "changed"):
                release.publish_all([{"tag": "ghcr.io/test/server:1", "imageId": "original"}])
            execute.assert_not_called()

    def test_push_uses_matching_registry_digest(self):
        expected = "ghcr.io/test/server@sha256:" + "a" * 64
        before = {"Id": "original"}
        after = {"Id": "original", "RepoDigests": ["other/server@sha256:" + "b" * 64, expected]}
        with patch.object(release, "inspect_image", side_effect=[before, after]), \
             patch.object(release, "run") as execute:
            result = release.publish_all([{"tag": "ghcr.io/test/server:1", "imageId": "original"}])
        self.assertEqual(result[0]["reference"], expected)
        execute.assert_called_once_with(["docker", "push", "ghcr.io/test/server:1"])

    def test_missing_pushed_digest_is_failure(self):
        with patch.object(release, "inspect_image", return_value={"Id": "original", "RepoDigests": []}), \
             patch.object(release, "run"):
            with self.assertRaisesRegex(release.ReleaseError, "digest"):
                release.publish_all([{"tag": "ghcr.io/test/server:1", "imageId": "original"}])

    def test_unpublished_build_does_not_generate_deploy_env(self):
        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary)
            release.write_manifest(output, {"images": []}, published=False)
            self.assertTrue((output / "build-manifest.json").exists())
            self.assertFalse((output / "release-manifest.json").exists())
            self.assertFalse((output / "images.env").exists())

    def test_published_env_uses_digests_not_mutable_tags(self):
        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary)
            digest = "ghcr.io/test/server@sha256:" + "a" * 64
            release.write_manifest(output, {"images": [{"imageVariable": "SERVER_IMAGE", "reference": digest}]}, published=True)
            self.assertEqual((output / "images.env").read_text(), f"SERVER_IMAGE={digest}\n")

    def test_gatv2_context_is_explicit_without_parent_build_context(self):
        image = {**IMAGES[2], "additionalContext": "gatv2"}
        command = release.docker_build(Path("/repo"), image, "test:1", "linux/amd64", "host", gatv2=Path("/pinned/gatv2"))
        self.assertIn("gatv2=/pinned/gatv2", command)
        self.assertIn("network.host", command)
        self.assertEqual(command[-1], Path("/repo/ml"))

    def test_bundle_tampering_fails_before_build(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            directory = root / "anomaly-service/models/gatv2-1m-baseline-v1"
            directory.mkdir(parents=True)
            (directory / "bundle.json").write_text(json.dumps({"files": {"model.pt": hashlib.sha256(b"good").hexdigest()}}))
            (directory / "model.pt").write_bytes(b"tampered")
            with self.assertRaisesRegex(release.ReleaseError, "checksum"):
                release.bundle_checksums(root)

    def test_source_snapshot_excludes_uncommitted_files_and_changes(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            repo = directory / "repo"
            repo.mkdir()
            subprocess.run(["git", "init", "-q", str(repo)], check=True)
            subprocess.run(["git", "-C", str(repo), "config", "user.email", "test@example.com"], check=True)
            subprocess.run(["git", "-C", str(repo), "config", "user.name", "Release test"], check=True)
            (repo / "source.txt").write_text("committed")
            subprocess.run(["git", "-C", str(repo), "add", "."], check=True)
            subprocess.run(["git", "-C", str(repo), "commit", "-qm", "fixture"], check=True)
            (repo / "source.txt").write_text("dirty")
            (repo / "untracked-secret.env").write_text("never-package")
            output = directory / "snapshot"
            release.snapshot(repo, "HEAD", output)
            self.assertEqual((output / "source.txt").read_text(), "committed")
            self.assertFalse((output / "untracked-secret.env").exists())


if __name__ == "__main__":
    unittest.main()
