FROM ghcr.io/xgen-sandbox/runtime-gui:latest

USER root

RUN apt-get update && apt-get install -y --no-install-recommends \
    chromium-browser \
    fonts-liberation \
    libnss3 \
    sudo \
    && echo "sandbox ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/sandbox \
    && chmod 0440 /etc/sudoers.d/sandbox \
    && rm -rf /var/lib/apt/lists/*

USER sandbox
WORKDIR /home/sandbox/workspace
