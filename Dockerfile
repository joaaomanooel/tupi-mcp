FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine3.24 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
RUN set -eu; \
    if [ "$TARGETARCH" = "arm" ]; then \
        GOARM="${TARGETVARIANT#v}" GOOS="$TARGETOS" GOARCH="$TARGETARCH" CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/tupi-mcp ./cmd/tupi-mcp; \
    else \
        GOOS="$TARGETOS" GOARCH="$TARGETARCH" CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/tupi-mcp ./cmd/tupi-mcp; \
    fi

FROM alpine:3.24

RUN apk add --no-cache ca-certificates && addgroup -S -g 65532 app && adduser -S -D -H -u 65532 -G app app
COPY --from=build --chown=65532:65532 /out/tupi-mcp /tupi-mcp

USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/tupi-mcp"]
