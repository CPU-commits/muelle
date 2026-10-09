# muelle — Diseño

> Deploy manager ultraligero sobre Docker Swarm: panel web + API, secretos, stages y proxy con certificados automáticos.
> Fecha: 2026-10-09 · Estado: aprobado en brainstorming, pendiente de plan de implementación.
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
- Arranca con `caddy run --resume` y una configuración inicial cuyo único contenido es `admin.listen: unix//run/caddy-admin/admin.sock`.
- **Por qué no es servicio Swarm:** un contenedor standalone se puede conectar a una red overlay attachable **en caliente** (`docker network connect`). A un servicio Swarm, agregarle una red lo recrea, y eso tumbaría todos los sitios cada vez que se crea un stage.
- **Ciclo de vida:** al arrancar, y cada vez que cambian la imagen o los puertos de Caddy, muelle se asegura de que el contenedor exista con la imagen, los puertos, los volúmenes y las redes correctas. Recrearlo (al actualizar Caddy o cambiar puertos) provoca una caída de pocos segundos.

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
  ghcr.io/cpu-commits/muelle install \
  --panel-domain panel.tudominio.cl --email tu@correo \
  [--cloudflare-token …] [--public-ip …] [--advertise-addr …] [--http-port 80] [--https-port 443]
```

Pasos:
1. Verifica Docker. Si Swarm no está activo, ejecuta `docker swarm init` (con `--advertise-addr` si la máquina tiene varias IPs).
2. **Si los puertos elegidos para el proxy están ocupados (p. ej. 80/443 por el Traefik de Dokploy), aborta** con un mensaje claro que sugiere `--http-port`/`--https-port` (ver §16).
3. Crea la red `muelle_panel`.
4. Genera una clave maestra de 32 bytes, la guarda como secreto Swarm `muelle_master_key` e **imprime la clave una sola vez** para que el usuario la respalde.
5. Detecta la IP pública si no se pasó `--public-ip` (vía `https://1.1.1.1/cdn-cgi/trace`) y la imprime.
6. Despliega el stack `muelle` (solo el servicio muelle). Los flags de instalación se le pasan como configuración inicial y muelle los guarda en settings en su primer arranque.
7. En ese primer arranque, muelle crea el contenedor muelle-caddy (§3.1) y envía la ruta del panel.
8. Imprime una URL de setup de un solo uso (`https://panel…/setup?token=…`, vence en 24 h) para crear el primer usuario.
9. Sobre el dominio del panel: con `--cloudflare-token`, muelle crea su registro DNS y usa DNS-01; sin token, el registro DNS tiene que existir de antemano y se usa HTTP-01 (que exige `--http-port 80`).

**Actualizar muelle:** `docker service update --image ghcr.io/cpu-commits/muelle:<versión> muelle_muelle`. Los sitios no se ven afectados, porque Caddy es un contenedor aparte.

## 4. Componentes (Go)

Módulo: `github.com/CPU-commits/muelle`. Go 1.26. HTTP con la librería estándar `net/http` (patrones con método y ruta).

