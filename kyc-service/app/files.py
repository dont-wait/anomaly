from fastapi import UploadFile


async def read_limited_bytes(upload_file: UploadFile, max_bytes: int) -> tuple[bytes, bool]:
    payload = await upload_file.read(max_bytes + 1)
    overflowed = len(payload) > max_bytes
    if overflowed:
        payload = payload[:max_bytes]
    await upload_file.seek(0)
    return payload, overflowed
