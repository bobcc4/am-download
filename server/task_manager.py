from __future__ import annotations

import json
import os
import re
import signal
import subprocess
import threading
import time
import uuid
from collections import deque
from pathlib import Path
from typing import Any
from urllib.parse import urlparse


FINAL_STATES = {"completed", "failed", "cancelled"}
SUPPORTED_KINDS = {"album", "song", "playlist", "atmos", "aac", "video", "artist", "station", "debug"}


class TaskManager:
    def __init__(self, binary: str, data_dir: Path) -> None:
        self.binary = binary
        self.data_dir = data_dir
        self.history_path = data_dir / "tasks.json"
        self._tasks: dict[str, dict[str, Any]] = {}
        self._queue: deque[str] = deque()
        self._processes: dict[str, subprocess.Popen[str]] = {}
        self._lock = threading.RLock()
        self._condition = threading.Condition(self._lock)
        self._stopping = False
        self._load()
        self._worker = threading.Thread(target=self._run_worker, name="download-worker", daemon=True)
        self._worker.start()

    def _load(self) -> None:
        if not self.history_path.exists():
            return
        try:
            values = json.loads(self.history_path.read_text(encoding="utf-8"))
            for task in values:
                if task.get("state") in {"queued", "running"}:
                    task["state"] = "failed"
                    task["message"] = "Service restarted before the task finished"
                    task["finished_at"] = int(time.time())
                self._tasks[task["id"]] = task
        except (OSError, ValueError, TypeError, KeyError):
            self._tasks = {}

    def _persist_locked(self) -> None:
        ordered = sorted(self._tasks.values(), key=lambda item: item["created_at"], reverse=True)[:100]
        temp_path = self.history_path.with_suffix(".tmp")
        temp_path.write_text(json.dumps(ordered, ensure_ascii=False, indent=2), encoding="utf-8")
        os.replace(temp_path, self.history_path)

    @staticmethod
    def _validate_url(url: str) -> str:
        cleaned = url.strip()
        parsed = urlparse(cleaned)
        if parsed.scheme != "https" or parsed.hostname not in {"music.apple.com", "beta.music.apple.com"}:
            raise ValueError("Enter a valid Apple Music https URL")
        return cleaned

    def create(self, payload: dict[str, Any]) -> dict[str, Any]:
        kind = str(payload.get("kind", "album")).lower()
        if kind not in SUPPORTED_KINDS:
            raise ValueError("Unsupported download type")
        url = self._validate_url(str(payload.get("url", "")))
        task_id = uuid.uuid4().hex[:12]
        now = int(time.time())
        task = {
            "id": task_id,
            "kind": kind,
            "url": url,
            "state": "queued",
            "progress": 0,
            "message": "Waiting in queue",
            "created_at": now,
            "started_at": None,
            "finished_at": None,
            "options": {
                "aac_type": payload.get("aac_type", "aac-lc"),
                "alac_max": int(payload.get("alac_max", 192000)),
                "atmos_max": int(payload.get("atmos_max", 2768)),
                "mv_audio_type": payload.get("mv_audio_type", "atmos"),
                "mv_max": int(payload.get("mv_max", 2160)),
            },
            "logs": [],
        }
        with self._condition:
            self._tasks[task_id] = task
            self._queue.append(task_id)
            self._persist_locked()
            self._condition.notify()
        return self.get(task_id)

    def _argv(self, task: dict[str, Any]) -> list[str]:
        options = task["options"]
        args = [
            self.binary,
            "--alac-max", str(options["alac_max"]),
            "--atmos-max", str(options["atmos_max"]),
            "--aac-type", str(options["aac_type"]),
            "--mv-audio-type", str(options["mv_audio_type"]),
            "--mv-max", str(options["mv_max"]),
        ]
        kind = task["kind"]
        if kind == "song":
            args.append("--song")
        elif kind == "atmos":
            args.append("--atmos")
        elif kind == "aac":
            args.append("--aac")
        elif kind == "artist":
            args.append("--all-album")
        elif kind == "debug":
            args.append("--debug")
        args.append(task["url"])
        return args

    def _append_log(self, task_id: str, line: str) -> None:
        cleaned = re.sub(r"\x1b\[[0-9;]*[A-Za-z]", "", line).strip()
        if not cleaned:
            return
        with self._lock:
            task = self._tasks[task_id]
            task["logs"].append(cleaned[:1000])
            task["logs"] = task["logs"][-500:]
            percent = re.search(r"(?:^|\s)(\d{1,3})%", cleaned)
            if percent:
                task["progress"] = min(99, int(percent.group(1)))
            queue_match = re.search(r"Queue\s+(\d+)\s+of\s+(\d+)", cleaned, re.IGNORECASE)
            if queue_match and int(queue_match.group(2)) > 0:
                task["progress"] = min(95, int((int(queue_match.group(1)) - 1) * 100 / int(queue_match.group(2))))
            task["message"] = cleaned[:180]

    def _run_worker(self) -> None:
        while True:
            with self._condition:
                self._condition.wait_for(lambda: self._queue or self._stopping)
                if self._stopping:
                    return
                task_id = self._queue.popleft()
                task = self._tasks.get(task_id)
                if not task or task["state"] != "queued":
                    continue
                task["state"] = "running"
                task["started_at"] = int(time.time())
                task["message"] = "Starting downloader"
                self._persist_locked()

            try:
                process = subprocess.Popen(
                    self._argv(task),
                    cwd=self.data_dir,
                    stdin=subprocess.DEVNULL,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.STDOUT,
                    text=True,
                    encoding="utf-8",
                    errors="replace",
                    bufsize=1,
                    start_new_session=True,
                )
                with self._lock:
                    self._processes[task_id] = process
                if process.stdout:
                    for line in process.stdout:
                        self._append_log(task_id, line)
                exit_code = process.wait()
                with self._lock:
                    current = self._tasks[task_id]
                    if current["state"] != "cancelled":
                        current["state"] = "completed" if exit_code == 0 else "failed"
                        current["progress"] = 100 if exit_code == 0 else current["progress"]
                        current["message"] = "Download completed" if exit_code == 0 else f"Downloader exited with code {exit_code}"
            except Exception as exc:  # Keep the worker alive after an individual task failure.
                with self._lock:
                    current = self._tasks[task_id]
                    if current["state"] != "cancelled":
                        current["state"] = "failed"
                        current["message"] = str(exc)[:180]
                        current["logs"].append(f"ERROR: {exc}")
            finally:
                with self._lock:
                    self._processes.pop(task_id, None)
                    current = self._tasks[task_id]
                    current["finished_at"] = int(time.time())
                    self._persist_locked()

    def list(self) -> list[dict[str, Any]]:
        with self._lock:
            return [self._copy_task(item) for item in sorted(self._tasks.values(), key=lambda value: value["created_at"], reverse=True)]

    def get(self, task_id: str) -> dict[str, Any]:
        with self._lock:
            if task_id not in self._tasks:
                raise KeyError(task_id)
            return self._copy_task(self._tasks[task_id])

    @staticmethod
    def _copy_task(task: dict[str, Any]) -> dict[str, Any]:
        value = dict(task)
        value["options"] = dict(task["options"])
        value["logs"] = list(task["logs"])
        return value

    def cancel(self, task_id: str) -> dict[str, Any]:
        with self._condition:
            task = self._tasks.get(task_id)
            if not task:
                raise KeyError(task_id)
            if task["state"] in FINAL_STATES:
                return self._copy_task(task)
            task["state"] = "cancelled"
            task["message"] = "Cancelled"
            task["finished_at"] = int(time.time())
            process = self._processes.get(task_id)
            if process and process.poll() is None:
                try:
                    os.killpg(process.pid, signal.SIGTERM)
                except ProcessLookupError:
                    pass
            self._persist_locked()
            return self._copy_task(task)

    def clear_finished(self) -> None:
        with self._lock:
            self._tasks = {key: value for key, value in self._tasks.items() if value["state"] not in FINAL_STATES}
            self._persist_locked()

    def shutdown(self) -> None:
        with self._condition:
            self._stopping = True
            processes = list(self._processes.values())
            self._condition.notify_all()
        for process in processes:
            if process.poll() is None:
                try:
                    os.killpg(process.pid, signal.SIGTERM)
                except ProcessLookupError:
                    pass
