FROM ubuntu:22.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    wget \
    git \
    build-essential \
    ca-certificates \
    sudo \
    vim \
    unzip \
    jq \
    && rm -rf /var/lib/apt/lists/*

# Create sandbox user
RUN groupadd -g 1000 sandbox && \
    useradd -u 1000 -g sandbox -m -s /bin/bash sandbox && \
    echo "sandbox ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers

# Create workspace
RUN mkdir -p /home/sandbox/workspace && \
    chown -R sandbox:sandbox /home/sandbox

USER sandbox
WORKDIR /home/sandbox/workspace

CMD ["sleep", "infinity"]
