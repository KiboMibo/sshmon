#!/usr/bin/env bash
# Пересобирает demo/demo.gif: собирает sshmon, поднимает временный флот из трёх
# SSH-контейнеров и записывает TUI через vhs. Нужен только Docker.
#
#   bash demo/record.sh
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
NET=sshmon-demo-net
IMAGE=sshmon-demo-sshd
WORK="$(mktemp -d)"

cleanup() {
  docker rm -f sshmon-demo-web1 sshmon-demo-db1 sshmon-demo-cache1 >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT
cleanup

echo ">> собираю sshmon (linux/amd64)"
docker run --rm -v "$ROOT":/src:ro -v "$WORK":/out -w /src \
  -e CGO_ENABLED=0 -e GOOS=linux -e GOARCH=amd64 \
  golang:1.26 go build -trimpath -buildvcs=false -o /out/sshmon ./cmd/sshmon
# go build в контейнере создаёт файлы от root — возвращаем владельца текущему
# пользователю, чтобы дальше свободно писать в рабочую папку.
docker run --rm -v "$WORK":/w alpine chown -R "$(id -u):$(id -g)" /w

echo ">> собираю образ SSH-сервера"
docker build -q -t "$IMAGE" "$HERE/sshd" >/dev/null

echo ">> сеть и флот"
docker network create "$NET" >/dev/null
docker run -d --name sshmon-demo-web1   --hostname web1   --network "$NET" --network-alias web1   -e ROLE=web   "$IMAGE" >/dev/null
docker run -d --name sshmon-demo-db1    --hostname db1    --network "$NET" --network-alias db1    -e ROLE=db    "$IMAGE" >/dev/null
docker run -d --name sshmon-demo-cache1 --hostname cache1 --network "$NET" --network-alias cache1 -e ROLE=cache "$IMAGE" >/dev/null

echo ">> жду sshd"
for c in sshmon-demo-web1 sshmon-demo-db1 sshmon-demo-cache1; do
  timeout 40 bash -c "until docker logs $c 2>&1 | grep -q 'Server listening on 0.0.0.0'; do sleep 0.5; done"
done
sleep 2

echo ">> запись vhs"
cp "$HERE/config.yaml" "$HERE/demo.tape" "$WORK/"
chmod 600 "$WORK/config.yaml"
docker run --rm --network "$NET" -v "$WORK":/demo -w /demo \
  ghcr.io/charmbracelet/vhs:latest /demo/demo.tape

cp "$WORK/demo.gif" "$HERE/demo.gif"
echo ">> готово: demo/demo.gif"
