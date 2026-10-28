import hashlib

import numpy as np

from app.services.embedding_model import similarity
from app.services.liveness import Challenge, pose_and_blink


class VideoEvidence:
    def __init__(self, model, reference=None):
        self.model = model
        self.reference = reference
        self.challenge = Challenge(model.policy)
        self.frames = 0
        self.valid_faces = 0
        self.multiple = False
        self.consistent = True
        self.anchor = None
        self.last_embedding_at = -1
        self.scores = []
        self.times = []
        self.duration = 0.0
        self.face_state = "not_found"
        self.quality_state = "low"
        self.last_frame_digest = None
        self.last_distinct_frame_at = 0.0

    def observe(self, rgb, timestamp):
        self.frames += 1
        self.duration = timestamp
        digest = hashlib.sha256(rgb.tobytes()).digest()
        if digest != self.last_frame_digest:
            self.last_distinct_frame_at = timestamp
            self.last_frame_digest = digest
        elif timestamp - self.last_distinct_frame_at > self.model.policy.max_gap_seconds:
            # FFmpeg's fps filter can repeat a frame across a timestamp gap.
            # Repetition is not fresh liveness evidence.
            self.challenge.invalid_gap = True
        result = self.model.detect(rgb)
        faces = result.face_landmarks
        self.multiple |= len(faces) > 1
        self.face_state = "single" if len(faces) == 1 else "multiple" if faces else "not_found"
        if len(faces) != 1:
            return
        quality = self.model.quality(rgb, faces[0])
        self.quality_state = "passed" if quality else "low"
        if not quality:
            return
        self.valid_faces += 1
        yaw, blink = pose_and_blink(result)
        yaw *= -self.model.policy.left_yaw_sign
        self.challenge.observe(timestamp, yaw, blink)
        if timestamp - self.last_embedding_at < 0.2:
            return
        self.last_embedding_at = timestamp
        vector = self.model.embed(self.model.aligned(rgb, faces[0]))
        if self.anchor is None:
            self.anchor = vector
        else:
            self.consistent &= (
                similarity(self.anchor, vector) >= self.model.policy.consistency_threshold
            )
        if self.reference is not None and abs(yaw) <= self.model.policy.frontal_degrees:
            self.scores.append(similarity(self.reference, vector))
            self.times.append(timestamp)

    @property
    def match_score(self):
        return float(np.median(self.scores)) if self.scores else 0.0

    @property
    def matched(self):
        p = self.model.policy
        # Three separated periods, not neighboring frames of a single instant.
        bins = {int(t // (p.min_duration / 3)) for t in self.times}
        return (
            len(self.scores) >= p.min_frames
            and len(bins) >= 3
            and self.match_score >= p.match_threshold
            and sum(s >= p.match_threshold for s in self.scores) / len(self.scores)
            >= p.min_match_frame_ratio
        )

    def reason(self, require_match=True, require_liveness=True):
        p = self.model.policy
        if not self.frames or not p.min_duration <= self.duration <= p.max_duration:
            return "LIVE_VIDEO_QUALITY_LOW"
        if self.multiple:
            return "MULTIPLE_FACES_DETECTED"
        if not self.valid_faces:
            return "LIVE_FACE_NOT_FOUND"
        if self.valid_faces / self.frames < p.min_face_ratio:
            return "LIVE_VIDEO_QUALITY_LOW"
        if not self.consistent:
            return "LIVENESS_NOT_CONFIDENT"
        if (
            require_liveness
            and self.challenge.last_time is not None
            and self.duration - self.challenge.last_time > p.max_gap_seconds
        ):
            return "LIVENESS_NOT_CONFIDENT"
        if require_liveness and not self.challenge.passed:
            return "LIVENESS_CHALLENGE_FAILED"
        if require_liveness and self.challenge.score < p.liveness_threshold:
            return "LIVENESS_NOT_CONFIDENT"
        if require_match and not self.matched:
            return "FACE_MISMATCH"
        return None

    def progress(self):
        candidate = self.reason() is None
        return {
            "type": "progress",
            "processedMediaTimeMs": round(self.duration * 1000),
            "faceState": self.face_state,
            "qualityState": self.quality_state,
            "challengeStep": min(3, self.challenge.step),
            "challengePassed": self.challenge.passed,
            "matchingState": "candidate_match" if candidate else "collecting",
            "feedbackCode": (
                "MULTIPLE_FACES_DETECTED"
                if self.multiple
                else "LIVENESS_NOT_CONFIDENT"
                if not self.consistent
                else None
            ),
        }
