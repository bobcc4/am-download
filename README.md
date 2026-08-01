# AM download

[English](README_EN.md) | 中文

AM download 是一个带 Web 界面的 Apple Music 下载管理器。它将原下载核心的命令行参数转换为浏览器操作，并提供 Docker Compose 与飞牛 fnOS 部署配置。

> 本项目仅用于管理使用者有权访问的内容。使用者需要自行遵守 Apple 服务条款、当地法律和上游组件许可证。

## 功能

- Web 界面创建、查看、取消和清理下载任务
- 支持专辑、单曲、播放列表、艺人、Dolby Atmos、AAC 和音乐视频
- 支持 ALAC、Atmos、AAC-LC、AAC、AAC Binaural 与 AAC Downmix
- 在网页中完成 Apple ID 登录、2FA 验证和 Wrapper 重启
- 支持嵌入封面和下载后格式转换
- 嵌入歌词可独立选择逐字或逐行
- 独立歌词文件可独立选择逐字或逐行
- 配置、登录状态、下载结果持久化保存
- 针对 Docker 和飞牛 fnOS 的双容器部署

## 架构

AM download 由两个容器组成：

| 服务 | 镜像 | 用途 |
| --- | --- | --- |
| `app` | `bobcc4/am-download:v1.0.4` | Web UI、API、任务管理和下载核心 |
| `wrapper` | `bobcc4/am-download-wrapper:v1.0.4` | Apple Music Android/FairPlay 辅助服务 |

Wrapper 必须以特权模式运行，但其管理端口不应映射到主机。默认通过主机端口 `8333` 访问，映射到 App 容器内部的 `8080`。

当前 Docker 镜像仅提供 `linux/amd64`。ARM64 NAS 需要启用模拟，性能和兼容性不作保证。

## 快速部署

下面的 Compose 可直接用于 Docker Compose 或飞牛 fnOS。部署前修改两处 `change-this-manager-token`，确保值完全相同；再修改 `change-this-password`。

```yaml
name: am-download

services:
  wrapper:
    image: bobcc4/am-download-wrapper:v1.0.4
    platform: linux/amd64
    container_name: am-download-wrapper
    restart: unless-stopped
    privileged: true
    environment:
      TZ: Asia/Shanghai
      # 修改此值，并确保与 app 中的值完全相同。
      MANAGER_TOKEN: change-this-manager-token
    volumes:
      - ./wrapper-data:/app/rootfs/data
    networks:
      - am-download
    healthcheck:
      test: ["CMD", "/usr/local/bin/wrapper-manager", "healthcheck"]
      interval: 15s
      timeout: 5s
      start_period: 20s
      retries: 5

  app:
    image: bobcc4/am-download:v1.0.4
    platform: linux/amd64
    container_name: am-download
    restart: unless-stopped
    depends_on:
      wrapper:
        condition: service_healthy
    environment:
      TZ: Asia/Shanghai
      MANAGER_TOKEN: change-this-manager-token
      # 部署前修改 Web 登录密码。
      AM_DOWNLOAD_PASSWORD: change-this-password
    ports:
      - name: web
        target: 8080
        published: "8333"
        host_ip: 0.0.0.0
        protocol: tcp
    volumes:
      - ./app-data:/data
      - ./downloads:/downloads
    networks:
      - am-download
    read_only: true
    tmpfs:
      - /tmp:size=256m,mode=1777
    cap_drop:
      - ALL
    cap_add:
      - CHOWN
      - DAC_OVERRIDE
      - FOWNER
      - SETGID
      - SETUID
    security_opt:
      - no-new-privileges:true

networks:
  am-download:
    name: am-download
    driver: bridge
```

仓库内也提供相同的 [compose.fnos.yaml](compose.fnos.yaml)。飞牛用户可将以上内容直接粘贴到 Compose 项目。普通 Docker 用户保存为 `compose.yaml` 后运行：

```bash
docker compose up -d
```

默认访问地址为 `http://主机地址:8333`。需要更换主机端口时，只修改 `published: "8333"`；容器端口 `target: 8080` 必须保持不变。

持久化目录：

| 目录 | 内容 |
| --- | --- |
| `./app-data` | 配置和任务记录 |
| `./wrapper-data` | Apple 登录与 FairPlay 数据 |
| `./downloads` | 下载结果 |

