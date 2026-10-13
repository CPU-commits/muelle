# muelle — Contrato de las Fases 2, 3 y 5

> Fecha: 2026-10-13 · Estado: aprobado para la etapa de contratos (rama `fase-23-base`).
> Complementa el diseño ([`2026-10-09-muelle-design.md`](2026-10-09-muelle-design.md)); las referencias `§N` apuntan a sus secciones. Donde este documento y el diseño difieren, manda este documento, y el diseño ya se actualizó.

Las Fases 2 (deploy), 3 (ingress) y 5 (distribución) se implementan **en paralelo**, cada una en su rama y sin hablar con las otras. Este documento fija lo que comparten:

- la API HTTP que ve el panel, la GitHub Action y cualquier cliente;
- los puertos entre módulos, que ya están en el código;
- cómo `muelle install` le pasa la configuración al primer arranque.

El panel (`web/`, Fase 4) ya está hecho contra una versión supuesta de este contrato (`web/src/modules/{deploy,ingress}/README.md` y los DTOs de `admin`). La sección 7 lista dónde el contrato decidió algo distinto.

## 0. Convenciones

- **Base:** `/api/v1`, JSON en snake_case.
- **Fechas:** RFC 3339 en UTC, con a lo más milisegundos (`2026-10-10T19:36:53.77Z`).
- **Cuerpos:** se leen con `encoding/json/v2` en modo estricto, hasta 1 MiB. Un campo desconocido, una clave repetida o datos de más dan `400 invalid_json`.
- **Ids:** los recursos se identifican por slug. `{app}` abajo es `/projects/{p}/stages/{s}/apps/{a}`.
- **Errores:** `{"error": {"code": "…", "message": "…"}}`. El `message` es una oración en español para mostrar; los clientes deciden por el `code`.

### 0.1 Acceso

| Nivel | En el código | Quién |
|---|---|---|
| sesión | `utils.AccessSession` | Solo la cookie `muelle_session` (sesión completa). Un token recibe `403 token_not_allowed`. |
| token | `utils.AccessToken` | La sesión, o un token de API dentro de su alcance. `GET` es lectura; cualquier otro método es escritura (`Principal.Allows`). |

Las peticiones con cookie que modifican algo exigen `Origin` = `MUELLE_PANEL_ORIGIN` (`403 bad_origin`). Los tokens no lo necesitan.

### 0.2 Códigos de error comunes

| Status | `code` | Cuándo |
|---|---|---|
| 400 | `invalid_json` | El cuerpo no es el esperado (campo desconocido, tipo equivocado, JSON roto) |
| 401 | `unauthenticated`, `mfa_required` | Sin sesión, o con el 2FA a medias |
| 403 | `forbidden`, `token_not_allowed`, `out_of_scope`, `bad_origin`, `totp_setup_required` | Ver §10.1 |
| 404 | `no_route` | **La ruta no existe** (una fase que todavía no llegó). El panel muestra "Llega con la Fase N" |
| 404 | `not_found` | La ruta existe, pero el recurso no |
| 405 | `method_not_allowed` | La ruta existe con otro método (header `Allow`) |
| 409 | `conflict`, `not_empty` | Ya existe / primero hay que borrar lo que contiene |
| 422 | `invalid` | Validación. El mensaje empieza con `"<campo>: "`, así el panel lo marca en ese campo (ver abajo) |
| 429 | `rate_limited` | Con `Retry-After` |
| 501 | `not_implemented` | Un método de un esqueleto (`entity.ErrNotImplemented`); no debería llegar a producción |
| 502 | `cloudflare_unavailable` | Cloudflare no respondió (`entity.ErrCloudflareUnavailable`) |
| 503 | `docker_unavailable` | Docker no responde o el nodo no es manager de un Swarm (`entity.ErrDockerUnavailable`, §13) |

**El campo de un `422`** es el nombre del campo del cuerpo o de la query: `image`, `target_port`, `offset`. Hay dos formas compuestas:
- `images.<servicio>`, porque el panel marca el servicio;
- `compose_yaml`, para un problema dentro del compose.

Nunca va un índice (`panel_allowed_cidrs[1]`): el error de una lista nombra la lista.

### 0.3 SSE

Los streams (`…/log/stream` y `…/logs`) son `text/event-stream`:

- **Headers:** `Cache-Control: no-cache` y `X-Accel-Buffering: no`.
- **Eventos:** cada uno lleva `event:` y un `data:` de **una sola línea**: texto sin `\n`, o JSON compacto.
- **Heartbeat:** cada 15 s se manda el comentario `: ping`, para que Caddy y los navegadores no corten la conexión.
- **Plazo de escritura:** no hay `WriteTimeout` global (ver `newHTTPServer` en `main.go`). Cada handler pone el suyo con `http.ResponseController.SetWriteDeadline` antes de cada escritura (30 s): un cliente que no lee no retiene la goroutine.
- **Cierre:**
  - Cuando el cliente se va (`r.Context()`), el stream termina.
  - También termina con el `ctx` de `Module.Run`, que se cancela con la señal de apagado. Si no, `srv.Shutdown` esperaría los 8 s completos.
- **Errores antes de abrir:** un error antes del primer byte se responde como siempre (JSON con status). Después, solo se puede cerrar el stream.

## 1. Quién es dueño de qué

| Endpoints | Módulo | Fase |
|---|---|---|
| `POST/GET {app}/deploys`, `GET {app}/deploys/{id}`, `…/log`, `…/log/stream`, `POST {app}/rollback` | `deploy` | 2 |
| `GET {app}/status`, `GET {app}/logs` | `deploy` (tiene el cliente de Docker de los servicios) | 2 |
| `/registries` | `deploy` (son las credenciales de las imágenes) | 2 |
| `GET/POST {app}/domains`, `PATCH/DELETE {app}/domains/{hostname}`, `POST …/adopt`, `POST …/retry` | `ingress` | 3 |
| `GET /proxy` | `ingress` | 3 |
| `GET/PATCH /settings` (claves del proxy y el DNS) | `admin`, con el aviso a `ingress` | ya está (etapa de contratos) |

> El estado y los logs en vivo eran de la Fase 3 en §17. Pasan a la Fase 2: usan el mismo cliente de Docker que el deploy, y el panel ya los trata como Fase 2 (`ComingSoon phase={2}`).

Las rutas se registran recién cuando están implementadas. El esqueleto de `deploy.Module.RegisterRoutes` no registra nada, así que hoy esas rutas responden `404 no_route`.

### 1.1 Puertos entre módulos (ya en el código)

Los módulos no se importan (§4.1 regla 1): cada uno declara su interfaz en su `domain/service`, y `container.go` conecta los módulos por tipado estructural. Una firma distinta no compila.

