# muelle

Deploy manager ultraligero sobre Docker Swarm: panel web + API, secretos, stages y proxy con certificados automáticos. Go (`net/http` de la biblioteca estándar, SQLite en Go puro) con arquitectura hexagonal/DDD.

- Diseño: [`docs/superpowers/specs/2026-10-09-muelle-design.md`](docs/superpowers/specs/2026-10-09-muelle-design.md)
- Plan de la Fase 1: [`docs/superpowers/plans/2026-10-10-muelle-fase1-hexagonal.md`](docs/superpowers/plans/2026-10-10-muelle-fase1-hexagonal.md)
- Contrato de las Fases 2, 3 y 5 (API, puertos entre módulos, install): [`docs/superpowers/specs/2026-10-10-contrato-http-fases-2-3.md`](docs/superpowers/specs/2026-10-10-contrato-http-fases-2-3.md)

## Estado

Fase 1 (Base): API REST de usuarios, sesiones, 2FA, tokens de API, proyectos, stages, apps, variables cifradas, settings, auditoría y backups.

Etapa de contratos de las Fases 2, 3 y 5:
- el cliente de Docker compartido, con su fake;
- los esqueletos de `deploy` e `ingress`. Crear un stage ya crea su red overlay, y borrar una app hace `docker stack rm`;
- los ajustes del proxy y el DNS en `/settings`;
- las migraciones reservadas.

Fase 2 (Deploy): despliega apps de formulario y compose (render con las restricciones de §6.3, una cola por app, digests con las credenciales de los registries, `docker stack deploy`, espera de convergencia y rollback), con el log del deploy por polling y en vivo, los registries, y el estado y los logs en vivo de cada app. Las rutas de Caddy llegan con la Fase 3: mientras tanto, cada deploy deja una advertencia en su log.

## Estructura (spec §4.1)

```
main.go             subcomandos serve y genkey
src/
├── cmd/http/       ServeMux, middleware y errores → HTTP
├── cmd/cron/       ejecuta las tareas periódicas de los módulos
├── internal/
│   ├── identity/   usuarios, sesiones, 2FA, invitaciones, tokens de API
│   ├── catalog/    proyectos, stages y apps
│   ├── secrets/    variables cifradas de stage y app
│   ├── admin/      settings, auditoría, backup y limpieza periódica
│   ├── deploy/     (Fase 2) deploys, registries, estado y logs de las apps
│   └── ingress/    (Fase 3) redes de stage, dominios, Cloudflare y muelle-caddy
├── shared/         dominio común, nombres de Docker, el cliente de Docker y container.New, que arma todo
├── package/        interfaces finas: cipher, clock, audit
└── settings/       carga y validación de MUELLE_*
```

Cada módulo tiene `domain/`, `app/` e `infra/` (in/out). Los módulos no se importan entre sí.

## Desarrollo local

La clave maestra descifra todos los secretos: el archivo tiene que quedar con permisos 0600 (el `umask 077` va en un subshell para no cambiar el de tu shell).

```bash
(umask 077; go run . genkey > /tmp/muelle.key)
MUELLE_PANEL_ORIGIN=http://127.0.0.1:18080 \
MUELLE_SECURE_COOKIES=false \
MUELLE_LISTEN=127.0.0.1:18080 \
MUELLE_DATA_DIR=/tmp/muelle-data \
MUELLE_MASTER_KEY_FILE=/tmp/muelle.key \
go run . serve
```

Con el binario, igual: `(umask 077; muelle genkey > muelle.key)`.

### Variables

| Variable | Por defecto | Qué es |
|---|---|---|
| `MUELLE_PANEL_ORIGIN` | (obligatoria) | Origen exacto del panel, en forma canónica (con el puerto si no es el por defecto); se compara con el header `Origin` (CSRF) y arma los links de setup e invitaciones |
| `MUELLE_LISTEN` | `:8080` | Dirección HTTP. No publiques este puerto: solo se llega a muelle por el proxy (ver abajo) |
| `MUELLE_DATA_DIR` | `/data` | Carpeta de `muelle.db` y `backups/` |
| `MUELLE_MASTER_KEY_FILE` | `/run/secrets/muelle_master_key` | Clave maestra en base64 (32 bytes), con permisos 0600 |
| `MUELLE_SECURE_COOKIES` | `true` | `false` solo en desarrollo por http; con un `MUELLE_PANEL_ORIGIN` https, muelle no arranca |

### Primer usuario

