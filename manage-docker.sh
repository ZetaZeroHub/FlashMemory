#!/usr/bin/env bash
# manage-docker.sh
# 用于构建、运行、停止、删除、查看日志 Linux/Windows 镜像和容器

set -euo pipefail

# 配置
DOCKERFILE_LINUX="Dockerfile.linux"
DOCKERFILE_WINDOWS="Dockerfile.windows"
IMAGE_LINUX="myapp-linux"
IMAGE_WINDOWS="myapp-windows"
CONTAINER_LINUX="myapp-linux-ctr"
CONTAINER_WINDOWS="myapp-windows-ctr"

function usage() {
  cat <<EOF
Usage: $0 <target> <action> [extra args]

Targets:
  linux       操作 Linux 镜像/容器
  windows     操作 Windows 镜像/容器

Actions:
  build       构建镜像
  run         运行容器 (后台模式)
  stop        停止容器
  rm          删除容器
  logs        查看容器日志
  rebuild     重建镜像 (stop, rm, build)

示例：
  $0 linux build
  $0 windows run -p 8080:80
  $0 linux logs
EOF
  exit 1
}

if [[ $# -lt 2 ]]; then
  usage
fi

TARGET=$1
ACTION=$2
shift 2

case "$TARGET" in
  linux)
    DOCKERFILE=$DOCKERFILE_LINUX
    IMAGE=$IMAGE_LINUX
    CONTAINER=$CONTAINER_LINUX
    ;;
  windows)
    DOCKERFILE=$DOCKERFILE_WINDOWS
    IMAGE=$IMAGE_WINDOWS
    CONTAINER=$CONTAINER_WINDOWS
    ;;
  *)
    echo "Unknown target: $TARGET"
    usage
    ;;
esac

case "$ACTION" in
  build)
    echo "Building image $IMAGE from $DOCKERFILE..."
    docker build -f "$DOCKERFILE" -t "$IMAGE" .
    ;;

  run)
    echo "Running container $CONTAINER from image $IMAGE..."
    docker run -d --name "$CONTAINER" "$@" "$IMAGE"
    ;;

  stop)
    echo "Stopping container $CONTAINER..."
    docker stop "$CONTAINER"
    ;;

  rm)
    echo "Removing container $CONTAINER..."
    docker rm "$CONTAINER"
    ;;

  logs)
    echo "Tailing logs from container $CONTAINER..."
    docker logs -f "$CONTAINER"
    ;;

  rebuild)
    echo "Rebuild: stop, rm, build"
    set +e
    docker stop "$CONTAINER" && docker rm "$CONTAINER"
    set -e
    docker build -f "$DOCKERFILE" -t "$IMAGE" .
    ;;

  *)
    echo "Unknown action: $ACTION"
    usage
    ;;
esac

exit 0
