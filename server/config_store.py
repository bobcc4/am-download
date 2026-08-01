from __future__ import annotations

import os
import shutil
import tempfile
import threading
from pathlib import Path
from typing import Any

import yaml


class ConfigStore:
    FOLDER_KEYS = {
        "alac-save-folder": "ALAC",
        "atmos-save-folder": "Atmos",
        "aac-save-folder": "AAC",
        "mv-save-folder": "MV",
    }

    def __init__(self, path: Path, default_path: Path, download_root: Path) -> None:
        self.path = path
        self.default_path = default_path
        self.download_root = download_root
        self._lock = threading.RLock()
        self.ensure()

    def ensure(self) -> None:
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self.download_root.mkdir(parents=True, exist_ok=True)
        if not self.path.exists():
            shutil.copyfile(self.default_path, self.path)
        with self._lock:
            config = self._read()
            self._normalize(config)
            self._write(config)

    def _read(self) -> dict[str, Any]:
        with self.path.open("r", encoding="utf-8") as stream:
            value = yaml.safe_load(stream) or {}
        if not isinstance(value, dict):
            raise ValueError("config.yaml must contain a mapping")
        return value

    def _write(self, config: dict[str, Any]) -> None:
        fd, temp_name = tempfile.mkstemp(prefix="config-", suffix=".yaml", dir=self.path.parent)
        try:
            with os.fdopen(fd, "w", encoding="utf-8") as stream:
                yaml.safe_dump(config, stream, allow_unicode=True, sort_keys=False, width=120)
            os.replace(temp_name, self.path)
        finally:
            if os.path.exists(temp_name):
                os.unlink(temp_name)

    def _normalize(self, config: dict[str, Any]) -> None:
        root = self.download_root.resolve()
        storefront = str(config.get("storefront", "")).strip().lower()
        config["storefront"] = storefront if len(storefront) == 2 and storefront.isalpha() else "us"
        for key, fallback in self.FOLDER_KEYS.items():
            raw = str(config.get(key, fallback)).strip().replace("\\", "/")
            leaf = Path(raw).name or fallback
            target = (root / leaf).resolve()
            if root not in target.parents:
                target = root / fallback
            target.mkdir(parents=True, exist_ok=True)
            config[key] = str(target)
        config["decrypt-m3u8-port"] = "wrapper:10020"
        config["get-m3u8-port"] = "wrapper:20020"
        config["exit-on-error"] = True
        for key, fallback in (("embed-lrc-type", "word"), ("save-lrc-file-type", "line")):
            value = str(config.get(key, fallback)).strip().lower()
            config[key] = value if value in {"word", "line"} else fallback

    def public_settings(self) -> dict[str, Any]:
        with self._lock:
            config = self._read()
        token = str(config.get("media-user-token", ""))
        token_set = bool(token and token != "your-media-user-token")
        return {
            "storefront": config.get("storefront", "us"),
            "language": config.get("language", ""),
            "media_user_token": "",
            "media_user_token_set": token_set,
            "alac_max": config.get("alac-max", 192000),
            "atmos_max": config.get("atmos-max", 2768),
            "aac_type": config.get("aac-type", "aac-lc"),
            "mv_audio_type": config.get("mv-audio-type", "atmos"),
            "mv_max": config.get("mv-max", 2160),
            "embed_cover": bool(config.get("embed-cover", True)),
            "embed_lrc": bool(config.get("embed-lrc", True)),
            "embed_lrc_type": config.get("embed-lrc-type", "word"),
            "save_lrc_file": bool(config.get("save-lrc-file", False)),
            "save_lrc_file_type": config.get("save-lrc-file-type", "line"),
            "cover_size": config.get("cover-size", "5000x5000"),
            "cover_format": config.get("cover-format", "jpg"),
            "album_folder_format": config.get("album-folder-format", "{AlbumName}"),
            "playlist_folder_format": config.get("playlist-folder-format", "{PlaylistName}"),
            "song_file_format": config.get("song-file-format", "{SongNumer}. {SongName}"),
            "artist_folder_format": config.get("artist-folder-format", "{UrlArtistName}"),
            "convert_after_download": bool(config.get("convert-after-download", False)),
            "convert_format": config.get("convert-format", "flac"),
            "convert_keep_original": bool(config.get("convert-keep-original", False)),
            "alac_fix": bool(config.get("alac-fix", False)),
        }

    def update(self, values: dict[str, Any]) -> dict[str, Any]:
        mapping = {
            "storefront": "storefront",
            "language": "language",
            "alac_max": "alac-max",
            "atmos_max": "atmos-max",
            "aac_type": "aac-type",
            "mv_audio_type": "mv-audio-type",
            "mv_max": "mv-max",
            "embed_cover": "embed-cover",
            "embed_lrc": "embed-lrc",
            "embed_lrc_type": "embed-lrc-type",
            "save_lrc_file": "save-lrc-file",
            "save_lrc_file_type": "save-lrc-file-type",
            "cover_size": "cover-size",
            "cover_format": "cover-format",
            "album_folder_format": "album-folder-format",
            "playlist_folder_format": "playlist-folder-format",
            "song_file_format": "song-file-format",
            "artist_folder_format": "artist-folder-format",
            "convert_after_download": "convert-after-download",
            "convert_format": "convert-format",
            "convert_keep_original": "convert-keep-original",
            "alac_fix": "alac-fix",
        }
        with self._lock:
            config = self._read()
            for public_key, config_key in mapping.items():
                if public_key in values and values[public_key] is not None:
                    config[config_key] = values[public_key]
            if values.get("media_user_token") is not None:
                supplied = str(values["media_user_token"]).strip()
                if supplied:
                    config["media-user-token"] = supplied
                elif values.get("clear_media_user_token"):
                    config["media-user-token"] = ""
            self._normalize(config)
            self._write(config)
        return self.public_settings()
