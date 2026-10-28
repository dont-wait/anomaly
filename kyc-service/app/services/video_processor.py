"""Bounded incremental decode shared by file and streaming verification."""

import subprocess
import tempfile
import threading
import time

import numpy as np


class VideoDecoder:
    width, height, fps = 640, 480, 15

    def __init__(self, executable="ffmpeg", timeout=30, source=None, container="matroska"):
        self.process = subprocess.Popen(
            [
                executable,
                "-hide_banner",
                "-loglevel",
                "error",
                "-nostdin",
                "-threads",
                "1",
                "-max_pixels",
                "2073600",
                "-probesize",
                "32768",
                "-analyzeduration",
                "100000",
                "-protocol_whitelist",
                "pipe,file" if source else "pipe",
                "-f",
                container,
                "-i",
                source or "pipe:0",
                "-an",
                "-sn",
                "-dn",
                "-vf",
                "fps=15,scale=640:480:force_original_aspect_ratio=decrease,"
                "pad=640:480:(ow-iw)/2:(oh-ih)/2",
                "-threads",
                "1",
                "-pix_fmt",
                "rgb24",
                "-f",
                "rawvideo",
                "pipe:1",
            ],
            stdin=subprocess.DEVNULL if source else subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL,
            bufsize=0,
        )
        self.expired = False
        self.timer = threading.Timer(timeout, self._expire)
        self.timer.start()

    def _expire(self):
        self.expired = True
        self.process.kill()

    def write(self, chunk):
        view = memoryview(chunk)
        while view:
            written = self.process.stdin.write(view)
            if not written:
                raise RuntimeError("decoder closed its input")
            view = view[written:]

    def finish(self):
        if not self.process.stdin.closed:
            self.process.stdin.close()

    def frames(self, max_frames=300):
        size = self.width * self.height * 3
        index = 0
        while True:
            data = bytearray()
            while len(data) < size:
                chunk = self.process.stdout.read(size - len(data))
                if not chunk:
                    break
                data.extend(chunk)
            if not data:
                break
            if len(data) != size or index >= max_frames:
                raise ValueError("invalid or excessive video frames")
            yield (
                np.frombuffer(data, np.uint8).reshape(self.height, self.width, 3),
                index / self.fps,
            )
            index += 1
        code = self.process.wait(timeout=2)
        if self.expired:
            raise TimeoutError("video decoding exceeded processing budget")
        if code or index == 0:
            raise ValueError("video cannot be decoded")

    def close(self):
        self.timer.cancel()
        if self.process.poll() is None:
            self.process.kill()
        self.process.wait(timeout=2)
        for stream in (self.process.stdin, self.process.stdout):
            if stream:
                stream.close()


def analyze_file(data, evidence):
    if data[:4] == b"\x1a\x45\xdf\xa3":
        container = "matroska"
    elif data[4:8] == b"ftyp":
        container = "mov"
    else:
        raise ValueError("unsupported video container")
    # A finalized MP4 can keep its index at the end. A seekable private file
    # is needed for final verification even though live WebM uses a pipe.
    with tempfile.NamedTemporaryFile(prefix="anomaly-kyc-", suffix=".media") as temporary:
        temporary.write(data)
        temporary.flush()
        decoder = VideoDecoder(
            evidence.model.policy.ffmpeg, source=temporary.name, container=container
        )
        started = time.monotonic()
        try:
            for rgb, timestamp in decoder.frames():
                if time.monotonic() - started > 30:
                    raise TimeoutError("inference exceeded processing budget")
                evidence.observe(rgb, timestamp)
        finally:
            decoder.close()