| Paquete | Responsabilidad | Depende de |
|---|---|---|
| `cmd/muelle` | Subcomandos `serve` e `install` | todos |
| `internal/store` | SQLite (`modernc.org/sqlite`, en modo WAL), migraciones SQL embebidas con un runner propio, repositorios | — |
| `internal/crypto` | AES-256-GCM con la clave maestra; nonce aleatorio por valor; AAD = `<tabla>:<dueño>:<clave>` | — |
| `internal/auth` | Usuarios (argon2id vía `golang.org/x/crypto/argon2`), sesiones, invitaciones, tokens de API, TOTP (`github.com/pquerna/otp`), rate limiting de login | store, crypto |
| `internal/render` | App (formulario o compose) + variables + dominios → compose final validado, usando `github.com/compose-spec/compose-go/v2` | — (funciones puras) |
| `internal/deployer` | Cola por app, resolución de digests, `docker stack deploy`, espera de convergencia, log de deploy, reconciliación al arrancar | render, docker, store, proxy |
| `internal/proxy` | Ciclo de vida del contenedor muelle-caddy (crear o recrear según imagen y puertos, conectar y desconectar redes de stage) y configuración: rutas → JSON de Caddy, `POST /load` por el socket unix, mutex global | docker |
| `internal/dns` | Cliente mínimo propio de la API de Cloudflare (zonas y registros: listar, crear, actualizar, borrar), detrás de una interfaz | — |
| `internal/docker` | Envoltorio del SDK oficial de Docker (moby): servicios, tareas, logs en vivo, stats, inspección de distribución | — |
| `internal/api` | REST `/api/v1`, middlewares de autenticación (sesión o token), SSE para logs, sirve el SPA embebido | auth, store, deployer, dns, docker |
| `web/` | SvelteKit con `adapter-static` (SPA con fallback `index.html`); se compila y se embebe con `go:embed` | — |

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
| `domains` | `id`, `app_id`, `hostname` (único global), `target_service`, `target_port`, `proxied` (bool), `tls_mode` (`cloudflare`\|`http`), `cf_zone_id`, `cf_record_id` (nulo si muelle no maneja el registro), `status` (`pending`\|`active`\|`error`), `last_error`, `cert_issuer`, `cert_expires_at` |
| `deploys` | `id`, `app_id`, `status` (`queued`\|`running`\|`succeeded`\|`failed`\|`superseded`), `images` (JSON `servicio → ref@digest`), `trigger` (`panel`\|`api`\|`rollback`), `actor_type` (`user`\|`token`), `actor_id`, `error`, `log` (texto, truncado a 256 KB), `created_at`, `started_at`, `finished_at` |
| `registries` | `id`, `host` (único), `username`, `password` (cifrado) |
| `users` | `id`, `username` (único), `password_hash`, `totp_secret` (cifrado, nulo si no tiene 2FA), `totp_pending` (cifrado, generado pero sin confirmar), `created_by`, `created_at`, `failed_logins`, `locked_until` |
| `invites` | `id`, `token_hash`, `created_by`, `expires_at`, `used_at` |
| `sessions` | `id_hash`, `user_id`, `mfa_pending` (contraseña correcta, falta el código TOTP), `created_at`, `last_seen_at`, `expires_at`, `user_agent`, `ip` |
| `api_tokens` | `id`, `name`, `token_hash`, `project_id`, `stage_id` (nulo = todo el proyecto), `created_by`, `last_used_at`, `expires_at` (opcional) |
| `audit_events` | `id`, `actor_type`, `actor_id`, `action`, `target`, `details` (JSON, sin valores secretos), `at` |
| `settings` | `key`, `value`, `encrypted` (bool). Claves: `cloudflare_token`, `public_ip`, `public_ipv6`, `acme_email`, `panel_domain`, `require_2fa`, `panel_allowed_cidrs`, `manager_node_id`, `proxy_http_port`, `proxy_https_port`, `caddy_image` |

### 5.2 Reglas

1. **Slugs.** Solo `[a-z0-9-]`, empiezan con letra y no terminan en `-`. Proyecto, app y servicio de una app compose: hasta 16 caracteres (`^[a-z]([a-z0-9-]{0,14}[a-z0-9])?$`). Stage: hasta 12 (`^[a-z]([a-z0-9-]{0,10}[a-z0-9])?$`). Como el `_` no es válido en un slug, sirve de separador: stack `proyecto_stage_app`, servicio Swarm `proyecto_stage_app_servicio`, que nunca pasa de 63 caracteres.
2. **Nombres de servicio únicos por stage.** `docker stack deploy` registra el nombre corto de cada servicio como alias en sus redes, y todas las apps de un stage comparten la red del stage. Si dos apps del mismo stage tuvieran un servicio `web`, Docker repartiría el tráfico entre ambos. En una app de formulario, el único servicio se llama igual que la app. En una app compose, se valida al guardar que ningún servicio choque con otro del stage. Entre stages distintos no hay choque, porque no comparten redes (§3.2).
3. **Variables.** Se combinan las del stage con las de la app, y la app gana.
   - **App de formulario:** se inyectan todas como `environment` del servicio.
   - **App compose:** se usan para sustituir `${VAR}` dentro del YAML (sustitución nativa de `compose-go`); cada servicio declara las que necesita. Una `${VAR}` no definida y sin valor por defecto (`${VAR:-x}`) es un error de render.
   - **Variables `secret`:** el panel nunca vuelve a mostrar su valor (son de solo escritura). Las no secretas sí se muestran.
