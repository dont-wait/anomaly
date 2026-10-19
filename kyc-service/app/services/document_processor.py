import cv2
import numpy as np


class DocumentRejected(ValueError):
    def __init__(self, reason="CCCD_PORTRAIT_NOT_FOUND"):
        self.reason = reason
        super().__init__(reason)


def reference_face(data: bytes, model):
    image = cv2.imdecode(np.frombuffer(data, np.uint8), cv2.IMREAD_COLOR)
    if image is None or image.shape[0] * image.shape[1] > 20_000_000:
        raise DocumentRejected("CCCD_IMAGE_QUALITY_LOW")
    # Bound contour work; the actual portrait is cropped from the rectified card.
    scale = min(1, 1600 / max(image.shape[:2]))
    image = cv2.resize(image, None, fx=scale, fy=scale)
    gray = cv2.cvtColor(image, cv2.COLOR_BGR2GRAY)
    contours, _ = cv2.findContours(
        cv2.Canny(gray, 60, 180), cv2.RETR_EXTERNAL, cv2.CHAIN_APPROX_SIMPLE
    )
    cards = []
    for contour in contours:
        if cv2.contourArea(contour) < gray.size * 0.2:
            continue
        polygon = cv2.approxPolyDP(contour, 0.02 * cv2.arcLength(contour, True), True)
        if len(polygon) != 4 or not cv2.isContourConvex(polygon):
            continue
        points = polygon.reshape(4, 2).astype(np.float32)
        ordered = np.array(
            [
                points[np.argmin(points.sum(1))],
                points[np.argmin(np.diff(points, axis=1))],
                points[np.argmax(points.sum(1))],
                points[np.argmax(np.diff(points, axis=1))],
            ],
            np.float32,
        )
        width = np.linalg.norm(ordered[1] - ordered[0])
        height = np.linalg.norm(ordered[3] - ordered[0])
        if 1.4 <= width / max(height, 1) <= 1.85:
            cards.append((cv2.contourArea(contour), ordered))
    if not cards:
        raise DocumentRejected()
    points = max(cards, key=lambda c: c[0])[1]
    matrix = cv2.getPerspectiveTransform(
        points, np.float32([[0, 0], [855, 0], [855, 539], [0, 539]])
    )
    card = cv2.warpPerspective(image, matrix, (856, 540))
    x0, y0, x1, y1 = model.policy.portrait_roi
    portrait = cv2.cvtColor(
        card[int(y0 * 540) : int(y1 * 540), int(x0 * 856) : int(x1 * 856)], cv2.COLOR_BGR2RGB
    )
    faces = model.detect(portrait).face_landmarks
    if len(faces) != 1:
        raise DocumentRejected()
    if not model.quality(portrait, faces[0]):
        raise DocumentRejected("CCCD_IMAGE_QUALITY_LOW")
    return model.embed(model.aligned(portrait, faces[0]))
