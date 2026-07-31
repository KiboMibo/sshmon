#!/usr/bin/env bash
# Одноразовый SSH-сервер для демо-флота sshmon. НЕ для продакшена: пароль root:demo.
set -e
ssh-keygen -A >/dev/null 2>&1 || true

ROLE="${ROLE:-app}"

# Именованные процессы, чтобы экран Processes выглядел живым.
spawn() { bash -c "exec -a '$1' sleep infinity" & }

# Умеренная CPU-нагрузка (не пережигаем ядро) — чтобы графики не были пустыми.
burn() {
  nice -n 15 sh -c 'while :; do i=0; while [ $i -lt 120000 ]; do i=$((i+1)); done; sleep 0.35; done' &
}

case "$ROLE" in
  web)
    spawn "nginx: master process"
    spawn "nginx: worker process"
    python3 -m http.server 80  --bind 0.0.0.0 >/dev/null 2>&1 &
    python3 -m http.server 443 --bind 0.0.0.0 >/dev/null 2>&1 &
    ;;
  db)
    spawn "postgres: checkpointer"
    spawn "postgres: walwriter"
    python3 -m http.server 5432 --bind 0.0.0.0 >/dev/null 2>&1 &
    ;;
  cache)
    spawn "redis-server *:6379"
    python3 -m http.server 6379 --bind 0.0.0.0 >/dev/null 2>&1 &
    ;;
esac
burn

# Засеваем syslog, чтобы панель логов и экран Logs показывали живые строки,
# а не падали (в контейнере нет journald).
mkdir -p /var/log
seed_log() {
  H="$(hostname)"
  {
    for i in $(seq 1 40); do
      echo "$(date '+%b %e %H:%M:%S') $H systemd[1]: Started Session $i of user root."
    done
  } > /var/log/syslog
  ( while :; do
      echo "$(date '+%b %e %H:%M:%S') $H ${ROLE}[${RANDOM}]: request handled ok (${RANDOM}ms)" >> /var/log/syslog
      sleep 2
    done ) &
}
seed_log

exec /usr/sbin/sshd -D -e