Al arrancar sin usuarios, muelle imprime en el log un link de setup de un solo uso (vence en 24 h):

```
{"level":"INFO","msg":"todavía no hay usuarios: abre este link para crear el primero (vence en 24 h)","event":"setup_url","url":"http://127.0.0.1:18080/setup?token=<TOKEN>"}
```

La página `/setup` llega con el panel (Fase 4); mientras tanto, el usuario se crea con `curl`, con el `token` del link. Toda petición que modifica algo con la cookie de sesión tiene que traer el header `Origin` igual a `MUELLE_PANEL_ORIGIN` (si no, `403 bad_origin`):

```bash
O=http://127.0.0.1:18080
curl -c cookies -H "Origin: $O" -H 'Content-Type: application/json' \
  -d '{"token":"<TOKEN>","username":"admin","password":"una-contraseña-larga"}' \
  $O/api/v1/auth/setup                      # 201 {"username":"admin"}; la sesión queda en ./cookies
curl -b cookies $O/api/v1/auth/me
curl -b cookies -H "Origin: $O" -H 'Content-Type: application/json' \
  -d '{"slug":"auditfy","name":"Auditfy"}' $O/api/v1/projects
curl -b cookies -H "Origin: $O" -H 'Content-Type: application/json' \
  -d '{"name":"ci","project":"auditfy"}' $O/api/v1/tokens   # el token en claro se muestra una sola vez
```

La contraseña tiene al menos 12 caracteres. Los tokens de API (`Authorization: Bearer mk_…`) no necesitan `Origin`.

### Docker

Crear y borrar un stage crea y borra su red overlay, y borrar una app hace `docker stack rm` (con el CLI `docker` en el `PATH`). Para eso, el Docker local tiene que ser manager de un Swarm (`docker swarm init`); si no, esas peticiones responden `503 docker_unavailable` y no cambian nada. El resto de la API anda sin Docker. Sin un Swarm, esos caminos se prueban con los tests, que usan el Docker en memoria (`dockertest`, que `container.WithDocker` inyecta).

Un deploy necesita lo mismo (el CLI `docker` en el `PATH` y un Swarm del que el nodo sea manager) y, además, llegar al registry de cada imagen: el paso 2 consulta el digest de cada una, también de las que ya traen uno. Con Docker caído el deploy se encola igual (`202`) y falla en el paso 2.

### Desplegar una app con curl

Después de "Primer usuario" (con la sesión en `./cookies`), un stage, una app de formulario y un token del proyecto. El deploy, el log y el rollback van con el token, como en CI:

```bash
O=http://127.0.0.1:18080
A=$O/api/v1/projects/auditfy/stages/staging/apps/web
curl -b cookies -H "Origin: $O" -H 'Content-Type: application/json' \
  -d '{"slug":"staging"}' $O/api/v1/projects/auditfy/stages          # crea la red auditfy_staging
curl -b cookies -H "Origin: $O" -H 'Content-Type: application/json' \
  -d '{"slug":"web","kind":"service","spec":{"image":"nginx:alpine","port":80,"healthcheck":{"type":"http","path":"/"}}}' \
  $O/api/v1/projects/auditfy/stages/staging/apps
T=mk_…                                                                # el token de "Primer usuario"
curl -H "Authorization: Bearer $T" -X POST $A/deploys               # 202 {"id":1,"status":"queued",…}
curl -H "Authorization: Bearer $T" "$A/deploys/1/log?offset=0"      # repetir con next_offset hasta status final y chunk ""
curl -N -H "Authorization: Bearer $T" $A/deploys/1/log/stream       # o en vivo (SSE), hasta el status final
curl -H "Authorization: Bearer $T" $A/status                        # servicios, tareas, CPU y memoria
curl -N --max-time 5 -H "Authorization: Bearer $T" "$A/logs?service=web&tail=10"   # con &follow=1 sigue abierto
curl -H "Authorization: Bearer $T" -X POST $A/rollback              # 409 no_previous_deploy hasta el segundo deploy
curl -b cookies -H "Origin: $O" -X DELETE $A                         # docker stack rm (los volúmenes quedan)
```

Un deploy sin cuerpo repite las imágenes del deploy actual; `{"image":"nginx:1.27"}` (o `{"images":{"<servicio>":"…"}}` en una app compose) cambia la imagen. Las credenciales de un registry privado se guardan con la sesión: `POST /api/v1/registries` con `{"host":"ghcr.io","username":"…","password":"…"}`.

## Solo detrás de Caddy (o de otro proxy)

