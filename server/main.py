from __future__ import annotations

import asyncio
import json
import os
import secrets
import shutil
import urllib.error
import urllib.request
from contextlib import asynccontextmanager
from pathlib import Path
from typing import Any, Literal

from fastapi import FastAPI, HTTPException, Request, Response
from fastapi.responses import FileResponse, JSONResponse
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel, Field, field_validator

from config_store import ConfigStore
from task_manager import TaskManager


VERSION = "v1.0.4"
DATA_DIR = Path(os.environ.get("DATA_DIR", "/data"))
DOWNLOAD_DIR = Path(os.environ.get("DOWNLOAD_DIR", "/downloads"))
WEB_DIR = Path(os.environ.get("WEB_DIR", "/app/web"))
DEFAULT_CONFIG = Path(os.environ.get("DEFAULT_CONFIG", "/defaults/config.yaml.example"))
DOWNLOADER_BIN = os.environ.get("DOWNLOADER_BIN", "/usr/local/bin/apple-music-dl")
WRAPPER_URL = os.environ.get("WRAPPER_MANAGER_URL", "http://wrapper:18080").rstrip("/")
MANAGER_TOKEN = os.environ.get("MANAGER_TOKEN", "")
APP_PASSWORD = os.environ.get("AM_DOWNLOAD_PASSWORD", "")

DATA_DIR.mkdir(parents=True, exist_ok=True)
config_store = ConfigStore(DATA_DIR / "config.yaml", DEFAULT_CONFIG, DOWNLOAD_DIR)
task_manager = TaskManager(DOWNLOADER_BIN, DATA_DIR)
sessions: set[str] = set()


class AuthRequest(BaseModel):
    password: str = Field(max_length=512)


class WrapperLoginRequest(BaseModel):
    username: str = Field(min_length=3, max_length=320)
    password: str = Field(min_length=1, max_length=512)


class WrapperTwoFactorRequest(BaseModel):
    code: str = Field(pattern=r"^\d{6}$")


class TaskCreate(BaseModel):
    kind: Literal["album", "song", "playlist", "atmos", "aac", "video", "artist", "station", "debug"] = "album"
    url: str = Field(min_length=10, max_length=2048)
    aac_type: Literal["aac-lc", "aac", "aac-binaural", "aac-downmix"] = "aac-lc"
    alac_max: int = Field(default=192000, ge=44100, le=192000)
    atmos_max: int = Field(default=2768, ge=2448, le=2768)
    mv_audio_type: Literal["atmos", "ac3", "aac"] = "atmos"
    mv_max: int = Field(default=2160, ge=480, le=2160)


class SettingsUpdate(BaseModel):
    storefront: str | None = None
    language: str | None = None
    media_user_token: str | None = Field(default=None, max_length=8192)
    clear_media_user_token: bool = False
    alac_max: int | None = Field(default=None, ge=44100, le=192000)
    atmos_max: int | None = Field(default=None, ge=2448, le=2768)
    aac_type: Literal["aac-lc", "aac", "aac-binaural", "aac-downmix"] | None = None
    mv_audio_type: Literal["atmos", "ac3", "aac"] | None = None
    mv_max: int | None = Field(default=None, ge=480, le=2160)
    embed_cover: bool | None = None
    embed_lrc: bool | None = None
    embed_lrc_type: Literal["word", "line"] | None = None
    save_lrc_file: bool | None = None
    save_lrc_file_type: Literal["word", "line"] | None = None
    cover_size: str | None = Field(default=None, max_length=32)
    cover_format: Literal["jpg", "png", "original"] | None = None
    album_folder_format: str | None = Field(default=None, max_length=512)
    playlist_folder_format: str | None = Field(default=None, max_length=512)
    song_file_format: str | None = Field(default=None, max_length=512)
    artist_folder_format: str | None = Field(default=None, max_length=512)
    convert_after_download: bool | None = None
    convert_format: Literal["flac", "mp3", "opus", "wav", "copy"] | None = None
    convert_keep_original: bool | None = None
    alac_fix: bool | None = None

    @field_validator("storefront")
    @classmethod
    def validate_storefront(cls, value: str | None) -> str | None:
        if value is None:
            return None
        cleaned = value.strip().lower()
        if len(cleaned) != 2 or not cleaned.isalpha():
            raise ValueError("storefront must be a two-letter country code")
        return cleaned


def wrapper_request(path: str, method: str = "GET", payload: dict[str, Any] | None = None, timeout: int = 10) -> dict[str, Any]:
    body = json.dumps(payload).encode("utf-8") if payload is not None else None
    request = urllib.request.Request(
        WRAPPER_URL + path,
        data=body,
        method=method,
        headers={"Content-Type": "application/json", "X-Manager-Token": MANAGER_TOKEN},
    )
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            return json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        try:
            detail = json.loads(exc.read().decode("utf-8")).get("error", str(exc))
        except (ValueError, UnicodeDecodeError):
            detail = str(exc)
        raise RuntimeError(detail) from exc
    except (urllib.error.URLError, TimeoutError) as exc:
        raise RuntimeError("Wrapper service is unavailable") from exc


def authenticated(request: Request) -> bool:
    return not APP_PASSWORD or request.cookies.get("am_session") in sessions


@asynccontextmanager
async def lifespan(_: FastAPI):
    yield
    task_manager.shutdown()


app = FastAPI(title="AM download", version=VERSION, docs_url=None, redoc_url=None, lifespan=lifespan)


