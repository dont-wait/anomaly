import threading

import cv2
import mediapipe as mp
import numpy as np
import onnxruntime as ort

from app.services.vision_config import ModelManifest, VisionConfig


class FaceModel:
    def __init__(self, policy: VisionConfig, manifest: ModelManifest, weights, landmarks):
        self.policy = policy
        self.manifest = manifest
        if not set(policy.providers) <= set(ort.get_available_providers()):
            raise ValueError("requested inference provider unavailable")
        opts = ort.SessionOptions()
        opts.intra_op_num_threads = 1
        self.session = ort.InferenceSession(str(weights), opts, providers=policy.providers)
        inputs = self.session.get_inputs()
        if len(inputs) != 1 or inputs[0].name != manifest.input_name:
            raise ValueError("embedding model input does not match manifest")
        if inputs[0].type != "tensor(float)":
            raise ValueError("embedding adapter requires float32 input")
        if manifest.output_name not in {o.name for o in self.session.get_outputs()}:
            raise ValueError("embedding output missing")
        self.landmarker = mp.tasks.vision.FaceLandmarker.create_from_options(
            mp.tasks.vision.FaceLandmarkerOptions(
                base_options=mp.tasks.BaseOptions(model_asset_path=str(landmarks)),
                num_faces=2,
                output_face_blendshapes=True,
                output_facial_transformation_matrixes=True,
            )
        )
        # MediaPipe task instances are shared; serialize access, without blocking the event loop.
        self.lock = threading.Lock()
        self.embed(np.zeros((manifest.height, manifest.width, 3), dtype=np.uint8))

    def detect(self, rgb):
        with self.lock:
            return self.landmarker.detect(mp.Image(image_format=mp.ImageFormat.SRGB, data=rgb))

    def aligned(self, rgb, landmarks):
        h, w = rgb.shape[:2]

        # Eye centers, nose and mouth corners, ordered by image coordinates.
        def point(i):
            return np.array([landmarks[i].x * w, landmarks[i].y * h])

        eyes = sorted(
            [(point(33) + point(133)) / 2, (point(362) + point(263)) / 2], key=lambda p: p[0]
        )
        mouth = sorted([point(61), point(291)], key=lambda p: p[0])
        source = np.float32([*eyes, point(1), *mouth])
        m = self.manifest
        dest = np.float32(m.alignment_points) * np.float32([m.width, m.height])
        matrix, _ = cv2.estimateAffinePartial2D(source, dest, method=cv2.LMEDS)
        if matrix is None or not np.isfinite(matrix).all():
            raise ValueError("face alignment failed")
        return cv2.warpAffine(rgb, matrix, (m.width, m.height))

    def embed(self, aligned):
        m = self.manifest
        image = cv2.resize(aligned, (m.width, m.height)).astype(np.float32)
        if m.color == "BGR":
            image = image[:, :, ::-1]
        image = (image - np.array(m.mean, np.float32)) / np.array(m.std, np.float32)
        if m.layout == "NCHW":
            image = image.transpose(2, 0, 1)
        vector = np.asarray(
            self.session.run([m.output_name], {m.input_name: np.ascontiguousarray(image[None])})[0],
            dtype=np.float32,
        ).reshape(-1)
        norm = np.linalg.norm(vector)
        if vector.size != m.embedding_size or not np.isfinite(vector).all() or norm < 1e-8:
            raise RuntimeError("invalid model embedding")
        return vector / norm

    def quality(self, rgb, landmarks):
        h, w = rgb.shape[:2]
        xs = [p.x * w for p in landmarks]
        ys = [p.y * h for p in landmarks]
        if min(xs) < 0 or max(xs) >= w or min(ys) < 0 or max(ys) >= h:
            return False
        x0, x1 = max(0, int(min(xs))), min(w, int(max(xs)))
        y0, y1 = max(0, int(min(ys))), min(h, int(max(ys)))
        if min(x1 - x0, y1 - y0) < self.policy.min_face_pixels:
            return False
        gray = cv2.cvtColor(rgb[y0:y1, x0:x1], cv2.COLOR_RGB2GRAY)
        return (
            self.policy.min_brightness <= gray.mean() <= self.policy.max_brightness
            and np.mean(gray >= 245) <= self.policy.max_glare_ratio
            and cv2.Laplacian(gray, cv2.CV_64F).var() >= self.policy.min_blur
        )


def similarity(a, b):
    return float(np.clip((np.dot(a, b) + 1) / 2, 0, 1))
