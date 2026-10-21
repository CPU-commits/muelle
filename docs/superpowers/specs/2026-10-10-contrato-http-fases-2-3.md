# muelle — Contrato de las Fases 2, 3 y 5

> Fecha: 2026-10-13 · Revisado: 2026-10-14 (revisión de la etapa de contratos), 2026-10-17 (lo que pidió el panel, §7) y 2026-10-21 (los certificados de Cloudflare, §3.7) · Estado: aprobado para la etapa de contratos, e implementado en este árbol (Fases 2, 3 y 5). Los certificados de Cloudflare (§3.7 y lo que suman §1, §3.1–§3.5, §4, §6 y §7) están aprobados y todavía sin implementar.
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

Las peticiones con cookie que modifican algo exigen un `Origin` permitido, que se compara exacto (esquema, host y puerto); si no, `403 bad_origin`. Los tokens no lo necesitan. Hay dos permitidos (`cmd/http`, hecho):
- el origen en que muelle-caddy sirve el panel según los ajustes **de ese momento**: `https://<panel_domain>`, con `:<proxy_https_port>` si no es 443 (`ProxySettings.PanelOrigin`, que da `admin.Module.PanelOrigin`). `""` sin `panel_domain` (en desarrollo), y entonces no pasa ninguno por acá. Va en memoria: `PATCH /settings` lo invalida cuando cambia algún ajuste del proxy, así después de cambiar `proxy_https_port` (§4.1) el origen nuevo pasa desde que responde el `PATCH`, y el viejo ya no. Un cambio hecho fuera de muelle (el `UPDATE` a mano de §4.1) se ve en 30 s como mucho (`usecases.PanelOriginTTL`);
- `MUELLE_PANEL_ORIGIN`, si está: una variable opcional para un origen más, puesto a mano (detrás de otro proxy, o en desarrollo, con una IP o `localhost`). install no la escribe: escribe `MUELLE_PANEL_DOMAIN` (§5.2). Con ella, los ajustes solo se consultan cuando el `Origin` no es ese.

Los links de setup (`<origen>/setup?token=…`) y de las invitaciones (`POST /invites`, `url`) se arman con `MUELLE_PANEL_ORIGIN` si está y, si no, con el origen de los ajustes, en cada uno: siguen al puerto HTTPS igual. Un link relativo no abre nada, así que nunca sale uno: si no hay origen (sin `MUELLE_PANEL_ORIGIN`, los ajustes no se pudieron leer), `POST /invites` responde `500 internal` sin crear la invitación, y serve, sin usuarios, no arranca sin el link de setup (§5.4).

### 0.2 Códigos de error comunes

| Status | `code` | Cuándo |
|---|---|---|
| 400 | `invalid_json` | El cuerpo no es el esperado (campo desconocido, tipo equivocado, JSON roto) |
| 401 | `unauthenticated`, `mfa_required` | Sin sesión, o con el 2FA a medias |
| 403 | `forbidden`, `token_not_allowed`, `out_of_scope`, `bad_origin`, `totp_setup_required` | Ver §10.1 |
| 404 | `no_route` | **La ruta no existe.** El panel va embebido con el backend entero (§5.5), así que para él es una ruta mal armada: un error inesperado, como un `405`, que no reintenta |
| 404 | `not_found` | La ruta existe, pero el recurso no |
| 405 | `method_not_allowed` | La ruta existe con otro método (header `Allow`) |
| 409 | `conflict`, `not_empty` | Ya existe / primero hay que borrar lo que contiene |
| 409 | `network_conflict` | Al crear un stage (`POST /projects/{p}/stages`): ya hay una red de Docker con el nombre de la del stage (`<proyecto>_<stage>`, `naming.Network`) que no es overlay attachable, así que no es de muelle. No se usa ni se borra, y el stage no se guarda: hay que borrarla o renombrarla en el servidor (`ErrNotStageNetwork`, que registra ingress, §3.4) |
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
- **La credencial se revisa mientras dura:** cada 60 s (cada 4 heartbeats), el handler vuelve a validar la credencial con que se abrió, la cookie o el Bearer, con su alcance. Si ya no vale (la sesión se cerró o venció, el token se revocó o venció, o ya no alcanza a ese proyecto), cierra el stream sin mandar nada más. Para eso `utils.Kit` suma `Revalidate(r, access) error` (Fase 2), que repite las comprobaciones de `Handle` sin escribir la respuesta.
- **Límites:**
  - A lo más 8 streams abiertos a la vez por principal (la misma sesión o el mismo token), sumando los de deploys y los de logs: alcanza para el panel abierto en varias pestañas, y acota los streams de Docker que tienen abiertos los de logs. El noveno recibe `429 rate_limited`, antes de abrir.
  - Un stream dura a lo más 1 h. El de logs termina con `end` (`data: timeout`); el de un deploy cierra sin el `status` final, y el cliente sigue por polling o reconecta con `Last-Event-ID`.
  - El panel ya trata así un stream que se corta: deja de leer y decide (polling, reconectar, o preguntar por la sesión).

## 1. Quién es dueño de qué

| Endpoints | Módulo | Fase |
|---|---|---|
| `POST/GET {app}/deploys`, `GET {app}/deploys/{id}`, `…/log`, `…/log/stream`, `POST {app}/rollback` | `deploy` | 2 |
| `GET {app}/status`, `GET {app}/logs` | `deploy` (tiene el cliente de Docker de los servicios) | 2 |
| `/registries` | `deploy` (son las credenciales de las imágenes) | 2 |
| `GET/POST {app}/domains`, `PATCH/DELETE {app}/domains/{hostname}`, `POST …/adopt`, `POST …/retry` | `ingress` | 3 |
| `GET /proxy` | `ingress` | 3 |
| `GET/PATCH /settings` (claves del proxy y el DNS) | `admin`, con el aviso a `ingress` | ya está (etapa de contratos) |

> El estado y los logs en vivo eran de la Fase 3 en §17. Pasan a la Fase 2: usan el mismo cliente de Docker que el deploy.

`deploy.Module.RegisterRoutes` e `ingress.Module.RegisterRoutes` registran todas las rutas de §2 y §3. Solo una ruta que no existe responde `404 no_route` (§0.2).

### 1.1 Puertos entre módulos (ya en el código)

Los módulos no se importan (§4.1 regla 1): cada uno declara su interfaz en su `domain/service`, y `container.go` conecta los módulos por tipado estructural. Una firma distinta no compila.

| De → a | Puerto (dónde se declara) | Lo implementa | Para qué |
|---|---|---|---|
| catalog → ingress | `catalog/domain/service.StageNetworks`: `EnsureStageNetwork`, `RemoveStageNetwork` | `ingress.Module` (hecho) | Crear y borrar la red overlay del stage, y conectar o desconectar Caddy (§3.2). Al crear un stage, `EnsureStageNetwork` va después de lo que se responde sin Docker (`404` del proyecto, `422` del slug, `409 conflict` si el stage ya existe) y antes de guardarlo: Docker caído es `503 docker_unavailable`, y una red ajena con ese nombre, `409 network_conflict` (§0.2, §3.4); en los dos casos el stage no se guarda. Opcional en `catalog.Deps`: con `nil`, catalog no toca Docker |
| catalog → deploy | `catalog/domain/service.AppRuntime`: `RemoveApp(ctx, p, s, a)` | `deploy.Module` (hecho: `docker stack rm`) | Al borrar una app. Fase 2: antes del `stack rm`, deja de aceptar deploys de la app por 2 min (`409 conflict`, lo que tarda catalog en borrar la fila), cancela el deploy en curso (`failed`: "la app se borró durante el deploy") y marca `failed` los `queued`. Si el `stack rm` falla, vuelve a aceptar deploys enseguida. Si el deploy en curso no se detiene en 15 s, no sigue (ni `queued` ni `stack rm`): vuelve a aceptar deploys y el `DELETE` de la app responde `409 conflict` ("un deploy de la app todavía no se detiene"), sin borrarla; se reintenta. Opcional |
| deploy → secrets | `deploy/domain/service.EnvResolver`: `ResolveEnv(ctx, appID) (vars, secretValues, err)` | `secrets.Module` (hecho) | Las variables del stage con las de la app encima, y los valores secretos para enmascarar el log |
| deploy → ingress | `deploy/domain/service.Ingress`: `EnsureStageNetwork`, `SyncRoutes(ctx) error` | `ingress.Module` (hecho) | §7 pasos 4 y 7 |
| admin → ingress | `admin/domain/service.ProxyHook`: `CheckCloudflareToken`, `CheckCloudflareOriginCA(ctx, token, field)`, `CheckProxyPorts(ctx, old, next)`, `ProxySettingsChanged(ctx, old, next)` | `ingress.Module` (hecho, salvo `CheckCloudflareOriginCA`, §3.7) | Ver §3.5 |
| ingress → admin | `ingress.Deps.PortsRolledBack func()` (opcional) | `container.go`, con `admin.Module.InvalidatePanelOrigin` (hecho) | Al volver a los puertos anteriores del proxy, el origen del panel en caché se descarta (§3.5) |

**Lecturas por SQL** (un adaptador en `infra/out/service` del que lee, que puede leer la tabla de otro módulo):

| Módulo | Lee | Escribe | Adaptador |
|---|---|---|---|
| deploy | `apps` (catalog) | Solo `storage_node_id` (si está vacío) y `last_deployed_at` (solo hacia adelante) | `deploy/infra/out/service.NewCatalogApps` → `service.Apps` (hecho) |
| deploy | `domains` (ingress): los destinos, para validar el render (§6.3) | — | `NewDomainTargets` → `service.DomainTargets` (hecho) |
| deploy | `settings.manager_node_id` y `settings.init_registry_applied` | `manager_node_id` al arrancar; `init_registry_applied`, la marca del registry inicial de install (§2.10), en el primer arranque de la base | Fase 2 |
| ingress | `settings` (admin): `entity.ProxySettings`, con el token descifrado | `cloudflare_zones`, las claves reservadas de §3.6 y, solo al volver a los puertos anteriores (§3.5), `proxy_http_port` y `proxy_https_port` | `ingress/infra/out/service.NewProxySettingsReader` (hecho) |
| ingress | `apps`, `stages` y `projects` (catalog), para el kind de la app y los nombres de los servicios | — | `ingress/infra/out/service.NewCatalogReader` (hecho) |
| ingress | `registries` (deploy): la credencial del host de la imagen de Caddy, si hay una (las imágenes de muelle son privadas, §3.4) | — | `NewRegistriesReader` (hecho) |
| catalog | `domains`: cuántos tiene una app antes de borrarla | — | Hecho |

`deploy` lee `domains` por SQL y no llama a `ingress` para eso: así la Fase 2 se prueba sin la Fase 3.

### 1.2 Arranque y segundo plano

| Momento | Qué | Dónde |
|---|---|---|
| `container.New` | Migra, verifica la clave maestra y guarda los ajustes de arranque: `panel_domain` siempre, y `MUELLE_INIT_*` solo en el primer arranque de la base (§5.2) | `admin.ApplyStartupSettings` (hecho) |
| `container.New`, justo después | El registry inicial de install (`MUELLE_INIT_REGISTRY_FILE`), solo en el primer arranque de la base (§2.10) | `deploy.SeedInitRegistry` (Fase 2) |
| `main` → `cron.Run(bgCtx, c.Jobs)` | Corre cada job **al arrancar** y después según su intervalo | `cmd/cron` |
| `main` → `c.Run(bgCtx)` | `deploy.Run` e `ingress.Run` en paralelo; vuelve cuando los dos terminaron | `container.Run` (hecho) |

**`deploy.Run(ctx)`** (Fase 2) hace, en orden, la reconciliación de §7:
1. Guarda `manager_node_id` con `Docker.Info().NodeID` si falta. Con Docker caído, lo reintenta en el primer deploy.
2. Arranca un worker por cada app que tiene un deploy `running` o `queued`. Cada worker cierra primero el `running` que quedó del arranque anterior, con una línea en su log ("muelle se reinició durante el deploy"):
   - si ningún servicio de la app lleva la etiqueta `muelle.deploy` de ese deploy, `docker stack deploy` nunca llegó a Swarm: `failed` ("muelle se reinició antes de aplicar el deploy; Swarm no cambió");
   - si solo algunos la llevan, el CLI se cortó a mitad: `failed`;
   - si todos la llevan, sigue esperando la convergencia como en el paso 6 (con el `deploy_timeout_s` de la app, contado desde ahí) y lo cierra igual que un deploy normal;
   - con Docker caído, el deploy queda `running` y el worker reintenta cada 15 s. Los `queued` de esa app esperan detrás; las otras apps siguen.
3. Después, cada worker retoma el `queued` de su app.
4. Queda con un worker por app hasta que `ctx` se cancela. Un worker sin nada que hacer termina, y el próximo deploy de su app lo vuelve a crear.

Un `docker stack deploy` cortado por el apagado se resuelve en el próximo arranque, por el paso 2: el apagado nunca cierra un deploy, lo deja `running`.

**`ingress`** (Fase 3, hecho):
- El job `"proxy"`, cada 5 min y al arrancar, en este orden (con el proxy deshabilitado no hace nada, §3.4):
  - asegura las redes de todos los stages y conecta Caddy;
  - asegura muelle-caddy (§3.4) y la red `muelle_panel`, salvo que haya una recreación agendada por `ProxySettingsChanged`: esa la hace `ingress.Run`, después de que vuelve el hook (§3.5). El job nunca vuelve a otros puertos: si los de los ajustes no arrancan, reintenta con los mismos;
  - borra las redes con etiquetas de stage (`muelle.project`/`muelle.stage`) cuyo stage ya no existe. Solo las que se llaman `naming.Network` de esas etiquetas, y recién **la segunda vez seguida** que el job las ve huérfanas: catalog crea la red antes de guardar el stage, y así no se borra la de un stage que se está creando. Una red en uso queda para la próxima vez;
  - reenvía la configuración completa. Si acaba de crear muelle-caddy, espera a que su admin API responda;
  - con token, actualiza `cloudflare_zones` y el registro DNS del panel (§3.4, §3.6), y si cambió la zona del panel (y con ella su TLS), reenvía la configuración otra vez. Va al final: un Cloudflare lento no atrasa la configuración de los sitios.
  - con los certificados de Cloudflare encendidos (§3.7, todavía sin implementar), despierta su reconciliación, sin esperarla: lee el `proxied` de los registros, emite, renueva y revoca.
  - Un paso que falla no corta los que siguen.
- El job `"certificados"`, cada hora (§3.1).
- `ingress.Run` lleva los reintentos con backoff (1 s, 2 s… 60 s) de una configuración que Caddy no aceptó, las verificaciones de un dominio 30 s después de crearlo, adoptarlo o reintentarlo y la de los `pending` cada minuto (§3.1), la recreación de muelle-caddy que agenda `ProxySettingsChanged`, que vuelve a los puertos anteriores si los nuevos no arrancan (§3.5), y la reconciliación de los certificados de Cloudflare (§3.7, todavía sin implementar): una pasada a la vez, con un plazo de 5 min cada una.

Un error de Docker al arrancar nunca detiene a muelle.

### 1.3 Lo que comparten las ramas

Las tres fases corren en paralelo y se integran después. Cada una toca **solo** lo suyo en los lugares compartidos, y ahí solo agrega: no renombra, no cambia firmas, no borra.

| Lugar | Quién lo toca | Qué puede hacer |
|---|---|---|
| `src/internal/deploy`, `src/internal/ingress` y el paquete nuevo de install | Fase 2, Fase 3 y Fase 5 | Todo: es suyo |
| `shared/domain/compose` (nuevo) | Fase 2 | La parte pura del compose (parsear, la lista de §6.3 del diseño, los nombres de los servicios) |
| `catalog/domain/entity.ValidateCompose` y sus llamadas | Fase 2 | Cambiar la validación por la de `shared/domain/compose`, con los nombres de servicio únicos por stage, y sumar `services` a la respuesta de una app (§2.12). Nada más de catalog: los destinos de dominio no se verifican al guardar, solo en el render (§3.2 y §6.3 del diseño) |
| `shared/domain/entity` | Fase 3 | Agregar las constantes `panel_cf_*` (§3.6) |
| `shared/domain/naming`, `shared/domain/validate` | Las tres | Agregar |
| `shared/infra/out/docker` (`Engine` y `dockertest`) | Las tres | Agregar métodos, cada uno con su fake y sus tests |
| `cmd/http/utils` | Fase 2 | Agregar `Revalidate` (§0.3) |
| `shared/infra/container/container.go` | La fase cuyos `Deps` cambian | Solo las líneas de su módulo |
| `main.go` | Fase 5 | El subcomando `install` |
| `go.mod` / `go.sum` | Fase 2 (compose-go) y la que necesite otra | Agregar dependencias; al integrar, el conflicto se resuelve con `go mod tidy` |
| `0002_deploy.sql` / `0003_ingress.sql` | Fase 2 / Fase 3 | Lo de "Migraciones reservadas" (§6), sin tocar las columnas congeladas |
| README, "Notas para las fases siguientes" | Cada fase, en su bloque | Tachar lo que cumplió y agregar lo que deja |
| Los documentos de diseño y este contrato | Nadie, salvo para corregir un error | Un cambio de contrato se acuerda antes, no en una rama |

**Columnas congeladas:** otros módulos las leen por SQL (§1.1), así que no cambian de nombre, de tipo ni de sentido:
- `domains`: `app_id`, `hostname`, `target_service` y `target_port`. Las leen `deploy` (`DomainTargets`) y `catalog` (el conteo antes de borrar una app);
- `registries`: `id`, `host`, `username` y `password`, cifrada con la AAD `registries:password:<id>`. `ingress` puede leerlas para una `caddy_image` privada;
- `0001_init.sql` no se edita: sus tablas son de la Fase 1.

## 2. Deploys (Fase 2, módulo `deploy`)

### 2.1 El deploy

