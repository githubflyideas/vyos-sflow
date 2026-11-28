# syntax=docker/dockerfile:1
# vyos-sflow image: linux/amd64 only (other architectures are out of scope).
#
# The binary links DuckDB through cgo, so the build mirrors the release
# build in .github/workflows/ci.yml: external link mode, fully static,
# with the libstdc++ archive made findable and getentropy wrapped.
# The result runs in an empty runtime layer; no libc is needed.

ARG GO_VERSION=1.27.4

FROM golang:${GO_VERSION} AS build
RUN apt-get update \
 && apt-get install -y --no-install-recommends g++ \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG VERSION=dev
ARG COMMIT=unknown
ENV CGO_ENABLED=1 GOFLAGS=-buildvcs=false
RUN static_dir="$(mktemp -d)" \
 && ln -s "$(g++ -print-file-name=libstdc++.a)" "$static_dir/libstdc++.a" \
 && CGO_LDFLAGS="-L$static_dir" go build -trimpath -tags netgo,osusergo \
      -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -linkmode=external -extldflags '-static -Wl,--wrap=getentropy'" \
      -o /out/vyos-sflow ./cmd/vyos-sflow \
 && ldd /out/vyos-sflow 2>&1 | grep -Eq 'not a dynamic executable|statically linked'

# busybox provides sh and wget (health check) next to the static binary.
FROM busybox:1.37-musl

LABEL org.opencontainers.image.title="vyos-sflow" \
      org.opencontainers.image.description="Local sFlow traffic analysis for VyOS and Linux: one binary, embedded DuckDB, TUI and Web UI" \
      org.opencontainers.image.source="https://github.com/githubflyideas/vyos-sflow" \
      org.opencontainers.image.licenses="Apache-2.0"

COPY --from=build /out/vyos-sflow /usr/local/bin/vyos-sflow

# Accounts and history (embedded DuckDB + Parquet) live in the data
# directory; keep it on a volume. A named volume inherits this ownership;
# with a bind mount, chown the host directory to 65532 or pass
# --user $(id -u):$(id -g).
RUN mkdir -p /data && chown 65532:65532 /data
WORKDIR /data

# Unlike a direct install, the Web UI binds all container interfaces so
# published ports (or host networking) can reach it; the sFlow listener
# already defaults to all interfaces. Both can be overridden with
# -e VYOS_SFLOW_ADDR=... / -e VYOS_SFLOW_LISTEN=... or with flags after
# the image name.
ENV TZ=UTC \
    VYOS_SFLOW_ADDR=0.0.0.0:8066 \
    VYOS_SFLOW_DATA=/data
VOLUME /data
EXPOSE 8066/tcp 6343/udp

USER 65532:65532

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s \
  CMD wget -q -O /dev/null http://127.0.0.1:8066/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/vyos-sflow"]
CMD ["serve"]