4. **Cambios pendientes.** Cambiar variables (de la app o de su stage), spec o compose actualiza `config_changed_at` y no redespliega. El panel muestra "cambios pendientes" mientras `config_changed_at > last_deployed_at`. **Los dominios no pasan por el deploy:** crearlos, editarlos o borrarlos aplica de inmediato el DNS y las rutas de Caddy, porque no afectan al compose.
5. **Digest.** Cada deploy guarda el digest exacto de cada imagen, y el compose final referencia `repo:tag@sha256:…`.
6. **Rollback.** El deploy "actual" es el `succeeded` más reciente. El rollback toma las imágenes (con digest) del `succeeded` inmediatamente anterior y despliega con la configuración **actual** (variables, spec). Si no hay un `succeeded` anterior, responde `409`. Las variables no se versionan en la v1.
7. **Retención.** Últimos 20 deploys por app; `audit_events` 180 días; las sesiones vencidas se limpian a diario.

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
  - `http`: genera `CMD-SHELL wget -qO- http://127.0.0.1:<port><path> >/dev/null || exit 1`; requiere `wget`.
  - `bun-http`: genera `CMD bun -e "fetch('http://127.0.0.1:<port><path>').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"`.
  - `command`: un comando libre.
  - `none`: sin healthcheck. El panel advierte que entonces no hay garantía de cero downtime.

### 6.2 Inyecciones (las mismas para ambos tipos)

Para cada servicio del compose resultante:

| Qué | Valor | ¿Respeta lo que definió el usuario? |
|---|---|---|
| Redes | La red del stage (externa), que **reemplaza** cualquier red que defina el compose. Las redes propias de un compose no están soportadas en la v1 | No |
| Logging | `json-file`, `max-size: 10m`, `max-file: 3` | Sí, si el servicio ya define `logging` |
| `deploy.update_config` | `parallelism: 1`, `order: start-first`, `failure_action: rollback`, `monitor: 30s` | Sí, si lo define |
| `deploy.rollback_config` | `parallelism: 1`, `order: start-first` | Sí, si lo define |
| Servicios con volúmenes | `order: stop-first` en update y rollback (dos instancias escribiendo el mismo volumen lo corrompen) y la restricción `node.id == <storage_node_id>` | Sí, si define su propia restricción de nodo |
| Etiquetas | `muelle.project`, `muelle.stage`, `muelle.app`, `muelle.deploy` | Se agregan |
| Imagen | `ref@sha256:…` resuelta | — |

`storage_node_id` toma por defecto el `manager_node_id` en el primer deploy de una app con volúmenes. Los volúmenes nombrados quedan como `<stack>_<nombre>`, con el driver local.

### 6.3 Restricciones validadas (son errores de render)

- `ports:` publicados: no soportado en la v1.
- `build:`: no soportado; las imágenes vienen de un registry.
- Bind mounts a rutas del host: no soportados en la v1, solo volúmenes nombrados.
- `container_name`, `network_mode`, servicios con `privileged` y secciones `networks:` propias: rechazados.
- El destino de un dominio tiene que existir en el compose y su puerto tiene que ser un entero válido.

## 7. Flujo de deploy

Disparador: `POST /api/v1/projects/{p}/stages/{s}/apps/{a}/deploys`.

**Body:**
- App de formulario: `{"image": "ghcr.io/…:sha-<commit>"}`.
- App compose: `{"images": {"web": "…"}}`.
- Vacío: redespliega las refs actuales y vuelve a resolver los digests, por si los tags se movieron.

**Pasos:**

