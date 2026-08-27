from app.domain.enums import ReasonCode


_REASON_MESSAGES: dict[ReasonCode, str] = {
    ReasonCode.CCCD_IMAGE_QUALITY_LOW: "The CCCD image is too blurry, too dark, or has too much glare.",
    ReasonCode.CCCD_PORTRAIT_NOT_FOUND: "The portrait could not be extracted from the CCCD image.",
    ReasonCode.LIVE_FACE_NOT_FOUND: "No clear live face was detected in the video.",
    ReasonCode.MULTIPLE_FACES_DETECTED: "More than one face was detected in the live video.",
    ReasonCode.LIVE_VIDEO_QUALITY_LOW: "The live video is too short or does not meet quality requirements.",
    ReasonCode.FACE_OCCLUDED: "The face is partially covered and cannot be verified reliably.",
    ReasonCode.LIVENESS_CHALLENGE_FAILED: "The live liveness challenge was not completed successfully.",
    ReasonCode.LIVENESS_NOT_CONFIDENT: "Liveness confidence is too low to verify this attempt.",
    ReasonCode.FACE_MISMATCH: "The live face does not match the portrait from the CCCD.",
    ReasonCode.SESSION_EXPIRED: "The verification session has expired.",
    ReasonCode.RATE_LIMITED: "Too many requests were sent in a short period of time.",
    ReasonCode.INTERNAL_ERROR: "The service could not complete the verification attempt.",
}

_ERROR_MESSAGES: dict[str, str] = {
    "session_not_found": "Verification session not found.",
    "session_already_closed": "This verification session has already been completed or closed.",
    "invalid_request": "The request payload is invalid.",
}


def reason_message(reason_code: ReasonCode | None) -> str | None:
    if reason_code is None:
        return None
    return _REASON_MESSAGES.get(reason_code, "The verification attempt could not be completed.")


def error_message(error_code: str) -> str:
    return _ERROR_MESSAGES.get(error_code, "The request could not be processed.")
