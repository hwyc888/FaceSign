#!/usr/bin/env python3
"""FaceSign local CPU face engine.

Implements the small subset of the CompreFace recognition API used by FaceSign,
but runs entirely on CPU with OpenCV YuNet + SFace ONNX models.
"""

from __future__ import annotations

import argparse
import os
import sqlite3
import threading
import time
import uuid
from pathlib import Path
from typing import Dict, List

import cv2
import numpy as np
from fastapi import FastAPI, File, HTTPException, Query, UploadFile

ENGINE = None
app = FastAPI(title="FaceSign Local CPU Face Engine", docs_url=None, redoc_url=None)


class FaceEngine:
    def __init__(self, models_dir: Path, database_path: Path) -> None:
        self.models_dir = models_dir
        self.database_path = database_path
        self.database_path.parent.mkdir(parents=True, exist_ok=True)
        detector_model = models_dir / "face_detection_yunet_2023mar.onnx"
        recognizer_model = models_dir / "face_recognition_sface_2021dec.onnx"
        if not detector_model.is_file():
            raise RuntimeError(f"missing YuNet model: {detector_model}")
        if not recognizer_model.is_file():
            raise RuntimeError(f"missing SFace model: {recognizer_model}")

        self.detector = cv2.FaceDetectorYN.create(
            str(detector_model), "", (320, 320), 0.80, 0.30, 5000
        )
        self.recognizer = cv2.FaceRecognizerSF.create(str(recognizer_model), "")
        self.lock = threading.RLock()
        self.embeddings: Dict[str, List[np.ndarray]] = {}
        self._migrate()
        self._reload_cache()

    def _connect(self) -> sqlite3.Connection:
        conn = sqlite3.connect(self.database_path, timeout=10)
        conn.execute("PRAGMA journal_mode=WAL")
        conn.execute("PRAGMA synchronous=NORMAL")
        return conn

    def _migrate(self) -> None:
        with self._connect() as conn:
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS face_embeddings (
                    id TEXT PRIMARY KEY,
                    subject TEXT NOT NULL,
                    embedding BLOB NOT NULL,
                    created_at INTEGER NOT NULL
                )
                """
            )
            conn.execute(
                "CREATE INDEX IF NOT EXISTS idx_face_embeddings_subject ON face_embeddings(subject)"
            )

    def _reload_cache(self) -> None:
        cache: Dict[str, List[np.ndarray]] = {}
        with self._connect() as conn:
            rows = conn.execute(
                "SELECT subject, embedding FROM face_embeddings ORDER BY created_at"
            ).fetchall()
        for subject, blob in rows:
            vector = np.frombuffer(blob, dtype=np.float32).copy()
            if vector.size == 0:
                continue
            norm = float(np.linalg.norm(vector))
            if norm <= 0:
                continue
            vector /= norm
            cache.setdefault(subject, []).append(vector)
        self.embeddings = cache

    @staticmethod
    def decode_image(raw: bytes) -> np.ndarray:
        if not raw:
            raise HTTPException(status_code=400, detail="图片为空")
        if len(raw) > 12 * 1024 * 1024:
            raise HTTPException(status_code=413, detail="图片过大")
        image = cv2.imdecode(np.frombuffer(raw, dtype=np.uint8), cv2.IMREAD_COLOR)
        if image is None or image.size == 0:
            raise HTTPException(status_code=400, detail="无法解析图片")
        return image

    def detect(self, image: np.ndarray, threshold: float) -> List[np.ndarray]:
        height, width = image.shape[:2]
        if width < 40 or height < 40:
            return []
        threshold = min(0.99, max(0.10, float(threshold)))
        self.detector.setInputSize((width, height))
        if hasattr(self.detector, "setScoreThreshold"):
            self.detector.setScoreThreshold(threshold)
        _, faces = self.detector.detect(image)
        if faces is None:
            return []
        return [faces[index] for index in range(faces.shape[0])]

    def feature(self, image: np.ndarray, face: np.ndarray) -> np.ndarray:
        aligned = self.recognizer.alignCrop(image, face)
        feature = self.recognizer.feature(aligned).reshape(-1).astype(np.float32)
        norm = float(np.linalg.norm(feature))
        if norm <= 0:
            raise HTTPException(status_code=422, detail="无法提取有效人脸特征")
        return feature / norm

    def enroll(self, subject: str, image: np.ndarray, threshold: float) -> str:
        subject = subject.strip()
        if not subject:
            raise HTTPException(status_code=400, detail="subject 不能为空")
        with self.lock:
            faces = self.detect(image, threshold)
            if not faces:
                raise HTTPException(status_code=422, detail="未检测到清晰正脸，请调整光线和角度后重试")
            if len(faces) > 1:
                raise HTTPException(status_code=409, detail="画面中检测到多张人脸，请只保留一名学生")
            vector = self.feature(image, faces[0])
            image_id = uuid.uuid4().hex
            with self._connect() as conn:
                conn.execute(
                    "INSERT INTO face_embeddings(id, subject, embedding, created_at) VALUES(?,?,?,?)",
                    (image_id, subject, vector.astype(np.float32).tobytes(), int(time.time())),
                )
            self.embeddings.setdefault(subject, []).append(vector)
            return image_id

    def recognize(self, image: np.ndarray, threshold: float) -> List[dict]:
        with self.lock:
            faces = self.detect(image, threshold)
            results = []
            for face in faces:
                vector = self.feature(image, face)
                best_subject = ""
                best_score = -1.0
                for subject, samples in self.embeddings.items():
                    for sample in samples:
                        raw_cosine = float(np.dot(vector, sample))
                        # SFace cosine is [-1, 1]. Map to [0, 1] so FaceSign's
                        # existing threshold UI remains intuitive.
                        score = max(0.0, min(1.0, (raw_cosine + 1.0) / 2.0))
                        if score > best_score:
                            best_score = score
                            best_subject = subject
                subjects = []
                if best_subject:
                    subjects.append({"subject": best_subject, "similarity": best_score})
                results.append({"subjects": subjects})
            return results

    def subject_names(self) -> List[str]:
        with self.lock:
            return sorted(self.embeddings.keys())

    def sample_count(self) -> int:
        with self.lock:
            return sum(len(samples) for samples in self.embeddings.values())


def require_engine() -> FaceEngine:
    if ENGINE is None:
        raise HTTPException(status_code=503, detail="本地 CPU 人脸引擎尚未初始化")
    return ENGINE


@app.get("/health")
def health() -> dict:
    engine = require_engine()
    return {
        "ok": True,
        "engine": "facesign-localcpu",
        "device": "cpu",
        "gpu_required": False,
        "subjects": len(engine.subject_names()),
        "samples": engine.sample_count(),
    }


@app.get("/api/v1/recognition/subjects/")
def subjects() -> dict:
    engine = require_engine()
    return {"subjects": engine.subject_names()}


@app.post("/api/v1/recognition/faces/")
async def enroll_face(
    subject: str = Query(..., min_length=1),
    det_prob_threshold: float = Query(0.80, ge=0.0, le=1.0),
    file: UploadFile = File(...),
) -> dict:
    engine = require_engine()
    raw = await file.read()
    image = engine.decode_image(raw)
    image_id = engine.enroll(subject, image, det_prob_threshold)
    return {"image_id": image_id, "subject": subject}


@app.post("/api/v1/recognition/recognize")
async def recognize_face(
    prediction_count: int = Query(1, ge=1, le=10),
    det_prob_threshold: float = Query(0.80, ge=0.0, le=1.0),
    file: UploadFile = File(...),
) -> dict:
    del prediction_count  # FaceSign only needs the best subject for each detected face.
    engine = require_engine()
    raw = await file.read()
    image = engine.decode_image(raw)
    return {"result": engine.recognize(image, det_prob_threshold)}


def main() -> None:
    parser = argparse.ArgumentParser(description="FaceSign local CPU face recognition engine")
    parser.add_argument("--host", default=os.environ.get("FACESIGN_FACE_ENGINE_HOST", "127.0.0.1"))
    parser.add_argument("--port", type=int, default=int(os.environ.get("FACESIGN_FACE_ENGINE_PORT", "18081")))
    parser.add_argument("--models", default=os.environ.get("FACESIGN_FACE_ENGINE_MODELS", str(Path(__file__).resolve().parent / "models")))
    parser.add_argument("--data", default=os.environ.get("FACESIGN_FACE_ENGINE_DB", str(Path(__file__).resolve().parent / "data" / "faces.db")))
    args = parser.parse_args()

    global ENGINE
    ENGINE = FaceEngine(Path(args.models).resolve(), Path(args.data).resolve())

    import uvicorn

    uvicorn.run(app, host=args.host, port=args.port, log_level="info", access_log=False)


if __name__ == "__main__":
    main()
