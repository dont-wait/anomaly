from functools import lru_cache

from fastapi import HTTPException, status

from app.core.config import get_settings
from app.services.pipeline import (
    VisionPipeline,
    VisionPipelineConfigurationError,
    build_vision_pipeline,
)


@lru_cache(maxsize=1)
def get_vision_pipeline() -> VisionPipeline:
    try:
        return build_vision_pipeline(get_settings())
    except VisionPipelineConfigurationError as exc:
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail={
                "code": "vision_pipeline_unavailable",
                "message": str(exc),
            },
        ) from exc
