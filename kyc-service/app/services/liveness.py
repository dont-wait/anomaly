"""Temporal evidence only. This is active challenge checking, not a PAD classifier."""

import math


class Challenge:
    def __init__(self, policy):
        self.policy = policy
        self.step = 0
        self.hold_since = None
        self.last_time = None
        self.open_seen = False
        self.closed_since = None
        self.closed_frames = 0
        self.invalid_gap = False

    def observe(self, timestamp, yaw, blink):
        if self.last_time is None and timestamp > self.policy.max_gap_seconds:
            self.invalid_gap = True
        if self.last_time is not None and timestamp - self.last_time > self.policy.max_gap_seconds:
            self.invalid_gap = True
        self.last_time = timestamp
        p = self.policy
        if self.step < 3:
            pose_ok = (
                abs(yaw) <= p.frontal_degrees
                if self.step == 0
                else yaw <= -p.yaw_degrees
                if self.step == 1
                else yaw >= p.yaw_degrees
            )
            if not pose_ok:
                self.hold_since = None
            elif self.hold_since is None:
                self.hold_since = timestamp
            elif timestamp - self.hold_since >= p.pose_hold_seconds:
                self.step += 1
                self.hold_since = None
        elif self.step == 3 and abs(yaw) <= p.frontal_degrees:
            if blink < p.blink_open:
                if (
                    self.open_seen
                    and self.closed_frames >= 2
                    and self.closed_since is not None
                    and 0.05 <= timestamp - self.closed_since <= 0.8
                ):
                    self.step = 4
                self.open_seen = True
                self.closed_since = None
                self.closed_frames = 0
            elif blink >= p.blink_closed and self.open_seen and self.closed_since is None:
                self.closed_since = timestamp
                self.closed_frames = 1
            elif blink >= p.blink_closed and self.closed_since is not None:
                self.closed_frames += 1

    @property
    def passed(self):
        return self.step == 4 and not self.invalid_gap

    @property
    def score(self):
        return self.step / 4 if not self.invalid_gap else 0.0


def pose_and_blink(result):
    matrix = result.facial_transformation_matrixes[0]
    yaw = math.degrees(math.atan2(float(matrix[0, 2]), float(matrix[2, 2])))
    blend = {c.category_name: c.score for c in result.face_blendshapes[0]}
    return yaw, min(blend.get("eyeBlinkLeft", 0), blend.get("eyeBlinkRight", 0))