```json
{
  "id": 143,
  "status": "succeeded",
  "trigger": "api",
  "images": { "api": "ghcr.io/aumi-app/backend-app:sha-3f9c2e1@sha256:9b2de1f4…" },
  "requested_images": { "api": "ghcr.io/aumi-app/backend-app:sha-3f9c2e1" },
  "actor_type": "token",
  "actor_id": 4,
  "actor_name": "github-actions",
  "error": null,
  "superseded_by": null,
  "routes_warning": false,
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
| `images` | objeto `servicio → ref` | **Mientras está `queued`:** los overrides del cuerpo (`{}` si no hubo); en un rollback, las imágenes del deploy al que vuelve (§2.7). **Desde que se resolvieron (§7 paso 2):** todos los servicios, con `ref@sha256:…`. Si falló antes de resolver, quedan los overrides |
| `requested_images` | objeto `servicio → ref` | La ref **pedida** de cada servicio, la que se resolvió (`deploys.requested_images`, §2.2 "Qué imagen se despliega"). Tiene digest solo si alguien lo fijó: un override con digest, una ref de la configuración con digest o un rollback (§2.7), y un deploy sin cuerpo repite esa ref mientras no cambie la de la configuración. Así se sabe qué servicios deja fijos en un digest el deploy actual, también después de que el rollback que los fijó salió del historial (§2.3). `{}` hasta que se resolvieron las imágenes (§7 paso 2): mientras está `queued`, en un `superseded` y en uno que falló antes. Nunca null |
| `actor_type` | `user` · `token` | |
| `actor_id` | int o null | El id del usuario o del token |
| `actor_name` | string | El username o el nombre del token **al crear el deploy**: sigue aunque se borren. Nunca es null |
| `error` | string o null | Por qué falló, en una línea de hasta 500 caracteres, ya enmascarada. Null si no falló |
| `superseded_by` | int o null | El deploy que lo dejó `superseded` |
| `routes_warning` | bool | `true` si el paso 7 (§7 del diseño) no pudo actualizar las rutas de Caddy. La advertencia de §2.11 va al log si entra: una que no entra por el tope de §2.5 igual deja la marca. Un error de Caddy no hace fallar el deploy (§13 del diseño): la app corre, pero Caddy puede no tener sus rutas nuevas, y el motivo está en `GET /proxy` (`error`, §3.3, solo con sesión). Se marca en el paso 7, antes de escribir la línea: quien la ve en el log ya la encuentra en el deploy, y puede verse `true` con `status: running` hasta el cierre, que viene enseguida. El paso 7 guarda su resultado también cuando sí actualiza Caddy: un deploy que muelle retoma después de reiniciarse (§1.2) vuelve a correr el paso 7, y si esta vez Caddy se actualizó queda en `false`, aunque su log traiga la advertencia del intento anterior. `false` en los demás: mientras está `queued`, mientras corre antes del paso 7 y en uno que no llegó a él. Nunca null |
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
- `409 conflict`: la app se está borrando (`DELETE` de la app en curso; dura a lo más 2 min, ver `RemoveApp` en §1.1).

El orden: primero la app (`404`), después el cuerpo (`400`) y al final su validación (`422`).

**Al encolar:** los deploys `queued` de la misma app pasan a `superseded`, con `superseded_by` = el nuevo y `finished_at`. Solo sobrevive el último. Un deploy `running` no se toca: el nuevo espera.

No se llama a Docker al encolar: con Docker caído igual se responde `202`, y el deploy falla en el paso 2 con "Docker no responde".

**Auditoría:** `deploy.create`, target `app:<p>/<s>/<a>`, details `{"deploy": 145, "trigger": "api", "images": {…overrides}}`. Las refs no son secretas.

#### Qué imagen se despliega

Cada deploy guarda dos cosas por servicio, además de `images` (la ref resuelta):
- `deploys.requested_images`: la ref **pedida**, la que se resolvió. Tiene digest solo si alguien lo fijó a propósito: un override con digest, una ref de la configuración con digest, o un rollback (que pide las refs resueltas del deploy al que vuelve). Sale en el deploy como `requested_images` (§2.1);
- `deploys.config_images`: la ref de la configuración (`spec.image`, o el `image:` del compose ya interpolado) que leyó el deploy.

Para cada servicio, en este orden:
1. El override del cuerpo, si viene.
2. Si no, la ref **pedida** de ese servicio en el **deploy actual** (el `succeeded` más reciente), tal cual: un tag se vuelve a resolver y un digest queda fijo. Nunca se le quita un digest a una ref: así no se pierde uno fijado a propósito, y `ghcr.io/x/y@sha256:…` nunca pasa a `ghcr.io/x/y`, que Docker leería como `:latest`.

   Una excepción: si la ref de la configuración cambió desde ese deploy (es distinta de su `config_images`), se usa la de la configuración.
3. Si el servicio no estaba en el deploy actual (primer deploy, o un servicio nuevo del compose), la ref de la configuración.

Así:
- un deploy sin cuerpo después de un deploy de CI con `sha-…` mantiene esa imagen (y vuelve a resolver el tag), y uno de CI por digest (`@sha256:…`) mantiene ese digest;
- un digest fijado en la configuración (`spec.image: ghcr.io/x/y:1.2@sha256:…`) o en un override se mantiene en los deploys sin cuerpo que siguen;
- editar `spec.image` en el panel y desplegar sin override usa la imagen nueva;
- **un rollback no se deshace con el próximo deploy sin cuerpo:** el rollback pide las refs con digest, y el deploy sin cuerpo las repite. Para volver a seguir un tag que se mueve (`:staging`) después de un rollback, se despliega con override o se cambia la ref en la configuración.

Si una ref ya trae digest (`…@sha256:…`), la resolución (§7 paso 2 del diseño) solo verifica que exista y que haya acceso, y lo conserva.

**La ref resuelta** (`images`) es la pedida, con el tag explícito y el digest: `nginx` → `nginx:latest@sha256:…`, `ghcr.io/x/y:sha-1` → `ghcr.io/x/y:sha-1@sha256:…`. Una ref pedida con digest queda igual (`ghcr.io/x/y@sha256:…` no gana un `:latest`).

### 2.3 `GET {app}/deploys` → 200 · token

Un array plano con los deploys que guarda la retención, del más nuevo al más viejo (§5.2 regla 7): hasta 22. Cada elemento tiene la forma de §2.1.

**La retención** se aplica al cerrar cada deploy. Se quedan:
- los 20 más nuevos;
- siempre, aunque sean más viejos, el deploy actual (el `succeeded` más reciente) y el `succeeded` anterior, que es el destino del rollback (§2.7);
- los `queued` y `running`, que nunca se borran.

El resto se borra. Así, 20 deploys fallidos seguidos (un CI que empuja un build roto) no borran la versión que corre. Sin el deploy actual, un deploy sin cuerpo volvería a la ref de la configuración (§2.2, "Qué imagen se despliega", paso 2), por ejemplo `:staging` en vez del `sha-…` de CI, y el rollback respondería `409 no_previous_deploy`.

### 2.4 `GET {app}/deploys/{id}` → 200 · token

Un deploy (§2.1). `404 not_found` si no existe **en esa app**: un id de otra app también es `not_found`, aunque exista.

### 2.5 `GET {app}/deploys/{id}/log?offset=N` → 200 · token

El polling de la API (la GitHub Action, §14) y el respaldo del panel cuando el SSE falla.

```json
{ "chunk": "2026-10-10T14:32:07.123Z resolviendo imágenes\n", "next_offset": 1234, "status": "running", "superseded_by": null }
```

**El log:**
- Es un texto con una línea por evento: `<RFC 3339 con ms> <texto>\n`. La hora lleva siempre tres decimales (`14:32:07.120Z`), y el texto nunca trae `\r` (se quita): un `\r` cortaría el evento del SSE.
- Ya viene enmascarado, con las reglas de abajo.
- Solo crece: los offsets son bytes desde el principio y nunca se mueven.
- Una línea larga se corta a 8 KiB con `…` (la línea entera, sin el `\n`, mide a lo más 8192 bytes; el corte respeta UTF-8).
- **Tope de 256 KB** (262 144 bytes): las líneas normales entran mientras el log no pase de 256 KB − 4 KiB. La primera que no entra agrega una vez `… (log truncado: se superaron los 256 KB)`, y las siguientes se descartan. Los 4 KiB reservados son para las líneas de cierre (el resultado, el error), que siempre se escriben. La marca de que el log se truncó queda en la base (`deploys.log_truncated`), así sigue valiendo después de un reinicio.
- Cada línea se guarda en la base **antes** de transmitirse por SSE: lo que se vio por el stream ya está en `?offset`.

**El enmascarado** (el mismo en el SSE del deploy, §2.6, y en los logs de las apps, §2.9):
- Cada valor secreto de `ResolveEnv`, de la app y del stage, se reemplaza por `***`, del más largo al más corto.
- **Un valor de varias líneas** (una clave PEM) se enmascara entero y, además, línea por línea: el log se procesa por línea, así que el valor entero casi nunca aparece en una. Cada línea del valor, sin los espacios de los bordes, cuenta como otro valor a enmascarar.
- **Un valor, o una línea de un valor, de menos de 8 bytes no se enmascara.** `1`, `true` o `3000` taparían texto que no tiene nada que ver, y la GitHub Action guarda todo como secreto por defecto (`env-secret`, §14 del diseño). El panel puede advertirlo al guardar un secreto así.
- La línea se enmascara antes de cortarla (a 8 KiB acá, a 16 KiB en §2.9), así un corte no deja la mitad de un secreto a la vista.
- **Las líneas que parte el cliente de Docker:** `shared/infra/out/docker` entrega una línea de más de 16 KiB en pedazos, y los que siguen al primero llegan sin hora ni tarea (`LogLine.Time` cero y `TaskID` vacío). Antes de enmascarar, cada pedazo así se une al anterior del mismo stream, hasta 64 KiB (el resto se descarta), y recién después se corta. Vale para los logs de las tareas que entran al log del deploy (§2.11) y para §2.9. Un secreto que cruza el corte de 16 KiB que hace el propio Docker en su `json-file` (dos líneas con hora) no se detecta: es una limitación aceptada.

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

Si no hay deploy actual, o no hay un `succeeded` anterior: `409 no_previous_deploy`. Si la app se está borrando, `409 conflict` (§2.2).

**Las imágenes:**
- El destino se elige al encolar: las refs resueltas (`images`) de ese `succeeded` pasan a ser los overrides del rollback, y se ven en su `images` mientras está `queued` (§2.1).
- Al correr, un servicio del destino que ya no está en la configuración se omite, con una línea en el log. Un servicio de la configuración que no estaba en el destino sigue la regla de §2.2, como en un deploy sin cuerpo.
- Como pide refs con digest, el próximo deploy sin cuerpo repite esas imágenes (§2.2).

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
          "error": null,
          "cpu_percent": 1.7,
          "memory_bytes": 311427072
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
| `cpu_percent`, `memory_bytes` | La suma de `tasks[].cpu_percent` y de `tasks[].memory_bytes` que no son null. Null si no se midió ninguna |
| `memory_limit_bytes` | El límite de **cada** tarea (`Service.MemoryLimitBytes`); null si no tiene |
| `update_state` | `Service.Update.State`, o null si nunca se actualizó |

**Cada tarea:**

| Campo | Qué |
|---|---|
| `tasks` | Las vigentes y las históricas que guarda Swarm. Primero las que tienen `desired_state: running`, después por `slot` y por `since`, de la más nueva a la más vieja |
| `id` | El id de la tarea |
| `slot` | Null en un servicio global |
| `state`, `desired_state` | Los de Swarm, tal cual |
| `health` | `healthy`, `unhealthy` o `starting`, del contenedor en el nodo local (`InspectContainer`); null si no tiene healthcheck, no está en este nodo o es una tarea histórica (`desired_state` no es `running`) |
| `node` | El hostname del nodo (`Node`, con caché de 5 min). Null si la tarea todavía no tiene nodo (`pending`); el id del nodo si su hostname no se puede leer (el nodo ya no está) |
| `since` | `Task.UpdatedAt`, truncada al milisegundo (Docker la da con nanosegundos; §0). Null si Swarm no la da |
| `error` | `Task.Err`, enmascarado como el log (§2.5) con los valores secretos de la app y del stage: puede traer un argumento de `command` ya interpolado. Null si está vacío |
| `cpu_percent`, `memory_bytes` | Los de su contenedor (`ContainerStats`), si la tarea corre en el nodo local. `cpu_percent` es el % de un núcleo, como `docker stats`. Null si la tarea no está en el nodo local, no corre o no se pudo medir. Las muestras se toman en paralelo, con un plazo total de 2 s |

El panel lee los valores de cada tarea (suma el CPU y promedia la RAM de las que se midieron, para compararla con `memory_limit_bytes`, que es por tarea). Las sumas del servicio son para otros clientes.

**Caché.** El panel lo pide cada 5 s mientras la pestaña está visible, y la página de proyectos lo pide una vez por fila. Medir es caro (`ContainerStats` tarda cerca de 1 s por tarea), así que:
- las medidas (`cpu_percent` y `memory_bytes` de cada tarea) se guardan 5 s por app, como supuso el panel. Varias pestañas, o varios clientes, comparten la misma medición;
- el resto (servicios, réplicas, tareas, `health`) se puede guardar hasta 2 s por app, para que un deploy en curso no se vea atrasado;
- una tarea que no estaba en la medición guardada sale con las medidas en null hasta la próxima.

**Errores:** `404 not_found` si la app no existe; `503 docker_unavailable`.

### 2.9 `GET {app}/logs?service=<s>&tail=<n>|since=<t>&follow=1` → `text/event-stream` · token

Los logs de un servicio, en vivo. No se guarda nada (§10).

**La query:**

| Query | Qué |
|---|---|
| `service` | El nombre corto. Obligatorio (`422 invalid` con `service: es obligatorio`). Tiene que ser un slug de servicio, de hasta 16 caracteres (§5.2 regla 1 del diseño); si no, `422 invalid` con `service: …`. Si no existe en Swarm para la app: `404 not_found` (ver abajo) |
| `tail` | De 0 a 5000; 200 por defecto. Otro valor: `422 invalid` con `tail: …` |
| `since` | Una fecha RFC 3339 (el panel manda milisegundos: `2026-10-10T14:31:40.118Z`). Reemplaza a `tail`: si vienen los dos, manda `since` y `tail` no se mira. Se mandan las líneas escritas desde `since`, **inclusive** (la hora de Docker, con nanosegundos, ≥ `since`), hasta las últimas 5000 (`LogOptions{Since: since, Tail: 5000}`). Si no es una fecha: `422 invalid` con `since: …` |
| `follow` | `1` deja el stream abierto. Si no viene, viene vacío o es `0`, manda las líneas de `tail` (o de `since`), un `end` y cierra. Otro valor: `422 invalid` con `follow: …` |

El orden de los errores: primero la app (`404`), después la query (`422`, en el orden de la tabla) y al final el servicio en Swarm (`404`, `503`).

**Cómo se busca el servicio.** `InspectService` y `ServiceLogs` de Docker también aceptan un id o un prefijo de id, y este endpoint acepta tokens: el parámetro nunca llega tal cual a Docker.
1. Se valida como slug (arriba): sin `_`, no alcanza el servicio de otra app.
2. Se busca con `InspectService(naming.Service(p, s, a, service))`. Ese nombre lleva `_`, así que nunca es un id ni un prefijo de id (los de Swarm son `[a-z0-9]`).
3. El servicio tiene que llevar todas las etiquetas de `naming.AppLabels(p, s, a)`. Si no existe, o no las lleva (uno con ese nombre que no creó muelle): `404 not_found`.
4. `ServiceLogs` recibe el `ID` que devolvió la búsqueda.

**Los eventos:**

```
event: log
data: {"time":"2026-10-10T14:31:40.118Z","task":"api.2","stream":"stdout","line":"WARN consulta lenta · 812 ms"}

