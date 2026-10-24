# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.27.3
FROM golang:${GO_VERSION}-alpine AS build

ARG VERSION=dev
ARG COMMIT=unknown
ARG COMMIT_TIME=unknown
ARG MODIFIED=unknown
ARG BUILD_DATE=unknown
ARG BUILT_BY=unknown
ARG BUILD_HOST_OS=unknown
ARG BUILD_HOST_KERNEL=unknown

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w \
      -X git.seasonalnet.org/SeasonalNet/keebacked/internal/buildinfo.version=${VERSION} \
      -X git.seasonalnet.org/SeasonalNet/keebacked/internal/buildinfo.commit=${COMMIT} \
      -X git.seasonalnet.org/SeasonalNet/keebacked/internal/buildinfo.commitTime=${COMMIT_TIME} \
      -X git.seasonalnet.org/SeasonalNet/keebacked/internal/buildinfo.modified=${MODIFIED} \
      -X git.seasonalnet.org/SeasonalNet/keebacked/internal/buildinfo.buildDate=${BUILD_DATE} \
      -X git.seasonalnet.org/SeasonalNet/keebacked/internal/buildinfo.builtBy=${BUILT_BY} \
      -X git.seasonalnet.org/SeasonalNet/keebacked/internal/buildinfo.buildHostOS=${BUILD_HOST_OS} \
      -X git.seasonalnet.org/SeasonalNet/keebacked/internal/buildinfo.buildHostKernel=${BUILD_HOST_KERNEL}" \
    -o /out/keebacked \
    ./cmd/keebacked

FROM scratch

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
ARG SOURCE_URL=https://git.seasonalnet.org/SeasonalNet/keebacked

LABEL org.opencontainers.image.title="keebacked" \
      org.opencontainers.image.description="Verified, atomic, versioned backups for an opaque KeePass KDBX file" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.source="${SOURCE_URL}"

COPY --from=build /out/keebacked /keebacked
COPY config/keebacked.toml /etc/keebacked/keebacked.toml

USER 65532:65532
ENTRYPOINT ["/keebacked"]
