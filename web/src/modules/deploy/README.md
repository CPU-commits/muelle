# Módulo `deploy` (Fase 2): el contrato que usa el panel

La fuente es el contrato HTTP de las Fases 2 y 3:
`docs/superpowers/specs/2026-10-10-contrato-http-fases-2-3.md` (§0 y §2), en su versión de
`fase-2-deploy` (la Fase 2 terminada; se mergea en `fase-23-base`).
Este archivo resume lo que el panel lee y manda, y cómo se comporta con cada respuesta. Si
el contrato cambia, se cambian este README, `data/dtos/deployDto.ts` y los mocks
(`src/mocks/{db,handlers}/deploy.ts`). Lo que el panel usa y el contrato todavía no tiene
está en [Diferencias con el contrato](#diferencias-con-el-contrato).

## Convenciones

- Base `/api/v1`, JSON en snake_case, fechas RFC 3339 en UTC con a lo más milisegundos.
- Errores: `{"error": {"code": "…", "message": "…"}}`. El panel decide por el `code`; el
  `message` de un `422` empieza con `"<campo>: "` y el panel lo marca en ese campo.
- Acceso: sesión o token dentro de su alcance; el panel usa la cookie de sesión.
- `{app}` abajo es `/projects/{p}/stages/{s}/apps/{a}`. Los campos marcados `?` pueden
  venir `null`; un campo que el panel no conoce se ignora.

## Errores

| Respuesta                                   | Qué ve el panel                                                                      |
| ------------------------------------------- | ------------------------------------------------------------------------------------ |
| `404 no_route`, `405 method_not_allowed`    | La Fase 2 todavía no está: `<ComingSoon phase={2}>` (`UnavailableError` con `phase`) |
| `404 not_found`                             | El recurso no existe. En un deploy, `DeployNotFound`: "Ese deploy ya no existe"      |
| `503 docker_unavailable`                    | `DockerUnavailable`: "Docker no responde en el servidor de muelle…", con reintento   |
| `409 no_previous_deploy` (rollback)         | `NoPreviousDeploy`: "No hay un deploy exitoso anterior al actual", en el diálogo     |
| `409 conflict` (deploy o rollback)          | `AppRemoving`: "La app se está borrando…" (un `DELETE` en curso, a lo más 2 min)     |
| `422 invalid` en `image` / `images.<s>`     | `ImageRejected`, con su motivo y su texto en el campo (ver `POST {app}/deploys`)     |
| `422 invalid` en `offset` (polling del log) | El log se vuelve a leer desde 0, una vez (ver `GET …/log`)                           |
| `401`                                       | El cliente HTTP manda al login                                                       |

## `GET {app}/status` → 200

El estado real en Swarm. Lo pide el Resumen cada 5 s con la pestaña visible, Logs (para la
lista de servicios) y Proyectos (una vez por fila). **El backend puede guardar la respuesta
en caché hasta 2 s por app.**

```json
{
	"services": [
		{
			"name": "api",
			"image": "ghcr.io/aumi-app/backend-app:sha-3f9c2e1@sha256:9b2de1f4…",
			"replicas_desired": 2,
			"replicas_running": 2,
			"cpu_percent": 1.7,
			"memory_bytes": 162529280,
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
					"error": null,
					"cpu_percent": 1.7,
					"memory_bytes": 162529280
				},
				{
					"id": "p8w2n4c1d0e9f8g7h6i5j4k3l",
					"slot": 2,
					"state": "running",
					"desired_state": "running",
					"health": null,
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

| Campo                                  | Qué lee el panel                                                                                                                |
| -------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| `services`                             | Ordenados por `name`. **`[]` = la app nunca se desplegó** (o su stack se borró): "Sin desplegar"                                |
| `name`                                 | El nombre corto del servicio (`api`)                                                                                            |
| `image?`                               | La imagen de sus tareas. Proyectos la muestra si no tiene el historial de deploys                                               |
| `replicas_desired`, `replicas_running` | Réplicas deseadas y corriendo                                                                                                   |
| `cpu_percent?`, `memory_bytes?`        | Las sumas de las tareas medidas. **Son para otros clientes: el panel lee las de cada tarea**                                    |
| `memory_limit_bytes?`                  | El límite de **cada** tarea                                                                                                     |
| `update_state?`                        | `Service.UpdateStatus.State`, o null si nunca se actualizó                                                                      |
| `tasks`                                | Primero las vigentes (`desired_state: running`), después por `slot` y de la más nueva a la más vieja. Las históricas, atenuadas |
| `tasks[].health?`                      | `healthy` · `unhealthy` · `starting`; null si no tiene healthcheck, no corre en el nodo de muelle o es histórica                |
| `tasks[].node?`                        | El hostname del nodo; null mientras la tarea no tiene nodo (`pending`), y el id del nodo si ya no está: "—" sin él              |
| `tasks[].since?`                       | `Task.UpdatedAt`, truncada al milisegundo; null si Swarm no la da                                                               |
| `tasks[].error?`                       | `Task.Err`, **enmascarado** como el log (puede traer un argumento de `command`): el panel marca los `***` como el log           |
| `tasks[].cpu_percent?`                 | % de un núcleo, como `docker stats`. **Null si la tarea no corre en el nodo de muelle**, no corre o no se pudo medir            |
| `tasks[].memory_bytes?`                | La RAM de su contenedor; null con el mismo criterio                                                                             |

Con las tareas **vigentes y medidas** el panel calcula la CPU sumada, la memoria **promedio
por réplica** ("155 MB c/u · límite 512 MB") y, si no se midieron todas, "1 de 2 réplicas
medidas (solo las del nodo de muelle)". Sin ninguna medida, "—".

Estado de la app que deriva el panel: `running`, `degraded` (falta alguna réplica), `down`
(hay deseadas y no corre ninguna), `stopped` (todo en 0 a propósito), `not_deployed`.

## El deploy

`GET {app}/deploys` → 200: un array plano, del más nuevo al más viejo, con lo que guarda la
retención (**hasta 22**: los 20 más nuevos, más el actual y su anterior si son más viejos, y
los que están en cola o corriendo). `GET {app}/deploys/{id}` → 200: uno; `404 not_found`
si no es de esa app.

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

| Campo                         | Qué                                                                                                                                                         |
| ----------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `status`                      | `queued` · `running` · `succeeded` · `failed` · `superseded` (los tres últimos, finales)                                                                    |
| `images`                      | **En cola:** los overrides del cuerpo (`{}` si no hubo); en un rollback, las del deploy al que vuelve. **Desde que se resolvieron:** todos, con `@sha256:…` |
| `actor_name`                  | Una copia del username o del nombre del token al crear el deploy: **nunca null** (puede ser `""`)                                                           |
| `actor_id?`                   | El id del usuario o del token. El panel no lo usa                                                                                                           |
| `error?`                      | Por qué falló, en una línea ya enmascarada (el panel la muestra arriba del log)                                                                             |
| `superseded_by?`              | El deploy que lo dejó `superseded`: el historial dice "Reemplazado por #145"                                                                                |
| `started_at?`, `finished_at?` | La duración que muestra el panel es `finished_at − started_at`. Un `superseded` tiene `finished_at` y no `started_at`                                       |

El deploy "actual" es el `succeeded` más reciente; el rollback va al `succeeded` anterior.
Un `?deploy=N` que no está en el historial se pide aparte con `GET …/deploys/{id}`: si
existe se abre, y si no, el panel avisa "El deploy #N ya no existe" y abre el más reciente.
Cambiar el `?deploy=` sin salir de la ruta abre el nuevo.

## `POST {app}/deploys` → 202

"Desplegar". El panel manda **uno** de:

| Cuerpo                         | Cuándo                                                                                                       |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------ |
| `{}`                           | Sin override: repite la ref **pedida** del deploy actual (un tag se vuelve a resolver; un digest queda fijo) |
| `{"image": "<ref>"}`           | App de formulario: la imagen de su único servicio                                                            |
| `{"images": {"<s>": "<ref>"}}` | App compose: solo los servicios que cambian                                                                  |

Elige `image` o `images` con el `kind` del catálogo (Resumen y Deploys lo piden con
`appQuery` del barrel de `catalog`) o, si el catálogo no carga, con los servicios del estado
(un único servicio con el nombre de la app = formulario). Los campos del diálogo son los
`services` de la configuración (§2.12), contra los que el backend valida `images`, aunque la
app no se haya desplegado; sin ellos, los servicios de Swarm (`deployableServices`).

Antes de mandar valida cada imagen con **la misma regla que el backend**
(`validate.ImageRef` → `reference.ParseNormalizedNamed` de distribution/reference v0.6.0,
portada en `domain/value-objects/imageRef.ts` y probada contra el código Go con
`imageRefOracle.json`).

Respuesta: `{"id": 145, "status": "queued", "status_url": "…/deploys/145"}` (y el header
`Location`). El panel va a Deploys, que abre el más reciente (ese).

Los `422` de una imagen, cada uno con su texto en el campo del servicio:

| Mensaje del backend                                  | Motivo            | Texto del panel                                                           |
| ---------------------------------------------------- | ----------------- | ------------------------------------------------------------------------- |
| `image: manda image o images, no los dos`            | `both`            | "Va una imagen para la app o una por servicio, no las dos."               |
| `image: una app compose recibe images, por servicio` | `compose`         | "Esta app es compose: la imagen va por servicio."                         |
| `images.<s>: el servicio no existe en la app`        | `unknown_service` | "El servicio <s> ya no existe en la configuración de la app."             |
| `image: …` / `images.<s>: …` (la ref no pasa)        | `invalid`         | "No es una referencia de imagen válida…" (también lo que no se reconozca) |

Al encolar, los deploys `queued` de la misma app pasan a `superseded` con `superseded_by`.

**Qué imagen sale sin override** (contrato §2.2): la ref **pedida** del deploy actual, tal
cual. Un tag se vuelve a resolver y un digest queda fijo, así que **un rollback no se
deshace con el próximo deploy sin cuerpo**: repite sus refs con digest, y el que sigue
también. Por eso el diálogo de "Desplegar" avisa qué sigue fijado por el último rollback
(`rollbackPin` de `domain/entities/deploy.ts`) y que para volver a seguir un tag hay que
escribir la imagen (o cambiarla en la configuración). El backend no expone la ref pedida
(`requested_images`), así que el panel la deduce: un servicio sigue fijado si en el deploy
actual tiene la misma imagen (ref y digest) que en el último rollback `succeeded`. Se
equivoca solo si un override pide el mismo tag y resuelve al mismo digest (avisa de más), o
si el rollback ya salió del historial (no avisa).

`409 conflict`: la app se está borrando (el `DELETE` de la app frena sus deploys hasta 2
min). El diálogo lo dice en su banner. El orden de los errores es `404` (la app), `400` (el
cuerpo) y `422` (su validación).

## `POST {app}/rollback` → 202

Sin cuerpo. Responde como `POST …/deploys`, con `trigger: "rollback"`. Sin un `succeeded`
anterior al actual: `409 no_previous_deploy`; con la app borrándose, `409 conflict`
(`AppRemoving`, otro texto). Las refs del destino, con digest, pasan a ser los overrides del
rollback: el próximo deploy sin cuerpo las repite.

## `GET {app}/deploys/{id}/log?offset=N` → 200

El polling: lo usa el panel para un deploy terminado y como respaldo cuando el SSE falla.

```json
{
	"chunk": "2026-10-10T14:32:07.123Z resolviendo imágenes\n",
	"next_offset": 1234,
	"status": "running",
	"superseded_by": null
}
```

- El log es `<RFC 3339 con ms> <texto>\n` por línea (la hora siempre con tres decimales,
  y el texto nunca trae `\r`), ya enmascarado, y solo crece. Los offsets son **bytes**
  desde el principio; el panel nunca los calcula, solo devuelve los que recibió.
- Una línea se corta a 8 KiB con `…`. Al llegar a 256 KB se agrega una vez
  `… (log truncado: se superaron los 256 KB)` y se descartan las siguientes, salvo las de
  cierre (el resultado y el error), que siempre se escriben. `log_truncated` queda en la
  base, pero no viene en la respuesta: el panel ve la línea.
- `chunk` va desde `offset`, **hasta 64 KiB y siempre en líneas completas**; puede ser `""`.
  `next_offset = offset + len(chunk)`.
- `superseded_by` se repite acá.
- `offset` ausente es 0. Uno que no es entero ≥ 0, o que **pasa el largo del log**:
  `422 invalid` con `offset: …`. El panel solo usa offsets del servidor, así que no
  debería pasar; si pasa, vuelve a leer el log desde 0 (una vez) y descarta lo que tenía.
- El panel pide de nuevo enseguida si vino un chunk, y cada 1 s si no. Termina cuando
  `status` es final y `chunk` viene vacío.

## `GET {app}/deploys/{id}/log/stream` → `text/event-stream`

El log en vivo. El panel lo abre solo para un deploy `queued` o `running`.

```
event: status
data: running

event: log
id: 1234
data: 2026-10-10T14:32:07.123Z resolviendo imágenes

: ping

event: status
data: succeeded
```

- Empieza en el `Last-Event-ID`, o en `?offset=N`, o en 0.
- `log`: una línea por evento, sin el `\n`; `id` es el `next_offset` después de esa línea.
- `status`: **al conectar solo si el deploy no terminó**, y cada vez que cambia. El final va
  **siempre después de la última línea**, y el servidor cierra; el panel también deja de
  leer al recibirlo.
- `: ping` cada 15 s (el panel lo ignora).
- Un stream dura a lo más 1 h; si cierra sin el `status` final, o se corta, o no abre, el
  panel sigue por polling desde el último `id`, sin repetir líneas.

El panel pinta las líneas que llegan juntas en un solo frame. Mientras sigue el final, cada
cambio de alto del contenido (una línea larga que se parte, una que se pinta recién al
llegar al final por `content-visibility`) vuelve a bajar: la última línea queda a la vista.
Solo deja de seguir cuando quien lee sube (rueda, teclado o la barra).

## `GET {app}/logs?service=<s>&tail=<n>&follow=1` → `text/event-stream`

Los logs en vivo de un servicio (no se guarda nada).

| Query     | Qué                                                                                                                                                                       |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `service` | El nombre corto. **Obligatorio**: sin él, `422 invalid` con `service: es obligatorio`. Si no existe en Swarm, `404 not_found`                                             |
| `tail`    | De 0 a 5000 (200 por defecto). El panel manda 100, 200 o 500                                                                                                              |
| `follow`  | Siempre `1` desde el panel (vacío o `0` manda las líneas, un `end` y cierra; otro valor es `422 follow`)                                                                  |
| `since`   | Al reconectar, **en vez de `tail`**: la hora de la última línea (RFC 3339 con ms). Todo lo escrito desde ahí, inclusive, hasta 5000. Con los dos, el backend mira `since` |

El orden de los errores: la app (`404`), la query (`422`, en el orden de la tabla) y el
servicio en Swarm (`404`, `503`).

```
event: log
data: {"time":"2026-10-10T14:31:40.118Z","task":"api.2","stream":"stdout","line":"WARN consulta lenta · 812 ms"}

event: end
data: eof
```

- `log`: JSON compacto. `task` es `<servicio>.<slot>` (o los primeros 7 del id de la tarea
  en un servicio global); `line` va sin el `\n`, cortada a 16 KiB con `…` y enmascarada. El
  backend une antes los pedazos de una línea larga; `time` y `task` vienen null solo en el
  pedazo que llegó sin su línea: se muestra igual, sin hora, y no cuenta para `since`. Un
  `data` que no es ese JSON se muestra igual, como texto.
- `end`: se terminó el log. `data: eof` es que el servicio se borró: el panel dice "El
  servicio dejó de existir", no reconecta solo y ofrece "Volver a conectar".
  `data: timeout` es que el stream llegó a su hora de vida (1 h): el panel vuelve a
  conectar enseguida, sin aviso.
- `: ping` cada 15 s.

El panel saca los escapes ANSI y **nunca recorta la línea**: el nivel solo da color, y sale
de un token claro (`ERROR`/`WARN` en mayúsculas al principio, `[warn]`, `level=error` de
logfmt o `level`/`severity` de un JSON).

**Cortes y reconexión:**

- **Se corta** (estaba abierto): "Se cortó la conexión con los logs", y reconecta solo con
  espera creciente (1, 2, 4, 8, 16 y 30 s; vuelve a 1 s después de un stream que anduvo
  10 s). "Reconectar ahora" no espera.
- **No abre**: "No se pudo conectar a los logs" (no "se cortó"), y reintenta igual. Un
  EventSource no deja ver el status: el panel pregunta `GET {app}/status`, y si es **401**
  el cliente HTTP manda al login y no se reintenta. Un `429` (más de 8 streams abiertos por
  la misma sesión, contando los de deploys) tampoco se ve: espera como cualquier otro.
- **Pestaña oculta más de 30 s**: cierra el stream y lo retoma al volver.
- **Al reconectar** manda `since` (sin `tail`), y descarta lo de antes de `since` y, de ese
  mismo milisegundo, tantas copias de cada línea como ya tenía. Un backend sin `since` (lo
  ignoraría y mandaría las últimas 200) tampoco haría repetir: quedaría un hueco.
- Si el log de Docker se corta por otra causa, el stream se cierra sin `end`: es un corte,
  y el panel reconecta con `since`.
- El backend vuelve a validar la sesión cada 60 s y cierra el stream si ya no vale: el
  panel lo ve como un corte; al reconectar el stream no abre, pregunta por la sesión y va
  al login.

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

Siguen el contrato (offsets en bytes, chunks de hasta 64 KiB, `status` al conectar solo si
no terminó, `: ping`, `end`, los `422` de la query de los logs en el orden del backend —
`service`, `tail`, `since`, `follow`— antes del `404` del servicio, la retención de 22). Los
deploys se calculan
con la hora, así el polling y el SSE ven lo mismo y un F5 no los pierde. La primera vez que
se abre una app se siembra el historial del diseño (#139–#143) y un #144 que pasa
`queued → running → succeeded` en ~10 s.

- "Desplegar" con una imagen que contenga `fail` o `broken` termina `failed` por
  healthcheck; con `missing`, por imagen inexistente.
- Desplegar dos veces seguidas deja el primero `superseded` (con `superseded_by`).
- Apps `nats` y `redis`: un solo deploy (el rollback responde `409 no_previous_deploy`).
  Apps que empiezan con `nueva`/`new` o `agents`: nunca desplegadas.
- Las réplicas del slot 2 corren en `worker-1`: su CPU y memoria vienen en null.
- Los logs en vivo son una línea cada ~1,1 s, la misma para la misma hora en cada stream:
  `since` devuelve exactamente lo escrito desde ahí. A los 15 min, `end` con `timeout`.

## Diferencias con el contrato

Ninguna: lo de arriba es lo que dice el contrato (`since` ya está en §2.9).