| De → a | Puerto (dónde se declara) | Lo implementa | Para qué |
|---|---|---|---|
| catalog → ingress | `catalog/domain/service.StageNetworks`: `EnsureStageNetwork`, `RemoveStageNetwork` | `ingress.Module` (hecho) | Crear y borrar la red overlay del stage, y conectar o desconectar Caddy (§3.2). Opcional en `catalog.Deps`: con `nil`, catalog no toca Docker |
| catalog → deploy | `catalog/domain/service.AppRuntime`: `RemoveApp(ctx, p, s, a)` | `deploy.Module` (hecho: `docker stack rm`) | Al borrar una app (Fase 2: además detiene su worker). Opcional |
| deploy → secrets | `deploy/domain/service.EnvResolver`: `ResolveEnv(ctx, appID) (vars, secretValues, err)` | `secrets.Module` (hecho) | Las variables del stage con las de la app encima, y los valores secretos para enmascarar el log |
| deploy → ingress | `deploy/domain/service.Ingress`: `EnsureStageNetwork`, `SyncRoutes(ctx) error` | `ingress.Module` (red hecha; `SyncRoutes` es esqueleto) | §7 pasos 4 y 7 |
| admin → ingress | `admin/domain/service.ProxyHook`: `CheckCloudflareToken`, `ProxySettingsChanged(ctx, old, next)` | `ingress.Module` (esqueleto: no verifica y no aplica) | Ver §4.3 |

**Lecturas por SQL** (un adaptador en `infra/out/service` del que lee, que puede leer la tabla de otro módulo):

| Módulo | Lee | Escribe | Adaptador |
|---|---|---|---|
| deploy | `apps` (catalog) | Solo `storage_node_id` (si está vacío) y `last_deployed_at` (solo hacia adelante) | `deploy/infra/out/service.NewCatalogApps` → `service.Apps` (hecho) |
| deploy | `domains` (ingress): los destinos, para validar el render (§6.3) | — | `NewDomainTargets` → `service.DomainTargets` (hecho) |
| deploy | `settings.manager_node_id` | `manager_node_id` al arrancar | Fase 2 |
| ingress | `settings` (admin): `entity.ProxySettings`, con el token descifrado | `cloudflare_zones` y las claves reservadas de §3.6 | `ingress/infra/out/service.NewProxySettingsReader` (hecho) |
| ingress | `apps`, `stages` y `projects` (catalog), para el kind de la app y los nombres de los servicios | — | Fase 3 |
| ingress | `registries` (deploy), solo si `caddy_image` es privada | — | Opcional, Fase 3 |
| catalog | `domains`: cuántos tiene una app antes de borrarla | — | Hecho |

`deploy` lee `domains` por SQL y no llama a `ingress` para eso: así la Fase 2 se prueba sin la Fase 3.

### 1.2 Arranque y segundo plano

| Momento | Qué | Dónde |
|---|---|---|
| `container.New` | Migra, verifica la clave maestra y guarda los ajustes de arranque: `panel_domain` siempre, y `MUELLE_INIT_*` solo en el primer arranque de la base (§5.2) | `admin.ApplyStartupSettings` (hecho) |
| `main` → `cron.Run(bgCtx, c.Jobs)` | Corre cada job **al arrancar** y después según su intervalo | `cmd/cron` |
| `main` → `c.Run(bgCtx)` | `deploy.Run` e `ingress.Run` en paralelo; vuelve cuando los dos terminaron | `container.Run` (hecho) |

**`deploy.Run(ctx)`** (Fase 2) hace, en orden, la reconciliación de §7:
1. Guarda `manager_node_id` con `Docker.Info().NodeID` si falta. Con Docker caído, lo reintenta en el primer deploy.
2. Cierra los deploys `running` según el estado real de sus servicios (la misma regla de convergencia del paso 6), con una línea en su log: "muelle se reinició durante el deploy".
3. Retoma los `queued`.
4. Queda con un worker por app hasta que `ctx` se cancela.

Un `docker stack deploy` cortado por el apagado se resuelve en el próximo arranque, por el paso 2.

**`ingress`** (Fase 3):
- El job `"proxy"`, cada 5 min y al arrancar:
  - asegura muelle-caddy (§3.1) y la red `muelle_panel`;
  - asegura las redes de todos los stages y conecta Caddy;
  - borra las redes con etiquetas de stage (`muelle.project`/`muelle.stage`) cuyo stage ya no existe;
  - reenvía la configuración completa.
- El job `"certificados"`, cada hora.
- `ingress.Run` lleva los reintentos con backoff (1 s, 2 s… 60 s) de una configuración que Caddy no aceptó, y la verificación de un dominio 30 s después de crearlo.

Un error de Docker al arrancar nunca detiene a muelle.

## 2. Deploys (Fase 2, módulo `deploy`)

### 2.1 El deploy

```json
{
  "id": 143,
  "status": "succeeded",
  "trigger": "api",
  "images": { "api": "ghcr.io/aumi-app/backend-app:sha-3f9c2e1@sha256:9b2de1f4…" },
  "actor_type": "token",
  "actor_id": 4,
  "actor_name": "github-actions",
  "error": null,
  "superseded_by": null,
  "created_at": "2026-10-10T14:19:49.12Z",
  "started_at": "2026-10-10T14:19:49.3Z",
  "finished_at": "2026-10-10T14:20:41.002Z"
}
```

| Campo | Tipo | Qué |
|---|---|---|
| `id` | int > 0 | Autoincremental; nunca se reutiliza |
| `status` | `queued` · `running` · `succeeded` · `failed` · `superseded` | Los finales son `succeeded`, `failed` y `superseded` |
| `trigger` | `panel` · `api` · `rollback` | `panel` si lo pidió una sesión, `api` si un token, `rollback` por `POST …/rollback` |
| `images` | objeto `servicio → ref` | **Mientras está `queued`:** los overrides del cuerpo (`{}` si no hubo). **Desde que se resolvieron (§7 paso 2):** todos los servicios, con `ref@sha256:…`. Si falló antes de resolver, quedan los overrides |
| `actor_type` | `user` · `token` | |
| `actor_id` | int o null | El id del usuario o del token |
| `actor_name` | string | El username o el nombre del token **al crear el deploy**: sigue aunque se borren. Nunca es null |
| `error` | string o null | Por qué falló, en una línea de hasta 500 caracteres, ya enmascarada. Null si no falló |
| `superseded_by` | int o null | El deploy que lo dejó `superseded` |
| `created_at` | RFC 3339 | |
| `started_at`, `finished_at` | RFC 3339 o null | Null mientras no empezó o no terminó. Un `superseded` tiene `finished_at` y no `started_at` |

El log no viene en el objeto: se lee con `…/log` (§2.5).

### 2.2 `POST {app}/deploys` → 202 · token

Crea un deploy y lo encola (§7 paso 1). El cuerpo es opcional; vacío es lo mismo que `{}`.

| Cuerpo | Qué |
|---|---|
| `{}` (o vacío) | Redespliega las refs actuales y vuelve a resolver los digests |
| `{"image": "<ref>"}` | Solo en una app de formulario: la imagen de su único servicio |
| `{"images": {"<servicio>": "<ref>"}}` | En los dos tipos: solo los servicios que cambian. En una app de formulario el único servicio se llama como la app |

**Respuesta** (con `Location` = `status_url`):

