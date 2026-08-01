# AM download

AM download 是一个通过浏览器操作的 Apple Music 下载管理器。`v1.0.4` 更新了歌词解析器，并支持为嵌入歌词和独立歌词文件分别选择逐字或逐行格式。仅提供 `linux/amd64` Docker 镜像。

## 镜像

- `bobcc4/am-download:v1.0.4`：Web UI、任务服务和下载核心
- `bobcc4/am-download-wrapper:v1.0.4`：Apple Music Wrapper sidecar

两个容器需要同时运行。Wrapper 容器必须使用特权模式，且不应把它的管理端口暴露到主机。

## Docker Compose

下载 `compose.dockerhub.yaml`，在同目录创建 `.env`：

```env
DOCKERHUB_NAMESPACE=bobcc4
AM_DOWNLOAD_BIND=127.0.0.1
AM_DOWNLOAD_PORT=8080
AM_DOWNLOAD_PASSWORD=change-this-password
MANAGER_TOKEN=replace-with-a-long-random-value
TZ=Asia/Shanghai
```

启动：

```bash
docker compose -f compose.dockerhub.yaml up -d
```

浏览器访问 `http://127.0.0.1:8080`。需要从局域网访问时，把 `AM_DOWNLOAD_BIND` 改为主机局域网 IP，并务必设置 `AM_DOWNLOAD_PASSWORD`。

## 数据

| Docker 卷 | 内容 |
| --- | --- |
| `am-download-data` | 配置与最近 100 条任务记录 |
| `am-download-wrapper-data` | Apple Music 登录与 FairPlay 数据 |
| `am-download-files` | 下载结果 |

查看下载卷位置：

```bash
docker volume inspect am-download-files
```

停止服务不会删除数据：

```bash
docker compose -f compose.dockerhub.yaml down
```

不要使用 `down -v`，除非确定要删除配置、登录状态和下载内容。

## 安全边界

- Apple ID 密码只在首次登录请求期间使用，不写入 AM download 配置或任务日志。
- `media-user-token` 保存在持久化配置中，Web API 不会回传原值。
- 默认仅监听 `127.0.0.1`。
- Wrapper 基于 Linux x86-64 Android 运行环境，本版本不支持原生 ARM64。

本软件仅用于管理用户有权访问的内容。使用者需要自行确认 Apple 服务条款、当地法规及所用上游组件的分发许可。
