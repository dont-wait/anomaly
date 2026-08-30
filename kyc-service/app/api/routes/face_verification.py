from fastapi import APIRouter, Depends, File, Form, HTTPException, UploadFile, status

from app.api.deps import get_vision_pipeline
from app.core.config import get_settings
from app.files import read_limited_bytes
from app.schemas.face_verification import (
    ExtractIdFaceResponse,
    MatchFaceResponse,
    VerifyFaceResponse,
)
from app.services.pipeline import VisionPipeline

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