```json
{ "id": 145, "status": "queued", "status_url": "/api/v1/projects/auditfy/stages/staging/apps/api/deploys/145" }
```

**Errores:**
- `404 not_found`: la app no existe.
- `422 invalid`:
  - `image: manda image o images, no los dos`;
  - `image: una app compose recibe images, por servicio`;
  - `image: …` o `images.<servicio>: …` si la ref no pasa `validate.ImageRef`;
  - `images.<servicio>: el servicio no existe en la app` (según la configuración actual).
- `400 invalid_json`.

**Al encolar:** los deploys `queued` de la misma app pasan a `superseded`, con `superseded_by` = el nuevo y `finished_at`. Solo sobrevive el último. Un deploy `running` no se toca: el nuevo espera.

No se llama a Docker al encolar: con Docker caído igual se responde `202`, y el deploy falla en el paso 2 con "Docker no responde".

**Auditoría:** `deploy.create`, target `app:<p>/<s>/<a>`, details `{"deploy": 145, "trigger": "api", "images": {…overrides}}`. Las refs no son secretas.

#### Qué imagen se despliega

Para cada servicio, en este orden:
1. El override del cuerpo, si viene.
2. Si no, la ref sin digest de ese servicio en el **deploy actual** (el `succeeded` más reciente). Una excepción: si la ref de la configuración (`spec.image` o el `image:` del compose) cambió desde ese deploy, se usa la de la configuración. Para saberlo, cada deploy guarda en `deploys.config_images` las refs de la configuración que leyó.
3. Si el servicio no estaba en el deploy actual (primer deploy, o un servicio nuevo del compose), la ref de la configuración.

Así:
- un deploy sin cuerpo después de un deploy de CI con `sha-…` mantiene esa imagen;
- editar `spec.image` en el panel y desplegar sin override usa la imagen nueva;
- un rollback no se deshace con el próximo deploy sin cuerpo.

Si una ref ya trae digest (`…@sha256:…`), el paso 2 solo verifica que exista y que haya acceso, y lo conserva.

### 2.3 `GET {app}/deploys` → 200 · token

Un array plano con los últimos 20, del más nuevo al más viejo (§5.2 regla 7). Cada elemento tiene la forma de §2.1.

La retención se aplica al cerrar cada deploy: borra los más viejos que no estén `queued` ni `running`.

### 2.4 `GET {app}/deploys/{id}` → 200 · token

Un deploy (§2.1). `404 not_found` si no existe **en esa app**: un id de otra app también es `not_found`, aunque exista.

### 2.5 `GET {app}/deploys/{id}/log?offset=N` → 200 · token

El polling de la API (la GitHub Action, §14) y el respaldo del panel cuando el SSE falla.

```json
{ "chunk": "2026-10-10T14:32:07.123Z resolviendo imágenes\n", "next_offset": 1234, "status": "running", "superseded_by": null }
```

**El log:**
- Es un texto con una línea por evento: `<RFC 3339 con ms> <texto>\n`.
- Ya viene enmascarado: cada valor de una variable `secret` de la app o del stage es `***` (los de `ResolveEnv`, del más largo al más corto).
- Solo crece: los offsets son bytes desde el principio y nunca se mueven.
- Una línea larga se corta a 8 KiB con `…`.
- **Tope de 256 KB:** al llegar, se agrega una vez `… (log truncado: se superaron los 256 KB)` y se descartan las líneas siguientes. Se reservan 4 KiB para las líneas de cierre (el resultado, el error), que siempre se escriben.
- Cada línea se guarda en la base **antes** de transmitirse por SSE: lo que se vio por el stream ya está en `?offset`.

**La respuesta:**
- `chunk` va desde `offset`, hasta 64 KiB y siempre en líneas completas. Puede ser `""`.
- `next_offset` es `offset + len(chunk)` en bytes.
- `status` es el del deploy en ese momento. `superseded_by` se repite acá para que la Action no pida otra cosa.

**El offset:**
- `offset` ausente es 0.
- No es un entero ≥ 0, o pasa el largo del log: `422 invalid` con `offset: …`.
- Se usa 0 o un `next_offset` que devolvió el servidor. Otro valor puede caer a mitad de una línea.

**Fin:** el cliente termina cuando `status` es final y `chunk` viene vacío.

### 2.6 `GET {app}/deploys/{id}/log/stream` → `text/event-stream` · token

Los mismos bytes que §2.5, en vivo. **Desde dónde empieza:** el header `Last-Event-ID` si viene; si no, `?offset=N`; si no, 0.

```
event: status
data: running

event: log
id: 1234
data: 2026-10-10T14:32:07.123Z resolviendo imágenes

event: status
data: succeeded
```

- **`log`:** una línea del log por evento, sin el `\n`. El `id` es el `next_offset` después de esa línea, con la misma semántica que el polling.
- **`status`:** el estado, en el `data`.
  - Se manda al conectar si el deploy **no** terminó, y cada vez que cambia.
  - Las líneas de un paso van antes del `status` que lo cierra.
  - **Un `status` final siempre va después de la última línea.** Si el deploy ya había terminado al conectar, primero van las líneas que faltan, después el `status` final, y se cierra.
- **Cierre:** después del `status` final, el servidor cierra. El panel también cierra al recibirlo.
- **Errores:** un `404 not_found` o un `422` (offset) se responde como JSON, antes de abrir el stream.

### 2.7 `POST {app}/rollback` → 202 · token

Sin cuerpo (o `{}`; otra cosa es `400 invalid_json`). Responde como §2.2. Crea un deploy con `trigger: "rollback"` y las imágenes, con digest, del `succeeded` inmediatamente anterior al actual (§5.2 regla 6). Usa la configuración **actual**.

Si no hay deploy actual, o no hay un `succeeded` anterior: `409 no_previous_deploy`.

Dos rollbacks seguidos alternan entre las dos últimas versiones: es la regla del diseño, y el panel lo muestra como "Rollback a #N".

**Auditoría:** `deploy.create` con `trigger: "rollback"`.

### 2.8 `GET {app}/status` → 200 · token

El estado real en Swarm de los servicios de la app (filtrados por `naming.AppLabels`).

```json
{
  "services": [
    {
      "name": "api",
      "image": "ghcr.io/aumi-app/backend-app:sha-3f9c2e1@sha256:9b2de1f4…",
      "replicas_desired": 2,
      "replicas_running": 2,
      "cpu_percent": 3.4,
      "memory_bytes": 622854144,
      "memory_limit_bytes": 536870912,
      "update_state": "completed",
      "tasks": [
        {
          "id": "k3x9q2m0a1b2c3d4e5f6g7h8i",
          "slot": 1,
          "state": "running",
          "desired_state": "running",
          "health": "healthy",
          "node": "manager-1",
          "since": "2026-10-10T14:20:05Z",
          "error": null
        }
      ]
    }
  ]
}
```

**El servicio:**

