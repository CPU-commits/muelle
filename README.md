<p align="center">
  <img src="web/src/shared/ui/assets/favicon.svg" width="72" height="72" alt="">
</p>

<h1 align="center">muelle</h1>

<p align="center">
  Despliega contenedores en tu propio servidor con Docker Swarm: panel web, API, secretos cifrados, stages y dominios con HTTPS automático.
</p>

---

muelle es un PaaS self-hosted y liviano. Despliega imágenes que ya construiste en CI, guarda sus variables y secretos, separa cada proyecto en stages y sirve cada app en sus dominios con certificados automáticos. En el servidor corren solo un binario de Go con SQLite y un Caddy: sin Postgres, Redis ni Node.

- **Apps de formulario** (imagen, puerto, réplicas, healthcheck, volúmenes, límites) **o compose**.
- **Proyectos → stages → apps**, cada stage en su propia red.
- **Variables y secretos** por stage y por app, cifrados y enmascarados en los logs.
- **Deploys sin corte**, con log en vivo, historial y rollback. Si la versión nueva no pasa el healthcheck, la anterior sigue sirviendo.
- **Dominios con HTTPS:** crea el registro DNS en Cloudflare y saca el certificado de Let's Encrypt, o, si lo activas, uno de Cloudflare Origin CA.
- **Imágenes privadas** de cualquier registry (GHCR, Docker Hub…), con sus credenciales cifradas.
- **Panel web** con varios usuarios, 2FA, IPs permitidas, auditoría y backup diario.
- **API REST, GitHub Action y servidor MCP** para Claude Code.

No construye imágenes, no publica puertos TCP y no respalda tus bases de datos.

## Cómo funciona

```
          Internet :80 / :443
                 │
         ┌───────▼────────┐  admin API (socket)  ┌──────────────────┐
         │  muelle-caddy  │◄─────────────────────│  muelle          │
         │  (proxy)       │                      │  panel + API     │── docker.sock
         └──┬──────────┬──┘                      │  SQLite          │
            │          │                         └──────────────────┘
   red tienda_staging  red tienda_production
   ┌────────┴───────┐  ┌────────┴───────┐
   │ api, worker…   │  │ api, worker…   │      (stacks de Swarm)
   └────────────────┘  └────────────────┘
```

- **muelle** es un servicio de Swarm. Sirve el panel y la API, guarda todo en SQLite y maneja Docker por `docker.sock`. Su puerto nunca se publica: solo se llega a él por Caddy.
- **muelle-caddy** es un contenedor aparte con los puertos 80 y 443 del host. muelle le arma la configuración completa y se la manda por la admin API. Se conecta en caliente a la red de cada stage, y reiniciar muelle no corta los sitios.
- **Cada stage** es una red overlay propia: staging no ve a production, ni un proyecto a otro.

**Un deploy:**
1. Se encola. Cada app despliega de a uno, y apps distintas en paralelo.
2. Resuelve el digest de cada imagen, así un rollback vuelve exactamente a la anterior.
3. Arma el compose final: la red del stage, `order: start-first`, rollback automático, las variables. Rechaza lo peligroso: montar el host o `docker.sock`, pedir privilegios, leer los archivos de muelle.
4. Corre `docker stack deploy` y espera a que Swarm converja. Si algo falla, el deploy falla con el motivo y la versión anterior sigue sirviendo.
5. Le actualiza las rutas a Caddy.

**Dominios.** Si el dominio está en tu Cloudflare, muelle crea el registro `A`/`AAAA` y Caddy valida el certificado por DNS-01. Si no, creas el registro a mano y el certificado sale por HTTP-01.

**Seguridad.** muelle tiene `docker.sock`, que equivale a ser root en el host, así que:
- los secretos van cifrados con AES-256-GCM, y la clave maestra vive solo en un secreto de Swarm;
- las contraseñas van con Argon2id, con bloqueos progresivos y 2FA opcional (o exigido);
- los tokens de API se limitan a un proyecto o un stage, y pueden ser de solo lectura;
- todo cambio queda en la auditoría.