1. **Encolar.** Se valida la autenticación y el alcance. Se crea el deploy `queued` y se responde `202 {id, status_url}`. Si la app ya tenía deploys `queued`, se marcan `superseded`: solo sobrevive el último. Cada app tiene un worker que corre un deploy a la vez; apps distintas pueden desplegar en paralelo.
2. **Resolver imágenes.** Se llama a `GET /distribution/{ref}/json` de la API de Docker con `X-Registry-Auth` del registry que corresponda. Si falla (no existe, sin permiso): `failed` y Swarm no se toca.
3. **Render.** Ver §6. Si falla: `failed` y Swarm no se toca.
4. **Asegurar redes.** Se crea la red del stage si no existe y se verifica que muelle-caddy esté conectado a ella.
5. **Aplicar.**
   - Se escribe un `DOCKER_CONFIG` temporal (`0700`) con las credenciales de los registries.
   - Se ejecuta `docker stack deploy --with-registry-auth --prune --detach=true -c - <stack>`, pasando el compose por stdin.
   - Se borra el directorio temporal, aunque el comando falle.
   - El stderr del comando va al log.
6. **Esperar convergencia** (polling cada 2 s por el SDK, con el `deploy_timeout_s` de la app):
   - Servicio con `UpdateStatus`:
     - `completed` → OK.
     - `rollback_started`, `rollback_completed` o `paused` → `failed`. El log recibe el `Status.Err` de la última tarea fallida y sus últimas 50 líneas de log.
   - Servicio nuevo (sin `UpdateStatus`): OK cuando las tareas corriendo igualan a `replicas` y, si hay healthcheck, todas están `healthy`.
   - Se pasa el timeout → `failed` con "timeout esperando convergencia". Swarm sigue actuando por su cuenta; el estado real se ve en el panel.
7. **Rutas.** Se regenera y envía la configuración de Caddy (§8), por si el deploy agregó o quitó servicios que son destino de un dominio.
8. **Cerrar.** El deploy pasa a `succeeded`, se guardan las imágenes con digest y se actualiza `last_deployed_at`.

**Log del deploy:**
- Cada paso escribe en el log, con timestamp.
- Antes de persistir o transmitir una línea, se reemplaza cualquier aparición de un valor de variable `secret` de la app o el stage por `***`.
- El panel lo recibe por SSE; la API, por polling con `?offset=N`.

**Reconciliación al arrancar muelle:**
- Los deploys en `running` se reevalúan contra el estado real del servicio y se cierran como `succeeded` o `failed`.
- Los deploys en `queued` se retoman.
- Se asegura el contenedor muelle-caddy y sus redes (§3.1), se reenvía la configuración a Caddy y se verifica que existan las redes de todos los stages.

**Rollback** (`POST …/apps/{a}/rollback`): crea un deploy con `trigger: rollback` y las imágenes del último `succeeded` anterior al actual, y sigue el mismo flujo.

## 8. Proxy (Caddy)

- muelle arma **la configuración completa** desde la base de datos y la envía con `POST /load` por `unix//run/caddy-admin/admin.sock`. Caddy valida antes de aplicar y, si falla, conserva la configuración anterior.
- **La configuración generada siempre incluye `admin.listen` apuntando al socket**; si se omitiera, Caddy volvería a su admin por defecto en TCP.
- **Rutas:**
  - Una por dominio: `hostname → reverse_proxy <proyecto>_<stage>_<app>_<servicio>:<puerto>`, usando el nombre DNS de Swarm (IP virtual del servicio).
  - El panel: `panel_domain → muelle_muelle:8080`. Si hay `panel_allowed_cidrs`, va con un matcher `remote_ip`.
  - Un servicio que es destino de un dominio pero todavía no existe (antes de su primer deploy) responde `502` hasta que exista. La ruta igual se crea.
- **TLS** (políticas de automatización por dominio):
  - `tls_mode: cloudflare` → emisor ACME con el desafío DNS `cloudflare` y el token incluido en el JSON.
  - `tls_mode: http` → emisor ACME por defecto.
  - Email ACME tomado de settings.
  - Nota: el token de Cloudflare queda en el `autosave.json` del volumen `muelle_caddy_config`.
