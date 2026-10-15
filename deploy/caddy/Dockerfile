# syntax=docker/dockerfile:1.27.1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e
# muelle-caddy (contrato §3.4 y §5.4): Caddy con el módulo de DNS de
# Cloudflare (dns.providers.cloudflare, para el DNS-01 de Let's Encrypt), en
# versiones fijas. Sin CMD ni configuración propios: ingress crea el
# contenedor con `caddy run --resume` y CADDY_ADMIN en el socket
# (naming.CaddyCommand y CaddyEnv). Desde la raíz del repo:
#   docker build --build-arg VERSION=v0.1.0 -t ghcr.io/cpu-commits/muelle-caddy:v0.1.0 deploy/caddy
#
# Las dos imágenes base son de la misma versión de Caddy, con tag exacto y el
# digest del índice (todas las arquitecturas); se publica solo linux/amd64.

FROM caddy:2.11.7-builder-alpine@sha256:d6161a2c82fd58b3205048958fb6246d0c5d892beee8a47e90944610c4efd100 AS builder
# El builder deja el binario en /usr/bin/caddy (su WORKDIR), con
# cap_net_bind_service (XCADDY_SETCAP=1).
RUN xcaddy build v2.11.7 --with github.com/caddy-dns/cloudflare@v0.2.4

FROM caddy:2.11.7-alpine@sha256:d8542f48d34a9cf4e4c11a478865229840e87e4c96ea3f439101f31a5d35f75f
COPY --from=builder /usr/bin/caddy /usr/bin/caddy
# Falla el build si el módulo no quedó adentro.
RUN caddy list-modules | grep -qx dns.providers.cloudflare
# La versión es la de muelle (el mismo tag que su imagen), no la de Caddy que
# trae la etiqueta de la imagen base.
ARG VERSION=dev
LABEL org.opencontainers.image.source="https://github.com/CPU-commits/muelle" \
      org.opencontainers.image.title="muelle-caddy" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.description="Caddy con el DNS de Cloudflare, el proxy de muelle"