**muelle nunca debe quedar expuesto directo.** Para limitar los logins por IP usa la IP del cliente, y confía en el header `X-Real-IP` cuando la conexión viene de loopback o de una red privada (RFC 1918, ULA), que es por donde llega Caddy (spec §8). Si un cliente puede conectarse a muelle sin pasar por el proxy desde una de esas redes, escribe su propio `X-Real-IP` y se salta el límite por IP.

- El `MUELLE_LISTEN=:8080` por defecto escucha en todas las interfaces: **no publiques ese puerto** (ni con `-p`/`ports:` de Docker ni en el firewall del host). En producción muelle está en la red `muelle_panel`, a la que solo llega Caddy.
- El proxy tiene que **pisar** `X-Real-IP` con la IP real del cliente, nunca reenviar la que mande el cliente. En un Caddyfile es `header_up X-Real-IP {client_ip}`; en la configuración JSON que manda muelle a muelle-caddy, `{http.vars.client_ip}` (el atajo `{client_ip}` no existe en JSON; contrato §3.4).
- En desarrollo, escucha en `127.0.0.1`, como en el ejemplo de arriba.

## Tests

```bash
gofmt -l $(git ls-files '*.go')   # no debe imprimir nada
go mod tidy -diff
go vet ./...
go test -race ./...
```

Los tests de `src/shared/infra/container` arman muelle completo con los cuatro módulos (matriz de permisos de los tokens, contratos entre módulos, fechas). La prueba de punta a punta con el binario real y `curl` (spec §15) por ahora es manual: los pasos de "Primer usuario" de arriba, más los endpoints de la fase.

## Convenciones

Comentarios y mensajes de error en español; identificadores en inglés.

## Notas para las fases siguientes

Hallazgos de la revisión de código que las próximas fases deben respetar.

**Fase 2, render**
- ~~Escapar `$` como `$$` en todo string que venga del spec de formulario (`command`, `healthcheck.command`, imagen, mounts, valores de env).~~
- ~~Generar el healthcheck `http` en forma exec (sin shell).~~
- ~~Fijar el nombre del proyecto compose explícitamente y usar variables de entorno solo para interpolación (las claves `COMPOSE_*` están reservadas).~~
- ~~La lista de permitidos y de rechazos de §6.3: entre otras, `secrets:`/`configs:` (con `file:` o `external` montarían la clave maestra en la app), volúmenes con `driver`/`driver_opts`/`external`/`name` (un `driver_opts` con `o: bind` monta `docker.sock`), `env_file`/`extends`/`include` y etiquetas `muelle.*` propias. Un test por cada rechazo.~~
- ~~Interpolar solo con las variables de `ResolveEnv` (nunca `os.Environ()`), y escapar todo `$` como `$$` en el compose final, porque `docker stack deploy` vuelve a interpolar.~~
- ~~Comprobar que el tamaño del env mezclado stage+app cabe en el `ARG_MAX` de Linux.~~
- ~~`secrets.ResolveEnv` ya está conectado al deploy (puerto `EnvResolver`), igual que `docker stack rm` al borrar una app.~~
- ~~Reemplazar `validateCompose` por la validación real de compose-go, incluyendo nombres de servicio únicos por stage. La parte pura (parsear, las restricciones de §6.3, los nombres de los servicios) va en un paquete de `shared/domain`, para que catalog la use al guardar y deploy al renderizar sin importarse.~~
- compose-go puede escribir advertencias con logrus en stderr (por ejemplo, `version` obsoleta). No llevan valores de variables, y la configuración global de logrus no se toca.

**Fase 2, worker de deploy**
- ~~Las goroutines nunca deben usar el ctx de una transacción después de que `InTx` retorna; derivar de un ctx externo (`context.WithoutCancel`).~~
- ~~La retención guarda siempre el deploy actual y el destino del rollback, además de los 20 más nuevos (contrato §2.3).~~
- ~~Enmascarar también cada línea de un secreto de varias líneas, y nada de menos de 8 bytes (contrato §2.5).~~
- ~~Los SSE revisan la credencial cada 60 s y tienen límites por principal y de duración (contrato §0.3).~~
- Un deploy sin cuerpo repite la ref **pedida** del deploy actual (`requested_images`): un tag se vuelve a resolver y un digest fijado a propósito (override, configuración o rollback) se mantiene; un rollback no se deshace con el próximo deploy sin cuerpo, y para volver a seguir un tag hace falta un override o cambiar la configuración (contrato §2.2).
- `SyncRoutes` devuelve `ErrNotImplemented` hasta que se mergee la Fase 3: cada deploy deja `advertencia: no se pudo actualizar Caddy: …` en su log y termina igual (contrato §2.11). El test de contrato del contenedor acepta las dos líneas.
- La convergencia de un servicio que ya existía y no cambió espera un margen de 5 s sin `UpdateStatus` nuevo (decisión 6 del plan de la Fase 2). Se podría reemplazar comparando el spec con `PreviousSpec`, con un método nuevo del cliente de Docker.

