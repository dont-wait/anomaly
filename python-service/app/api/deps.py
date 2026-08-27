from functools import lru_cache

from app.services.pipeline import StubVisionPipeline, VisionPipeline


@lru_cache(maxsize=1)
def get_vision_pipeline() -> VisionPipeline:
    return StubVisionPipeline()