这些目录会由 `app` 容器入口程序自动初始化。该 Compose 只创建 `app` 和 `wrapper` 两个容器。停止服务可运行 `docker compose down`，不要删除上述目录。

## 首次登录

1. 打开 Web 页面并输入 `AM_DOWNLOAD_PASSWORD`。
2. 进入“系统状态”，选择 Apple Music 登录。
3. 输入 Apple ID 和密码。
4. 如果 Apple 要求 2FA，直接在网页弹窗中输入六位验证码。
5. 登录完成后确认 Wrapper 状态为“已登录”。

Apple ID 密码只在当前登录请求中使用，不写入 AM download 配置或任务日志。登录状态保存在 `wrapper-data`，正常升级容器不需要重新登录。

## 下载与歌词

在“下载”页面选择任务类型并粘贴 Apple Music 链接。下载任务在后台执行，关闭浏览器不会停止容器中的任务。

歌词相关设置位于“设置”页面：

| 设置 | 可选值 | 默认值 |
| --- | --- | --- |
| 嵌入歌词 | 逐字、逐行 | 逐字 |
| 保存独立歌词文件 | 逐字、逐行 | 逐行 |

两个选项相互独立。例如可以在音频标签中嵌入逐字歌词，同时保存逐行 LRC 文件。获取歌词、AAC-LC 和音乐视频通常需要有效的 `media-user-token`。

## 更新

固定版本更新：

```bash
docker compose -f compose.dockerhub.yaml pull
docker compose -f compose.dockerhub.yaml up -d
```

飞牛用户在 Compose 项目中将两个镜像标签更新到相同版本，然后重新拉取并部署。不要删除 `app-data`、`wrapper-data` 或 `downloads`。

## 数据备份

普通 Docker Compose 使用以下命名卷：

| 卷 | 内容 |
| --- | --- |
| `am-download-data` | 配置与最近任务记录 |
| `am-download-wrapper-data` | Apple 登录状态 |
| `am-download-files` | 下载结果 |

飞牛使用绑定目录，停止 Compose 项目后直接备份三个目录即可。备份文件可能包含账号登录状态，应按敏感数据管理。

## 从源码构建

构建并启动本地版本：

```bash
docker compose -f compose.yaml build
docker compose -f compose.yaml up -d
```

前端开发：

```bash
cd web
npm install
npm run dev
```

主要目录：

| 路径 | 说明 |
| --- | --- |
| `web/` | React/Vite Web 前端 |
| `server/` | FastAPI 服务和任务管理 |
| `assets/apple-music-downloader/` | Go 下载核心 |
| `assets/Wrapper/manager/` | Wrapper 管理服务 |
| `Dockerfile` | App 多阶段构建 |
| `compose.fnos.yaml` | 飞牛部署配置 |

## 测试

```bash
cd assets/apple-music-downloader
go test ./...

cd ../../server
python -m unittest discover -s tests -v

cd ../web
npm run build
```

## 故障排查

### 页面无法访问

- 检查 Compose 中 `published` 端口是否被占用。
- 确认只修改主机端口，容器端口保持 `8080`。
- 使用 `docker compose ps` 确认两个容器均为 healthy。

### App 显示部分运行

App 会等待 Wrapper 健康后启动。检查：

```bash
docker logs --tail=200 am-download-wrapper
docker logs --tail=200 am-download
```

### 2FA 超时

保持登录弹窗打开，在 60 秒内将六位验证码提交到网页。不要在主机上手动创建 `2fa.txt`；Web 服务会把验证码写入 Wrapper 所需位置。

### 修改主机端口后不可用

正确格式是 `主机端口:8080`。飞牛长语法中只修改 `published`，不要修改 `target`。

### 权限错误

确认飞牛项目目录可被 Docker 访问。当前入口程序会在 `app` 容器内部初始化 `app-data` 和 `downloads` 权限；正常部署的容器列表应仅包含 `app` 和 `wrapper`。

## 上游与许可

本项目基于以下开源项目继续开发：

- [2744317644/apple_music_downloader](https://github.com/2744317644/apple_music_downloader)
- [zhaarey/apple-music-downloader](https://github.com/zhaarey/apple-music-downloader)
- [WorldObservationLog/wrapper](https://github.com/WorldObservationLog/wrapper)

各目录和组件继续受其各自上游许可证约束。本仓库当前未声明覆盖全部组合代码的额外许可证；二次分发前请核对相关上游许可。