@app.middleware("http")
async def auth_middleware(request: Request, call_next):
    public_paths = {"/api/auth/status", "/api/auth/login", "/healthz"}
    if request.url.path.startswith("/api/") and request.url.path not in public_paths and not authenticated(request):
        return JSONResponse(status_code=401, content={"detail": "Authentication required"})
    return await call_next(request)


@app.get("/healthz")
async def healthz():
    return {"status": "ok", "version": VERSION}


@app.get("/api/auth/status")
async def auth_status(request: Request):
    return {"enabled": bool(APP_PASSWORD), "authenticated": authenticated(request)}


@app.post("/api/auth/login")
async def auth_login(payload: AuthRequest, response: Response):
    if APP_PASSWORD and not secrets.compare_digest(payload.password, APP_PASSWORD):
        raise HTTPException(status_code=401, detail="Incorrect password")
    token = secrets.token_urlsafe(32)
    sessions.add(token)
    response.set_cookie("am_session", token, httponly=True, samesite="strict", secure=False, max_age=86400 * 30)
    return {"ok": True}


@app.post("/api/auth/logout")
async def auth_logout(request: Request, response: Response):
    token = request.cookies.get("am_session")
    if token:
        sessions.discard(token)
    response.delete_cookie("am_session")
    return {"ok": True}


@app.get("/api/status")
async def status():
    try:
        wrapper = await asyncio.to_thread(wrapper_request, "/status")
    except RuntimeError as exc:
        wrapper = {"manager": "unavailable", "wrapper_running": False, "logged_in": False, "error": str(exc)}
    usage = shutil.disk_usage(DOWNLOAD_DIR)
    tasks = task_manager.list()
    return {
        "name": "AM download",
        "version": VERSION,
        "wrapper": wrapper,
        "tasks": {
            "queued": sum(item["state"] == "queued" for item in tasks),
            "running": sum(item["state"] == "running" for item in tasks),
            "completed": sum(item["state"] == "completed" for item in tasks),
            "failed": sum(item["state"] == "failed" for item in tasks),
        },
        "storage": {"total": usage.total, "used": usage.used, "free": usage.free, "path": str(DOWNLOAD_DIR)},
    }


@app.post("/api/wrapper/login")
async def wrapper_login(payload: WrapperLoginRequest):
    try:
        return await asyncio.to_thread(wrapper_request, "/login", "POST", payload.model_dump(), 250)
    except RuntimeError as exc:
        raise HTTPException(status_code=502, detail=str(exc)) from exc


@app.post("/api/wrapper/2fa")
async def wrapper_two_factor(payload: WrapperTwoFactorRequest):
    try:
        return await asyncio.to_thread(wrapper_request, "/2fa", "POST", payload.model_dump(), 130)
    except RuntimeError as exc:
        raise HTTPException(status_code=502, detail=str(exc)) from exc


@app.post("/api/wrapper/login/cancel")
async def wrapper_login_cancel():
    try:
        return await asyncio.to_thread(wrapper_request, "/login/cancel", "POST", {}, 10)
    except RuntimeError as exc:
        raise HTTPException(status_code=502, detail=str(exc)) from exc


@app.post("/api/wrapper/restart")
async def wrapper_restart():
    try:
        return await asyncio.to_thread(wrapper_request, "/restart", "POST", {}, 20)
    except RuntimeError as exc:
        raise HTTPException(status_code=502, detail=str(exc)) from exc


@app.get("/api/wrapper/logs")
async def wrapper_logs():
    try:
        return await asyncio.to_thread(wrapper_request, "/logs")
    except RuntimeError as exc:
        raise HTTPException(status_code=502, detail=str(exc)) from exc


@app.get("/api/settings")
async def settings_get():
    return config_store.public_settings()


@app.put("/api/settings")
async def settings_update(payload: SettingsUpdate):
    return config_store.update(payload.model_dump(exclude_unset=True))


@app.get("/api/tasks")
async def tasks_list():
    return {"items": task_manager.list()}


@app.post("/api/tasks", status_code=201)
async def tasks_create(payload: TaskCreate):
    try:
        status_value = await asyncio.to_thread(wrapper_request, "/status")
    except RuntimeError as exc:
        raise HTTPException(status_code=503, detail=str(exc)) from exc
    if not status_value.get("logged_in"):
        raise HTTPException(status_code=409, detail="Sign in to Apple Music before adding downloads")
    if not status_value.get("wrapper_running"):
        raise HTTPException(status_code=503, detail="Wrapper is not running")
    try:
        return task_manager.create(payload.model_dump())
    except ValueError as exc:
        raise HTTPException(status_code=422, detail=str(exc)) from exc


@app.get("/api/tasks/{task_id}")
async def task_get(task_id: str):
    try:
        return task_manager.get(task_id)
    except KeyError as exc:
        raise HTTPException(status_code=404, detail="Task not found") from exc


@app.post("/api/tasks/{task_id}/cancel")
async def task_cancel(task_id: str):
    try:
        return task_manager.cancel(task_id)
    except KeyError as exc:
        raise HTTPException(status_code=404, detail="Task not found") from exc


@app.delete("/api/tasks")
async def tasks_clear():
    task_manager.clear_finished()
    return {"ok": True}


if WEB_DIR.exists():
    assets_dir = WEB_DIR / "assets"
    if assets_dir.exists():
        app.mount("/assets", StaticFiles(directory=assets_dir), name="assets")

    @app.get("/{path:path}", include_in_schema=False)
    async def spa(path: str):
        requested = WEB_DIR / path
        if path and requested.is_file() and WEB_DIR.resolve() in requested.resolve().parents:
            return FileResponse(requested)
        return FileResponse(WEB_DIR / "index.html")
