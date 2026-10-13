# Módulo `deploy` (Fase 2): contrato que el panel supone

El backend de la Fase 2 todavía no existe. Este módulo (Resumen, Deploys, Logs y las
columnas de Proyectos) está hecho contra el spec (`docs/superpowers/specs/2026-10-09-muelle-design.md`,
§5.1 tabla `deploys`, §5.2, §7 y §10) más las suposiciones de este archivo. **Lo que dice
acá es lo que el panel lee y manda hoy**: si la Fase 2 decide otra cosa, hay que cambiar
este README, `data/dtos/deployDto.ts` y los mocks (`src/mocks/{db,handlers}/deploy.ts`).

Lo que el spec no fija y se supuso está marcado con **(supuesto)**.

## Convenciones

- Base `/api/v1`, JSON, snake_case, fechas RFC 3339 con zona (`2026-10-10T14:32:07Z`).
- Errores con el sobre de siempre: `{"error": {"code": "…", "message": "…"}}`.
- Todo con sesión (cookie `muelle_session`); el spec abre estado, deploys, logs y rollback
  también a tokens de API dentro de su alcance, pero el panel usa la cookie.
- `{app}` abajo es `/projects/{p}/stages/{s}/apps/{a}`.
- Los campos marcados `?` pueden faltar o venir `null`: el panel los trata igual. Un campo
  que el panel no conoce se ignora.

## Detección de "todavía no existe"

Contra un backend sin la Fase 2, todas estas rutas responden `404 {"error":{"code":"no_route"}}`
(ruta desconocida) o `405 method_not_allowed`. El repositorio lo traduce a
`UnavailableError` con `phase: 2` y la pantalla muestra `<ComingSoon phase={2}>`. Un
**`404 not_found`** en cambio es "el recurso no existe" (la app o el deploy): en
`GET {app}/deploys/{id}` y `…/log` es `DeployNotFound`.

> Mientras `shared/data/http/domainErrors` siga leyendo el status (404/405) en vez del
> código, un `not_found` en las rutas de la app (status, deploys, logs) también se ve como
> "fase pendiente". Las rutas de un deploy ya mapean `not_found` por código.

## Endpoints

### `GET {app}/status` → 200

El estado real en Swarm (spec §10 "Estado"). Lo pide el Resumen cada 5 s (solo con la
pestaña visible), Logs (para la lista de servicios) y Proyectos.

