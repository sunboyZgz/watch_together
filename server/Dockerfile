# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates

ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=sum.golang.google.cn

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/roomserver \
    ./cmd/roomserver && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/outboxworker \
    ./cmd/outboxworker && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/derivedworker \
    ./cmd/derivedworker && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/mediaservice \
    ./cmd/mediaservice && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/timelineservice \
    ./cmd/timelineservice

FROM scratch

WORKDIR /app
COPY --from=builder /out/roomserver /app/roomserver
COPY --from=builder /out/outboxworker /app/outboxworker
COPY --from=builder /out/derivedworker /app/derivedworker
COPY --from=builder /out/mediaservice /app/mediaservice
COPY --from=builder /out/timelineservice /app/timelineservice

EXPOSE 8080
USER 65532:65532

ENTRYPOINT ["/app/roomserver"]
