# muelle — Diseño

> Deploy manager ultraligero sobre Docker Swarm: panel web + API, secretos, stages y proxy con certificados automáticos.
> Fecha: 2026-10-09 · Actualizado: 2026-10-10 (arquitectura hexagonal, §4; revisión final de la Fase 1), 2026-10-13 (etapa de contratos de las Fases 2, 3 y 5), 2026-10-14 (su revisión: §6.3, Caddy en JSON, el cambio de puertos) y 2026-10-21 (los certificados de Cloudflare, Origin CA: §5.1, §8, §9, §11, §12, §13 y §15) · Estado: aprobado; plan de la Fase 1 en `docs/superpowers/plans/2026-10-10-muelle-fase1-hexagonal.md`.
> La API, los puertos entre módulos y la configuración de install de las Fases 2, 3 y 5 están en detalle en [`2026-10-10-contrato-http-fases-2-3.md`](2026-10-10-contrato-http-fases-2-3.md). Los certificados de Cloudflare (Origin CA), opcionales, en [`2026-10-21-origin-ca-design.md`](2026-10-21-origin-ca-design.md) y en el contrato, §3.7.
> "muelle" es un nombre provisional.

## 1. Contexto y objetivo

Dokploy (lo más liviano que existe entre los PaaS self-hosted conocidos) sigue siendo demasiado pesado para el servidor actual: levanta Next.js + Postgres + Redis + Traefik. De todo lo que ofrece solo se usa:

