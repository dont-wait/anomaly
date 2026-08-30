from collections import deque
from collections.abc import Callable

from fastapi import APIRouter, Depends, File, Form, HTTPException, UploadFile, status
from fastapi.responses import JSONResponse
from starlette.types import ASGIApp, Message, Receive, Scope, Send

from app.api.deps import get_vision_pipeline
from app.constants.endpoints import ENDPOINTS
from app.core.config import Settings, get_settings
from app.files import read_limited_bytes
from app.schemas.face_verification import (
    ExtractIdFaceResponse,
    MatchFaceResponse,
    VerifyFaceResponse,
)
from app.services.pipeline import VisionPipeline

MULTIPART_OVERHEAD_BYTES = 1024 * 1024


def _extract_id_face_request_limit(settings: Settings) -> int:
    return settings.max_image_bytes + MULTIPART_OVERHEAD_BYTES


def _run_liveness_request_limit(settings: Settings) -> int:
    return settings.max_video_bytes + MULTIPART_OVERHEAD_BYTES


def _match_face_request_limit(settings: Settings) -> int:
    return settings.max_image_bytes + settings.max_video_bytes + MULTIPART_OVERHEAD_BYTES


_REQUEST_BODY_LIMITS: dict[str, Callable[[Settings], int]] = {
    f"{ENDPOINTS['kyc_prefix']}{ENDPOINTS['extract_id_face']}": _extract_id_face_request_limit,
    f"{ENDPOINTS['kyc_prefix']}{ENDPOINTS['run_liveness']}": _run_liveness_request_limit,
    f"{ENDPOINTS['kyc_prefix']}{ENDPOINTS['match_face']}": _match_face_request_limit,
    f"{ENDPOINTS['kyc_prefix']}{ENDPOINTS['verify_face']}": _match_face_request_limit,
}


class MultipartBodyLimitMiddleware:
    def __init__(self, app: ASGIApp):
        self.app = app

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return

        limit_factory = _REQUEST_BODY_LIMITS.get(scope["path"])
        if limit_factory is None:
            await self.app(scope, receive, send)
            return

        limit_bytes = limit_factory(get_settings())
        buffered_messages: deque[Message] = deque()
        received_bytes = 0

        while True:
            message = await receive()
            buffered_messages.append(message)

            if message["type"] != "http.request":
                break

            received_bytes += len(message.get("body", b""))
            if received_bytes > limit_bytes:
                response = JSONResponse(
                    status_code=status.HTTP_413_CONTENT_TOO_LARGE,
                    content={"detail": "payload_too_large"},
                )
                await response(scope, receive, send)
                return

            if not message.get("more_body", False):
                break

        async def replay_receive() -> Message:
            if buffered_messages:
                return buffered_messages.popleft()
            return {"type": "http.request", "body": b"", "more_body": False}

        await self.app(scope, replay_receive, send)


router = APIRouter(tags=["face-verification"])


async def _ensure_within_limit(
    upload_file: UploadFile,
    max_bytes: int,
    detail: str,
) -> None:
    _, overflowed = await read_limited_bytes(upload_file, max_bytes)
    if overflowed:
        raise HTTPException(status_code=status.HTTP_413_CONTENT_TOO_LARGE, detail=detail)


@router.post("/extract-id-face", response_model=ExtractIdFaceResponse)
async def extract_id_face(
    cccd_front_image: UploadFile = File(...),
    pipeline: VisionPipeline = Depends(get_vision_pipeline),
) -> ExtractIdFaceResponse:
    settings = get_settings()
    await _ensure_within_limit(
        cccd_front_image,
        settings.max_image_bytes,
        "cccd_front_image_too_large",
    )
    return await pipeline.extract_id_face(cccd_front_image)


@router.post("/run-liveness", response_model=VerifyFaceResponse)
async def run_liveness(
    live_video: UploadFile = File(...),
    challenge_type: str = Form(...),
    pipeline: VisionPipeline = Depends(get_vision_pipeline),
) -> VerifyFaceResponse:
    settings = get_settings()
    await _ensure_within_limit(
        live_video,
        settings.max_video_bytes,
        "live_video_too_large",
    )
    return await pipeline.run_liveness(live_video=live_video, challenge_type=challenge_type)


@router.post("/match-face", response_model=MatchFaceResponse)
async def match_face(
    cccd_front_image: UploadFile = File(...),
    live_video: UploadFile = File(...),
    pipeline: VisionPipeline = Depends(get_vision_pipeline),
) -> MatchFaceResponse:
    settings = get_settings()
    await _ensure_within_limit(
        cccd_front_image,
        settings.max_image_bytes,
        "cccd_front_image_too_large",
    )
    await _ensure_within_limit(
        live_video,
        settings.max_video_bytes,
        "live_video_too_large",
    )
    return await pipeline.match_face(cccd_front_image=cccd_front_image, live_video=live_video)


@router.post("/verify-face", response_model=VerifyFaceResponse)
async def verify_face(
    cccd_front_image: UploadFile = File(...),
    live_video: UploadFile = File(...),
    challenge_type: str = Form(...),
    pipeline: VisionPipeline = Depends(get_vision_pipeline),
) -> VerifyFaceResponse:
    settings = get_settings()
    await _ensure_within_limit(
        cccd_front_image,
        settings.max_image_bytes,
        "cccd_front_image_too_large",
    )
    await _ensure_within_limit(
        live_video,
        settings.max_video_bytes,
        "live_video_too_large",
    )
    return await pipeline.verify_face(
        cccd_front_image=cccd_front_image,
        live_video=live_video,
        challenge_type=challenge_type,
    )
