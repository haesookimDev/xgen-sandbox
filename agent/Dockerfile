FROM golang:1.23-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /agent ./cmd/agent

FROM alpine:3.19

RUN apk add --no-cache ca-certificates
COPY --from=builder /agent /usr/local/bin/agent

USER 1000:1000
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/agent"]
