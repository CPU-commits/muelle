# Módulo `deploy` (Fase 2): contrato que el panel supone

El backend de la Fase 2 todavía no existe. Este módulo (Resumen, Deploys, Logs y las
columnas de Proyectos) está hecho contra el spec (`docs/superpowers/specs/2026-10-09-muelle-design.md`,
§5.1 tabla `deploys`, §5.2, §7 y §10), la migración `0002_deploy.sql` de `fase-23-base` y
las suposiciones de este archivo. **Lo que dice aquí es lo que el panel lee y manda hoy**:
si la Fase 2 decide otra cosa, hay que cambiar este README, `data/dtos/deployDto.ts` y los
mocks (`src/mocks/{db,handlers}/deploy.ts`).

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

Se decide por el `code`, no por el status:

- `404 no_route` (la ruta no existe) y `405 method_not_allowed` (existe con otro método) →
  `UnavailableError` con `phase: 2`, y la pantalla muestra `<ComingSoon phase={2}>`.
- `404 not_found` es un recurso que no existe (la app, el deploy, el servicio): en
  `GET {app}/deploys/{id}` y `…/log` es `DeployNotFound` ("Ese deploy ya no existe").
- `503` (Docker no responde) es `UnavailableError` **sin** fase: un error de verdad, con
  reintento.

## Endpoints

### `GET {app}/status` → 200

El estado real en Swarm (spec §10 "Estado"). Lo pide el Resumen cada 5 s (solo con la
pestaña visible), Logs (para la lista de servicios) y Proyectos (una vez por fila).

```json
{
	"services": [
		{
			"name": "api",
			"replicas_desired": 2,
			"replicas_running": 2,
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
					"error": null,
					"cpu_percent": 1.7,
					"memory_bytes": 162529280
				},
				{
					"id": "p8w2n4c1d0e9f8g7h6i5j4k3l",
					"slot": 2,
					"state": "running",
					"desired_state": "running",
					"health": "healthy",
					"node": "worker-1",
					"since": "2026-10-10T14:20:07Z",
					"error": null,
					"cpu_percent": null,
					"memory_bytes": null
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
| `tasks[].cpu_percent?`                 | number ≥ 0                           | **(supuesto)** CPU de **esa** tarea, en % de un núcleo (como `docker stats`). **Null si no se midió**: solo se miden las tareas del nodo donde corre muelle.               |
| `tasks[].memory_bytes?`                | number ≥ 0                           | **(supuesto)** RAM de esa tarea. Null si no se midió (mismo criterio).                                                                                                     |

Lo que el panel calcula con las tareas **vigentes y medidas** de cada servicio: la CPU
sumada, la memoria **promedio por réplica** ("155 MB c/u · límite 512 MB") y, si no se
midieron todas, "1 de 2 réplicas medidas (solo las del nodo de muelle)". Sin ninguna
medida, "—".

**(supuesto)** El backend guarda las medidas al menos **5 s** en cache: `docker stats` es
caro y cada panel abierto en el Resumen pide `status` cada 5 s.

Estado de la app que deriva el panel: `running` (todo completo), `degraded` (falta alguna
réplica), `down` (hay deseadas y no corre ninguna), `stopped` (todo en 0 a propósito),
`not_deployed` (sin servicios).

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
		"finished_at": "2026-10-10T14:20:41Z",
		"superseded_by": null
	}
]
```

