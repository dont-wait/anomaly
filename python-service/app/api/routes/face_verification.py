from fastapi import APIRouter, Depends, File, Form, UploadFile

from app.api.deps import get_vision_pipeline
from app.schemas.face_verification import (
    ExtractIdFaceResponse,
    MatchFaceResponse,
    VerifyFaceResponse,
)
from app.services.pipeline import VisionPipeline

router = APIRouter(tags=["face-verification"])


@router.post("/extract-id-face", response_model=ExtractIdFaceResponse)
async def extract_id_face(
    cccd_front_image: UploadFile = File(...),
    pipeline: VisionPipeline = Depends(get_vision_pipeline),
) -> ExtractIdFaceResponse:
    return await pipeline.extract_id_face(cccd_front_image)


@router.post("/run-liveness", response_model=VerifyFaceResponse)
async def run_liveness(
    live_video: UploadFile = File(...),
    challenge_type: str = Form(...),
    pipeline: VisionPipeline = Depends(get_vision_pipeline),
) -> VerifyFaceResponse:
    return await pipeline.run_liveness(live_video=live_video, challenge_type=challenge_type)


@router.post("/match-face", response_model=MatchFaceResponse)
async def match_face(
    cccd_front_image: UploadFile = File(...),
    live_video: UploadFile = File(...),
    pipeline: VisionPipeline = Depends(get_vision_pipeline),
) -> MatchFaceResponse:
    return await pipeline.match_face(cccd_front_image=cccd_front_image, live_video=live_video)


@router.post("/verify-face", response_model=VerifyFaceResponse)
async def verify_face(
    cccd_front_image: UploadFile = File(...),
    live_video: UploadFile = File(...),
    challenge_type: str = Form(...),
    pipeline: VisionPipeline = Depends(get_vision_pipeline),
) -> VerifyFaceResponse:
    return await pipeline.verify_face(
        cccd_front_image=cccd_front_image,
        live_video=live_video,
        challenge_type=challenge_type,
    )