| Campo | Qué |
|---|---|
| `services` | Ordenados por `name`. `[]` si la app nunca se desplegó o su stack se borró |
| `name` | El nombre **corto** (`proyecto_stage_app_api` → `api`) |
| `image` | La imagen de sus tareas (`Service.Image`) |
| `replicas_desired` | `Service.Replicas`, o `DesiredTasks` en un servicio global |
| `replicas_running` | `RunningTasks` |
| `cpu_percent`, `memory_bytes` | La suma de las tareas que corren en el nodo local (`ContainerStats`, en paralelo, con un plazo total de 2 s). `cpu_percent` es el % de un núcleo, como `docker stats`. Null si no se pudo medir |
| `memory_limit_bytes` | El límite de **cada** tarea (`Service.MemoryLimitBytes`); null si no tiene |
| `update_state` | `Service.Update.State`, o null si nunca se actualizó |

**Cada tarea:**

| Campo | Qué |
|---|---|
| `tasks` | Las vigentes y las históricas que guarda Swarm. Primero las que tienen `desired_state: running`, después por `slot` y por `since`, de la más nueva a la más vieja |
| `id` | El id de la tarea |
| `slot` | Null en un servicio global |
| `state`, `desired_state` | Los de Swarm, tal cual |
| `health` | `healthy`, `unhealthy` o `starting`, del contenedor en el nodo local (`InspectContainer`); null si no tiene healthcheck o no está en este nodo |
| `node` | El hostname del nodo (`Node`, con caché) |
| `since` | `Task.UpdatedAt` |
| `error` | `Task.Err`; null si está vacío |

El panel lo pide cada 5 s mientras la pestaña está visible. El backend puede guardar la respuesta en caché hasta 2 s por app.

**Errores:** `404 not_found` si la app no existe; `503 docker_unavailable`.

### 2.9 `GET {app}/logs?service=<s>&tail=<n>&follow=1` → `text/event-stream` · token

Los logs de un servicio, en vivo. No se guarda nada (§10).

**La query:**

| Query | Qué |
|---|---|
| `service` | El nombre corto. Obligatorio (`422 invalid` con `service: es obligatorio`). Si no existe en Swarm para la app: `404 not_found` |
| `tail` | De 0 a 5000; 200 por defecto. Otro valor: `422 invalid` con `tail: …` |
| `follow` | `1` deja el stream abierto. Si no viene, o es `0`, manda las últimas `tail` líneas, un `end` y cierra |

**Los eventos:**

```
event: log
data: {"time":"2026-10-10T14:31:40.118Z","task":"api.2","stream":"stdout","line":"WARN consulta lenta · 812 ms"}

event: end
data: eof
```

- **`log`:** `data` es JSON compacto.
  - `task` es `<servicio>.<slot>`, o `<servicio>.<primeros 7 del id de la tarea>` en un servicio global.
  - `stream` es `stdout` o `stderr`.
  - `line` va sin el `\n` final, cortada a 16 KiB.
- **Enmascarado:** como el log del deploy, con los valores secretos de la app y del stage. Es una defensa extra: las apps a veces imprimen su configuración.
- **`end`:** se termina el log (el servicio se borró, o no había `follow`). El servidor cierra después.

**Errores antes de abrir:** `404`, `422` y `503 docker_unavailable`.

### 2.10 Registries (`/registries`) · sesión

Son las credenciales de los registries privados, una por host, con la tabla `registries` de la migración 0002. Las maneja `deploy`, que es quien las usa.

```json
{ "host": "ghcr.io", "username": "bot", "created_at": "…", "updated_at": "…" }
```

**El host** es lo que va antes de la primera `/` de una imagen, como lo da `reference.Domain(reference.ParseNormalizedNamed(ref))`:
- `ghcr.io`, `registry.example.com:5000`, `localhost:5000`;
- `docker.io` para Docker Hub.

Se valida en su forma canónica, sin normalizar: minúsculas, etiquetas `[a-z0-9-]` separadas por `.`, `:puerto` opcional, sin esquema, ruta ni `/` final. Va url-encoded en la ruta. El panel lo normaliza antes de mandarlo.

| Endpoint | Cuerpo | Respuesta | Errores |
|---|---|---|---|
| `GET /registries` | — | `200 [registry]`, ordenados por host | |
| `POST /registries` | `{host, username, password}` | `201 registry` | `409 conflict` si el host ya está; `422` en `host`, `username` (1–255, sin caracteres de control) o `password` (1–4096 bytes, sin saltos de línea) |
| `PATCH /registries/{host}` | `{username, password}`, los dos obligatorios: reemplaza las credenciales | `200 registry` | `404 not_found`, `422` |
| `DELETE /registries/{host}` | — | `204` | `404 not_found` |

- **La contraseña nunca vuelve.** Se cifra con la AAD `registries:password:<id>` (ya está en la lista de `shared/infra/out/crypto`).
- **Auditoría:** `registry.create`, `registry.update` y `registry.delete`, con target `registry:<host>`. En create y update, details `{"username": "…"}`.
- **Uso en el deploy:**
  - el paso 2 pasa la credencial del host de cada imagen a `DistributionDigest` (exacta por host; sin credencial es anónima);
  - el paso 5 pasa a `DeployStack` solo las credenciales de los hosts que usa el compose.
- Cambiar o borrar un registry no redespliega nada.

### 2.11 Lo que escribe el deploy en el log

No es un contrato de texto (los clientes deciden por `status`), pero cada paso de §7 deja al menos una línea, en este orden:

```
… deploy #145 de auditfy/staging/api (api · github-actions)
… resolviendo imágenes
…   api: ghcr.io/aumi-app/backend-app:sha-3f9c2e1 → sha256:9b2d…
… render: 1 servicio
… red auditfy_staging lista
… docker stack deploy auditfy_staging_api
<la salida del CLI, una línea por línea>
… esperando convergencia (hasta 300 s)
… api: 2/2 réplicas corriendo
… rutas de Caddy actualizadas            (o: advertencia: no se pudo actualizar Caddy: …)
… succeeded en 34 s                      (o: failed: <motivo>)
```

Con `rollback_*` o `paused` (§7 paso 6), el log recibe el `Status.Err` de la última tarea fallida y sus últimas 50 líneas (`TaskLogs`). Un error de `SyncRoutes` es solo una advertencia (§13).

## 3. Dominios y proxy (Fase 3, módulo `ingress`) · sesión

Ningún endpoint de esta sección acepta tokens (§10.1).

### 3.1 El dominio

```json
{
  "hostname": "api.tudominio.cl",
  "target_service": "api",
  "target_port": 3000,
  "proxied": true,
  "tls_mode": "cloudflare",
  "cf_zone_id": "023e105f4ecef8ad9ca31a8372d0c353",
  "cf_record_id": "372e67954025e0ba6aaa6d586b9e0b59",
  "status": "pending",
  "last_error_code": null,
  "last_error": null,
  "cert_issuer": null,
  "cert_expires_at": null,
  "records": [
    { "type": "A", "name": "api.tudominio.cl", "content": "203.0.113.24" }
  ],
  "created_at": "2026-10-01T12:00:00Z",
  "updated_at": "2026-10-01T12:00:00Z"
}
```