## Instalar

Necesitas un servidor Linux amd64 con Docker y los puertos 80 y 443 libres. Si no hay un Swarm, install lo inicia; si hay uno, el nodo tiene que ser manager.

```bash
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/cpu-commits/muelle install \
  --panel-domain panel.tudominio.cl --email tu@correo.cl > install.txt
```

`install.txt` trae la **clave maestra** (se muestra una sola vez: respáldala fuera del servidor y borra el archivo) y el link para crear el primer usuario.

| Flag | Qué es |
|---|---|
| `--panel-domain`, `--email` | Obligatorios: el dominio del panel y el correo de Let's Encrypt |
| `--cloudflare-token` | Para crear los registros DNS y validar por DNS-01. Con `-` lo lee de stdin (`docker run -i … < cf-token.txt`) |
| `--public-ip`, `--public-ipv6` | La IP pública (la IPv4 se detecta si no viene) |
| `--http-port`, `--https-port` | Los puertos del proxy (80 y 443) |
| `--advertise-addr` | Para `docker swarm init`, si la máquina tiene varias IPs |
| `--reinstall` | Vuelve a desplegar después de un `docker stack rm muelle`, con la misma clave y la misma base |

- Usa siempre `--rm` y nunca `-t`: así la clave no queda en `docker logs` ni mezclada con el avance.
- Sin `--cloudflare-token`, el registro DNS del panel tiene que existir antes, y el puerto 80 tiene que llegar al servidor.
- **Junto a otro proxy** que ya usa 80 y 443 (Traefik, Dokploy…): instala con `--http-port 8080 --https-port 8443` y el token de Cloudflare. Cuando apagues el otro proxy, cambia los puertos en Ajustes.
- Para fijar una versión, usa `ghcr.io/cpu-commits/muelle:vX.Y.Z`.

## Usar

1. Abre el link de setup y crea tu usuario. Desde Ajustes invitas a otros.
2. En Ajustes, el token de Cloudflare, los registries de tus imágenes privadas y, si quieres, 2FA obligatorio e IPs permitidas.
3. Crea un proyecto, sus stages y una app; carga sus variables (o pega un `.env`) y sus dominios.
4. Despliega desde el panel, o desde CI con un token del stage.

Ponle un **healthcheck** a cada app (`http`, `bun-http` o un comando). Es lo que permite el deploy sin corte: Swarm no le manda tráfico a la versión nueva hasta que pasa.

## Actualizar

```bash
docker service update --image ghcr.io/cpu-commits/muelle:<versión> muelle_muelle
```

Caddy se actualiza solo con muelle (unos segundos de corte en los sitios). Si la versión nueva no arranca, Swarm vuelve a la anterior.

## GitHub Action

```yaml
- uses: CPU-commits/muelle/action@v0
  with:
    url: ${{ secrets.MUELLE_URL }}          # https://panel.tudominio.cl
    token: ${{ secrets.MUELLE_TOKEN }}      # un token del stage
    app: tienda/staging/api
    image: ghcr.io/tu-org/api:sha-${{ github.sha }}
    env-json: ${{ toJson(secrets) }}        # opcional
```

Guarda las variables de la app, crea el deploy y sigue su log. El paso falla si el deploy falla.

| Entrada | Qué es |
|---|---|
| `url`, `token`, `app` | Obligatorias. `app` es `proyecto/stage/app` |
| `image` / `images-json` | La imagen nueva (formulario) o un JSON `servicio → imagen` (compose). Sin ninguna, redespliega las actuales |
| `env-json` | Las variables de la app. **Reemplaza todas:** las que no vienen se borran |
| `env-exclude` | Claves que no se mandan, separadas por coma |
| `env-secret` | `true` (por defecto) las guarda como secretas |
| `timeout` | Segundos de espera (600) |

Usa un token por stage, guardado en el environment de GitHub de ese stage: así el job de staging no puede desplegar production.

