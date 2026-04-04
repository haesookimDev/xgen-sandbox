FROM ghcr.io/xgen-sandbox/runtime-base:latest

USER root

RUN curl -fsSL https://deb.nodesource.com/setup_20.x | bash - && \
    apt-get install -y nodejs && \
    npm install -g yarn pnpm && \
    rm -rf /var/lib/apt/lists/*

USER sandbox
WORKDIR /home/sandbox/workspace
