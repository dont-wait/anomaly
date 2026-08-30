from pydantic import BaseModel

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


class VerifyFaceResponse(BaseModel):
    success: bool
    decision: Decision
    reason_code: ReasonCode | None = None
    reason_message: str | None = None
    match_score: float
    liveness_score: float
    quality_checks: QualityChecksResponse
