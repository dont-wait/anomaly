import asyncio
import shutil
import threading
import time

from fastapi import HTTPException

from app.domain.enums import Decision, ReasonCode
from app.domain.messages import reason_message
from app.files import read_limited_bytes
from app.schemas.face_verification import (
    AnalysisResponse,
    ExtractIdFaceResponse,
    MatchFaceResponse,
    QualityChecksResponse,
    VerifyFaceResponse,
)
from app.services.document_processor import DocumentRejected, reference_face
from app.services.embedding_model import FaceModel
from app.services.face_matcher import VideoEvidence
from app.services.video_processor import analyze_file
from app.services.vision_config import load_config

CHALLENGE = "TURN_HEAD_LEFT_RIGHT_BLINK"


class RealVisionPipeline:
    def __init__(self, settings):
        self.settings = settings
        self.policy, self.manifest, weights, landmarks = load_config(
            settings.vision_config_path, settings.face_model_manifest_path
        )
        if not shutil.which(self.policy.ffmpeg):
            raise ValueError("ffmpeg is required")
        self.model = FaceModel(self.policy, self.manifest, weights, landmarks)
        self.slots = threading.BoundedSemaphore(self.policy.max_sessions)

    async def read(self, upload, limit):
        data, overflow = await read_limited_bytes(upload, limit)
        if overflow:
            raise HTTPException(413, "media_too_large")
        if not data:
            raise HTTPException(400, "empty_media")
        return data

    def reserve(self):
        if not self.slots.acquire(blocking=False):
            raise HTTPException(503, "vision_capacity_exceeded")

    async def extract_id_face(self, cccd_front_image):
        data = await self.read(cccd_front_image, self.settings.max_image_bytes)
        self.reserve()
        try:
            await asyncio.to_thread(reference_face, data, self.model)
            return ExtractIdFaceResponse(
                success=True, portrait_extracted=True, face_detected=True, image_quality_passed=True
            )
        except DocumentRejected as exc:
            code = ReasonCode(exc.reason)
            return ExtractIdFaceResponse(
                success=False,
                reason_code=code,
                reason_message=reason_message(code),
                portrait_extracted=False,
                face_detected=False,
                image_quality_passed=False,
            )
        finally:
            self.slots.release()

    async def verify_face(self, cccd_front_image, live_video, challenge_type):
        if challenge_type != CHALLENGE:
            raise HTTPException(400, "unsupported_challenge")
        return await self._analyze(cccd_front_image, live_video)

    async def run_liveness(self, live_video, challenge_type):
        if challenge_type != CHALLENGE:
            raise HTTPException(400, "unsupported_challenge")
        return await self._analyze(None, live_video)

    async def match_face(self, cccd_front_image, live_video):
        result = await self._analyze(cccd_front_image, live_video, require_liveness=False)
        return MatchFaceResponse(
            success=result.success,
            reason_code=result.reason_code,
            reason_message=result.reason_message,
            match_score=result.match_score,
            quality_checks=result.quality_checks,
        )

    async def _analyze(self, front, video, require_liveness=True):
        image = await self.read(front, self.settings.max_image_bytes) if front else None
        data = await self.read(video, self.settings.max_video_bytes)
        self.reserve()

        # A cancelled HTTP caller must not release capacity while the worker still runs.
        def work():
            started = time.monotonic()
            try:
                ref = reference_face(image, self.model) if image else None
                evidence = VideoEvidence(self.model, ref)
                analyze_file(data, evidence)
                return self.result(evidence, started, require_liveness)
            except DocumentRejected as exc:
                return self.failure(exc.reason)
            except ValueError:
                return self.failure("LIVE_VIDEO_QUALITY_LOW")
            finally:
                self.slots.release()

        return await asyncio.to_thread(work)

    def failure(self, reason):
        code = ReasonCode(reason)
        return VerifyFaceResponse(
            success=False,
            decision=Decision.RETRY_ALLOWED,
            reason_code=code,
            reason_message=reason_message(code),
            match_score=0,
            liveness_score=0,
            quality_checks=QualityChecksResponse(
                cccd_portrait_extracted=False,
                single_face_in_video=False,
                image_quality_passed=False,
            ),
        )

    def result(self, evidence, started, require_liveness=True):
        reason = evidence.reason(evidence.reference is not None, require_liveness)
        m, p = self.manifest, self.policy
        return VerifyFaceResponse(
            success=reason is None,
            decision=Decision.RETRY_ALLOWED if reason else Decision.VERIFIED,
            reason_code=ReasonCode(reason) if reason else None,
            reason_message=reason_message(ReasonCode(reason)) if reason else None,
            match_score=evidence.match_score,
            liveness_score=evidence.challenge.score,
            quality_checks=QualityChecksResponse(
                cccd_portrait_extracted=evidence.reference is not None,
                single_face_in_video=not evidence.multiple and evidence.valid_faces > 0,
                image_quality_passed=(
                    evidence.frames > 0
                    and evidence.valid_faces / evidence.frames >= p.min_face_ratio
                ),
            ),
            analysis=AnalysisResponse(
                model_id=m.model_id,
                model_version=m.model_version,
                model_sha256=m.sha256,
                pipeline_version=p.pipeline_version,
                calibration_version=p.calibration_version,
                matching_status=(
                    "not_run"
                    if evidence.reference is None
                    else "passed"
                    if evidence.matched
                    else "failed"
                ),
                liveness_status=(
                    "not_run"
                    if not require_liveness
                    else "passed"
                    if evidence.challenge.passed
                    else "failed"
                ),
                subject_consistency_passed=evidence.consistent,
                challenge_passed=evidence.challenge.passed,
                sampled_frames=evidence.frames,
                matched_frames=len(evidence.scores),
                video_duration_seconds=evidence.duration,
                processing_ms=round((time.monotonic() - started) * 1000),
            ),
        )