```json
{
	"services": [
		{
			"name": "api",
			"replicas_desired": 2,
			"replicas_running": 2,
			"cpu_percent": 3.4,
			"memory_bytes": 622854144,
			"memory_limit_bytes": 536870912,
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

| Campo                                  | Tipo                                 | Qué                                                                                                                                                                        |
| -------------------------------------- | ------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `services`                             | array                                | Un elemento por servicio del stack. **Vacío = la app nunca se desplegó** (el panel muestra "Sin desplegar").                                                               |
| `name`                                 | string                               | El servicio del compose; en una app de formulario, el slug de la app. **(supuesto)**: el nombre corto, no `proyecto_stage_app_servicio` (el panel arma ese con los slugs). |
| `replicas_desired`, `replicas_running` | int ≥ 0                              | Réplicas deseadas y corriendo.                                                                                                                                             |
| `cpu_percent?`                         | number ≥ 0                           | **(supuesto)** CPU de **todas** las tareas del servicio sumada, en % de un núcleo (como `docker stats`).                                                                   |
| `memory_bytes?`                        | number ≥ 0                           | **(supuesto)** RAM de **todas** las tareas corriendo sumada. El Resumen muestra el promedio por réplica (`memory_bytes / replicas_running`).                               |
| `memory_limit_bytes?`                  | number ≥ 0                           | **(supuesto)** El límite de **cada** tarea (`resources.memory_mb`); null si no tiene.                                                                                      |
| `tasks`                                | array                                | Las tareas vigentes y las históricas que guarde Swarm.                                                                                                                     |
| `tasks[].id`                           | string                               | Id de la tarea (el panel muestra los primeros 7).                                                                                                                          |
| `tasks[].slot?`                        | int                                  | El slot de la réplica (`api.1`); null en un servicio global.                                                                                                               |
| `tasks[].state`                        | string                               | El estado de Swarm tal cual (`running`, `starting`, `shutdown`, `failed`, `rejected`, `complete`…).                                                                        |
| `tasks[].desired_state`                | string                               | `running` en las vigentes; otra cosa (`shutdown`) en las históricas, que el panel muestra atenuadas.                                                                       |
| `tasks[].health?`                      | `healthy` · `unhealthy` · `starting` | Del healthcheck; null si no tiene.                                                                                                                                         |
| `tasks[].node?`                        | string                               | El hostname del nodo.                                                                                                                                                      |
| `tasks[].since?`                       | RFC 3339                             | Desde cuándo está en `state`.                                                                                                                                              |
| `tasks[].error?`                       | string                               | `Status.Err` de la tarea ("unhealthy container").                                                                                                                          |

Estado de la app que deriva el panel: `running` (todo completo), `degraded` (falta alguna
réplica), `down` (hay deseadas y no corre ninguna), `stopped` (todo en 0 a propósito),
`not_deployed` (sin servicios). Docker caído: **503** → "No se pudo contactar a muelle"
con reintento (no es la fase).

### `GET {app}/deploys` → 200

El historial (los últimos 20, spec §5.2 regla 7). **(supuesto)** Un array plano (como
`GET /projects`), del más nuevo al más viejo; igual el panel ordena por `id`.

```json
[
	{
		"id": 143,
		"status": "succeeded",
		"images": { "api": "ghcr.io/aumi-app/backend-app:sha-3f9c2e1@sha256:9b2de1f4…" },
		"trigger": "api",
		"actor_type": "token",
		"actor_name": "github-actions",
		"error": null,
		"created_at": "2026-10-10T14:19:49Z",
		"started_at": "2026-10-10T14:19:49Z",
		"finished_at": "2026-10-10T14:20:41Z"
	}
]
```

| Campo                         | Tipo                                                         | Qué                                                                                                                                         |
| ----------------------------- | ------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------- |
| `id`                          | int > 0                                                      | El autoincremental (`#143`).                                                                                                                |
| `status`                      | `queued` · `running` · `succeeded` · `failed` · `superseded` | Spec §5.1. Finales: `succeeded`, `failed`, `superseded`.                                                                                    |
| `images`                      | objeto `servicio → ref`                                      | Spec §5.1. Con digest (`ref@sha256:…`) una vez resuelto; **(supuesto)** sin `@…` mientras está en cola, y `{}` si todavía no hay nada.      |
| `trigger`                     | `panel` · `api` · `rollback`                                 | Spec §5.1.                                                                                                                                  |
| `actor_type`                  | `user` · `token`                                             | Spec §5.1.                                                                                                                                  |
| `actor_name?`                 | string                                                       | **(supuesto)** En vez de `actor_id`: el username o el nombre del token, para mostrar "API · github-actions". Null si el actor ya no existe. |
| `error?`                      | string                                                       | Por qué falló, en una línea (el panel la muestra arriba del log).                                                                           |
| `created_at`                  | RFC 3339                                                     |                                                                                                                                             |
| `started_at?`, `finished_at?` | RFC 3339                                                     | Null mientras no empezó / no terminó. La duración que muestra el panel es `finished_at − started_at`.                                       |

El deploy "actual" es el `succeeded` más reciente; el rollback va al `succeeded` anterior a
ese (spec §5.2 regla 6). El panel lo calcula del historial para mostrar "Rollback a #140".

### `GET {app}/deploys/{id}` → 200

Un deploy, con la misma forma que un elemento del historial. `404 not_found` →
`DeployNotFound`.