- **IP real del cliente:** `servers.trusted_proxies` con los rangos de IP de Cloudflare (lista estática dentro de muelle, que se actualiza con cada versión) y `client_ip_headers: ["CF-Connecting-IP", "X-Forwarded-For"]`. En la ruta del panel, Caddy reenvía a muelle la IP real en `X-Real-IP` (`header_up X-Real-IP {client_ip}`). muelle confía en ese header porque Caddy es el único otro miembro de `muelle_panel`, la única red de muelle.
- Las escrituras a Caddy pasan por un mutex global. Si fallan, el panel muestra "proxy desincronizado" y se reintenta con backoff (1 s, 2 s, 4 s… hasta 60 s). Además, cada 5 minutos un proceso reenvía todo.
- **Verificación de certificados:** cada hora, y 30 s después de crear un dominio, muelle se conecta por TLS a `muelle-caddy:443` con SNI = hostname a través de `muelle_panel`. Lee el emisor y el vencimiento del certificado servido y actualiza `cert_issuer`/`cert_expires_at`. Si el certificado es inválido o vence en menos de 14 días, el dominio pasa a `error`.

## 9. DNS (Cloudflare)

- **Al crear un dominio con token configurado:**
  1. Se busca la zona por el sufijo más largo entre las zonas visibles para el token.
  2. Se crea un registro `A` (y `AAAA` si hay `public_ipv6`) apuntando a la IP pública, con el `proxied` elegido. Se guarda `cf_record_id`.
  3. Se usa `tls_mode: cloudflare`.
- **Si el hostname no pertenece a ninguna zona:** `tls_mode: http`, `cf_record_id` nulo, y el panel indica que el registro DNS se crea a mano.
- **Si el registro ya existe y no lo creó muelle:** no se sobrescribe. El dominio queda en `error` ("el registro ya existe") con la acción **Adoptar**, que actualiza el contenido y guarda el `cf_record_id`.
- **Cambiar `proxied` o la IP pública:** actualiza los registros que maneja muelle.
- **Al borrar un dominio:** solo se borra el registro en Cloudflare si `cf_record_id` no es nulo.
- **Si Cloudflare responde con error:** el dominio queda en `error` con el mensaje y un botón para reintentar. **Un error de DNS nunca bloquea un deploy.**

## 10. API (`/api/v1`)

Todas las respuestas son JSON. Formato de error: `{"error": {"code": "string", "message": "string"}}`. Los recursos se identifican por slug.

| Recurso | Endpoints |
|---|---|
| Proyectos | `GET/POST /projects`, `GET/PATCH/DELETE /projects/{p}` |
| Stages | `GET/POST /projects/{p}/stages`, `GET/DELETE /projects/{p}/stages/{s}` |
| Apps | `GET/POST …/stages/{s}/apps`, `GET/PATCH/DELETE …/apps/{a}` |
| Variables | `GET …/stages/{s}/env`, `PUT …/stages/{s}/env`, `PATCH …/stages/{s}/env`; lo mismo en `…/apps/{a}/env`. `GET` enmascara los secretos. `PUT` reemplaza todo el set (`{"vars": {"K": "v"}, "secret": true}`, donde `secret` aplica a todas por defecto). `PATCH` hace upsert o borra (`{"set": {...}, "unset": ["K"]}`) |
| Deploys | `POST …/apps/{a}/deploys`, `GET …/apps/{a}/deploys`, `GET …/deploys/{id}`, `GET …/deploys/{id}/log?offset=N` → `{chunk, next_offset, status}`, `GET …/deploys/{id}/log/stream` (SSE), `POST …/apps/{a}/rollback` |
| Dominios | `GET/POST …/apps/{a}/domains`, `PATCH/DELETE …/domains/{hostname}`, `POST …/domains/{hostname}/adopt`, `POST …/domains/{hostname}/retry` |
| Estado | `GET …/apps/{a}/status` (por servicio: réplicas deseadas y corriendo, tareas con estado y error, CPU% y RAM actuales) |
| Logs | `GET …/apps/{a}/logs?service=<s>&tail=200&follow=1` (SSE; no se guarda nada) |
| Administración | `/users` (incluye `DELETE /users/{username}` y `DELETE /users/{username}/totp` para quien perdió su teléfono), `/invites`, `/tokens`, `/registries`, `/settings`, `/audit` |
| Sesión | `POST /auth/login`, `POST /auth/totp`, `POST /auth/logout`, `POST /auth/setup`, `POST /auth/invite/{token}`, `GET /auth/me`, `POST /auth/totp/setup`, `POST /auth/totp/enable` |

### 10.1 Autenticación y permisos

