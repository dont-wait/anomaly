from __future__ import annotations

import os
import shutil
import socket
import subprocess
import time
from collections.abc import Iterator
from contextlib import closing
from dataclasses import dataclass
from io import BytesIO

import boto3
import pytest
from botocore.client import BaseClient


def _free_port() -> int:
    with closing(socket.socket(socket.AF_INET, socket.SOCK_STREAM)) as sock:
        sock.bind(("127.0.0.1", 0))
        sock.listen(1)
        return int(sock.getsockname()[1])


def _wait_for_object_store(client: BaseClient, timeout_seconds: float = 15.0) -> None:
    deadline = time.time() + timeout_seconds
    last_error: Exception | None = None
    while time.time() < deadline:
        try:
            client.list_buckets()
            return
        except Exception as exc:  # pragma: no cover - readiness loop
            last_error = exc
            time.sleep(0.2)
    raise RuntimeError("object storage server did not become ready") from last_error


@dataclass(slots=True)
class TestObject:
    bucket: str
    name: str
    content_type: str
    payload: bytes


@dataclass(slots=True)
class RustFSTestStore:
    client: BaseClient
    bucket: str
    objects: dict[str, TestObject]

    def get(self, name: str) -> TestObject:
        return self.objects[name]


@pytest.fixture(scope="session")
def rustfs_server() -> Iterator[RustFSTestStore]:
    if shutil.which("docker") is None:
        pytest.skip("docker is required for RustFS-backed tests")

    api_port = _free_port()
    console_port = _free_port()
    endpoint = f"127.0.0.1:{api_port}"
    access_key = os.environ.get("RUSTFS_ROOT_USER", "rustfsadmin")
    secret_key = os.environ.get("RUSTFS_ROOT_PASSWORD", "rustfsadmin")
    container_name = f"anomaly-python-service-rustfs-{api_port}"
    image = os.environ.get("RUSTFS_DOCKER_IMAGE", "rustfs/rustfs:latest")
    process = subprocess.Popen(
        [
            "docker",
            "run",
            "--rm",
            "--name",
            container_name,
            "-p",
            f"{api_port}:9000",
            "-p",
            f"{console_port}:9001",
            image,
        ],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )

    # RustFS exposes an S3-compatible API, so the test uses boto3 as the
    # generic S3 client for bucket and object operations.
    client = boto3.client(
        "s3",
        endpoint_url=f"http://{endpoint}",
        aws_access_key_id=access_key,
        aws_secret_access_key=secret_key,
        region_name="us-east-1",
    )

    try:
        _wait_for_object_store(client)
        bucket = "python-service-test-data"
        existing_buckets = {
            item["Name"]
            for item in client.list_buckets().get("Buckets", [])
            if "Name" in item
        }
        if bucket not in existing_buckets:
            client.create_bucket(Bucket=bucket)

        objects = {
            "cccd_ok": TestObject(
                bucket=bucket,
                name="cccd-match.jpg",
                content_type="image/jpeg",
                payload=b"fake-jpeg-data",
            ),
            "cccd_bad": TestObject(
                bucket=bucket,
                name="cccd-low.jpg",
                content_type="image/jpeg",
                payload=b"fake-jpeg-data",
            ),
            "live_match": TestObject(
                bucket=bucket,
                name="live-match-pass.mp4",
                content_type="video/mp4",
                payload=b"fake-mp4-data",
            ),
            "live_mismatch": TestObject(
                bucket=bucket,
                name="live-mismatch-fail.mp4",
                content_type="video/mp4",
                payload=b"fake-mp4-data",
            ),
            "live_low": TestObject(
                bucket=bucket,
                name="live-low-fail.mp4",
                content_type="video/mp4",
                payload=b"fake-mp4-data",
            ),
        }

        for test_object in objects.values():
            client.put_object(
                Bucket=test_object.bucket,
                Key=test_object.name,
                Body=BytesIO(test_object.payload).read(),
                ContentType=test_object.content_type,
            )

        yield RustFSTestStore(client=client, bucket=bucket, objects=objects)
    finally:
        try:
            existing_buckets = {
                item["Name"]
                for item in client.list_buckets().get("Buckets", [])
                if "Name" in item
            }
            if "python-service-test-data" in existing_buckets:
                listed = client.list_objects_v2(Bucket="python-service-test-data")
                contents = listed.get("Contents", [])
                if contents:
                    client.delete_objects(
                        Bucket="python-service-test-data",
                        Delete={
                            "Objects": [{"Key": item["Key"]} for item in contents],
                            "Quiet": True,
                        },
                    )
                client.delete_bucket(Bucket="python-service-test-data")
        finally:
            subprocess.run(
                ["docker", "stop", container_name],
                check=False,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
            )
            process.wait(timeout=10)


@pytest.fixture()
def object_bytes(rustfs_server: RustFSTestStore):
    def _fetch(name: str) -> tuple[bytes, str, str]:
        test_object = rustfs_server.get(name)
        response = rustfs_server.client.get_object(Bucket=test_object.bucket, Key=test_object.name)
        payload = response["Body"].read()
        return payload, test_object.name, test_object.content_type

    return _fetch
