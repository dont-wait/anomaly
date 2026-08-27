from __future__ import annotations

from typing import Protocol

from fastapi import UploadFile

from app.core.config import Settings, get_settings
from app.files import read_limited_bytes
from app.domain.enums import Decision, ReasonCode
from app.domain.messages import reason_message
from app.schemas.face_verification import (
    ExtractIdFaceResponse,
    MatchFaceResponse,
    QualityChecksResponse,
    VerifyFaceResponse,
)


class VisionPipelineConfigurationError(RuntimeError):
    pass


class VisionPipeline(Protocol):
    async def extract_id_face(self, cccd_front_image: UploadFile) -> ExtractIdFaceResponse: ...

    async def run_liveness(self, live_video: UploadFile, challenge_type: str) -> VerifyFaceResponse: ...

    async def match_face(self, cccd_front_image: UploadFile, live_video: UploadFile) -> MatchFaceResponse: ...

    async def verify_face(
        self,
        cccd_front_image: UploadFile,
        live_video: UploadFile,
        challenge_type: str,
    ) -> VerifyFaceResponse: ...


class StubVisionPipeline:
    def __init__(self, settings: Settings | None = None) -> None:
        self._settings = settings or get_settings()

    async def extract_id_face(self, cccd_front_image: UploadFile) -> ExtractIdFaceResponse:
        image_bytes, image_overflowed = await read_limited_bytes(cccd_front_image, self._settings.max_image_bytes)
        image_ok = self._is_allowed_image(cccd_front_image.content_type) and not image_overflowed
        if not image_ok:
            return ExtractIdFaceResponse(
                success=False,
                reason_code=ReasonCode.CCCD_IMAGE_QUALITY_LOW,
                reason_message=reason_message(ReasonCode.CCCD_IMAGE_QUALITY_LOW),
                portrait_extracted=False,
                face_detected=False,
                image_quality_passed=False,
            )

        return ExtractIdFaceResponse(
            success=True,
            portrait_extracted=True,
            face_detected=True,
            image_quality_passed=True,
        )

    async def run_liveness(self, live_video: UploadFile, challenge_type: str) -> VerifyFaceResponse:
        video_bytes, video_overflowed = await read_limited_bytes(live_video, self._settings.max_video_bytes)
        quality_checks = QualityChecksResponse(
            cccd_portrait_extracted=False,
            single_face_in_video=bool(video_bytes),
            image_quality_passed=True,
        )
        if not self._is_allowed_video(live_video.content_type) or video_overflowed:
            return self._verification_failure(
                decision=Decision.RETRY_ALLOWED,
                reason_code=ReasonCode.LIVE_VIDEO_QUALITY_LOW,
                match_score=0.0,
                liveness_score=0.0,
                quality_checks=quality_checks.model_copy(update={"image_quality_passed": False}),
            )

        liveness_score = self._score_from_name(live_video.filename, self._settings.liveness_threshold)
        if not challenge_type:
            return self._verification_failure(
                decision=Decision.RETRY_ALLOWED,
                reason_code=ReasonCode.LIVENESS_CHALLENGE_FAILED,
                match_score=0.0,
                liveness_score=0.0,
                quality_checks=quality_checks,
            )
        if liveness_score < self._settings.liveness_threshold:
            return self._verification_failure(
                decision=Decision.RETRY_ALLOWED,
                reason_code=ReasonCode.LIVENESS_NOT_CONFIDENT,
                match_score=0.0,
                liveness_score=liveness_score,
                quality_checks=quality_checks,
            )

        return VerifyFaceResponse(
            success=True,
            decision=Decision.VERIFIED,
            match_score=0.0,
            liveness_score=liveness_score,
            quality_checks=quality_checks,
        )

    async def match_face(self, cccd_front_image: UploadFile, live_video: UploadFile) -> MatchFaceResponse:
        image_bytes, image_overflowed = await read_limited_bytes(cccd_front_image, self._settings.max_image_bytes)
        video_bytes, video_overflowed = await read_limited_bytes(live_video, self._settings.max_video_bytes)
        quality_checks = QualityChecksResponse(
            cccd_portrait_extracted=bool(image_bytes),
            single_face_in_video=bool(video_bytes),
            image_quality_passed=True,
        )
        if not self._is_allowed_image(cccd_front_image.content_type) or image_overflowed:
            return self._match_failure(ReasonCode.CCCD_IMAGE_QUALITY_LOW, 0.0, quality_checks.model_copy(update={"image_quality_passed": False, "cccd_portrait_extracted": False}))
        if not self._is_allowed_video(live_video.content_type) or video_overflowed:
            return self._match_failure(ReasonCode.LIVE_VIDEO_QUALITY_LOW, 0.0, quality_checks.model_copy(update={"image_quality_passed": False}))

        match_score = self._pair_score(cccd_front_image.filename, live_video.filename, self._settings.match_threshold)
        if match_score < self._settings.match_threshold:
            return self._match_failure(ReasonCode.FACE_MISMATCH, match_score, quality_checks)

        return MatchFaceResponse(
            success=True,
            match_score=match_score,
            quality_checks=quality_checks,
        )

    async def verify_face(
        self,
        cccd_front_image: UploadFile,
        live_video: UploadFile,
        challenge_type: str,
    ) -> VerifyFaceResponse:
        image_bytes, image_overflowed = await read_limited_bytes(cccd_front_image, self._settings.max_image_bytes)
        video_bytes, video_overflowed = await read_limited_bytes(live_video, self._settings.max_video_bytes)
        quality_checks = QualityChecksResponse(
            cccd_portrait_extracted=bool(image_bytes),
            single_face_in_video=bool(video_bytes),
            image_quality_passed=True,
        )

        if not self._is_allowed_image(cccd_front_image.content_type) or image_overflowed:
            return self._verification_failure(
                decision=Decision.RETRY_ALLOWED,
                reason_code=ReasonCode.CCCD_IMAGE_QUALITY_LOW,
                match_score=0.0,
                liveness_score=0.0,
                quality_checks=quality_checks.model_copy(update={"cccd_portrait_extracted": False, "image_quality_passed": False}),
            )
        if not self._is_allowed_video(live_video.content_type) or video_overflowed:
            return self._verification_failure(
                decision=Decision.RETRY_ALLOWED,
                reason_code=ReasonCode.LIVE_VIDEO_QUALITY_LOW,
                match_score=0.0,
                liveness_score=0.0,
                quality_checks=quality_checks.model_copy(update={"image_quality_passed": False}),
            )

        liveness_score = self._score_from_name(live_video.filename, self._settings.liveness_threshold)
        if not challenge_type:
            return self._verification_failure(
                decision=Decision.RETRY_ALLOWED,
                reason_code=ReasonCode.LIVENESS_CHALLENGE_FAILED,
                match_score=0.0,
                liveness_score=0.0,
                quality_checks=quality_checks,
            )
        if liveness_score < self._settings.liveness_threshold:
            return self._verification_failure(
                decision=Decision.RETRY_ALLOWED,
                reason_code=ReasonCode.LIVENESS_NOT_CONFIDENT,
                match_score=0.0,
                liveness_score=liveness_score,
                quality_checks=quality_checks,
            )

        match_score = self._pair_score(cccd_front_image.filename, live_video.filename, self._settings.match_threshold)
        if match_score < self._settings.match_threshold:
            return self._verification_failure(
                decision=Decision.RETRY_ALLOWED,
                reason_code=ReasonCode.FACE_MISMATCH,
                match_score=match_score,
                liveness_score=liveness_score,
                quality_checks=quality_checks,
            )

        return VerifyFaceResponse(
            success=True,
            decision=Decision.VERIFIED,
            match_score=match_score,
            liveness_score=liveness_score,
            quality_checks=quality_checks,
        )

    def _verification_failure(
        self,
        decision: Decision,
        reason_code: ReasonCode,
        match_score: float,
        liveness_score: float,
        quality_checks: QualityChecksResponse,
    ) -> VerifyFaceResponse:
        return VerifyFaceResponse(
            success=False,
            decision=decision,
            reason_code=reason_code,
            reason_message=reason_message(reason_code),
            match_score=match_score,
            liveness_score=liveness_score,
            quality_checks=quality_checks,
        )

    def _match_failure(
        self,
        reason_code: ReasonCode,
        match_score: float,
        quality_checks: QualityChecksResponse,
    ) -> MatchFaceResponse:
        return MatchFaceResponse(
            success=False,
            reason_code=reason_code,
            reason_message=reason_message(reason_code),
            match_score=match_score,
            quality_checks=quality_checks,
        )

    def _is_allowed_image(self, content_type: str | None) -> bool:
        return content_type in {"image/jpeg", "image/png", "image/webp"}

    def _is_allowed_video(self, content_type: str | None) -> bool:
        return content_type in {"video/mp4", "video/quicktime", "video/webm"}

    def _score_from_name(self, filename: str | None, fallback: float) -> float:
        if not filename:
            return fallback
        lowered = filename.lower()
        if "low" in lowered:
            return max(fallback - 0.2, 0.0)
        if "pass" in lowered or "high" in lowered:
            return min(fallback + 0.05, 1.0)
        return fallback

    def _pair_score(self, image_filename: str | None, video_filename: str | None, fallback: float) -> float:
        joined = f"{image_filename or ''}:{video_filename or ''}".lower()
        if "mismatch" in joined or "fail" in joined:
            return max(fallback - 0.2, 0.0)
        if "match" in joined or "pass" in joined:
            return min(fallback + 0.05, 1.0)
        return fallback


def build_vision_pipeline(settings: Settings) -> VisionPipeline:
    backend = settings.vision_pipeline_backend.strip().lower()
    stub_allowed = settings.allow_stub_vision_pipeline or settings.app_env == "test"

    if backend == "stub":
        if not stub_allowed:
            raise VisionPipelineConfigurationError(
                "Stub vision pipeline is disabled outside tests or an explicit development override.",
            )
        return StubVisionPipeline(settings)

    raise VisionPipelineConfigurationError(
        "No real vision pipeline is configured. Configure a supported backend before serving requests.",
    )