| Campo                         | Tipo                                                         | Qué                                                                                                                                                                           |
| ----------------------------- | ------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `id`                          | int > 0                                                      | El autoincremental (`#143`).                                                                                                                                                  |
| `status`                      | `queued` · `running` · `succeeded` · `failed` · `superseded` | Spec §5.1. Finales: `succeeded`, `failed`, `superseded`.                                                                                                                      |
| `images`                      | objeto `servicio → ref`                                      | Spec §5.1. Con digest (`ref@sha256:…`) una vez resuelto; **(supuesto)** sin `@…` mientras está en cola, y `{}` si todavía no hay nada.                                        |
| `trigger`                     | `panel` · `api` · `rollback`                                 | Spec §5.1.                                                                                                                                                                    |
| `actor_type`                  | `user` · `token`                                             | Spec §5.1.                                                                                                                                                                    |
| `actor_name`                  | string                                                       | Una **copia** del username o del nombre del token al crear el deploy (`actor_name TEXT NOT NULL DEFAULT ''`): sigue aunque el actor se borre. **Nunca null**; puede ser `""`. |
| `error?`                      | string                                                       | Por qué falló, en una línea (el panel la muestra arriba del log).                                                                                                             |
| `created_at`                  | RFC 3339                                                     |                                                                                                                                                                               |
| `started_at?`, `finished_at?` | RFC 3339                                                     | Null mientras no empezó / no terminó. La duración que muestra el panel es `finished_at − started_at`.                                                                         |
| `superseded_by?`              | int > 0                                                      | En un `superseded`: el deploy que lo reemplazó. El historial dice "Reemplazado por #145"; sin él, "Reemplazado".                                                              |

El deploy "actual" es el `succeeded` más reciente; el rollback va al `succeeded` anterior a
ese (spec §5.2 regla 6). El panel lo calcula del historial para mostrar "Rollback a #140".

### `GET {app}/deploys/{id}` → 200

Un deploy, con la misma forma que un elemento del historial. `404 not_found` →
`DeployNotFound`. El panel lo pide cuando el `?deploy=` de la URL no está entre los 20 del
historial: si existe lo abre; si no, avisa "El deploy #N ya no existe" y abre el más
reciente.

### `POST {app}/deploys` → 202

"Desplegar". Cuerpo (spec §7), **uno** de:

| Cuerpo                         | Cuándo                                                                                                                               |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------ |
| `{}`                           | Sin override: redespliega las refs actuales y vuelve a resolver los digests. **(supuesto)** El panel manda `{}`, no un cuerpo vacío. |
| `{"image": "ghcr.io/…:sha-…"}` | App de formulario (un solo servicio, que se llama como la app).                                                                      |
| `{"images": {"web": "…"}}`     | App compose: solo los servicios que cambian.                                                                                         |

El panel elige `image` o `images` con el `kind` del catálogo (lo conecta F2) o, sin él, con
los servicios del estado: un único servicio con el nombre de la app = formulario.

Antes de mandar valida cada imagen con **la misma regla que el backend**
(`validate.ImageRef` → `reference.ParseNormalizedNamed` de distribution/reference v0.6.0,
portada en `domain/value-objects/imageRef.ts` y probada contra el código Go con
`imageRefOracle.json`): no vacía y de hasta 512 bytes, sin `-` al principio ni `$`; el
repositorio en minúsculas con separadores `.`, `_`, `__` o guiones; registro opcional con
puerto; tag `[\w][\w.-]{0,127}`; digest `sha256|sha384|sha512` en hex minúscula con su
largo. `Ghcr.io/Foo/Bar:Latest`, `a//b`, `a::b`, `a:b:c` y `a/../b` no pasan; `a` y
`nginx:1.0-` sí. El mock rechaza lo mismo.

Respuesta (spec §7 paso 1): `{"id": 145, "status_url": "/api/v1/…/deploys/145"}`. El panel
usa `id` y va a Deploys, que abre el más reciente (ese); `status_url` es opcional.

Errores: `422 invalid` con `"image: <motivo>"` o `"images.<servicio>: <motivo>"` (el panel
lo marca en ese campo); `400 invalid_json` si el cuerpo no es el esperado.

Un deploy en cola que recibe otro encima queda `superseded`, con `superseded_by` (spec §7
paso 1).

### `POST {app}/rollback` → 202

Sin cuerpo. Misma respuesta que `POST {app}/deploys`; el deploy nuevo tiene
`trigger: "rollback"`.

Sin un `succeeded` anterior al actual: **409**. **(supuesto)** con
`code: "no_previous_deploy"`; mientras tanto el panel también acepta el `conflict` genérico
(hay un TODO para sacarlo cuando exista el código propio). Se muestra "No hay un deploy
exitoso anterior al actual" en el diálogo.

