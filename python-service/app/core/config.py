from functools import lru_cache

from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    app_name: str = Field(default="Anomaly Python Service", alias="APP_NAME")
    app_env: str = Field(default="development", alias="APP_ENV")
    app_host: str = Field(default="0.0.0.0", alias="APP_HOST")
    app_port: int = Field(default=8090, alias="APP_PORT")
    vision_pipeline_backend: str = Field(default="disabled", alias="VISION_PIPELINE_BACKEND")
    # update to true bool when has backend
    allow_stub_vision_pipeline: bool = Field(default=False, alias="ALLOW_STUB_VISION_PIPELINE")
    match_threshold: float = Field(default=0.78, alias="MATCH_THRESHOLD", ge=0.0, le=1.0)
    liveness_threshold: float = Field(default=0.85, alias="LIVENESS_THRESHOLD", ge=0.0, le=1.0)
    max_image_bytes: int = Field(default=10 * 1024 * 1024, alias="MAX_IMAGE_BYTES", ge=1)
    max_video_bytes: int = Field(default=25 * 1024 * 1024, alias="MAX_VIDEO_BYTES", ge=1)
    min_video_duration_seconds: int = Field(default=3, alias="MIN_VIDEO_DURATION_SECONDS", ge=1)
    default_challenge_type: str = Field(
        default="TURN_HEAD_LEFT_RIGHT_BLINK",
        alias="DEFAULT_CHALLENGE_TYPE",
    )

    model_config = SettingsConfigDict(env_file=".env", env_file_encoding="utf-8", extra="ignore")


@lru_cache(maxsize=1)
def get_settings() -> Settings:
    return Settings()