### `POST {app}/deploys` → 202

"Desplegar". Cuerpo (spec §7), **uno** de:

| Cuerpo                         | Cuándo                                                                                                                               |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------ |
| `{}`                           | Sin override: redespliega las refs actuales y vuelve a resolver los digests. **(supuesto)** El panel manda `{}`, no un cuerpo vacío. |
| `{"image": "ghcr.io/…:sha-…"}` | App de formulario (un solo servicio, que se llama como la app).                                                                      |
| `{"images": {"web": "…"}}`     | App compose: solo los servicios que cambian.                                                                                         |

El panel elige `image` o `images` con el `kind` del catálogo (lo conecta F2) o, sin él, con
los servicios del estado: un único servicio con el nombre de la app = formulario.
Valida antes de mandar: sin espacios, empieza con letra o número, solo `[a-z0-9._-/:]`
y un `@sha256:<64 hex>` opcional.

Respuesta (spec §7 paso 1): `{"id": 145, "status_url": "/api/v1/…/deploys/145"}`. El panel
usa `id` (y lleva a `Deploys?deploy=145`); `status_url` es opcional.

Errores: `422 invalid` con `"image: <motivo>"` o `"images.<servicio>: <motivo>"` (el panel
lo marca en ese campo); `400 invalid_json` si el cuerpo no es el esperado.

Un deploy en cola que recibe otro encima queda `superseded` (spec §7 paso 1).

### `POST {app}/rollback` → 202

Sin cuerpo. Misma respuesta que `POST {app}/deploys`; el deploy nuevo tiene
`trigger: "rollback"`.

Sin un `succeeded` anterior al actual: **409**. **(supuesto)** con
`code: "no_previous_deploy"`; el panel también acepta el `conflict` genérico. Se muestra
"No hay un deploy exitoso anterior al actual" en el diálogo.

### `GET {app}/deploys/{id}/log?offset=N` → 200

El polling de la API (spec §7, §10), y el respaldo del panel cuando el SSE falla.

```json
{
	"chunk": "2026-10-10T14:32:07Z resolviendo imágenes\n…",
	"next_offset": 1234,
	"status": "running"
}
```

| Campo         | Qué                                                                                                                                                                      |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `chunk`       | El log desde `offset` (puede ser `""`). **(supuesto)** Puede cortar una línea por la mitad: el panel guarda lo que quede después del último `\n` hasta el próximo chunk. |
| `next_offset` | El `offset` para la próxima llamada. Es opaco para el panel (**(supuesto)** bytes del log guardado): nunca lo calcula, solo devuelve lo que recibió.                     |
| `status`      | El estado del deploy en este momento.                                                                                                                                    |

El panel pide de nuevo enseguida si vino un chunk, y cada 1 s si no. Termina cuando
`status` es final y `chunk` viene vacío. `offset` ausente = 0. `422 invalid` con
`"offset: …"` si no es un entero ≥ 0.

### `GET {app}/deploys/{id}/log/stream` → `text/event-stream`

El log en vivo (spec §7 "El panel lo recibe por SSE"). Lo abre el panel solo para un
deploy `queued` o `running`; uno terminado se lee entero por `?offset`.

**(supuesto)** Eventos:

```
event: log
id: 1234
data: 2026-10-10T14:32:07Z resolviendo imágenes

event: status
data: running
```

- `log`: **una línea del log por evento** en `data` (sin el `\n`), con el secreto ya
  enmascarado (`***`). `id` = el `next_offset` después de esa línea, con la misma
  semántica que el polling. Si el SSE se corta, el panel sigue por polling desde el
  último `id` recibido, sin repetir líneas.
- `status`: el estado (`queued`, `running`, …) en `data`, cada vez que cambia (y al
  conectar). Las líneas de un paso van **antes** del `status` que lo cierra.
