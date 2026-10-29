"""Explicit demo bypass. No biometric inference or synthetic model evidence."""

import asyncio
import base64
import json
import time

from fastapi import HTTPException, UploadFile, WebSocket

from app.domain.enums import Decision
from app.files import read_limited_bytes
from app.schemas.face_verification import (
    AnalysisResponse,
    QualityChecksResponse,
    VerifyFaceResponse,
)
from app.services.pipeline import StubVisionPipeline

CHALLENGE = "TURN_HEAD_LEFT_RIGHT_BLINK"


class PocVisionPipeline(StubVisionPipeline):
    async def verify_face(
        self, cccd_front_image: UploadFile, live_video: UploadFile, challenge_type: str
    ) -> VerifyFaceResponse:
        image, image_overflow = await read_limited_bytes(
            cccd_front_image, self._settings.max_image_bytes
        )
        video, video_overflow = await read_limited_bytes(live_video, self._settings.max_video_bytes)
        if (
            not image
            or not video
            or image_overflow
            or video_overflow
            or not self._is_allowed_image(cccd_front_image.content_type)
            or not self._is_allowed_video(live_video.content_type)
            or challenge_type != CHALLENGE
        ):
            raise HTTPException(status_code=400, detail="invalid POC media")
        return VerifyFaceResponse(
            poc_bypass=True,
            success=True,
            decision=Decision.VERIFIED,
            match_score=0,
            liveness_score=0,
            quality_checks=QualityChecksResponse(
                cccd_portrait_extracted=False,
                single_face_in_video=False,
                image_quality_passed=False,
            ),
            analysis=AnalysisResponse(
                model_id="poc-bypass",
                model_version="none",
                model_sha256="",
                pipeline_version="poc-v1",
                calibration_version="none",
                matching_status="not_run",
                liveness_status="not_run",
                subject_consistency_passed=False,
                challenge_passed=False,
                sampled_frames=0,
                matched_frames=0,
                video_duration_seconds=0,
                processing_ms=0,
            ),
        )

    async def realtime(self, socket: WebSocket) -> None:
        config = await asyncio.wait_for(socket.receive_json(), 5)
        front = base64.b64decode(config.get("front", ""), validate=True)
        if (
            not front
            or len(front) > self._settings.max_image_bytes
            or config.get("challenge_type") != CHALLENGE
        ):
            raise ValueError("invalid POC configuration")
        await socket.send_json({"type": "ready"})
        start = await asyncio.wait_for(socket.receive_json(), 90)
        if start.get("type") != "start":
            raise ValueError("expected start")
        seq, size = 0, 0
        started = time.monotonic()
        while True:
            message = await asyncio.wait_for(socket.receive(), 25)
            if message["type"] == "websocket.disconnect":
                return
            chunk = message.get("bytes")
            if chunk is not None:
                if len(chunk) < 6 or chunk[0] != 1 or int.from_bytes(chunk[1:5], "big") != seq:
                    raise ValueError("invalid chunk sequence")
                size += len(chunk) - 5
                if size > self._settings.max_video_bytes or len(chunk) > (1 << 20) + 5:
                    raise ValueError("video limit exceeded")
                seq += 1
                elapsed = time.monotonic() - started
                # Demo progress only; no claim that a face or gesture was detected.
                await socket.send_json(
                    {
                        "type": "progress",
                        "challengeStep": min(3, int(elapsed / 3)),
                        "faceState": "not_run",
                        "qualityState": "not_run",
                        "matchingState": "not_run",
                        "challengePassed": False,
                        "processedMediaTimeMs": int(elapsed * 1000),
                    }
                )
            else:
                end = json.loads(message.get("text", "{}"))
                if end.get("type") != "end" or end.get("lastChunkSeq") != seq - 1 or seq == 0:
                    raise ValueError("invalid end sequence")
                await socket.send_json({"type": "stream_finished"})
                return
