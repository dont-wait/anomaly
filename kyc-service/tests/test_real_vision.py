"""Policy/temporal/decoder tests; these do not certify biometric accuracy."""

import hashlib
import json
import shutil
import subprocess
import threading
import time
import tomllib
from pathlib import Path
from types import SimpleNamespace

import numpy as np
import pytest
from pydantic import ValidationError

from app.core.config import Settings
from app.services.embedding_model import similarity
from app.services.face_matcher import VideoEvidence
from app.services.liveness import Challenge
from app.services.pipeline import VisionPipelineConfigurationError, build_vision_pipeline
from app.services.video_processor import VideoDecoder, analyze_file
from app.services.vision_config import VisionConfig, load_config


@pytest.fixture
def policy():
    data = tomllib.loads(Path("config/vision.example.toml").read_text())
    data["calibrated"] = True
    return VisionConfig.model_validate(data)


def test_uncalibrated_example_cannot_enable_real_verification():
    data = tomllib.loads(Path("config/vision.example.toml").read_text())
    with pytest.raises(ValidationError, match="calibrated"):
        VisionConfig.model_validate(data)


def test_stub_is_forbidden_in_production_even_with_override():
    with pytest.raises(VisionPipelineConfigurationError):
        build_vision_pipeline(
            Settings(
                APP_ENV="production",
                VISION_PIPELINE_BACKEND="stub",
                ALLOW_STUB_VISION_PIPELINE=True,
            )
        )


def test_missing_artifacts_fail_closed():
    with pytest.raises(VisionPipelineConfigurationError):
        build_vision_pipeline(
            Settings(VISION_PIPELINE_BACKEND="real", VISION_CONFIG_PATH="does-not-exist.toml")
        )


def test_corrupted_weights_rejected_before_model_loading(tmp_path, policy):
    import app.services.vision_config as configuration

    manifest = json.loads(Path("config/manifest.example.json").read_text())
    manifest["sha256"] = "a" * 64
    manifest["landmarker_sha256"] = "b" * 64
    (tmp_path / "manifest.json").write_text(json.dumps(manifest))
    (tmp_path / "face.onnx").write_bytes(b"wrong weights")
    assert hashlib.sha256(b"wrong weights").hexdigest() != manifest["sha256"]
    with pytest.MonkeyPatch.context() as patch:
        patch.setattr(configuration.tomllib, "loads", lambda _: policy.model_dump())
        (tmp_path / "vision.toml").write_text("configuration")
        with pytest.raises(ValueError, match="checksum"):
            load_config(str(tmp_path / "vision.toml"), str(tmp_path / "manifest.json"))


def poses(challenge):
    # Continuous, multi-frame holds, in the order requested by the server.
    now = 0.0
    for yaw in [0, -25, 25]:
        for _ in range(8):
            challenge.observe(now, yaw, 0.0)
            now += 0.067
    return now


def test_liveness_requires_real_ordered_poses_and_multiframe_blink(policy):
    challenge = Challenge(policy)
    now = poses(challenge)
    assert challenge.step == 3 and not challenge.passed
    for index, blink in enumerate([0.0, 0.9, 0.9, 0.0]):
        challenge.observe(now + index * 0.067, 0, blink)
    assert challenge.passed


def test_one_closed_eye_frame_and_static_face_do_not_pass(policy):
    challenge = Challenge(policy)
    for i in range(180):
        challenge.observe(i / 15, 0, 0.0)
    assert not challenge.passed
    challenge = Challenge(policy)
    now = poses(challenge)
    for index, blink in enumerate([0.0, 0.9, 0.0]):
        challenge.observe(now + index * 0.067, 0, blink)
    assert not challenge.passed


def test_missing_timeline_coverage_invalidates_completed_challenge(policy):
    challenge = Challenge(policy)
    now = poses(challenge)
    for index, blink in enumerate([0.0, 0.9, 0.9, 0.0]):
        challenge.observe(now + index * 0.067, 0, blink)
    assert challenge.passed
    challenge.observe(now + 2, 0, 0)
    assert not challenge.passed


def test_matching_uses_distribution_instead_of_one_matching_frame(policy):
    evidence = VideoEvidence(SimpleNamespace(policy=policy), np.array([1.0, 0.0]))
    evidence.scores = [0.99, 0.1, 0.2, 0.15, 0.1]
    evidence.times = [0, 3.4, 6.8, 9, 11]
    assert not evidence.matched
    evidence.scores = [0.95] * 5
    assert evidence.matched
    evidence.times = [0, 0.2, 0.4, 0.6, 0.8]
    assert not evidence.matched
    assert similarity(np.array([1.0, 0.0]), np.array([1.0, 0.0])) == 1
    assert similarity(np.array([1.0, 0.0]), np.array([-1.0, 0.0])) == 0


def test_repeated_decoder_frames_cannot_hide_missing_liveness_coverage(policy):
    model = SimpleNamespace(policy=policy, detect=lambda _: SimpleNamespace(face_landmarks=[]))
    evidence = VideoEvidence(model)
    frame = np.zeros((480, 640, 3), dtype=np.uint8)
    for i in range(20):
        evidence.observe(frame, i / 15)
    assert evidence.challenge.invalid_gap