- Cuando el deploy queda en un estado final, el servidor manda ese `status` y cierra. El
  panel también cierra al recibirlo.
- Query `?offset=N` opcional para empezar desde ahí; un reconectar de EventSource manda
  además `Last-Event-ID`.
- Un `error` del EventSource (no abrió, se cortó, `data` que no se entiende) → el panel
  cierra y sigue por polling. No reconecta solo.

### `GET {app}/logs?service=<s>&tail=<n>&follow=1` → `text/event-stream`

Los logs en vivo de un servicio (spec §10 "Logs"; no se guarda nada).

| Query     | Qué                                                                                                                |
| --------- | ------------------------------------------------------------------------------------------------------------------ |
| `service` | El nombre corto del servicio (el de `status`).                                                                     |
| `tail`    | Líneas viejas antes de seguir: el panel manda 100, 200 (por defecto) o 500, y `0` al reconectar (para no repetir). |
| `follow`  | Siempre `1` desde el panel.                                                                                        |

**(supuesto)** Eventos `log`, una línea por evento, con `data` en JSON:

```
event: log
data: {"time":"2026-10-10T14:31:40.118Z","task":"api.2","stream":"stdout","line":"WARN consulta lenta · 812 ms"}
```

| Campo    | Qué                                                                  |
| -------- | -------------------------------------------------------------------- |
| `time?`  | El timestamp de Docker.                                              |
| `task?`  | La tarea que la escribió (`api.2`): de ahí sale el filtro de tareas. |
| `stream` | `stdout` o `stderr` (por defecto `stdout`).                          |
| `line`   | La línea tal cual la escribió la app.                                |

Un `data` que no es ese JSON se muestra igual, como texto. El nivel (`info`/`warn`/
`error`) lo deduce el panel del texto: JSON con `level`/`msg`, `level=` de logfmt o un
prefijo `WARN …`/`[error] …`.

`404 not_found` si el servicio no existe en la app. Un corte → el panel muestra "Se cortó
la conexión" con "Reconectar" (que pide `tail=0`).

## Lo que el panel no manda nunca

- Variables ni secretos (eso es `secrets`). El log ya llega enmascarado.
- `actor_*`: lo pone el backend según la sesión o el token.

## Superficie pública (barrel)

- Pantallas: `SummaryScreen` (Resumen; props `project`, `stage`, `app` y, del catálogo,
  `kind`, `pendingChanges`, `pendingChangesHref`), `DeploysScreen` (`deployParam` = el
  `?deploy=` de la URL, y `kind`) y `LogsScreen`.
- Para Proyectos (F2): `AppDeployState` (una clase que se crea en la fila: `tone`,
  `statusText`, `lastDeployText`, `lastDeployFailed`, `image`, `imageShort`; "—" si la
  Fase 2 no está o falla) y las queries `appStatusQuery(ref)` y `lastDeployQuery(ref)`
  (comparte el cache de `deploysQuery`).
- Tipos y helpers: `AppRef`, `AppHealth`, `AppKind`, `AppStatus`, `Deploy`, `DeployId`,
  `DeployStatus`, `ServiceStatus`, `appHealth`, `currentDeploy`, `latestDeploy`,
  `replicaCount`, `deployKeys`.

## Mocks (`pnpm dev:mock`)

Los deploys se calculan con la hora (sin timers), así el polling y el SSE ven lo mismo y
un F5 no los pierde. La primera vez que se abre una app se siembra el historial del
diseño (#139–#143) y un #144 que pasa `queued → running → succeeded` en ~10 s.

- "Desplegar" con una imagen que contenga `fail` o `broken` termina `failed` por
  healthcheck (con la salida de la tarea y un `***`); con `missing`, por imagen inexistente.
- Desplegar dos veces seguidas deja el primero `superseded`.
- Apps `nats` y `redis`: un solo deploy (el rollback responde 409). Apps que empiezan con
  `nueva`/`new` o `agents`: nunca desplegadas.
