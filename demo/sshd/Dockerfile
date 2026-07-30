FROM debian:12-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
      openssh-server procps iproute2 python3 coreutils ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /run/sshd \
    && sed -i 's/^#\?PermitRootLogin.*/PermitRootLogin yes/' /etc/ssh/sshd_config \
    && sed -i 's/^#\?PasswordAuthentication.*/PasswordAuthentication yes/' /etc/ssh/sshd_config \
    && echo 'UsePAM no' >> /etc/ssh/sshd_config \
    && echo 'root:demo' | chpasswd
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
