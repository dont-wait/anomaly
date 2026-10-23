import hmac
from contextlib import asynccontextmanager

from fastapi import FastAPI, Request
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse

from app.api.routes.face_verification import MultipartBodyLimitMiddleware
from app.api.routes.face_verification import router as face_verification_router
from app.api.routes.health import router as health_router
from app.api.routes.realtime import router as realtime_router
from app.constants.endpoints import ENDPOINTS
from app.core.config import get_settings


def create_app() -> FastAPI:
    settings = get_settings()

    @asynccontextmanager
    async def lifespan(_app):
        import asyncio

        from app.api.deps import get_vision_pipeline

        backend = settings.vision_pipeline_backend.strip().lower()
        if backend == "stub" and settings.app_env not in {"test", "development"}:
            raise RuntimeError("Stub vision is forbidden in this environment")
        if backend == "real":
            if len(settings.service_token) < 32:
                raise RuntimeError(
                    "Real vision requires KYC_SERVICE_TOKEN of at least 32 characters"
                )
            await asyncio.to_thread(get_vision_pipeline)
        yield

    app = FastAPI(
        title=settings.app_name,
        description=(
            "Stateless CV service for CCCD portrait extraction, liveness, and face verification."
        ),
        version="0.1.0",
        docs_url=ENDPOINTS["docs"],
        redoc_url=ENDPOINTS["redoc"],
        openapi_url=ENDPOINTS["openapi"],
        lifespan=lifespan,
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
    app.include_router(realtime_router)

    @app.middleware("http")
    async def service_auth(request: Request, call_next):
        if (
            settings.vision_pipeline_backend.strip().lower() == "real"
            and request.url.path.startswith("/v1/")
        ):
            supplied = request.headers.get("X-KYC-Service-Token", "")
            if not settings.service_token or not hmac.compare_digest(
                supplied, settings.service_token
            ):
                return JSONResponse(status_code=401, content={"detail": "service_auth_required"})
        return await call_next(request)

    @app.get("/ready")
    async def readiness():
        import asyncio

        from app.api.deps import get_vision_pipeline

        pipeline = await asyncio.to_thread(get_vision_pipeline)
        return {"status": "ready", "backend": type(pipeline).__name__}

    return app


app = create_app()
