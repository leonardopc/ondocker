FROM --platform=$BUILDPLATFORM golang:1.18.2-alpine3.15 AS build
ARG TARGETOS TARGETARCH
COPY ./src /go/src/ondocker
WORKDIR /go/src/ondocker
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /go/bin/ondocker

FROM --platform=$BUILDPLATFORM alpine AS execute
ARG BUILD_DATE VERSION
WORKDIR /root/
COPY ./config/static /ondocker/static
COPY ./config/config.json /ondocker
COPY docker-entrypoint.sh /ondocker
RUN chmod +x /ondocker/docker-entrypoint.sh
COPY --from=build /go/bin/ondocker /ondocker/ondocker
ENTRYPOINT ["/ondocker/docker-entrypoint.sh"]
CMD ["/ondocker/ondocker"]
LABEL org.opencontainers.image.created=$BUILD_DATE org.opencontainers.image.version=$VERSION org.opencontainers.image.authors=github.com/leonardopc org.opencontainers.image.url=github.com/leonardopc/ondocker org.opencontainers.image.title=ondocker 
EXPOSE 10000