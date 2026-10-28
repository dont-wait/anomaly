"""Versioned deployment configuration. No model or calibration is guessed at runtime."""

import hashlib
import json
import tomllib
from pathlib import Path
from typing import Literal

from pydantic import BaseModel, ConfigDict, Field, model_validator


class ModelManifest(BaseModel):
    model_config = ConfigDict(allow_inf_nan=False)
    model_id: str = Field(min_length=1)
    model_version: str = Field(min_length=1)
    weights: str
    sha256: str = Field(pattern=r"^[a-f0-9]{64}$")
    source: str = Field(min_length=1)
    license: str = Field(min_length=1)
    input_name: str
    output_name: str
    width: int = Field(ge=32, le=640)
    height: int = Field(ge=32, le=640)
    embedding_size: int = Field(ge=16, le=4096)
    layout: str = Field(pattern="^(NCHW|NHWC)$")
    color: str = Field(pattern="^(RGB|BGR)$")
    mean: tuple[float, float, float]
    std: tuple[float, float, float]
    # Five destination landmarks, normalized to the embedding model input.
    alignment_points: list[tuple[float, float]] = Field(min_length=5, max_length=5)
    landmarker: str
    landmarker_sha256: str = Field(pattern=r"^[a-f0-9]{64}$")

    @model_validator(mode="after")
    def valid_preprocessing(self):
        if any(s <= 0 for s in self.std):
            raise ValueError("normalization std must be positive")
        if any(not 0 <= v <= 1 for point in self.alignment_points for v in point):
            raise ValueError("alignment points must be normalized")
        return self


class VisionConfig(BaseModel):
    model_config = ConfigDict(allow_inf_nan=False)
    pipeline_version: str = Field(min_length=1)
    calibration_version: str = Field(min_length=1)
    # Deployment must explicitly attest calibration; example config is disabled.
    calibrated: bool
    match_threshold: float = Field(gt=0, lt=1)
    consistency_threshold: float = Field(gt=0, lt=1)
    min_match_frame_ratio: float = Field(gt=0, le=1)
    liveness_threshold: float = Field(gt=0, le=1)
    min_face_ratio: float = Field(gt=0, le=1)
    min_frames: int = Field(ge=5, le=100)
    min_duration: float = Field(ge=3, le=15)
    max_duration: float = Field(ge=10, le=20)
    max_gap_seconds: float = Field(gt=0, le=1)
    min_blur: float = Field(gt=0)
    min_brightness: float = Field(ge=0, lt=255)
    max_brightness: float = Field(gt=0, le=255)
    max_glare_ratio: float = Field(gt=0, lt=1)
    min_face_pixels: int = Field(ge=24)
    yaw_degrees: float = Field(gt=0, lt=60)
    left_yaw_sign: Literal[-1, 1]
    frontal_degrees: float = Field(gt=0, lt=30)
    pose_hold_seconds: float = Field(gt=0, le=2)
    blink_closed: float = Field(gt=0, lt=1)
    blink_open: float = Field(gt=0, lt=1)
    providers: list[str] = Field(min_length=1)
    max_sessions: int = Field(ge=1, le=32)
    ffmpeg: str = "ffmpeg"
    # Named layout with normalized portrait ROI after card rectification.
    card_layout: str = Field(min_length=1)
    portrait_roi: tuple[float, float, float, float]

    @model_validator(mode="after")
    def valid_policy(self):
        x0, y0, x1, y1 = self.portrait_roi
        if not (0 <= x0 < x1 <= 1 and 0 <= y0 < y1 <= 1):
            raise ValueError("invalid portrait ROI")
        if not self.calibrated or self.min_duration >= self.max_duration:
            raise ValueError("a calibrated policy and valid duration bounds are required")
        if self.blink_open >= self.blink_closed:
            raise ValueError("blink open threshold must be below closed threshold")
        return self


def checked_artifact(root: Path, name: str, checksum: str) -> Path:
    path = (root / name).resolve()
    if hashlib.sha256(path.read_bytes()).hexdigest() != checksum:
        raise ValueError(f"artifact checksum mismatch: {path.name}")
    return path


def load_config(config_path: str, manifest_path: str):
    policy = VisionConfig.model_validate(tomllib.loads(Path(config_path).read_text()))
    manifest_file = Path(manifest_path)
    manifest = ModelManifest.model_validate(json.loads(manifest_file.read_text()))
    weights = checked_artifact(manifest_file.parent, manifest.weights, manifest.sha256)
    landmarks = checked_artifact(
        manifest_file.parent, manifest.landmarker, manifest.landmarker_sha256
    )
    return policy, manifest, weights, landmarks