def test_incremental_webm_decoder_emits_frames_before_end():
    executable = shutil.which("ffmpeg")
    if not executable:
        pytest.skip("ffmpeg required")
    data = subprocess.check_output(
        [
            executable,
            "-hide_banner",
            "-loglevel",
            "error",
            "-f",
            "lavfi",
            "-i",
            "testsrc=size=320x240:rate=30",
            "-t",
            "3",
            "-an",
            "-c:v",
            "libvpx",
            "-threads",
            "1",
            "-deadline",
            "realtime",
            "-g",
            "15",
            "-cluster_time_limit",
            "200",
            "-f",
            "webm",
            "pipe:1",
        ],
        timeout=15,
    )
    decoder = VideoDecoder(executable, timeout=15)
    observed = threading.Event()
    frames, errors = [], []

    def consume():
        try:
            for frame, timestamp in decoder.frames():
                frames.append((frame.shape, timestamp))
                observed.set()
        except Exception as exc:
            errors.append(exc)

    worker = threading.Thread(target=consume, daemon=True)
    worker.start()
    try:
        observed_before_end = False
        for offset in range(0, len(data), 1024):
            decoder.write(data[offset : offset + 1024])
            if (
                not observed_before_end
                and offset >= len(data) // 3
                and offset + 1024 < len(data)
            ):
                observed_before_end = observed.wait(2)
            time.sleep(0.01)
        assert observed_before_end, "decoder waited for end-of-file instead of streaming"
        decoder.finish()
        worker.join(5)
        assert not worker.is_alive() and not errors
        assert 40 <= len(frames) <= 46
        assert frames[0][0] == (480, 640, 3)
        assert frames[-1][1] > 2.5
    finally:
        decoder.close()
        worker.join(2)


def test_realtime_socket_requires_service_auth_and_streams_decoder_progress(monkeypatch, policy):
    import base64

    from fastapi.testclient import TestClient
    from starlette.websockets import WebSocketDisconnect

    from app.api.routes import realtime
    from app.main import app
    from app.services import document_processor
    from app.services.real_pipeline import RealVisionPipeline

    executable = shutil.which("ffmpeg")
    if not executable:
        pytest.skip("ffmpeg required")
    pipeline = RealVisionPipeline.__new__(RealVisionPipeline)
    pipeline.policy = policy
    pipeline.manifest = SimpleNamespace(model_id="test", model_version="test", sha256="a" * 64)
    pipeline.slots = threading.BoundedSemaphore(1)
    pipeline.model = SimpleNamespace(
        policy=policy, detect=lambda _: SimpleNamespace(face_landmarks=[])
    )
    # Inference is explicitly replaced only for this transport test, never in runtime real mode.
    monkeypatch.setattr(document_processor, "reference_face", lambda *_: np.array([1.0, 0.0]))
    monkeypatch.setattr(realtime, "get_vision_pipeline", lambda: pipeline)
    monkeypatch.setattr(
        realtime, "get_settings", lambda: Settings(KYC_SERVICE_TOKEN="internal-secret")
    )
    data = subprocess.check_output(
        [
            executable,
            "-hide_banner",
            "-loglevel",
            "error",
            "-f",
            "lavfi",
            "-i",
            "testsrc=size=320x240:rate=30",
            "-t",
            "3",
            "-an",
            "-c:v",
            "libvpx",
            "-threads",
            "1",
            "-deadline",
            "realtime",
            "-g",
            "15",
            "-cluster_time_limit",
            "200",
            "-f",
            "webm",
            "pipe:1",
        ],
        timeout=15,
    )
    client = TestClient(app)
    with pytest.raises(WebSocketDisconnect):
        with client.websocket_connect("/v1/kyc/realtime"):
            pass
    with client.websocket_connect(
        "/v1/kyc/realtime", headers={"X-KYC-Service-Token": "internal-secret"}
    ) as socket:
        socket.send_json(
            {
                "front": base64.b64encode(b"test fixture").decode(),
                "challenge_type": "TURN_HEAD_LEFT_RIGHT_BLINK",
            }
        )
        assert socket.receive_json()["type"] == "ready"
        socket.send_json({"type": "start"})
        last = -1
        for last, offset in enumerate(range(0, len(data), 1024)):
            socket.send_bytes(bytes([1]) + last.to_bytes(4, "big") + data[offset : offset + 1024])
        progress = socket.receive_json()
        assert progress["type"] == "progress"
        assert progress["processedMediaTimeMs"] > 0
        socket.send_json({"type": "end", "lastChunkSeq": last})
        while True:
            event = socket.receive_json()
            assert event["type"] != "error"
            if event["type"] == "stream_finished":
                assert event["result"]["success"] is False
                break


def test_final_verification_decodes_mp4_with_index_at_end_and_removes_temp_file(
    tmp_path, monkeypatch
):
    import tempfile

    executable = shutil.which("ffmpeg")
    if not executable:
        pytest.skip("ffmpeg required")
    source = tmp_path / "camera.mp4"
    subprocess.run(
        [
            executable,
            "-hide_banner",
            "-loglevel",
            "error",
            "-f",
            "lavfi",
            "-i",
            "testsrc=size=320x240:rate=30",
            "-t",
            "3",
            "-an",
            "-c:v",
            "libx264",
            "-threads",
            "1",
            "-pix_fmt",
            "yuv420p",
            str(source),
        ],
        check=True,
        timeout=15,
    )
    data = source.read_bytes()
    assert data.index(b"moov") > data.index(b"mdat")
    monkeypatch.setattr(tempfile, "tempdir", str(tmp_path))
    observations = []
    evidence = SimpleNamespace(
        model=SimpleNamespace(policy=SimpleNamespace(ffmpeg=executable)),
        observe=lambda rgb, timestamp: observations.append(timestamp),
    )
    analyze_file(data, evidence)
    assert len(observations) >= 40 and observations[-1] > 2.5
    assert not list(tmp_path.glob("anomaly-kyc-*.media"))