- **Sesión del panel:** cookie `muelle_session` (`HttpOnly`, `Secure`, `SameSite=Strict`). Se cierra tras 12 h sin uso y dura 7 días como máximo. Las peticiones que modifican algo exigen que el `Origin` coincida con `panel_domain`. Una sesión puede hacer todo.
- **Token de API:** `Authorization: Bearer mk_<43 caracteres base64url>` (32 bytes aleatorios). Se guarda su SHA-256 y se muestra una sola vez. Su alcance es un proyecto, o un proyecto + stage. **Solo permite:** leer proyecto/stage/app/estado/deploys/logs dentro del alcance, `GET/PUT/PATCH` de variables, crear deploys y hacer rollback. Cualquier otra operación responde `403`.
- **Login:**
  - Argon2id con `m=64MiB, t=3, p=2`.
  - Bloqueo progresivo por usuario: tras 5 fallos, 1 min; después se duplica hasta 1 h.
  - Por IP: 20 intentos cada 15 min.
  - Con TOTP, el login es en dos pasos: contraseña → código.
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
- **Ajustes:** Cloudflare (token, IP pública), email ACME, registries, usuarios e invitaciones, tokens, exigir 2FA, IPs permitidas para el panel y auditoría.

## 12. Seguridad

- **muelle tiene `docker.sock`, que equivale a ser root en el host.** Por eso: tokens con alcance mínimo, sesiones estrictas, rate limiting, 2FA disponible (y exigible) e IPs permitidas opcionales.
- **Secretos:**
  - Se cifran con AES-256-GCM y AAD ligada a la fila, así que un valor cifrado copiado a otra fila no se puede descifrar.
  - La clave maestra solo vive en el secreto Swarm; nunca en SQLite ni en logs.
  - Los valores se enmascaran en los logs de deploy.
  - El compose final no se persiste.
  - `details` de la auditoría nunca incluye valores.
- **Limitación aceptada:** quien tenga acceso a Docker en el host puede ver las variables con `docker inspect`. Esa persona ya es root.
- **Clave maestra:** se imprime en la instalación y se puede volver a leer con `docker exec` en el contenedor de muelle (`/run/secrets/muelle_master_key`). Si se pierde junto con el secreto Swarm, los secretos no se pueden recuperar.
- **Respaldo de muelle:** a diario, `VACUUM INTO /data/backups/muelle-<fecha>.db`, guardando los últimos 7.
- **Redes:** ver §3.2. Cada stage está aislado en su red; solo Caddy toca varias. La admin API de Caddy solo es accesible por el socket unix.

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
| Docker no responde | Los endpoints que usan Docker responden `503`; el panel muestra "degradado" |
| muelle se reinicia | Reconciliación (§7) |

## 14. GitHub Action (`action/action.yml`)

Composite action en bash, con `curl` y `jq` (ambos vienen en los runners de GitHub).

**Entradas:**
- `url`, `token`, `app` (`proyecto/stage/app`): obligatorias.
- `image`: opcional.
- `images-json`: opcional, para apps compose.
- `env-json`: opcional.
- `env-exclude`: lista separada por comas; siempre se agregan `GITHUB_TOKEN`, `MUELLE_URL` y `MUELLE_TOKEN`.
- `env-secret`: por defecto `true`.
- `timeout`: segundos, por defecto 600.

**Pasos:**
1. Si viene `env-json`, filtra las claves excluidas y hace `PUT …/apps/{a}/env`.
2. `POST …/deploys`.
3. Hace polling de `…/log?offset=N` cada 3 s e imprime el log.
4. Sale con código ≠ 0 si el deploy termina `failed` o si se pasa el timeout. Si termina `superseded` (lo reemplazó un deploy más nuevo de la misma app), emite un `::warning::` con el id del nuevo deploy y sale con 0.

**Ejemplo para auditfy** (reemplaza `sync-env.yaml` y `deploy.yaml`; `migrate.yaml` no cambia):

