FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY internal ./internal
COPY cmd ./cmd
RUN CGO_ENABLED=0 go test ./... && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/controlplane ./cmd/controlplane && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/agent ./cmd/agent

FROM alpine:3.20 AS controlplane
RUN adduser -D -u 10001 nxyz && mkdir -p /data && chown -R nxyz:nxyz /data
COPY --from=build /out/controlplane /usr/local/bin/nxyz-controlplane
USER nxyz
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/nxyz-controlplane"]

FROM alpine:3.20 AS agent
RUN adduser -D -u 10001 nxyz
COPY --from=build /out/agent /usr/local/bin/nxyz-agent
USER nxyz
ENTRYPOINT ["/usr/local/bin/nxyz-agent"]
