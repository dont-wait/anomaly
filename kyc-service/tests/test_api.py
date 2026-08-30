from io import BytesIO

from fastapi.testclient import TestClient

from app.constants.endpoints import ENDPOINTS
from app.main import app
from app.services.pipeline import StubVisionPipeline

client = TestClient(app)


def test_healthcheck() -> None:
    response = client.get(ENDPOINTS["health"])

    assert response.status_code == 200
    assert response.json() == {"status": "ok"}


def test_swagger_docs_available() -> None:
    response = client.get(ENDPOINTS["docs"])

    assert response.status_code == 200
    assert "Swagger UI" in response.text


def test_openapi_available() -> None:
    response = client.get(ENDPOINTS["openapi"])

    assert response.status_code == 200
    assert response.json()["info"]["title"] == "Anomaly Python Service"


def test_extract_id_face_success_with_minio_object(object_bytes) -> None:
    payload, filename, content_type = object_bytes("cccd_ok")

    response = client.post(
        f"{ENDPOINTS['kyc_prefix']}{ENDPOINTS['extract_id_face']}",
        files={"cccd_front_image": (filename, BytesIO(payload), content_type)},
    )

    assert response.status_code == 200
    assert response.json()["success"] is True
    assert response.json()["portrait_extracted"] is True


def test_verify_face_success_with_minio_objects(object_bytes) -> None:
    image_payload, image_name, image_type = object_bytes("cccd_ok")
    video_payload, video_name, video_type = object_bytes("live_match")

    response = client.post(
        f"{ENDPOINTS['kyc_prefix']}{ENDPOINTS['verify_face']}",
        data={"challenge_type": "TURN_HEAD_LEFT_RIGHT_BLINK"},
        files={
            "cccd_front_image": (image_name, BytesIO(image_payload), image_type),
            "live_video": (video_name, BytesIO(video_payload), video_type),
        },
    )

    assert response.status_code == 200
    assert response.json()["success"] is True
    assert response.json()["decision"] == "VERIFIED"


def test_verify_face_returns_reason_message_on_mismatch(object_bytes) -> None:
    image_payload, image_name, image_type = object_bytes("cccd_ok")
    video_payload, video_name, video_type = object_bytes("live_mismatch")

    response = client.post(
        f"{ENDPOINTS['kyc_prefix']}{ENDPOINTS['verify_face']}",
        data={"challenge_type": "TURN_HEAD_LEFT_RIGHT_BLINK"},
        files={
            "cccd_front_image": (image_name, BytesIO(image_payload), image_type),
            "live_video": (video_name, BytesIO(video_payload), video_type),
        },
    )

    assert response.status_code == 200
    assert response.json()["success"] is False
    assert response.json()["reason_code"] == "FACE_MISMATCH"
    assert response.json()["reason_message"]


def test_run_liveness_returns_retry_allowed_on_low_confidence(object_bytes) -> None:
    video_payload, video_name, video_type = object_bytes("live_low")

    response = client.post(
        f"{ENDPOINTS['kyc_prefix']}{ENDPOINTS['run_liveness']}",
        data={"challenge_type": "TURN_HEAD_LEFT_RIGHT_BLINK"},
        files={"live_video": (video_name, BytesIO(video_payload), video_type)},
    )

    assert response.status_code == 200
    assert response.json()["success"] is False
    assert response.json()["decision"] == "RETRY_ALLOWED"
    assert response.json()["reason_code"] == "LIVENESS_NOT_CONFIDENT"


def test_extract_id_face_rejects_oversized_image() -> None:
    oversized_payload = b"a" * (10 * 1024 * 1024 + 1)

    response = client.post(
        f"{ENDPOINTS['kyc_prefix']}{ENDPOINTS['extract_id_face']}",
        files={"cccd_front_image": ("oversized.jpg", BytesIO(oversized_payload), "image/jpeg")},
    )

    assert response.status_code == 413
    assert response.json()["detail"] == "cccd_front_image_too_large"


def test_extract_id_face_rejects_oversized_multipart_body_before_parsing() -> None:
    boundary = "boundary"
    oversized_body = (
        (
            f"--{boundary}\r\n"
            'Content-Disposition: form-data; name="cccd_front_image"; filename="oversized.jpg"\r\n'
            "Content-Type: image/jpeg\r\n\r\n"
        ).encode()
        + (b"a" * (11 * 1024 * 1024))
        + f"\r\n--{boundary}--\r\n".encode()
    )

    response = client.post(
        f"{ENDPOINTS['kyc_prefix']}{ENDPOINTS['extract_id_face']}",
        content=oversized_body,
        headers={"Content-Type": f"multipart/form-data; boundary={boundary}"},
    )

    assert response.status_code == 413
    assert response.json()["detail"] == "payload_too_large"


def test_run_liveness_rejects_oversized_video() -> None:
    oversized_payload = b"a" * (25 * 1024 * 1024 + 1)

    response = client.post(
        f"{ENDPOINTS['kyc_prefix']}{ENDPOINTS['run_liveness']}",
        data={"challenge_type": "TURN_HEAD_LEFT_RIGHT_BLINK"},
        files={"live_video": ("oversized.mp4", BytesIO(oversized_payload), "video/mp4")},
    )

    assert response.status_code == 413
    assert response.json()["detail"] == "live_video_too_large"


def test_stub_pipeline_can_be_reused_directly() -> None:
    pipeline = StubVisionPipeline()

    assert pipeline is not None
