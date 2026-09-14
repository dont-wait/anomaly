from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from app.api.routes.face_verification import MultipartBodyLimitMiddleware
from app.api.routes.face_verification import router as face_verification_router
from app.api.routes.health import router as health_router
from app.constants.endpoints import ENDPOINTS
from app.core.config import get_settings


def create_app() -> FastAPI:
    settings = get_settings()
    app = FastAPI(
        title=settings.app_name,
        description=(
            "Stateless CV service for CCCD portrait extraction, liveness, and face verification."
        ),
        version="0.1.0",
        docs_url=ENDPOINTS["docs"],
        redoc_url=ENDPOINTS["redoc"],
        openapi_url=ENDPOINTS["openapi"],
    )
    app.add_middleware(MultipartBodyLimitMiddleware)
    app.add_middleware(
        CORSMiddleware,
        allow_origins=settings.cors_allowed_origins,
        allow_methods=["GET", "POST"],
        allow_headers=["Content-Type"],
    )
    app.include_router(health_router)
    app.include_router(face_verification_router, prefix=ENDPOINTS["kyc_prefix"])
    return app


app = create_app()
