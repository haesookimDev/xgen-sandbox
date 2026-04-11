FROM golang:1.24-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /sidecar ./cmd/sidecar

FROM alpine:3.21

RUN apk add --no-cache ca-certificates bash util-linux
COPY --from=builder /sidecar /usr/local/bin/sidecar

# Sidecar runs as root in the pod (with dropped caps + SYS_ADMIN only)
# to support nsenter into the runtime container's mount namespace.
EXPOSE 9000 9001

ENTRYPOINT ["/usr/local/bin/sidecar"]
