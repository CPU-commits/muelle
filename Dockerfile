# syntax=docker/dockerfile:1.27.1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e
# Imagen de muelle (contrato §5.4): el panel (web/) construido con pnpm, el
# binario estático con el panel embebido (-tags panel) y el CLI de docker
# para docker stack deploy. Desde la raíz del repo, con web/ presente:
#   docker build --build-arg VERSION=v0.1.0 -t ghcr.io/cpu-commits/muelle:v0.1.0 .
#
# Las imágenes base (y el frontend de la línea syntax) van con tag exacto y el
# digest del índice (todas las arquitecturas); se publica solo linux/amd64.

FROM node:24.22.0-alpine3.24@sha256:44fce696f38ba8c987c4db89954051000bbd05ffd8d8659ba15ed4a8602e937f AS web
WORKDIR /web
# Nunca un build con MSW: la variable del proceso manda sobre cualquier .env
# (que tampoco entra al contexto, ver .dockerignore). CI=true: pnpm sin
# preguntas y con el lockfile congelado.
ENV VITE_USE_MOCKS=0 CI=true
# La versión de packageManager en web/package.json, sin corepack.
RUN npm install -g pnpm@11.1.2
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml web/.npmrc ./
# El prepare (pnpm codegen) falla acá porque todavía no están las fuentes; el
# script lo tolera, y el codegen corre de nuevo abajo.
RUN pnpm install --frozen-lockfile
COPY web/ ./
# El build tiene que traer lo que lee Go al arrancar (200.html y la CSP) y
# nunca el service worker de MSW.
RUN pnpm codegen && pnpm build \
 && test -f build/200.html && test -s build/csp-header.txt && test ! -e build/mockServiceWorker.js

FROM golang:1.27.2-alpine3.24@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS go
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
COPY src/ ./src/
COPY --from=web /web/build/ ./src/cmd/http/panel/build/
ARG VERSION=dev
# BuildKit los define con la plataforma de destino (linux/amd64 en el release).
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -buildvcs=false -tags panel \
      -ldflags "-s -w -X main.version=${VERSION}" -o /out/muelle .

FROM docker:29.9.0-cli@sha256:1a4c7cb63513f349bdad01fcc6e0f3f2f67d37b9da86f14dc0d4a0942eecda00 AS docker-cli

FROM alpine:3.24.2@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6
# Alpine trae /tmp con 1777 (DeployStack crea ahí su DOCKER_CONFIG temporal) y
# cat (para releer la clave maestra, §12 del diseño). Corre como root: usa
# docker.sock. Sin EXPOSE: el puerto de muelle nunca se publica (solo se llega
# por el proxy), y un `docker run -P` no lo abriría sin querer.
RUN apk add --no-cache ca-certificates
COPY --from=docker-cli /usr/local/bin/docker /usr/local/bin/docker
COPY --from=go /out/muelle /usr/local/bin/muelle
ARG VERSION=dev
LABEL org.opencontainers.image.source="https://github.com/CPU-commits/muelle" \
      org.opencontainers.image.title="muelle" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.description="Deploy manager ultraligero sobre Docker Swarm"
ENTRYPOINT ["muelle"]
CMD ["serve"]
