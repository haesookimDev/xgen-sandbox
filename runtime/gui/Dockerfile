FROM ghcr.io/xgen-sandbox/runtime-base:latest

USER root

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    xvfb \
    x11vnc \
    fluxbox \
    xterm \
    novnc \
    websockify \
    && rm -rf /var/lib/apt/lists/*

# Set up VNC
ENV DISPLAY=:99
ENV VNC_RESOLUTION=1280x720

COPY entrypoint-gui.sh /usr/local/bin/entrypoint-gui.sh
RUN sed -i 's/\r$//' /usr/local/bin/entrypoint-gui.sh && \
    chmod +x /usr/local/bin/entrypoint-gui.sh

USER sandbox
WORKDIR /home/sandbox/workspace

CMD ["/usr/local/bin/entrypoint-gui.sh"]