event: end
data: eof
```

- **`log`:** `data` es JSON compacto.
  - `time` es la hora de Docker, **truncada** al milisegundo (§0; nunca redondeada hacia arriba).
  - `task` es `<servicio>.<slot>`, o `<servicio>.<primeros 7 del id de la tarea>` en un servicio global. Una tarea que no estaba al abrir (un deploy durante un stream con `follow`) se busca una vez; si no aparece, sale con los 7 del id.
  - `time` y `task` son null solo en el pedazo de una línea larga que llegó sin la línea a la que pertenece (ver "Enmascarado").
  - `stream` es `stdout` o `stderr`.
  - `line` va sin el `\n` final, cortada a 16 KiB con `…` (el corte respeta UTF-8; un byte inválido sale como U+FFFD).
- **Enmascarado:** como el log del deploy (§2.5), con los valores secretos de la app y del stage, y con los pedazos de una línea larga unidos antes. Es una defensa extra: las apps a veces imprimen su configuración.
- **`end`:** se termina el log (el servicio se borró, o no había `follow`): `data: eof`. A la hora (§0.3): `data: timeout`. El servidor cierra después. Si el log de Docker se corta por otra causa, el stream se cierra sin `end`, y el panel reconecta con `since`.
- **Reconectar:** el panel vuelve a abrir el stream con `since` = el `time` de la última línea que recibió (`{service, since, follow: 1}`; si además manda `tail`, no se mira): al cortarse el stream, y al volver a una pestaña que estuvo oculta más de 30 s. Como `since` es inclusivo y `time` está truncado, vuelven a llegar las líneas de ese milisegundo, la última incluida, y el panel descarta las que ya tenía (por tarea, stream y texto). Así no se pierde ninguna: con `time` redondeado hacia arriba, o un `since` exclusivo, se perderían las de ese milisegundo que no alcanzaron a llegar. Sin `since`, cada reconexión repetiría las últimas 200.

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

**Los alias de Docker Hub** (`index.docker.io`, `registry-1.docker.io` y `registry.hub.docker.com`) tienen la forma canónica, pero una imagen de Docker Hub siempre da `docker.io` en `reference.Domain` (`index.docker.io/x` también), así que una credencial con ese host nunca se usaría, y los pulls irían anónimos sin aviso. Se rechazan con `422`: `host: para Docker Hub usa docker.io`. El panel puede convertirlos a `docker.io` al normalizar.

| Endpoint | Cuerpo | Respuesta | Errores |
|---|---|---|---|
| `GET /registries` | — | `200 [registry]`, ordenados por host | |
| `POST /registries` | `{host, username, password}` | `201 registry` | `409 conflict` si el host ya está; `422` en `host`, `username` (1–255 caracteres, UTF-8 válido, sin caracteres de control ni `:`, que la autenticación HTTP Basic usa de separador) o `password` (1–4096 bytes, sin saltos de línea) |
| `PATCH /registries/{host}` | `{username, password}`, los dos obligatorios: reemplaza las credenciales | `200 registry` | `404 not_found`, `422` |
| `DELETE /registries/{host}` | — | `204` | `404 not_found` |

- **La contraseña nunca vuelve.** Se cifra con la AAD `registries:password:<id>` (ya está en la lista de `shared/infra/out/crypto`).
- **Auditoría:** `registry.create`, `registry.update` y `registry.delete`, con target `registry:<host>`. En create y update, details `{"username": "…"}`.
- **Uso en el deploy:**
  - el paso 2 pasa la credencial del host de cada imagen a `DistributionDigest` (exacta por host; sin credencial es anónima);
  - el paso 5 pasa a `DeployStack` solo las credenciales de los hosts que usa el compose.
  - `docker.RegistryAuth` y `docker.StackDeploy` (que lleva el compose final, con los valores de las variables) se imprimen sin la contraseña ni el compose: `String`, `GoString`, `LogValue` y `MarshalJSON` los ocultan, así un `slog`, un `%v` o un `json.Marshal` no los filtran.
- Cambiar o borrar un registry no redespliega nada.
- **El registry inicial de install.** Las imágenes de muelle son privadas, así que install le pasa a muelle la credencial con que las bajó: un JSON `{"host", "username", "password"}` en un secreto Swarm, con su ruta en `MUELLE_INIT_REGISTRY_FILE` (la Fase 5 documenta el secreto y la variable en §5.2 y §5.3).
  - `settings.Load` lee y valida el archivo en cada arranque, como el del token de Cloudflare: JSON estricto (sin otros campos) y las reglas de esta sección para `host`, `username` y `password`. Si no sirve, muelle no arranca, y el error nombra la variable sin la contraseña ni el contenido del archivo.
  - deploy lo guarda como un registry (cifrado, con `registry.create`, actor `system` y details `{"username", "source": "install"}`) **solo en el primer arranque de la base**, como los demás `MUELLE_INIT_*` (§5.2). La marca es suya, `init_registry_applied` en `settings`, y se guarda en ese arranque haya o no archivo, en la misma transacción que el registry.
  - Por qué una marca: el secreto y la variable quedan en el servicio. Sin la marca, un registry que el admin borró en el panel (porque el token se filtró) volvería con el próximo reinicio. Una reinstalación sobre el mismo `muelle_data` no cambia las credenciales: se editan en el panel.
- **Limitación aceptada (v1): las credenciales son de todo muelle, no de un proyecto.** Un token con alcance al proyecto A puede mandar `{"image": "ghcr.io/<otra-org>/<privada>"}`: muelle la resuelve y la baja con la credencial de `ghcr.io`, y la corre en la app de A. Así, ese token llega a cualquier imagen que vea la credencial, también a las de otro proyecto.
  - Se acepta porque solo una sesión maneja las credenciales, y una sesión puede hacer todo (§10.1 del diseño). El token ya puede correr la imagen que quiera en sus apps; lo nuevo es que puede leer las imágenes privadas que ve la credencial.
  - Para acotarlo, la credencial debería ver solo las imágenes que despliega muelle (un token de solo lectura de esa organización). El panel puede decirlo al guardar un registry.

### 2.11 Lo que escribe el deploy en el log

No es un contrato de texto (los clientes deciden por `status`, y por `routes_warning` para la advertencia de Caddy), pero cada paso de §7 deja al menos una línea, en este orden:

```
… deploy #145 de auditfy/staging/api (api · github-actions)
… resolviendo imágenes
…   api: ghcr.io/aumi-app/backend-app:sha-3f9c2e1 → sha256:9b2de1f4a7c3…
… render: 1 servicio
… red auditfy_staging lista
… docker stack deploy auditfy_staging_api
<la salida del CLI, una línea por línea>
… esperando convergencia (hasta 300 s)
… api: 2/2 réplicas corriendo
… rutas de Caddy actualizadas            (o: advertencia: no se pudo actualizar Caddy; el motivo está en el estado del proxy (GET /api/v1/proxy))
… succeeded en 34 s                      (o: failed: <motivo>)
```

- La primera línea lleva el `trigger` y el `actor_name`. El digest se muestra con sus primeros 12 caracteres; el completo está en `images`.
- Leer la configuración (el spec, o el compose interpolado con las variables) va con "resolviendo imágenes", porque de ahí salen los servicios y sus refs. Un compose que no pasa la validación falla ahí con `failed: render: compose_yaml: …`.
- Con `rollback_*` o `paused` (§7 paso 6), el log recibe el `Status.Err` de la última tarea fallida y sus últimas 50 líneas (`TaskLogs`). Con el timeout, el estado de cada servicio que no llegó (`web: 0/2 réplicas corriendo`) y el `Status.Err` de su última tarea fallida, si hay.
  - "La última tarea fallida" es la que nombra Swarm en el mensaje de la actualización (`… early termination of task <id>`); si no nombra ninguna, la más nueva de **ese** deploy que falló o fue rechazada (una de antes, que Swarm ya reintentó, no cuenta).
  - Una tarea que no pasó el healthcheck termina `complete` y sin `Status.Err`: el log lo dice (`web: última tarea fallida: terminó en complete, sin un motivo de Swarm`) y trae igual sus líneas.
- Un error de `SyncRoutes` es solo una advertencia (§13). `SyncRoutes` es el `Sync` de ingress (§3.4): con el proxy deshabilitado no manda nada y no falla, así que la línea es `rutas de Caddy actualizadas`.
  - El deploy lleva la advertencia también como dato, `routes_warning: true` (§2.1): el panel la muestra por ese campo, no por el texto de la línea. Una línea que no entra en el log (el tope de 256 KB, §2.5) no le quita la marca.
  - La advertencia no trae el error. El log del deploy lo lee un token del proyecto, y el mensaje de Caddy puede nombrar los hosts y los upstreams de otros proyectos. El motivo queda en `GET /proxy` (`error`, §3.3, solo con sesión) y en el log del servidor.
- Un deploy que cerró la reconciliación (§1.2) suma "muelle se reinició durante el deploy" antes de su resultado.
- Todo lo que se escribe pasa por el enmascarado (§2.5), también la salida del CLI y los mensajes de error del render.

### 2.12 Los servicios de una app (catalog, Fase 2)

`GET {app}` y `GET …/apps` (catalog) suman `services`, para que el formulario de un dominio ofrezca los destinos (`target_service`, `target_port`, §3.2) sin esperar al primer deploy:

```json
{ "slug": "front", "kind": "compose", "…": "…", "services": [{ "name": "api", "ports": [3000] }, { "name": "web", "ports": [] }] }
```

| Campo | Qué |
|---|---|
| `services` | Siempre está, ordenado por `name`. App de formulario: un servicio con el slug de la app. App compose: los servicios del compose guardado (`shared/domain/compose.Services`). `[]` si el compose guardado no se puede leer |
| `name` | El nombre corto del servicio |
| `ports` | Los puertos que declara, ordenados: el `port` del spec en una app de formulario; los de `expose` en una app compose (un entero, o `N/tcp`; los rangos y los `${VAR}` se omiten). `[]` si no declara ninguno. No se publican: son solo una sugerencia para el destino |

Es solo lectura: viene de la configuración guardada, no de Swarm.

**Al guardar un compose** (`POST`/`PATCH` de la app), catalog lo valida con `shared/domain/compose` (§6.3 del diseño): `422 invalid` con `compose_yaml: …`. Además, ningún servicio puede llamarse como otro servicio del stage (§5.2 regla 2 del diseño): `compose_yaml: el servicio "web" ya existe en la app "front" del stage…`. Y una app de formulario nueva no puede tener el slug de un servicio de una app compose del stage: `slug: …`.

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
  "cert_source": "letsencrypt",
  "origin_ca": null,
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
| `proxied` | Detrás del proxy de Cloudflare. Solo tiene efecto si muelle maneja el registro (`cf_record_id`). Un dominio fuera de toda zona (o creado sin token) queda con `false`, aunque el `POST` mandara `true` |
| `tls_mode` | `cloudflare` (DNS-01) · `http` (HTTP-01) |
| `cf_zone_id` | La zona de Cloudflare; null si no cae en ninguna, o si no se pudo consultar |
| `cf_record_id` | El registro `A` que maneja muelle; null si no lo maneja (§9). El `AAAA` (`cf_record_id_aaaa`) no se expone |
| `status` | `pending` · `active` · `error` |
| `last_error_code` | Solo con `error`: `record_exists` (→ Adoptar), `cloudflare` (→ Reintentar) o `certificate` (→ Reintentar). La migración 0003 garantiza que un `error` siempre trae código |
| `last_error` | El mensaje, en español |
| `cert_issuer`, `cert_expires_at` | Lo último que vio la verificación de §8 en un certificado válido: la organización del emisor (o su CN, si no tiene) y su vencimiento. Un certificado inválido no los cambia. Es un texto observado, no un enum: para saber qué certificado se usa está `cert_source` |
| `cert_source` | `letsencrypt` · `cloudflare_origin_ca`. **Calculado, no se guarda:** qué certificado le pide muelle a Caddy para el dominio ahora. `letsencrypt` es el de ACME (DNS-01 con `tls_mode: cloudflare`, HTTP-01 con `http`); `cloudflare_origin_ca`, el de Cloudflare Origin CA que emitió muelle (§3.7). Es `cloudflare_origin_ca` si y solo si `origin_ca.status` es `active`. Nunca null |
| `origin_ca` | Objeto o null. **Calculado:** el estado del certificado de Cloudflare Origin CA del dominio (§3.7): `{"status", "error_code", "error", "retry_at", "expires_at"}`. Null con `cloudflare_origin_ca` apagado (§4), sin token o con el proxy deshabilitado (`disabled`, §3.3) |
| `records` | **Calculado, no se guarda:** los registros que el dominio necesita con los ajustes actuales. Un `A` con `public_ip` si hay, y un `AAAA` con `public_ipv6` si hay. `name` es el hostname. Con `cf_record_id` son los que maneja muelle; sin zona, los que hay que crear a mano. `[]` sin IP pública |
| `created_at`, `updated_at` | `updated_at` es el último cambio del dominio: crearlo, un `PATCH` que cambia algo, adoptarlo, un reintento que rehace el DNS y cada cambio de `status`. Una verificación que solo actualiza `cert_issuer`/`cert_expires_at` no lo mueve |

**Estados:**
- **Crear:**
  - Con token, y el hostname cae en una zona visible (el sufijo más largo):
    - Se crean `A` (y `AAAA` si hay `public_ipv6`) con el `proxied` pedido, y queda `pending` con `tls_mode: cloudflare`.
    - Si ya hay un registro de ese nombre que muelle no creó (un `A`, un `AAAA` o un `CNAME`), no se toca: `error` con `record_exists`, sin `cf_record_id`. La zona sí se encontró: el dominio queda con `cf_zone_id` y `tls_mode: cloudflare`, así el certificado sale por DNS-01 aunque el registro apunte a otro lado (§16 del diseño, con Dokploy todavía en 80/443), y adoptar usa esa zona.
    - Los registros que crea muelle llevan el comentario `muelle` en Cloudflare, para reconocerlos en el panel de Cloudflare.
    - Si Cloudflare falla: `error` con `cloudflare`. No se responde `502`: un error de DNS deja el dominio en error, y el `POST` igual responde `201`.
    - Si no hay `public_ip`: `error` con `cloudflare`, y el mensaje "configura la IP pública en Ajustes".
  - Sin token, o sin zona: `tls_mode: http`, `pending`, `proxied: false`, y el registro se crea a mano.
  - Si no se pudieron listar las zonas: `error` con `cloudflare`, con `tls_mode: http` provisorio. El reintento rehace todo.
- **`pending` → `active`:** cuando la verificación de §8 ve un certificado válido que vence en más de 14 días. Corre a los 30 s de crear, adoptar o reintentar; mientras el dominio sigue `pending`, otra vez cada minuto; y para todos, cada hora (el job `"certificados"`).
- **Un `pending` sin certificado válido 10 min después** de su último cambio (`updated_at`) pasa a `error` con `certificate`. Mientras tanto, el certificado autofirmado o vacío que sirve Caddy al emitir no cuenta como error.
- **`active` → `error`** con `certificate`: el certificado es inválido o vence en menos de 14 días.
- **`error` con `certificate` → `active`:** la verificación de cada hora también mira estos dominios, y uno que ya tiene un certificado válido vuelve a `active`. No mira los `error` con `cloudflare` ni con `record_exists`: primero hay que arreglar el DNS.
- **Los certificados de Cloudflare** (§3.7) no cambian estos estados: un dominio que se sirve con Origin CA sigue las mismas reglas (la verificación acepta la hoja que guardó muelle), y una emisión que falla no lo deja en `error`: el aviso va en `origin_ca`.
- **Cuándo no se sabe:** si la verificación no puede conectarse a Caddy, si el handshake TLS no termina en 5 s, o si el proxy no está `synced` (§3.3), no cambia nada: un Caddy que arranca, o que todavía no recibió la configuración, no sirve los certificados aunque existan. Un handshake que Caddy rechaza (no tiene un certificado para ese nombre) sí es un certificado inválido.

### 3.2 Endpoints

| Endpoint | Cuerpo | Respuesta |
|---|---|---|
| `GET {app}/domains` | — | `200 [dominio]`, ordenados por hostname |
| `POST {app}/domains` | `{hostname, target_service, target_port, proxied?}` (`proxied` por defecto `false`) | `201 dominio` |
| `PATCH {app}/domains/{hostname}` | `{proxied?, target_service?, target_port?}`; `{}` no cambia nada | `200 dominio` |
| `DELETE {app}/domains/{hostname}` | — | `204` |
| `POST {app}/domains/{hostname}/adopt` | sin cuerpo (o `{}`) | `200 dominio` |
| `POST {app}/domains/{hostname}/retry` | sin cuerpo (o `{}`) | `200 dominio` |

`{hostname}` va url-encoded y tiene que pertenecer a esa app. Si no: `404 not_found`, aunque exista en otra. En `adopt` y `retry`, un cuerpo que no es vacío ni `{}` es `400 invalid_json`, como en el rollback (§2.7).

**Validación** (`422 invalid`):
- `hostname`: `validate.Hostname`. Tampoco puede ser `panel_domain` ("es el dominio del panel").
- `target_service`: un slug de hasta 16 caracteres. En una app de formulario, tiene que ser el slug de la app. En una app compose, en la v1 no se verifica al guardar: lo verifica el render (§6.3), y el deploy falla con el dominio y el servicio en el mensaje.
- `target_port`: de 1 a 65535.
- En un `PATCH`, `proxied` sobre un dominio sin `cf_record_id`: `proxied: muelle no maneja el registro DNS de este dominio` (con el campo adelante, como todo `422`, §0.2).
- El `hostname` no se edita: mandarlo en el `PATCH` es `400 invalid_json`.

**Comportamiento:**
- `POST` con un hostname que ya usa cualquier app: `409 conflict`.
- Crear, editar y borrar aplican de inmediato el DNS y la ruta de Caddy, sin deploy (§5.2 regla 4). Con los certificados de Cloudflare encendidos (§3.7), crear, editar, adoptar, reintentar y borrar despiertan además su reconciliación, después de `SyncRoutes` y sin esperarla.
  - Primero se guarda, y después `SyncRoutes`.
  - Un fallo de Caddy no hace fallar la petición: se ve en `GET /proxy` y se reintenta.
  - Las operaciones sobre dominios (y la actualización de los registros de §3.5) van de a una, y no dependen del cliente: si se va a mitad de la petición, la operación termina igual (a lo más 30 s; 20 s en `retry`).
- `PATCH` de `proxied` actualiza los registros que maneja muelle (el `A` y el `AAAA`). Si Cloudflare falla (o ya no hay token), se guarda el valor pedido, el dominio queda en `error` con `cloudflare` y se responde `200`. Si funciona y el dominio estaba en `error` con `cloudflare`, vuelve a `pending`.
- `DELETE`:
  - Con `cf_record_id`, y con token, borra primero los registros en Cloudflare (`A` y `AAAA`). Un registro que ya no existe allá no es error.
  - Si Cloudflare no responde, o rechaza el borrado (un token sin permiso): `502 cloudflare_unavailable`, y no se borra nada en muelle. Para borrarlo solo en muelle, se quita el token en Ajustes.
  - Sin token, borra solo en muelle. Si muelle manejaba registros del dominio, la auditoría lo dice con `{"dns": "kept"}`: quedaron en Cloudflare.
- `adopt`:
  - Solo con `error` + `record_exists`; si no, `409 nothing_to_adopt`.
  - Necesita el token y `public_ip`. Sin alguno de los dos, el dominio queda en `error` con `cloudflare` y un mensaje que dice cuál falta, y se responde `200`. Lo mismo si Cloudflare falla.
  - Actualiza el contenido del registro existente a la IP pública y **conserva su `proxied`**, que se copia al dominio. Adoptar nunca cambia el modo de Cloudflare (§16); para cambiarlo, un `PATCH` después.
  - Un `CNAME` de ese nombre se convierte en un `A` a la IP pública, con el mismo `proxied`. Si hay más de un `A` o `CNAME` de ese nombre (o, con `public_ipv6`, más de un `AAAA`), no se toca ninguno: `error` con `cloudflare`, y el mensaje pide dejar uno solo.
  - Con `public_ipv6`, adopta también el `AAAA` que haya, con su `proxied` (o lo crea, con el del `A`). Sin `public_ipv6`, un `AAAA` ajeno no se toca ni pasa a manejarlo muelle.
  - Si el único registro ajeno es un `AAAA` (ni `A` ni `CNAME`), muelle crea su `A` con el `proxied` de ese `AAAA`, que se copia al dominio. Con `public_ipv6`, después adopta el `AAAA`; sin `public_ipv6`, el `AAAA` no se toca, como en el punto anterior.
  - Si el registro ajeno ya no está, crea los suyos, como al crear el dominio.
  - Guarda `cf_record_id` y deja el dominio `pending`.
- `retry`, sincrónico, con a lo más 20 s:
  - `cloudflare`: rehace el DNS (zona y registros). Si muelle ya maneja el `A`, solo lo actualiza (y crea el `AAAA` que falte).
  - `record_exists`: vuelve a mirar. Si el registro ajeno ya no está, lo crea.
  - `pending`, `certificate` y `active`: verifica el certificado ahora (con las reglas de §3.1).
    - `certificate`: con un certificado válido pasa a `active`; si no, vuelve a `pending` con otros 10 min de plazo, y muelle le reenvía la configuración a Caddy forzando la recarga (`Cache-Control: must-revalidate`), así Caddy vuelve a intentar la emisión.
    - `pending`: no reinicia el plazo de 10 min.
  - Sin zona: vuelve a buscar la zona, por si se agregó un token o una zona. Si la encuentra, crea los registros como al crear el dominio.
  - Con los certificados de Cloudflare encendidos (§3.7), además reinicia los reintentos del de Origin CA de ese dominio y despierta su reconciliación, sin esperarla. La respuesta trae `origin_ca` con `retry_at` en la hora del pedido si había una falla, y el resultado se ve en un `GET` después.
  - Responde con el estado resultante.
- **Auditoría:** `domain.create`, `domain.update`, `domain.delete`, `domain.adopt` y `domain.retry`, con target `domain:<hostname>` y details `{"app": "<p>/<s>/<a>"}`.

### 3.3 `GET /proxy` → 200 · sesión

El estado del proxy, para el banner "proxy desincronizado" (§8, §13):

```json
{
  "status": "synced",
  "error": null,
  "synced_at": "2026-10-10T14:20:41Z",
  "caddy": { "image": "ghcr.io/cpu-commits/muelle-caddy:v0.1.0", "running": true, "http_port": 80, "https_port": 443 },
  "ports_rollback": null,
  "panel_certificate": { "hostname": "panel.tudominio.cl", "cert_source": "letsencrypt", "origin_ca": null }
}
```

| Campo | Qué |
|---|---|
| `status` | `synced`: la última configuración se aplicó. `out_of_sync`: la última falló, o el contenedor no se pudo crear (un puerto ocupado, la imagen que no baja), y se reintenta con backoff. `recreating`: muelle-caddy se va a recrear, o se está recreando, por un cambio de puertos en `PATCH /settings` (§3.5); termina en `synced` u `out_of_sync`. `disabled`: `ProxySettings.Enabled` es falso (sin `panel_domain` o sin imagen de Caddy, en desarrollo) |
| `error` | Solo con `out_of_sync`: el último fallo del contenedor mientras no se resuelva (un puerto ocupado al recrear, la imagen que no baja) y, si no hay, el de `POST /load`. Null con los demás estados. Si el mensaje de Caddy trae el token de Cloudflare o una clave de un certificado de Origin CA (§3.7), va como `***` |
| `synced_at` | El último `POST /load` exitoso; null si todavía no hubo uno desde que arrancó muelle |
| `caddy.image`, `caddy.running` | Lo que corre de verdad, según `InspectContainer`. Si el contenedor no existe: `image: null` y `running: false` |
| `caddy.http_port`, `caddy.https_port` | Los puertos de los ajustes |
| `ports_rollback` | La última vez que muelle volvió a los puertos anteriores porque muelle-caddy no arrancó con los nuevos (§3.5), o null. `{"attempted": {"http_port": 80, "https_port": 443}, "restored": {"http_port": 8080, "https_port": 8443}, "error": "…", "at": "2026-10-16T20:41:07.12Z"}`: los que fallaron, a los que volvió (los de los ajustes desde entonces), por qué (en español, sin secretos) y cuándo. Va en memoria: se pierde al reiniciar muelle (queda en la auditoría), y se limpia con el próximo `PATCH /settings` que cambia los puertos. El `status` sigue las reglas de siempre: `synced` si el contenedor volvió con los puertos anteriores y Caddy tiene la configuración |
| `panel_certificate` | Objeto o null. El certificado del panel: null sin `panel_domain`. Si no, `{"hostname", "cert_source", "origin_ca"}`: `hostname` es `panel_domain`, y `cert_source` y `origin_ca` tienen la forma y las reglas de los de un dominio (§3.1, §3.7), salvo que el panel no tiene `retry`: su falla se reintenta sola, y su mensaje de `permission` no nombra Reintentar (§3.7). El panel no tiene `status` ni `cert_issuer`: muelle no lo verifica. Va también con `disabled`, sin consultar Docker, y ahí `origin_ca` es null |

Es un estado en memoria, más `InspectContainer` (y `panel_certificate`, que sale de la base). Con Docker caído: `503 docker_unavailable`. Con `disabled` no se consulta Docker: responde `200` con `image: null` y `running: false`, también sin Docker (desarrollo). Al arrancar muelle, hasta el primer `POST /load` (el job `"proxy"` corre enseguida), dice `out_of_sync` con el mensaje "muelle todavía no le mandó la configuración a Caddy".

### 3.4 Caddy

Lo del diseño (§3.1 y §8), con estas precisiones:

- **La configuración es JSON** (`POST /load`), así que los atajos del Caddyfile no existen: cada placeholder va con su nombre completo. En particular, `{client_ip}` es un atajo del Caddyfile (`caddyconfig/httpcaddyfile/shorthands.go`) de `{http.vars.client_ip}`. En JSON llega tal cual, y entonces `X-Real-IP` no trae una IP: `utils.ClientIP` cae a la IP de Caddy, todos los límites y bloqueos del login quedan en una sola IP (cualquiera dejaría fuera a todos), y `client_ip` y `would_lock_out` miran la IP de Caddy. El test golden de la Fase 3 (§15 del diseño) verifica que no quede ningún atajo (`{client_ip}`, `{host}`, `{remote_host}`…).
- **Las piezas del panel**, en JSON (un esqueleto: el resto, TLS y las rutas de las apps, según §8 del diseño):

  ```json
  {
    "admin": { "listen": "unix//run/caddy-admin/admin.sock" },
    "apps": { "http": { "servers": { "muelle": {
      "listen": [":443"],
      "trusted_proxies": { "source": "static", "ranges": ["173.245.48.0/20", "…los rangos de Cloudflare"] },
      "client_ip_headers": ["CF-Connecting-IP", "X-Forwarded-For"],
      "trusted_proxies_strict": 1,
      "routes": [{
        "match": [{ "host": ["panel.tudominio.cl"] }],
        "handle": [{ "handler": "subroute", "routes": [
          { "match": [{ "not": [{ "client_ip": { "ranges": ["190.5.6.0/24"] } }] }],
            "handle": [{ "handler": "static_response", "status_code": 403 }] },
          { "handle": [{ "handler": "reverse_proxy",
              "upstreams": [{ "dial": "muelle_muelle:8080" }],
              "headers": { "request": { "set": { "X-Real-IP": ["{http.vars.client_ip}"] } } } }] }
        ] }],
        "terminal": true
      }]
    } } } }
  }
  ```

- **IPs permitidas en el panel:** el matcher es **`client_ip`** (`{"client_ip": {"ranges": […]}}`), no `remote_ip`. Sin `panel_allowed_cidrs`, no va la ruta del `403`.
  - `remote_ip` mira el peer directo, que con el panel `proxied` es el edge de Cloudflare.
  - `client_ip` usa la IP real que resuelve `trusted_proxies` del servidor: los rangos de Cloudflare (`{"source": "static", "ranges": […]}`), con `client_ip_headers: ["CF-Connecting-IP", "X-Forwarded-For"]`. Esos headers solo cuentan si el peer está en esos rangos: un cliente que llega directo no puede poner su IP. Con `trusted_proxies_strict: 1`, `X-Forwarded-For` se lee de derecha a izquierda: cuenta la IP que vio Cloudflare, no una que el cliente puso adelante (solo importa si falta `CF-Connecting-IP`).
  - Es la misma IP que muelle recibe en `X-Real-IP` (`set` la pisa, nunca se reenvía la del cliente) y muestra como `client_ip` en `GET /settings`.
  - Una IP fuera de la lista recibe `403`, y la ruta del panel no se rompe.
- **Configuración siempre completa**, con `admin.listen` = `naming.CaddyAdminListen` (`unix//run/caddy-admin/admin.sock`).
- **muelle-caddy**: lo crea y lo recrea `ingress`, solo si `ProxySettings.Enabled(naming.CaddyImageFor(version))`. Si no (desarrollo), ingress no toca Docker por Caddy: no crea el contenedor, `SyncRoutes` devuelve `nil` sin mandar nada, el job `"proxy"` no hace nada y no se verifican certificados. Los registros DNS sí se manejan igual.
  - **Contenedor:** `naming.CaddyContainer`, `--restart unless-stopped`.
  - **Comando y entorno:** los pone ingress, no la imagen. `Cmd` es `naming.CaddyCommand()` (`caddy run --resume`) y `Env` es `naming.CaddyEnv()` (`CADDY_ADMIN=unix//run/caddy-admin/admin.sock`, `XDG_CONFIG_HOME=/config` y `XDG_DATA_HOME=/data`).
    - Sin autosave (el primer arranque, o un volumen nuevo), Caddy arranca vacío con la admin API en el socket, y el job `"proxy"` le manda la configuración.
    - Con autosave, `--resume` retoma la última configuración que mandó muelle, que ya trae `admin.listen`: los sitios vuelven sin esperar a muelle.
    - El CMD de una imagen de Caddy típica (`caddy run --config /etc/caddy/Caddyfile --adapter caddyfile`) deja la admin API en TCP (`localhost:2019`) y el socket nunca aparece. Por eso no se usa, y la imagen de la Fase 5 no necesita un CMD ni un archivo de configuración propios. Una `caddy_image` propia sirve si es Caddy 2.10 o más nuevo y trae el módulo `dns.providers.cloudflare`. Antes de la 2.10, la admin API por socket unix revisa el `Host` y rechaza el que manda muelle, así que todo `POST /load` falla (caddy#6832).
    - `XDG_*` dejan el autosave y los certificados en sus volúmenes aunque la imagen no defina esas variables. Si no, se perderían al recrear el contenedor, y se volverían a emitir los certificados.
  - **Puertos:** `proxy_http_port` → 80/tcp, y `proxy_https_port` → 443/tcp y 443/udp.
  - **Volúmenes:** `muelle_caddy_data:/data`, `muelle_caddy_config:/config` y `muelle_caddy_admin:/run/caddy-admin` (`naming.CaddyDataDir`, `CaddyConfigDir` y `CaddyAdminDir`).
  - **Redes:** se crea en `muelle_panel` y se conecta a cada red de stage.
    - Después de crearlo (también al volver a los puertos anteriores, §3.5), `EnsureCaddy` vuelve a leer los stages y lo conecta a las redes que no estaban en la primera lectura. La ventana va desde la primera lectura de los stages hasta `CreateContainer`, e incluye la bajada de la imagen y el `StopContainer` (hasta 10 s). Un `EnsureStageNetwork` que corre ahí (el paso 4 de un deploy) no conecta al contenedor nuevo, y sin esto las rutas de ese stage darían 502 hasta el próximo job `"proxy"`. Solo afecta a los stages que faltan en la primera lectura (los nuevos); los demás ya estaban en la lista. Si se conecta alguna red, queda un `Info` en el log.
    - catalog guarda un stage nuevo recién después de asegurar su red, así que puede llegar tarde a esa lectura. No importa: el stage no tiene rutas que servir hasta su primer deploy, cuyo paso 4 conecta la red, y si no, la conecta el job `"proxy"`.
    - **Una red ajena con el nombre del stage:** si ya existe una red de Docker que se llama como la del stage y no es overlay attachable (una red `bridge` creada a mano, por ejemplo), no la creó muelle: ingress no la usa, no la borra ni conecta Caddy, y `EnsureStageNetwork` devuelve `ErrNotStageNetwork`. Al crear el stage, `POST /projects/{p}/stages` responde `409 network_conflict` sin guardarlo (§0.2); en el paso 4 de un deploy, el deploy falla con `no se pudo asegurar la red …`. Una overlay attachable con ese nombre sí se usa, aunque no la haya creado muelle.
  - **Cuándo se recrea:** si cambian la imagen, los puertos, el comando (`Container.Cmd`) o los volúmenes, si no está corriendo, o si corre sin publicar sus puertos (`len(PublishedPorts) == 0` con `Ports` no vacío).
  - **Recrear es** bajar la imagen si falta, y recién entonces detenerlo (`StopContainer`, 10 s: Caddy cierra bien y suelta el socket), borrarlo y crear uno nuevo, **nunca rearrancar el mismo.** Si la imagen no baja, el contenedor anterior sigue sirviendo los sitios y el panel, y el error queda en `GET /proxy`. Docker 29 arranca sin puertos un contenedor cuyo arranque falló con `ErrPortInUse` (el fake lo imita). Uno detenido o caído también se recrea, en vez de arrancarlo.
  - **Puerto ocupado** (`docker.ErrPortInUse`): el contenedor anterior ya no existe.
    - Si fue la recreación de un cambio de puertos (§3.5), muelle vuelve a los puertos con que servía.
    - Si no (el job `"proxy"`: el primer arranque, un contenedor que no servía, un cambio hecho a mano con el `UPDATE` de §4.1), se reintenta con los mismos ajustes en el job, y el error queda en `GET /proxy`.
    - `PATCH /settings` prueba los puertos nuevos antes de guardarlos (§4.1), así que esto pasa solo si alguien los toma entre el `PATCH` y la recreación.
    - Los puertos se prueban antes de crear el contenedor nuevo, con uno descartable, `muelle-caddy-probe`: la misma imagen, en la red `bridge` y sin redes de Swarm, corre `caddy version` y se borra siempre. Si no, un arranque que falla por un puerto deja en Docker 29 una IP de `muelle_panel` tomada que no se suelta, y los reintentos cada 5 min agotarían la red.
- **Imagen:** `caddy_image` si está, o `naming.CaddyImageFor(version)`. Se baja con `PullImage` si `InspectImage` da `ErrNotFound`: con la credencial de `registries` de su host si hay una (§1.1), y si no, anónima.
  - Las imágenes de muelle son privadas (decisión del dueño, 2026-10-15): install baja la primera y siembra el registry `ghcr.io`, y con ese registry se bajan las versiones nuevas.
  - Si el registry no da acceso a la imagen, el `error` de `GET /proxy` dice qué hacer con el registry de ese host en Ajustes → Registries: agregarlo si no hay uno, o actualizar su credencial si la hay. La contraseña del registry nunca aparece en un mensaje, un log ni un JSON.
- **TLS** (§8 del diseño), en `apps.tls.automation.policies`, una política por modo:
  - `cloudflare`: el emisor `acme` con `challenges.dns.provider` = `{"name": "cloudflare", "api_token": …}` y `resolvers: ["1.1.1.1:53", "1.0.0.1:53"]` (la propagación se mira en Cloudflare, no en el DNS del host). Un dominio con `tls_mode: cloudflare` va por `http` si ya no hay token.
  - `http`: el emisor `acme`, sin desafío DNS (Let's Encrypt, HTTP-01 o TLS-ALPN-01).
  - `email` es `acme_email`, y no va si está vacío.
  - Con los certificados de Cloudflare (§3.7), un host que se sirve con Origin CA no va en ninguna política: su certificado va en `apps.tls.certificates.load_pem`, el hostname en `automatic_https.skip_certificates` del servidor `muelle`, y `apps.tls.disable_ocsp_stapling` es `true`. Sin ninguno, la configuración no cambia.
- **Sin placeholders en los ajustes:** Caddy expande placeholders en el email del emisor `acme` y en `api_token` (`a{x}@b.cl` hace fallar el `POST /load` entero).
  - `PATCH /settings` rechaza un `acme_email` o un `cloudflare_token` con `{` o `}`: `422` en ese campo, y no guarda nada (§4.1).
  - La configuración inicial también, con la misma regla (`validate.NoPlaceholders`): install, `--email` y `--cloudflare-token` (código 2 y el nombre del flag, antes de tocar Docker), y muelle, `MUELLE_INIT_ACME_EMAIL` y el token de `MUELLE_INIT_CLOUDFLARE_TOKEN_FILE` (no arranca, y el error nombra la variable; §5.2). Con ellos Caddy nunca recibiría la configuración, y el panel, que va por Caddy, no serviría para corregirlos. El error no cita el token.
  - Si igual llegan a los ajustes (un `UPDATE` a mano), muelle no manda la configuración, y `GET /proxy` dice `out_of_sync` con el motivo.
- **Errores de Caddy:** el mensaje de un `POST /load` rechazado se guarda (y se registra en el log) con el token de Cloudflare reemplazado por `***`, y también cada clave de un certificado de Origin CA (el PEM entero y cada una de sus líneas, §3.7), cortado a 500 caracteres. La configuración misma nunca va al log.
- **Ruta del panel:** `panel_domain → naming.MuelleService:naming.MuellePort` (`muelle_muelle:8080`), con `X-Real-IP` = `{http.vars.client_ip}` en `headers.request.set` del `reverse_proxy` (el JSON de arriba).
- **TLS del panel:**
  - `cloudflare` si hay token y `panel_domain` cae en una zona visible (`panel_cf_zone_id`, §3.6); si no, `http`.
  - El registro DNS del panel se crea solo si no existe, con `proxied: false`. Uno existente nunca se toca, y en la v1 no se adopta. Sin `public_ip`, no se crea ni se actualiza ninguno.
  - Su `AAAA` se crea solo si muelle maneja el `A`, con el `proxied` que tenga el `A`; sin `public_ipv6`, muelle borra el suyo.
  - Sus ids se guardan en las claves reservadas de §3.6. Si cambia la IP pública, muelle actualiza solo el contenido de los suyos: nunca cambia su `proxied`, que se puede cambiar en Cloudflare.
  - Antes de tocar un registro guardado, muelle lo lee: si ya no existe, ya no es de `panel_domain` (cambió `MUELLE_PANEL_DOMAIN` o `MUELLE_PANEL_ORIGIN`) o el token ya no lo puede leer (Cloudflare rechaza la lectura: un token de otra cuenta, o una zona que ya no ve), olvida su id sin borrarlo, y vuelve a crear uno solo si no hay otro. Si Cloudflare no responde, no olvida nada.
  - La excepción: si el `A` guardado ya no existe y su `AAAA` sí, el `AAAA` se borra. Si no, contaría como un registro que muelle no creó, y el panel quedaría sin `A`.
  - Con los certificados de Cloudflare encendidos, y si todos sus registros `A`, `AAAA` y `CNAME` tienen el proxy en Cloudflare (los cree muelle o no), el panel usa el de Origin CA (§3.7). Quien entre sin pasar por Cloudflare (con `/etc/hosts`, o a la IP) ve un certificado en el que su navegador no confía. Si se apaga el proxy del registro en Cloudflare, muelle vuelve a Let's Encrypt en a lo más 5 min.

### 3.5 Los hooks de admin

- **`CheckCloudflareToken(ctx, token)`** (antes de guardar un token nuevo en `PATCH /settings`):
  - verifica el token (`GET /user/tokens/verify`) y cuenta las zonas que ve (`GET /zones`, todas las páginas);
  - un token que ese endpoint rechaza pero que sí lista zonas (un token de una cuenta, que se verifica en otra ruta) se acepta;
  - un token rechazado (también uno desactivado o vencido), uno que no puede listar las zonas, o uno sin zonas, devuelve un `*validate.Error` en `cloudflare_token` (`422`), con un mensaje que dice qué pasó o qué permisos faltan (Zone:Read y DNS:Edit), sin el token;
  - si Cloudflare no responde (sin red, un 5xx o un 429), `entity.ErrCloudflareUnavailable` (`502`);
  - si falla, no se guarda nada del `PATCH`.
- **`CheckCloudflareOriginCA(ctx, token, field)`** (antes de guardar, después de `CheckCloudflareToken`, si `cloudflare_origin_ca` queda encendido y se enciende en ese `PATCH` o cambió el token; §3.7):
  - `field` es `cloudflare_origin_ca` si se enciende en ese `PATCH`, y `cloudflare_token` si cambió el token con la opción ya encendida;
  - lista las zonas que ve el token (`GET /zones`, todas las páginas) y comprueba Zone → SSL and Certificates → Edit en cada una de las **activas** (`status: active`). Una zona `pending` todavía no la usa Cloudflare de proxy, y no se revisa; si ninguna está activa, no hay nada que revisar y pasa:
    - si la zona trae `#ssl:edit` en `permissions` (un campo que Cloudflare marca como deprecado), pasa, sin efectos;
    - si no, emite un certificado de prueba de 7 días para el apex de la zona y lo revoca enseguida. Nunca lo sirvió nadie, así que revocarlo no corta nada. Cloudflare no documenta una forma de comprobarlo sin efectos. Si la revocación falla, muelle la reintenta después;
  - si falta en alguna, o el token no puede listar las zonas, o no ve ninguna, o Cloudflare rechaza la prueba por otro motivo: un `*validate.Error` en `field` (`422`), con los mensajes de §4.1, sin el token: el mensaje de Cloudflare que citan se corta a 300 caracteres y nunca trae el token;
  - si Cloudflare no responde (sin red, un 5xx o un 429), `entity.ErrCloudflareUnavailable` (`502`);
  - con un plazo de 30 s sobre el ctx de la petición; si falla, no se guarda nada del `PATCH`. `cloudflare_origin_ca: false` nunca llama a Cloudflare.
- **`CheckProxyPorts(ctx, old, next)`** (antes de guardar, si cambió `proxy_http_port` o `proxy_https_port`):
  - con el proxy deshabilitado (desarrollo) no toca Docker y devuelve `nil`;
  - prueba solo los puertos que cambian, de a un campo: `proxy_http_port` en TCP, y `proxy_https_port` en TCP y en UDP (HTTP/3). No prueba los que ya publica el muelle-caddy que sirve, que los suelta al recrearse (cambiar 80/443 por 443/80 prueba solo el 80 en UDP);
  - la prueba es la de §3.4: un contenedor descartable, `muelle-caddy-probe`, con la imagen del muelle-caddy que sirve (o la de los ajustes, si no hay uno), en la red `bridge` y sin redes de Swarm, que corre `caddy version` y se borra siempre. Nunca toca muelle-caddy;
  - un puerto ocupado (por otro contenedor o un proceso del host) devuelve un `*validate.Error` en ese campo (`422`): `proxy_https_port: el puerto 443 del host ya está en uso (en TCP o en UDP): libéralo o elige otro`;
  - Docker caído: `entity.ErrDockerUnavailable` (`503 docker_unavailable`). Si la prueba no se puede hacer por otra cosa (la imagen no está en el nodo), queda en el log y se guarda igual: la protección de fondo es volver atrás (abajo);
  - con un plazo de 30 s sobre el ctx de la petición; si falla, no se guarda nada del `PATCH`.
- **`ProxySettingsChanged(ctx, old, next)`** (después de guardar, antes de responder):
  - corre con su propio plazo de 2 min, derivado con `context.WithoutCancel`;
  - aplica, según `next.ChangedKeys(old)`:
    - IPs, email, token, lista de IPs o `cloudflare_origin_ca` → regenera la configuración de Caddy. `POST /load` no corta las conexiones abiertas, así que la respuesta del `PATCH` sale igual. Si en el mismo `PATCH` cambiaron los puertos, no la manda ahora: la manda la recreación, con los ajustes de ese momento;
    - `cloudflare_origin_ca`, o el token con la opción encendida → reinicia los reintentos de todos los certificados de Cloudflare y despierta su reconciliación (§3.7), sin esperarla. Al apagarla, la configuración que se manda ya no trae los de Origin CA, así que los dominios vuelven a Let's Encrypt antes de responder, los certificados se revocan después, y muelle olvida lo que vio en Cloudflare y las fallas, así al encenderla otra vez no arrastra una falla vieja (salvo la de una pasada que todavía corría, §3.7);
    - IP pública → actualiza los registros que maneja muelle, de los dominios y el del panel (los que fallan dejan su dominio en `error`/`cloudflare`):
      - un `public_ip` nuevo cambia el contenido de los `A` (nunca su `proxied`); borrar `public_ip` no toca ningún registro (borrarlos dejaría los sitios sin DNS);
      - un dominio en `error`/`cloudflare` cuyos registros se actualizan bien vuelve a `pending`; los demás no cambian de estado;
      - un `public_ipv6` nuevo crea el `AAAA` de cada dominio cuyo `A` maneja muelle (si ya hay un `AAAA` ajeno, ese dominio queda en `error`/`cloudflare`) y cambia los que ya maneja; borrarla borra los `AAAA` que maneja muelle;
      - sin token, los dominios cuyos registros maneja muelle quedan en `error`/`cloudflare`, porque no se pueden actualizar;
    - token nuevo → busca la zona del panel y su registro (§3.4), como el job `"proxy"`. Los dominios que se crearon sin token no cambian solos: el reintento busca su zona (§3.2);
    - puertos → **agenda** la recreación de muelle-caddy, y no la hace antes de volver. Además limpia `ports_rollback` de `GET /proxy`: es un intento nuevo;
    - la imagen nunca cambia por un `PATCH`: `caddy_image` no está en `/settings` (§4.1), y solo la escribe la configuración inicial (`--caddy-image`, `MUELLE_INIT_CADDY_IMAGE`, §5.2). La imagen es esa `caddy_image` fijada o, si está vacía, la de la versión de muelle: cambia con una versión nueva de muelle, y la aplica el job `"proxy"` del arranque (§3.4 y §5.4);
  - no devuelve error: el resultado se ve en `GET /proxy` y en cada dominio.
- **Por qué la recreación va después:** el `PATCH` mismo pasa por muelle-caddy. Si el hook lo borrara antes de responder, el cliente recibiría una conexión cortada por un cambio que sí se guardó.
  - `ingress.Run` recrea el contenedor 2 s después de que el hook volvió, que alcanza para que la respuesta salga por el Caddy viejo. Mientras tanto, y hasta que termina, `GET /proxy` dice `recreating`.
  - Dos cambios seguidos se juntan en una sola recreación, con los ajustes que estén guardados en ese momento.
  - Si muelle se reinicia antes, el job `"proxy"` del arranque recrea el contenedor igual (las diferencias de puertos, §3.4), pero sin volver atrás.
  - El panel ya avisa que cambiar los puertos corta unos segundos. Si cambió solo el puerto HTTP, sigue en el mismo origen y pregunta `GET /proxy` hasta que deja de decir `recreating`. Si cambió `proxy_https_port`, el origen actual deja de responder: el panel muestra el nuevo, `https://<panel_domain>[:<proxy_https_port>]` (con los valores de la respuesta del `PATCH`), y lleva ahí al usuario. La sesión sigue, porque la cookie no depende del puerto, y ese `Origin` pasa (§0.1); el del puerto viejo ya no.
- **Si muelle-caddy no arranca con los puertos nuevos, la recreación vuelve a los anteriores.** Cuando muelle-caddy servía (corría publicando sus puertos) con otros puertos y la recreación falla (por ejemplo, un puerto ocupado, o el contenedor que no se crea o no arranca), ingress:
  1. vuelve `proxy_http_port` y `proxy_https_port` a los puertos con que servía, solo si los guardados siguen siendo los que fallaron (en una transacción): si un `PATCH` los cambió mientras tanto, gana ese, y su recreación viene después;
  2. descarta el origen del panel en caché (`PortsRolledBack`): el `Origin` del puerto anterior pasa enseguida, y el del nuevo ya no (§0.1);
  3. lo audita (`settings.ports_rollback`, actor `system`, con `{"proxy_http_port": …, "proxy_https_port": …, "attempted": {"proxy_http_port": …, "proxy_https_port": …}, "error": "…"}`), lo registra en el log y lo deja en `GET /proxy` (`ports_rollback`, §3.3);
  4. recrea muelle-caddy con los puertos anteriores y le manda la configuración. Si tampoco arranca (alguien los tomó mientras tanto), el error queda en `GET /proxy` y el job lo reintenta con esos.
  - Con Docker caído no vuelve atrás: no es culpa de los puertos, y el job reintenta con los nuevos. El job `"proxy"` tampoco vuelve atrás (§1.2).
  - Un Caddy que arranca pero no responde en su admin API no cuenta como "no arrancó": los puertos ya se tomaron, y queda `out_of_sync` como siempre.
  - Lo que ve el panel: si cambió solo el puerto HTTP, sigue en el mismo origen, y `GET /proxy` deja de decir `recreating` y trae `ports_rollback` con el motivo. Si cambió `proxy_https_port` y el origen nuevo no responde, lo más probable es que muelle haya vuelto al anterior: el panel le ofrece al usuario volver a `https://<panel_domain>[:<puerto anterior>]` (lo sabe: es el que tenía antes del `PATCH`), donde `GET /proxy` trae `ports_rollback`. La vuelta tarda lo que tarda en detenerse el Caddy viejo (hasta 10 s) más el arranque.

### 3.6 Claves de settings que escribe ingress

| Clave | Qué |
|---|---|
| `cloudflare_zones` | Cuántas zonas ve el token, en decimal. La escribe admin al guardar un token; ingress la actualiza en el job `"proxy"`, solo si el token guardado sigue siendo el que usó (así un token borrado a la vez no deja un número viejo) |
| `panel_cf_zone_id` | La zona de `panel_domain`, si cae en una visible para el token. Se guarda aunque el registro sea ajeno: decide el TLS del panel (§3.4). Se borra si el dominio del panel ya no cae en ninguna zona |
| `panel_cf_record_id`, `panel_cf_record_id_aaaa` | El registro `A` y el `AAAA` del panel que creó muelle; vacías si no creó ninguno |

Las tres `panel_cf_*` las escribe solo ingress (constantes en `shared/domain/entity`, `panel_dns.go`); `GET /settings` no las muestra y `PATCH /settings` no las acepta.

Además, ingress reescribe `proxy_http_port` y `proxy_https_port`, que son de admin, en un solo caso: al volver a los puertos anteriores (§3.5), y solo si siguen siendo los que fallaron.

### 3.7 Certificados de Cloudflare (Origin CA)

Opcionales y apagados por defecto (`cloudflare_origin_ca`, §4). Con la opción encendida y un token con el permiso **Zone → SSL and Certificates → Edit**, cada dominio de Cloudflare con el proxy (nube naranja), también el del panel, usa un certificado de Cloudflare Origin CA en vez de Let's Encrypt. Sirve para el modo SSL **Full (strict)** de la zona, que muelle no cambia. **Solo Cloudflare confía en ese certificado, los navegadores no.** Los dominios en DNS only y los que están fuera de Cloudflare siguen con Let's Encrypt. Un dominio que pierde el proxy, o todos al apagar la opción, vuelven a Let's Encrypt. La investigación, las decisiones y los riesgos están en [`2026-10-21-origin-ca-design.md`](2026-10-21-origin-ca-design.md).

- **Qué hostnames:**
  - Un dominio con `tls_mode: cloudflare`, `cf_zone_id`, cuyo registro maneja muelle (`cf_record_id`), con `proxied: true`, y cuyos registros `A`, `AAAA` y `CNAME` de ese nombre tienen **todos** el proxy en Cloudflare, con el `A` de muelle entre ellos. El `status` del dominio no cuenta.
  - Un dominio de una zona (`cf_zone_id`) cuyo registro muelle no maneja no califica: siempre está en `error` (`record_exists` o `cloudflare`), y el registro puede apuntar a otro servidor (§16 del diseño). Después de Adoptar, califica. Uno fuera de toda zona, o creado sin token, tampoco: es `tls_mode: http` (puede estar `pending` o `active`), y su registro se crea a mano.
  - El panel califica si cae en una zona (`panel_cf_zone_id`, §3.6) y todos sus `A`, `AAAA` y `CNAME` tienen el proxy, aunque muelle no haya creado el registro: muelle nunca cambia el `proxied` del panel, solo lo lee.
  - muelle lee los registros de cada uno en cada pasada de la reconciliación: cada 5 min (el job `"proxy"`, §1.2), con un `PATCH /settings` que la enciende o cambia el token (§3.5) y después de crear, editar, adoptar, reintentar o borrar un dominio (§3.2). Si Cloudflare no responde, vale lo último que vio. Si rechaza la lectura, el host vuelve a Let's Encrypt.
  - Con el proxy deshabilitado (`disabled`, §3.3) no pasa nada de esto: no hay Caddy al que darle el certificado.
- **Emisión y renovación:**
  - muelle genera una clave ECDSA P-256 y un CSR, y pide `POST /certificates` con `request_type: origin-ecc`, `requested_validity: 90` y un solo hostname.
  - Antes de guardarlo, revisa que el certificado sea para ese hostname, que vaya con la clave y que valga ahora.
  - Se renueva cuando faltan 30 días, con otra clave. Cuando faltan 14, deja de servirse y el host vuelve a Let's Encrypt hasta que se renueve.
  - Lo emite y lo renueva muelle, en segundo plano: Caddy no renueva los certificados cargados. Al encender la opción, la primera pasada empieza enseguida; con muchos dominios puede seguir en la próxima.
- **Dónde se guarda:**
  - La clave privada va cifrada con la clave maestra en `origin_ca_certs` (AAD `origin_ca_certs:private_key:<id>`, migración 0006, §6). Nunca va al log, a un error, a la API ni a la auditoría.
  - En Caddy, como el token, queda en claro en el `autosave.json` del volumen `muelle_caddy_config` y en `GET /config/` del socket de admin.
- **En Caddy** (§3.4):
  - El certificado va en `apps.tls.certificates.load_pem`, con los tags `muelle_origin_ca` y `muelle_origin_ca:<hostname>`.
  - El hostname va en `automatic_https.skip_certificates` del servidor `muelle` y sale de las políticas de `apps.tls.automation`.
  - `apps.tls.disable_ocsp_stapling` es `true` si hay alguno.
  - ACME sigue igual para los demás. El JSON de ejemplo está en §7.2 del diseño de Origin CA.
- **La verificación** (§3.1, §8 del diseño): para un host que se sirve con Origin CA, la hoja que sirve Caddy es válida si es **idéntica** a la que guardó muelle, es de ese hostname y vale en ese momento, sin verificar la cadena contra las raíces del sistema. Una hoja que verifica contra las raíces del sistema también vale, así el cambio entre Let's Encrypt y Origin CA no deja el dominio en `error`. `cert_issuer` y `cert_expires_at` salen de la hoja, como siempre.
- **Revocación** (`DELETE /certificates/{id}`):
  - Cuándo: la opción se apaga, el host pierde el proxy (en muelle o en Cloudflare), se borra el dominio, cambia el dominio del panel o la zona, o se renueva (el anterior).
  - Siempre **después** de que Caddy cargó una configuración sin ese certificado (un `POST /load` exitoso que leyó la base después de retirarlo), y con 1 min de margen: Cloudflare deja de hablar con un origen que sirve un certificado revocado.
  - Cambiar el token no revoca nada. Sin token no se puede revocar: queda pendiente hasta que vuelva uno, o hasta que venza. Una revocación que falla se reintenta, de 5 min a 24 h.
  - Va al log del servidor, sin auditoría: es una acción del sistema, como las verificaciones.
- **Al volver a Let's Encrypt**, Caddy usa el de Let's Encrypt que todavía tenga en `/data` o lo emite (DNS-01, normalmente en menos de un minuto). Mientras tanto, Cloudflare puede responder `525`. Si la verificación de cada hora cae justo en esa ventana, un dominio `active` pasa a `error` + `certificate` con las reglas de §3.1, y vuelve a `active` con la verificación siguiente o con Reintentar.
- **Si la emisión falla**, el host sigue con Let's Encrypt y el dominio **no** pasa a `error`: el aviso va en `origin_ca`. Se reintenta solo, de 1 min a 1 h si Cloudflare no respondió (`cloudflare`), y cada 6 h si lo rechazó (`permission`, `rejected`). `retry` (§3.2) y un `PATCH /settings` que enciende la opción o cambia el token reintentan enseguida. El panel no tiene `retry`: el suyo se reintenta solo, o enseguida con un `PATCH /settings` que cambia el token. Mandar otra vez `cloudflare_origin_ca: true`, o el mismo token, no reintenta nada (no es un cambio, §4.1). Tampoco hay un reintento de todos a la vez: después de arreglar en Cloudflare el permiso del mismo token, cada dominio se reintenta con su `retry`, y lo demás espera el reintento solo (a lo más 6 h), con Let's Encrypt mientras tanto. Un `429` de Cloudflare bloquea el token 5 min: la pasada que lo recibe termina ahí, y sigue la del próximo job `"proxy"`.

**`origin_ca`** (en el dominio, §3.1, y en `panel_certificate` de `GET /proxy`, §3.3). Las cinco claves van siempre:

| Campo | Tipo | Qué |
|---|---|---|
| `status` | `active` · `issuing` · `failed` · `dns_only` · `not_eligible` | Abajo, en el orden en que se decide |
| `error_code` | `permission` · `cloudflare` · `rejected`, o null | El motivo de la última falla, solo con `failed` y con `active` (una renovación que falló, y el actual todavía sirve); null con los demás. `permission`: al token le falta Zone → SSL and Certificates → Edit en esa zona, el usuario dueño del token no tiene "API Access", o la zona es de otra cuenta. `cloudflare`: Cloudflare no respondió (sin red, un 5xx, un 429). `rejected`: Cloudflare rechazó el pedido por otro motivo, o devolvió un certificado que no sirve |
| `error` | string o null | El mensaje, en español (abajo). Null cuando `error_code` es null |
| `retry_at` | RFC 3339 o null | Cuándo se reintenta la emisión o la renovación. Null cuando `error_code` es null. Después de un `retry` o de un `PATCH /settings` que reintenta enseguida, es la hora de ese pedido, y queda en el pasado hasta que termina la pasada |
| `expires_at` | RFC 3339 o null | El vencimiento del certificado de Origin CA vigente que muelle tiene guardado para ese hostname, se sirva o no (por ejemplo, uno a menos de 14 días que todavía no se renovó); null si no tiene. Con `active` nunca es null |

**`status`**, el primero que aplica:
1. `not_eligible`: no califica (arriba): `tls_mode: http`, sin `cf_zone_id` o sin `cf_record_id`; en el panel, sin `panel_cf_zone_id`. Let's Encrypt.
2. `dns_only`: un dominio con `proxied: false` en muelle. Let's Encrypt, desde la respuesta del `PATCH` que lo apagó.
3. `dns_only`: en Cloudflare, alguno de sus `A`, `AAAA` o `CNAME` no tiene el proxy (la nube gris). Let's Encrypt. En un dominio, con `proxied: true` en muelle, quiere decir que alguien lo cambió en el dashboard de Cloudflare.
4. `not_eligible`: en Cloudflare no hay ningún `A`, `AAAA` ni `CNAME` de ese nombre (en un dominio, o no está el `A` de muelle), o Cloudflare rechazó la lectura. Let's Encrypt.
5. `active`: Caddy sirve el de Origin CA. Si falló una renovación, trae `error_code`, `error` y `retry_at`: el actual sirve hasta 14 días antes de vencer.
6. `failed`: califica, pero no hay uno que se pueda servir y la última emisión falló. Let's Encrypt mientras tanto.
7. `issuing`: califica y muelle lo está emitiendo, o todavía no leyó sus registros en Cloudflare. Let's Encrypt mientras tanto; dura lo que tarda la pasada (segundos, o unos minutos con muchos dominios).

**`cert_source`** es `cloudflare_origin_ca` si y solo si `origin_ca.status` es `active`; si no (también con `origin_ca: null`), `letsencrypt`. Es lo que muelle le pide a Caddy: con el proxy `out_of_sync` (§3.3), Caddy puede tener todavía la configuración anterior.

**Cuándo cambia lo que dice la API:**
- Apagar la opción (`PATCH /settings`): desde su respuesta, todo `origin_ca` es null y todo `cert_source` es `letsencrypt`, y Caddy ya recibió la configuración sin los de Origin CA.
- Encenderla: desde su respuesta, cada dominio y el panel dicen `issuing`, `dns_only` o `not_eligible`, y `active` cuando la pasada emite el suyo. Si se vuelve a encender mientras termina una pasada que empezó antes de apagarla, puede verse lo que vio esa pasada (también un `failed`) hasta que otra pasada vuelve a mirar ese host.
- `PATCH {app}/domains/{hostname}` que cambia `proxied`: muelle olvida lo que había visto en Cloudflare de ese hostname, pero no su última falla. Con `proxied: false`, `dns_only` desde la respuesta; con `proxied: true`, `issuing` hasta la pasada que despierta, o `failed` si tenía una falla guardada (se reintenta con su espera, o con `retry`).
- DNS only puesto en el dashboard de Cloudflare: `dns_only` en a lo más 5 min.

**El mensaje de `error`** es una oración en español para mostrar tal cual. Los clientes deciden por `status` y `error_code`, nunca por el texto:
- la plantilla sigue al `status`: muelle arma el mensaje al responder, con el `status` de ese momento. Con `failed` va la de emisión, también si lo que falló fue una renovación y el actual ya no se sirve (le quedan menos de 14 días). La de renovación va solo con `active`, mientras el actual todavía se sirve;
- emisión (con `failed`): `muelle no pudo emitir el certificado de Cloudflare: <causa>; mientras tanto usa Let's Encrypt`;
- renovación (con `active`): `muelle no pudo renovar el certificado de Cloudflare, que vence el <fecha>: <causa>; si no lo renueva antes del <fecha menos 14 días>, vuelve a Let's Encrypt`;
- `<causa>`, según `error_code`:
  - `permission`, en un dominio: `el token no tiene el permiso Zone → SSL and Certificates → Edit en la zona de <hostname> (Cloudflare: <mensaje>); agrégalo al token y pulsa Reintentar`;
  - `permission`, en el panel, que no tiene `retry`: `el token no tiene el permiso Zone → SSL and Certificates → Edit en la zona de <hostname> (Cloudflare: <mensaje>); agrégalo al token: muelle lo reintenta solo cada 6 h`;
  - `cloudflare`: `Cloudflare no respondió (<detalle>); se reintenta solo`;
  - `rejected`: `Cloudflare lo rechazó (<mensaje>)`, o `Cloudflare devolvió un certificado que no sirve (<motivo>)`;
- las fechas van en UTC como `2 ene 2027`, y el mensaje de Cloudflare se corta a 300 caracteres y nunca trae el token.

**Ejemplos.** Un dominio que se sirve con Origin CA (el resto de los campos, como en §3.1):

```json
{
  "hostname": "api.tudominio.cl",
  "proxied": true,
  "tls_mode": "cloudflare",
  "status": "active",
  "cert_issuer": "CloudFlare, Inc.",
  "cert_expires_at": "2027-01-19T12:00:00Z",
  "cert_source": "cloudflare_origin_ca",
  "origin_ca": { "status": "active", "error_code": null, "error": null, "retry_at": null, "expires_at": "2027-01-19T12:00:00Z" },
  "…": "…"
}
```

`cert_issuer` es lo que trae la hoja; el texto de la de Cloudflare no está verificado, y por eso el panel nombra el origen con `cert_source`.

Una emisión que falló, con Let's Encrypt mientras tanto:

```json
"cert_source": "letsencrypt",
"origin_ca": {
  "status": "failed",
  "error_code": "permission",
  "error": "muelle no pudo emitir el certificado de Cloudflare: el token no tiene el permiso Zone → SSL and Certificates → Edit en la zona de api.tudominio.cl (Cloudflare: Authentication error); agrégalo al token y pulsa Reintentar; mientras tanto usa Let's Encrypt",
  "retry_at": "2026-10-21T21:04:10.5Z",
  "expires_at": null
}
```

Una renovación que falló, con el actual todavía sirviendo:

```json
"cert_source": "cloudflare_origin_ca",
"origin_ca": {
  "status": "active",
  "error_code": "cloudflare",
  "error": "muelle no pudo renovar el certificado de Cloudflare, que vence el 19 ene 2027: Cloudflare no respondió (HTTP 503); se reintenta solo; si no lo renueva antes del 5 ene 2027, vuelve a Let's Encrypt",
  "retry_at": "2026-12-20T10:08:00Z",
  "expires_at": "2027-01-19T12:00:00Z"
}
```

El panel, en `GET /proxy` (§3.3), con su registro en DNS only en Cloudflare:

```json
"panel_certificate": {
  "hostname": "panel.tudominio.cl",
  "cert_source": "letsencrypt",
  "origin_ca": { "status": "dns_only", "error_code": null, "error": null, "retry_at": null, "expires_at": null }
}
```

## 4. Settings (`GET/PATCH /settings`) · sesión — ya implementado, salvo `cloudflare_origin_ca` (§3.7)

`GET` (y la respuesta de `PATCH`) trae **siempre** todas las claves, sin `omitempty`. El panel las trata como obligatorias: una respuesta sin alguna es un error de contrato.

```json
{
  "require_2fa": false,
  "cloudflare_token_set": true,
  "cloudflare_zones": 4,
  "cloudflare_origin_ca": false,
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
| `cloudflare_origin_ca` | Bool, nunca null: los certificados de Cloudflare (Origin CA, §3.7, todavía sin implementar) para los dominios con el proxy. `false` por defecto. No hay un `MUELLE_INIT_*` para esto: se enciende en Ajustes |
| `public_ip`, `public_ipv6`, `acme_email` | `""` si no están |
| `panel_domain` | **Solo lectura**: `MUELLE_PANEL_DOMAIN` (lo que escribe install) o, si no está, el host de `MUELLE_PANEL_ORIGIN` si es un dominio, o `""` (una IP o `localhost`, en desarrollo). Se escribe en cada arranque |
| `panel_allowed_cidrs` | Prefijos normalizados (`validate.CIDRs`). Nunca null; `[]` es cualquier IP |
| `client_ip` | La IP con que muelle ve esta petición (`utils.ClientIP`). No es un ajuste |

### 4.1 `PATCH /settings`

Manda solo las claves que cambian: `require_2fa`, `cloudflare_token`, `cloudflare_origin_ca`, `public_ip`, `public_ipv6`, `acme_email`, `panel_allowed_cidrs`, `proxy_http_port`, `proxy_https_port` y `confirm_lockout`. Cualquier otra, `panel_domain` incluida, es `400 invalid_json`, y también un valor de otro tipo (`cloudflare_origin_ca` es un bool).

| Regla | Respuesta |
|---|---|
| `public_ip` no es una IPv4 canónica, `public_ipv6` una IPv6 (RFC 5952), o `acme_email` un email | `422` en ese campo. `""` borra el valor |
| Un puerto fuera de 1–65535 | `422` en ese campo |
| `proxy_http_port` = `proxy_https_port` (con los valores resultantes) | `422` en `proxy_https_port` |
| Un puerto nuevo del proxy ya en uso en el host (`CheckProxyPorts`, §3.5) | `422` en ese campo; no se guarda nada |
| Docker no responde al probar los puertos nuevos | `503 docker_unavailable`; no se guarda nada |
| Un rango inválido en `panel_allowed_cidrs` | `422` en `panel_allowed_cidrs` |
| `cloudflare_token` con otra forma (de 20 a 512 caracteres visibles, sin espacios) | `422 cloudflare_token` |
| `acme_email` o `cloudflare_token` con `{` o `}`, que Caddy toma como placeholders (§3.4) | `422` en ese campo (`acme_email: no puede tener { ni }: Caddy los toma como placeholders`); no se guarda nada |
| `cloudflare_token` rechazado por Cloudflare | `422 cloudflare_token` |
| Cloudflare no responde al verificar el token | `502 cloudflare_unavailable` |
| `cloudflare_token: ""` | Borra el token y `cloudflare_zones` |
| `cloudflare_origin_ca: true` en un `PATCH` que la enciende, sin token con los valores resultantes (no hay uno guardado, o el mismo `PATCH` lo quita) | `422`: `cloudflare_origin_ca: necesita el token de Cloudflare: configúralo antes de activarla`; no se guarda nada |
| `cloudflare_token: ""` con la opción encendida, sin `"cloudflare_origin_ca": false` en el mismo `PATCH` | `422`: `cloudflare_origin_ca: sin token no se pueden usar los certificados de Cloudflare: mándala en false en el mismo cambio para quitar el token`; no se guarda nada |
| La opción queda encendida, y se enciende en este `PATCH` o cambia el token, y al token le falta Zone → SSL and Certificates → Edit en alguna de sus zonas activas (`CheckCloudflareOriginCA`, §3.5) | `422` en `cloudflare_origin_ca` si se enciende en este `PATCH`, y si no en `cloudflare_token`; no se guarda nada. Los mensajes, abajo |
| Cloudflare no responde al comprobar ese permiso | `502 cloudflare_unavailable`; no se guarda nada |
| `require_2fa: true` sin el 2FA propio | `409 totp_required_first` |
| Una lista de `panel_allowed_cidrs` que deja fuera `client_ip` | `409 would_lock_out`, salvo que venga `"confirm_lockout": true` |

- **El orden**, y el primero que falla es la respuesta: las validaciones de los campos (`422`; las dos de `cloudflare_origin_ca` sin token van después de la forma del token), `409 would_lock_out`, `CheckCloudflareToken`, `CheckCloudflareOriginCA`, `CheckProxyPorts` y, al guardar, `409 totp_required_first`.
- **Los mensajes de `CheckCloudflareOriginCA`**, en `<campo>` (`cloudflare_origin_ca` o `cloudflare_token`, arriba):
  - falta el permiso: `<campo>: el token no tiene el permiso Zone → SSL and Certificates → Edit en la zona auditfy.cl (lo necesitan los certificados de Cloudflare): agrégalo al token en Cloudflare, o deja la opción apagada`. En `cloudflare_token` el final es `…: agrégalo al token en Cloudflare, o apaga antes los certificados de Cloudflare`. Las zonas van ordenadas por nombre: `en la zona a.cl`, `en las zonas a.cl y b.cl` y, con tres o más, las dos primeras y cuántas más: `en las zonas a.cl, b.cl y 3 más`;
  - el token no puede listar las zonas: `<campo>: el token no puede listar las zonas (<mensaje de Cloudflare>): dale los permisos Zone:Read y DNS:Edit sobre las zonas de tus dominios`;
  - no ve ninguna zona: `<campo>: el token no ve ninguna zona: dale los permisos Zone:Read y DNS:Edit sobre las zonas de tus dominios`;
  - Cloudflare rechazó la prueba por otro motivo: `<campo>: Cloudflare rechazó la prueba del permiso en la zona <zona> (<mensaje de Cloudflare>)`.
  - Con un token nuevo, los dos del medio ya los da antes `CheckCloudflareToken`, en `cloudflare_token` (§3.5).
- `cloudflare_origin_ca: false` nunca llama a Cloudflare, y mandar el valor que ya tiene no es un cambio (no comprueba nada ni va a la auditoría). Por eso mandar otra vez `true`, o el mismo token, tampoco reintenta los certificados de Cloudflare que fallaron: eso es `retry` en cada dominio (§3.2), y el del panel se reintenta solo (§3.7). Quitar el token con la opción encendida es `{"cloudflare_token": "", "cloudflare_origin_ca": false}`.
- Todo se guarda en una transacción. Después, si cambió algo del proxy, se llama a `ProxySettingsChanged`. El `PATCH` responde cuando el hook vuelve, y eso es antes de recrear muelle-caddy (§3.5).
- **Cambiar `proxy_https_port` mueve el panel** a `https://<panel_domain>[:<puerto>]`. Desde que responde el `PATCH`, ese es el `Origin` que pasa y el de los links de las invitaciones; el del puerto viejo ya no pasa (§0.1). No hay que tocar la configuración de muelle. Si hay un `MUELLE_PANEL_ORIGIN` puesto a mano, ese sigue pasando y sigue siendo el de los links.
- Un error de DNS al cambiar la IP pública nunca hace fallar el `PATCH` (§9).
- **Auditoría:** `settings.update` con las claves y sus valores (`cloudflare_origin_ca` como bool). El token solo aparece como `"set"` o `"removed"`. Si después muelle vuelve a los puertos anteriores (§3.5), va otro evento, `settings.ports_rollback`, del actor `system`.

**Recuperar el acceso** después de guardar una lista que te deja fuera, desde el servidor: `muelle settings reset-panel-ips` (§5.4), con `docker exec` en el contenedor de muelle, deja la lista en `[]` y reinicia muelle, que al arrancar le manda a Caddy la configuración sin la lista:

```bash
docker exec "$(docker ps -q -f label=com.docker.swarm.service.name=muelle_muelle | head -n1)" muelle settings reset-panel-ips
```

**Si muelle-caddy quedó sin arrancar por los puertos** y muelle no volvió solo a los anteriores (§3.5: no había un muelle-caddy sirviendo, o los anteriores también estaban tomados), sin Caddy no se llega al panel. Los puertos se corrigen directo en la base. La imagen de muelle no trae `sqlite3`, así que se usa un contenedor aparte sobre el mismo volumen (SQLite en modo WAL admite la escritura con muelle corriendo). En el ejemplo, `80` y `443` se cambian por los anteriores u otros libres:

```bash
docker run --rm -v muelle_data:/data alpine sh -c \
  "apk add -q sqlite && sqlite3 /data/muelle.db \"UPDATE settings SET value='80' WHERE key='proxy_http_port'; UPDATE settings SET value='443' WHERE key='proxy_https_port'\""
```

Después se reinicia muelle (`docker service update --force muelle_muelle`), o se espera al job `"proxy"` (a lo más 5 min), que recrea el contenedor con esos puertos.

### 4.2 `GET /audit?limit=N&before=<id>` → 200 · sesión

Los eventos de la auditoría, del más nuevo al más viejo, paginados por id: `limit` va de 1 a 500 (100 por defecto) y `before` es el id más chico ya visto, para la página siguiente. Fuera de rango, o algo que no es un número, es `422` en ese campo. No dice si hay más: una página llena puede tener otra detrás. Se conservan 180 días (§5.2 del diseño).

```json
[
  {
    "id": 812,
    "actor_type": "token",
    "actor_id": 5,
    "actor_name": "github-actions",
    "action": "env.replace",
    "target": "stage:auditfy/staging",
    "details": {"keys": ["DATABASE_URL"]},
    "at": "2026-10-16T20:41:07.12Z"
  }
]
```

| Campo | Qué |
|---|---|
| `actor_type` | `user`, `token` o `system`: los jobs, el arranque, la CLI y lo que provoca alguien sin autenticarse (una contraseña incorrecta en el login; un código TOTP incorrecto ya es del usuario) |
| `actor_id` | El id del usuario o del token; null con `system` |
| `actor_name` | El username, el nombre del token o `"system"`, **copiado al escribir el evento**: sigue aunque se borren el usuario o el token, como el `actor_name` de un deploy (§2.1). Nunca es null. Es `""` solo en un evento de antes de la migración 0004 cuyo autor ya no existía al migrar y no dejó su nombre (un token borrado, o un usuario borrado sin su `auth.login` ni su `user.create` en la auditoría); el panel muestra entonces el tipo y el id |
| `details` | Un objeto JSON (`{}` si no hay). Nunca trae valores secretos |
| `at` | RFC 3339 (§0) |

**Quien audita no pasa el nombre:** `audit.Recorder.Record(ctx, action, target, details)` lo copia del Principal del ctx (`Principal.ActorName`), del mismo del que sale el actor. Así lo reciben todos los módulos, también los eventos del deploy (Fase 2: `deploy.create` y `registry.*`), sin cambiar una línea. Un Principal armado a mano (el de `auth.login` y el de `user.create`) trae `Username`.

## 5. Fase 5: install, imágenes y el panel embebido

### 5.1 Nombres fijos (`shared/domain/naming`)

| Qué | Nombre |
|---|---|
| Imagen de muelle | `ghcr.io/cpu-commits/muelle:<versión>` (`naming.MuelleImageFor`). Las versiones finales (sin `-sufijo`) también se publican como `:latest`, la que usa `docker run … ghcr.io/cpu-commits/muelle install` sin tag |
| Imagen de Caddy | `ghcr.io/cpu-commits/muelle-caddy:<versión>` (`naming.CaddyImageFor`). Se publica con el mismo tag que la de muelle, sin `:latest` |
| Versión | `vMAYOR.MENOR.PARCHE[-sufijo]`. Se fija al compilar: `go build -ldflags "-X main.version=v0.1.0"`, y la imprime `muelle version`. `dev` no tiene imagen de Caddy por defecto ni instala sin `--image` |
| Stack y servicio | `muelle` y `muelle_muelle` |
| Red | `muelle_panel`: overlay attachable, con subred privada. install verifica con `Network.Subnets` que sea privada (RFC 1918 o ULA, `netip.Addr.IsPrivate`) |
| Volúmenes | `muelle_data`, `muelle_caddy_data`, `muelle_caddy_config` y `muelle_caddy_admin` |
| Secretos Swarm | `muelle_master_key` (la clave maestra), y los opcionales `muelle_init_cloudflare_token` y `muelle_init_registry` (`naming.InitRegistrySecret`, la credencial de las imágenes privadas, §5.2) |
| Contenedores | `muelle-caddy`, que crea ingress en el primer arranque, no install; y `muelle-install-probe` (`naming.InstallProbeContainer`), el de la prueba de puertos, que install borra siempre |
| Etiqueta de Swarm | `com.docker.swarm.service.name` (`naming.LabelSwarmService`): la pone Swarm en cada contenedor de una tarea. `muelle settings reset-panel-ips` la usa para saber a qué servicio pertenece (§5.4) |

install acepta tres flags ocultos, solo para los tests de integración (no salen en la ayuda):
- `--name-prefix <p>` cambia el `muelle` de todos estos nombres: `<p>` (stack), `<p>_muelle`, `<p>_panel`, `<p>_data`, `<p>_caddy_admin`, `<p>_master_key`, `<p>_init_cloudflare_token`, `<p>_init_registry`, `<p>-install-probe` y `<p>-caddy` (el contenedor que mira `--reinstall`). Los arma `naming.InstallNamesFor(p)`; con `muelle` da los nombres fijos de arriba. Con otro prefijo, muelle-caddy, que ingress crea con los nombres fijos, no comparte la red ni el socket con ese muelle.
- `--image <ref>`: la imagen de muelle que se despliega y con la que se prueba los puertos (§5.4), en vez de `naming.MuelleImageFor(version)`. Así el test usa una imagen construida en el nodo.
- `--panel-origin <origen>`: escribe `MUELLE_PANEL_ORIGIN` en vez del `MUELLE_PANEL_DOMAIN` de `--panel-domain` (con él, `--panel-domain` no hace falta y, si viene, no se escribe). El test pasa una IP (`https://127.0.0.1:<puerto>`): sin `panel_domain`, `ProxySettings.Enabled` es falso e ingress queda `disabled` (§3.3), así el muelle del test nunca crea muelle-caddy ni la red `muelle_panel`, que tienen nombres fijos. El test lo verifica: si alguno de los dos aparece durante el test, falla (no los borra).

### 5.2 La configuración inicial: `MUELLE_INIT_*`

install despliega el servicio con estas variables. `settings.Load` las valida en cada arranque, con las reglas de los flags de abajo (`{` o `}` en `MUELLE_INIT_ACME_EMAIL` o en el token tampoco pasan, §3.4): si una no sirve, muelle no arranca y el error nombra la variable.

**Se aplican una sola vez**, en el primer arranque de la base: `admin.ApplyStartupSettings` guarda en la misma transacción la marca `init_applied`, y en los arranques siguientes no mira `MUELLE_INIT_*`. En ese primer arranque guarda cada valor que no es cero si la clave todavía no está, y deja un evento `settings.init` con las claves, sin valores.

- **Por qué una marca, y no "si la clave falta":** las variables y el secreto del token quedan en el servicio. Sin la marca, lo que el admin borra en el panel volvería con cualquier reinicio (`docker service update --image`, un nodo que se reinicia). Borrar el token (`cloudflare_token: ""`) borra la clave, así que un token quitado, quizás por filtrado, volvería a usarse en Caddy y en el DNS.
- **Una reinstalación sobre el mismo volumen `muelle_data`** ya tuvo su primer arranque: los flags de install no cambian ningún ajuste. install lo avisa si el volumen existe ("la base ya existe: cambia los ajustes en el panel").

| Flag de install | Variable | Clave de settings |
|---|---|---|
| `--panel-domain D`, obligatorio salvo con `--panel-origin` (§5.1) | `MUELLE_PANEL_DOMAIN=D`, sin esquema ni puerto (no es `MUELLE_INIT_*`: vale en cada arranque) | `panel_domain` (en cada arranque; ver abajo) |
| `--email`, obligatorio | `MUELLE_INIT_ACME_EMAIL` | `acme_email` |
| `--public-ip` (o la detectada, §3.3 paso 2 del diseño) | `MUELLE_INIT_PUBLIC_IP` | `public_ip` |
| `--public-ipv6` (opcional) | `MUELLE_INIT_PUBLIC_IPV6` | `public_ipv6` |
| `--http-port` (por defecto 80) | `MUELLE_INIT_PROXY_HTTP_PORT`, siempre, también con 80 | `proxy_http_port` |
| `--https-port` (por defecto 443) | `MUELLE_INIT_PROXY_HTTPS_PORT`, siempre, también con 443 | `proxy_https_port` |
| `--cloudflare-token` (`-` lo lee de la primera línea de stdin, con `docker run -i`) | El secreto `muelle_init_cloudflare_token` y `MUELLE_INIT_CLOUDFLARE_TOKEN_FILE=/run/secrets/muelle_init_cloudflare_token` | `cloudflare_token` (cifrado) |
| `--registry-user` y `--registry-password-stdin` (la contraseña o el token, de la primera línea de stdin), o el `config.json` de Docker montado en `/root/.docker/config.json` | El secreto `muelle_init_registry` y `MUELLE_INIT_REGISTRY_FILE=/run/secrets/muelle_init_registry`, solo si hay una credencial para el host de la imagen de muelle | Un registry (tabla `registries`, §2.10), que guarda deploy solo en el primer arranque de la base |
| `--caddy-image` (avanzado) | `MUELLE_INIT_CADDY_IMAGE` | `caddy_image` |
| `--advertise-addr` | — (solo `docker swarm init`; con el Swarm ya activo se ignora, con un aviso) | — |
| `--reinstall` | — (ver §5.4) | — |

Los flags se validan antes de tocar Docker, con las mismas reglas que `PATCH /settings` (§4.1): `validate.Hostname`, `Email`, `IPv4`, `IPv6`, `Port` e `ImageRef`; los dos puertos distintos; y el token con la forma de un token de Cloudflare (de 20 a 512 caracteres visibles, sin espacios). `--email` y el token, además, sin `{` ni `}` (§3.4). Uno que no sirve termina con código 2 y el nombre del flag.

**El origen del panel no va en la configuración de muelle:** install escribe el dominio (`MUELLE_PANEL_DOMAIN`) y el puerto HTTPS inicial (`MUELLE_INIT_PROXY_HTTPS_PORT`). El origen sale de los ajustes, `https://<panel_domain>[:<proxy_https_port>]`: si después se cambia `proxy_https_port` en el panel (la migración de §16 del diseño pasa de 8443 a 443), el `Origin` permitido y los links pasan al puerto nuevo, sin reiniciar muelle (§0.1).

`MUELLE_PANEL_ORIGIN` es opcional, para un origen más que install no conoce (el panel detrás de otro proxy). Se agrega a mano, lo que reinicia muelle, y pasa a ser también el origen de los links:

```bash
docker service update --env-add MUELLE_PANEL_ORIGIN=https://panel.otro-proxy.cl muelle_muelle
```

Una instalación anterior a `MUELLE_PANEL_DOMAIN` tiene `MUELLE_PANEL_ORIGIN` con el puerto de entonces, y ese origen sigue pasando aunque el puerto cambie. Para que siga a los ajustes: `docker service update --env-rm MUELLE_PANEL_ORIGIN --env-add MUELLE_PANEL_DOMAIN=panel.tudominio.cl muelle_muelle`.

**Fijas:**
- `MUELLE_LISTEN=:8080`
- `MUELLE_DATA_DIR=/data`
- `MUELLE_MASTER_KEY_FILE=/run/secrets/muelle_master_key`
- `MUELLE_SECURE_COOKIES` sin definir (`true`)

**El token:** nunca va en una variable de entorno (se vería en `docker inspect`), sino en un secreto Swarm. Lo único que se pasa es la ruta del archivo. Después del primer arranque, el secreto no se vuelve a leer como ajuste: cambiar o quitar el token en el panel es definitivo. install **no** quita el secreto del servicio: hacerlo reinicia muelle, y ese arranque anularía el link de setup que install acaba de imprimir (`EnsureSetupInvite`). Quitarlo después es opcional, con un solo `docker service update` que saque también `MUELLE_INIT_CLOUDFLARE_TOKEN_FILE`, porque `settings.Load` lee el archivo en cada arranque y, si falta, muelle no arranca (el README lo documenta):

```bash
docker service update --secret-rm muelle_init_cloudflare_token --env-rm MUELLE_INIT_CLOUDFLARE_TOKEN_FILE muelle_muelle
docker secret rm muelle_init_cloudflare_token
```

**La credencial del registry** (las imágenes de muelle son privadas): install baja las imágenes de muelle y de Caddy con ella (§5.4 paso 5) y se la pasa a muelle para que ingress pueda bajar la Caddy de la versión siguiente y las apps puedan usar imágenes del mismo registry.
- Viene de `--registry-user` y `--registry-password-stdin` (la contraseña nunca va en un flag ni en una variable), y si no, del `config.json` de Docker del host montado en solo lectura: `auths[<host>].auth`, la base64 de `usuario:contraseña`. Si `docker login` la guardó en un helper (`credHelpers[<host>]`, o `credsStore` con una entrada vacía de ese host), el helper no se puede correr dentro del contenedor de install: error que pide los flags. `--registry-password-stdin` y `--cloudflare-token -` no se pueden juntar (los dos leen stdin).
- El host es el de la imagen de muelle, como lo da `reference.Domain` (`ghcr.io`). Los flags dan solo la credencial de ese host; del `config.json` sale la de cada host (el de la imagen de Caddy puede ser otro).
- Se valida con las reglas de `POST /registries` (§2.10): usuario de 1 a 255 caracteres y contraseña de 1 a 4096 bytes, los dos sin caracteres de control. Ningún error ni salida de install cita la contraseña.
- El secreto `muelle_init_registry` es un JSON `{"host": "ghcr.io", "username": "…", "password": "…"}`, sin otros campos; `settings.Load` lo valida en cada arranque y deploy lo guarda en el primer arranque de la base (§2.10). Como el del token, install no lo quita del servicio, y `docker stack deploy` recibe la misma credencial (`StackDeploy.Registries`, el `--with-registry-auth`) para que Swarm pueda bajar la imagen al recrear la tarea. Quitarlo después es opcional:

```bash
docker service update --secret-rm muelle_init_registry --env-rm MUELLE_INIT_REGISTRY_FILE muelle_muelle
docker secret rm muelle_init_registry
```

### 5.3 El stack de muelle

install lo despliega con `DeployStack`. No pasa por el render de apps: puede montar `docker.sock` y `muelle_master_key`, que las apps no pueden (la lista de §6.3 del diseño: sin bind mounts, sin `driver_opts`, sin `secrets:` ni `configs:`).

```yaml
services:
  muelle:
    image: ghcr.io/cpu-commits/muelle:v0.1.0
    environment:
      MUELLE_PANEL_DOMAIN: panel.tudominio.cl
      MUELLE_LISTEN: ":8080"
      MUELLE_DATA_DIR: /data
      MUELLE_MASTER_KEY_FILE: /run/secrets/muelle_master_key
      MUELLE_INIT_ACME_EMAIL: tu@correo.cl
      MUELLE_INIT_PROXY_HTTP_PORT: "80"
      MUELLE_INIT_PROXY_HTTPS_PORT: "443"
      # MUELLE_INIT_PUBLIC_IP, MUELLE_INIT_PUBLIC_IPV6, MUELLE_INIT_CADDY_IMAGE,
      # MUELLE_INIT_CLOUDFLARE_TOKEN_FILE y MUELLE_INIT_REGISTRY_FILE, solo si tienen valor
    volumes:
      - { type: bind, source: /var/run/docker.sock, target: /var/run/docker.sock }
      - { type: volume, source: data, target: /data }
      - { type: volume, source: caddy_admin, target: /run/caddy-admin }
    secrets:
      - { source: master_key, target: muelle_master_key }
      # - { source: init_cloudflare_token, target: muelle_init_cloudflare_token }, si hay token
      # - { source: init_registry, target: muelle_init_registry }, si hay una credencial
    networks: [panel]
    logging: { driver: json-file, options: { max-size: 10m, max-file: "3" } }
    deploy:
      replicas: 1
      placement: { constraints: ["node.role == manager", "node.id == <el nodo donde corrió install>"] }
      update_config: { order: stop-first, failure_action: rollback, monitor: 30s }
      rollback_config: { order: stop-first }
volumes:
  data: { name: muelle_data }
  caddy_admin: { name: muelle_caddy_admin }
secrets:
  master_key: { external: true, name: muelle_master_key }
  # init_cloudflare_token: { external: true, name: muelle_init_cloudflare_token }, si hay token
  # init_registry: { external: true, name: muelle_init_registry }, si hay una credencial
networks:
  panel: { external: true, name: muelle_panel }
```

- **Formato:** install manda el compose como JSON (YAML es un superconjunto de JSON), armado con structs y `encoding/json`, sin plantillas de texto. Como `docker stack deploy` interpola `${VAR}` con su propio entorno, todo `$` del documento va como `$$` (`StackDeploy.Compose`).
- **Las claves cortas con `name:`** (`data`, `master_key`, `panel`) dan los mismos recursos que los nombres fijos de §5.1, y permiten el prefijo de los tests. El secreto se monta con `target: muelle_master_key`, así `MUELLE_MASTER_KEY_FILE` no cambia.
- **`node.id`:** `muelle_data` es un volumen local. Si se suma otro manager, sin esta restricción Swarm podría mover muelle a ese nodo, donde el volumen está vacío: muelle arrancaría con otra base y otro link de setup.
- **`stop-first`** en la actualización y en el rollback: dos tareas de muelle a la vez escribirían la misma base y correrían los mismos workers. `failure_action: rollback` con `monitor: 30s` vuelve a la imagen anterior si la nueva se cae al arrancar.
- **Sin `hostname`:** el hostname del contenedor queda como su id, y `muelle settings reset-panel-ips` lo usa para encontrarse (§5.4).
- **Logs acotados** con `json-file` (como las apps, §6.2 del diseño). install los lee con `TaskLogs`.

### 5.4 Pasos de install que dependen de este contrato

**Orden.** install no cambia nada hasta saber que va a poder terminar, así un puerto ocupado aborta sin dejar nada:

1. Valida los flags (código 2 si no sirven).
2. Docker (`Info`): si no responde, aborta y sugiere montar `/var/run/docker.sock`. Un nodo worker, o un Swarm `pending`, `error` o `locked`, aborta. Un Swarm `inactive` se inicia en el paso 7.
3. Una instalación anterior, y la red `muelle_panel` si ya existe (abajo).
4. La IP pública, si no vino `--public-ip` (§3.3 paso 2 del diseño): la línea `ip=` de `https://1.1.1.1/cdn-cgi/trace`, con un plazo de 5 s. Solo IPv4: una respuesta que no es una IPv4 canónica cuenta como no detectada (la IPv6 va solo con `--public-ipv6`). Si no se puede detectar, es un aviso y la variable no va.
5. Las imágenes de muelle y de Caddy (`--caddy-image`, o `naming.CaddyImageFor(version)`; ninguna con `dev`): `InspectImage` y, si da `ErrNotFound`, `PullImage` con la credencial de su host (§5.2), o anónima si no hay. Si el pull falla sin credencial, o con un `denied`/`unauthorized`, el error dice qué hacer: `docker login ghcr.io` con un token de GitHub con `read:packages` y montar `~/.docker/config.json`, o `--registry-user` y `--registry-password-stdin`. La imagen y las credenciales se resuelven antes del paso 2 (no tocan Docker).
6. La prueba de puertos (abajo).
7. `InitSwarm(--advertise-addr)` solo si el Swarm está `inactive`. Su id de nodo (o el de `Info.NodeID`) va en la restricción `node.id` de §5.3. Si falla sin `--advertise-addr` (Docker no elige entre varias IPs), el error lo sugiere.
8. La red, los secretos y el stack. Si algo falla desde que crea lo primero hasta que `DeployStack` funciona (las subredes de la red, un secreto, el secreto del token o del registry que quedó y no se pudo borrar, o `DeployStack`), install borra lo que creó en esta corrida (el stack, si el CLI alcanzó a crear el servicio; los volúmenes `muelle_data` y `muelle_caddy_admin` que no existían antes, si la tarea alcanzó a arrancar y crearlos, una vez que el servicio ya no existe; los secretos y la red; no el Swarm) y no imprime la clave: nunca queda un `muelle_master_key` cuya clave nadie vio, ni un `muelle_data` con una base cifrada con ella. Lo que no pudo borrar lo nombra en stderr, con el comando para borrarlo.
9. La clave maestra, impresa una sola vez, y después el link de setup.

**Salida:** la clave maestra y el link de setup (o el aviso de que muelle ya tiene usuarios) van a stdout; el avance y los avisos, a stderr. Así `docker run … install > install.txt` guarda solo lo que hay que conservar. Código 0 si terminó con el link (o con usuarios), 1 si algo falló (también el plazo del link), 2 si un flag no sirve. Ctrl-C cancela; lo que se crea en esta corrida antes de que `DeployStack` funcione se borra igual, como si `DeployStack` fallara.

**Una instalación anterior** (con el Swarm `inactive` no hay servicios ni secretos, y solo se mira el volumen):
- Si el servicio `muelle_muelle` existe, aborta siempre: muelle ya está instalado, y se actualiza con `docker service update --with-registry-auth --image` (abajo).
- Sin `--reinstall`, si existe el secreto `muelle_master_key` o el volumen `muelle_data`, aborta: una clave nueva no descifraría esa base (`clave maestra incorrecta`). El mensaje ofrece `--reinstall` o borrar el secreto y el volumen (se pierden los datos).
- Con `--reinstall` (para volver a desplegar después de un `docker stack rm muelle`, cuando su contenedor ya terminó: `docker stack rm` vuelve antes, y si no se espera habría dos muelle sobre la misma base), el secreto `muelle_master_key` tiene que existir: se reusa y no se imprime ninguna clave. Si viene `--cloudflare-token` y el secreto del token existe, se borra y se crea de nuevo (también sin `--reinstall`: el que quedó de una corrida cortada); si no viene, el secreto que quedó no se toca y el stack no lo monta. Lo mismo con `muelle_init_registry` y la credencial del registry. Si el volumen `muelle_data` existe, install avisa que los ajustes de install no se aplican sobre una base que ya arrancó (§5.2).

**Puertos ocupados (§3.3 paso 3 del diseño):** install corre dentro de un contenedor, así que no puede probar el puerto con `net.Listen`.
- Crea el contenedor temporal `muelle-install-probe`, con el comando `help`, que publica los mismos puertos que muelle-caddy: `--http-port`/tcp, `--https-port`/tcp y `--https-port`/udp (HTTP/3), y lo arranca. Uno que quedó de una corrida cortada se borra antes.
- Su imagen es la de muelle que se va a desplegar: `naming.MuelleImageFor(version)` (la de la versión de install), o `--image` (oculto, para los tests). Quien corre install pudo usar otro tag (`:latest`, o ninguno), así que esa imagen puede no estar en el nodo: se baja en el paso 5. Un binario sin versión publicada (`dev`) no instala sin `--image`.
- `docker.ErrPortInUse` es "ocupado": se aborta con la sugerencia de `--http-port`/`--https-port`. Si existe `muelle-caddy` (de una instalación anterior: `docker stack rm muelle` no lo borra), el mensaje dice que es él quien tiene los puertos y cómo borrarlo (`docker rm -f muelle-caddy`).
- El contenedor temporal se borra siempre, también si install se corta (con un ctx sin cancelación y un plazo propio).
- Con `--reinstall` y un `muelle-caddy` que ya existe, la prueba no se hace: ese contenedor tiene tomados los puertos, y ingress lo recrea si cambian.
- Sin `--cloudflare-token` y con `--http-port` distinto de 80, install avisa que Let's Encrypt valida por HTTP-01 en el puerto 80 público: solo funciona si el 80 del router llega a ese puerto. Es un aviso, no un error, por ese caso.

**La red:** si `muelle_panel` ya existe, se usa si es overlay, attachable y de scope `swarm`; si no, aborta. Eso se revisa en el paso 3 (`InspectNetwork` funciona con el Swarm inactivo), así una red local con ese nombre aborta antes de iniciar el Swarm. Sus `Subnets` tienen que ser privadas (paso 8). Una red recién creada puede venir sin `Subnets` unos milisegundos (`docker.Networks`): se reintenta hasta 5 s.

**Clave maestra:** `crypto.GenerateKey()`, que se guarda con `CreateSecret(naming.MasterKeySecret, …)` y se imprime **una sola vez**, después de que `DeployStack` funcionó.

**El link de setup (§3.3 paso 10 del diseño):**
- install busca la línea JSON con `"event":"setup_url"` y lee su `url`. La escribe `logSetupURL` en `main.go`; el mensaje puede cambiar, `event` y `url` no.
- La `url` es siempre absoluta (§0.1). Si no hay origen del panel (sin `MUELLE_PANEL_ORIGIN`, y los ajustes no se pudieron leer), serve no escribe un link relativo: termina con un error (una línea `ERROR`, sin el token), y Swarm reintenta la tarea, que arma un link nuevo.
- **Solo vale la de la tarea que corre ahora.** Cada arranque sin usuarios invalida el link anterior (`EnsureSetupInvite`). Si la primera tarea se reinició, los logs del servicio traen varias líneas `setup_url`, y solo la última sirve. Por eso install lee los de la tarea actual (`ListTasks` del servicio: la de `desired_state: running` y `state: running`, la más nueva) con `TaskLogs`, desde el principio y con `Follow`. Si esa tarea cambia durante el plazo, vuelve a buscar la actual y su línea.
- **`"event":"listening"`:** serve la escribe cuando empieza a atender (`muelle escuchando`), siempre después de la de `setup_url`. Si llega sin un `setup_url` antes en la misma tarea, muelle ya tiene usuarios: install lo dice y termina bien. Trae `panel`, el origen del panel de muelle (§0.1; no va si no hay), y con usuarios install dice que se entre ahí: sale de los ajustes de la base, así que con `--reinstall` vale aunque `--https-port` sea otro (solo se aplica en el primer arranque). Sin `panel` (un muelle anterior), install lo arma con sus flags.
- Una línea de nivel `ERROR` de esa tarea (muelle no arrancó: una variable que no sirve, la clave) se muestra en stderr mientras se espera: Swarm reintenta la tarea, y el error explica por qué no llega el link.
- **Sin una tarea corriendo:** muelle que sale enseguida (la clave, una variable) deja la tarea `failed` en milisegundos, y Swarm la reintenta cada ~5 s; una tarea `rejected` (el bind de `/var/run/docker.sock` no existe, con Docker rootless) o `pending` sin un nodo posible (la restricción no se cumple: "no suitable node") nunca corre, y el motivo está solo en su `Err`. Por eso, mientras ninguna corre, install toma la tarea más nueva del servicio (con cualquier `desired_state`), muestra en stderr su estado y su `Err` (`aviso: …`) y, si tuvo contenedor y terminó, las líneas `ERROR` de su log, una vez por tarea.
- El plazo es de 2 min. Si se pasa, el stack queda desplegado e install termina con código 1 y sugiere `docker service logs muelle_muelle` y `docker service ps --no-trunc muelle_muelle`.

**Actualizar:** `docker service update --with-registry-auth --image ghcr.io/cpu-commits/muelle:<v> muelle_muelle`, con `docker login ghcr.io` hecho en el servidor (las imágenes son privadas: `--with-registry-auth` le pasa a Swarm la credencial para bajar la nueva).
- La imagen de Caddy de la versión nueva la baja ingress con el registry `ghcr.io` que guardó install (§5.2).
- Con `caddy_image` vacío, el job `"proxy"` del arranque recrea muelle-caddy con la imagen de la versión nueva, con unos segundos de corte.
- Con `caddy_image` fijo (solo se fija al instalar, con `--caddy-image`), Caddy no cambia.

**`muelle settings reset-panel-ips`** ("Recuperar el acceso", §4.1) corre **dentro** del contenedor de muelle, con su entorno:

```bash
docker exec "$(docker ps -q -f label=com.docker.swarm.service.name=muelle_muelle | head -n1)" muelle settings reset-panel-ips
```

- Abre la base de `MUELLE_DATA_DIR` (sin migrar; si no existe, aborta y dice que corra con `docker exec`) y deja `panel_allowed_cidrs` en `[]` con `admin.ResetPanelAllowedCIDRs`, en una transacción, con un evento `settings.update` de actor `system` y details `{"panel_allowed_cidrs": [], "via": "cli"}`. Si la lista ya estaba vacía no escribe nada y lo dice (el problema es otro, quizás los puertos del proxy).
- Después reinicia su propio servicio, como `docker service update --force`: busca su contenedor por el hostname (`os.Hostname`, que es el id) y lee `com.docker.swarm.service.name`, y llama a `ForceUpdateService` (`docker.ServiceRestarter`, Fase 5). Al arrancar, el job `"proxy"` le manda a Caddy la configuración sin la lista. Reinicia también si la lista ya estaba vacía: ese arranque le reenvía la configuración a Caddy, que es lo que sirve si el problema es otro. Con `--no-restart` solo cambia la base e imprime el comando para reiniciar a mano.
- Si no puede reiniciar (no encuentra su contenedor, el contenedor no tiene la etiqueta, o Docker falla), la base ya quedó cambiada: lo dice, imprime el comando para reiniciar a mano y termina con código 1.
- Códigos: 0 si terminó bien, 1 si algo falló, 2 si los argumentos no sirven.
- El reinicio corta unos segundos el panel y los streams. Un deploy en curso se resuelve con la reconciliación (§1.2).

**Imágenes:**
- `Dockerfile` en la raíz: una etapa `node` + `pnpm` que construye `web/`; una etapa `golang` que copia `web/build` en `src/cmd/http/panel/build` y compila con `CGO_ENABLED=0`, `-tags panel` y `-ldflags "-s -w -X main.version=<v>"`; y un `alpine` con el binario, el CLI de `docker` (de la imagen `docker:<v>-cli`) para `stack deploy`, `ca-certificates` y el usuario root (necesita `docker.sock`). `ENTRYPOINT ["muelle"]`, `CMD ["serve"]`.
- Alpine trae un `/tmp` escribible con permisos `1777` (`os.TempDir()`): `DeployStack` crea ahí el `DOCKER_CONFIG` temporal y el directorio de trabajo del CLI. Una imagen `scratch` o distroless no lo trae, y tampoco tiene el `cat` con que se relee la clave maestra (§12 del diseño).
- `deploy/caddy/Dockerfile`: `xcaddy` (la imagen `caddy:<v>-builder-alpine`) con `github.com/caddy-dns/cloudflare`, en versiones fijas, sobre `caddy:<v>-alpine`. No necesita un CMD ni un archivo de configuración propios: el comando y el entorno los pone ingress (§3.4).
- Las imágenes base van fijadas por tag y digest. Se publican para `linux/amd64`.
- **Publicación:** `.github/workflows/release.yml` construye y sube las dos imágenes al empujar un tag `vX.Y.Z[-sufijo]` (el mismo formato que `naming.MuelleImageFor`), después de los tests. El tag de la imagen es el del git; `:latest` solo para las versiones finales de muelle, y nunca para muelle-caddy. Con `workflow_dispatch`, construye sin subir (para probar los `Dockerfile`).
- **Cookie:** el prefijo `__Host-muelle_session` con cookies `Secure` no es de la Fase 5 (los archivos son de identity y de `cmd/http/utils`): queda en las notas del README para la integración.

### 5.5 El panel embebido

- **Compilación:** el build de `web/` (`pnpm build` → `web/build`) se copia en `src/cmd/http/panel/build/` (ignorado por git) y se compila con `-tags panel`, que activa `//go:embed all:build` (sin `all:` se pierde `_app/`). Sin el tag, `go build ./...` y `go test ./...` funcionan sin el panel, y toda ruta del panel responde `404` con una página corta que dice que el binario no lo trae (con `Content-Security-Policy: default-src 'none'`, `nosniff` y `no-cache`).
- **Al arrancar** se leen del embed `200.html` y `csp-header.txt`. Si falta alguno, o la CSP está vacía o no es una sola línea, muelle no arranca.
- **Qué rutas:** todo lo que no es `/api` ni `/api/…` y no tiene una ruta registrada. `/api/v1/…` sin ruta sigue siendo `404 no_route` (§0.2), y una ruta registrada con otro método, `405 method_not_allowed` del mux (fuera de `/api` no hay ninguna). El `Origin` se revisa antes, como en toda petición.
- **Qué responde:** el archivo del build si existe con ese path (el path se limpia antes, y un directorio cuenta como que no existe); si no, `200.html` con `200`, que resuelve el router del panel. `/` es `200.html`. Excepciones: un `/_app/…` que no existe responde `404` en texto (un chunk viejo nunca recibe HTML), y `csp-header.txt` no se sirve (responde `200.html`). Solo `GET` y `HEAD` (con `Range` y las condiciones de `http.ServeContent`); otro método es `405 method_not_allowed`, en JSON, con `Allow: GET, HEAD`.
- **ETag:** los archivos fuera de `_app/immutable/` (el `200.html`, `_app/version.json`, el favicon) llevan un `ETag` fuerte con el sha256 de su contenido, calculado al arrancar, así el `no-cache` revalida con `304`. Los de `_app/immutable/` no lo necesitan, y no se leen enteros al arrancar: una página del embed que no se lee no suma RSS (§1 del diseño).
- **Headers** en todas las respuestas del panel (también el `404` y el `405`):
  - `Content-Security-Policy`: el contenido de `csp-header.txt` (la política del `<meta>` más `frame-ancestors 'none'`);
  - `X-Content-Type-Options: nosniff`, con el `Content-Type` de una tabla propia (`.js` es `text/javascript; charset=utf-8`, `.woff2` es `font/woff2`…), sin depender de `/etc/mime.types`;
  - `Referrer-Policy: same-origin`: los links con secretos (`/setup?token=…`, `/invite/<token>`) nunca salen del origen. No es `no-referrer` porque con esa política el navegador manda `Origin: null` en un `POST` que no es `fetch` (un `<form>` nativo), y muelle lo rechazaría con `403 bad_origin`;
  - `Cache-Control: public, max-age=31536000, immutable` en `/_app/immutable/…`, y `no-cache` en todo lo demás (el `200.html`, `_app/version.json`, el favicon).
- **Dónde se monta:** `cmd/http.Config.Panel` (un `http.Handler`; nil deja `no_route` como antes) y una línea en `container.go`. Es lo único que la Fase 5 toca de `cmd/http`.

## 6. Migraciones reservadas

Mientras no haya una versión publicada, `0002_deploy.sql` y `0003_ingress.sql` se pueden editar. Las fases no agregan migraciones nuevas: cambian estas, con su test de esquema, sin tocar las columnas congeladas de §1.3.

- **0002** (deploy):
  - `deploys`: `actor_name`, `superseded_by`, `config_images`, `requested_images` (§2.2) y `log_truncated` (§2.5) además de las columnas de §5.1, con índices por app y por estado activo;
  - `registries`: con `created_at` y `updated_at`.
- **0003** (ingress): `domains`, con:
  - `cf_record_id_aaaa`, `last_error_code`, `created_at` y `updated_at`;
  - `app_id` con `ON DELETE RESTRICT`: catalog no borra una app con dominios, `409 not_empty`.
- **0004** (Fase 1, un pendiente de su revisión final): `audit_events.actor_name` (§4.2), con los eventos que ya había llenados con lo que se sabe de su autor. Es una migración nueva, y no un cambio a `0001_init.sql` (que no se edita, §1.3), para que corra también sobre una base de la Fase 1.
- **0005** (lo pidió el panel): `deploys.routes_warning` (§2.1), 0 o 1. Los deploys que ya había quedan con 1 si son `succeeded` y su log trae la advertencia del paso 7 (§2.11). La migración solo tiene el texto, así que se equivoca en dos casos, los dos raros: uno anterior a la 0005 cuya advertencia no entró en el log (el tope de 256 KB, §2.5) queda con 0, y uno que muelle retomó después de reiniciarse (§1.2), cuyo paso 7 falló la primera vez y anduvo la segunda, queda con 1 aunque Caddy ya tenga sus rutas (su log trae la advertencia y después `rutas de Caddy actualizadas`). Los deploys nuevos guardan el resultado del paso 7 (§2.1). Es nueva, y no un cambio a `0002_deploy.sql`, por lo mismo que la 0004: para que corra también sobre una base que ya tiene deploys.
- **0006** (los certificados de Cloudflare, §3.7): `origin_ca_certs` (los certificados de Origin CA que emitió muelle, con la clave cifrada con la AAD `origin_ca_certs:private_key:<id>` y su estado, `active` o `retired`) y `origin_ca_hosts` (lo que vio en Cloudflare de cada hostname, y la última falla con su reintento). Es nueva, y no un cambio a `0003_ingress.sql`, por lo mismo que la 0004: para que corra también sobre una base que ya tiene dominios. La próxima migración nueva es la 0007.

## 7. Diferencias con lo que supuso el panel

| Tema | El panel supuso | Quedó | Por qué |
|---|---|---|---|
| Estado y logs | Fase 2 | Fase 2 (§17 del diseño decía 3) | Mismo cliente de Docker que el deploy |
| `actor_name` | Null si el actor ya no existe | Siempre string: se copia al crear el deploy | No se pierde al borrar el usuario o el token |
| Deploy | — | Campos nuevos `actor_id` y `superseded_by` | La Action informa el deploy que lo reemplazó (§14) |
| Advertencia de Caddy del paso 7 | El panel la reconoce por el texto de la línea del log (`advertencia: no se pudo actualizar Caddy`) | El deploy suma `routes_warning` (bool, §2.1), `true` cuando el paso 7 no pudo actualizar las rutas de Caddy (la advertencia va al log si entra) | El texto del log no es un contrato (§2.11) |
| Qué servicios fija un rollback | El panel lo adivina comparando la ref y el digest de los deploys | El deploy suma `requested_images` (servicio → ref pedida, §2.1): un servicio cuya ref pedida trae `@sha256:` está fijo en ese digest | Sin adivinar: comparar daba un falso positivo cuando la configuración o un override volvía al mismo digest, y perdía el digest fijo cuando el rollback salía del historial de 22 |
| `images` en cola | Todos los servicios, sin digest | Solo los overrides del cuerpo; todos con digest desde el paso 2 | La configuración se lee al correr, no al encolar |
| `POST …/deploys` | — | La respuesta suma `status: "queued"` y el header `Location` | Compatible |
| `chunk` del log | Puede cortar una línea | Siempre líneas completas | Más simple; el manejo del panel sigue sirviendo |
| Log por polling | — | Suma `superseded_by` | Para la Action |
| Log por polling: `offset` más allá del final | No es error: `chunk: ""` y `next_offset` = el final | `422 invalid` con `offset: …` (también en el SSE, antes de abrir) | El log solo crece, así que ese offset es un error del cliente. El panel no lo ve: solo manda los `next_offset` que recibió |
| SSE del deploy | `status` al conectar | Solo si no terminó. El `status` final va siempre después de la última línea | Si no, el panel cerraría antes de leer el log de un deploy que terminó justo |
| SSE de logs | — | Suma el evento `end`, y `follow=0` | El panel ignora eventos desconocidos |
| SSE de logs: `since` | Al reconectar manda `{service, since, follow: 1}`: RFC 3339 con ms, inclusivo, hasta las últimas 5000; `422` con `since: …` | Igual. Si vienen `since` y `tail`, manda `since` y `tail` no se mira (así sirve también un cliente que manda los dos); `time` va truncado al milisegundo. `follow` distinto de `0`/`1` es `422 follow` | Sin `since`, cada reconexión repetiría las últimas líneas |
| Servicios de una app | El formulario de un dominio pide el destino a mano | `GET` de una app (catalog) suma `services: [{name, ports}]` (§2.12) | Ofrecer los destinos sin esperar al primer deploy |
| Guardar un compose | Validación al guardar | `422 compose_yaml` con las reglas de §6.3 del diseño, y con un servicio que choca con otro del stage. Una app de formulario nueva con el slug de un servicio compose del stage: `422 slug` | Los servicios de un stage comparten la red (§5.2 regla 2) |
| Deploy o rollback de una app que se está borrando | — | `409 conflict` (a lo más 2 min) | No desplegar un stack que el `DELETE` acaba de borrar |
| `409 conflict` con varios sentidos | Pidió códigos propios (`app_removing`, `deploy_stopping`) | No en la v1: sigue `409 conflict`, y el panel lo distingue por el endpoint: en `POST …/deploys` y `…/rollback`, la app se está borrando (§2.2); en el `DELETE` de una app, un deploy que no se detiene (§1.1); en un `POST` que crea algo, ya existe | Los tres casos ya se distinguen por la ruta |
| Los motivos de un `422` y el `error` de `GET /proxy` | Pidió un motivo legible por máquina (la regla), para traducirlo | No en la v1: el `message` y el `error` siguen siendo oraciones en español (§0), que el panel muestra tal cual; el campo del `422` sí es un dato (§0.2) | Las reglas de §6.3 del diseño y los errores de Caddy y Docker no tienen una lista cerrada |
| Crear un stage con una red ajena del mismo nombre | El panel lo mapea a `StageNetworkConflict` desde el código | `409 network_conflict` (§0.2, §3.4), sin guardar el stage | El código ya existía en ingress; faltaba en el contrato |
| Deploy sin cuerpo después de un rollback | — | Repite las imágenes del rollback (con digest); para volver a seguir un tag, override o cambiar la configuración | El rollback no se deshace solo (§2.2) |
| SSE, los dos | — | La credencial se revisa cada 60 s; a lo más 8 por principal (`429`) y 1 h por stream | El panel ya trata un stream que se corta |
| Enmascarado | — | También cada línea de un secreto de varias líneas; nada por debajo de 8 bytes | Una clave PEM no se veía; un `1` tapaba texto ajeno |
| `GET …/status` | `cpu_percent` y `memory_bytes` por tarea | Por tarea, como lo lee el panel (null fuera del nodo local), y además su suma por servicio. Suma `image` y `update_state` | Compatible |
| `GET …/status`: caché | Las medidas, al menos 5 s | Las medidas, 5 s por app; el resto, hasta 2 s | Compatible. El estado de un deploy en curso no se atrasa 5 s |
| `GET …/deploys` | Los últimos 20 | Hasta 22: los 20 más nuevos, más el actual y su anterior si son más viejos | La retención nunca borra la versión que corre ni el destino del rollback |
| Rollback sin anterior | `409 no_previous_deploy` (o `conflict`) | `409 no_previous_deploy` | Igual |
| Rutas de un dominio | Bajo la app | Bajo la app | Igual |
| Hostname repetido | `409 conflict` o `hostname_taken` | `409 conflict` | El código genérico ya existe |
| Dominio | — | Suma `updated_at`; `records` es calculado; `cf_record_id_aaaa` no se expone | Compatible |
| `proxied` al crear | El panel manda `true` siempre | Opcional, por defecto `false` | Igual al default de la tabla |
| Adoptar | Pone el `proxied` del dominio | Conserva el `proxied` del registro y lo copia al dominio | §16: adoptar no cambia hacia dónde va el tráfico |
| `DELETE` de un dominio con Cloudflare caído | — | `502 cloudflare_unavailable` y no borra nada | No deja registros huérfanos |
| `target_service` en una app compose | Validado | Lo valida el render, no al guardar | ingress no parsea compose en la v1 |
| `GET /proxy` | No existe | Endpoint nuevo para el banner "proxy desincronizado" | §8 y §13 |
| `GET /proxy`: sin contenedor | — | `caddy.image: null` y `running: false`; `error` solo con `out_of_sync`; `out_of_sync` también al arrancar, hasta el primer `POST /load` | Compatible: el DTO del panel ya acepta `image` null |
| `proxied` fuera de Cloudflare | El mock lo deja en `false` | Igual: sin zona o sin token, `proxied: false` aunque el `POST` mande `true` | Sin registro que maneje muelle no tiene efecto |
| Adoptar un `CNAME` o varios registros | — | El `CNAME` pasa a ser un `A` a la IP pública; con varios `A`/`CNAME`, `error` + `cloudflare` y no se toca nada; sin token o sin `public_ip`, `error` + `cloudflare` (`200`) | Adoptar es "apuntar a muelle"; con varios no se adivina cuál |
| `DELETE` con Cloudflare que rechaza | — | `502 cloudflare_unavailable` también si Cloudflare rechaza el borrado (no solo si no responde); un `404` allá no es error | No deja registros huérfanos |
| `retry` de `certificate` | Verifica ahora | Si el certificado sigue sin ser válido, vuelve a `pending` con otros 10 min y fuerza la recarga de Caddy | Así Caddy vuelve a intentar la emisión |
| Verificación de certificados | A los 30 s y cada hora | También cada minuto mientras el dominio sigue `pending`, y la de cada hora mira los `error` + `certificate`; no cambia nada si no llega a Caddy | El panel ve `active` apenas Caddy emite, y un Caddy que arranca no deja los dominios en error |
| `adopt` y `retry` con cuerpo | Sin cuerpo | Vacío o `{}`; otra cosa es `400 invalid_json` | Como el rollback |
| `GET /proxy` con `disabled` | — | No consulta Docker: `200` con `image: null` y `running: false`, también sin Docker | El panel muestra "El proxy de muelle está apagado" en desarrollo, en vez de nada |
| Settings: `cloudflare_zones` | Abierto (¿consulta en cada `GET`?) | Valor guardado al guardar el token, que actualiza el job `"proxy"` | Un `GET` no depende de Cloudflare |
| Settings: `""` en `public_ip` | El mock lo rechaza | `""` borra `public_ip`, `public_ipv6` y `acme_email` | Hay que poder quitar un valor |
| Settings: quitar el token | Abierto (el mock rechaza `""`) | `cloudflare_token: ""` lo borra | |
| Settings: token rechazado o Cloudflare caído | — | `422 cloudflare_token` / `502 cloudflare_unavailable` | El panel tiene que mostrar el 502 como reintentable |
| Settings: `{` o `}` en `acme_email` o en el token | Los rechaza el panel, en el cliente | El backend también: `422` en ese campo (§4.1) | Guardados, Caddy deja de recibir la configuración (`out_of_sync`, §3.4); así tampoco los guarda otro cliente de la API |
| Settings: cambio de IP pública | — | Los registros que maneja muelle siguen a la IP (solo el contenido); un dominio en `error`/`cloudflare` que se actualiza bien vuelve a `pending`, y uno que falla queda en `error`/`cloudflare`. El `PATCH` responde `200` igual | Compatible: el panel vuelve a leer los dominios |
| Settings: lista que te deja fuera | Guarda sugerida | `409 would_lock_out`; se guarda con `confirm_lockout: true` | La pide la revisión del panel |
| Settings: cambio de puertos | Abierto | El `PATCH` responde antes de recrear Caddy, que se recrea 2 s después; `GET /proxy` dice `recreating` mientras tanto. Si cambió `proxy_https_port`, el panel lleva al usuario al origen nuevo | La respuesta pasa por el Caddy que se recrea: si no, el cliente vería una conexión cortada |
| Settings: puerto del proxy ocupado | — | El `PATCH` prueba los puertos nuevos antes de guardar: uno ocupado es `422` en su campo (`proxy_http_port` o `proxy_https_port`), y Docker caído `503`. Si se ocupa después, la recreación vuelve a los anteriores y `GET /proxy` suma `ports_rollback`; si el origen nuevo no responde, el panel ofrece volver al anterior | Un puerto ocupado dejaba el panel sin Caddy hasta arreglarlo a mano desde el servidor |
| `Origin` después de cambiar los puertos | — | Pasa `https://<panel_domain>[:<proxy_https_port>]` con el puerto de ese momento (y el del puerto viejo ya no), además de `MUELLE_PANEL_ORIGIN` si está; los links de setup e invitaciones usan el mismo origen. install escribe `MUELLE_PANEL_DOMAIN` en vez de `MUELLE_PANEL_ORIGIN` | Si no, todo el panel, el login incluido, quedaría en `403 bad_origin`, y los links en el puerto viejo, hasta actualizar la configuración de muelle |
| Lo que dice el panel del origen | `403 bad_origin`: "revisa `MUELLE_PANEL_ORIGIN`"; después de cambiar `proxy_https_port`, actualizar `MUELLE_PANEL_ORIGIN` con `docker service update` | `bad_origin`: el origen permitido es `https://<panel_domain>[:<proxy_https_port>]`, con los valores de Ajustes, o `MUELLE_PANEL_ORIGIN` si alguien la puso a mano (detrás de otro proxy, o en desarrollo). Cambiar los puertos no pide tocar la configuración de muelle, y el panel no muestra ese paso. Solo una instalación anterior que escribió `MUELLE_PANEL_ORIGIN` con el puerto de entonces lo necesita, y es otro comando: `docker service update --env-rm MUELLE_PANEL_ORIGIN --env-add MUELLE_PANEL_DOMAIN=<dominio> muelle_muelle` (README) | install ya no escribe `MUELLE_PANEL_ORIGIN` (§5.2): en producción, la pista nombraría una variable que no está |
| Settings: `client_ip` | String o null | Siempre string | Compatible |
| Settings: certificados de Cloudflare | — | `cloudflare_origin_ca` (bool, `false` por defecto, siempre en `GET /settings`) en `GET`/`PATCH /settings` (§4, §4.1). Encenderla sin token, quitar el token sin apagarla, o un token sin Zone → SSL and Certificates → Edit en alguna zona activa es `422`; Cloudflare caído, `502` | El dueño quiere Full (strict) con certificados de Cloudflare, sin dejar Let's Encrypt como lo normal (§3.7) |
| Dominio: certificado | `cert_issuer` dice el emisor | `cert_issuer` sigue igual (lo que vio la verificación); el dominio suma `cert_source` (`letsencrypt` o `cloudflare_origin_ca`) y `origin_ca` (null con la opción apagada), los dos calculados y siempre presentes (§3.1, §3.7) | `cert_issuer` es un texto observado, no un enum: el panel nombra el origen sin depender del texto de la CA |
| Dominio: falla del de Origin CA | — | No deja el dominio en `error`: sigue con Let's Encrypt, y el aviso va en `origin_ca` (`failed`, o `active` con `error_code` si falló una renovación) | El dominio funciona: un `error` lo mostraría caído y lo sacaría de la verificación |
| `GET /proxy`: el certificado del panel | — | Suma `panel_certificate` (null sin `panel_domain`), siempre presente (§3.3) | El panel no está en `domains`, y Ajustes muestra su certificado |
| Registries: `PATCH` | `{username, password}` | Los dos obligatorios: reemplaza | Igual al payload del panel |
| Registries: host | Normalizado por el panel | El backend exige la forma canónica y no normaliza; Docker Hub es `docker.io`, y sus alias (`index.docker.io`…) son `422` | Una credencial con un alias nunca se usaría |
| Registries: respuesta | `{host, username}` | Suma `created_at` y `updated_at` | Compatible |
| Auditoría: `actor_name` | Lo pide en `GET /audit` | Siempre string, copiado al escribir el evento: el username, el nombre del token o `system` (§4.2) | Como el de un deploy: no se pierde al borrar el usuario o el token |
