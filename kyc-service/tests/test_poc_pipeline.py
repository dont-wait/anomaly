import base64

import pytest
from fastapi.testclient import TestClient
from starlette.websockets import WebSocketDisconnect

from app.api.deps import get_vision_pipeline
from app.core.config import Settings, get_settings
from app.main import create_app
from app.services.pipeline import VisionPipelineConfigurationError, build_vision_pipeline


@pytest.mark.parametrize("environment,enabled", [("production", True), ("development", False)])
def test_poc_requires_explicit_development_configuration(environment, enabled):
    with pytest.raises(VisionPipelineConfigurationError):
        build_vision_pipeline(
            Settings(APP_ENV=environment, VISION_PIPELINE_BACKEND="poc", KYC_POC_BYPASS=enabled)
        )


@pytest.fixture
def client(monkeypatch):
    monkeypatch.setenv("APP_ENV", "development")
    monkeypatch.setenv("VISION_PIPELINE_BACKEND", "poc")
    monkeypatch.setenv("KYC_POC_BYPASS", "true")
    monkeypatch.setenv("KYC_SERVICE_TOKEN", "poc-test-token")
    get_settings.cache_clear()
    get_vision_pipeline.cache_clear()
    try:
        with TestClient(create_app()) as client:
            yield client
    finally:
        get_settings.cache_clear()
        get_vision_pipeline.cache_clear()


def test_poc_realtime_and_completion_without_models(client):
    assert client.get("/ready").json()["backend"] == "PocVisionPipeline"
    with pytest.raises(WebSocketDisconnect):
        with client.websocket_connect("/v1/kyc/realtime"):
            pass
    with client.websocket_connect(
        "/v1/kyc/realtime", headers={"X-KYC-Service-Token": "poc-test-token"}
    ) as socket:
        socket.send_json(
            {
                "front": base64.b64encode(b"POC image").decode(),
                "challenge_type": "TURN_HEAD_LEFT_RIGHT_BLINK",
            }
        )
        assert socket.receive_json()["type"] == "ready"
        socket.send_json({"type": "start"})
        socket.send_bytes(b"\x01\x00\x00\x00\x00demo-video")
        progress = socket.receive_json()
        assert progress["type"] == "progress"
        assert progress["matchingState"] == "not_run"
        socket.send_json({"type": "end", "lastChunkSeq": 0})
        assert socket.receive_json()["type"] == "stream_finished"
    files = {
        "cccd_front_image": ("front.png", b"POC image", "image/png"),
        "live_video": ("live.webm", b"demo-video", "video/webm"),
    }
    payload = {"challenge_type": "TURN_HEAD_LEFT_RIGHT_BLINK"}
    assert client.post("/v1/kyc/verify-face", files=files, data=payload).status_code == 401
    response = client.post(
        "/v1/kyc/verify-face",
        files=files,
        data=payload,
        headers={"X-KYC-Service-Token": "poc-test-token", "X-Request-ID": "poc-request"},
    )
    assert response.status_code == 200
    result = response.json()
    assert result["poc_bypass"] and result["success"] and result["decision"] == "VERIFIED"
    assert result["analysis"]["request_id"] == "poc-request"
    assert result["analysis"]["model_id"] == "poc-bypass"
    assert result["analysis"]["matched_frames"] == 0
    assert result["analysis"]["matching_status"] == "not_run"
    assert result["match_score"] == 0


@pytest.mark.parametrize(
    "chunk,end", [(b"\x01\x00\x00\x00\x01x", None), (None, {"type": "end", "lastChunkSeq": -1})]
)
def test_poc_rejects_out_of_order_or_empty_stream(client, chunk, end):
    with client.websocket_connect(
        "/v1/kyc/realtime", headers={"X-KYC-Service-Token": "poc-test-token"}
    ) as socket:
        socket.send_json(
            {
                "front": base64.b64encode(b"image").decode(),
                "challenge_type": "TURN_HEAD_LEFT_RIGHT_BLINK",
            }
        )
        assert socket.receive_json()["type"] == "ready"
        socket.send_json({"type": "start"})
        if chunk:
            socket.send_bytes(chunk)
        else:
            socket.send_json(end)
        assert socket.receive_json()["type"] == "error"