| Campo | Qué |
|---|---|
| `hostname` | Único en todo muelle, en su forma canónica (`validate.Hostname`: minúsculas, sin punto final, sin comodines) |
| `target_service`, `target_port` | El nombre corto del servicio y su puerto. Caddy enruta a `naming.Upstream(naming.Service(p, s, a, target_service), target_port)` |
| `proxied` | Detrás del proxy de Cloudflare. Solo tiene efecto si muelle maneja el registro (`cf_record_id`) |
| `tls_mode` | `cloudflare` (DNS-01) · `http` (HTTP-01) |
| `cf_zone_id` | La zona de Cloudflare; null si no cae en ninguna, o si no se pudo consultar |
| `cf_record_id` | El registro `A` que maneja muelle; null si no lo maneja (§9). El `AAAA` (`cf_record_id_aaaa`) no se expone |
| `status` | `pending` · `active` · `error` |
| `last_error_code` | Solo con `error`: `record_exists` (→ Adoptar), `cloudflare` (→ Reintentar) o `certificate` (→ Reintentar). La migración 0003 garantiza que un `error` siempre trae código |
| `last_error` | El mensaje, en español |
| `cert_issuer`, `cert_expires_at` | Lo último que vio la verificación de §8 |
| `records` | **Calculado, no se guarda:** los registros que el dominio necesita con los ajustes actuales. Un `A` con `public_ip` si hay, y un `AAAA` con `public_ipv6` si hay. `name` es el hostname. Con `cf_record_id` son los que maneja muelle; sin zona, los que hay que crear a mano. `[]` sin IP pública |

**Estados:**
- **Crear:**
  - Con token, y el hostname cae en una zona visible (el sufijo más largo):
    - Se crean `A` (y `AAAA` si hay `public_ipv6`) con el `proxied` pedido, y queda `pending` con `tls_mode: cloudflare`.
    - Si ya hay un registro de ese nombre que muelle no creó, no se toca: `error` con `record_exists`, sin `cf_record_id`.
    - Si Cloudflare falla: `error` con `cloudflare`. No se responde `502`: un error de DNS deja el dominio en error, y el `POST` igual responde `201`.
    - Si no hay `public_ip`: `error` con `cloudflare`, y el mensaje "configura la IP pública en Ajustes".
  - Sin token, o sin zona: `tls_mode: http`, `pending`, y el registro se crea a mano.
  - Si no se pudieron listar las zonas: `error` con `cloudflare`, con `tls_mode: http` provisorio. El reintento rehace todo.
- **`pending` → `active`:** cuando la verificación de §8 ve un certificado válido que vence en más de 14 días. Corre a los 30 s de crear o reintentar, y cada hora.
- **Un `pending` sin certificado válido 10 min después** de su último cambio pasa a `error` con `certificate`. Mientras tanto, el certificado autofirmado o vacío que sirve Caddy al emitir no cuenta como error.
- **`active` → `error`** con `certificate`: el certificado es inválido o vence en menos de 14 días.

### 3.2 Endpoints

| Endpoint | Cuerpo | Respuesta |
|---|---|---|
| `GET {app}/domains` | — | `200 [dominio]`, ordenados por hostname |
| `POST {app}/domains` | `{hostname, target_service, target_port, proxied?}` (`proxied` por defecto `false`) | `201 dominio` |
| `PATCH {app}/domains/{hostname}` | `{proxied?, target_service?, target_port?}`; `{}` no cambia nada | `200 dominio` |
| `DELETE {app}/domains/{hostname}` | — | `204` |
| `POST {app}/domains/{hostname}/adopt` | sin cuerpo | `200 dominio` |
| `POST {app}/domains/{hostname}/retry` | sin cuerpo | `200 dominio` |

`{hostname}` va url-encoded y tiene que pertenecer a esa app. Si no: `404 not_found`, aunque exista en otra.

**Validación** (`422 invalid`):
- `hostname`: `validate.Hostname`. Tampoco puede ser `panel_domain` ("es el dominio del panel").
- `target_service`: un slug de hasta 16 caracteres. En una app de formulario, tiene que ser el slug de la app. En una app compose, en la v1 no se verifica al guardar: lo verifica el render (§6.3), y el deploy falla con el dominio y el servicio en el mensaje.
- `target_port`: de 1 a 65535.
- En un `PATCH`, `proxied` sobre un dominio sin `cf_record_id`: "muelle no maneja el registro DNS de este dominio".
- El `hostname` no se edita: mandarlo en el `PATCH` es `400 invalid_json`.

**Comportamiento:**
- `POST` con un hostname que ya usa cualquier app: `409 conflict`.
- Crear, editar y borrar aplican de inmediato el DNS y la ruta de Caddy, sin deploy (§5.2 regla 4).
  - Primero se guarda, y después `SyncRoutes`.
  - Un fallo de Caddy no hace fallar la petición: se ve en `GET /proxy` y se reintenta.
- `PATCH` de `proxied` actualiza los registros que maneja muelle. Si Cloudflare falla, se guarda el valor pedido, el dominio queda en `error` con `cloudflare` y se responde `200`.
- `DELETE`:
  - Con `cf_record_id`, y con token, borra primero el registro en Cloudflare. Un registro que ya no existe allá no es error.
  - Si Cloudflare no responde: `502 cloudflare_unavailable`, y no se borra nada.
  - Sin token, borra solo en muelle, con `{"dns": "kept"}` en la auditoría.
- `adopt`:
  - Solo con `error` + `record_exists`; si no, `409 nothing_to_adopt`.
  - Actualiza el contenido del registro existente a la IP pública y **conserva su `proxied`**, que se copia al dominio. Adoptar nunca cambia el modo de Cloudflare (§16); para cambiarlo, un `PATCH` después.
  - Guarda `cf_record_id` y deja el dominio `pending`.
- `retry`, sincrónico, con a lo más 20 s:
  - `cloudflare`: rehace el DNS (zona y registros).
  - `record_exists`: vuelve a mirar. Si el registro ajeno ya no está, lo crea.
  - `pending`, `certificate` y `active`: verifica el certificado ahora.
  - Sin zona: vuelve a buscar la zona, por si se agregó un token o una zona.
  - Responde con el estado resultante.
- **Auditoría:** `domain.create`, `domain.update`, `domain.delete`, `domain.adopt` y `domain.retry`, con target `domain:<hostname>` y details `{"app": "<p>/<s>/<a>"}`.

### 3.3 `GET /proxy` → 200 · sesión

El estado del proxy, para el banner "proxy desincronizado" (§8, §13):

```json
{
  "status": "synced",
  "error": null,
  "synced_at": "2026-10-10T14:20:41Z",
  "caddy": { "image": "ghcr.io/cpu-commits/muelle-caddy:v0.1.0", "running": true, "http_port": 80, "https_port": 443 }
}
```

| Campo | Qué |
|---|---|
| `status` | `synced`: la última configuración se aplicó. `out_of_sync`: la última falló, y se reintenta con backoff. `disabled`: `ProxySettings.Enabled` es falso (sin `panel_domain` o sin imagen de Caddy, en desarrollo) |
| `error` | El motivo de `out_of_sync`, o del último fallo del contenedor (un puerto ocupado al recrear, la imagen que no baja) |
| `synced_at` | El último `POST /load` exitoso |
| `caddy.image`, `caddy.running` | Lo que corre de verdad, según `InspectContainer` |
| `caddy.http_port`, `caddy.https_port` | Los puertos de los ajustes |

