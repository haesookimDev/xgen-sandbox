FROM golang:1.22-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /sidecar ./cmd/sidecar

FROM alpine:3.19

RUN apk add --no-cache ca-certificates bash
COPY --from=builder /sidecar /usr/local/bin/sidecar

USER 1000:1000
EXPOSE 9000 9001

ENTRYPOINT ["/usr/local/bin/sidecar"]
