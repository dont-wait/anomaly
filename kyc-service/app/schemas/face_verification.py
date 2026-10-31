from typing import Literal

from pydantic import BaseModel, Field

from app.domain.enums import Decision, ReasonCode


class QualityChecksResponse(BaseModel):
    cccd_portrait_extracted: bool
    single_face_in_video: bool
    image_quality_passed: bool


class ExtractIdFaceResponse(BaseModel):
    success: bool
    reason_code: ReasonCode | None = None
    reason_message: str | None = None
    portrait_extracted: bool
    face_detected: bool
    image_quality_passed: bool


class MatchFaceResponse(BaseModel):
    success: bool
    reason_code: ReasonCode | None = None
    reason_message: str | None = None
    match_score: float
    quality_checks: QualityChecksResponse


class AnalysisResponse(BaseModel):
    request_id: str = ""
    model_id: str
    model_version: str
    model_sha256: str
    pipeline_version: str
    calibration_version: str
    matching_status: Literal["not_run", "passed", "failed"]
    liveness_status: Literal["not_run", "passed", "failed"]
    subject_consistency_passed: bool
    challenge_passed: bool
    sampled_frames: int = Field(ge=0)
    matched_frames: int = Field(ge=0)
    video_duration_seconds: float = Field(ge=0)
    processing_ms: int = Field(ge=0)


class VerifyFaceResponse(BaseModel):
    poc_bypass: bool = False
    success: bool
    decision: Decision
    reason_code: ReasonCode | None = None
    reason_message: str | None = None
    match_score: float = Field(ge=0, le=1, allow_inf_nan=False)
    liveness_score: float = Field(ge=0, le=1, allow_inf_nan=False)
    quality_checks: QualityChecksResponse
    analysis: AnalysisResponse | None = None
