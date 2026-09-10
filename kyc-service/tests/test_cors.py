from fastapi.testclient import TestClient

from app.main import create_app


def test_client_multipart_preflight_is_allowed() -> None:
    client = TestClient(create_app())
    response = client.options(
        "/v1/kyc/verify-face",
        headers={
            "Origin": "http://localhost:1420",
            "Access-Control-Request-Method": "POST",
            "Access-Control-Request-Headers": "content-type",
        },
    )
    assert response.status_code == 200
    assert response.headers["access-control-allow-origin"] == "http://localhost:1420"


def test_unknown_origin_is_not_allowed() -> None:
    client = TestClient(create_app())
    response = client.options(
        "/v1/kyc/verify-face",
        headers={"Origin": "https://untrusted.example", "Access-Control-Request-Method": "POST"},
    )
    assert response.status_code == 400
    assert "access-control-allow-origin" not in response.headers