### `GET {app}/deploys/{id}/log?offset=N` → 200

El polling de la API (spec §7, §10), y el respaldo del panel cuando el SSE falla.

```json
{
	"chunk": "2026-10-10T14:32:07Z resolviendo imágenes\n…",
	"next_offset": 1234,
	"status": "running"
}
```

| Campo         | Qué                                                                                                                                                                                                         |
| ------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `chunk`       | El log desde `offset` (puede ser `""`). **(supuesto)** Puede cortar una línea por la mitad (el panel guarda lo que quede después del último `\n` hasta el próximo chunk), pero **nunca un carácter UTF-8**. |
| `next_offset` | El `offset` para la próxima llamada. Es opaco para el panel (**(supuesto)** bytes del log guardado, siempre en el borde de un carácter): nunca lo calcula, solo devuelve lo que recibió.                    |
| `status`      | El estado del deploy en este momento.                                                                                                                                                                       |

El panel pide de nuevo enseguida si vino un chunk, y cada 1 s si no. Termina cuando
`status` es final y `chunk` viene vacío. `offset` ausente = 0; uno más allá del final **no
es error**: `chunk: ""` y `next_offset` = el final del log. `422 invalid` con
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

: ping
```

- `log`: **una línea del log por evento** en `data` (sin el `\n`), con el secreto ya
  enmascarado (`***`). **Todo** `log` lleva `id` = el `next_offset` después de esa línea,
  con la misma semántica que el polling. Si el SSE se corta, el panel sigue por polling
  desde el último `id` recibido, sin repetir líneas.
- `status`: el estado (`queued`, `running`, …) en `data`, cada vez que cambia (y al
  conectar). Las líneas de un paso van **antes** del `status` que lo cierra.
- Un comentario `: ping` cada ~15 s mientras no haya otra cosa que mandar, para que un
  proxy no corte la conexión por inactividad. El panel lo ignora.
- Cuando el deploy queda en un estado final, el servidor manda ese `status` y cierra. El
  panel también cierra al recibirlo.
- Query `?offset=N` opcional para empezar desde ahí; un reconectar de EventSource manda
  además `Last-Event-ID`.
- Un `error` del EventSource (no abrió, se cortó, `data` que no se entiende) → el panel
  cierra y sigue por polling. No reconecta solo.

El panel pinta las líneas que llegan juntas en un solo frame, y el autoscroll sigue al
final aunque lleguen muchas por segundo: solo se pausa cuando quien lee sube (rueda,
teclado o arrastrando la barra).

### `GET {app}/logs?service=<s>&tail=<n>&follow=1` → `text/event-stream`

Los logs en vivo de un servicio (spec §10 "Logs"; no se guarda nada).

| Query     | Qué                                                                                                                                                                                                                                                                              |
| --------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `service` | El nombre corto del servicio (el de `status`).                                                                                                                                                                                                                                   |
| `tail`    | Líneas viejas antes de seguir: el panel manda 100, 200 (por defecto) o 500. Entero de 0 a **5000** (`MAX_LOG_TAIL`); fuera de eso, `422 invalid` con `"tail: …"`.                                                                                                                |
| `since`   | **(supuesto)** Al reconectar, en vez de `tail`: la hora (RFC 3339 con milisegundos) de la última línea que recibió el panel. Manda todo lo escrito desde ahí, **inclusive** (como `since` de Docker), hasta las últimas 5000. `422 invalid` con `"since: …"` si no es una fecha. |
| `follow`  | Siempre `1` desde el panel.                                                                                                                                                                                                                                                      |

**(supuesto)** Eventos `log`, una línea por evento, con `data` en JSON, y el mismo
`: ping` cada ~15 s que el log de un deploy:

```
event: log
data: {"time":"2026-10-10T14:31:40.118Z","task":"api.2","stream":"stdout","line":"WARN consulta lenta · 812 ms"}
```

| Campo    | Qué                                                                  |
| -------- | -------------------------------------------------------------------- |
| `time?`  | El timestamp de Docker (con `since` es lo que manda el panel).       |
| `task?`  | La tarea que la escribió (`api.2`): de ahí sale el filtro de tareas. |
| `stream` | `stdout` o `stderr` (por defecto `stdout`).                          |
| `line`   | La línea tal cual la escribió la app.                                |

Un `data` que no es ese JSON se muestra igual, como texto. El panel saca los escapes ANSI
(colores de terminal) y **nunca recorta la línea**: el nivel solo da color, y sale de un
token de nivel claro (`ERROR`/`WARN` en mayúsculas al principio, `[warn]`, `level=error` de
logfmt o `level`/`severity` de un JSON). Una palabra común ("error al conectar") no cuenta.

`404 not_found` si el servicio no existe en la app.

Lo que hace el panel con este stream:

- **Se corta** (estaba abierto): avisa "Se cortó la conexión con los logs" y **reconecta
  solo** con espera creciente (1, 2, 4, 8, 16 y 30 s de ahí en más; vuelve a 1 s después
  de un stream que anduvo 10 s), con `since` = la hora de la última línea. Lo de ese mismo
  instante que ya tenía se descarta (por tarea, stream y texto). "Reconectar ahora" no
  espera.
- **No abre**: dice "No se pudo conectar a los logs" (no "se cortó") y reintenta igual.
  Como un EventSource no deja ver el status, el panel pregunta `GET {app}/status`: si es
  **401**, la sesión venció, el cliente HTTP manda al login y no se reintenta más.
- **Pestaña oculta más de 30 s**: cierra el stream y lo retoma con `since` al volver.
  Mientras tanto lo que llega no crece sin tope (se quedan las últimas 5000 líneas).

## Lo que el panel no manda nunca

- Variables ni secretos (eso es `secrets`). El log ya llega enmascarado.
- `actor_*`: lo pone el backend según la sesión o el token.

## Superficie pública (barrel)

- Pantallas: `SummaryScreen` (Resumen; props `project`, `stage`, `app` y, del catálogo,
  `kind`, `pendingChanges`, `pendingChangesHref` y `healthcheck` como texto),
  `DeploysScreen` (`deployParam` = el `?deploy=` de la URL, y `kind`) y `LogsScreen`.
- Para Proyectos (F2): `AppDeployState`, una clase que se crea en la fila:
  `new AppDeployState(() => ref, () => enabled)` (`enabled` opcional: false no pide nada
  y todo dice "—"). Da `tone`, `statusText`, `lastDeployText`, `lastDeployFailed`,
  `image` e `imageShort`; "—" si la Fase 2 no está o falla, sin reintentos.
- Queries: `appStatusQuery(ref, options)`, `deploysQuery(ref, options)` y
  `lastDeployQuery(ref, options)` (comparte el cache de `deploysQuery`), con
  `options = { refreshEvery?, enabled?, noRetry? }`; `deployKeys` para invalidar.
- Tipos y helpers: `AppRef`, `AppHealth`, `AppKind`, `AppStatus`, `Deploy`, `DeployId`,
  `DeployStatus`, `ServiceStatus`, `appHealth`, `currentDeploy`, `latestDeploy`,
  `replicaCount`.

## Mocks (`pnpm dev:mock`)

Los deploys se calculan con la hora (sin timers), así el polling y el SSE ven lo mismo y
un F5 no los pierde. La primera vez que se abre una app se siembra el historial del
diseño (#139–#143) y un #144 que pasa `queued → running → succeeded` en ~10 s.

- "Desplegar" con una imagen que contenga `fail` o `broken` termina `failed` por
  healthcheck (con la salida de la tarea y un `***`); con `missing`, por imagen inexistente.
- Desplegar dos veces seguidas deja el primero `superseded` (con `superseded_by`).
- Apps `nats` y `redis`: un solo deploy (el rollback responde 409). Apps que empiezan con
  `nueva`/`new` o `agents`: nunca desplegadas.
- Las réplicas del slot 2 corren en `worker-1`: su CPU y memoria vienen en null.
- Los logs en vivo son una línea cada ~1,1 s, la misma en cada stream para la misma hora:
  `since` devuelve exactamente lo escrito desde ahí. Incluyen colores ANSI y logfmt.