- Desplegar contenedores Docker.
- Manejar secretos / variables de entorno.
- Separar proyectos por stages (staging, production).
- Un proxy con certificados automáticos (Let's Encrypt, con DNS de Cloudflare).
- Una API para GitHub Actions. Hoy auditfy/app, Extractor y Aumi_Agents usan el mismo patrón: `sync-env` → `application.saveEnvironment` y `deploy` → `application.deploy`.

**Objetivo:** una herramienta que cubra exactamente eso, con un consumo de RAM de decenas de MB en vez de cientos.

### Criterios de éxito

1. En reposo, muelle usa < 40 MB de RSS, y muelle + Caddy < 100 MB.
2. auditfy/staging se despliega de punta a punta desde GitHub Actions con la action de muelle, sin downtime: un `curl` en bucle contra la app durante el deploy no registra errores.
3. Un deploy cuya versión nueva no pasa el healthcheck deja la versión anterior sirviendo y hace fallar el job de GitHub Actions con el motivo.
4. Al agregar un dominio en Cloudflare se crea el registro DNS y Caddy sirve un certificado válido en < 2 minutos.

### Fuera de alcance (v1)

Construir imágenes desde git (se construyen en CI), backups de bases de datos, administrar nodos desde el panel, terminal web (exec), notificaciones, historial de métricas, roles/permisos, entornos de preview, publicar puertos TCP y comandos de release/migraciones. Postgres y Mongo viven fuera de muelle; las migraciones siguen corriendo en GitHub Actions.

## 2. Decisiones principales

| Decisión | Elección | Alternativas descartadas |
|---|---|---|
| Orquestador | Docker Swarm, exclusivamente | Contenedores sueltos / compose: soportar ambos duplica el modelo de deploy |
| Topología | 1 nodo ahora, diseñado para sumar nodos | — |
| Interfaz | Panel web como interfaz principal + API REST | Solo CLI |
| Unidad de deploy | Apps por formulario **y** apps compose, compilando al mismo modelo | Solo una de las dos |
| Lenguaje | Go (Docker, Caddy y `compose-go` son Go) | Rust (ganancia de RAM marginal y sin parser oficial de compose), Bun (más RAM, sin parser oficial) |
| Proxy | Caddy, como contenedor **separado** de muelle | Caddy embebido (reiniciar el panel tumbaría todos los sitios), nginx (su ACME nativo no soporta DNS-01), HAProxy (DNS-01 solo vía el proceso Data Plane API), Traefik (más pesado) |
| Usuarios | Varias cuentas con los mismos permisos | Roles por proyecto |
| DNS | Automático con Cloudflare + certificado por DNS-01 | DNS manual |

## 3. Arquitectura

```
             Internet :80 / :443 (+443/udp)   (publicados directo en el host → IP real del cliente)
                      │
              ┌───────▼────────┐   socket unix en volumen   ┌──────────────────┐
              │  muelle-caddy  │◄──────── (admin API) ───────│      muelle      │
              │ (contenedor    │                             │   panel + API    │
              │  standalone)   │                             │ (servicio Swarm) │
              └─┬────┬──────┬──┘                             └────────┬─────────┘
                │    │      │ red "muelle_panel" (solo caddy + muelle) │ docker.sock
                │    │      └──────────────────────────────────────────┤ /data (SQLite)
                │    │ conectado a CADA red de stage (network connect, en caliente)
     red        │    └───────────────────┐ red
     auditfy_staging                     │ auditfy_production
   ┌────────────┴─────────┐   ┌──────────┴───────────┐
   │ auditfy_staging_api  │   │ auditfy_production_  │   (stacks desplegados por muelle)
   │ nats / redis …       │   │ api, nats / redis …  │
   └──────────────────────┘   └──────────────────────┘
```

### 3.1 Componentes del sistema

Ambos viven en el nodo manager.

**muelle-caddy** (contenedor standalone, no servicio Swarm; muelle se encarga de su ciclo de vida)
- Imagen propia compilada con `xcaddy` + `github.com/caddy-dns/cloudflare`, con versiones fijas.
- Puertos publicados directamente en el host: `proxy_http_port` (por defecto 80) → 80/tcp, y `proxy_https_port` (por defecto 443) → 443/tcp y 443/udp. Sin la red de entrada de Swarm, así que la IP del cliente llega intacta.
- `--restart unless-stopped`.
- Volúmenes: `muelle_caddy_data` (certificados), `muelle_caddy_config` (autosave) y `muelle_caddy_admin` (socket).
- Redes: `muelle_panel` y todas las redes de stage.
- Arranca con `caddy run --resume` y `CADDY_ADMIN=unix//run/caddy-admin/admin.sock`, que pone ingress al crear el contenedor (`naming.CaddyCommand` y `naming.CaddyEnv`), sin depender del CMD de la imagen. Sin autosave, Caddy arranca vacío con la admin API en el socket; con autosave, retoma la última configuración (contrato §3.4).
- **Por qué no es servicio Swarm:** un contenedor standalone se puede conectar a una red overlay attachable **en caliente** (`docker network connect`). A un servicio Swarm, agregarle una red lo recrea, y eso tumbaría todos los sitios cada vez que se crea un stage.
- **Ciclo de vida:** al arrancar, y cada vez que cambian la imagen o los puertos de Caddy, muelle se asegura de que el contenedor exista con la imagen, los puertos, los volúmenes y las redes correctas. Recrearlo (al actualizar Caddy o cambiar puertos) provoca una caída de pocos segundos. Cuando el cambio viene de Ajustes, se recrea 2 s después de responder el `PATCH`, que también pasa por Caddy (contrato §3.5). Un puerto nuevo ocupado no deja el panel sin Caddy: el `PATCH` prueba los puertos nuevos antes de guardarlos (uno ocupado es un `422` en su campo), y si igual el contenedor no arranca con ellos, muelle vuelve a los anteriores, en los ajustes y en el contenedor, y lo dice en `GET /proxy` y en la auditoría (contrato §3.5). En desarrollo (sin dominio del panel o sin imagen de Caddy) muelle no crea el contenedor ni le manda configuración (contrato §3.4).

**muelle** (servicio Swarm del stack `muelle`, restricción `node.role == manager`)
- Imagen mínima con el binario de muelle (estático, sin CGO) y el CLI de `docker` (para `docker stack deploy`).
- Montajes: `/var/run/docker.sock`, `muelle_data:/data` y `muelle_caddy_admin:/run/caddy-admin`.
- Secreto Swarm: `muelle_master_key`.
- Red: `muelle_panel`.
- Escucha en `:8080` sin publicar el puerto; solo se llega a muelle a través de Caddy.

### 3.2 Redes

- **`<proyecto>_<stage>`** (overlay, attachable, una por stage): todas las apps del stage, más muelle-caddy.
  - muelle la crea al crear el stage, conecta Caddy en caliente, y al borrar el stage (que tiene que estar vacío) desconecta Caddy y borra la red.
  - Aísla staging de production y de otros proyectos. **Caddy es el único contenedor conectado a más de una red de stage.**
- **`muelle_panel`** (overlay, attachable): solo Caddy y muelle. Las apps no pueden llamar a muelle directamente.
- **No hay una red "edge" compartida entre apps.** `docker stack deploy` registra el nombre corto de cada servicio como alias en **todas** sus redes. Con una red compartida, `api` de staging podría resolver al `api` de production. Caddy no sufre ese problema porque siempre enruta por el nombre completo y único del servicio (`proyecto_stage_app_servicio`).

### 3.3 Instalación

```
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock \
  -v ~/.docker/config.json:/root/.docker/config.json:ro \
  ghcr.io/cpu-commits/muelle:<versión> install \
  --panel-domain panel.tudominio.cl --email tu@correo \
  [--cloudflare-token …] [--public-ip …] [--public-ipv6 …] [--advertise-addr …] \
  [--http-port 80] [--https-port 443] [--caddy-image …] [--reinstall] \
  [--registry-user … --registry-password-stdin]
```

Sin `:<versión>` se usa `:latest`, la última versión final. `--cloudflare-token -` lee el token de stdin (`docker run -i`), así no queda en el historial de la shell.

Las imágenes de muelle son privadas: antes se hace `docker login ghcr.io` en el servidor (un token de GitHub con `read:packages`) y se monta su `config.json`, o se pasa la credencial con `--registry-user` y `--registry-password-stdin`. install baja con ella las imágenes de muelle y de Caddy, y se la pasa a muelle, que la guarda como el registry `ghcr.io` en su primer arranque (contrato §5.2).

Pasos (el orden y los detalles están en el contrato de las fases 2 y 3, §5.4). install no cambia nada hasta el paso 4, así un puerto ocupado aborta sin dejar nada:
1. Verifica Docker y el estado del Swarm, y que no haya una instalación anterior (con `--reinstall`, reusa la clave maestra de una que se borró con `docker stack rm muelle`).
2. Detecta la IP pública si no se pasó `--public-ip` (vía `https://1.1.1.1/cdn-cgi/trace`) y la imprime. Si no puede, avisa y sigue.
3. **Si los puertos elegidos para el proxy están ocupados (p. ej. 80/443 por el Traefik de Dokploy), aborta** con un mensaje claro que sugiere `--http-port`/`--https-port` (ver §16).
4. Si Swarm no está activo, ejecuta `docker swarm init` (con `--advertise-addr` si la máquina tiene varias IPs).
5. Crea la red `muelle_panel`.
6. Genera una clave maestra de 32 bytes y la guarda como secreto Swarm `muelle_master_key`.
7. Despliega el stack `muelle` (solo el servicio muelle, fijo al nodo donde corrió install, porque `muelle_data` es un volumen local). Los flags de instalación se le pasan como configuración inicial (variables `MUELLE_INIT_*`; el token de Cloudflare, como el secreto Swarm `muelle_init_cloudflare_token`). muelle la guarda en settings solo en el primer arranque de la base: después mandan los cambios del panel, y un reinicio no devuelve lo que se borró ahí. El detalle está en el contrato de las fases 2 y 3, §5. Si algo falla antes de que el deploy funcione (también un Ctrl-C), install borra lo que creó: el stack y sus volúmenes si alcanzó a arrancar, la red y los secretos.
8. **Imprime la clave maestra una sola vez** para que el usuario la respalde.
9. En ese primer arranque, muelle crea el contenedor muelle-caddy (§3.1) y envía la ruta del panel.
10. Imprime una URL de setup de un solo uso (`https://panel…/setup?token=…`, vence en 24 h) para crear el primer usuario. Si muelle ya tiene usuarios (una reinstalación), lo dice.
11. Sobre el dominio del panel: con `--cloudflare-token`, muelle crea su registro DNS y usa DNS-01; sin token, el registro DNS tiene que existir de antemano y se usa HTTP-01, que necesita que el puerto 80 público llegue a `--http-port` (install avisa si no es 80).

**Actualizar muelle:** `docker service update --with-registry-auth --image ghcr.io/cpu-commits/muelle:<versión> muelle_muelle`, con el `docker login ghcr.io` del servidor. Caddy es un contenedor aparte, pero sigue a la versión de muelle: con `caddy_image` vacío, el job `"proxy"` del arranque recrea muelle-caddy con la imagen de la versión nueva, y los sitios se cortan unos segundos. Si `caddy_image` se fijó en la instalación (`--caddy-image`), Caddy no cambia (contrato de las fases 2 y 3, §5.4).

**Recuperar el acceso** si la lista de IPs permitidas te deja fuera del panel: `docker exec <contenedor de muelle> muelle settings reset-panel-ips` vacía la lista y reinicia muelle, que al arrancar reenvía la configuración a Caddy (contrato §4.1 y §5.4).

## 4. Componentes (Go)

Módulo: `github.com/CPU-commits/muelle`. Go 1.27. HTTP con la librería estándar `net/http` (patrones con método y ruta).

### 4.1 Arquitectura

DDD con arquitectura hexagonal, con la misma estructura que Extractor (plantilla `Template_Go-DDD`). Cada módulo de `src/internal/` tiene estas capas:

- `domain/`
  - `entity`: entidades con constructores que validan.
  - `vo`: value objects.
  - `repository`: interfaces de repositorio con métodos explícitos.
  - `service`: reglas de dominio y puertos hacia afuera.
- `app/`
  - `dto`.
  - `usecases`: un struct por caso de uso, con sus dependencias y `Execute(ctx, dto)`.
- `infra/in/`: adaptadores de entrada (`http`, `form`, `cron`).
- `infra/out/`: adaptadores de salida (`repository/sqlite`, `service`).

```
main.go                 subcomandos serve y genkey (install en la Fase 5)
src/
├── cmd/http/           ServeMux, middleware, errores → HTTP; utils/ con el kit HTTP que usan los módulos
├── cmd/cron/           ticker que ejecuta las tareas periódicas que registran los módulos
├── internal/
│   ├── identity/       usuarios, sesiones, 2FA, invitaciones, tokens de API, límites de login
│   ├── catalog/        proyectos, stages y apps (el spec de formulario es un value object)
│   ├── secrets/        variables cifradas de stage y app: leer enmascaradas, PUT, PATCH, mezclar
│   ├── admin/          settings, auditoría, backup y limpieza periódica
│   ├── deploy/         (Fase 2) render, cola de deploys, rollback, reconciliación, registries, estado y logs de las apps
│   └── ingress/        (Fase 3) red de cada stage, dominios, Cloudflare, contenedor muelle-caddy y sus rutas
├── shared/
│   ├── domain/         errores comunes, Transactor, Principal y alcance de tokens, reglas de validación
│   └── infra/
│       ├── container/  container.New(settings): arma todo; lo usan main y los tests de integración
│       └── out/        sqlite (conexión, migraciones, Transactor, registro de auditoría), crypto (AES-256-GCM),
│                       docker (cliente compartido por deploy, ingress e install, con su fake dockertest)
├── package/            interfaces finas: cipher, clock, audit
└── settings/           carga y validación de MUELLE_*
web/                    (Fase 4) SvelteKit con adapter-static, embebido con go:embed
```

**Reglas:**
1. **Sin imports entre módulos.** Lo que un módulo necesita de otro lo pide con una interfaz en su `domain/service`. El adaptador vive en su `infra/out/service` y el contenedor lo conecta. Como todos los módulos comparten la base, ese adaptador puede leer directo las tablas del otro módulo; así cada uno se construye y se prueba solo.
2. **`domain` no importa `infra`** ni librerías de infraestructura (SQLite, `net/http`).
3. **Principal compartido.** `shared/domain` define `Principal` (usuario o token, con su alcance) y lo transporta en el `context`. `identity` lo produce y los demás módulos lo consultan para permisos y auditoría.
4. **Transacciones.** Un caso de uso que necesita atomicidad usa `Transactor.InTx(ctx, fn)`. Los repositorios SQLite toman la transacción del `ctx`.
5. **Diferencias con Extractor:**
   - `net/http` en vez de Gin.
   - SQLite en vez de MongoDB.
   - El contenedor es un struct armado con `container.New`, no variables globales, para que cada test levante su propia base.
   - Repositorios con métodos explícitos en vez de Criteria.
   - Errores `{"error":{"code","message"}}`, sin i18n.
   - Sin bus ni eventos de dominio en la v1.

### 4.2 Piezas

| Pieza | Dónde | Notas |
|---|---|---|
| SQLite | `shared/infra/out/sqlite` | `modernc.org/sqlite` en modo WAL, migraciones SQL embebidas (una sola carpeta, porque hay claves foráneas entre módulos), runner propio |
| Cifrado | `shared/infra/out/crypto` detrás de `package/cipher` | AES-256-GCM con la clave maestra; nonce aleatorio por valor; AAD ligada a la fila y la columna, nunca vacía: `env:app:<id>:<key>`, `env:stage:<id>:<key>`, `users:totp:<id>`, `users:totp_pending:<id>`, `settings:<key>` (§12) |
| Contraseñas | `identity` (puerto `PasswordHasher`) | Argon2id vía `golang.org/x/crypto/argon2` (§10.1) |
| TOTP | `identity` (puerto `TOTP`) | `github.com/pquerna/otp` |
| Render | `deploy/domain/service` (Fase 2) | App (formulario o compose) + variables + dominios → compose final validado, con `github.com/compose-spec/compose-go/v2`; funciones puras |
| Deployer | `deploy/app` + worker en `deploy/infra/in` (Fase 2) | Cola por app, resolución de digests, `docker stack deploy`, espera de convergencia, log de deploy, reconciliación al arrancar |
| Docker | `shared/infra/out/docker` | SDK oficial (moby): servicios, tareas, logs en vivo, stats, inspección de distribución, redes, contenedores, secretos de Swarm, y `docker stack deploy/rm` por el CLI. Lo comparten deploy, ingress e install; cada módulo pide solo las partes que usa, y los tests usan el fake `dockertest` |
| Proxy | `ingress/infra/out/service/caddy` (Fase 3) | Ciclo de vida de muelle-caddy (crear o recrear según imagen y puertos, conectar y desconectar redes de stage) y configuración: rutas → JSON de Caddy, `POST /load` por el socket unix, mutex global |
| DNS | `ingress/infra/out/service/cloudflare` (Fase 3) | Cliente mínimo propio de la API de Cloudflare (zonas y registros) |
| API | `cmd/http` + `infra/in/http` de cada módulo | REST `/api/v1`, autenticación por sesión o token, SSE para logs; sirve el SPA embebido |
| Panel | `web/` (Fase 4) | SvelteKit con `adapter-static` (SPA con fallback `index.html`), embebido con `go:embed` |

Además, en el mismo repositorio:
- `deploy/caddy/Dockerfile` — imagen de Caddy con el módulo de Cloudflare.
- `Dockerfile` — imagen de muelle.
- `action/action.yml` — composite action de GitHub (`uses: CPU-commits/muelle/action@v1`).

## 5. Modelo de datos

```
proyecto (auditfy)
 └─ stage (staging, production)              → red overlay  auditfy_staging
     ├─ variables del stage (compartidas)
     └─ app (api, extractor, nats…)           → stack Swarm   auditfy_staging_api
         ├─ tipo: service (formulario) | compose
         ├─ variables de la app (pisan a las del stage)
         ├─ dominios → servicio:puerto
         └─ deploys (historial)
```

### 5.1 Tablas

| Tabla | Campos principales |
|---|---|
| `projects` | `id`, `slug` (único), `name`, `created_at` |
| `stages` | `id`, `project_id`, `slug` (único por proyecto), `created_at` |
| `apps` | `id`, `stage_id`, `slug` (único por stage), `kind` (`service`\|`compose`), `spec` (JSON, solo si es `service`), `compose_yaml` (solo si es `compose`), `deploy_timeout_s` (por defecto 300), `storage_node_id` (nodo donde viven sus volúmenes), `config_changed_at`, `last_deployed_at` |
| `env_vars` | `id`, `stage_id` o `app_id` (exactamente uno, con borrado en cascada), `key`, `value` (cifrado, siempre), `secret` (bool), `updated_at`; única la clave por dueño |
| `domains` | `id`, `app_id` (una app con dominios no se borra), `hostname` (único global), `target_service`, `target_port`, `proxied` (bool), `tls_mode` (`cloudflare`\|`http`), `cf_zone_id`, `cf_record_id` y `cf_record_id_aaaa` (nulos si muelle no maneja el registro), `status` (`pending`\|`active`\|`error`), `last_error_code` (`record_exists`\|`cloudflare`\|`certificate`, solo con `error`), `last_error`, `cert_issuer`, `cert_expires_at`, `created_at`, `updated_at` |
| `deploys` | `id`, `app_id`, `status` (`queued`\|`running`\|`succeeded`\|`failed`\|`superseded`), `images` (JSON `servicio → ref@digest`), `requested_images` (las refs pedidas, antes de resolver), `config_images` (las refs de la configuración que leyó el deploy), `trigger` (`panel`\|`api`\|`rollback`), `actor_type` (`user`\|`token`), `actor_id`, `actor_name` (copiado al crear), `superseded_by`, `error`, `log` (texto, truncado a 256 KB, con `log_truncated`), `routes_warning` (el paso 7 no pudo actualizar Caddy), `created_at`, `started_at`, `finished_at` |
| `registries` | `id`, `host` (único), `username`, `password` (cifrado), `created_at`, `updated_at` |
| `origin_ca_certs` | `id`, `hostname` (de un dominio o del panel, sin FK), `zone_id`, `cf_cert_id` (único: el id de Cloudflare, para revocarlo), `certificate` (PEM de la hoja), `private_key` (cifrada; vacía en uno retirado), `expires_at`, `state` (`active`\|`retired`; a lo más un `active` por hostname), `created_at`, `retired_at`, `revoke_failures`, `next_revoke_at` |
| `origin_ca_hosts` | `hostname`, `observed` (`proxied`\|`dns_only`\|`missing`\|`unreadable`: lo último que se vio en Cloudflare), `observed_at`, `failures`, `last_error_code` (`permission`\|`cloudflare`\|`rejected`), `last_error`, `next_attempt_at`, `updated_at` |
| `users` | `id`, `username` (único), `password_hash`, `totp_secret` (cifrado, nulo si no tiene 2FA), `totp_pending` (cifrado, generado pero sin confirmar), `created_by`, `created_at` |
| `invites` | `id`, `token_hash`, `created_by`, `expires_at`, `used_at` |
| `sessions` | `id_hash`, `user_id`, `mfa_pending` (contraseña correcta, falta el código TOTP), `created_at`, `last_seen_at`, `expires_at`, `user_agent`, `ip` |
| `api_tokens` | `id`, `name`, `token_hash`, `project_id`, `stage_id` (nulo = todo el proyecto), `created_by`, `last_used_at`, `expires_at` (opcional) |
| `audit_events` | `id`, `actor_type` (`user`\|`token`\|`system`), `actor_id`, `actor_name` (el username, el nombre del token o `system`, copiado al escribir), `action`, `target`, `details` (JSON, sin valores secretos), `at` |
| `settings` | `key`, `value`, `encrypted` (bool). Claves: `cloudflare_token`, `cloudflare_zones`, `public_ip`, `public_ipv6`, `acme_email`, `panel_domain` (`MUELLE_PANEL_DOMAIN`, o el host de `MUELLE_PANEL_ORIGIN`; de solo lectura), `require_2fa`, `panel_allowed_cidrs`, `manager_node_id`, `proxy_http_port`, `proxy_https_port`, `caddy_image` (vacío: la imagen de Caddy de la misma versión de muelle), `cloudflare_origin_ca` (los certificados de Cloudflare, `false` por defecto) y las del registro DNS del panel (`panel_cf_*`, de ingress) |

La migración inicial (`0001_init.sql`) crea las tablas de la Fase 1: `projects`, `stages`, `apps`, `env_vars`, `users`, `invites`, `sessions`, `api_tokens`, `audit_events` y `settings`. `deploys` y `registries` llegan con la migración `0002_deploy.sql`, y `domains` con `0003_ingress.sql`. Las dos las reservó la etapa de contratos; cada fase edita la suya, en vez de agregar otra, mientras no haya una versión publicada. `0004_audit_actor_name.sql`, un pendiente de la Fase 1, suma `audit_events.actor_name` y lo llena en los eventos que ya había, `0005_deploy_routes_warning.sql` suma `deploys.routes_warning`, y `0006_origin_ca.sql` crea `origin_ca_certs` y `origin_ca_hosts` (contrato §6).

### 5.2 Reglas

1. **Slugs.** Solo `[a-z0-9-]`, empiezan con letra y no terminan en `-`. Proyecto, app y servicio de una app compose: hasta 16 caracteres (`^[a-z]([a-z0-9-]{0,14}[a-z0-9])?$`). Stage: hasta 12 (`^[a-z]([a-z0-9-]{0,10}[a-z0-9])?$`). Como el `_` no es válido en un slug, sirve de separador: stack `proyecto_stage_app`, servicio Swarm `proyecto_stage_app_servicio`, que nunca pasa de 63 caracteres. Los slugs de proyecto `muelle` y `docker` están reservados: con ellos, una red de stage podría llamarse `muelle_panel` (la red privada de muelle y Caddy) o `docker_gwbridge` (la de Swarm).
2. **Nombres de servicio únicos por stage.** `docker stack deploy` registra el nombre corto de cada servicio como alias en sus redes, y todas las apps de un stage comparten la red del stage. Si dos apps del mismo stage tuvieran un servicio `web`, Docker repartiría el tráfico entre ambos. En una app de formulario, el único servicio se llama igual que la app. En una app compose, se valida al guardar que ningún servicio choque con otro del stage. Entre stages distintos no hay choque, porque no comparten redes (§3.2).
3. **Variables.** Se combinan las del stage con las de la app, y la app gana.
   - **App de formulario:** se inyectan todas como `environment` del servicio.
   - **App compose:** se usan para sustituir `${VAR}` dentro del YAML (sustitución nativa de `compose-go`); cada servicio declara las que necesita. Una `${VAR}` no definida y sin valor por defecto (`${VAR:-x}`) es un error de render.
   - **Variables `secret`:** el panel nunca vuelve a mostrar su valor (son de solo escritura). Las no secretas sí se muestran.
   - **Claves:** el prefijo `COMPOSE_` está reservado, porque compose-go lee esas variables del entorno (nombre del proyecto, perfiles).
4. **Cambios pendientes.** Cambiar variables (de la app o de su stage), spec o compose actualiza `config_changed_at` y no redespliega. El panel muestra "cambios pendientes" mientras `config_changed_at > last_deployed_at`. **Los dominios no pasan por el deploy:** crearlos, editarlos o borrarlos aplica de inmediato el DNS y las rutas de Caddy, porque no afectan al compose.
5. **Digest.** Cada deploy guarda el digest exacto de cada imagen, y el compose final referencia `repo:tag@sha256:…`.
6. **Rollback.** El deploy "actual" es el `succeeded` más reciente. El rollback toma las imágenes (con digest) del `succeeded` inmediatamente anterior y despliega con la configuración **actual** (variables, spec). Si no hay un `succeeded` anterior, responde `409`. Las variables no se versionan en la v1. Un deploy sin cuerpo después del rollback repite esas imágenes con su digest: el rollback no se deshace solo (contrato §2.2).
7. **Retención.** Últimos 20 deploys por app, más siempre el actual y el `succeeded` anterior (el destino del rollback), aunque sean más viejos; `audit_events` 180 días; las sesiones vencidas o sin uso se borran cada hora.

## 6. Render: del modelo al compose final

### 6.1 Spec de una app de formulario (`apps.spec`)

```json
{
  "image": "ghcr.io/aumi-app/backend-app:staging",
  "port": 3000,
  "replicas": 1,
  "command": null,
  "healthcheck": { "type": "http", "path": "/health", "interval_s": 10, "timeout_s": 5, "retries": 3, "start_period_s": 20 },
  "volumes": [{ "name": "data", "mount": "/data" }],
  "resources": { "memory_mb": 512, "cpus": 1.0 }
}
```

- `healthcheck.type` (Swarm ejecuta el healthcheck **dentro** del contenedor, así que el binario tiene que existir en la imagen):
  - `http`: genera la forma exec `["CMD", "wget", "-q", "-O", "/dev/null", "http://127.0.0.1:<port><path>"]`, sin shell; requiere `wget`. El path tampoco admite metacaracteres de shell (`&`, `;`, `|`, `$`, comillas, espacios).
  - `bun-http`: genera la forma exec `["CMD", "bun", "-e", "fetch('http://127.0.0.1:<port><path>').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"]`.
  - `command`: un comando libre, en forma exec (`["CMD", …command]`, sin shell; para un shell, `["sh", "-c", "…"]`).
  - `none`: muelle no agrega healthcheck (si la imagen trae un `HEALTHCHECK`, sigue valiendo). El panel advierte que entonces no hay garantía de cero downtime.
  - Los tiempos (`interval_s`, `timeout_s`, `start_period_s`) van como duraciones (`10s`) y `retries` tal cual.
- `command`, si no está vacío, va en forma exec (`command: [...]`). `resources.memory_mb` y `resources.cpus` van a `deploy.resources.limits`. Cada volumen es un volumen nombrado del stack (`<stack>_<name>`) montado en `mount`. Todas las variables resueltas van como `environment`.

### 6.2 Inyecciones (las mismas para ambos tipos)

Para cada servicio del compose resultante:

| Qué | Valor | ¿Respeta lo que definió el usuario? |
|---|---|---|
| Redes | La red del stage (externa), que **reemplaza** cualquier red que defina el compose. Las redes propias de un compose no están soportadas en la v1 | No |
| Logging | `json-file`, `max-size: 10m`, `max-file: 3` | Sí, si el servicio ya define `logging` |
| `deploy.update_config` | `parallelism: 1`, `order: start-first`, `failure_action: rollback`, `monitor: 30s` | Sí, si lo define |
| `deploy.rollback_config` | `parallelism: 1`, `order: start-first` | Sí, si lo define |
| Servicios con volúmenes | `order: stop-first` en update y rollback (dos instancias escribiendo el mismo volumen lo corrompen) y la restricción `node.id == <storage_node_id>` | Sí, si define su propia restricción de nodo |
| Etiquetas | `muelle.project`, `muelle.stage`, `muelle.app` y `muelle.deploy` en el servicio (`deploy.labels`); las tres primeras en sus contenedores (`labels`) | Se agregan. Una etiqueta `muelle.*` del usuario es un error de render (§6.3) |
| Imagen | `ref@sha256:…` resuelta | — |

- **Campo por campo:** en `logging`, `update_config` y `rollback_config`, lo que el usuario define gana, y los campos que no define se completan con los de la tabla. Un servicio con volúmenes recibe `order: stop-first` en vez de `start-first`, salvo que el usuario ponga su propio `order`.
- **`muelle.deploy` no va en los contenedores:** cambia en cada deploy, y en el contenedor cambiaría la plantilla de las tareas, así que Swarm reiniciaría todos los servicios de la app en cada deploy, aunque no cambien. Con la etiqueta solo en el servicio, un deploy que no cambia nada de un servicio no lo reinicia.
- **El compose final va en JSON** (YAML 1.2 es un superconjunto: `docker stack deploy` lo lee igual), con las claves ordenadas. Así no hay ambigüedades de tipos de YAML (`on`, `no`, `010`) y el golden de los tests es estable.

`storage_node_id` toma por defecto el `manager_node_id` en el primer deploy de una app con volúmenes. Los volúmenes nombrados quedan como `<stack>_<nombre>`, con el driver local.

### 6.3 Restricciones validadas (son errores de render)

muelle tiene `docker.sock` (es root en el host) y la clave maestra. Una app nunca puede montar el socket (contrato §5.3), salir de su contenedor ni leer archivos de muelle. Son errores de render, y también `422` en `compose_yaml` al guardar (la parte pura va en un paquete de `shared/domain`, que usan catalog y deploy). La excepción son los destinos de dominio (abajo), que en la v1 solo verifica el render.

**Lista de permitidos.** El render arma el compose final solo con las claves que conoce. Cualquier otra, en un servicio o en el primer nivel, es un error, salvo las extensiones `x-*`. Así, una clave nueva de la especificación de compose no pasa sin revisarla: `use_api_socket` monta el socket de Docker, y `models`, `provider` o `gpus` no tienen sentido acá.

- **Primer nivel:** `services`, `volumes` (ver abajo) y `x-*`. `version` se acepta y se descarta. `name` no: el nombre del proyecto lo fija el render.
- **Servicio:** `image` (obligatoria), `command`, `entrypoint`, `environment`, `healthcheck`, `deploy`, `labels`, `logging`, `volumes`, `tmpfs`, `user`, `working_dir`, `stop_grace_period`, `stop_signal`, `read_only`, `init`, `ulimits`, `cap_drop`, `hostname`, `domainname`, `extra_hosts`, `dns`, `dns_search`, `expose`, `tty`, `stdin_open` y `x-*`. `depends_on`, `restart` y `platform` se aceptan y se descartan: Swarm los ignora (la plataforma la elige por el manifiesto de la imagen y el nodo), y así no se rompe un compose que ya existe.
- **Dentro de `healthcheck`:** `test`, `interval`, `timeout`, `retries`, `start_period`, `start_interval` y `disable`.
- **Dentro de `deploy`:** `mode` (`replicated` o `global`; los jobs no: son comandos de release, §1), `replicas`, `placement` (`constraints`, `preferences` con `spread`, `max_replicas_per_node`), `resources` (`cpus` y `memory` en `limits` y `reservations`), `restart_policy` (`condition`, `delay`, `max_attempts`, `window`), `update_config` y `rollback_config` (`parallelism`, `delay`, `failure_action`, `monitor`, `max_failure_ratio`, `order`), `endpoint_mode` y `labels`.
- **Dentro de `logging`:** `driver` y `options` (libres, porque el driver ya está acotado: abajo).
- **Nombres:** cada servicio y cada volumen es un slug de hasta 16 caracteres (§5.2 regla 1), y un compose tiene de 1 a 32 servicios.
- **`dns_opt` no:** `docker stack deploy` lo rechaza (su esquema no lo tiene), así que se rechaza al guardar, con ese motivo.

**Rechazados, y por qué** (la lista de permitidos ya los deja fuera; se nombran para que el mensaje lo explique y para que haya un test por cada uno):

- **Publicar o construir:** `ports:` (no soportado en la v1) y `build:` (las imágenes vienen de un registry).
- **Montar el host:**
  - en `volumes:` de un servicio, todo lo que no sea `type: volume` sobre un volumen del compose (con `read_only` y `volume.nocopy`), o `type: tmpfs` (con `tmpfs.size` y `tmpfs.mode`): los bind mounts (`/ruta:/x`, `./x:/x`, `type: bind`), `npipe`, `cluster` e `image`, un volumen anónimo (sin `source`), y `volume.subpath`;
  - en `volumes:` de primer nivel, `driver`, `driver_opts`, `external` y `name` (solo queda `labels`). Un volumen con `driver_opts: {type: none, o: bind, device: /var/run}` monta una ruta del host con el driver local, y le da a la app `docker.sock`. `external` o `name` llegan a volúmenes de otro stack o de muelle (`muelle_data`). Los volúmenes quedan como `<stack>_<nombre>` con el driver local (§6.2);
  - `devices`, `device_cgroup_rules` y `volumes_from`.
- **Escalar privilegios o salir del aislamiento:** `privileged`, `cap_add`, `security_opt`, `sysctls`, `pid`, `ipc`, `uts`, `userns_mode`, `cgroup`, `cgroup_parent`, `runtime`, `isolation` y `credential_spec`.
- **Leer archivos de muelle:** compose-go y `docker stack deploy` leen archivos del sistema de archivos **de muelle**, no del de la app.
  - `secrets:` y `configs:`, de primer nivel y de servicio, no se soportan en la v1: las apps reciben sus secretos como variables (§5.2 regla 3). Con `file:`, `docker stack deploy` leería `/run/secrets/muelle_master_key` o `/data/muelle.db` y los montaría en la app. Con `external: true` o `name:`, la app montaría `muelle_master_key` o `muelle_init_cloudflare_token`.
  - `env_file`, `label_file`, `extends` e `include`, que hacen que compose-go lea archivos.
- **Redes y nombres:** `container_name`, `network_mode`, `links`, `external_links` y `networks:` propias, de primer nivel o de servicio. El render pone la red del stage (§6.2).
- **Logging:** solo los drivers `json-file`, `local` y `none`. Los demás (`syslog`, `gelf`, `fluentd`…) los corre el daemon en la red del host, y su dirección podría apuntar a un servicio local del host.
- **Etiquetas `muelle.*`:** las inyecta el render (§6.2), y deploy, ingress y el estado de las apps filtran por ellas. Una del usuario con ese prefijo se rechaza en `labels` y en `deploy.labels` del servicio, y en las de un volumen. Si no, una app podría hacerse pasar por otra app, o por otro stage. Lo mismo con `com.docker.stack.*`, que pone `docker stack deploy` (y con la que `docker stack rm` encuentra lo que borra).
- **El entorno de cada servicio** (sus variables ya interpoladas; en una app de formulario, todas las del stage y la app) tiene que caber en lo que Linux le da a un proceso al arrancar (`execve`): cada `CLAVE=valor` hasta 128 KiB, y el total (con 8 bytes más por variable) hasta 1,5 MiB. `ARG_MAX` suele ser 2 MiB, y el resto queda para los argumentos y el entorno de la imagen. Si no cabe, el contenedor ni arranca (`E2BIG`). Solo lo verifica el render.
- **Destinos de dominio:** el destino de un dominio tiene que existir en el compose, y su puerto tiene que ser un entero válido. Solo lo verifica el render, con los destinos que lee deploy de `domains` (contrato §1.1): catalog no lee los dominios, así que guardar un compose que deja un dominio sin su servicio no es un error, y el próximo deploy falla con el dominio y el servicio en el mensaje (contrato §3.2).

**La interpolación** de `${VAR}` usa solo las variables resueltas de la app (`ResolveEnv`), nunca el entorno de muelle: en compose-go, el `loader` con `ConfigDetails.Environment` = esas variables y un `LookupValue` que solo mira ahí, sin `cli.WithOsEnv` ni `cli.WithDotEnv` (que leería un `.env`). Una `${VAR}` sin definir y sin valor por defecto es un error de render (`${VAR:-x}`, `${VAR-x}` y `${VAR:+x}` no). Una variable de `environment` sin valor (`- VAR` o `VAR:`) toma el de la variable `VAR` de la app, como en compose con el entorno de la shell; si la app no la tiene, no va al compose final (sin valor, `docker stack deploy` la tomaría del entorno de muelle). Después, `docker stack deploy` vuelve a interpolar el compose final con el entorno de su proceso. Por eso, en el compose final todo `$` va como `$$`, venga del spec de formulario, del compose o del valor de una variable (en los valores; las claves no se interpolan).

**Al guardar**, catalog valida sin interpolar (todavía no hay variables): la lista de permitidos, los rechazos y los nombres. Lo que depende del valor de una variable (un `logging.driver: ${D}`, la imagen) se verifica en el render.

**compose-go no lee archivos:** el compose va en memoria (`ConfigFile.Content`), con `SkipInclude`, `SkipExtends`, `SkipResolveEnvironment` y `SkipResolveLabels` (que leerían `include`, `extends.file`, `env_file` y `label_file`), `ResolvePaths: false` y un `WorkingDir` que no existe. Esas claves quedan en el modelo y la lista de permitidos las rechaza.

**Defensa extra:** el CLI corre en un directorio temporal vacío (`runCLI`), así una ruta relativa que se le escape al render no resuelve dentro de los archivos de muelle. No reemplaza a la validación: una ruta absoluta, o con `..`, no se frena con eso.

## 7. Flujo de deploy

Disparador: `POST /api/v1/projects/{p}/stages/{s}/apps/{a}/deploys`.

**Body:**
- App de formulario: `{"image": "ghcr.io/…:sha-<commit>"}`.
- App compose: `{"images": {"web": "…"}}`.
- Vacío: redespliega las refs actuales y vuelve a resolver los digests, por si los tags se movieron.

**Pasos:**

1. **Encolar.** Se valida la autenticación y el alcance. Se crea el deploy `queued` y se responde `202 {id, status_url}`. Si la app ya tenía deploys `queued`, se marcan `superseded`: solo sobrevive el último. Cada app tiene un worker que corre un deploy a la vez; apps distintas pueden desplegar en paralelo.
2. **Resolver imágenes.** Se lee la configuración de la app (el spec, o el compose interpolado con sus variables: de ahí salen los servicios y sus refs) y se elige la ref de cada servicio (contrato §2.2). Para cada una se llama a `GET /distribution/{ref}/json` de la API de Docker con `X-Registry-Auth` del registry que corresponda (exacto por host; sin credencial, anónimo), con un plazo de 60 s por imagen. Si falla (no existe, sin permiso, Docker no responde): `failed` y Swarm no se toca.
3. **Render.** Ver §6. Si falla: `failed` y Swarm no se toca. Si la app tiene volúmenes y todavía no tiene `storage_node_id`, se fija acá (§6.2).
4. **Asegurar redes.** Se crea la red del stage si no existe y se verifica que muelle-caddy esté conectado a ella.
5. **Aplicar.**
   - Se escribe un `DOCKER_CONFIG` temporal (`0700`) con las credenciales de los registries.
   - Se ejecuta `docker stack deploy --with-registry-auth --prune --detach=true -c - <stack>`, pasando el compose por stdin.
   - Se borra el directorio temporal, aunque el comando falle.
   - La salida del comando (stdout y stderr) va al log, enmascarada. El comando tiene un plazo de 2 min.
6. **Esperar convergencia** (polling cada 2 s por el SDK, con el `deploy_timeout_s` de la app). Se miran los servicios del compose final, y para cada uno el `UpdateStatus` de **este** deploy: el que empezó después de aplicar (`StartedAt` ≥ el momento en que se aplicó, con 2 s de margen). Uno más viejo es de un deploy anterior.
   - Servicio con `UpdateStatus` de este deploy:
     - `completed` → OK.
     - `rollback_started`, `rollback_completed`, `rollback_paused` o `paused` → `failed`. El log recibe el `Status.Err` de la última tarea fallida y sus últimas 50 líneas de log.
     - `updating` → se sigue esperando.
   - Servicio nuevo (creado por este deploy): OK cuando las tareas corriendo llegan a las deseadas (`replicas`, o `DesiredTasks` en uno global). Con healthcheck, Swarm deja la tarea en `starting` hasta que está `healthy`, así que ya cuenta solo las sanas.
   - Servicio que ya existía y sin `UpdateStatus` de este deploy: su plantilla no cambió (solo `muelle.deploy`, §6.2) y Swarm no lo actualiza. OK cuando pasaron 5 s desde que se aplicó (lo que tarda Swarm en empezar una actualización) y las tareas corriendo llegan a las deseadas.
   - Se pasa el timeout → `failed` con "timeout esperando convergencia", y el estado de cada servicio que no llegó. Swarm sigue actuando por su cuenta; el estado real se ve en el panel.
   - Si Docker deja de responder mientras se espera, se reintenta hasta el timeout.
7. **Rutas.** Se regenera y envía la configuración de Caddy (§8), por si el deploy agregó o quitó servicios que son destino de un dominio. Un error no hace fallar el deploy (§13): es una advertencia en el log, y el deploy queda con `routes_warning` (contrato de las fases 2 y 3, §2.1).
8. **Cerrar.** El deploy pasa a `succeeded` (las imágenes con digest ya quedaron en el paso 2), `last_deployed_at` avanza hasta el `started_at` del deploy (el momento en que leyó la configuración: un cambio guardado mientras corría sigue pendiente) y se aplica la retención (§5.2 regla 7).

Un deploy que termina `failed` también aplica la retención. El apagado de muelle nunca cierra un deploy: lo deja `running` para la reconciliación.

**Log del deploy:**
- Cada paso escribe en el log, con timestamp.
- Antes de persistir o transmitir una línea, se reemplaza cualquier aparición de un valor de variable `secret` de la app o el stage por `***`.
- El panel lo recibe por SSE; la API, por polling con `?offset=N`.

**Reconciliación al arrancar muelle:**
- Los deploys en `running` se reevalúan contra el estado real del servicio y se cierran como `succeeded` o `failed`: si sus servicios llevan su etiqueta `muelle.deploy`, se sigue esperando la convergencia (paso 6); si ninguno la lleva, nunca llegó a Swarm (detalle en el contrato, §1.2).
- Los deploys en `queued` se retoman.
- Se asegura el contenedor muelle-caddy y sus redes (§3.1), se reenvía la configuración a Caddy y se verifica que existan las redes de todos los stages.

**Rollback** (`POST …/apps/{a}/rollback`): crea un deploy con `trigger: rollback` y las imágenes del último `succeeded` anterior al actual, y sigue el mismo flujo.

## 8. Proxy (Caddy)

- muelle arma **la configuración completa** desde la base de datos y la envía con `POST /load` por `unix//run/caddy-admin/admin.sock`. Caddy valida antes de aplicar y, si falla, conserva la configuración anterior.
- **La configuración generada siempre incluye `admin.listen` apuntando al socket**; si se omitiera, Caddy volvería a su admin por defecto en TCP.
- **Rutas:**
  - Una por dominio: `hostname → reverse_proxy <proyecto>_<stage>_<app>_<servicio>:<puerto>`, usando el nombre DNS de Swarm (IP virtual del servicio).
  - El panel: `panel_domain → muelle_muelle:8080`. Si hay `panel_allowed_cidrs`, va con un matcher `client_ip` (no `remote_ip`, que con el panel `proxied` vería el edge de Cloudflare): usa la IP real que resuelve `trusted_proxies`, la misma que muelle recibe en `X-Real-IP`.
  - Un servicio que es destino de un dominio pero todavía no existe (antes de su primer deploy) responde `502` hasta que exista. La ruta igual se crea.
- **TLS** (políticas de automatización por dominio):
  - `tls_mode: cloudflare` → emisor ACME con el desafío DNS `cloudflare` y el token incluido en el JSON.
  - `tls_mode: http` → emisor ACME por defecto.
  - Email ACME tomado de settings.
  - Opcional (`cloudflare_origin_ca`, apagado por defecto): los dominios de Cloudflare con el proxy, y el panel, usan un certificado de Cloudflare Origin CA que emite y renueva muelle (90 días, ECDSA). Va en `apps.tls.certificates.load_pem` y fuera de ACME, y se revoca después de dejar de servirlo. Solo Cloudflare confía en él: es para el modo SSL Full (strict). Los dominios en DNS only y los de fuera de Cloudflare siguen con Let's Encrypt (contrato §3.7).
  - Nota: el token de Cloudflare y las claves de los certificados de Origin CA quedan en claro en el `autosave.json` del volumen `muelle_caddy_config`.
- **IP real del cliente:** `servers.trusted_proxies` con los rangos de IP de Cloudflare (lista estática dentro de muelle, que se actualiza con cada versión) y `client_ip_headers: ["CF-Connecting-IP", "X-Forwarded-For"]`, con `trusted_proxies_strict` (`X-Forwarded-For` de derecha a izquierda: no cuenta una IP que el cliente puso adelante). En la ruta del panel, Caddy reenvía a muelle la IP real en `X-Real-IP`: en el JSON, `"headers": {"request": {"set": {"X-Real-IP": ["{http.vars.client_ip}"]}}}` en el `reverse_proxy`. El atajo `{client_ip}` es solo del Caddyfile y en JSON no se reemplaza (contrato §3.4). muelle confía en ese header porque Caddy es el único otro miembro de `muelle_panel`, la única red de muelle: lo acepta solo si la conexión viene de loopback o de una dirección privada (RFC 1918, ULA), y si no usa la IP de la conexión. Por eso el puerto de muelle (`MUELLE_LISTEN`, `:8080`) nunca se publica.
- Las escrituras a Caddy pasan por un mutex global. Si fallan, el panel muestra "proxy desincronizado" (`GET /proxy`) y se reintenta con backoff (1 s, 2 s, 4 s… hasta 60 s). Además, cada 5 minutos un proceso reenvía todo.
- **Verificación de certificados:** 30 s después de crear, adoptar o reintentar un dominio, cada minuto mientras sigue `pending` y cada hora para todos, muelle se conecta por TLS a `muelle-caddy:443` con SNI = hostname a través de `muelle_panel`. Lee el emisor y el vencimiento del certificado servido y, si es válido, actualiza `cert_issuer`/`cert_expires_at`. Para un host con Origin CA, también es válida la hoja servida que es idéntica a la que guardó muelle (en una renovación, también la vieja hasta que Caddy carga la nueva), sin verificar la cadena contra las raíces del sistema. Si el certificado es inválido o vence en menos de 14 días, el dominio pasa a `error` (uno `pending`, recién 10 min después de su último cambio); uno en `error` por el certificado vuelve a `active` cuando ya es válido. Si muelle no llega a Caddy, o el proxy no está sincronizado, no cambia nada (contrato §3.1).

## 9. DNS (Cloudflare)

- **Al crear un dominio con token configurado:**
  1. Se busca la zona por el sufijo más largo entre las zonas visibles para el token.
  2. Se crea un registro `A` (y `AAAA` si hay `public_ipv6`) apuntando a la IP pública, con el `proxied` elegido. Se guarda `cf_record_id`.
  3. Se usa `tls_mode: cloudflare`.
- **Si el hostname no pertenece a ninguna zona:** `tls_mode: http`, `cf_record_id` nulo, y el panel indica que el registro DNS se crea a mano.
- **Si el registro ya existe y no lo creó muelle** (un `A`, `AAAA` o `CNAME` de ese nombre): no se sobrescribe. El dominio queda en `error` ("el registro ya existe") con la acción **Adoptar**, que actualiza el contenido y guarda el `cf_record_id`.
- **Cambiar `proxied` o la IP pública:** actualiza los registros que maneja muelle.
- **Al borrar un dominio:** solo se borra el registro en Cloudflare si `cf_record_id` no es nulo. Si Cloudflare no responde o rechaza el borrado, no se borra nada (`502`), así no quedan registros huérfanos.
- **Si Cloudflare responde con error:** el dominio queda en `error` con el mensaje y un botón para reintentar. **Un error de DNS nunca bloquea un deploy.**
- **Con los certificados de Cloudflare encendidos** (contrato §3.7), muelle lee cada 5 min, y después de cada cambio, el `proxied` de los registros de los dominios que califican y del panel. No los cambia: decide con eso si usa Origin CA o Let's Encrypt.

## 10. API (`/api/v1`)

Todas las respuestas son JSON. Formato de error: `{"error": {"code": "string", "message": "string"}}`. Los recursos se identifican por slug. Las fechas van en RFC 3339, en UTC y con a lo más milisegundos (`2026-10-10T19:36:53.77Z`), lo mismo en la respuesta de un create que en el GET: el reloj (`package/clock`) ya da la hora así, con la precisión que guarda SQLite.

Un `429` lleva `Retry-After: 60` (fijo por ahora). El log de un error interno (`500`) trae el método y la ruta de la petición. Un cliente que se fue con la petición en curso (su contexto se canceló y el error es `context.Canceled`, como el panel al cambiar de pestaña) no es un error interno: va al log en Debug.

Los cuerpos JSON se leen con `encoding/json/v2` en modo estricto: se rechazan campos desconocidos, claves duplicadas, UTF-8 inválido y datos después del valor. El cuerpo se limita a 1 MiB.

| Recurso | Endpoints |
|---|---|
| Proyectos | `GET/POST /projects`, `GET/PATCH/DELETE /projects/{p}` |
| Stages | `GET/POST /projects/{p}/stages`, `GET/DELETE /projects/{p}/stages/{s}` |
| Apps | `GET/POST …/stages/{s}/apps`, `GET/PATCH/DELETE …/apps/{a}` |
| Variables | `GET …/stages/{s}/env`, `PUT …/stages/{s}/env`, `PATCH …/stages/{s}/env`; lo mismo en `…/apps/{a}/env`. `GET` enmascara los secretos. `PUT` reemplaza todo el set (`{"vars": {"K": "v"}, "secret": true}`, donde `secret` aplica a todas por defecto). `PATCH` hace upsert o borra (`{"set": {...}, "unset": ["K"]}`) |
| Deploys | `POST …/apps/{a}/deploys`, `GET …/apps/{a}/deploys`, `GET …/deploys/{id}`, `GET …/deploys/{id}/log?offset=N` → `{chunk, next_offset, status}`, `GET …/deploys/{id}/log/stream` (SSE), `POST …/apps/{a}/rollback` |
| Dominios | `GET/POST …/apps/{a}/domains`, `PATCH/DELETE …/domains/{hostname}`, `POST …/domains/{hostname}/adopt`, `POST …/domains/{hostname}/retry` |
| Estado | `GET …/apps/{a}/status` (por servicio: réplicas deseadas y corriendo, tareas con estado y error, CPU% y RAM actuales); `GET /proxy` (estado del proxy) |
| Logs | `GET …/apps/{a}/logs?service=<s>&tail=200&follow=1`, o `since=<fecha>` en vez de `tail` al reconectar (SSE; no se guarda nada; contrato §2.9) |
| Administración | `/users` (incluye `DELETE /users/{username}` y `DELETE /users/{username}/totp` para quien perdió su teléfono), `/invites`, `/tokens`, `/registries`, `/settings`, `/audit` |
| Sesión | `POST /auth/login`, `POST /auth/totp`, `POST /auth/logout`, `POST /auth/setup`, `POST /auth/invite/{token}`, `GET /auth/me`, `POST /auth/totp/setup`, `POST /auth/totp/enable` |

### 10.1 Autenticación y permisos

- **Sesión del panel:** cookie `muelle_session` (`HttpOnly`, `Secure`, `SameSite=Strict`). Se cierra tras 12 h sin uso y dura 7 días como máximo. Las peticiones que modifican algo exigen que el `Origin` coincida exactamente con el origen en que muelle-caddy sirve el panel según los ajustes de ese momento (`https://<panel_domain>`, con `:<proxy_https_port>` si no es 443), o con `MUELLE_PANEL_ORIGIN`, una variable de entorno opcional para un origen más (detrás de otro proxy, o en desarrollo). install no la escribe: escribe `MUELLE_PANEL_DOMAIN`, el dominio sin puerto. El origen lleva el esquema y el puerto: así el panel funciona en `https://panel…:8443` durante la migración (§16), y en `https://panel…` en cuanto los puertos del proxy pasan a 80/443, sin tocar la configuración de muelle; desde ese momento `:8443` ya no pasa (contrato §0.1). Los links de setup y de las invitaciones usan el mismo origen (`MUELLE_PANEL_ORIGIN` si está). Como la comparación es literal, el origen configurado tiene que estar en forma canónica: esquema y host en minúsculas, sin puerto por defecto y con las IP en su forma estándar. Si no lo está, muelle no arranca y sugiere el valor correcto. Una sesión puede hacer todo.
  - `MUELLE_SECURE_COOKIES=false` (la cookie sin `Secure`) existe solo para desarrollo por http: con un `MUELLE_PANEL_ORIGIN` https, o con `MUELLE_PANEL_DOMAIN`, muelle no arranca.
- **Token de API:** `Authorization: Bearer mk_<43 caracteres base64url>` (32 bytes aleatorios). Se guarda su SHA-256 y se muestra una sola vez. Su alcance es un proyecto, o un proyecto + stage. **Solo permite:** leer proyecto/stage/app/estado/deploys/logs dentro del alcance, `GET/PUT/PATCH` de variables, crear deploys y hacer rollback. Cualquier otra operación responde `403`.
  - `GET /tokens` dice quién creó cada token (`created_by`: el username, o `null` si ese usuario se borró), para encontrar los tokens de una persona que se va.
  - **Borrar un usuario no revoca sus tokens:** un token es del proyecto, no de quien lo creó, y el CI depende de él. Al dar de baja a alguien, se revisan sus tokens en `GET /tokens` y se revocan a mano (`DELETE /tokens/{id}`) los que haga falta.
- **Login:**
  - Argon2id con `m=19MiB, t=2, p=1`, el mínimo que recomienda OWASP. Un semáforo permite como máximo 2 hashes a la vez, para que un pico de logins no rompa el presupuesto de RAM (§1). Los parámetros van dentro del hash (formato PHC), así que se pueden subir más adelante.
  - Bloqueo progresivo de la contraseña, por usuario e IP: tras 5 contraseñas incorrectas del mismo usuario desde la misma IP (en IPv6, el mismo /64), 1 min; después se duplica hasta 1 h. Una contraseña correcta desde esa IP lo reinicia. Se aplica igual a los nombres que no existen, así un bloqueo no revela qué usuarios hay. Es por usuario e IP, y no solo por usuario, para que un atacante no pueda dejar afuera al admin desde otra IP (recomendación de OWASP).
  - Por IP: 20 peticiones cada 15 min (en IPv6, por /64), sumando el login, el código TOTP, el setup y las invitaciones.
  - Con TOTP, el login es en dos pasos: contraseña → código. La sesión intermedia, que solo sirve para mandar el código, dura 5 minutos. Cada código, bueno o malo, cuenta en el límite por IP de arriba, pero los bloqueos por códigos malos no son por IP, porque a ese paso solo llega quien ya sabe la contraseña y podría usar muchas IPs. Son por sesión y por usuario:
    - con una sesión pendiente se validan a lo más 5 códigos. Cada intento se reserva antes de validar el código, así los pedidos que llegan a la vez no pasan del límite. Si ninguno es correcto, la sesión se borra y hay que volver a poner la contraseña; una sesión agotada no acepta ni el código correcto, aunque falle su borrado;
    - el usuario se bloquea para el 2FA desde cualquier IP, con el mismo calendario (5 códigos malos, 1 min; se duplica hasta 1 h); solo un código correcto lo reinicia.
  - Cada código TOTP vale una sola vez (RFC 6238 §5.2): un código ya usado, también el de la activación del 2FA, se rechaza.
  - Los bloqueos, el límite por IP, los contadores de códigos malos (por sesión y por usuario) y los códigos TOTP ya usados viven en memoria, en mapas acotados. Reiniciar muelle los borra todos. Reiniciar el 2FA de un usuario (lo hace un admin) borra los de ese usuario: sus fallos de código, su bloqueo del 2FA y sus códigos usados, así no queda bloqueado con el secreto nuevo.
  - La auditoría registra los logins (`auth.login`, `auth.logout`), los fallos (`auth.login_failed`) y cada vez que algo queda bloqueado (`auth.locked`), sin contraseñas ni códigos.
    - Los eventos del paso de la contraseña los puede provocar cualquiera sin autenticarse, así que tienen un límite global: 2 por segundo, con ráfaga de 100 (a lo más unos 31 millones de filas en los 180 días de retención). Los que sobran se descartan y se informan a lo más una vez por minuto, mientras duren y al minuto de terminar: un aviso en el log y una fila `auth.audit_dropped` con la cuenta (`{"count":N}`).
    - Los del paso del código no tienen ese límite: solo llega ahí quien sabe la contraseña, y el bloqueo por usuario ya los acota (unos 33 códigos malos por día y usuario). Así, una avalancha de contraseñas malas no puede esconder un ataque al 2FA.
  - Si `require_2fa` está activo, un usuario sin TOTP solo puede configurarlo.
  - No se puede activar `require_2fa` sin tener el propio TOTP configurado (si no, quien lo activa queda encerrado en la configuración).
- **Invitaciones:** de un solo uso, vencen en 24 h; se guarda el hash del token.

## 11. Panel (Svelte)

- **Pantallas de acceso:** setup inicial, login, configuración de 2FA y aceptar invitación.
- **Inicio:** proyectos → stages → apps, cada una con estado (corriendo, degradada, caída), imagen actual, último deploy y la marca "cambios pendientes".
- **Detalle de app**, en pestañas:
  - **Resumen:** réplicas, CPU/RAM (se refresca cada 5 s solo mientras la pestaña está abierta), imagen y digest, botón "Desplegar" (con override opcional de imagen) y "Rollback".
  - **Configuración:** el formulario, o un editor YAML (CodeMirror) para compose, con validación al guardar.
  - **Variables:** las del stage (solo lectura desde la app, con link) y las de la app; marca de secreto; pegar un `.env` completo.
  - **Dominios:** estado de DNS y del certificado, `proxied`, adoptar y reintentar.
  - **Deploys:** historial y log en vivo.
  - **Logs:** en vivo por servicio.
- **Ajustes:** Cloudflare (token, IP pública y los certificados de Cloudflare, opcionales), email ACME, registries, usuarios e invitaciones, tokens, exigir 2FA, IPs permitidas para el panel y auditoría.

## 12. Seguridad

- **muelle tiene `docker.sock`, que equivale a ser root en el host.** Por eso: tokens con alcance mínimo, sesiones estrictas, rate limiting, 2FA disponible (y exigible) e IPs permitidas opcionales.
- **Secretos:**
  - Se cifran con AES-256-GCM y AAD ligada a la fila y la columna, así que un valor cifrado copiado a otra fila no se puede descifrar. Las AAD son `env:app:<id>:<key>` y `env:stage:<id>:<key>` (`env_vars.value`), `users:totp:<id>` (`users.totp_secret`), `users:totp_pending:<id>` (`users.totp_pending`), `settings:<key>` (un setting cifrado) y `origin_ca_certs:private_key:<id>` (la clave de un certificado de Origin CA). Solo el último segmento es texto libre; una columna cifrada nueva agrega su prefijo (la lista canónica está en `shared/infra/out/crypto`).
  - La clave maestra solo vive en el secreto Swarm; nunca en SQLite ni en logs.
  - Los valores se enmascaran en los logs de deploy.
  - El compose final no se persiste.
  - `details` de la auditoría nunca incluye valores.
- **Limitación aceptada:** quien tenga acceso a Docker en el host puede ver las variables con `docker inspect`. Esa persona ya es root.
- **Limitación aceptada:** las credenciales de los registries son de todo muelle, una por host. Un token con alcance a un proyecto puede desplegar en sus apps cualquier imagen que vea esa credencial, también una privada de otro proyecto (contrato §2.10).
- **Clave maestra:** se imprime en la instalación y se puede volver a leer con `docker exec` en el contenedor de muelle (`/run/secrets/muelle_master_key`). Si se pierde junto con el secreto Swarm, los secretos no se pueden recuperar.
- **Verificación de la clave:** en el primer arranque muelle guarda en `settings` un valor de control cifrado con la clave maestra. En cada arranque lo descifra, y si falla no arranca ("clave maestra incorrecta"). Así, recrear el secreto Swarm con otra clave no mezcla datos cifrados con dos claves.
- **Ids no reutilizables:** las tablas cuyos ids forman parte de la AAD (`stages`, `apps`, `users`, `origin_ca_certs`) usan `AUTOINCREMENT`, para que un valor cifrado sacado de un backup no descifre en una fila nueva con el mismo id.
- **Respaldo de muelle:** a diario, `VACUUM INTO /data/backups/muelle-<fecha>.db`, guardando los últimos 7.
- **Redes:** ver §3.2. Cada stage está aislado en su red; solo Caddy toca varias. La admin API de Caddy solo es accesible por el socket unix.
- **Apps:** nunca montan `docker.sock`, ni una ruta del host, ni los secretos o los archivos de muelle. El render solo acepta las claves de compose que conoce (§6.3).

## 13. Manejo de errores

| Falla | Comportamiento |
|---|---|
| Imagen inexistente o sin permiso | Deploy `failed` en el paso 2; Swarm no se toca |
| Compose o spec inválido | Error al guardar (`422`) y otra vez en el render; Swarm no se toca |
| `docker stack deploy` falla | Deploy `failed` con el stderr |
| La versión nueva no pasa el healthcheck | Swarm hace rollback; deploy `failed` con el error y los logs de la tarea |
| Timeout de convergencia | Deploy `failed`; el panel muestra el estado real |
| Caddy rechaza la configuración o el socket no responde | Se conserva la configuración anterior; banner "proxy desincronizado"; reintentos con backoff y el proceso de cada 5 minutos |
| Error de la API de Cloudflare | Dominio en `error` con el mensaje y reintento; no bloquea deploys |
| Certificado inválido o por vencer | Dominio en `error` según la verificación de §8 |
| Cloudflare no emite o no renueva un certificado de Origin CA | El dominio sigue con Let's Encrypt (no pasa a `error`) y avisa en `origin_ca`; reintentos con backoff (contrato §3.7) |
| Docker no responde | Los endpoints que usan Docker responden `503`; el panel muestra "degradado" |
| muelle se reinicia | Reconciliación (§7) |

## 14. GitHub Action (`action/action.yml`)

Composite action en bash, con `curl` y `jq` (ambos vienen en los runners de GitHub). El script es `action/deploy.sh`; `action.yml` solo le pasa las entradas por variables de entorno, nunca dentro del `run:` (así una entrada no puede inyectar comandos).

**Entradas:**
- `url`, `token`, `app` (`proyecto/stage/app`): obligatorias.
  - `url` es el origen del panel (`https://panel.tudominio.cl`); una `/` final se quita. Tiene que ser `https://`, salvo `http://localhost` o `http://127.0.0.1` (el token nunca viaja en claro).
  - `app` son tres slugs separados por `/` (§5.2 regla 1), así no puede cambiar la ruta de la petición.
- `image`: opcional.
- `images-json`: opcional, para apps compose (un objeto `servicio → ref`). `image` e `images-json` juntos son un error.
- `env-json`: opcional (un objeto de strings).
- `env-exclude`: lista separada por comas; siempre se agregan `GITHUB_TOKEN`, `MUELLE_URL` y `MUELLE_TOKEN`. La comparación no distingue mayúsculas: `toJson(secrets)` trae el token del runner como `github_token`.
- `env-secret`: por defecto `true`.
- `timeout`: segundos, por defecto 600, contados desde que se creó el deploy.

Las entradas se validan antes de cualquier petición: `images-json` y `env-json` tienen que ser objetos JSON con valores string, `env-secret` es `true` o `false`, y `timeout` un entero mayor que 0. Si algo no sirve, un `::error::` y código 1, sin tocar muelle.

**Salidas:** `deploy-id` y `status` (el estado final, o el último que vio si se pasó el timeout).

**Pasos:**
1. Si viene `env-json`, filtra las claves excluidas y hace `PUT …/apps/{a}/env` con `{"vars": {…}, "secret": <env-secret>}`. `PUT` reemplaza todo el set de la app: una variable que no viene se borra. Hasta 3 intentos (con 2 s y 4 s entre ellos) si muelle no responde o responde 5xx o 429. Con un 429 espera lo que pide `Retry-After` si es más, hasta 60 s; si pide más que eso, falla.
2. `POST …/deploys`, una sola vez (no es idempotente). Si no hay respuesta, falla sin reintentar: el deploy puede haberse creado, y el mensaje dice que se revise en el panel.
3. Hace polling de `…/log?offset=N` (sobre el `status_url` de la respuesta, que tiene que empezar con `/api/v1/`: el token solo va a `url`) e imprime el log. Mientras llegan líneas pide la siguiente parte enseguida; con un `chunk` vacío espera 3 s. Si muelle no responde, o responde 5xx o 429, reintenta hasta el timeout. El log va entre `::stop-commands::<token al azar>` y `::<token>::`: lo que imprime una app nunca se lee como un comando del runner (`::error::`, `::add-mask::`…).
4. Termina cuando el `status` es final y el `chunk` viene vacío:
   - `succeeded`: código 0;
   - `failed`: un `::error::` con el `error` del deploy (lo lee de `GET` sobre el `status_url`) y código 1;
   - `superseded` (lo reemplazó un deploy más nuevo de la misma app): un `::warning::` con el id del nuevo (`superseded_by`, que trae la respuesta del log) y código 0;
   - el timeout: un `::error::` y código 1. El deploy sigue en muelle.

Una respuesta de error de muelle se muestra con su `code` y su `message`; un `404 no_route` dice que ese muelle todavía no tiene la API de deploys. Un `4xx` que no es `429` no se reintenta, en ningún paso. Los mensajes de `::error::` y `::warning::` van escapados como pide GitHub (`%`, `\r`, `\n`).

**Versión de la action:** se referencia con el tag de una versión (`@v0.1.0`) o con el de su versión mayor, que se mueve en cada release (`@v0` mientras no haya 1.0; README, "Publicar una versión").

**Ejemplo para auditfy** (reemplaza `sync-env.yaml` y `deploy.yaml`; `migrate.yaml` no cambia):

```yaml
deploy:
  needs: migrate
  runs-on: ubuntu-latest
  environment: ${{ github.ref_name == 'master' && 'production' || 'staging' }}
  steps:
    - uses: CPU-commits/muelle/action@v0
      with:
        url: ${{ secrets.MUELLE_URL }}
        token: ${{ secrets.MUELLE_TOKEN }}
        app: auditfy/${{ github.ref_name == 'master' && 'production' || 'staging' }}/api
        image: ghcr.io/aumi-app/backend-app:sha-${{ github.sha }}
        env-json: ${{ toJson(secrets) }}
        env-exclude: EXPORT_PASSPHRASE
```

Nota: `build-push.yaml` ya publica el tag `sha-<commit completo>` (`type=sha,format=long`).

## 15. Testing

**Por capa:**
- **Dominio:** tests unitarios puros (entidades, value objects, reglas).
- **Repositorios SQLite:** SQLite real en `t.TempDir()`, con un helper compartido que crea la base y corre las migraciones.
- **Casos de uso:** con los repositorios SQLite reales y un reloj controlado (`package/clock`), sin mocks.
- **HTTP:** `httptest` contra el servidor armado de verdad.
- **Punta a punta:** el panel embebido en un `muelle serve` de verdad (`-tags panel`) contra el Swarm local: `pnpm test:e2e:real` (`web/e2e-real/`, prefijo `it7`, puertos 24000–24999) recorre con Playwright el setup del primer usuario, el 2FA, el catálogo, una variable, los deploys, el rollback, la auditoría y el borrado. Necesita un Swarm en el que el nodo es manager y no lo crea; CI lo corre en el job `e2e-real`, que hace un `docker swarm init` de un nodo en el runner. Los pasos con `curl` del README quedan como la prueba manual de la API.
- **Contratos entre módulos:** como los módulos no se importan (§4.1), lo que uno lee o escribe de las tablas de otro se prueba con el contenedor completo (`shared/infra/container`).

**Unitarios:**
- `render`: golden files (`testdata/render/*.input.json` → `*.golden.json`, porque el compose final va en JSON, §6.2) cubriendo:
  - inyección de la red del stage, logging, update_config y etiquetas
  - servicios con volúmenes (stop-first y restricción de nodo)
  - respeto de los overrides del usuario
  - interpolación y variables faltantes
  - cada restricción de §6.3
- `proxy`: rutas → JSON golden de Caddy (incluye `admin.listen`, políticas TLS y `trusted_proxies`, y ningún atajo del Caddyfile como `{client_ip}`), también con los certificados de Origin CA (`load_pem`, `skip_certificates` y `disable_ocsp_stapling`).
- `crypto`: ida y vuelta; un valor alterado o una AAD distinta fallan.
- `auth`:
  - matriz de alcance de tokens (operación × alcance → permitido o 403)
  - bloqueo progresivo
  - vencimiento de sesiones e invitaciones
  - flujo de TOTP
- repositorios y reglas: mezcla de variables, validación de slugs, servicios duplicados dentro de un stage, cálculo de "cambios pendientes", retención.
- `dns`: contra un servidor HTTP falso: búsqueda de zona por sufijo, conflicto/adopción, borrar solo lo propio, y Origin CA (emitir, revocar y la prueba del permiso).

**Integración** (`//go:build integration`, `go test -tags integration ./...`). Corren en local, con un Swarm activo en el que el nodo es manager, y se saltan si no lo hay o si falta una imagen que necesitan (`caddy:2-alpine`, `nginx:alpine`, `it5-muelle:test`…, ver el README); CI no los corre. Cubren:
- deploy de `traefik/whoami` por formulario y por compose
- convergencia `succeeded`
- healthcheck que falla → `failed` con rollback detectado
- `--prune` elimina un servicio quitado del compose
- deploy de un servicio con volumen (stop-first)
- Caddy real con `tls internal` enrutando a whoami por hostname
- Caddy real sirviendo un PEM cargado (`load_pem`) para un host y la CA interna para otro, y la vuelta a ACME
- crear un stage nuevo conecta Caddy a su red sin reiniciarlo (el contenedor conserva su id y su uptime)
- aislamiento: un contenedor de un stage no resuelve ni alcanza servicios de otro stage
- reconciliación tras matar muelle a mitad de un deploy

**Manual antes de migrar:**
- Cloudflare DNS-01 contra una zona de prueba.
- auditfy/staging completo desde GitHub Actions en una VM local.
- Verificar los criterios de éxito de §1.

**Frontend** (`web/`): `pnpm verify` (lint, `svelte-check` y vitest: dominio, casos de uso e integración contra MSW en node; componentes y viewModels en Chromium), `pnpm build` y `pnpm test:e2e` (Playwright contra el build con MSW, bajo la CSP). Los e2e son caros y puntuales: solo cubren los flujos críticos (que el panel corre bajo la CSP del header, en cada pantalla; el setup del primer usuario con el 2FA y el login con el código; y de un proyecto nuevo a un deploy exitoso y su rollback). Lo que hace cada feature lo prueban los tests de dominio, de viewModel y de integración contra MSW; una feature suma un e2e solo si crea un flujo crítico nuevo (`web/CLAUDE.md`, "Tests"). CI corre los tres en el job `web`, y el job `panel` compila y prueba Go con ese build embebido (`-tags panel`). Contra el backend de verdad, `pnpm test:e2e:real` (punta a punta, arriba).

## 16. Migración desde Dokploy

Dokploy también usa Swarm, así que ambos pueden convivir en el mismo cluster. Lo único que choca son los puertos 80/443 que ocupa el Traefik de Dokploy. DNS-01 no necesita esos puertos, y eso permite preparar todo sin downtime:

1. Probar muelle completo en una VM local con Swarm.
2. En el servidor: `muelle install --cloudflare-token … --http-port 8080 --https-port 8443`. El panel queda en `https://panel…:8443`, con certificado por DNS-01. La configuración de muelle lleva solo el dominio (`MUELLE_PANEL_DOMAIN`): el puerto sale de los ajustes.
3. Crear proyectos, stages, apps (mismas imágenes de GHCR), variables y dominios en muelle.
   - Los registros DNS de Cloudflare ya existen (apuntan al mismo servidor), así que cada dominio queda en "el registro ya existe" → **Adoptar**.
   - Como la IP pública es la misma, adoptar no cambia hacia dónde va el tráfico: sigue entrando por 80/443 al Traefik de Dokploy. Conviene adoptar con el mismo `proxied` que tiene hoy el registro.
4. Desplegar cada app y probarla contra muelle: `curl --resolve host:8443:<ip> https://host:8443/health`.
5. Cambio (segundos de downtime):
   - bajar el Traefik de Dokploy (`docker service scale dokploy-traefik=0`, o el nombre que corresponda)
   - en Ajustes de muelle, cambiar los puertos del proxy a 80/443. El `PATCH` responde, y 2 s después muelle recrea muelle-caddy en los puertos nuevos. Si el Traefik sigue con 80 o 443, el `PATCH` responde `422` en ese puerto y no cambia nada; si algo los toma justo después, muelle vuelve a 8080/8443 y el panel sigue en `:8443`
   - el panel pasa a `https://panel…`, con la misma sesión: ese origen se acepta desde que responde el `PATCH`, y `:8443` ya no. Los links de invitación también salen con el origen nuevo; no hay que tocar la configuración de muelle
6. Cambiar los workflows a la action de muelle.
7. Dokploy queda detenido hasta confirmar que todo funciona y después se desinstala.

## 17. Orden sugerido de construcción

Cada fase deja algo que se puede probar de punta a punta:

1. **Base:** los módulos `identity`, `catalog`, `secrets` y `admin` sobre la base compartida (`shared`, `package`, `settings`, `cmd/http`). Se construye en tres etapas:
   1. **Base compartida** (secuencial): `settings`, `package/`, `shared/domain`, SQLite con la migración inicial (las tablas de la Fase 1, §5.1; `deploys`/`registries` y `domains` llegan con migraciones de las fases 2 y 3), `crypto` y el esqueleto de `cmd/http`.
   2. **Módulos** (en paralelo): `identity`, `catalog`, `secrets` y `admin`, cada uno completo con sus tests.
   3. **Integración** (secuencial): el contenedor, `main.go`, el cron, la prueba de punta a punta, CI y el README.
2. **Deploy:** el módulo `deploy` (`render` y `deployer`, sobre el cliente compartido `shared/infra/out/docker`). Apps de formulario y compose desplegables por API, con log, rollback, reconciliación, registries, y el estado y los logs en vivo de cada app.
3. **Proxy y DNS:** el módulo `ingress`: el contenedor muelle-caddy, las rutas, `dns` de Cloudflare, la verificación de certificados y el estado del proxy.
4. **Panel:** el SPA de Svelte sobre la API ya existente.
5. **Distribución:** `install`, las imágenes (muelle y Caddy) y la GitHub Action. Después, la migración de auditfy.

Antes de las Fases 2, 3 y 5, una etapa de contratos (secuencial) dejó el cliente de Docker compartido, los nombres (`shared/domain/naming`), las migraciones reservadas, los esqueletos de `deploy` e `ingress` con sus puertos, los ajustes del proxy en `/settings` y el contrato HTTP. Después, las tres fases corren en paralelo.
