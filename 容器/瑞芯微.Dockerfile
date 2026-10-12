# syntax=docker/dockerfile:labs

# 0. 准备镜像
ARG PYTHON_VERSION="3.13-slim-bookworm"
ARG GO_VERSION="1.25-bookworm"


# 1. 构建 go2rtc 可执行文件
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION} AS build
ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH

ENV GOOS=${TARGETOS}
ENV GOARCH=${TARGETARCH}

WORKDIR /build

# 缓存依赖项
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -ldflags "-s -w" -trimpath


# 2. 最终镜像
FROM python:${PYTHON_VERSION}

# 配置 apt 以使用 BuildKit 缓存
RUN rm -f /etc/apt/apt.conf.d/docker-clean \
  && echo 'Binary::apt::APT::Keep-Downloaded-Packages "true";' >/etc/apt/apt.conf.d/keep-cache

# 安装 FFmpeg、tini（处理信号）
# 及 echo 来源所需的其他常用工具。
# libasound2-plugins 提供 ALSA 支持
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked --mount=type=cache,target=/var/lib/apt,sharing=locked \
    apt-get -y update && apt-get -y install tini \
        curl jq \
        libasound2-plugins && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

COPY --from=build /build/go2rtc /usr/local/bin/
ADD --chmod=755 https://github.com/MarcA711/Rockchip-FFmpeg-Builds/releases/download/6.1-8-no_extra_dump/ffmpeg /usr/local/bin

EXPOSE 1984 8554 8555 8555/udp
ENTRYPOINT ["/usr/bin/tini", "--"]
VOLUME /config
WORKDIR /config

CMD ["go2rtc", "-config", "/config/go2rtc.yaml"]
