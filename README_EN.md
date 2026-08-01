# AM download

[中文](README.md) | English

AM download is a browser-based Apple Music download manager with Docker Compose and fnOS deployment support.

> Use this project only for content you are authorized to access. You are responsible for complying with Apple terms, local law, and upstream licenses.

## Features

- Web UI for creating, monitoring, cancelling, and clearing download jobs
- Albums, songs, playlists, artists, Dolby Atmos, AAC, and music videos
- Apple ID login and 2FA in the browser
- Persistent configuration, login state, and downloads
- Independent word-timed or line-timed lyrics for embedded metadata and sidecar files
- Docker Compose configuration tailored for fnOS

## Images

| Service | Image |
| --- | --- |
| App | `bobcc4/am-download:v1.0.4` |
| Wrapper | `bobcc4/am-download-wrapper:v1.0.4` |

The published runtime images currently support `linux/amd64` only.

## Docker Compose

```bash
git clone https://github.com/bobcc4/am-download.git
cd am-download
docker compose -f compose.dockerhub.yaml up -d
```

Edit `compose.dockerhub.yaml` directly before starting. Change `MANAGER_TOKEN` to the same private value in both services and set a strong `AM_DOWNLOAD_PASSWORD`. No `.env` file is required.

Open `http://host-address:8333`. The host port can be changed in Compose, but the container port must remain `8080`.

## fnOS

Use [compose.fnos.yaml](compose.fnos.yaml) or paste the complete Compose shown in the Chinese [README](README.md). Change `MANAGER_TOKEN` and `AM_DOWNLOAD_PASSWORD` directly in the Compose editor.

The default mapping is host port `8333` to container port `8080`. To use another host port, change only `published`; keep `target: 8080`.

Persistent directories:

| Directory | Content |
| --- | --- |
| `./app-data` | Configuration and task history |
| `./wrapper-data` | Apple login and FairPlay state |
| `./downloads` | Downloaded files |

## Lyrics

The settings page provides separate modes for embedded lyrics and sidecar lyrics:

- `word`: word-timed lyrics
- `line`: line-timed lyrics

The two outputs can use different modes. Existing installations are migrated automatically with embedded lyrics defaulting to `word` and sidecar lyrics defaulting to `line`.

## Upgrade

```bash
docker compose -f compose.dockerhub.yaml pull
docker compose -f compose.dockerhub.yaml up -d
```

Do not remove the persistent volumes or fnOS bind directories during an upgrade.

## Build and test

```bash
docker compose -f compose.yaml build

cd assets/apple-music-downloader
go test ./...

cd ../../server
python -m unittest discover -s tests -v

cd ../web
npm run build
```

## Upstream and licensing

This repository builds on:

- [2744317644/apple_music_downloader](https://github.com/2744317644/apple_music_downloader)
- [zhaarey/apple-music-downloader](https://github.com/zhaarey/apple-music-downloader)
- [WorldObservationLog/wrapper](https://github.com/WorldObservationLog/wrapper)

Individual directories and components remain subject to their upstream licenses. This repository does not currently declare an additional license covering the combined work.