Es un estado en memoria, más `InspectContainer`. Con Docker caído: `503 docker_unavailable`.

### 3.4 Caddy

Lo del diseño (§3.1 y §8), con estas precisiones:

- **IPs permitidas en el panel:** el matcher es **`client_ip`** (`{"client_ip": {"ranges": […]}}`), no `remote_ip`.
  - `remote_ip` mira el peer directo, que con el panel `proxied` es el edge de Cloudflare.
  - `client_ip` usa la IP real que resuelve `servers.trusted_proxies`: los rangos de Cloudflare (`{"source": "static", "ranges": […]}`), con `client_ip_headers: ["CF-Connecting-IP", "X-Forwarded-For"]`.
  - Es la misma IP que muelle recibe en `X-Real-IP` y muestra como `client_ip` en `GET /settings`.
  - Una IP fuera de la lista recibe `403`, y la ruta del panel no se rompe.
- **Configuración siempre completa**, con `admin.listen: "unix//run/caddy-admin/admin.sock"` (`naming.CaddyAdminSocket`).
- **muelle-caddy**: lo crea y lo recrea `ingress`, solo si `ProxySettings.Enabled(naming.CaddyImageFor(version))`.
  - **Contenedor:** `naming.CaddyContainer`, `--restart unless-stopped`.
  - **Puertos:** `proxy_http_port` → 80/tcp, y `proxy_https_port` → 443/tcp y 443/udp.
  - **Volúmenes:** `muelle_caddy_data:/data`, `muelle_caddy_config:/config` y `muelle_caddy_admin:/run/caddy-admin`.
  - **Redes:** se crea en `muelle_panel` y se conecta a cada red de stage.
  - **Cuándo se recrea:** si cambian la imagen o los puertos, o si corre sin publicar sus puertos (`len(PublishedPorts) == 0` con `Ports` no vacío).
  - **Recrear es** borrar y crear uno nuevo, **nunca rearrancar el mismo.** Docker 29 arranca sin puertos un contenedor cuyo arranque falló con `ErrPortInUse` (el fake lo imita).
  - **Puerto ocupado** (`docker.ErrPortInUse`): el contenedor anterior ya no existe. Se reintenta con los mismos ajustes en el job `"proxy"` y el error queda en `GET /proxy`. No se vuelve solo a los puertos viejos.
- **Imagen:** `caddy_image` si está, o `naming.CaddyImageFor(version)`. Se baja con `PullImage` (anónimo) si `InspectImage` da `ErrNotFound`.
- **Ruta del panel:** `panel_domain → naming.MuelleService:naming.MuellePort` (`muelle_muelle:8080`), con `header_up X-Real-IP {client_ip}`.
- **TLS del panel:**
  - `cloudflare` si hay token y `panel_domain` cae en una zona visible; si no, `http`.
  - El registro DNS del panel se crea solo si no existe. Uno existente nunca se toca, y en la v1 no se adopta.
  - Sus ids se guardan en las claves reservadas de §3.6.

### 3.5 Los hooks de admin

- **`CheckCloudflareToken(ctx, token)`** (antes de guardar un token nuevo en `PATCH /settings`):
  - verifica el token (`GET /user/tokens/verify`) y cuenta las zonas que ve;
  - un token rechazado, o sin zonas, devuelve un `*validate.Error` en `cloudflare_token` (`422`);
  - si Cloudflare no responde, `entity.ErrCloudflareUnavailable` (`502`);
  - si falla, no se guarda nada del `PATCH`.
- **`ProxySettingsChanged(ctx, old, next)`** (después de guardar, antes de responder):
  - corre con su propio plazo de 2 min, derivado con `context.WithoutCancel`;
  - aplica, según `next.ChangedKeys(old)`:
    - IPs, email, token o lista de IPs → regenera la configuración de Caddy;
    - IP pública → actualiza los registros que maneja muelle (los que fallan dejan su dominio en `error`/`cloudflare`);
    - puertos o imagen → recrea muelle-caddy;
  - no devuelve error: el resultado se ve en `GET /proxy` y en cada dominio.
  - El `PATCH` responde cuando termina: el panel ya avisa que cambiar los puertos corta unos segundos.

### 3.6 Claves de settings que escribe ingress

| Clave | Qué |
|---|---|
| `cloudflare_zones` | Cuántas zonas ve el token, en decimal. La escribe admin al guardar un token; ingress la actualiza en el job `"proxy"` |
| `panel_cf_zone_id`, `panel_cf_record_id`, `panel_cf_record_id_aaaa` | El registro DNS del panel que creó muelle. Reservadas: la Fase 3 agrega sus constantes en `shared/domain/entity` |

## 4. Settings (`GET/PATCH /settings`) · sesión — ya implementado

`GET` (y la respuesta de `PATCH`) trae **siempre** todas las claves, sin `omitempty`. El panel usa su presencia para saber que la fase llegó.

```json
{
  "require_2fa": false,
  "cloudflare_token_set": true,
  "cloudflare_zones": 4,
  "public_ip": "203.0.113.24",
  "public_ipv6": "",
  "acme_email": "tu@correo.cl",
  "panel_domain": "panel.tudominio.cl",
  "panel_allowed_cidrs": [],
  "proxy_http_port": 80,
  "proxy_https_port": 443,
  "client_ip": "190.5.6.7"
}
```

| Campo | Qué |
|---|---|
| `cloudflare_token_set` | Si hay token. **El token nunca vuelve** |
| `cloudflare_zones` | Valor guardado (no se consulta a Cloudflare en cada `GET`); null si no se sabe o no hay token |
| `public_ip`, `public_ipv6`, `acme_email` | `""` si no están |
| `panel_domain` | **Solo lectura**: el host de `MUELLE_PANEL_ORIGIN` si es un dominio, o `""` (una IP o `localhost`, en desarrollo). Se escribe en cada arranque |
| `panel_allowed_cidrs` | Prefijos normalizados (`validate.CIDRs`). Nunca null; `[]` es cualquier IP |
| `client_ip` | La IP con que muelle ve esta petición (`utils.ClientIP`). No es un ajuste |

### 4.1 `PATCH /settings`

Manda solo las claves que cambian: `require_2fa`, `cloudflare_token`, `public_ip`, `public_ipv6`, `acme_email`, `panel_allowed_cidrs`, `proxy_http_port`, `proxy_https_port` y `confirm_lockout`. Cualquier otra, `panel_domain` incluida, es `400 invalid_json`.

