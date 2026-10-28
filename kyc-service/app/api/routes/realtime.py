import asyncio
import base64
import hmac
import time

from fastapi import APIRouter, WebSocket, WebSocketDisconnect

from app.api.deps import get_vision_pipeline
from app.core.config import get_settings

router = APIRouter()


@router.websocket("/v1/kyc/realtime")
async def realtime(socket: WebSocket):
    settings = get_settings()
    token = socket.headers.get("X-KYC-Service-Token", "")
    if not settings.service_token or not hmac.compare_digest(token, settings.service_token):
        await socket.close(code=1008)
        return
    await socket.accept()
    decoder = None
    pipeline = None
    reserved = False
    worker = None
    try:
        from app.services.document_processor import reference_face
        from app.services.face_matcher import VideoEvidence
        from app.services.real_pipeline import CHALLENGE, RealVisionPipeline
        from app.services.video_processor import VideoDecoder

        pipeline = await asyncio.to_thread(get_vision_pipeline)
        if not isinstance(pipeline, RealVisionPipeline):
            raise ValueError("realtime requires real backend")
        config = await asyncio.wait_for(socket.receive_json(), 5)
        if config.get("challenge_type") != CHALLENGE:
            raise ValueError("unsupported challenge")
        front = base64.b64decode(config["front"], validate=True)
        if len(front) > settings.max_image_bytes:
            raise ValueError("image too large")
        pipeline.reserve()
        reserved = True
        reference = await asyncio.to_thread(reference_face, front, pipeline.model)
        evidence = VideoEvidence(pipeline.model, reference)
        await socket.send_json({"type": "ready"})
        start = await asyncio.wait_for(socket.receive_json(), 90)
        if start.get("type") != "start":
            raise ValueError("expected start")
        decoder = VideoDecoder(pipeline.policy.ffmpeg, timeout=25)
        loop = asyncio.get_running_loop()
        events = asyncio.Queue(maxsize=2)

        def publish(event):
            if events.full():
                events.get_nowait()
            events.put_nowait(event)

        def analyze():
            last_event = -1
            for frame, timestamp in decoder.frames():
                evidence.observe(frame, timestamp)
                if timestamp - last_event >= 0.2:
                    last_event = timestamp
                    loop.call_soon_threadsafe(publish, evidence.progress())

        async def ingest():
            seq, size = 0, 0
            while True:
                message = await socket.receive()
                if message["type"] == "websocket.disconnect":
                    raise WebSocketDisconnect()
                chunk = message.get("bytes")
                if chunk is not None:
                    if len(chunk) < 6 or chunk[0] != 1 or int.from_bytes(chunk[1:5], "big") != seq:
                        raise ValueError("invalid chunk sequence")
                    size += len(chunk) - 5
                    if size > settings.max_video_bytes or len(chunk) > (1 << 20) + 5:
                        raise ValueError("video limit exceeded")
                    seq += 1
                    await asyncio.to_thread(decoder.write, chunk[5:])
                else:
                    import json

                    end = json.loads(message.get("text", "{}"))
                    if end.get("type") != "end" or end.get("lastChunkSeq") != seq - 1:
                        raise ValueError("invalid end sequence")
                    await asyncio.to_thread(decoder.finish)
                    return

        async def emit():
            while True:
                await socket.send_json(await events.get())

        started = time.monotonic()
        worker = asyncio.create_task(asyncio.to_thread(analyze))
        sender = asyncio.create_task(emit())
        receiver = asyncio.create_task(ingest())
        try:
            # A decode error must stop ingest too, rather than leave it blocked on the pipe.
            done, _ = await asyncio.wait(
                {worker, receiver}, timeout=22, return_when=asyncio.FIRST_COMPLETED
            )
            if not done:
                raise TimeoutError("realtime budget exceeded")
            for task in done:
                task.result()
            await asyncio.wait_for(asyncio.shield(receiver), 2)
            await asyncio.wait_for(asyncio.shield(worker), 5)
            sender.cancel()
            await asyncio.gather(sender, return_exceptions=True)
            result = pipeline.result(evidence, started)
            await socket.send_json({"type": "stream_finished", "result": result.model_dump()})
        finally:
            receiver.cancel()
            sender.cancel()
            await asyncio.gather(receiver, sender, return_exceptions=True)
    except (Exception, asyncio.CancelledError) as exc:
        try:
            await socket.send_json(
                {"type": "error", "code": getattr(exc, "reason", "REALTIME_UNAVAILABLE")}
            )
        except Exception:
            pass
    finally:
        if decoder:
            await asyncio.to_thread(decoder.close)
        if worker:
            # Decoder termination unblocks its worker; never release the slot early.
            await asyncio.gather(worker, return_exceptions=True)
        if reserved:
            pipeline.slots.release()
        try:
            await socket.close()
        except RuntimeError:
            pass