**Fase 2, estado y logs**
- ~~`GET …/logs` acepta `since` en vez de `tail` (inclusivo, hasta 5000 líneas), con `time` truncado al milisegundo: el panel lo manda al reconectar (contrato §2.9).~~
- ~~El servicio de los logs se busca por `naming.Service(p, s, a, service)` y con las etiquetas de la app, nunca con el parámetro tal cual: Docker también acepta un id o un prefijo de id (contrato §2.9).~~
- ~~Las medidas de `GET …/status` (`ContainerStats`) se guardan 5 s por app (contrato §2.8).~~
- Los pedazos de una línea de más de 16 KiB que parte el cliente de Docker se unen antes de enmascarar, pero un secreto que cruza el corte de 16 KiB que hace el propio Docker en su `json-file` (dos líneas, cada una con su hora) no se detecta (contrato §2.5).
- Un stream abierto mantiene viva la sesión: `Kit.Revalidate` usa el mismo `Authenticator`, que toca `last_seen_at` (a lo más una vez por minuto), así que la sesión no vence por las 12 h sin uso mientras el panel tenga un stream abierto. El tope de 7 días sigue valiendo y un stream dura a lo más 1 h.

**Fase 3**
- Crear un stage ya crea la red overlay y conecta Caddy (etapa de contratos).
- El pool de direcciones de la overlay debe ser privado (RFC1918/ULA) para que se pueda confiar en `X-Real-IP` (`docker.Network.Subnets`).
- Las IPs permitidas del panel van con el matcher `client_ip` de Caddy, no `remote_ip` (contrato §3.4).
- La configuración de Caddy es JSON: placeholders completos (`{http.vars.client_ip}`), nunca los atajos del Caddyfile. El golden lo verifica.
- muelle-caddy se crea con `naming.CaddyCommand()` y `naming.CaddyEnv()`, nunca con el CMD de la imagen (la admin API quedaría en TCP, sin el socket).
- Docker 29 arranca sin puertos un contenedor cuyo arranque falló con `ErrPortInUse`: muelle-caddy se recrea, nunca se rearranca (`Container.PublishedPorts`).

**Fase 5 / install**
- Considerar el prefijo `__Host-` en el nombre de la cookie de sesión cuando las cookies son Secure.
- La imagen de muelle necesita un `/tmp` escribible; la prueba de puertos incluye 443/udp y usa `MuelleImageFor(version)`, bajándola si falta; el link de setup se lee de la tarea actual (contrato §5.4).

**Pendiente de la revisión final de la Fase 1**
- Pedir de nuevo la contraseña (o el código TOTP actual) para cambiar el TOTP propio y para desactivar `require_2fa`: hoy alcanza con la sesión.
- Auditoría: guardar el nombre del actor (hoy solo `actor_type` + `actor_id`, que deja de resolverse si el usuario o el token se borran) y registrar los 403 (`out_of_scope`, `token_not_allowed`), que son sondeos.
- Migraciones: no arrancar si la base tiene una versión más nueva que la del binario (hoy un downgrade arranca sobre un esquema desconocido).
- El semáforo de Argon2id debería respetar el `ctx` de la petición: hoy un login espera su turno aunque el cliente ya se haya ido.
- Listar y revocar invitaciones (`GET /invites`, `DELETE /invites/{id}`).
- Un endpoint de salud (`GET /healthz`) para Swarm y Caddy.
- Hacer configurable el rango de proxies de confianza para `X-Real-IP` (hoy: loopback y redes privadas).
- `Retry-After` con la duración real: el 429 lleva un valor fijo de 60 s y el 423 ninguno, porque `ErrRateLimited` y `ErrLocked` no traen cuánto falta (habría que pasarlo desde `LoginLimiter` y los bloqueos).
- El reloj (`package/clock`) da la hora en UTC truncada al milisegundo, sin lectura monotónica: los límites y bloqueos en memoria siguen al reloj de pared, así que un salto del reloj del host los acorta o alarga.