| Regla | Respuesta |
|---|---|
| `public_ip` no es una IPv4 canónica, `public_ipv6` una IPv6 (RFC 5952), o `acme_email` un email | `422` en ese campo. `""` borra el valor |
| Un puerto fuera de 1–65535 | `422` en ese campo |
| `proxy_http_port` = `proxy_https_port` (con los valores resultantes) | `422` en `proxy_https_port` |
| Un rango inválido en `panel_allowed_cidrs` | `422` en `panel_allowed_cidrs` |
| `cloudflare_token` con otra forma (de 20 a 512 caracteres visibles, sin espacios) | `422 cloudflare_token` |
| `cloudflare_token` rechazado por Cloudflare | `422 cloudflare_token` |
| Cloudflare no responde al verificar el token | `502 cloudflare_unavailable` |
| `cloudflare_token: ""` | Borra el token y `cloudflare_zones` |
| `require_2fa: true` sin el 2FA propio | `409 totp_required_first` |
| Una lista de `panel_allowed_cidrs` que deja fuera `client_ip` | `409 would_lock_out`, salvo que venga `"confirm_lockout": true` |

- Todo se guarda en una transacción. Después, si cambió algo del proxy, se llama a `ProxySettingsChanged`.
- Un error de DNS al cambiar la IP pública nunca hace fallar el `PATCH` (§9).
- **Auditoría:** `settings.update` con las claves y sus valores. El token solo aparece como `"set"` o `"removed"`.

**Recuperar el acceso** después de guardar una lista que te deja fuera, desde el servidor. La imagen de muelle no trae `sqlite3`, así que se usa un contenedor aparte sobre el mismo volumen (SQLite en modo WAL admite la escritura con muelle corriendo):

```bash
docker run --rm -v muelle_data:/data alpine sh -c \
  "apk add -q sqlite && sqlite3 /data/muelle.db \"UPDATE settings SET value='[]' WHERE key='panel_allowed_cidrs'\""
```

Después se reinicia muelle (`docker service update --force muelle_muelle`), o se espera al job `"proxy"`, que reenvía la configuración cada 5 min. La Fase 5 agrega el subcomando `muelle settings reset-panel-ips`, que hace lo mismo con `docker exec` en el contenedor de muelle.

## 5. Fase 5: install e imágenes

### 5.1 Nombres fijos (`shared/domain/naming`)

| Qué | Nombre |
|---|---|
| Imagen de muelle | `ghcr.io/cpu-commits/muelle:<versión>` (`naming.MuelleImageFor`) |
| Imagen de Caddy | `ghcr.io/cpu-commits/muelle-caddy:<versión>` (`naming.CaddyImageFor`). Se publica con el mismo tag que la de muelle |
| Versión | `vMAYOR.MENOR.PARCHE[-sufijo]`. Se fija al compilar: `go build -ldflags "-X main.version=v0.1.0"`. `dev` no tiene imagen de Caddy por defecto |
| Stack y servicio | `muelle` y `muelle_muelle` |
| Red | `muelle_panel`: overlay attachable, con subred privada. install verifica con `Network.Subnets` que sea RFC 1918 |
| Volúmenes | `muelle_data`, `muelle_caddy_data`, `muelle_caddy_config` y `muelle_caddy_admin` |
| Secretos Swarm | `muelle_master_key` (la clave maestra) y `muelle_init_cloudflare_token` (opcional) |
| Contenedor | `muelle-caddy`, que crea ingress en el primer arranque, no install |

### 5.2 La configuración inicial: `MUELLE_INIT_*`

install despliega el servicio con estas variables. `settings.Load` las valida en cada arranque: si una no sirve, muelle no arranca y el error nombra la variable.

**Se aplican una sola vez**, en el primer arranque de la base: `admin.ApplyStartupSettings` guarda en la misma transacción la marca `init_applied`, y en los arranques siguientes no mira `MUELLE_INIT_*`. En ese primer arranque guarda cada valor que no es cero si la clave todavía no está, y deja un evento `settings.init` con las claves, sin valores.

- **Por qué una marca, y no "si la clave falta":** las variables y el secreto del token quedan en el servicio. Sin la marca, lo que el admin borra en el panel volvería con cualquier reinicio (`docker service update --image`, un nodo que se reinicia). Borrar el token (`cloudflare_token: ""`) borra la clave, así que un token quitado, quizás por filtrado, volvería a usarse en Caddy y en el DNS.
- **Una reinstalación sobre el mismo volumen `muelle_data`** ya tuvo su primer arranque: los flags de install no cambian ningún ajuste. install lo avisa ("la base ya existe: cambia los ajustes en el panel").

| Flag de install | Variable | Clave de settings |
|---|---|---|
| `--panel-domain D` (+ `--https-port`) | `MUELLE_PANEL_ORIGIN=https://D`, o `https://D:<puerto>` si el puerto HTTPS no es 443 | `panel_domain` (en cada arranque) |
| `--email` | `MUELLE_INIT_ACME_EMAIL` | `acme_email` |
| `--public-ip` (o la detectada, §3.3 paso 5) | `MUELLE_INIT_PUBLIC_IP` | `public_ip` |
| `--public-ipv6` (opcional) | `MUELLE_INIT_PUBLIC_IPV6` | `public_ipv6` |
| `--http-port` | `MUELLE_INIT_PROXY_HTTP_PORT` | `proxy_http_port` |
| `--https-port` | `MUELLE_INIT_PROXY_HTTPS_PORT` | `proxy_https_port` |
| `--cloudflare-token` | El secreto `muelle_init_cloudflare_token` y `MUELLE_INIT_CLOUDFLARE_TOKEN_FILE=/run/secrets/muelle_init_cloudflare_token` | `cloudflare_token` (cifrado) |
| `--caddy-image` (avanzado) | `MUELLE_INIT_CADDY_IMAGE` | `caddy_image` |
| `--advertise-addr` | — (solo `docker swarm init`) | — |

**Fijas:**
- `MUELLE_LISTEN=:8080`
- `MUELLE_DATA_DIR=/data`
- `MUELLE_MASTER_KEY_FILE=/run/secrets/muelle_master_key`
- `MUELLE_SECURE_COOKIES` sin definir (`true`)

**El token:** nunca va en una variable de entorno (se vería en `docker inspect`), sino en un secreto Swarm. Lo único que se pasa es la ruta del archivo. Después del primer arranque, el secreto no se vuelve a leer como ajuste: cambiar o quitar el token en el panel es definitivo. install puede quitar el secreto del servicio cuando muelle ya lo guardó (`settings.init` en la auditoría, o `cloudflare_token_set`), pero no es obligatorio. Si lo quita, quita también `MUELLE_INIT_CLOUDFLARE_TOKEN_FILE` en el mismo `docker service update`: `settings.Load` lee el archivo en cada arranque, y si falta, muelle no arranca.

### 5.3 El stack de muelle

install lo despliega con `DeployStack`. No pasa por el render de apps: puede montar `docker.sock`, que las apps no pueden.

```yaml
services:
  muelle:
    image: ghcr.io/cpu-commits/muelle:v0.1.0
    environment: { MUELLE_PANEL_ORIGIN: …, MUELLE_INIT_…: … }
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - data:/data
      - caddy_admin:/run/caddy-admin
    secrets: [muelle_master_key]          # + muelle_init_cloudflare_token si hay
    networks: [muelle_panel]
    deploy:
      placement: { constraints: [node.role == manager] }
volumes:
  data: { name: muelle_data }
  caddy_admin: { name: muelle_caddy_admin }
secrets:
  muelle_master_key: { external: true }
networks:
  muelle_panel: { external: true }
```

