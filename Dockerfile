# syntax=docker/dockerfile:1
# vyos-sflow container image: sFlow collector + Web UI.
ARG GO_IMAGE=golang:1.27.4-bookworm
ARG RUNTIME_IMAGE=debian:bookworm-slim

FROM ${GO_IMAGE} AS build
WORKDIR /src
ENV CGO_ENABLED=1 GOTOOLCHAIN=local
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=1.3.5
ARG COMMIT=docker
RUN go build -trimpath -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
      -o /out/vyos-sflow ./cmd/vyos-sflow

FROM ${RUNTIME_IMAGE}
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --system --uid 10066 --home-dir /data --shell /usr/sbin/nologin vyos-sflow \
 && mkdir -p /data && chown vyos-sflow:vyos-sflow /data
COPY --from=build /out/vyos-sflow /usr/local/bin/vyos-sflow
ENV VYOS_SFLOW_ADDR=0.0.0.0:8066 \
    VYOS_SFLOW_LISTEN=0.0.0.0:6343 \
    VYOS_SFLOW_DATA=/data
USER vyos-sflow
WORKDIR /data
VOLUME ["/data"]
EXPOSE 8066/tcp 6343/udp
ENTRYPOINT ["/usr/local/bin/vyos-sflow"]
CMD ["serve"]
