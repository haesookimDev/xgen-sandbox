FROM golang:1.26-bookworm AS go

FROM ghcr.io/xgen-sandbox/runtime-base:latest

USER root

COPY --from=go /usr/local/go /usr/local/go

RUN mkdir -p /home/sandbox/go && chown -R sandbox:sandbox /home/sandbox/go

ENV GOPATH=/home/sandbox/go
ENV PATH=/usr/local/go/bin:/home/sandbox/go/bin:$PATH

USER sandbox
WORKDIR /home/sandbox/workspace