```yaml
deploy:
  needs: migrate
  runs-on: ubuntu-latest
  environment: ${{ github.ref_name == 'master' && 'production' || 'staging' }}
  steps:
    - uses: CPU-commits/muelle/action@v1
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

**Unitarios:**
- `render`: golden files (`testdata/*.input.yaml` → `*.golden.yaml`) cubriendo:
  - inyección de la red del stage, logging, update_config y etiquetas
  - servicios con volúmenes (stop-first y restricción de nodo)
  - respeto de los overrides del usuario
  - interpolación y variables faltantes
  - cada restricción de §6.3
- `proxy`: rutas → JSON golden de Caddy (incluye `admin.listen`, políticas TLS y `trusted_proxies`).
- `crypto`: ida y vuelta; un valor alterado o una AAD distinta fallan.
- `auth`:
  - matriz de alcance de tokens (operación × alcance → permitido o 403)
  - bloqueo progresivo
  - vencimiento de sesiones e invitaciones
  - flujo de TOTP
- `store`/reglas: mezcla de variables, validación de slugs, servicios duplicados dentro de un stage, cálculo de "cambios pendientes", retención.
- `dns`: contra un servidor HTTP falso: búsqueda de zona por sufijo, conflicto/adopción, borrar solo lo propio.

**Integración** (`//go:build integration`; en CI con `docker swarm init` en el runner):
- deploy de `traefik/whoami` por formulario y por compose
- convergencia `succeeded`
- healthcheck que falla → `failed` con rollback detectado
- `--prune` elimina un servicio quitado del compose
- deploy de un servicio con volumen (stop-first)
- Caddy real con `tls internal` enrutando a whoami por hostname
- crear un stage nuevo conecta Caddy a su red sin reiniciarlo (el contenedor conserva su id y su uptime)
- aislamiento: un contenedor de un stage no resuelve ni alcanza servicios de otro stage
- reconciliación tras matar muelle a mitad de un deploy

**Manual antes de migrar:**
- Cloudflare DNS-01 contra una zona de prueba.
- auditfy/staging completo desde GitHub Actions en una VM local.
- Verificar los criterios de éxito de §1.

**Frontend:** `svelte-check` en CI; pruebas manuales.

## 16. Migración desde Dokploy

Dokploy también usa Swarm, así que ambos pueden convivir en el mismo cluster. Lo único que choca son los puertos 80/443 que ocupa el Traefik de Dokploy. DNS-01 no necesita esos puertos, y eso permite preparar todo sin downtime:

1. Probar muelle completo en una VM local con Swarm.
2. En el servidor: `muelle install --cloudflare-token … --http-port 8080 --https-port 8443`. El panel queda en `https://panel…:8443`, con certificado por DNS-01.
3. Crear proyectos, stages, apps (mismas imágenes de GHCR), variables y dominios en muelle.
   - Los registros DNS de Cloudflare ya existen (apuntan al mismo servidor), así que cada dominio queda en "el registro ya existe" → **Adoptar**.
   - Como la IP pública es la misma, adoptar no cambia hacia dónde va el tráfico: sigue entrando por 80/443 al Traefik de Dokploy. Conviene adoptar con el mismo `proxied` que tiene hoy el registro.
4. Desplegar cada app y probarla contra muelle: `curl --resolve host:8443:<ip> https://host:8443/health`.
5. Cambio (segundos de downtime):
   - bajar el Traefik de Dokploy (`docker service scale dokploy-traefik=0`, o el nombre que corresponda)
   - en Ajustes de muelle, cambiar los puertos del proxy a 80/443; muelle recrea muelle-caddy
6. Cambiar los workflows a la action de muelle.
7. Dokploy queda detenido hasta confirmar que todo funciona y después se desinstala.

## 17. Orden sugerido de construcción

Cada fase deja algo que se puede probar de punta a punta:

1. **Base:** `store` (migraciones), `crypto`, `auth` (usuarios, sesiones, tokens) y el esqueleto de `api` con proyectos, stages, apps y variables.
2. **Deploy:** `docker`, `render` y `deployer`. Apps de formulario y compose desplegables por API, con log, rollback y reconciliación.
3. **Proxy y DNS:** el contenedor muelle-caddy, las rutas, `dns` de Cloudflare, la verificación de certificados, el estado y los logs en vivo.
4. **Panel:** el SPA de Svelte sobre la API ya existente.
5. **Distribución:** `install`, las imágenes (muelle y Caddy) y la GitHub Action. Después, la migración de auditfy.
