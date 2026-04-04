#!/bin/bash
set -e

# Start Xvfb
Xvfb :99 -screen 0 ${VNC_RESOLUTION}x24 &
sleep 1

# Start fluxbox window manager
fluxbox &
sleep 1

# Start VNC server
x11vnc -display :99 -forever -nopw -shared -rfbport 5900 &

# Start noVNC (websockify proxy)
websockify --web /usr/share/novnc 6080 localhost:5900 &

# Keep container running
wait