### 5.4 Pasos de install que dependen de este contrato

- **Puertos ocupados (§3.3 paso 2):** install corre dentro de un contenedor, así que no puede probar el puerto con `net.Listen`.
  - Crea un contenedor temporal (`muelle-install-probe`, con la imagen de muelle y el comando `help`) que publica los puertos elegidos, y lo arranca.
  - `docker.ErrPortInUse` es "ocupado": se aborta con la sugerencia de `--http-port`/`--https-port`.
  - El contenedor temporal se borra siempre.
- **Clave maestra:** `crypto.GenerateKey()`, que se guarda con `CreateSecret(naming.MasterKeySecret, …)` y se imprime **una sola vez**. Si el secreto ya existe (`ErrConflict`), install no lo pisa: aborta, o sigue si es una reinstalación explícita.
- **El link de setup (§3.3 paso 8):**
  - install sigue los logs del servicio (`ServiceLogs` con `Follow`) hasta la línea JSON con `"event":"setup_url"` y lee su `url`. La escribe `logSetupURL` en `main.go`; el mensaje puede cambiar, `event` y `url` no.
  - El plazo es de 2 min.
  - Si muelle ya tiene usuarios, esa línea no sale e install lo dice.
- **Actualizar:** `docker service update --image ghcr.io/cpu-commits/muelle:<v> muelle_muelle`.
  - Con `caddy_image` vacío, el job `"proxy"` del arranque recrea muelle-caddy con la imagen de la versión nueva, con unos segundos de corte.
  - Con `caddy_image` fijo, Caddy no cambia.
- **Imágenes:**
  - `Dockerfile` en la raíz: el binario estático (sin CGO), el CLI de `docker` para `stack deploy` y el usuario root (necesita `docker.sock`).
  - `deploy/caddy/Dockerfile`: `xcaddy` con `github.com/caddy-dns/cloudflare`, en versiones fijas.
- **Cookie:** considerar `__Host-muelle_session` con cookies `Secure` (README, notas de la Fase 5).

## 6. Migraciones reservadas

Mientras no haya una versión publicada, `0002_deploy.sql` y `0003_ingress.sql` se pueden editar. Las fases no agregan migraciones nuevas: cambian estas, con su test de esquema.

- **0002** (deploy):
  - `deploys`: `actor_name`, `superseded_by` y `config_images` además de las columnas de §5.1, con índices por app y por estado activo;
  - `registries`: con `created_at` y `updated_at`.
- **0003** (ingress): `domains`, con:
  - `cf_record_id_aaaa`, `last_error_code`, `created_at` y `updated_at`;
  - `app_id` con `ON DELETE RESTRICT`: catalog no borra una app con dominios, `409 not_empty`.

## 7. Diferencias con lo que supuso el panel

| Tema | El panel supuso | Quedó | Por qué |
|---|---|---|---|
| Estado y logs | Fase 2 | Fase 2 (§17 del diseño decía 3) | Mismo cliente de Docker que el deploy |
| `actor_name` | Null si el actor ya no existe | Siempre string: se copia al crear el deploy | No se pierde al borrar el usuario o el token |
| Deploy | — | Campos nuevos `actor_id` y `superseded_by` | La Action informa el deploy que lo reemplazó (§14) |
| `images` en cola | Todos los servicios, sin digest | Solo los overrides del cuerpo; todos con digest desde el paso 2 | La configuración se lee al correr, no al encolar |
| `POST …/deploys` | — | La respuesta suma `status: "queued"` y el header `Location` | Compatible |
| `chunk` del log | Puede cortar una línea | Siempre líneas completas | Más simple; el manejo del panel sigue sirviendo |
| Log por polling | — | Suma `superseded_by` | Para la Action |
| SSE del deploy | `status` al conectar | Solo si no terminó. El `status` final va siempre después de la última línea | Si no, el panel cerraría antes de leer el log de un deploy que terminó justo |
| SSE de logs | — | Suma el evento `end`, y `follow=0` | El panel ignora eventos desconocidos |
| `GET …/status` | — | Suma `image` y `update_state`; `cpu_percent` y `memory_bytes` son la suma de las tareas del nodo local | Compatible |
| Rollback sin anterior | `409 no_previous_deploy` (o `conflict`) | `409 no_previous_deploy` | Igual |
| Rutas de un dominio | Bajo la app | Bajo la app | Igual |
| Hostname repetido | `409 conflict` o `hostname_taken` | `409 conflict` | El código genérico ya existe |
| Dominio | — | Suma `updated_at`; `records` es calculado; `cf_record_id_aaaa` no se expone | Compatible |
| `proxied` al crear | El panel manda `true` siempre | Opcional, por defecto `false` | Igual al default de la tabla |
| Adoptar | Pone el `proxied` del dominio | Conserva el `proxied` del registro y lo copia al dominio | §16: adoptar no cambia hacia dónde va el tráfico |
| `DELETE` de un dominio con Cloudflare caído | — | `502 cloudflare_unavailable` y no borra nada | No deja registros huérfanos |
| `target_service` en una app compose | Validado | Lo valida el render, no al guardar | ingress no parsea compose en la v1 |
| `GET /proxy` | No existe | Endpoint nuevo para el banner "proxy desincronizado" | §8 y §13 |
| Settings: `cloudflare_zones` | Abierto (¿consulta en cada `GET`?) | Valor guardado al guardar el token, que actualiza el job `"proxy"` | Un `GET` no depende de Cloudflare |
| Settings: `""` en `public_ip` | El mock lo rechaza | `""` borra `public_ip`, `public_ipv6` y `acme_email` | Hay que poder quitar un valor |
| Settings: quitar el token | Abierto (el mock rechaza `""`) | `cloudflare_token: ""` lo borra | |
| Settings: token rechazado o Cloudflare caído | — | `422 cloudflare_token` / `502 cloudflare_unavailable` | El panel tiene que mostrar el 502 como reintentable |
| Settings: lista que te deja fuera | Guarda sugerida | `409 would_lock_out`; se guarda con `confirm_lockout: true` | La pide la revisión del panel |
| Settings: cambio de puertos | Abierto | El `PATCH` espera a que se recree Caddy (hasta 2 min) y no falla; el resultado va en `GET /proxy` | |
| Settings: `client_ip` | String o null | Siempre string | Compatible |
| Registries: `PATCH` | `{username, password}` | Los dos obligatorios: reemplaza | Igual al payload del panel |
| Registries: host | Normalizado por el panel | El backend exige la forma canónica y no normaliza; Docker Hub es `docker.io` | |
| Registries: respuesta | `{host, username}` | Suma `created_at` y `updated_at` | Compatible |
| Auditoría: `actor_name` | Lo pide en `GET /audit` | Sigue pendiente (README, revisión final de la Fase 1); no es de este contrato | |