## Claude Code (MCP)

muelle sirve un servidor MCP en `/api/v1/mcp`. Crea un token en Ajustes → Tokens (el panel te muestra el comando) y agrégalo:

```bash
claude mcp add --transport http --scope user muelle https://panel.tudominio.cl/api/v1/mcp \
  --header "Authorization: Bearer mk_…"
```

- **Lee:** proyectos, apps, estado, deploys y su log, logs de los servicios, variables (sin los secretos) y dominios.
- **Escribe** (solo con un token que no es de solo lectura): `deploy`, `rollback`, `set_env` y `unset_env`. Claude Code pide confirmación en cada llamada.
- Para depurar, usa un token de solo lectura. Para desplegar, uno de escritura de un stage, nunca de production.
- Los logs son texto de tus apps y pueden traer instrucciones plantadas: revisa lo que apruebas.

## Operación

**Recuperar el acceso** si la lista de IPs permitidas te deja fuera:

```bash
docker exec "$(docker ps -q -f label=com.docker.swarm.service.name=muelle_muelle | head -n1)" muelle settings reset-panel-ips
```

**Leer la clave maestra:** está en `/run/secrets/muelle_master_key` del mismo contenedor.

**Empezar de cero** (borra todos los datos de muelle; tus apps no se tocan):

```bash
docker stack rm muelle && docker rm -f muelle-caddy   # espera a que el contenedor de muelle termine
docker volume rm muelle_data muelle_caddy_data muelle_caddy_config muelle_caddy_admin
docker network rm muelle_panel
docker secret rm muelle_master_key muelle_init_cloudflare_token muelle_init_registry   # los que existan
```

## Desarrollo

Go (`net/http` y SQLite en Go puro, sin CGO) con arquitectura hexagonal: un módulo por área en `src/internal/` (`identity`, `catalog`, `secrets`, `admin`, `deploy`, `ingress`), que no se importan entre sí. El panel (`web/`) es una SPA de SvelteKit que el binario embebe con `-tags panel`. Comentarios y mensajes en español; identificadores en inglés.

```bash
(umask 077; go run . genkey > /tmp/muelle.key)
MUELLE_PANEL_ORIGIN=http://127.0.0.1:18080 MUELLE_SECURE_COOKIES=false \
MUELLE_LISTEN=127.0.0.1:18080 MUELLE_DATA_DIR=/tmp/muelle-data \
MUELLE_MASTER_KEY_FILE=/tmp/muelle.key go run . serve
```

El log trae el link para crear el primer usuario. Sin `-tags panel` solo está la API; para el panel, mira [`web/README.md`](web/README.md). Desplegar necesita un Swarm local (`docker swarm init`).

**Tests:**

```bash
go vet ./... && go test -race ./...
go test -tags integration -race -count=1 -timeout 40m ./...   # contra el Swarm real
cd web && pnpm verify && pnpm test:e2e                        # el panel
```

**Publicar una versión:** `git tag -a vX.Y.Z -m "muelle vX.Y.Z" && git push origin vX.Y.Z`. `release.yml` corre los tests y sube `ghcr.io/cpu-commits/muelle` y `muelle-caddy` (`linux/amd64`). Después, mueve `v0` para la action: `git tag -f v0 'vX.Y.Z^{}' && git push -f origin v0`. GitHub crea cada paquete nuevo como privado: si algún día se recrean, hay que volver a pasarlos a públicos (Package settings → Change visibility).

## Documentación

- [Diseño](docs/superpowers/specs/2026-10-09-muelle-design.md): arquitectura, modelo de datos, flujo de deploy, proxy, DNS y seguridad.
- [API](docs/superpowers/specs/2026-10-10-contrato-http-fases-2-3.md): cada endpoint y sus errores.
- [Servidor MCP](docs/superpowers/specs/2026-10-21-mcp-design.md) y [Origin CA](docs/superpowers/specs/2026-10-21-origin-ca-design.md).

## Licencia

[MIT](LICENSE)
