# muelle — Fase 3 (ingress): Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** El módulo `ingress` completo: muelle crea y mantiene el contenedor muelle-caddy, le manda la configuración completa por la admin API (rutas de los dominios y del panel, TLS por Cloudflare o HTTP, IPs permitidas del panel), maneja los registros DNS en Cloudflare (crear, adoptar, reintentar, borrar, IP pública), verifica los certificados, expone `…/domains` y `GET /proxy`, y aplica los cambios de `/settings` que le avisa admin.

**Architecture:** Spec §4.1 (hexagonal). Todo vive en `src/internal/ingress/`:
- `domain/` tiene las reglas puras: la entidad `Domain` y sus estados, la zona por sufijo, la configuración del proxy como datos (`service.ProxyConfig`, sin JSON de Caddy) y la regla de los certificados. Los puertos hacia Docker, Caddy, Cloudflare, el TLS de Caddy, catalog, registries y settings están en `domain/service`.
- `app/usecases` orquesta: dominios, `Syncer` (un único `POST /load` a la vez), el ciclo de vida, los jobs, los reintentos y los hooks de admin.
- `infra/out/service/caddy` tiene el JSON de Caddy, el cliente de la admin API y el ciclo de vida del contenedor (spec §4.2). `infra/out/service/cloudflare` es el cliente mínimo de Cloudflare. Los dos tienen su fake HTTP de prueba (`caddytest`, `cftest`).
- Lo que ingress lee de otros módulos (apps, stages, registries, settings) va por adaptadores SQL en `infra/out/service` (§4.1 regla 1).

**Tech Stack:**
- Go 1.27, librería estándar (`net/http`, `crypto/tls`, `crypto/x509`, `encoding/json/v2`).
- `modernc.org/sqlite`, ya en `go.mod`.
- `github.com/distribution/reference`, ya en `go.mod`: el host de la imagen de Caddy, para su credencial.
- **Sin dependencias nuevas:** ni SDK de Cloudflare ni de Caddy.
- Para los tests de integración: las imágenes `caddy:2-alpine` y `alpine/socat:latest`, que ya están en el nodo. Si faltan, el test se salta y no las baja.

**Spec:** [`docs/superpowers/specs/2026-10-09-muelle-design.md`](../specs/2026-10-09-muelle-design.md) (§3.1, §3.2, §8, §9, §12, §13 y §15) y el contrato [`2026-10-10-contrato-http-fases-2-3.md`](../specs/2026-10-10-contrato-http-fases-2-3.md) (§0, §1, §3, §4 y §7). **El contrato manda:** cada endpoint, status, código y campo JSON de este plan sale de su §3. Si algo de acá no coincide con el contrato, es un error de este plan: se sigue el contrato y se avisa.

**Estado de partida (rama `fase-3-ingress`, cortada de `fase-23-base`):**
- `ingress.Module` ya existe como esqueleto.
- Ya funcionan `EnsureStageNetwork` y `RemoveStageNetwork`, que usan catalog y deploy, y `ProxySettingsReader`.
- `SyncRoutes` devuelve `entity.ErrNotImplemented`. `CheckCloudflareToken` devuelve `(nil, nil)` y `ProxySettingsChanged` no hace nada.
- Las rutas responden `404 no_route`.
- La tabla `domains` existe (`0003_ingress.sql`).
- El contrato ya tiene las precisiones de la Fase 3 (commits `4479f23` y `5e353d3`), y el diseño está al día (`8f732d2`).
- La revisión de este plan sumó al contrato (`2249ccc`) y al diseño (`a2220f7`): `record_exists` conserva la zona y el TLS de Cloudflare (D7), la imagen de Caddy se baja antes de detener el contenedor (D11), `trusted_proxies_strict` y sin llaves en el email ni el token (D16).
- `EnsureStageNetwork` y `RemoveStageNetwork` todavía conectan el nombre fijo `muelle-caddy` (`naming.CaddyContainer`). T2 Step 5 les pasa el nombre de `Options` **antes** del primer test de integración, que así nunca toca un `muelle-caddy` real.

**Decisión del dueño (2026-10-15, `fase-5-dist` `51f6fbb`): las imágenes de muelle son privadas.**
- `ghcr.io/cpu-commits/muelle-caddy` siempre es privada.
- Install la baja con credenciales, así queda en el nodo para el primer arranque. También siembra el registry `ghcr.io`: deploy lo guarda en `registries` desde `MUELLE_INIT_REGISTRY_FILE` (Fase 2).
- ingress no cambia de mecanismo: baja las versiones nuevas con la credencial del host de la imagen (D11, `TestEnsureCaddyImage`), y si no baja, el contenedor anterior sigue.
- Lo que suma este plan por la decisión:
  - si el registry rechaza la imagen, el mensaje de `GET /proxy` dice qué hacer en Ajustes → Registries (T3 Step 3);
  - esa contraseña, que ahora se usa en cada recreación, nunca sale en un log, un error ni un JSON ("Secretos", T3 Step 2).

---

## Etapas y paralelismo

| Etapa | Tareas | Cómo corre |
|---|---|---|
| Fase 3 | T1 → T2 → T3 → T4 → T5 | Secuencial, en `fase-3-ingress` (worktree `.claude/worktrees/fase-3-ingress`). Cada tarea la hace un agente nuevo y se revisa antes de la siguiente |
| En paralelo | Fase 2 (`fase-2-deploy`) y Fase 5 (`fase-5-dist`) | Otras ramas desde `fase-23-base`; se integran después |

**Qué toca esta fase fuera de `src/internal/ingress/`** (cambios mínimos, solo en lo de ingress):

| Archivo | Tarea | Qué |
|---|---|---|
| `src/shared/infra/out/sqlite/migrations/0003_ingress.sql` y `TestDomainsSchema` en `migrations_test.go` | T1 | Tres `CHECK` nuevos (D19) |
| `src/shared/domain/entity/panel_dns.go` (nuevo) | T1 | Las constantes `panel_cf_*` (contrato §3.6) |
| `src/shared/infra/container/container_test.go` | T3, T4 | Solo la línea de `TestNew` con los nombres de los jobs |
| `src/shared/infra/container/container.go` | T2 | `WithIngress`, `build` recibe el `options` entero y pasa `Options` a `ingress.New` |
| `src/shared/infra/container/ports_test.go` | T2 | `TestContractProxySettings` usa el Cloudflare falso, mucho antes de que T5 cablee el hook que lo llama |
| `src/shared/infra/container/ingress_test.go` (nuevo) | T5 | Contratos de ingress con el contenedor completo |
| `README.md` | T5 | La línea de la Fase 3 en "Estado" y el bloque "Fase 3" de las notas |
| Contrato (§1.1 y §1.2 de ingress, §3, filas de ingress de §7) y diseño (§3.1, §3.2, §8, §9) | T5 | Marcar "hecho" y lo que cambie al implementar |

**No se toca:** `src/internal/deploy/**`, `src/internal/catalog/**`, `shared/domain/compose`, los tipos `RegistryAuth`/`StackDeploy`, la migración 0002, `web/` ni lo de la Fase 5. Tampoco se agregan métodos a `shared/infra/out/docker`: alcanza con los que hay (D20).

**Al integrar** (no lo hace este plan):
- Los tests `ctr23` del paquete docker corren recién al mergear.
- **Conflictos esperables y cómo se resuelven:**
  - **La línea de jobs de `TestNew`,** si la Fase 2 agrega jobs. Se resuelve en el orden de `c.Jobs`: identity, admin, ingress, deploy.
  - **`options` en `container.go`:** si otra fase suma una opción, se juntan los dos campos. `WithIngress` no toca nada más.
  - **`build` en `container.go`:** la Fase 5 ya le sumó `panelHandler http.Handler`, en la firma y en la llamada de `New`. T2 Step 5 le suma `o options`, el valor entero, así la próxima opción no vuelve a cambiar la firma. Las dos líneas chocan, y se dejan los dos parámetros: `build(ctx, db, ciph, engine, s, log, panelHandler, o)` y `func build(…, log *slog.Logger, panelHandler http.Handler, o options)`.
  - **Contrato §1.2:** la Fase 2 reescribe la lista de `deploy.Run`, justo arriba del bloque de ingress. T5 solo toca el bloque **`ingress`** (Fase 3). Si chocan, se deja la lista de la Fase 2 y el bloque de ingress de esta rama.
- **La integración de esta fase y la de la Fase 5 no corren a la vez.** El test de install de la Fase 5 falla si desaparece una red con la etiqueta `muelle.project` mientras corre, y los tests de esta fase crean y borran redes `it3<id>_*` con esa etiqueta. Queda para la revisión final: o se corren por separado, o el guard de la Fase 5 ignora las redes con los prefijos `it2` e `it3`.

## Convenciones (todas las tareas)

- **Dónde:** solo en `/home/felipe/Escritorio/dev/muelle/.claude/worktrees/fase-3-ingress`. Cada comando empieza con `cd` a esa carpeta o usa `git -C …`. Nunca `git switch`/`checkout` ni otro checkout.
- **Shell:** fish. Lo que sea sintaxis bash va dentro de `bash -c '…'`.
- **Antes de cada commit** (todo tiene que pasar):
  ```bash
  cd /home/felipe/Escritorio/dev/muelle/.claude/worktrees/fase-3-ingress
  bash -c 'test -z "$(gofmt -l $(git ls-files "*.go"))"'   # nunca gofmt -l . (entra a .claude/worktrees)
  go vet ./... && go vet -tags integration ./src/internal/ingress/...
  go test -race ./...
  go mod tidy -diff
  ```
  Ojo: `git ls-files` solo ve archivos ya agregados. Haz `git add` antes del gofmt, o corre `gofmt -l` sobre los archivos nuevos.
  Ojo también con el encadenado: todo hasta el `git commit` va con `&&` (o dentro de un `bash -c 'set -e; …'`). En T2, un `… | grep -v ok ; go mod tidy -diff && git commit` dejó pasar dos commits con un test sin gofmt: el `;` corta la cadena, y el gofmt que falló se saltó también el `go vet` y el `go test` (lo arregló `862ccb9`).
- **Commits:**
  - en español con los acentos reales;
  - `git commit -m "<mensaje>" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"`;
  - chicos: uno por paso que dice "Commit".
- **TDD:** el test primero → correrlo y verlo fallar (por la razón esperada) → implementar → verlo pasar → commit.
- **Idioma:** comentarios, mensajes de error y de log en español; identificadores en inglés.
- **Tests:**
  - **Base y reloj:** SQLite real (`sqlitetest.New(t)`) y `clock.NewManual`.
  - **Docker:** `dockertest.Fake`.
  - **Cloudflare:** solo `cftest` (T1), un servidor HTTP falso. **Nunca la API real.**
    - En producción, el cliente usa `https://api.cloudflare.com/client/v4`.
    - Todo test que llegue a Cloudflare pasa `Options.CloudflareURL` al falso.
    - Desde T2, `newTestModule` (`module_test.go`) siempre pasa un `cftest` nuevo en `Options.CloudflareURL`, y todo test del contenedor completo que mande un token usa `WithIngress` con un `cftest`. Así, cuando T5 cablea los hooks, ningún test existente sale a la red.
    - Tampoco a mano: la prueba con el binario (T5 Step 7) nunca manda un token bien formado.
  - **Caddy:** `caddytest` (T2), una admin API falsa.
  - Se corren con `-race`.
- **Tests de integración** (`//go:build integration`, solo los de `./src/internal/ingress/...`):
  ```bash
  go test -tags integration -count=1 -run Integration -v ./src/internal/ingress/...
  ```
  - Necesitan Docker como manager de un Swarm activo; si no, se saltan.
  - **Nombres:** todo lo que crean empieza con `it3<id>` o `it3-<id>-` (`id` = 4 hex al azar por test), y se borra en `t.Cleanup`, también si el test falla. Los nombres exactos están en `itEnv` (T2 Step 6), y la limpieza busca **los dos** prefijos.
  - **`Run` en segundo plano:** solo con `e.run(m)` (T2 Step 6). Su limpieza cancela `Run` y espera a que vuelva **antes** de borrar contenedores y redes, así el `Recreator` no crea un `it3-<id>-caddy` después de la limpieza.
  - **Stages:** toda red de stage que crea un test tiene su stage en la base (`ingresstest.Stage`). Si no, el job `"proxy"` la toma por huérfana y la borra a la mitad del test.
  - **Puertos:** solo del host entre 21000 y 21999, por "ranuras": la ranura `n` usa los puertos `21000+100n+1` en adelante (D4). Nunca 80, 443 ni 2019.
  - **Prohibido:** los nombres fijos (`muelle_panel`, `muelle-caddy`, `muelle_caddy_*`, `muelle_muelle`, …) y los tests `ctr23` del paquete docker.
  - No se corren en paralelo: sin `t.Parallel()`. Tampoco a la vez que la integración de la Fase 5 (ver "Al integrar").
  - **Cloudflare:** `newITEnv` pone `opts.CloudflareURL` en un `cftest` nuevo, aunque el test no mande un token.
- **Secretos:**
  - el token de Cloudflare nunca va a un log, a un error ni a un JSON de respuesta;
  - la configuración de Caddy nunca va al log;
  - `service.ProxyConfig` lo oculta en `String`/`LogValue`, como `ProxySettings`;
  - **la contraseña de un registry** (`service.Credential.Password`, descifrada) nunca va a un log, a un error ni a un JSON. `Credential` la oculta en `String`, `GoString`, `LogValue` y `MarshalJSON` (T3 Step 2), y nadie registra un `CaddyWant` ni un `docker.RegistryAuth`.
- **Alcance:** no modifiques archivos que la tarea no lista. Si algo de fuera parece necesario, detente y repórtalo (`NEEDS_CONTEXT`).

## Decisiones

Lo que el contrato no fija y este plan decide. Cada una con su porqué.

- **D1. La configuración se separa en datos y JSON.**
  - `domain/service.BuildProxyConfig` (puro) decide qué host va a qué upstream, con qué TLS, y las IPs del panel.
  - `infra/out/service/caddy.Render` lo pasa al JSON de Caddy.
  - *Por qué:* el dominio no conoce el formato de Caddy (§4.1 regla 2). El golden de §15 prueba el JSON, y las reglas se prueban sin JSON.
- **D2. El ciclo de vida de muelle-caddy va en el adaptador** `caddy.Runtime`, detrás de un puerto grueso: `CaddyRuntime.Ensure(ctx, want)` y `Status(ctx)`.
  - *Por qué:* el spec §4.2 lo pone en `infra/out/service/caddy`. Además, un puerto fino obligaría a copiar en el dominio todos los tipos de Docker.
- **D3. `ingress.Options` en `Deps`:** los nombres de Docker, cómo se llega a la admin API, la dirección TLS y las raíces para verificar certificados, la CA interna de Caddy, el prefijo de redes huérfanas, la URL de Cloudflare y los plazos (`Timing`).
  - El valor cero es producción: los nombres de `naming`, el socket unix, `muelle-caddy:443`, las raíces del sistema, ACME, sin prefijo, la API real y los plazos del contrato.
  - Solo los tests lo cambian.
  - *Por qué:* los tests de integración no pueden usar los nombres fijos, el puerto 443 ni la API real.
- **D4. En integración, muelle llega a la admin API por un puente:** un contenedor `alpine/socat` monta el volumen del socket y lo publica en `127.0.0.1:<ranura+3>`.
  - Con eso, la configuración que se manda es la de producción (`admin.listen` al socket).
  - Caddy no revisa el `Host` en un socket unix (verificado: `"admin endpoint on open interface; host checking disabled"`), así que el puente funciona.
    - Eso es de **Caddy 2.10 en adelante** (caddy#6832, en v2.10.0-beta.5; la imagen local es v2.11.6). En las versiones anteriores, el `Host: caddy` que manda `caddy.Admin` se rechaza, y todo `/load` falla. Vale también en producción, no solo para el puente.
    - Por eso una `caddy_image` propia necesita Caddy ≥ 2.10 y el módulo `dns.providers.cloudflare`. Ya está en el contrato §3.4, y T5 lo pone en el README.
  - También se verificó que un Caddy arranca bien sobre el socket que dejó otro matado con `docker kill`.
  - El TLS se prueba con `InternalCA` (`{"module": "internal"}` y `pki…install_trust: false`). La imagen oficial no trae el módulo de Cloudflare: el modo `cloudflare` se prueba solo con el golden.
- **D5. Crear un dominio guarda primero la fila y después hace el DNS.**
  - La fila se guarda `pending`, con `tls_mode: http` provisorio.
  - Así el `UNIQUE` de `hostname` reserva el nombre antes de tocar Cloudflare, y un `409` nunca deja registros creados.
  - Un corte a la mitad deja el dominio "sin zona", y `retry` lo arregla (contrato §3.2).
  - **En cuanto `ensure` encuentra la zona, fija `cf_zone_id` y `tls_mode: cloudflare`**, antes de mirar los registros. Quedan así también con `record_exists`, sin `public_ip` o si falla la creación. `http` con zona no existe: solo queda `http` sin token, sin zona, o si no se pudieron listar las zonas.
- **D6. Las operaciones de DNS van de a una:** un `sync.Mutex` (`usecases.Ops`) para crear, editar, borrar, adoptar, reintentar, aplicar un certificado, `EnsurePanelDNS` y cada registro de `UpdateManagedRecords`.
  - Cada una corre con `opCtx = context.WithTimeout(context.WithoutCancel(ctx), limit)`: 30 s (`Timing.DomainOp`), y 20 s en `retry`.
  - **El plazo corre desde antes de esperar el lock.** Si `ctx` ya trae un plazo más corto (el de 2 min del hook), gana ese. Así el hook nunca pasa de sus 2 min, aunque tenga muchos dominios.
  - **La espera del lock no se corta, pero tiene un límite:** nadie toma el lock sin un plazo, y la base es rápida. Una operación que esperó llega con menos tiempo para Cloudflare. Si se le acaba, falla por plazo y guarda el error del dominio, como si Cloudflare no respondiera.
  - Las escrituras a la base usan `dbCtx = context.WithoutCancel(ctx)`: así un plazo vencido en Cloudflare igual guarda el error del dominio.
  - **Fuera del lock**, después de la operación, `syncer.Sync` y `certs.ScheduleFirst` reciben `context.WithoutCancel(ctx)`. Si el cliente se va, el proxy no queda `out_of_sync` por un `ctx` cancelado. `Sync` tiene su propio plazo de 30 s.
  - *Por qué:* contrato §3.2 ("van de a una y no dependen del cliente").
- **D7. "Registro ajeno" es un `A`, un `AAAA` o un `CNAME` de ese nombre** que muelle no maneja.
  - Los que crea muelle llevan el comentario `muelle` (`cloudflare.RecordComment`).
  - Ya está en el contrato §3.1.
  - **Un registro ajeno nunca quita la zona.** El dominio en `record_exists` queda con `cf_zone_id` y `tls_mode: cloudflare`: DNS-01 no depende del registro, y es lo que necesita la migración (spec §16) mientras Dokploy tiene 80/443. `adopt` usa esa zona y no vuelve a buscarla.
- **D8. Zonas:** se listan en cada operación, todas las páginas (`per_page=50`), sin caché y en cualquier estado. La zona es la del sufijo más largo.
  - *Por qué:* son pocas llamadas, y una caché agregaría un estado que se puede quedar viejo cuando el usuario agrega una zona.
- **D9. Verificar un certificado:**
  - **Conexión TCP que falla:** "no se sabe" (`service.ErrCaddyUnreachable`) y no cambia nada.
  - **Handshake que falla, o certificado que no verifica:** el certificado es inválido.
  - **Emisor:** la primera `Organization` del emisor, o su `CommonName` si no tiene.
  - *Por qué:* el contrato §3.1 ("Cuándo no se sabe") separa los dos casos.
- **D10. Cuándo se verifica:**
  - **A los 30 s de crear, adoptar o reintentar:** un timer por dominio, en `Run`.
  - **`pending`:** un barrido cada minuto de todos los `pending`.
  - **Todos los demás:** el job `"certificados"`, cada hora: `pending`, `active` y `error` + `certificate`.
  - *Por qué:* es simple, y sobrevive a un reinicio (el barrido encuentra los `pending` igual).
- **D11. Recrear muelle-caddy:** primero la imagen, después `StopContainer` (10 s), `RemoveContainer(force)` y `CreateContainer`. Las redes de stage se conectan **antes** de `StartContainer`.
  - **La imagen se baja antes de tocar el contenedor que corre** (`InspectImage`, y `PullImage` con la credencial si falta). Si no baja (el registry caído o con rate limit, o una `caddy_image` privada sin credencial: `ErrDenied`), `Ensure` devuelve el error y el contenedor anterior sigue sirviendo los sitios y el panel; el error queda en `GET /proxy`. *Por qué:* en una actualización de muelle (contrato §5.4), detenerlo antes dejaría todo caído durante el pull, y sin proxy (ni panel) si el pull falla: arreglarlo pediría SSH.
  - **Puerto ocupado (`ErrPortInUse`):** se borra enseguida el contenedor que no arrancó, para que nadie lo arranque sin puertos (Docker 29).
  - **Los puertos se prueban antes de crear muelle-caddy** (hallado en T3 Step 7): con un contenedor descartable `<CaddyContainer>-probe` (`muelle-caddy-probe`) de la misma imagen, en la red bridge por defecto, sin redes de Swarm, que corre `caddy version` y se borra siempre. Si el arranque de muelle-caddy falla por un puerto ocupado, Docker 29 no suelta la conexión de Swarm de la red con que se creó (la panel): queda una tarea `attachment` en `running` que no se borra (ni con `docker network disconnect -f`), con una IP de la red, y la red ya no se puede borrar (`is in use by task …`). Con un puerto ocupado, el job `"proxy"` reintentaría cada 5 minutos y se comería la red panel (una /24) en un día. La prueba que falla por otra cosa (sin la red bridge) se registra y no frena la creación. T5: sumar `muelle-caddy-probe` al contrato §3.4 (es un nombre que aparece en el host).
  - **Una red de stage que ya no existe** no es un error, ni al crear ni al conectar en caliente: se registra en el log y se salta.
  - **Con `Enabled` falso:** nunca se borra un contenedor que exista.
- **D12. El job `"proxy"`, en orden:**
  1. Si está `disabled`, no hace nada.
  2. Asegura las redes de stage.
  3. Asegura muelle-caddy, salvo que haya una recreación agendada o frenada por un hook (`Recreator.Hold`, D15): esa la hace `Run`.
  4. Borra las redes huérfanas.
  5. `Sync`.
  6. Con token, Cloudflare (T5): las zonas y el registro del panel. Si cambió la zona del panel, `Sync` otra vez.
  - *Por qué:* un Cloudflare lento no atrasa la configuración de los sitios.
- **D13. Red huérfana:** tiene `muelle.project` y `muelle.stage`, se llama `naming.Network(project, stage)` de esas etiquetas, y no hay un stage así en la base.
  - Se borra la segunda vez seguida que el job la ve (contrato §1.2).
  - Con `Options.NetworkPrefix`, solo se consideran las redes con ese prefijo. Producción: `""`. Integración: `it3<id>`.
- **D14. `Syncer.Sync`:**
  - Arma la configuración desde la base **dentro** del mutex global y la manda. Así un `POST /load` viejo nunca pisa uno nuevo.
  - Con `disabled`, devuelve `nil` sin mandar nada (contrato §3.4).
  - Un fallo deja `out_of_sync` y despierta al `Retrier`: 1 s, 2 s, 4 s… hasta 60 s.
- **D15. `GET /proxy`:**
  - El estado se calcula en el momento: `disabled` si `!Enabled`; si no, `recreating` si hay una recreación agendada o en curso; si no, `out_of_sync` si hay un error del contenedor (gana) o de `/load`; si no, `synced`.
  - La recreación usa generaciones, así un cambio que llega durante una recreación no se pierde.
  - **El plazo de 2 s cuenta desde que el hook vuelve** (contrato §3.5), no desde que empieza.
    - `ProxySettingsChanged` hace `release := recreator.Hold()` al principio y `defer release()`.
    - `Hold` hace `BeginRecreate`, así `GET /proxy` dice `recreating` desde ya, y frena `Recreator.Run`: no recrea mientras haya un `Hold` sin soltar.
    - `release` hace `Schedule`, y recién ahí corren los 2 s.
    - Así el trabajo de Cloudflare del hook (hasta 2 min) nunca deja que muelle-caddy se detenga antes de que salga la respuesta del `PATCH`. Tampoco lo detiene una recreación anterior que estaba esperando su plazo.
- **D16. El JSON de Caddy:**
  - Un servidor `muelle` en `:443`, con `trusted_proxies` estáticos (los rangos de Cloudflare, una lista verificada el 2026-10-14 en cloudflare.com/ips-v4 y ips-v6) y `client_ip_headers`.
  - `trusted_proxies_strict: 1`: `X-Forwarded-For` se lee de derecha a izquierda, así cuenta la IP que vio Cloudflare y no una que el cliente puso adelante. Solo importa si falta `CF-Connecting-IP`. Verificado con `caddy validate` en `caddy:2-alpine`.
  - **Sin placeholders en los ajustes:** Caddy expande placeholders en el `email` del emisor `acme` (verificado: `a{x}@b.cl` da "unrecognized placeholder {x}" y el `/load` falla entero) y caddy-dns/cloudflare los expande en `api_token` (un `{file.…}` leería un archivo). Por eso `Render` devuelve `service.ErrPlaceholderInSettings` si el email o el token tienen `{` o `}`, sin mandar nada. Así se mantiene el invariante "el único placeholder es `{http.vars.client_ip}`". `validate.Email` acepta llaves (son legales en RFC 5322), y el token de admin también (`[\x21-\x7e]`).
  - Se serializa con `jsonv2.Deterministic(true)`: `encoding/json/v2` no ordena las claves de un mapa sin esa opción (`go doc encoding/json/v2 Deterministic`), y los golden dependen del orden.
  - La ruta del panel va primero y siempre como `subroute`: la del `403` solo si hay `panel_allowed_cidrs`. Las de los dominios van ordenadas por hostname; cada ruta es `terminal`.
  - TLS en dos políticas con `subjects` explícitos y ordenados: primero `cloudflare` (si hay token y hosts), después `http`. No hay política sin `subjects`.
  - `email` va solo si no está vacío.
  - El emisor es solo `acme`, es decir, Let's Encrypt. Esto saca el respaldo de ZeroSSL que Caddy pone por defecto (pregunta al dueño, abajo).
  - Se incluyen los dominios en cualquier estado: la ruta de un dominio en error no molesta, y DNS-01 puede emitir igual.
  - Se saltan los dominios cuyo hostname es `panel_domain` o cuya app no está.
- **D17. Adoptar un `AAAA` existente conserva su `proxied`**, como el `A`: adoptar nunca cambia el modo de Cloudflare. Ya está en el contrato §3.2.
- **D18. Sin token**, un cambio que necesita Cloudflare deja el dominio en `error` + `cloudflare` con "agrega el token de Cloudflare en Ajustes". Eso vale para un `PATCH` de `proxied`, para la IP pública (§3.5) y para `retry` de un dominio con registros propios. Ya está en el contrato.
- **D19. Tres `CHECK` en 0003** (los invariantes de los que depende el código):
  - `cf_record_id_aaaa` ⇒ `cf_record_id`;
  - `cf_record_id` ⇒ `cf_zone_id`;
  - `tls_mode = 'cloudflare'` ⇒ `cf_zone_id`.
  - Los `INSERT` de otros módulos en sus tests usan `tls_mode 'http'` sin ids, así que siguen pasando.
- **D20. Sin métodos nuevos en `shared/infra/out/docker`.**
  - Si en la implementación hiciera falta uno, va en `client_ingress.go`, con su fake en `dockertest/fake_ingress.go` y sus tests. Se avisa en la revisión.
  - Para ver la credencial de un `PullImage`, el test envuelve el fake en un tipo propio del test.
- **D21. `target_service`** se valida con `validate.VolumeName("target_service", s)`: es la regla de un servicio compose, y su comentario lo dice. No hace falta una función nueva en `shared/domain/validate`.
- **D22.** En un `PATCH`, `proxied` presente sobre un dominio sin `cf_record_id` es `422`, aunque el valor sea el mismo. Es lo literal del contrato, y el panel no lo manda.
- **D23. `retry` decide en este orden:**
  1. `error` con `cloudflare` o `record_exists`: rehace el DNS. Si el registro ajeno sigue, queda en `record_exists` con su zona y `tls_mode: cloudflare` (D7).
  2. Si no, sin zona y sin registros propios: busca la zona y, si la encuentra, crea los registros.
  3. Si no: verifica el certificado.
  - Después de los pasos 1 y 2, agenda la verificación de los 30 s.
- **D24.** Con `disabled`, `GET /proxy` no consulta Docker. Ya está en el contrato §3.3.
- **D25. `cftest` y `caddytest`** son paquetes (no archivos `_test`), porque los usan los tests de varios paquetes de ingress.
- **D26. `proxied` sin zona:** se guarda `false` si no hay token o no hay zona. Si no se pudieron listar las zonas, se guarda el pedido, para que `retry` lo use.
- **D27. El registro del panel:**
  - Se crea solo si no hay ningún `A`, `AAAA` ni `CNAME` de ese nombre que muelle no maneje. Va con `proxied: false` y el comentario `muelle`.
  - Su `AAAA` se crea solo si muelle maneja el `A`.
  - Nunca se cambia su `proxied`.
  - Un id guardado que ya no existe, o que es de otro nombre, se olvida y no se borra (contrato §3.4).
  - **Un id guardado que el token ya no puede leer también se olvida**, sin borrarlo.
    - Es un `Record(store.ZoneID, id)` que da un `*DNSError` (un `400` o un `403`, no solo el `404`): un token nuevo de otra cuenta, o una zona que el token ya no ve.
    - Las zonas ya se listaron con ese mismo token, así que el token sirve: lo que no ve es ese registro.
    - Olvidar siempre es seguro: nunca se borra nada, y crear sigue mirando si hay otro registro de ese nombre en la zona nueva.
    - Si no, cada job `"proxy"` fallaría en esa lectura, y el registro del panel y `panel_cf_zone_id` nunca pasarían a la zona nueva.
    - Un `ErrCloudflareUnavailable` (sin red, 5xx, 429) no olvida nada: vuelve con el error.
    - Ya está en el contrato §3.4 ("TLS del panel").
  - Excepción (D29): si el `A` guardado ya no existe y el `AAAA` guardado sí, y es de ese nombre, el `AAAA` se borra. Si no, ese `AAAA` contaría como "otro registro", y el panel quedaría sin `A`.
- **D28. `cloudflare_zones`** se actualiza dentro de `EnsurePanelDNS`, que ya listó las zonas. Se guarda con `SaveZones(token, n)`, un compare-and-set en una transacción: si el token guardado ya no es el que se usó, no se guarda (contrato §3.6).
- **D29. Si desaparece el `A` propio, muelle tampoco maneja más el `AAAA`.**
  - Pasa cuando Cloudflare da `404` al actualizar o leer el `A` guardado. Entonces muelle borra en Cloudflare el `AAAA` propio (un `404` ahí no es error) y olvida los dos ids. Después sigue la regla de cada caso: `error` + `cloudflare` en `UpdateManagedRecords` y `setProxied`, y crear de nuevo en `ensure` y `retry`.
  - Si el borrado del `AAAA` falla, igual se olvidan los dos ids, y el mensaje lo dice. Ese `AAAA` pasa a ser ajeno, y el reintento lo muestra como `record_exists` (→ Adoptar).
  - Vale igual para el registro del panel (`panel_cf_record_id` y `panel_cf_record_id_aaaa`).
  - *Por qué:* el `CHECK` de D19 (`cf_record_id_aaaa` ⇒ `cf_record_id`) haría fallar el `repo.Update`, y el error del dominio no se guardaría. Si el `AAAA` solo se olvidara, el reintento lo vería como ajeno y nunca volvería a crear los registros.
- **D30. Los ajustes se leen dentro del lock** en cada `Ops.Do` de `UpdateManagedRecords` y en `EnsurePanelDNS`, junto con el dominio (uno que ya no está se salta).
  - Los registros van a los ajustes guardados en ese momento, no al `next` que recibió el hook. Así, de dos `PATCH /settings` seguidos, el hook que termina último nunca deja la IP del primero.
  - `old`/`next` solo deciden si hay algo que hacer.
- **D31. `VerifyToken` mira `result.status`.** `GET /user/tokens/verify` responde `200` también con un token `disabled` o `expired` (verificado en la documentación de la API de Cloudflare). Un estado que no es `active` es un `*DNSError` (rechazado), como un `401`.
- **D32. Adoptar cuando el único registro ajeno es un `AAAA`** (ni `A` ni `CNAME`; el contrato §3.2 no lo fija):
  - muelle crea su `A` con el `proxied` de ese `AAAA`, que se copia al dominio: adoptar nunca cambia el modo de Cloudflare (D17);
  - con `public_ipv6`, después adopta el `AAAA`. Va **después** del `A`: si el `A` falla, no queda un `cf_record_id_aaaa` sin `cf_record_id` (D19);
  - sin `public_ipv6`, el `AAAA` no se toca y sigue ajeno, igual que cuando hay un `A` (contrato §3.2).
  - Con `public_ipv6` y más de un `AAAA`, no se toca ninguno: `error` + `cloudflare` con "deja uno solo". Sin `public_ipv6`, los `AAAA` no cuentan.
  - Ya está en el contrato §3.2 (`adopt`).

## Mapa de archivos

```
src/
├── shared/
│   ├── domain/entity/panel_dns.go                  (T1) SettingPanelCFZoneID, …RecordID, …RecordIDAAAA
│   └── infra/
│       ├── out/sqlite/migrations/0003_ingress.sql  (T1) + CHECK (D19); test en migrations_test.go
│       └── container/
│           ├── container.go                        (T2) WithIngress, build recibe options, Options a ingress.New
│           ├── ports_test.go                       (T2) TestContractProxySettings con cftest
│           └── ingress_test.go                     (T5) contratos de ingress con el contenedor
└── internal/ingress/
    ├── module.go                                   (T2…T5) Deps.Options, cableado, Run, hooks
    ├── options.go                                  (T2) Options, Timing, withDefaults
    ├── ingresstest/ingresstest.go                  (T1) siembra de stages, apps, settings, registries
    ├── integration_env_test.go                     (T2) //go:build integration: itEnv (redes, volúmenes, puente, backend, run)
    ├── integration_test.go                         (T2…T4) //go:build integration
    ├── domain/
    │   ├── entity/domain.go                        (T1) Domain, Status, ErrorCode, TLSMode, transiciones
    │   ├── entity/records.go                       (T1) DNSRecord, Records(host, ip4, ip6)
    │   ├── repository/domains.go                   (T1) Domains
    │   └── service/
    │       ├── ports.go                            (T1…T4) Networks, Settings, Catalog, Registries, DNS, CaddyAdmin, CaddyRuntime, CertProbe
    │       ├── credential.go                       (T3) Credential oculta la contraseña (String, GoString, LogValue, MarshalJSON)
    │       ├── errors.go                           (T1…T3)
    │       ├── zones.go                            (T1) MatchZone
    │       ├── proxy_config.go                     (T2) ProxyConfig, BuildProxyConfig
    │       └── certs.go                            (T4) CertObservation, ApplyCert
    ├── app/
    │   ├── dto/dto.go                              (existe) StageRef
    │   ├── dto/proxy.go                            (T3) ProxyOutput
    │   ├── dto/domains.go                          (T4) entradas y DomainOutput
    │   └── usecases/
    │       ├── stage_network.go                    (T2) recibe el nombre del contenedor
    │       ├── proxy_state.go, sync.go             (T2)
    │       ├── caddy.go, orphans.go, retrier.go,
    │       │   recreator.go, proxy_job.go,
    │       │   proxy_status.go, sleep.go           (T3)
    │       ├── ops.go, messages.go, dns.go,
    │       │   domains.go, adopt.go,
    │       │   retry_domain.go, certs.go           (T4)
    │       └── token.go, panel_dns.go,
    │           records_update.go,
    │           settings_changed.go                 (T5)
    └── infra/
        ├── in/http/{errors,handler,proxy,domains,body}.go (T3, T4)
        ├── in/cron/jobs.go                         (T3 "proxy", T4 "certificados")
        └── out/
            ├── repository/sqlite/domains.go        (T1)
            └── service/
                ├── networks.go                     (T3) + List y Labels
                ├── settings.go                     (T1) + PanelDNS, SavePanelDNS, SaveZones
                ├── catalog.go, registries.go       (T1)
                ├── certs.go                        (T4) TLSProbe
                ├── cloudflare/client.go            (T1)
                ├── cloudflare/cftest/server.go     (T1)
                ├── caddy/config.go, ranges.go,
                │   admin.go, testdata/*.golden.json (T2)
                ├── caddy/caddytest/admin.go        (T2)
                └── caddy/container.go              (T3) Runtime
```

---

### Task T1: Datos de ingress y cliente de Cloudflare

**Objetivo:** Todo lo que guarda y lee ingress, y el DNS:
- la entidad `Domain` con sus estados;
- la migración con sus invariantes;
- el repositorio de `domains`;
- los adaptadores SQL de catalog, registries y settings (con las claves del panel y `cloudflare_zones`);
- el cliente de Cloudflare con su servidor falso.

No cambia ningún comportamiento visible: el módulo sigue siendo esqueleto.

**Files:**
- Modify: `src/shared/infra/out/sqlite/migrations/0003_ingress.sql`, `src/shared/infra/out/sqlite/migrations_test.go` (solo `insertDomain`/`TestDomainsSchema`)
- Create: `src/shared/domain/entity/panel_dns.go`, `panel_dns_test.go`
- Create: `src/internal/ingress/domain/entity/{domain,records}.go` + `domain_test.go`, `records_test.go`
- Create: `src/internal/ingress/domain/repository/domains.go`
- Modify: `src/internal/ingress/domain/service/ports.go`, `errors.go`
- Create: `src/internal/ingress/domain/service/zones.go` + `zones_test.go`
- Create: `src/internal/ingress/infra/out/repository/sqlite/domains.go` + `domains_test.go`
- Create: `src/internal/ingress/infra/out/service/{catalog,registries}.go` + tests
- Modify: `src/internal/ingress/infra/out/service/settings.go` + `settings_test.go`
- Create: `src/internal/ingress/infra/out/service/cloudflare/client.go` + `client_test.go`
- Create: `src/internal/ingress/infra/out/service/cloudflare/cftest/server.go` + `server_test.go`
- Create: `src/internal/ingress/ingresstest/ingresstest.go`

- [x] **Step 1: Migración 0003 — los invariantes (D19).**

  **Tests primero** (`TestDomainsSchema`):
  - `insertDomain` suma la columna `cf_zone_id` (un parámetro más, `nil` en las filas `http`). La fila `cloudflare` del test pasa a tener `"z1"`.
  - Se agregan con `mustFail`:
    - `tls_mode cloudflare` sin zona;
    - un `cf_record_id` sin zona;
    - un `cf_record_id_aaaa` sin `cf_record_id` (para este, un `INSERT` propio con las columnas de ids).
  - Se agrega con `mustExec`: una fila completa (zona, `A` y `AAAA`) que pasa.
  - Verlo fallar.

  **Implementar:** al final de `CREATE TABLE domains`, antes del `)`:
  ```sql
  -- Invariantes del DNS (contrato §3.1): muelle maneja el AAAA solo si maneja
  -- el A, un registro siempre está en una zona, y el TLS por Cloudflare
  -- necesita la zona.
  CHECK (cf_record_id_aaaa IS NULL OR cf_record_id IS NOT NULL),
  CHECK (cf_record_id IS NULL OR cf_zone_id IS NOT NULL),
  CHECK (tls_mode = 'http' OR cf_zone_id IS NOT NULL)
  ```
  `go test -race ./src/shared/infra/out/sqlite/ ./src/internal/... ./src/shared/infra/container/` pasa: los otros `INSERT INTO domains` del repo usan `'http'` y no se rompen.

  **Commit:** `feat(sqlite): 0003 garantiza que el AAAA va con el A, los registros con su zona y el TLS de Cloudflare con zona`

- [x] **Step 2: Las claves del panel en `shared/domain/entity`.**

  **Test primero** (`panel_dns_test.go`): `TestPanelDNSKeys`.
  - Las tres constantes valen `panel_cf_zone_id`, `panel_cf_record_id` y `panel_cf_record_id_aaaa`.
  - Ninguna está en `ProxySettingKeys`, así admin nunca las lee ni las escribe en `/settings`.

  **Implementar** `panel_dns.go`:
  ```go
  // Claves de settings del registro DNS del panel (contrato §3.6). Las escribe
  // solo ingress; GET /settings no las muestra y PATCH /settings no las acepta.
  const (
  	// SettingPanelCFZoneID es la zona de panel_domain, si cae en una visible
  	// para el token; decide el TLS del panel.
  	SettingPanelCFZoneID = "panel_cf_zone_id"
  	// SettingPanelCFRecordID y SettingPanelCFRecordIDAAAA son los registros A
  	// y AAAA del panel que creó muelle; no están si no creó ninguno.
  	SettingPanelCFRecordID     = "panel_cf_record_id"
  	SettingPanelCFRecordIDAAAA = "panel_cf_record_id_aaaa"
  )
  ```

  **Commit:** `feat(entity): las claves panel_cf_* del registro DNS del panel`

- [x] **Step 3: La entidad `Domain`.**

  **Tests primero** (`domain_test.go`, `records_test.go`):
  - `TestNewDomain`:
    - uno válido queda `pending`, `TLSHTTP`, sin ids, con `CreatedAt == UpdatedAt == now` y con el `Proxied` pedido;
    - los errores son `*validate.Error` con el campo correcto: `hostname` (vacío, `API.x.cl`, `*.x.cl`, `x.cl.`, `1.2.3.4`), `target_service` (`Api`, `-a`, 17 caracteres), `target_port` (0, 65536).
  - `TestDomainTransitions`:
    - `Fail(CodeCloudflare, "m", t1)` deja `error`, el código y el mensaje, con `UpdatedAt = t1`;
    - `MarkPending(t2)` limpia código y mensaje, con `UpdatedAt = t2`;
    - `MarkActive(t3)` igual;
    - `Touch(t4)` solo mueve `UpdatedAt`;
    - `SetCertificate(issuer, exp)` no mueve `UpdatedAt` (contrato §3.1).
  - `TestSetTarget`: un destino igual devuelve `changed=false` y no toca `UpdatedAt`; uno distinto, `true`; uno inválido, el `*validate.Error` del campo.
  - `TestRecords`:
    - `("a.x.cl", "203.0.113.24", "2001:db8::24")` da `[A, AAAA]` con `name` el hostname;
    - solo IPv4 da `[A]`;
    - sin IPs da `[]`, **no nil**.

  **Implementar** `domain.go`. Lo que importa:
  ```go
  package entity

  type Status string     // StatusPending "pending", StatusActive "active", StatusError "error"
  type ErrorCode string  // CodeRecordExists "record_exists", CodeCloudflare "cloudflare", CodeCertificate "certificate"
  type TLSMode string    // TLSCloudflare "cloudflare", TLSHTTP "http"

  // Domain es una fila de domains (§5.1, contrato §3.1). Un string vacío es NULL.
  type Domain struct {
  	ID, AppID      int64
  	Hostname       string
  	TargetService  string
  	TargetPort     int
  	Proxied        bool
  	TLSMode        TLSMode
  	CFZoneID       string
  	CFRecordID     string // el A que maneja muelle
  	CFRecordIDAAAA string // el AAAA que maneja muelle (nunca sin CFRecordID)
  	Status         Status
  	ErrorCode      ErrorCode // solo con StatusError
  	LastError      string    // solo con StatusError
  	CertIssuer     string
  	CertExpiresAt  *time.Time
  	CreatedAt      time.Time
  	UpdatedAt      time.Time // el último cambio (contrato §3.1); el plazo de 10 min cuenta desde acá
  }

  func NewDomain(appID int64, hostname, service string, port int, proxied bool, now time.Time) (Domain, error)
  func ValidateTarget(service string, port int) error // validate.VolumeName("target_service") y validate.Port("target_port")
  func (d *Domain) SetTarget(service string, port int, now time.Time) (changed bool, err error)
  func (d *Domain) Fail(code ErrorCode, msg string, now time.Time)
  func (d *Domain) MarkPending(now time.Time)
  func (d *Domain) MarkActive(now time.Time)
  func (d *Domain) Touch(now time.Time)
  func (d *Domain) SetCertificate(issuer string, expires time.Time) (changed bool)
  func (d Domain) Managed() bool { return d.CFRecordID != "" }
  ```
  `records.go`: `type DNSRecord struct{ Type, Name, Content string }` y `func Records(hostname, ipv4, ipv6 string) []DNSRecord`.

  **Commit:** `feat(ingress): la entidad Domain con sus estados y los registros que necesita`

- [x] **Step 4: Puertos y zona por sufijo.**

  **Tests primero** (`zones_test.go`): `TestMatchZone`.
  - `api.a.example.com`, con las zonas `example.com` y `a.example.com`, da `a.example.com`.
  - `example.com` cae en `example.com` (el ápice).
  - `badexample.com` no cae en `example.com`.
  - Mayúsculas en el nombre de la zona: se compara en minúsculas.
  - Sin zonas: `false`.

  **Implementar** en `domain/service`:
  - `zones.go`: `func MatchZone(hostname string, zones []Zone) (Zone, bool)`.
  - `ports.go`: se agregan estos puertos (los existentes no cambian):
  ```go
  // Zone es una zona de Cloudflare visible para el token.
  type Zone struct{ ID, Name string }
  // Record es un registro DNS de Cloudflare.
  type Record struct {
  	ID, Type, Name, Content string
  	Proxied                 bool
  }
  // RecordChange: los campos nil no cambian.
  type RecordChange struct {
  	Type, Content *string
  	Proxied       *bool
  }
  // DNS es la API de Cloudflare (adaptador: infra/out/service/cloudflare). Cada
  // método recibe el token: los ajustes pueden cambiar entre una llamada y otra.
  // Errores: un error que envuelve entity.ErrCloudflareUnavailable (sin red, 5xx,
  // 429, respuesta ilegible), ErrDNSNotFound (404) o un *DNSError (Cloudflare
  // rechazó el pedido). Ninguno trae el token.
  type DNS interface {
  	VerifyToken(ctx context.Context, token string) error
  	Zones(ctx context.Context, token string) ([]Zone, error)              // todas las páginas
  	Records(ctx context.Context, token, zoneID, name string) ([]Record, error) // todos los tipos con ese nombre exacto
  	Record(ctx context.Context, token, zoneID, id string) (Record, error)
  	CreateRecord(ctx context.Context, token, zoneID string, r Record) (Record, error) // ttl automático y el comentario "muelle"
  	UpdateRecord(ctx context.Context, token, zoneID, id string, c RecordChange) (Record, error)
  	DeleteRecord(ctx context.Context, token, zoneID, id string) error
  }

  // App es una app de catalog como la necesita ingress.
  type App struct {
  	ID                   int64
  	Project, Stage, Slug string
  	Kind                 string // KindService o KindCompose
  }
  const (
  	KindService = "service"
  	KindCompose = "compose"
  )
  // Stage es un stage de catalog.
  type Stage struct{ Project, Stage string }
  // Catalog lee apps, stages y projects (de catalog, §4.1 regla 1).
  type Catalog interface {
  	App(ctx context.Context, project, stage, app string) (App, error) // repository.ErrNotFound
  	Apps(ctx context.Context) (map[int64]App, error)
  	Stages(ctx context.Context) ([]Stage, error) // ordenados por proyecto y stage
  }

  // Credential es la credencial de un registry (tabla registries, de deploy).
  type Credential struct{ Host, Username, Password string }
  type Registries interface {
  	// Credential devuelve nil, nil si no hay una para ese host.
  	Credential(ctx context.Context, host string) (*Credential, error)
  }

  // PanelDNS son las claves panel_cf_* (contrato §3.6); "" = la clave no está.
  type PanelDNS struct{ ZoneID, RecordID, RecordIDAAAA string }
  ```
  - `Settings` crece (en el commit del Step 6, junto con su adaptador: así cada commit compila, porque `ProxySettingsReader` afirma `var _ service.Settings`):
  ```go
  type Settings interface {
  	ProxySettings(ctx context.Context) (entity.ProxySettings, error)
  	PanelDNS(ctx context.Context) (PanelDNS, error)
  	// SavePanelDNS escribe las claves no vacías y borra las vacías.
  	SavePanelDNS(ctx context.Context, p PanelDNS) error
  	// SaveZones guarda cloudflare_zones solo si el token guardado sigue siendo
  	// token (en una transacción); saved dice si lo guardó.
  	SaveZones(ctx context.Context, token string, zones int) (saved bool, err error)
  }
  ```
  - `errors.go` suma:
  ```go
  var (
  	// ErrNothingToAdopt: adopt sobre un dominio que no está en error con record_exists (409 nothing_to_adopt).
  	ErrNothingToAdopt = errors.New("no hay un registro que adoptar")
  	// ErrHostnameTaken: el hostname ya lo usa un dominio de muelle (409 conflict).
  	ErrHostnameTaken = fmt.Errorf("ese hostname ya está en uso en muelle: %w", repository.ErrConflict)
  	// ErrDNSNotFound: Cloudflare respondió 404 (el registro o la zona no existen).
  	ErrDNSNotFound = errors.New("Cloudflare: no existe")
  )
  // DNSError es un pedido que Cloudflare rechazó (un 4xx que no es 404 ni 429,
  // o success: false). Message es el de Cloudflare.
  type DNSError struct {
  	Status, Code int
  	Message      string
  }
  func (e *DNSError) Error() string // "Cloudflare rechazó el pedido (<status>): <message>"
  ```
  - `repository/domains.go`:
  ```go
  type Domains interface {
  	Create(ctx context.Context, d *entity.Domain) error // asigna d.ID; service.ErrHostnameTaken si el hostname ya existe
  	Get(ctx context.Context, hostname string) (entity.Domain, error) // repository.ErrNotFound
  	ListByApp(ctx context.Context, appID int64) ([]entity.Domain, error) // por hostname; [] si no hay
  	List(ctx context.Context) ([]entity.Domain, error)                   // todos, por hostname
  	Update(ctx context.Context, d entity.Domain) error                   // todas las columnas menos id, app_id, hostname y created_at; ErrNotFound
  	Delete(ctx context.Context, id int64) error                          // ErrNotFound
  }
  ```
  Imports: la interfaz de `domain/repository` solo nombra `service.ErrHostnameTaken` en su comentario y no importa `service`; quien lo devuelve es el repositorio SQLite (infra). `ErrHostnameTaken` vive en `service` y envuelve el `ErrConflict` compartido (`shared/domain/repository`), así que no hay ciclo.

  **Commit:** `feat(ingress): los puertos de Cloudflare, catalog, registries y settings, y la zona por sufijo`

- [x] **Step 5: El repositorio de `domains`.**

  **Tests primero** (`infra/out/repository/sqlite/domains_test.go`), con la app sembrada por `ingresstest` (Step 8: créalo primero o en el mismo paso):
  - `TestDomainsCRUD`:
    - `Create` asigna el id;
    - `Get` devuelve lo mismo: ida y vuelta de todos los campos, también los NULL (`""` y `CertExpiresAt == nil`) y los llenos (`CertExpiresAt` a ms en UTC);
    - `ListByApp` y `List` ordenan por hostname y dan `[]` sin filas;
    - `Update` cambia estado, ids y certificado;
    - `Delete`.
  - `TestDomainsHostnameTaken`: el mismo hostname en otra app da `errors.Is(err, service.ErrHostnameTaken)` y también `repository.ErrConflict`.
  - `TestDomainsNotFound`: `Get`, `Update` y `Delete` de algo que no existe dan `repository.ErrNotFound`.
  - `TestDomainsInvariants`: `Update` con un `CFRecordIDAAAA` sin `CFRecordID` falla (el `CHECK` del Step 1).

  **Implementar** con `sqlite.Millis`/`sqlite.Time`/`sqlite.NullMillis`/`sqlite.NullTime`, `sql.NullString` para los NULL y `sqlite.IsUnique` → `service.ErrHostnameTaken`.

  **Commit:** `feat(ingress): repositorio SQLite de domains`

- [x] **Step 6: Adaptadores de catalog, registries y settings.**

  **Tests primero:**
  - `TestCatalogApp`: encuentra la app con su `Kind`; una app de otro stage o que no existe da `repository.ErrNotFound`.
  - `TestCatalogApps`: el mapa id → `App`, con los slugs.
  - `TestCatalogStages`: todos, ordenados.
  - `TestRegistriesCredential`:
    - con una fila cifrada con `registries:password:<id>` devuelve la credencial;
    - otro host da `nil, nil`;
    - una contraseña cifrada con otra AAD da error (sin la contraseña en el texto).
  - `TestPanelDNSRoundTrip`:
    - vacío da `PanelDNS{}`;
    - guardar los tres y leerlos;
    - guardar con `RecordIDAAAA: ""` borra la fila de esa clave (`SELECT COUNT(*)`).
  - `TestSaveZonesCompareAndSet`:
    - con el token guardado `t1`, `SaveZones(t1, 3)` da `saved` y la fila vale `"3"`;
    - `SaveZones(t2, 5)` da `!saved` y la fila sigue en `"3"`;
    - sin token guardado da `!saved`.

  **Implementar:**
  - `catalog.go` (`NewCatalogReader(db)`): las consultas son como `deploy/infra/out/service/apps.go`, con `JOIN stages` y `JOIN projects`.
  - `registries.go` (`NewRegistriesReader(db, cipher)`): `SELECT id, username, password FROM registries WHERE host = ?`. La AAD es `fmt.Sprintf("registries:password:%d", id)`.
  - `settings.go`: suma los métodos a `ProxySettingsReader`. El constructor no cambia de nombre: lo usa `container/ports_test.go`.
    - `SavePanelDNS` hace `INSERT … ON CONFLICT (key) DO UPDATE` con `encrypted = 0`, o `DELETE`.
    - `SaveZones` usa `db.InTx`: lee y descifra el token con su AAD, compara, y escribe `cloudflare_zones`.

  **Commit:** `feat(ingress): leer apps, stages y registries, y escribir las claves del panel y cloudflare_zones`

- [x] **Step 7: El cliente de Cloudflare y su servidor falso.**

  **`cftest` primero** (`cloudflare/cftest/server.go`), con su test `server_test.go`. Es un servidor `httptest` con estado en memoria:
  ```go
  type Server struct { /* httptest.Server, mu, tokens, zones, records por zona, fallas, llamadas */ }
  func New(t testing.TB) *Server                 // se cierra con t.Cleanup
  func (s *Server) URL() string                  // la base, para Options.CloudflareURL y cloudflare.New
  func (s *Server) AddToken(token string)        // /user/tokens/verify lo da "active"
  func (s *Server) AddAccountToken(token string) // /user/tokens/verify responde 401, pero lista zonas
  func (s *Server) SetTokenStatus(token, status string) // "disabled" o "expired": verify responde 200 con ese status, y lo demás 401
  func (s *Server) AddZone(id, name string)
  func (s *Server) PutRecord(zoneID string, r service.Record) string // devuelve el id (r.ID vacío: uno nuevo)
  func (s *Server) Records(zoneID string) []service.Record          // copia, ordenada por nombre y tipo
  func (s *Server) Comment(zoneID, id string) string                // el comentario guardado
  func (s *Server) SetDown(down bool)                               // todo responde 503
  func (s *Server) FailNext(method, pathContains string, status, code int, msg string) // una vez
  func (s *Server) Hang(d time.Duration)                            // demora cada respuesta (para los plazos)
  func (s *Server) Calls() []string                                 // "GET /zones?page=1&per_page=50", "POST /zones/z1/dns_records"…
  ```
  - **Implementa** `GET /user/tokens/verify`, `GET /zones` (con `page`, `per_page` y `result_info.total_pages`), `GET`/`POST /zones/{z}/dns_records` (filtra por `name.exact` y `type`) y `GET`/`PATCH`/`DELETE /zones/{z}/dns_records/{id}`.
  - **Respuestas:**
    - un token desconocido recibe `401` con `{"success":false,"errors":[{"code":1000,"message":"Invalid API Token"}]}`;
    - una zona o un registro que no existen reciben `404` con `errors`;
    - `POST` guarda `comment`, y `PATCH` cambia solo los campos que vienen (también `type`).

  **Tests del cliente primero** (`client_test.go`, siempre contra `cftest` o un `httptest` propio):
  - `TestVerifyToken`:
    - un token activo da `nil`;
    - uno rechazado da `*service.DNSError` con `Status 401`;
    - uno con `SetTokenStatus(t, "disabled")` o `"expired"` da `*service.DNSError` (`Status 200`), con el estado en el mensaje (D31);
    - el header es `Authorization: Bearer <token>`;
    - el texto de ningún error contiene el token.
  - `TestUnavailable`: dan `errors.Is(err, entity.ErrCloudflareUnavailable)`:
    - `SetDown` (503);
    - un `429`;
    - un servidor cerrado (sin red);
    - un cuerpo que no es JSON;
    - un `ctx` vencido.
  - `TestZonesPaginates`: 120 zonas en 3 páginas llegan todas, y `Calls` muestra `page=1..3`.
  - `TestRecordsByName`: manda `name.exact=<host>` y `per_page=100`, y devuelve los `A`, `AAAA` y `CNAME` (también un `TXT`) de ese nombre y de ningún otro.
  - `TestCreateRecord`: el cuerpo es `{"type","name","content","ttl":1,"proxied","comment":"muelle"}`, y devuelve el id.
  - `TestUpdateRecord`: un `RecordChange` con solo `Content` manda solo `content`; con `Type` y `Content`, los dos (el `CNAME` pasa a `A`).
  - `TestRecordNotFound`: `Record`, `UpdateRecord` y `DeleteRecord` de un id que no existe dan `service.ErrDNSNotFound`.
  - `TestSuccessFalse`: un `200` con `success:false` da `*service.DNSError`.

  **Implementar** `cloudflare/client.go`:
  ```go
  const (
  	DefaultBaseURL = "https://api.cloudflare.com/client/v4"
  	RecordComment  = "muelle"
  )
  type Client struct {
  	base string
  	http *http.Client // Timeout 15 s
  }
  var _ service.DNS = (*Client)(nil)
  func New(baseURL string) *Client // "" → DefaultBaseURL
  // do arma la petición con el token, limita la respuesta a 1 MiB, lee el sobre
  // {success, errors, result, result_info} y clasifica: sin respuesta, 5xx, 429
  // o JSON ilegible → ErrCloudflareUnavailable; 404 → ErrDNSNotFound (envolviendo
  // el *DNSError); otro 4xx o success:false → *DNSError.
  func (c *Client) do(ctx context.Context, token, method, path string, q url.Values, body, out any) (pages int, err error)
  ```
  Usa `encoding/json/v2`, que ignora los campos desconocidos por defecto. Los mensajes de error nombran la operación y nunca el token.

  `VerifyToken` lee `result.status`: si no es `active`, devuelve `&service.DNSError{Status: 200, Message: "el token está desactivado"}` (o "vencido", o el estado tal cual si es otro) (D31).

  **Commit** (dos): `test(ingress): Cloudflare falso para los tests (cftest)` y `feat(ingress): cliente mínimo de Cloudflare (token, zonas y registros DNS)`

  **Lo que sumó la implementación** (para las tareas que siguen):
  - `cftest.URL()` termina en `cftest.BasePath` (`/client/v4`), como la API real, y `Calls()` lo quita.
  - `cftest` imita los conflictos de Cloudflare en `POST` y `PATCH`: un `A`/`AAAA` no convive con un `CNAME` del mismo nombre, ni dos `CNAME` (`400`, código `81053`), y dos registros idénticos tampoco (`81058`). Varios `A` del mismo nombre sí. `PutRecord` no revisa nada.
  - `SetDown` responde `503` con un cuerpo HTML, como un corte real.
  - El cliente no sigue redirects (el token nunca va a otro host): un `3xx` es `ErrCloudflareUnavailable`.
  - Un token vacío o que no es ASCII visible no se manda: es un `*DNSError` (`Status 400`) sin llamar a Cloudflare.
  - Un mensaje de Cloudflare que trajera el token lo trae como `***`. Un `ctx` vencido también envuelve `context.DeadlineExceeded`.
  - `Records` también pagina (`per_page=100`), y vuelve a filtrar por el nombre.
  - Los errores llevan la operación adelante: `Cloudflare, listando las zonas: …`.

- [x] **Step 8: `ingresstest`.** Helpers para los tests de varios paquetes de ingress (`testing.TB`, con `t.Fatal` ante cualquier error):
  ```go
  func Stage(t testing.TB, db *sqlite.DB, project, stage string)               // crea el proyecto y el stage si faltan (sin apps)
  func App(t testing.TB, db *sqlite.DB, project, stage, app, kind string) int64 // usa Stage; spec '{}' o compose_yaml 'services: {}'
  func SetProxySettings(t testing.TB, db *sqlite.DB, c cipher.Cipher, p entity.ProxySettings) // Encode() + el token cifrado (o lo borra)
  func Registry(t testing.TB, db *sqlite.DB, c cipher.Cipher, host, user, pass string)
  ```
  **Commit:** junto con el Step 5, o aparte: `test(ingress): ingresstest siembra apps, ajustes y registries`

- [x] **Step 9: Cierre de T1.** Las verificaciones de "Convenciones" pasan. No hay tests de integración en esta tarea.

---

### Task T2: La configuración de Caddy y `SyncRoutes`

**Objetivo:** De la base a Caddy:
- `BuildProxyConfig` (puro) y su JSON (golden);
- el cliente de la admin API por el socket unix, con los errores sin el token;
- `ProxyState` y `Syncer`: un `POST /load` a la vez, y nunca uno viejo;
- `Module.SyncRoutes` deja de ser esqueleto;
- `Options`, y las redes de stage conectan el muelle-caddy que dice `Options` (nunca el nombre fijo en un test).

Un test de integración prueba que **Caddy real** acepta la configuración y enruta.

**Files:**
- Create: `src/internal/ingress/domain/service/proxy_config.go` + `proxy_config_test.go`; Modify: `ports.go`, `errors.go`
- Create: `src/internal/ingress/infra/out/service/caddy/{config,ranges,admin}.go` + `config_test.go`, `admin_test.go`, `testdata/*.golden.json`
- Create: `src/internal/ingress/infra/out/service/caddy/caddytest/admin.go` + `admin_test.go`
- Create: `src/internal/ingress/app/usecases/{proxy_state,sync}.go` + tests
- Create: `src/internal/ingress/options.go` + `options_test.go`
- Modify: `src/internal/ingress/app/usecases/stage_network.go` + `stage_network_test.go`
- Modify: `src/internal/ingress/module.go`, `module_test.go`
- Modify: `src/shared/infra/container/container.go` (`WithIngress` y `build`), `ports_test.go` (solo `TestContractProxySettings`)
- Create: `src/internal/ingress/integration_env_test.go`, `integration_test.go` (`//go:build integration`)

- [x] **Step 1: `BuildProxyConfig`.**

  **Tests primero** (`proxy_config_test.go`):
  - `TestBuildProxyConfigRoutes`:
    - dos apps (`auditfy/staging/api`, de formulario, y `auditfy/staging/infra`, compose) y tres dominios dan `Routes` ordenadas por hostname;
    - el upstream es `naming.Upstream(naming.Service(p, s, a, svc), port)` (`auditfy_staging_api_api:3000`);
    - también va un dominio en `error`.
  - `TestBuildProxyConfigPanel`:
    - el `Panel` va al upstream `muelle_muelle:8080` con los `AllowedCIDRs` de los ajustes;
    - sin `panel_domain`, el `Panel` es nil.
  - `TestBuildProxyConfigTLS`:
    - con token y `panelZoneID`, el panel es `cloudflare`;
    - con token y sin zona, `http`;
    - sin token, todos `http`, aunque el dominio diga `cloudflare` (contrato §3.4).
  - `TestBuildProxyConfigSkips`: se salta un dominio cuyo hostname es el del panel, y uno cuya app no está en el mapa.
  - `TestProxyConfigHidesToken`: `fmt.Sprint(cfg)`, `fmt.Sprintf("%#v", cfg)`, un `slog.JSONHandler` con `cfg` como atributo y `json.Marshal(cfg)` no contienen el token.

  **Implementar:**
  ```go
  // Route es la ruta de un dominio: hostname → upstream por la red del stage.
  type Route struct {
  	Hostname, Upstream string
  	TLS                entity.TLSMode
  }
  // PanelRoute es la ruta del panel (contrato §3.4).
  type PanelRoute struct {
  	Hostname, Upstream string
  	AllowedCIDRs       []string // [] = cualquiera
  	TLS                entity.TLSMode
  }
  // ProxyConfig es la configuración completa de Caddy como datos (D1). El token
  // va en claro: String, GoString, LogValue y MarshalJSON lo ocultan.
  type ProxyConfig struct {
  	Panel           *PanelRoute
  	Routes          []Route
  	ACMEEmail       string
  	CloudflareToken string
  	InternalCA      bool // solo integración: la CA interna de Caddy en vez de ACME
  }
  func BuildProxyConfig(p shared.ProxySettings, panelZoneID string, domains []entity.Domain, apps map[int64]App, internalCA bool) ProxyConfig
  ```
  Usa el alias `shared` para `src/shared/domain/entity` y `entity` para el de ingress.

  También van en `ports.go`/`errors.go`:
  ```go
  // CaddyAdmin es la admin API de muelle-caddy (adaptador: caddy.Admin).
  type CaddyAdmin interface {
  	// Load manda la configuración completa (POST /load); force agrega
  	// Cache-Control: must-revalidate (recarga aunque sea igual).
  	Load(ctx context.Context, cfg ProxyConfig, force bool) error
  }
  // ErrCaddyUnreachable: no se pudo hablar con la admin API, o con el TLS de Caddy.
  var ErrCaddyUnreachable = errors.New("muelle no pudo conectarse con Caddy")
  // CaddyRejected: Caddy no aceptó la configuración. Message ya viene sin el
  // token (***) y cortado a 500 caracteres.
  type CaddyRejected struct{ Message string }
  // ErrPlaceholderInSettings: el email ACME o el token de Cloudflare tienen { o },
  // que Caddy expandiría como placeholders (D16). No se manda nada.
  var ErrPlaceholderInSettings = errors.New("el email ACME o el token de Cloudflare tienen { o }, que Caddy tomaría como placeholders")
  ```

  **Commit:** `feat(ingress): BuildProxyConfig, la configuración del proxy como datos`

- [x] **Step 2: El JSON de Caddy (golden).**

  **Tests primero** (`caddy/config_test.go`). Usa `testdata/<caso>.golden.json`, indentado con dos espacios (`encoding/json.Indent` sobre la salida de `Render`) y regenerable con `go test ./… -run Golden -update`. Los casos:
  - `panel_only`: sin token, sin CIDRs y con email.
    - La ruta del panel es un `subroute` con solo el `reverse_proxy`.
    - Hay una política `http`, con `email`.
  - `full`: con token y la zona del panel; CIDRs `190.5.6.0/24` y `2001:db8::/32`; tres dominios (dos `cloudflare` y uno `http`).
    - El `subroute` del panel tiene el `403` con `not client_ip` primero.
    - Hay dos políticas: `cloudflare` (`subjects` ordenados, con el panel) y `http`.
  - `no_token`: los dominios `cloudflare` van todos a la política `http`.
  - `no_email`: ningún `"email"` en el JSON.
  - `internal`: los emisores son `{"module":"internal"}` y está `apps.pki.certificate_authorities.local.install_trust: false`.

  Y además:
  - `TestRenderInvariants`, sobre todos los casos:
    - `admin.listen == naming.CaddyAdminListen`;
    - el único placeholder es `{http.vars.client_ip}`: con la regexp `\{[^}]*\}` sobre cada texto del JSON decodificado (claves y valores; sobre los bytes crudos casaría las llaves de los objetos), todo match es exactamente ese, así no queda ningún atajo del Caddyfile (`{client_ip}`, `{host}`, `{remote_host}`…);
    - el token aparece exactamente una vez por política `cloudflare`, dentro de `api_token`;
    - cada ruta de primer nivel es `terminal`;
    - `trusted_proxies.ranges == CloudflareRanges` y `trusted_proxies_strict == 1`.
  - `TestRenderRejectsPlaceholders`: el email `a{x}@b.cl`, y un token con `{file./etc/passwd}` (y otro con solo `}`), dan `errors.Is(err, service.ErrPlaceholderInSettings)`, sin bytes. El texto del error no contiene el token.
  - `TestRenderDeterministic`: 50 `Render` del caso `full` dan los mismos bytes.
  - `TestCloudflareRanges`: todos parsean con `netip.ParsePrefix` y están en forma canónica (`p.Masked().String() == s`). Son 15 IPv4 y 7 IPv6.

  **Implementar:**
  - `ranges.go`:
  ```go
  // CloudflareRanges son los rangos de Cloudflare (cloudflare.com/ips-v4 e
  // ips-v6, verificados el 2026-10-14). Se actualizan con cada versión (§8).
  var CloudflareRanges = []string{
  	"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22",
  	"141.101.64.0/18", "108.162.192.0/18", "190.93.240.0/20", "188.114.96.0/20",
  	"197.234.240.0/22", "198.41.128.0/17", "162.158.0.0/15", "104.16.0.0/13",
  	"104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
  	"2400:cb00::/32", "2606:4700::/32", "2803:f800::/32", "2405:b500::/32",
  	"2405:8100::/32", "2a06:98c0::/29", "2c0f:f248::/32",
  }
  ```
  - `config.go`: `func Render(cfg service.ProxyConfig) ([]byte, error)`, con structs propios que tienen tags JSON y `jsonv2.Marshal(v, jsonv2.Deterministic(true))` (D16).
    - Antes de armar nada: si `cfg.ACMEEmail` o `cfg.CloudflareToken` tienen `{` o `}` (`strings.ContainsAny`), devuelve `service.ErrPlaceholderInSettings`.
    - La forma (verificada con `caddy validate` en `caddy:2-alpine`; el `acme` con `dns` falla ahí solo porque esa imagen no trae el módulo):
  ```json
  {"admin": {"listen": "unix//run/caddy-admin/admin.sock"},
   "apps": {
    "http": {"servers": {"muelle": {
      "listen": [":443"],
      "trusted_proxies": {"source": "static", "ranges": ["…CloudflareRanges"]},
      "client_ip_headers": ["CF-Connecting-IP", "X-Forwarded-For"],
      "trusted_proxies_strict": 1,
      "routes": [
        {"match": [{"host": ["panel…"]}], "handle": [{"handler": "subroute", "routes": [
          {"match": [{"not": [{"client_ip": {"ranges": ["…"]}}]}], "handle": [{"handler": "static_response", "status_code": 403}]},
          {"handle": [{"handler": "reverse_proxy", "upstreams": [{"dial": "muelle_muelle:8080"}],
            "headers": {"request": {"set": {"X-Real-IP": ["{http.vars.client_ip}"]}}}}]}]}],
         "terminal": true},
        {"match": [{"host": ["api…"]}], "handle": [{"handler": "reverse_proxy", "upstreams": [{"dial": "auditfy_staging_api_api:3000"}]}], "terminal": true}
      ]}}},
    "tls": {"automation": {"policies": [
      {"subjects": ["…"], "issuers": [{"module": "acme", "email": "…",
        "challenges": {"dns": {"provider": {"name": "cloudflare", "api_token": "…"}, "resolvers": ["1.1.1.1:53", "1.0.0.1:53"]}}}]},
      {"subjects": ["…"], "issuers": [{"module": "acme", "email": "…"}]}]}}
   }}
  ```
  - Sin políticas, no va `apps.tls`.
  - Con `InternalCA`, cada emisor es `{"module": "internal"}`, y se agrega `"pki": {"certificate_authorities": {"local": {"install_trust": false}}}`.

  **Commit:** `feat(ingress): la configuración de Caddy en JSON, con golden y sin atajos del Caddyfile`

- [x] **Step 3: La admin API falsa y el cliente.**

  **`caddytest` primero** (`caddy/caddytest/admin.go`, con su test). Es un `httptest.Server` en TCP:
  ```go
  func New(t testing.TB) *Admin
  func (a *Admin) Dial(ctx context.Context) (net.Conn, error)   // para Options.CaddyAdminDial; falla si está Down
  func (a *Admin) Loads() [][]byte                                // los cuerpos recibidos, en orden
  func (a *Admin) Last(t testing.TB) map[string]any               // el último, decodificado
  func (a *Admin) Forced() []bool                                 // si cada uno traía Cache-Control: must-revalidate
  func (a *Admin) Reject(msg string)                              // los próximos POST /load responden 400 {"error": msg}, hasta Accept()
  func (a *Admin) Accept()
  func (a *Admin) SetDown(down bool)
  func (a *Admin) MaxInFlight() int                               // el máximo de /load a la vez que vio
  func (a *Admin) Delay(d time.Duration)                          // demora cada /load
  ```

  **Tests del cliente primero** (`caddy/admin_test.go`):
  - `TestLoadOverUnixSocket`:
    - un `http.Server` en un socket unix dentro de `os.MkdirTemp("", "it3")` (rutas cortas: el límite es de 108 bytes);
    - `NewAdmin` con un dial a ese socket manda `POST /load`, con `Content-Type: application/json` y el JSON de `Render`.
  - `TestLoadDefaultDialsCaddySocket`: con `dial == nil`, el error de conectar menciona `naming.CaddyAdminSocket` y es `ErrCaddyUnreachable`. En la máquina de test el socket no existe.
  - `TestLoadForce`: `force` agrega `Cache-Control: must-revalidate`; sin `force`, no va.
  - `TestLoadRejected`:
    - un `400` con `{"error":"loading config: … api_token \"tok-123\" …"}` da `*service.CaddyRejected`, con `***` en vez del token;
    - un mensaje de 2000 caracteres queda en 500 runas más "…".
  - `TestRedactEscapedToken`: admin acepta cualquier ASCII visible en el token (`[\x21-\x7e]`), así que un token puede traer `"`, `\`, `<` o `&`.
    - Con el token `cf"tok\en<1234567890&x`, un mensaje de Caddy que lo trae tal cual, citado con `%q` (`strconv.Quote`) o escapado como JSON (con y sin el escape de HTML de `encoding/json`, `\u003c`/`\u0026`) queda con `***` en cada caso.
    - Ninguna de esas formas del token queda en el resultado.
    - Con token vacío, `redact` no reemplaza nada: un `ReplaceAll` con `""` metería `***` entre cada letra.
  - `TestLoadUnreachable`: con `SetDown`, `errors.Is(err, service.ErrCaddyUnreachable)` y el token no aparece en el texto.

  **Implementar** `caddy/admin.go`:
  ```go
  type Admin struct{ http *http.Client }
  // NewAdmin: dial nil → (&net.Dialer{}).DialContext(ctx, "unix", naming.CaddyAdminSocket).
  // Transport con DialContext que ignora network/addr y usa dial,
  // DisableKeepAlives, Timeout 30 s. La URL es http://caddy/load.
  func NewAdmin(dial func(ctx context.Context) (net.Conn, error)) *Admin
  var _ service.CaddyAdmin = (*Admin)(nil)
  func (a *Admin) Load(ctx context.Context, cfg service.ProxyConfig, force bool) error
  // redact reemplaza por "***" el token y sus formas escapadas (sin las
  // comillas de afuera): strconv.Quote, y JSON con y sin el escape de HTML,
  // también una dentro de otra (dos niveles: un %q de un pedazo de JSON).
  // Las reemplaza de la más larga a la más corta, y corta a 500 runas. Con
  // token "" no reemplaza nada.
  func redact(msg, token string) string
  ```
  - Un rechazo que no trae `{"error": …}` (un 5xx con texto) usa el cuerpo, o "respondió <status>", también con `redact` (`TestLoadRejectedPlainBody`). El mensaje pasa por `strings.ToValidUTF8`: `encoding/json/v2` no serializa UTF-8 inválido, y el mensaje va a `GET /proxy`.
  - `TestLoadPlaceholder`: con `{` en el token, `Load` no manda nada.

  **Commit** (dos): `test(ingress): admin API de Caddy falsa (caddytest)` y `feat(ingress): cliente de la admin API de Caddy por el socket, con los errores sin el token`

- [x] **Step 4: `ProxyState` y `Syncer`.**

  **Tests primero** (`usecases/proxy_state_test.go`, `sync_test.go`). Para `Syncer`, con sqlite real, `ingresstest`, el repositorio, el catalog reader, `caddytest` y `caddy.NewAdmin(fake.Dial)`:
  - `TestProxyStateInitial`: `Snapshot(true)` da `out_of_sync` con `NotSyncedYet` ("muelle todavía no le mandó la configuración a Caddy"), `syncedAt nil`, y `Synced() == false`.
  - `TestProxyStatePrecedence`:
    - `Snapshot(false)` es `disabled` sin error;
    - una recreación en curso da `recreating` sin error;
    - el error del contenedor gana sobre el de `/load`;
    - `Loaded(t)` y `ContainerOK()` dan `synced` con `syncedAt = t`.
  - `TestProxyStateRecreateGenerations`: con `g1 := BeginRecreate()` y `g2 := BeginRecreate()`, `EndRecreate(g1)` sigue `recreating`, y `EndRecreate(g2)` lo termina.
  - `TestSyncLoadsRoutes`:
    - con los ajustes `Enabled` (`panel_domain` y `caddy_image`) y dos dominios, `Sync` manda una configuración con los dos hosts y el panel;
    - el estado queda `synced` con `syncedAt = clock.Now()`.
  - `TestSyncDisabled`: sin `panel_domain`, devuelve `nil`, `Loads()` queda vacío y el estado no cambia.
  - `TestSyncRejected`:
    - `Reject("…<token>…")` hace que `Sync` devuelva error;
    - el estado da `out_of_sync` con `***`;
    - `onFail` se llamó una vez;
    - el log (un `slog` sobre un buffer) no contiene el token ni la configuración (busca `"routes"`).
  - `TestSyncRecovers`: después de un rechazo, `Accept` y `Sync` dan `synced` y el error se limpia.
  - `TestSyncPlaceholder`: con `acme_email` `a{x}@b.cl`, `Sync` devuelve un error con `ErrPlaceholderInSettings`, `Loads()` queda vacío y el estado da `out_of_sync` con el mensaje de abajo.
  - `TestSyncForce`: `Sync(ctx, true)` llega con `Forced() == [true]`.
  - `TestSyncUnreachable` (`SetDown`) y `TestSyncInternalError` (un puerto ilegible en `settings`): el mensaje de cada uno, abajo, y `onFail` una vez.
  - `TestSyncSerialized`:
    - 20 goroutines `Sync` con `Delay(5 ms)` dan `MaxInFlight() == 1`;
    - el último `Load` tiene los dominios de la base en ese momento (agrega un dominio antes de la última tanda).

  **Implementar:**
  ```go
  const NotSyncedYet = "muelle todavía no le mandó la configuración a Caddy"
  type ProxyStatus string // "synced", "out_of_sync", "recreating", "disabled"
  type ProxyState struct { /* mu; containerErr, loadErr string; syncedAt *time.Time; gen, endedGen uint64 */ }
  func NewProxyState() *ProxyState
  func (s *ProxyState) Loaded(at time.Time)
  func (s *ProxyState) LoadFailed(msg string)
  func (s *ProxyState) ContainerOK()
  func (s *ProxyState) ContainerFailed(msg string)
  func (s *ProxyState) BeginRecreate() uint64
  func (s *ProxyState) EndRecreate(gen uint64)
  func (s *ProxyState) Recreating() bool
  func (s *ProxyState) Synced() bool // !recreating, sin errores y con un Load
  func (s *ProxyState) Snapshot(enabled bool) (status ProxyStatus, errMsg string, syncedAt *time.Time)

  type Syncer struct { /* mu sync.Mutex; settings; domains; catalog; admin; state; clock; defImage string; internalCA bool; onFail func(); log */ }
  func NewSyncer(settings service.Settings, domains repository.Domains, catalog service.Catalog, admin service.CaddyAdmin,
  	state *ProxyState, clk clock.Clock, defImage string, internalCA bool, onFail func(), log *slog.Logger) *Syncer
  // Sync, con s.mu tomado y un plazo de 30 s: lee los ajustes (disabled → nil),
  // PanelDNS, los dominios y las apps, arma con BuildProxyConfig y manda. Un
  // error queda en el estado (y en el log, sin la configuración) y llama a onFail.
  func (s *Syncer) Sync(ctx context.Context, force bool) error
  ```
  El mensaje del estado:
  - un `CaddyRejected` deja su `Message`;
  - un `ErrCaddyUnreachable` deja "muelle no pudo conectarse con la admin API de Caddy; se reintenta solo";
  - un `ErrPlaceholderInSettings` deja "el email ACME o el token de Cloudflare tienen { o }, que Caddy tomaría como placeholders; corrígelo en Ajustes" (sin el token ni el email);
  - otro error (de la base) deja "error interno armando la configuración" y queda en el log completo.

  **Commit:** `feat(ingress): Syncer manda la configuración de a una, con el estado del proxy`

- [x] **Step 5: `Options`, el nombre de muelle-caddy en las redes de stage y `Module.SyncRoutes`.**

  Va antes del test de integración del Step 6: hasta acá, `EnsureStageNetwork` conecta el `muelle-caddy` fijo (`naming.CaddyContainer`), y el test conectaría un `muelle-caddy` real si existiera.

  **Tests primero:**
  - `options_test.go`, `TestOptionsDefaults`:
    - `Options{}.withDefaults()` da los nombres de `naming`, `CaddyTLSAddr == "muelle-caddy:443"`, `CloudflareURL == cloudflare.DefaultBaseURL` y los plazos de `Timing` de abajo;
    - un `CaddyContainer` propio cambia también el `CaddyTLSAddr` por defecto (`<nombre>:443`).
  - `stage_network_test.go`:
    - el fixture recibe el nombre del contenedor. Los casos que ya están pasan `naming.CaddyContainer`;
    - `TestEnsureStageNetworkUsesContainerName` (con `"it3-otro"`): conecta ese contenedor, no `naming.CaddyContainer`;
    - `TestRemoveStageNetworkUsesContainerName`: desconecta ese.
  - `module_test.go`:
    - `newTestModule` recibe `Options` (o un `func(*Options)`) y **siempre** pone `CloudflareURL` en un `cftest.New(t)` nuevo, que devuelve para que el test lo siembre. Ningún test del módulo puede llegar a la API real cuando T5 cablee los hooks.
      - Hecho así: `newTestModule(t, opts ...func(*Options)) *testModule`. `testModule` embebe el `*Module` y trae `docker` (`dockertest.Fake`), `cf` (`cftest`), `caddy` (`caddytest`), `db` y `cipher`. Pone `CloudflareURL` y también `CaddyAdminDial` (el `caddytest`) **después** de `opts`, así ningún test del módulo llega a la API real de Cloudflare ni a un socket de Caddy. `Version` es `v0.1.0`: con `panel_domain`, el proxy está habilitado.
    - `TestModuleStageNetworkCustomName`: con `Options.CaddyContainer "it3-x"`, `EnsureStageNetwork` y `RemoveStageNetwork` conectan y desconectan `it3-x`, y **ninguna** entrada de `Calls()` del fake nombra `naming.CaddyContainer` (`muelle-caddy`).
    - Cambiar `TestModuleStubs` → `TestModuleSyncRoutes`:
      - con los ajustes vacíos (desarrollo), `SyncRoutes` devuelve `nil` y no llama a la admin API;
      - con `Enabled` y `caddytest` en `Options.CaddyAdminDial`, devuelve `nil` y llegó un `Load`;
      - con `Reject`, devuelve error.
    - El resto de `TestModuleStubs` (`CheckCloudflareToken`, `ProxySettingsChanged`, `Jobs`, `Run`) queda como está.
  - `src/shared/infra/container/ports_test.go`, solo `TestContractProxySettings`:
    - arma un `cf := cftest.New(t)` con `cf.AddToken("cf-token-1234567890abcdef")` y **una zona** (`cf.AddZone("z1", "otra.cl")`, fuera de `panel.test`). Con cero zonas, `CheckCloudflareToken` responde `422` desde T5;
    - pasa `WithIngress(ingress.Options{CloudflareURL: cf.URL()})` a `New`.
    - Ahora el hook todavía es esqueleto y el test pasa igual. Desde T5, sin esto, el `PATCH` del token iría a `https://api.cloudflare.com` y fallaría (`422` o `502`).

  **Implementar** `options.go`:
  ```go
  // Options cambian cómo ingress llega a Docker, Caddy y Cloudflare. El cero es
  // producción (D3); solo los tests las cambian.
  type Options struct {
  	CaddyContainer, PanelNetwork                         string // naming.CaddyContainer, naming.PanelNetwork
  	CaddyDataVolume, CaddyConfigVolume, CaddyAdminVolume string // naming.Caddy*Volume
  	CaddyAdminDial func(ctx context.Context) (net.Conn, error) // nil: el socket unix
  	CaddyTLSAddr   string         // "": <CaddyContainer>:443, por muelle_panel
  	TLSRoots       *x509.CertPool // nil: las del sistema
  	InternalCA     bool           // la CA interna de Caddy (tls internal), para los tests de integración
  	NetworkPrefix  string         // el job "proxy" solo borra redes huérfanas con este prefijo ("": todas)
  	CloudflareURL  string         // "": cloudflare.DefaultBaseURL
  	Timing         Timing
  }
  type Timing struct {
  	RetryMin, RetryMax time.Duration // 1 s y 60 s: reintentos de /load
  	RecreateDelay      time.Duration // 2 s: la recreación después del PATCH
  	FirstCheck         time.Duration // 30 s: la verificación después de crear, adoptar o reintentar
  	PendingEvery       time.Duration // 1 min: el barrido de los pending
  	DomainOp           time.Duration // 30 s: crear, editar, borrar, adoptar, EnsurePanelDNS y cada registro del hook (D6)
  	DomainRetry        time.Duration // 20 s: retry
  	Hook               time.Duration // 2 min: ProxySettingsChanged
  }
  func (o Options) withDefaults() Options
  ```
  `stage_network.go`: `NewEnsureStageNetwork(n Networks, caddyContainer string, log)` y `NewRemoveStageNetwork(n, caddyContainer, log)`. Usan ese nombre en `Connect`/`Disconnect`, y sus comentarios dicen "muelle-caddy (el nombre de `Options`)".

  `module.go`:
  - `Deps.Options Options`, y `New` exige `Cipher` (panic como con `DB`).
  - `o := d.Options.withDefaults()`, y las redes de stage reciben `o.CaddyContainer`.
  - Arma el `ProxySettingsReader`, el `CatalogReader`, el repositorio de dominios, `caddy.NewAdmin(o.CaddyAdminDial)`, `NewProxyState()` y `NewSyncer(…, naming.CaddyImageFor(d.Version), o.InternalCA, func() {}, …)`. El `onFail` real llega en T3.
  - `SyncRoutes(ctx)` pasa a ser `m.syncer.Sync(ctx, false)`, con el doc actualizado: disabled → nil; si Caddy no acepta o no responde, un error que deploy registra como advertencia.

  `src/shared/infra/container/container.go`, en las líneas de ingress:
  ```go
  type options struct {
  	docker  docker.Engine
  	ingress ingress.Options
  }
  // WithIngress cambia cómo ingress llega a Docker, Caddy y Cloudflare (los
  // nombres, la admin API, la URL de Cloudflare). Solo para tests: el cero es producción.
  func WithIngress(o ingress.Options) Option { return func(op *options) { op.ingress = o } }
  ```
  - `build` recibe el `options` entero, `o options`, como último parámetro (en la firma y en la llamada de `New`), y pasa `ingress.Deps{…, Options: o.ingress}`. Con el valor entero, la próxima opción de otra fase no vuelve a cambiar la firma. El choque con la `panelHandler` de la Fase 5 está en "Al integrar".
  - Revisa con `git grep -n 'cloudflare_token\|CloudflareToken' -- 'src/shared/infra/container/*_test.go'` que ningún otro test del contenedor mande un token sin `WithIngress`.

  **Commit** (dos):
  - `feat(ingress): Options, y las redes de stage usan su nombre de muelle-caddy` (`options.go`, `stage_network.go`, `Deps.Options`, `newTestModule` y `TestModuleStageNetworkCustomName`);
  - `feat(ingress): SyncRoutes manda la configuración a Caddy, y WithIngress en el contenedor para que los tests usen fakes` (lo demás).

- [x] **Step 6: Integración — Caddy real acepta la configuración y enruta.**

  **`integration_env_test.go`** (`//go:build integration`, `package ingress`), con los helpers que reusan T3 y T4:
  ```go
  // ranura n: puertos 21000+100n+1 (http), +2 (https), +3 (puente admin), +4/+5 (http/https de repuesto), +6 (ocupado a propósito).
  type slot struct{ HTTP, HTTPS, Admin, HTTP2, HTTPS2, Busy int }
  func slotPorts(n int) slot
  type itEnv struct {
  	t       *testing.T
  	docker  *docker.Client
  	id      string // 4 hex
  	project string // "it3"+id
  	ports   slot
  	opts    Options // los nombres de abajo, dial al puente, TLSAddr 127.0.0.1:<https>, InternalCA, NetworkPrefix "it3"+id, CloudflareURL de un cftest
  	cf      *cftest.Server // el de opts.CloudflareURL; ningún test llega a la API real
  	db      *sqlite.DB
  	cipher  *crypto.Cipher
  }
  func newITEnv(t *testing.T, slotN int) *itEnv // salta sin Swarm manager o sin caddy:2-alpine / alpine/socat:latest; registra la limpieza
  func (e *itEnv) startAdminBridge()           // alpine/socat "it3-<id>-admin": monta el volumen admin en /run/caddy-admin, Cmd ["TCP-LISTEN:<admin>,fork,reuseaddr", "UNIX-CONNECT:/run/caddy-admin/admin.sock"], publica 127.0.0.1:<admin>
  func (e *itEnv) startBackend(stage, app string) // caddy:2-alpine "it3-<id>-<app>" en la red del stage, alias naming.Service(project, stage, app, app), Cmd ["caddy","respond","--listen",":8080","--body","hola "+app]
  func (e *itEnv) startBackendCmd(stage, app string, cmd []string) // igual, con otro Cmd (la sonda de aislamiento de T3)
  func (e *itEnv) startCaddyManually(stages ...string) // solo T2: crea la red panel (opts.PanelNetwork, overlay attachable) si falta, y un muelle-caddy de prueba con naming.CaddyCommand()/CaddyEnv(), los volúmenes y puertos de prueba, en esa red y en las de los stages
  // run corre m.Run en segundo plano. Registra su t.Cleanup (cancelar y esperar
  // a que Run vuelva, hasta 30 s) DESPUÉS de la limpieza de newITEnv: t.Cleanup
  // es LIFO, así Run ya terminó cuando se borran contenedores y redes.
  func (e *itEnv) run(m *Module)
  func (e *itEnv) settings(p shared.ProxySettings) // panel.it3<id>.test, caddy_image "caddy:2-alpine" y los puertos de la ranura
  func (e *itEnv) get(host, path string) (status int, body string, err error) // HTTPS a 127.0.0.1:<https> con ServerName y Host = host (InsecureSkipVerify: el TLS se prueba en T4)
  func (e *itEnv) eventually(timeout time.Duration, what string, check func() (bool, error))
  ```
  Hecho así, con algunos helpers más (`integration_env_test.go`):
  - `module()` arma ingress con `e.db`, `e.cipher`, el Docker real y `e.opts`; `host(sub)`, `panelHost()`, `addDomain(appID, host, svc, port)` y `expectGet(host, path, status, body)` (un check para `eventually`).
  - Todo contenedor pasa por `createAndStart(spec, redesExtra...)`, que lo anota para la limpieza **antes** de crearlo, se niega a crear uno sin el prefijo del test y le pone la etiqueta `muelle.test=it3`.
  - **`waitNetwork`:** antes de conectar un contenedor a una overlay, `createAndStart` espera a que Swarm le dé subred (hasta 30 s). Sin eso, arrancar un contenedor en una overlay recién creada falló con "attaching to network failed … context deadline exceeded" (lo mismo que ya espera el test de redes del paquete docker).
  - La limpieza toma como del test los nombres que empiezan con `it3<id>_` o `it3-<id>-` (con el separador: el id tiene largo fijo, pero así ni se acerca a otro).
  - `settings(p)` cambia 80 y 443 (o 0) por los puertos de la ranura: un test nunca los publica.
  - `startCaddyManually` publica en `127.0.0.1`, como el puente.

  **Los nombres exactos** (los fija `newITEnv`; ninguno es un nombre fijo de producción):

  | Qué | Nombre |
  |---|---|
  | El proyecto, y `opts.NetworkPrefix` | `it3<id>` |
  | Las redes de stage (`naming.Network`) | `it3<id>_<stage>` |
  | La red panel (`opts.PanelNetwork`) | `it3<id>_panel`. Hasta T3 la crea solo `startCaddyManually`; desde T3, `caddy.Runtime` |
  | muelle-caddy (`opts.CaddyContainer`) | `it3-<id>-caddy` |
  | Los volúmenes (`opts.CaddyDataVolume`, `…ConfigVolume`, `…AdminVolume`) | `it3-<id>-caddy-data`, `it3-<id>-caddy-config`, `it3-<id>-caddy-admin` |
  | El puente | `it3-<id>-admin` |
  | Los backends y la sonda | `it3-<id>-<app>` |

  **Limpieza** (`t.Cleanup`, siempre; la registra `newITEnv`, antes que cualquier `run`). Un nombre es del test si empieza con `it3<id>` **o** con `it3-<id>-`:
  1. `RemoveContainer(force)` de los contenedores que creó el test (los anota al crearlos) y de `opts.CaddyContainer`. El cliente no lista contenedores, y no hace falta: todos los nombres son de la tabla;
  2. las redes con uno de los dos prefijos (`ListNetworks(nil)`), reintentando durante 30 s mientras estén en uso;
  3. los volúmenes con uno de los dos prefijos (`ListVolumes(nil)` + `RemoveVolume(force)`).

  **Test** `TestIntegrationCaddyAcceptsConfig` (ranura 1):
  1. Arranca el puente; siembra el stage `s1` y crea su red con `m.EnsureStageNetwork` (el módulo con `e.opts`: si conecta algo, es `it3-<id>-caddy`, nunca `muelle-caddy`; Step 5); arranca el backend `web`; arranca Caddy a mano (con la red panel).
  2. Siembra el dominio `web.it3<id>.test` → `web:8080`.
  3. `eventually(30 s)`: `m.SyncRoutes(ctx) == nil` (Caddy está arrancando).
  4. `eventually(30 s)`: `get("web.it3<id>.test")` da `200 "hola web"`.
  5. Con `panel_allowed_cidrs: ["203.0.113.0/24"]`, otra vez `SyncRoutes`: el panel da `403`. La IP del cliente es la del bridge de Docker, que no está en la lista. Prueba el matcher `client_ip` en Caddy real.
  6. Sin la lista, el panel da `502`: no hay un `muelle_muelle`, y la ruta existe.

  **Correr:** `go test -tags integration -count=1 -run Integration -v ./src/internal/ingress/...`. Expected: `PASS`, o `SKIP` sin Swarm o sin las imágenes.

  Después, `docker ps -a --filter name=it3-`, `docker network ls --filter name=it3` y `docker volume ls --filter name=it3` no muestran nada del test (también la red `it3<id>_panel` y los tres volúmenes `it3-<id>-caddy-*`).

  **Commit:** `test(ingress): integración, Caddy real acepta la configuración y enruta por hostname y por client_ip`

- [x] **Step 7: Cierre de T2.** Las verificaciones de "Convenciones" pasan, y el test de integración del Step 6 también.
  - Hecho: gofmt, `go vet ./...` (y con `-tags integration` en ingress), `go test -race ./...` (51 paquetes), `go mod tidy -diff` y `go test -tags integration -race ./src/internal/ingress/...` (`TestIntegrationCaddyAcceptsConfig` en ~10 s, tres corridas seguidas sin fallar). Después, `docker ps -a`, `docker network ls` y `docker volume ls` con `it3` no muestran nada.
  - `caddy validate` en `caddy:2-alpine` acepta los golden `panel_only`, `no_token` e `internal`; `full` y `no_email` fallan ahí solo por `dns.providers.cloudflare`, que esa imagen no trae (D4).

---

### Task T3: muelle-caddy, el job `"proxy"` y `GET /proxy`

**Objetivo:** Este es el ciclo de vida del contrato §3.4. ingress:
- crea y recrea muelle-caddy (nunca lo rearranca), con los puertos de los ajustes y `ErrPortInUse`;
- le conecta la red `muelle_panel` y las de stage;
- corre el job `"proxy"` cada 5 min, que también borra las redes huérfanas;
- reintenta `/load` con backoff;
- recrea el contenedor 2 s después del último `Schedule`, y nunca mientras un hook lo frena (`Hold`, D15);
- responde `GET /api/v1/proxy`.

**Files:**
- Modify: `src/internal/ingress/domain/service/ports.go` (`Network.Labels`, `Networks.List`, `CaddyRuntime`, `CaddyWant`), `errors.go` (`ErrPortInUse`, `ErrCaddyImagePull`)
- Create: `src/internal/ingress/domain/service/credential.go` + `credential_test.go` (los métodos que ocultan la contraseña de `Credential`, que es de T1)
- Modify: `src/internal/ingress/infra/out/service/networks.go` + `networks_test.go` (nuevo)
- Create: `src/internal/ingress/infra/out/service/caddy/container.go` + `container_test.go`
- Create: `src/internal/ingress/app/usecases/{caddy,orphans,retrier,recreator,proxy_job,proxy_status,sleep}.go` + tests
- Create: `src/internal/ingress/app/dto/proxy.go`
- Create: `src/internal/ingress/infra/in/http/{handler,proxy}.go` + `proxy_test.go`; Modify: `errors.go`
- Create: `src/internal/ingress/infra/in/cron/jobs.go` + `jobs_test.go`
- Modify: `src/internal/ingress/module.go`, `module_test.go`
- Modify: `src/shared/infra/container/container_test.go` (solo la línea de jobs de `TestNew`)
- Modify: `src/internal/ingress/integration_test.go`

- [x] **Step 1: Las redes se listan con sus etiquetas.** (El nombre de muelle-caddy en las redes de stage ya lo hizo T2 Step 5.)

  **Tests primero:**
  - `networks_test.go`, `TestDockerNetworksList`: con `dockertest`, devuelve las redes con sus `Labels` y `Overlay`; `Inspect` también trae `Labels`.

  **Implementar:**
  - `service.Network` suma `Labels map[string]string`, y `Networks` suma `List(ctx) ([]Network, error)`.
  - `DockerNetworks.List` es `ListNetworks(ctx, nil)` mapeado como `Inspect`.

  **Commit:** `feat(ingress): las redes se listan con sus etiquetas`

- [x] **Step 2: `caddy.Runtime`, el contenedor.**

  **Tests primero, la credencial** (`domain/service/credential_test.go`), `TestCredentialHidesPassword`, con `Credential{Host: "ghcr.io", Username: "bot", Password: "reg-secret-1234567890"}`:
  - `fmt.Sprint(c)`, `fmt.Sprintf("%+v", c)`, `fmt.Sprintf("%#v", c)` y `fmt.Sprint(&c)` no contienen la contraseña, y sí el host y el usuario;
  - un `slog.JSONHandler` y un `slog.TextHandler` con `c` como atributo, y también con un `CaddyWant{Auth: &c}`, tampoco;
  - `json.Marshal(c)` y `json.Marshal(CaddyWant{Auth: &c})` tampoco.
  - Solo hace falta en `Credential`: es el único secreto de `CaddyWant`. slog serializa un `CaddyWant` con `encoding/json`, que llama a `Credential.MarshalJSON`, y `fmt` imprime un puntero anidado como dirección.

  **Tests primero, el contenedor** (`caddy/container_test.go`, con `dockertest.Fake`, `SetImagePresent` y nombres de `Names{…}` de prueba):
  - `TestEnsureCreates`, desde vacío:
    - crea la red panel (overlay attachable) si falta;
    - crea el contenedor con `Name`, `Image`, `Cmd == naming.CaddyCommand()`, `Env == naming.CaddyEnv()`, `RestartPolicy "unless-stopped"`, `Network` panel y los tres `Mounts` (data → `/data`, config → `/config`, admin → `/run/caddy-admin`);
    - `Ports` son `{http,80,tcp}`, `{https,443,tcp}` y `{https,443,udp}`;
    - lo conecta a las redes de stage de `want.Networks` **antes** de `StartContainer` (el orden de `Calls()`);
    - una red de `want.Networks` que no existe se salta: el contenedor se crea, arranca y queda en las demás (D11);
    - devuelve `recreated == true`.
  - `TestEnsureIdempotent`: la segunda llamada no tiene `CreateContainer`, `RemoveContainer` ni `StopContainer` en `Calls()`, y devuelve `recreated == false`.
  - `TestEnsureConnectsNewNetwork`:
    - con el contenedor ya bien y una red de stage nueva en `want.Networks`, solo agrega `ConnectNetwork`;
    - el id y `StartedAt` no cambian.
  - `TestEnsureReconnectsPanel`: con el contenedor ya bien pero desconectado de la red panel (`DisconnectNetwork` en el fake), `Ensure` lo vuelve a conectar (`ConnectNetwork <panel> <contenedor>`), con el mismo id y `recreated == false`. Sin eso, el panel quedaría sin llegar a muelle hasta otra recreación.
  - `TestEnsureRecreates`, una tabla con un cambio por caso:
    - otra imagen;
    - otros puertos;
    - otro `Cmd` (un contenedor creado a mano con `caddy run --config …`);
    - otros volúmenes;
    - detenido (`StopContainer`);
    - corriendo sin `PublishedPorts` (el truco del fake: `BusyHostPort`, el `Start` falla, `FreeHostPort` y `StartContainer` otra vez).
    - En cada caso: aparece `StopContainer` antes de `RemoveContainer` cuando corría, el id es nuevo y está corriendo con `PublishedPorts`.
    - En el caso de otra imagen, sin la imagen nueva en el nodo: `PullImage` aparece en `Calls()` **antes** de `StopContainer`.
    - **La imagen nueva no baja** (`FailNext("PullImage", docker.ErrDenied)`, sin la imagen en el nodo): `Ensure` devuelve un error con `service.ErrCaddyImagePull` y `docker.ErrDenied`, y el contenedor anterior sigue corriendo con el mismo id y la imagen vieja. `Calls()` no tiene `StopContainer`, `RemoveContainer` ni `CreateContainer` (D11).
  - `TestEnsurePullsImage`:
    - sin la imagen en el nodo, `PullImage` una vez;
    - con `want.Auth`, la credencial llega al pull (un tipo del test envuelve el fake y anota el `auth` de `PullImage`, D20);
    - con la imagen ya en el nodo, no hay pull.
  - `TestEnsurePortInUse`:
    - con `BusyHostPort(http)`, el error cumple `errors.Is(err, service.ErrPortInUse)` y el contenedor ya no existe (D11);
    - después de `FreeHostPort`, `Ensure` lo crea bien.
  - `TestEnsureDockerDown`: `SetUnavailable` da `entity.ErrDockerUnavailable`.
  - `TestEnsureHidesRegistryPassword`: con `want.Auth` (contraseña `reg-secret-1234567890`), un `slog` sobre un buffer, y `FailNext` en `PullImage` (sin la imagen en el nodo), en `CreateContainer` y en `StartContainer`, uno por caso: ni el texto del error ni el log contienen la contraseña.
  - `TestStatus`: sin contenedor, `found == false`; con uno, su `Image` y `Running`.

  **Implementar:**
  ```go
  // credential.go: Credential (ports.go, de T1) suma, como ProxySettings, la
  // contraseña como "***" (si hay) en las cuatro vistas.
  func (c Credential) String() string
  func (c Credential) GoString() string
  func (c Credential) LogValue() slog.Value
  func (c Credential) MarshalJSON() ([]byte, error)

  // ports.go
  type CaddyWant struct {
  	Image               string
  	HTTPPort, HTTPSPort int
  	Networks            []string // las redes de stage a las que tiene que estar conectado
  	Auth                *Credential // la contraseña nunca sale: ver credential.go
  }
  // CaddyRuntime es el contenedor muelle-caddy (adaptador: caddy.Runtime, D2).
  type CaddyRuntime interface {
  	// Ensure lo deja como pide want (contrato §3.4) y devuelve si lo creó de
  	// nuevo. Errores: la imagen que no baja (el contenedor anterior sigue
  	// intacto), ErrPortInUse (el contenedor queda borrado) o Docker caído.
  	Ensure(ctx context.Context, want CaddyWant) (recreated bool, err error)
  	Status(ctx context.Context) (image string, running, found bool, err error)
  }
  // errors.go
  var ErrPortInUse = errors.New("un puerto del host que publica muelle-caddy ya está en uso")
  // ErrCaddyImagePull: la imagen de Caddy no bajó; el contenedor anterior sigue (D11).
  var ErrCaddyImagePull = errors.New("no se pudo bajar la imagen de Caddy")

  // caddy/container.go
  type Names struct{ Container, PanelNetwork, DataVolume, ConfigVolume, AdminVolume string }
  type Engine interface {
  	docker.Networks
  	docker.Containers
  	docker.Images
  }
  type Runtime struct { /* docker Engine; names Names; log */ }
  func NewRuntime(d Engine, n Names, log *slog.Logger) *Runtime
  ```
  `Ensure`, en orden:
  1. Asegura la red panel.
  2. Arma el `docker.ContainerSpec` deseado e inspecciona el contenedor.
  3. Si existe y `matches(c, spec)` (misma imagen, `Cmd`, puertos y volúmenes como conjunto, `Running`, y si tiene `Ports` también `PublishedPorts`), conecta las redes que no están en `c.Networks`: **la red panel** y las de `want.Networks`. `ErrConflict` (ya conectado) no es error, y una red de stage que no existe se registra y se salta. Después vuelve.
  4. Si no (D11):
     1. `InspectImage`, y `PullImage(image, auth)` si da `ErrNotFound`. Si el pull falla, vuelve con `fmt.Errorf("%w %s: %w", service.ErrCaddyImagePull, image, err)` **sin tocar el contenedor que existe**. Si el error es de Docker caído, se devuelve tal cual (`ErrDockerUnavailable`, sin `ErrCaddyImagePull`), así `EnsureCaddy` muestra "Docker no responde…";
     2. `StopContainer(10 s)`, ignorando su error con un log;
     3. `RemoveContainer(force)`;
     4. `CreateContainer`;
     5. `ConnectNetwork` a cada red. Igual que en el paso 3: `ErrConflict` no es error, y una red que no existe se registra y se salta;
     6. `StartContainer`. Con `docker.ErrPortInUse`: `RemoveContainer(force)` y `fmt.Errorf("%w: %w", service.ErrPortInUse, err)`.
  - `Runtime` pasa `want.Auth` a `*docker.RegistryAuth` solo para `PullImage`. Nunca registra `want`, la credencial ni el `RegistryAuth`, y sus errores nombran la imagen, no la credencial.

  **Commit** (dos):
  - `feat(ingress): la credencial de un registry nunca muestra su contraseña` (`credential.go` y su test, con `CaddyWant`, `CaddyRuntime` y los errores de `ports.go`/`errors.go`, que el test ya usa);
  - `feat(ingress): el ciclo de vida de muelle-caddy, que se recrea y nunca se rearranca`.

- [x] **Step 3: `EnsureCaddy`, las redes huérfanas, el `Retrier` y el `Recreator`.**

  **Tests primero** (usecases, con `dockertest`, sqlite, `caddytest` y `clock.Manual`):
  - `TestEnsureCaddyDisabled`: sin `panel_domain`, `Calls()` del fake queda vacío.
  - `TestEnsureCaddyImage`:
    - con `caddy_image` vacío y `version v0.1.0`, la imagen es `ghcr.io/cpu-commits/muelle-caddy:v0.1.0`;
    - con `caddy_image` fijo, esa;
    - con un registry `ghcr.io` sembrado, la credencial llega a `want.Auth`;
    - sin registry, va nil.
  - `TestEnsureCaddyNetworks`: `want.Networks` son las de todos los stages (`naming.Network`).
  - `TestEnsureCaddyPortInUse`: el estado da `out_of_sync` con un mensaje que nombra los dos puertos ("los puertos 8080 o 8443 del host ya están en uso…"). Después de liberarlo y otro `Execute`, el error del contenedor se limpia.
  - `TestEnsureCaddyPullFails`: con muelle-caddy corriendo y `caddy_image` cambiada a `ghcr.io/cpu-commits/muelle-caddy:v0.2.0`, que `DenyImage` no deja bajar:
    - el estado da `out_of_sync` con "muelle no pudo bajar la imagen <imagen> de Caddy (…); muelle-caddy sigue con la anterior";
    - el contenedor sigue corriendo con el mismo id;
    - **qué hacer** (las imágenes de muelle son privadas, ver el encabezado). Con `ErrDenied`, o con `ErrNotFound` (`FailNext("PullImage", docker.ErrNotFound)`: hay registries que dan "no existe" a una imagen privada), el mensaje termina con la acción para el host de la imagen:
      - sin un registry `ghcr.io` sembrado: "… Si la imagen es privada, agrega el registry ghcr.io en Ajustes → Registries, con un usuario y un token que puedan leerla";
      - con uno: "… El registry ghcr.io no dio acceso con la credencial guardada: actualízala en Ajustes → Registries (el token puede haber vencido)".
    - Cualquier otro error de pull deja solo el mensaje base.
  - `TestEnsureCaddyHidesRegistryPassword`: con el registry `ghcr.io` sembrado (contraseña `reg-secret-1234567890`) y la imagen denegada, ni el mensaje del estado, ni el error de `Execute`, ni el log contienen la contraseña.
  - `TestEnsureCaddySerialized`: dos `Execute` a la vez crean un solo contenedor (`CreateContainer` una vez).
  - `TestOrphanNetworks`:
    - una red `auditfy_viejo` con las etiquetas de un stage que no existe queda en la primera pasada y se borra en la segunda (antes, se desconecta muelle-caddy);
    - una red con etiquetas de stage pero otro nombre, una sin etiquetas y la de un stage que existe no se tocan nunca;
    - una huérfana en uso (con un contenedor conectado) no falla: queda para la próxima;
    - con `NetworkPrefix "it3x"`, una huérfana `auditfy_viejo` no se toca.
  - `TestNextBackoff`: 0 → 1 s, 1 → 2 s… 32 s → 60 s, y 60 s → 60 s (con `RetryMin`/`RetryMax`).
  - `TestRetrierRecovers`:
    - con `Timing{RetryMin: 1ms, RetryMax: 4ms}`, `caddytest.Reject`, un `Sync` que falla y `Run` corriendo;
    - después de `Accept`, el estado llega a `synced` (`eventually` de 2 s);
    - `Run` vuelve al cancelar el ctx.
  - `TestRecreatorDebounce`:
    - con `RecreateDelay: 20ms`, dos `Schedule()` seguidos dan una sola recreación: `CreateContainer` una vez después de cambiar los puertos en los ajustes;
    - mientras tanto, `Snapshot` dice `recreating`;
    - al final queda `synced`, con un `Load` después de la recreación.
  - `TestRecreatorKeepsLaterSchedule`: un `Schedule()` durante la recreación (un `caddytest.Delay`) deja `recreating` hasta la segunda.
  - `TestRecreatorWaitsForHold` (D15), con `RecreateDelay: 20ms` y `Run` corriendo:
    - `release := Hold()`: `Snapshot` dice `recreating` enseguida;
    - 100 ms después, `Calls()` no tiene `CreateContainer`;
    - `release()`: aparece un `CreateContainer`, no antes de 20 ms después del `release`;
    - un segundo `release()` no hace nada (una sola recreación).
  - `TestRecreatorHoldStopsPendingSchedule`: un `Schedule()` y, antes de que venza su plazo, un `Hold()`. No hay recreación mientras dure el `Hold`; después del `release`, una sola (`CreateContainer` una vez).

  **Implementar:**
  - `sleep.go`: `func sleep(ctx context.Context, d time.Duration) bool`, con un timer y `select` sobre `ctx.Done()`.
  - `caddy.go`:
  ```go
  type EnsureCaddy struct { /* mu; runtime; settings; catalog; registries; state; defImage; httpsMsg; log */ }
  // Execute: disabled → (false, nil) sin tocar Docker. Si no: la imagen
  // (CaddyImageOr), la credencial del host (reference.Domain de
  // reference.ParseNormalizedNamed; si no parsea, anónima), las redes de todos
  // los stages y runtime.Ensure. El resultado va al estado (ContainerOK o
  // ContainerFailed con un mensaje en español: el puerto ocupado, la imagen que
  // no baja y Docker caído tienen su texto propio; la imagen que no baja por
  // ErrDenied o ErrNotFound dice qué hacer en Ajustes → Registries para su
  // host, distinto si había credencial). Nunca registra want ni la credencial.
  func (u *EnsureCaddy) Execute(ctx context.Context) (recreated bool, err error)
  ```
  - `orphans.go`: `RemoveOrphanNetworks{networks, catalog, caddyContainer, prefix, mu, seen map[string]bool}.Execute(ctx) error`, según D13.
  - `retrier.go`:
  ```go
  type Retrier struct { /* kick chan struct{} (buffer 1); sync func(ctx) error; min, max time.Duration */ }
  func (r *Retrier) Kick() // no bloquea
  // Run: al recibir un Kick espera el backoff y llama a sync; si falla, duplica
  // hasta max; si funciona, vacía el Kick pendiente y vuelve a esperar.
  func (r *Retrier) Run(ctx context.Context)
  func nextBackoff(d, min, max time.Duration) time.Duration
  ```
  - `recreator.go`:
  ```go
  type Recreator struct { /* ch chan struct{} (buffer 1); mu; holds int; state; caddy *EnsureCaddy; syncer *Syncer; delay; log */ }
  // Schedule agenda la recreación (state.BeginRecreate) y no espera.
  func (r *Recreator) Schedule()
  // Hold suma uno a holds y marca la recreación como pendiente
  // (state.BeginRecreate: GET /proxy dice recreating). release resta uno y hace
  // Schedule: así los 2 s cuentan desde que vuelve el hook (D15). release es
  // idempotente (sync.Once).
  func (r *Recreator) Hold() (release func())
  // Run espera un Schedule; después espera delay, y vuelve a esperar mientras
  // lleguen más (un loop con etiqueta). Si al vencer el plazo hay un Hold sin
  // soltar, vuelve a esperar un Schedule (el release lo manda). Si no, toma la
  // generación actual, llama a caddy.Execute y a syncer.Sync, y termina con
  // EndRecreate(gen).
  func (r *Recreator) Run(ctx context.Context)
  ```
  El `onFail` del `Syncer` pasa a ser `func() { m.retrier.Kick() }` (closure en `module.New`).

  Hecho así, con lo que sumó la implementación:
  - `ProxyState.Generation()` es la generación que toma el `Recreator` antes de recrear (sin agendar otra, como haría `BeginRecreate`).
  - **El `Recreator` espera al Caddy nuevo:** si `EnsureCaddy` lo creó de nuevo, el `Sync` que sigue reintenta con el backoff de `Timing` (`NewRecreator(…, delay, retryMin, retryMax, log)`) mientras dé `ErrCaddyUnreachable`, hasta 30 s (`syncStarting`, en `sync.go`; el job `"proxy"` lo usa igual, ver el Step 4). Un Caddy recién creado tarda un momento en abrir la admin API: sin eso, `GET /proxy` pasaba de `recreating` a `out_of_sync` ("se reintenta solo") y recién después a `synced`. Lo prueba `TestRecreatorWaitsForCaddy`.
  - **El mensaje de la imagen que no baja** termina en "muelle-caddy sigue con la anterior" solo si el contenedor anterior corre (`runtime.Status`); si no (el primer arranque), "muelle-caddy no está corriendo" (`TestEnsureCaddyPullFailsFirstTime`). La causa es "el registry no dio acceso" (`ErrDenied`), "el registry no la encontró" (`ErrNotFound`) o el error de Docker, cortado a 300 caracteres.
  - El error de `Execute` y el log pasan por `hideSecret` con la contraseña del registry (Docker no la repite, pero por las dudas); un error leyendo la base no cambia el estado.
  - `RemoveOrphanNetworks` borra la huérfana la segunda pasada **seguida**: una que dejó de estar huérfana vuelve a contar desde cero (`TestOrphanNetworksConsecutive`), y una en uso queda anotada para la pasada siguiente (`TestOrphanNetworksInUse`).

  **Commit** (dos):
  - `feat(ingress): EnsureCaddy y el borrado de las redes de stage huérfanas a la segunda pasada`
  - `feat(ingress): reintentos de /load con backoff y la recreación agendada de muelle-caddy`

- [x] **Step 4: El job `"proxy"`.**

  **Tests primero** (`proxy_job_test.go` y `infra/in/cron/jobs_test.go`):
  - `TestProxyJobDisabled`: no llama a Docker ni a la admin API.
  - `TestProxyJobFirstRun`, desde Docker vacío con dos stages en la base:
    - crea las dos redes, la red panel y muelle-caddy conectado a las dos;
    - manda un `Load`;
    - el estado da `synced`.
  - `TestProxyJobSkipsCaddyWhileRecreating`: con un `BeginRecreate` pendiente, o con un `Hold` sin soltar, no hay `CreateContainer`/`RemoveContainer`, pero sí `Sync`.
  - `TestProxyJobDockerDown`: devuelve un error con `ErrDockerUnavailable`, y el estado da `out_of_sync` con "Docker no responde…".
  - `TestJobs`: `Jobs(job)` da `[{Name:"proxy", Every: 5*time.Minute}]`, y su `Run` llama al job.

  **Implementar:**
  - `proxy_job.go`: `ProxyJob.Run(ctx) error`, en el orden de D12 (sin la parte de Cloudflare, que llega en T5). Junta los errores con `errors.Join`; cada uno ya quedó en el log.
  - `infra/in/cron/jobs.go`: `func Jobs(proxy func(ctx) error) []cron.Job`. T4 le suma `"certificados"`.

  Hecho así: si la red de un stage falla con Docker caído, no prueba las demás (fallarían todas). **Si el job creó muelle-caddy** (el primer arranque, o uno detenido), el `Sync` espera a que la admin API responda (`syncStarting`, con `NewProxyJob(…, defImage, retryMin, retryMax, log)`): si no, el primer `job()` de la integración (Step 7) fallaba con `ErrCaddyUnreachable` en vez de dar `nil`, y el primer arranque pasaba por `out_of_sync` hasta que el `Retrier` reintentaba (`TestProxyJobWaitsForNewCaddy`). `TestProxyJobRemovesOrphans` y `TestProxyJobKeepsGoing` (una red ajena con el nombre de un stage no corta el job: muelle-caddy y el `Sync` van igual) se sumaron a la lista.

  **Commit:** `feat(ingress): el job proxy asegura redes y muelle-caddy, borra huérfanas y reenvía la configuración`

- [x] **Step 5: `GET /api/v1/proxy`.**

  **Tests primero** (`infra/in/http/proxy_test.go`, con `httptestutil` y los casos de uso armados en el test: `GetProxyStatus` con sqlite, el `ProxySettingsReader`, un `caddy.Runtime` sobre `dockertest.Fake` y un `ProxyState`). No usan un `ingress.Module`: el paquete `ingress` importa este, y su test no puede importarlo de vuelta (ciclo). El camino completo, del job a `GET /proxy`, lo prueba `TestModuleProxyStatusAfterJob` (Step 6).
  - `TestProxyStatusInitial`: con `Enabled` y sin contenedor, `200`:
    ```json
    {"status":"out_of_sync","error":"muelle todavía no le mandó la configuración a Caddy","synced_at":null,"caddy":{"image":null,"running":false,"http_port":80,"https_port":443}}
    ```
    Compara el mapa decodificado entero: están todas las claves, y los null lo son.
  - `TestProxyStatusSynced`: después de `runtime.Ensure`, `state.ContainerOK()` y `state.Loaded(t)`: `status "synced"`, `error null`, `synced_at` en RFC 3339 con ms, y `caddy.image` la del contenedor, con `running true` y los puertos de los ajustes.
  - `TestProxyStatusDisabled`: sin `panel_domain`, y con el fake `SetUnavailable(true)`:
    - `200 {"status":"disabled","error":null,…,"caddy":{"image":null,"running":false,…}}`;
    - `Calls()` sin `InspectContainer` (contrato §3.3).
  - `TestProxyStatusDockerDown`: con `Enabled`, `SetUnavailable` da `503 docker_unavailable`.
  - `TestProxyStatusRecreating`: después de `state.BeginRecreate()`, `status "recreating"` con `error null`.
  - `TestProxyStatusToken`: con un Bearer, `403 token_not_allowed`.

  **Implementar:**
  - `dto/proxy.go`:
  ```go
  type ProxyOutput struct {
  	Status   string      `json:"status"`
  	Error    *string     `json:"error"`
  	SyncedAt *time.Time  `json:"synced_at"`
  	Caddy    CaddyOutput `json:"caddy"`
  }
  type CaddyOutput struct {
  	Image     *string `json:"image"`
  	Running   bool    `json:"running"`
  	HTTPPort  int     `json:"http_port"`
  	HTTPSPort int     `json:"https_port"`
  }
  ```
  - `usecases/proxy_status.go`: `GetProxyStatus.Execute(ctx)`. Lee los ajustes; si está `disabled`, no consulta Docker; si no, `runtime.Status`.
  - `infra/in/http/handler.go`: `type UseCases struct{ ProxyStatus *usecases.GetProxyStatus }` y `func Register(k *utils.Kit, uc UseCases)`, que llama a `RegisterErrors` y registra `k.Handle("GET /api/v1/proxy", utils.AccessSession, h.proxy)`.
  - `module.go`: `RegisterRoutes` llama a `inhttp.Register`.
  - Hecho así: el cambio de `module.go` pasó al Step 6, junto con el resto del cableado (el `caddy.Runtime` que necesita `GetProxyStatus` se arma ahí) y con `TestModuleRoutes`, que hasta ese commit todavía espera `no_route`. `synced_at` sale en UTC y cortado a ms (`TestProxyStatusSynced` lo prueba con una hora de otra zona y nanosegundos).

  **Commit:** `feat(ingress): GET /api/v1/proxy con el estado del proxy (contrato §3.3)`

- [x] **Step 6: Cableado en el módulo.**

  **Tests primero** (`module_test.go`):
  - `TestModuleJobs`: `m.Jobs()` tiene `"proxy"` cada 5 min.
  - `TestModuleRun`: `Run` vuelve al cancelar, con el `Retrier` y el `Recreator` corriendo.
  - `TestModuleRoutes`: `GET /api/v1/proxy` ya no es `no_route`, y `…/domains` sigue siéndolo.
  - `TestModuleProxyStatusAfterJob`: con `Enabled`, `dockertest` y `caddytest`, después de `m.Jobs()[0].Run`, `GET /api/v1/proxy` da `synced` con la imagen del contenedor.
  - `TestModuleRecreatesWithCustomName`: con `Options.CaddyContainer "it3-x"`, el job crea `it3-x`, y ninguna entrada de `Calls()` nombra `muelle-caddy`. (Las redes de stage ya lo prueban desde T2 Step 5.)

  También, en `src/shared/infra/container/container_test.go` (solo esa línea de `TestNew`):
  ```go
  if got := strings.Join(names, ", "); got != "sesiones vencidas, retención de auditoría, backup diario, proxy" {
  ```
  Y su comentario: `// identity (sesiones vencidas) + admin (retención y backup) + ingress (proxy).`

  **Implementar** `module.go`:
  - `Module` guarda `state`, `syncer`, `caddy`, `orphans`, `proxyJob`, `retrier`, `recreator`, `jobs` y `httpUC`.
  - `New` arma el `caddy.Runtime` con `Names` de `Options`.
  - `Jobs()` devuelve `m.jobs`.
  - `Run(ctx)` hace `wg.Go(retrier.Run)` y `wg.Go(recreator.Run)`, y después `wg.Wait()`.
  - Un método sin exportar `scheduleRecreate()` que llama a `recreator.Schedule()`; lo usa la integración (Step 7). T5 le pasa el `Recreator` a `ProxySettingsChanged`, que usa `Hold`.
  - Hecho así: también `RegisterRoutes` → `inhttp.Register(k, m.httpUC)` (lo que el Step 5 dejaba en `module.go`). `TestModuleStubs` queda solo con `CheckCloudflareToken` y `ProxySettingsChanged`; `TestModuleRun` prueba el cableado de verdad: un `SyncRoutes` rechazado despierta al `Retrier` (el `onFail`) hasta `synced`, y `scheduleRecreate` hace que el `Recreator` cree muelle-caddy.

  **Commit:** `feat(ingress): el módulo corre el job proxy, los reintentos y la recreación`

- [x] **Step 7: Integración — el ciclo de vida contra Docker real.**

  `TestIntegrationCaddyLifecycle` (ranura 2), con el módulo armado con `e.opts` y el cliente de Docker real:
  1. **Primera pasada:** arranca el puente, siembra el stage `s1` y la app `web` con su dominio, y arranca el backend.
     - `job()` (`m.Jobs()[0].Run`) da `nil`: crea muelle-caddy con los nombres `it3` y lo conecta a `it3<id>_s1`.
     - `eventually`: el estado da `synced` y `get("web.it3<id>.test")` da `200 "hola web"`.
  2. **Red nueva en caliente:** con el id y `StartedAt` del contenedor guardados, `ingresstest.Stage(…, project, "s2")` y después `m.EnsureStageNetwork(ctx, project, "s2")`.
     - **Ojo (visto en T2):** conectar un contenedor a una overlay recién creada puede fallar con "attaching to network failed … context deadline exceeded" mientras Swarm no le dio subred (`Subnets` vacío). `EnsureStageNetwork` crea la red y conecta muelle-caddy enseguida. Si este paso lo muestra, el arreglo va en el código (que `DockerNetworks.CreateOverlay`, o `Connect`, espere la subred con un plazo), no en el test: en producción es crear un stage. El stage va en la base **antes**: si no, el `job()` de los pasos siguientes borraría `it3<id>_s2` por huérfana. Queda el mismo id y `StartedAt`, y `Networks` incluye `it3<id>_s2` (spec §15).
  3. **Aislamiento entre stages** (spec §15: "un contenedor de un stage no resuelve ni alcanza servicios de otro stage"):
     - arranca el backend `web2` en `s2`, siembra la app `web2` en `s2` con el dominio `web2.it3<id>.test` → `web2:8080`, y corre `job()`;
     - `eventually(30 s)`: `get("web2.it3<id>.test")` da `200 "hola web2"`. Así `web2` ya responde, y el negativo de la sonda no pasa solo porque `web2` todavía no arrancó;
     - arranca la sonda `probe` en `s1` con `startBackendCmd` y `Cmd ["sh", "-c", "ok=0; for i in $(seq 1 20); do wget -T 3 -q -O /dev/null http://<alias de web en s1>:8080 && { ok=1; break; }; sleep 1; done; [ $ok = 1 ] || exit 1; wget -T 3 -q -O /dev/null http://<alias de web2 en s2>:8080 && exit 1; exec caddy respond --listen :8080 --body aislado"]`, con los alias de `naming.Service`. El control se reintenta hasta 20 veces: el DNS de la red overlay puede tardar un momento en resolver `web`, y un primer `wget` fallido no puede hacer salir la sonda para siempre;
     - siembra la app `probe` en `s1` con el dominio `probe.it3<id>.test` → `probe:8080`, y corre `job()`;
     - `eventually(60 s)`: `get("probe.it3<id>.test")` da `200 "aislado"`.
     - La sonda solo sirve si alcanzó su propio stage y **no** alcanzó el otro; si no, sale, y el `get` da `502` hasta que vence el `eventually`. El caso de control (el `web` de `s1`) prueba que el `wget` funciona. Así no hace falta `exec` ni el código de salida, que el cliente de Docker no tiene.
  4. **Recreación:** los ajustes cambian a los puertos de repuesto (`HTTP2`/`HTTPS2`); `m.scheduleRecreate()` con `e.run(m)` (nunca `go m.Run` a mano) y `Timing.RecreateDelay: 50ms`.
     - `eventually`: hay un id nuevo, con `PublishedPorts` en los puertos nuevos;
     - el estado da `synced`;
     - `get` por el puerto nuevo da `200` (`e.ports.HTTPS = HTTPS2` para el helper).
  5. **Puerto ocupado:** `net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", Busy))`, como `TestIntegrationPortInUse` del paquete docker, y los ajustes cambian el HTTP a `Busy`.
     - `job()` devuelve un error con `ErrPortInUse`;
     - `GET /api/v1/proxy` (con `httptestutil` y el módulo) da `out_of_sync` con un `error` que nombra el puerto;
     - el contenedor no existe.
     - Se cierra el listener y `job()` lo crea de nuevo: queda `synced`.
  6. **Huérfana:** una red `it3<id>_viejo`, overlay attachable con las etiquetas `{project: "it3<id>", stage: "viejo"}`, sigue después del primer `job()` y no existe después del segundo.

  **Correr:** el comando de integración. Expected: PASS. Después, `docker ps -a --filter name=it3-` no muestra nada.

  Hecho así:
  - En el paso 1, `m.EnsureStageNetwork(…, "s1")` va antes de `startBackend` (el backend necesita la red), y una segunda pasada del job no recrea nada (el mismo id: lo que devuelve Docker coincide con lo que pidió muelle). La red nueva del paso 2 se conectó en caliente sin el problema de la subred que vio T2.
  - **La primera corrida dejó una red que no se puede borrar:** el paso 5 encontró que Docker no suelta la conexión de Swarm de la red panel cuando el arranque de muelle-caddy falla por el puerto ocupado (D11). El arreglo, en el código (`b8a7fc3`): los puertos se prueban antes con `<CaddyContainer>-probe`, sin redes de Swarm. La limpieza de `itEnv` borra también ese contenedor. La red `it3febd_panel` de esa corrida sigue en el nodo (la tarea `attachment` solo se va reiniciando el daemon de Docker).
  - `itEnv.caddy()` inspecciona el muelle-caddy del test, y `itEnv.proxyStatus(m, want)` es un check de `eventually` sobre el estado.
  - Tres corridas seguidas pasan (~17 s), y después `docker ps -a`, `docker network ls` y `docker volume ls` con `it3` no muestran nada del test.

  **Commit:** `test(ingress): integración del ciclo de vida de muelle-caddy (redes en caliente, aislamiento entre stages, recreación, puerto ocupado, huérfanas)`

- [x] **Step 8: Cierre de T3.** Las verificaciones de "Convenciones" pasan, y los tests de integración de T2 y T3 también.
  - Hecho: gofmt, `go vet ./...` (y con `-tags integration` en ingress), `go test -race -count=1 ./...` (53 paquetes), `go mod tidy -diff` y `go test -tags integration -race -count=1 -run Integration ./src/internal/ingress/...` (`TestIntegrationCaddyAcceptsConfig` ~7 s y `TestIntegrationCaddyLifecycle` ~16 s). Después no queda nada con `it3` salvo la red `it3febd_panel` de la primera corrida del Step 7 (la tarea `attachment` de Docker; se va reiniciando el daemon).
  - Para T5: el contrato §3.4 todavía no nombra `muelle-caddy-probe` (D11) ni la espera al Caddy recién creado (`syncStarting`).

---

### Task T4: Dominios y certificados

**Objetivo:**
- La API de dominios del contrato §3.1/§3.2: listar, crear, editar, borrar, adoptar y reintentar, con Cloudflare, el DNS ajeno, `proxied` y la auditoría.
- Cada cambio aplica la ruta de Caddy sin deploy.
- La verificación de certificados de §8: a los 30 s, cada minuto los `pending` y el job `"certificados"` cada hora, con las transiciones exactas.

**Files:**
- Create: `src/internal/ingress/domain/service/certs.go` + `certs_test.go`; Modify: `ports.go` (`CertProbe`)
- Create: `src/internal/ingress/infra/out/service/certs.go` + `certs_test.go`
- Create: `src/internal/ingress/app/dto/domains.go`
- Create: `src/internal/ingress/app/usecases/{ops,messages,dns,domains,adopt,retry_domain,certs}.go` + `domains_test.go`, `adopt_test.go`, `retry_domain_test.go`, `certs_test.go`, `dns_test.go`
- Create: `src/internal/ingress/infra/in/http/{domains,body}.go` + `domains_test.go`; Modify: `handler.go`, `errors.go`
- Modify: `src/internal/ingress/infra/in/cron/jobs.go` + test
- Modify: `src/internal/ingress/module.go`, `module_test.go`
- Modify: `src/shared/infra/container/container_test.go` (la línea de `TestNew`)
- Modify: `src/internal/ingress/integration_test.go`

- [x] **Step 1: La regla de los certificados (pura).**

  **Tests primero** (`domain/service/certs_test.go`), una tabla de `ApplyCert`. `now = T0`, y `good` vence en 60 días:

  | Estado de partida | Observación | Resultado |
  |---|---|---|
  | `pending` | `good` | `active`, `UpdatedAt = now`, issuer y vencimiento guardados |
  | `pending`, 5 min después de `UpdatedAt` | inválido | nada cambia (`changed == false`) |
  | `pending`, 5 min después de `UpdatedAt` | válido pero vence en 10 días | sigue `pending`, sin error (el plazo no venció); el issuer y el vencimiento se guardan (`changed == true`) y `UpdatedAt` no se mueve |
  | `pending`, 10 min después | inválido | `error` + `certificate`, con el `Problem` en el mensaje |
  | `pending`, 10 min después | válido pero vence en 10 días | `error` + `certificate`; el issuer y el vencimiento se guardan |
  | `active` | `good` con otro issuer | sigue `active`, el issuer cambia, `UpdatedAt` no se mueve |
  | `active` | vence en 13 días | `error` + `certificate`, con el mensaje "el certificado vence el …" |
  | `active` | inválido | `error` + `certificate` |
  | `error` + `certificate` | `good` | `active` |
  | `error` + `certificate` | inválido (sin retry) | nada |
  | `error` + `certificate` | inválido con `retry` | `pending` con `UpdatedAt = now` |
  | `pending` | inválido con `retry` | nada: `retry` no reinicia el plazo |
  | `error` + `cloudflare` o `record_exists` | cualquiera | nada |

  **Implementar:**
  ```go
  const (
  	RenewMargin  = 14 * 24 * time.Hour
  	PendingGrace = 10 * time.Minute
  )
  // CertObservation es lo que vio la verificación en el certificado que sirve Caddy.
  type CertObservation struct {
  	Valid    bool
  	Issuer   string
  	NotAfter time.Time
  	Problem  string // por qué no es válido, en español
  }
  // CertProbe se conecta por TLS a Caddy con SNI = hostname (adaptador: TLSProbe).
  // ErrCaddyUnreachable si no llega (D9).
  type CertProbe interface {
  	Probe(ctx context.Context, hostname string) (CertObservation, error)
  }
  // ApplyCert aplica las reglas del contrato §3.1 y dice si cambió algo. No lo
  // llama nadie con un proxy que no está synced (ese caso es "no se sabe").
  func ApplyCert(d *entity.Domain, o CertObservation, now time.Time, retry bool) (changed bool)
  ```
  Los mensajes:
  - **Plazo vencido:** "Caddy no consiguió un certificado válido en 10 minutos: <problema>". Con `tls_mode: http` agrega: "revisa que el registro DNS apunte a la IP pública y que los puertos 80 y 443 lleguen a muelle-caddy". Con `cloudflare`: "revisa que el token de Cloudflare pueda editar el DNS de la zona".
  - **Por vencer:** "el certificado vence el 2 ene 2027 y Caddy no lo renovó". La fecha con meses en español: escribe un helper chico, sin depender del locale.

  **Commit:** `feat(ingress): la regla de los certificados (pending, active y error, con los 14 días y los 10 minutos)`

- [x] **Step 2: `TLSProbe`.**

  **Tests primero** (`infra/out/service/certs_test.go`). Usa una CA de prueba generada en el test con `crypto/x509` (raíz, intermedia y hoja para `a.it3.test`), y `httptest.NewUnstartedServer` con `TLS.Certificates`:
  - `TestProbeValid`:
    - con la raíz en `Roots`: `Valid`, `Issuer` = la `Organization` de la intermedia y `NotAfter` (UTC, ms);
    - si la intermedia no tiene `Organization`, `Issuer` es su CN.
  - `TestProbeUnknownCA`: sin la raíz, `Valid == false` y el `Problem` en español menciona la CA.
  - `TestProbeWrongName`: un SNI de otro nombre da `Valid == false`.
  - `TestProbeExpired`: `Valid == false`.
  - `TestProbeHandshakeFails`: un listener que cierra al aceptar da `Valid == false` ("Caddy no presentó un certificado para …"), sin error.
  - `TestProbeUnreachable`: un puerto sin nadie da `errors.Is(err, service.ErrCaddyUnreachable)`.
  - `TestProbeHandshakeTimeout` (sumado al implementar): un listener que acepta y no responde, con `Timeout` 50 ms, da `ErrCaddyUnreachable` y no un inválido. Un Caddy colgado es "no se sabe" (D9): si no, la verificación de cada hora pasaría los `active` a `error`.

  **Implementar:**
  ```go
  type TLSProbe struct {
  	Addr    string
  	Roots   *x509.CertPool
  	Timeout time.Duration // 5 s
  }
  ```
  - Dial TCP; si falla, `ErrCaddyUnreachable`.
  - `tls.Client` con `ServerName = host`, `InsecureSkipVerify: true` y TLS 1.2 como mínimo.
  - `HandshakeContext`; si falla, inválido, salvo que se haya vencido el plazo (`ErrCaddyUnreachable`).
  - `leaf.Verify(x509.VerifyOptions{DNSName: host, Roots, Intermediates: <el resto de la cadena>})`.

  **Commit:** `feat(ingress): verificar el certificado que sirve Caddy por TLS con SNI`

- [x] **Step 3: `Ops`, los mensajes y el DNS de un dominio.**

  **Tests primero** (`usecases/dns_test.go`, contra `cftest` y el cliente real; ver "Convenciones"), para `dnsOps.ensure`. La zona es `z1 tudominio.cl`, y `public_ip`/`public_ipv6` son `203.0.113.24`/`2001:db8::24`:
  - `TestEnsureNoToken`: `tls http`, `pending`, `proxied false` (aunque se pidió `true`), sin ids y sin llamadas a `cftest`.
  - `TestEnsureNoZone`: el host `api.otro.com` da `tls http`, `pending`, `proxied false` y `CFZoneID ""`.
  - `TestEnsureCreates`: crea el `A` y el `AAAA` con el `proxied` pedido y `comment "muelle"`, y queda `pending`, `tls cloudflare`, con `CFZoneID`, `CFRecordID` y `CFRecordIDAAAA`.
  - `TestEnsureNoPublicIP`: `error` + `cloudflare` con el mensaje "configura la IP pública en Ajustes", `tls cloudflare` y la zona guardada.
  - `TestEnsureForeign`, uno por tipo (`A`, `AAAA` y `CNAME` ajenos), con `proxied: true` pedido:
    - `error` + `record_exists`, con `tls cloudflare` y `CFZoneID == "z1"` (D5, D7);
    - `CFRecordID` y `CFRecordIDAAAA` vacíos;
    - `Proxied == true`, el pedido;
    - `cftest` no recibió `POST`/`PATCH`/`DELETE`.
  - `TestEnsureZonesFail`: con `SetDown`, `error` + `cloudflare`, `tls http` y el `proxied` pedido se conserva (D26).
  - `TestEnsureAAAAFails`: si crear el `AAAA` falla después de crear el `A`, queda `CFRecordID` sin `AAAA` y `error` + `cloudflare`. (Al implementar: `FailNext("POST", "dns_records", …)` le pega al primer `POST`, el del `A`; el test envuelve el `service.DNS` en un tipo que rechaza solo el `CreateRecord` de un `AAAA`.)
  - `TestEnsureManaged`: con el `A` propio, actualiza su contenido y su `proxied`, crea el `AAAA` que falta y queda `pending`. Sin `public_ipv6` y con el `AAAA` propio, lo borra y limpia el id.
  - `TestEnsureManagedAGone` (D29): con los ids de un `A` que ya no está en `cftest` y de un `AAAA` que sí está:
    - el `AAAA` se borra en `cftest`;
    - se crean un `A` y un `AAAA` nuevos, con ids nuevos, y queda `pending`;
    - `repo.Update` del dominio pasa: el `CHECK` de D19 se cumple.
    - Con `FailNext("DELETE", …, 403)` en el `AAAA`: los dos ids se olvidan, ese `AAAA` queda en `cftest` y el dominio da `record_exists`.
  - `TestEnsureManagedNoToken`: `error` + `cloudflare`, con el mensaje del token (D18).
  - Sumados al implementar: `TestEnsureManagedForeignAAAA` (con el `A` propio y un `AAAA` ajeno, no crea el `AAAA`: `error` + `cloudflare`, contrato §3.5), `TestEnsureManagedAGoneDeleteFails` (la segunda mitad de `TestEnsureManagedAGone`, en su propio test) y `TestFindZone`.
  - `setProxied`, `remove` y `adopt` se prueban en el Step 5, por sus casos de uso, y se implementan ahí.

  **Implementar:**
  - `ops.go`:
  ```go
  // Ops serializa lo que escribe dominios o registros DNS (D6).
  type Ops struct{ mu sync.Mutex }
  // Do arma opCtx = WithTimeout(WithoutCancel(ctx), limit) ANTES de esperar el
  // lock (si ctx tiene un plazo más corto, usa ese), y corre fn con el lock
  // tomado: opCtx para Cloudflare, Caddy y TLS, y dbCtx (sin cancelar) para la base.
  func (o *Ops) Do(ctx context.Context, limit time.Duration, fn func(opCtx, dbCtx context.Context) error) error
  ```
  Tests de `Ops` (`ops_test.go`):
  - `TestOpsSerializes`: dos `Do` a la vez nunca corren juntos.
  - `TestOpsDeadlineIncludesWait`: con un `Do` que tiene el lock 40 ms, otro `Do` con `limit` 50 ms recibe un `opCtx` al que le quedan 10 ms o menos.
  - `TestOpsShorterParent`: con un `ctx` que vence en 20 ms y `limit` 1 min, `opCtx` vence cerca de los 20 ms, y `dbCtx` no.
  - `TestOpsCanceledParent`: un `ctx` ya cancelado corre fn igual; `opCtx` y `dbCtx` no están cancelados.
  - `messages.go`: los textos en español en un solo lugar (`msgNoPublicIP`, `msgNoToken`, `msgRecordExists(typ, host)`, `msgMultiple(typ, host)`, `msgForeignAAAA(host)`, `msgRecordGone(typ, host, aaaaKept)` para D29, `cloudflareMsg(err)` → "Cloudflare no responde; reintenta en unos minutos" o el texto de `*DNSError`).
  - `dns.go`:
  ```go
  type dnsOps struct {
  	dns   service.DNS
  	clock clock.Clock
  }
  // ensure aplica a d las reglas de crear (contrato §3.1) con los ajustes p.
  // No guarda: el que llama guarda d. Con registros propios, los actualiza (si
  // el A ya no está, D29, sigue como sin registros). Si no, busca la zona y, al
  // encontrarla, fija CFZoneID y TLSCloudflare (D5) antes de revisar los
  // registros ajenos y crear los suyos.
  func (o dnsOps) ensure(ctx context.Context, d *entity.Domain, p shared.ProxySettings)
  // findZone: solo busca la zona y, si la encuentra, hace ensure. Es para retry
  // "sin zona" (D23). found false: no tocó d.
  func (o dnsOps) findZone(ctx context.Context, d *entity.Domain, p shared.ProxySettings) (found bool)
  // adopt usa d.CFZoneID, que record_exists siempre trae (D7): no vuelve a
  // buscar la zona. Deja TLSCloudflare.
  func (o dnsOps) adopt(ctx context.Context, d *entity.Domain, p shared.ProxySettings)
  // setProxied: si el A ya no está, D29 y error + cloudflare.
  func (o dnsOps) setProxied(ctx context.Context, d *entity.Domain, p shared.ProxySettings, proxied bool)
  // remove borra los registros propios; un 404 no es error. Devuelve kept si
  // había registros propios y no hay token. Cualquier otro fallo es un error
  // que envuelve entity.ErrCloudflareUnavailable (502, contrato §3.2).
  func (o dnsOps) remove(ctx context.Context, d entity.Domain, p shared.ProxySettings) (kept bool, err error)
  ```

  **Commit:** `feat(ingress): el DNS de un dominio en Cloudflare (zona, registros propios y ajenos)`

- [x] **Step 4: `CertChecker` y su `Run`.**

  Va antes de los casos de uso: crear, adoptar y reintentar llaman a `ScheduleFirst` y a `checkLocked`. No depende de ellos.

  **Tests primero** (`usecases/certs_test.go`):
  - `TestCheckNotSynced`: con el estado inicial, no se llama a la sonda y nada cambia.
  - `TestCheckUnreachable`: con `ErrCaddyUnreachable`, nada cambia.
  - `TestCheckAppliesRule`: un `pending` con la sonda válida queda `active` en la base.
  - `TestCheckSkipsChangedMeanwhile`: el dominio cambia entre la sonda y la aplicación (un probe falso que bloquea en un canal, y un `repo.Update` con otro `UpdatedAt` mientras tanto, como haría un `PATCH`) → el resultado de la sonda no se aplica.
  - `TestCheckLockedRetry`: `checkLocked` dentro de un `Ops.Do`, sobre un `error` + `certificate` con la sonda inválida, lo deja `pending` y devuelve `repush == true`; sobre un `pending` con la sonda inválida, no cambia nada y `repush == false`. (Al implementar: `checkLocked` no llama a `Sync`, porque D6 lo pone fuera del lock; el `Load` con `Forced() == true` lo manda `retry` después del `Ops.Do`, y lo prueba `retry_domain_test.go`.)
  - `TestCheckAll`: el filtro de cada hora mira `pending`, `active` y `error` + `certificate`, y no mira `error` + `cloudflare` (la sonda no recibe ese host).
  - `TestCertRun`:
    - con `Timing{FirstCheck: 10ms, PendingEvery: 30ms}`, `ScheduleFirst(host)` dispara una sonda de ese host cerca de los 10 ms;
    - el barrido llama a la sonda para los `pending`;
    - `Run` vuelve al cancelar.

  **Implementar:**
  ```go
  type CertChecker struct { /* probe; repo; state; ops; clock; first chan string (64); firstDelay, every, opLimit; log */ }
  // ScheduleFirst agenda la verificación de los 30 s (D10). No bloquea: si la cola está llena, la cubre el barrido.
  func (c *CertChecker) ScheduleFirst(hostname string)
  // Check sondea fuera del lock y aplica ApplyCert dentro, solo si el UpdatedAt
  // no cambió entretanto.
  func (c *CertChecker) Check(ctx context.Context, hostname string) error
  // checkLocked es para retry, que ya tiene el lock: sondea y aplica con retry
  // = true. repush: un certificate volvió a pending, y retry, ya fuera del
  // lock (D6), hace Sync(force).
  func (c *CertChecker) checkLocked(opCtx, dbCtx context.Context, d *entity.Domain) (repush bool, err error)
  func (c *CertChecker) CheckAll(ctx context.Context, include func(entity.Domain) bool) error
  func (c *CertChecker) Run(ctx context.Context) // las ScheduleFirst y el ticker de los pending, con un WaitGroup que espera
  ```
  `infra/in/cron/jobs.go` suma `{Name: "certificados", Every: time.Hour}`, con `CheckAll(EligibleHourly)`: `Jobs(proxy, certs)`.

  Como `Jobs` cambia de firma, este paso ya cablea en `module.go` el `TLSProbe`, `Ops`, el `CertChecker`, el job y `wg.Go(certs.Run)`, con `TestModuleJobs` y la línea de `TestNew` (`container_test.go`). El Step 7 suma los casos de uso.

  **Commit:** `feat(ingress): verificación de certificados a los 30 s, cada minuto los pending y cada hora`

- [x] **Step 5: Los casos de uso de dominios.**

  **Tests primero** (`usecases/domains_test.go`, `adopt_test.go`, `retry_domain_test.go`). Usan un fixture con:
  - sqlite e `ingresstest`;
  - la app de formulario `auditfy/staging/api` y la compose `auditfy/staging/infra`;
  - `cftest` y el cliente;
  - `caddytest` y el `Syncer`;
  - `clock.Manual`;
  - `sqlite.NewAuditRecorder`;
  - un `CertProbe` falso (devuelve lo que pone el test, o `ErrCaddyUnreachable`), dentro del `CertChecker` real del Step 4 (sin `Run`: el test lee su cola `first` para ver lo que agendó `ScheduleFirst`);
  - el estado `Loaded` para que esté `synced`.

  Lo que tienen que cubrir:
  - **Crear:**
    - el dominio vuelve con `records` calculados;
    - después de guardar llegó un `Load` con el host;
    - se agendó la verificación (la cola `first` del `CertChecker` recibió el hostname);
    - auditoría `domain.create`, target `domain:<host>` y details `{"app":"auditfy/staging/api"}`;
    - errores:
      - el hostname del panel: `422` en `hostname` ("es el dominio del panel");
      - en una app de formulario, un `target_service` que no es su slug: `422` en `target_service`;
      - en la compose, cualquier slug válido pasa;
      - un hostname que ya usa otra app: `ErrHostnameTaken`;
      - una app que no existe: `repository.ErrNotFound`;
    - **el cliente se va:** un ctx ya cancelado igual crea los registros y guarda (D6), y el `Sync` de después igual llega a `caddytest`: el estado queda `synced`, no `out_of_sync`;
    - **plazo:** con `cftest.Hang(200ms)` y `Timing.DomainOp: 50ms`, el dominio queda guardado en `error` + `cloudflare`, y `Execute` no tarda mucho más que el plazo.
  - **Listar:** solo los de la app, ordenados por hostname, `[]` sin dominios y `records` con los ajustes **actuales** (cambia `public_ip` y se ve).
  - **Editar:**
    - `{}` no cambia nada: el mismo `UpdatedAt` y sin auditoría;
    - un destino nuevo guarda, manda un `Load` con el upstream nuevo y audita `domain.update`;
    - en la app de formulario, un `target_service` que no es su slug: `422` en `target_service`, sin guardar ni auditar (la validación del contrato §3.2 vale también en el `PATCH`); en la compose, otro slug válido pasa;
    - `proxied` sobre un dominio sin `cf_record_id`: `422` en `proxied` con "muelle no maneja el registro DNS de este dominio";
    - `proxied` con registros propios cambia el `A` y el `AAAA` en `cftest`, y un `error` + `cloudflare` vuelve a `pending`;
    - si Cloudflare falla (o no hay token), el valor se guarda, queda `error` + `cloudflare` y no se devuelve error;
    - un dominio de otra app: `ErrNotFound`.
  - **Borrar:**
    - con registros propios y token, se borran en `cftest`, después la fila, un `Load` y la auditoría `domain.delete` (sin `dns`);
    - un registro que ya no está (404) no es error;
    - si `cftest` está caído o rechaza (`FailNext DELETE 403`), es un error con `entity.ErrCloudflareUnavailable` y la fila queda;
    - sin token y con registros propios, se borra solo en muelle, con details `{"app":…,"dns":"kept"}`.
  - **Adoptar:** cada caso de D17 y del contrato §3.2. En todos los que adoptan o crean, el dominio queda con `tls_mode: cloudflare` y la misma `cf_zone_id` que tenía en `record_exists`, y `cftest` no recibió un `GET /zones` (D7):
    - un `A` ajeno con `proxied: true` pasa a la IP, conserva `proxied: true`, queda `cf_record_id` y `pending`;
    - un `CNAME` pasa a `A` (un `PATCH` con `type` y `content`);
    - dos `A`: `error` + `cloudflare` con "deja uno solo", sin tocar nada;
    - con `public_ipv6`, un `AAAA` ajeno se adopta con su `proxied`; sin `AAAA`, se crea con el del `A`; sin `public_ipv6`, el `AAAA` ajeno no se toca;
    - **solo un `AAAA` ajeno** (D32), con `proxied: true`:
      - con `public_ipv6`: se crea el `A` con `proxied: true` y después se adopta el `AAAA` (en `Calls()`, el `POST` antes del `PATCH`); quedan los dos ids, `Proxied == true` y `pending`;
      - sin `public_ipv6`: se crea el `A` con `proxied: true`, y el `AAAA` queda igual en `cftest`, sin `cf_record_id_aaaa`;
      - si el `POST` del `A` falla, el `AAAA` no se tocó y no queda `cf_record_id_aaaa` (D19);
    - **más de un `AAAA`** ajeno: con `public_ipv6`, `error` + `cloudflare` con "deja uno solo", sin tocar nada; sin `public_ipv6`, no cuentan, y el `A` se adopta;
    - sin token o sin `public_ip`, `error` + `cloudflare` con el mensaje de lo que falta;
    - si el ajeno ya no está, crea los suyos;
    - un estado que no es `record_exists`: `ErrNothingToAdopt`;
    - auditoría `domain.adopt` y verificación agendada.
  - **Reintentar** (D23):
    - `cloudflare` con el `A` propio: lo actualiza y crea el `AAAA`;
    - `record_exists` que sigue: igual; que ya no está: crea y queda `pending`;
    - `pending`, con la sonda válida: `active`; con la sonda inválida: sigue `pending` con el mismo `UpdatedAt`;
    - `certificate`, con la sonda inválida: `pending` con `UpdatedAt = now` y un `Load` con `Forced() == true`; con la válida: `active`;
    - sin zona, con un token y una zona que ahora sí están: crea los registros, `tls cloudflare` y un `Load`;
    - con el proxy sin `synced`, o la sonda con `ErrCaddyUnreachable`, nada cambia;
    - auditoría `domain.retry`;
    - el plazo `Timing.DomainRetry` se respeta (como en crear).

  **Implementar:**
  - `dto/domains.go`:
  ```go
  type AppRef struct{ Project, Stage, App string }
  type CreateDomainInput struct {
  	AppRef        `json:"-"`
  	Hostname      string `json:"hostname"`
  	TargetService string `json:"target_service"`
  	TargetPort    int    `json:"target_port"`
  	Proxied       *bool  `json:"proxied"`
  }
  type UpdateDomainInput struct {
  	AppRef        `json:"-"`
  	Hostname      string  `json:"-"` // el de la ruta
  	Proxied       *bool   `json:"proxied"`
  	TargetService *string `json:"target_service"`
  	TargetPort    *int    `json:"target_port"`
  }
  type DomainRef struct {
  	AppRef
  	Hostname string
  }
  type RecordOutput struct {
  	Type    string `json:"type"`
  	Name    string `json:"name"`
  	Content string `json:"content"`
  }
  // DomainOutput es el dominio del contrato §3.1: todas las claves siempre, null donde no hay valor.
  type DomainOutput struct {
  	Hostname      string         `json:"hostname"`
  	TargetService string         `json:"target_service"`
  	TargetPort    int            `json:"target_port"`
  	Proxied       bool           `json:"proxied"`
  	TLSMode       string         `json:"tls_mode"`
  	CFZoneID      *string        `json:"cf_zone_id"`
  	CFRecordID    *string        `json:"cf_record_id"`
  	Status        string         `json:"status"`
  	LastErrorCode *string        `json:"last_error_code"`
  	LastError     *string        `json:"last_error"`
  	CertIssuer    *string        `json:"cert_issuer"`
  	CertExpiresAt *time.Time     `json:"cert_expires_at"`
  	Records       []RecordOutput `json:"records"` // nunca nil
  	CreatedAt     time.Time      `json:"created_at"`
  	UpdatedAt     time.Time      `json:"updated_at"`
  }
  ```
  - `usecases/domains.go`: un `DomainDeps` compartido (`Repo`, `Catalog`, `Settings`, `DNS`, `Syncer`, `Certs`, `Ops`, `Clock`, `Audit`, `Log`, y los plazos `DomainOp` y `DomainRetry`: `Timing` está en el paquete `ingress`, que `usecases` no puede importar).
    - La app y los ajustes se leen con `context.WithoutCancel(ctx)` (hallado al implementar): con un `ctx` ya cancelado, la lectura de la app fallaba antes del `Ops.Do`, y el test "el cliente se va" no creaba nada.
    - `PATCH` lee el dominio y valida **dentro** del `Ops.Do`: así el `422` de `proxied` mira los registros de ese momento, no los de antes de esperar el lock.
    - Las fixtures de los Steps 4 y 5 viven en `domains_fixture_test.go` (`ucFixture` y `fakeProbe`).
    - Constructores: `NewListDomains`, `NewCreateDomain`, `NewUpdateDomain` y `NewDeleteDomain`; en sus archivos, `NewAdoptDomain` y `NewRetryDomain`. Cada uno con `Execute(ctx, in)`.
    - `toOutput(d, p)` arma la salida: los `*string` son nil para `""`, y `records` sale de `entity.Records(d.Hostname, p.PublicIP, p.PublicIPv6)`.
    - **Orden de crear** (D5):
      1. la app (404);
      2. `entity.NewDomain` (422);
      3. el dominio del panel y la regla de la app de formulario (422);
      4. `Ops.Do(DomainOp)`: `repo.Create` (409), `ensure`, `repo.Update`;
      5. fuera del lock: `syncer.Sync(context.WithoutCancel(ctx), false)`, ignorando su error (queda en el estado), y después `certs.ScheduleFirst(host)` (D6);
      6. la auditoría.
    - **Editar** sigue el mismo orden: el dominio (404), `ValidateTarget` y la regla de la app de formulario sobre el destino que queda (422), `Ops.Do`, y el `Sync` con `context.WithoutCancel`. Borrar, adoptar y reintentar también hacen su `Sync` así.
    - `Delete` responde el error de `remove` tal cual: el `502` sale de `ErrCloudflareUnavailable`, que ya está registrado en `cmd/http`.

  **Commit** (dos): `feat(ingress): crear, listar, editar y borrar dominios con su DNS y su ruta` y `feat(ingress): adoptar y reintentar un dominio`

- [x] **Step 6: HTTP de los dominios.**

  **Tests primero** (`infra/in/http/domains_test.go`, con `httptestutil.New` y el `Register` real con los casos de uso armados como en el Step 5). Comparan las claves y los tipos del JSON con el contrato §3.1: `httptestutil.Decode[map[string]any]`, el conjunto exacto de claves, y `null` donde corresponde.
  - `GET {app}/domains`: `200 []` sin dominios, ordenados.
  - `POST`:
    - `201` con el dominio;
    - `proxied` ausente vale `false`;
    - errores:
      - campo desconocido (`"project"`) y `target_port` de tipo string: `400 invalid_json`;
      - `422 invalid` con el mensaje que empieza en `"hostname: "`, `"target_service: "` o `"target_port: "`;
      - hostname tomado: `409 conflict`;
      - app que no existe: `404 not_found`.
  - `PATCH {app}/domains/{hostname}`:
    - `{}` da `200`;
    - `{"hostname":"x"}` da `400 invalid_json`;
    - `proxied` sin registro: `422` con `"proxied: muelle no maneja el registro DNS de este dominio"`;
    - en una app de formulario, `{"target_service":"otro"}`: `422` con un mensaje que empieza en `"target_service: "`;
    - un host de otra app: `404 not_found`.
  - `DELETE`:
    - `204` sin cuerpo;
    - con Cloudflare caído: `502 cloudflare_unavailable`, y el dominio sigue en el `GET`.
  - `POST …/adopt`:
    - sin cuerpo, `200`; con `{}`, `200`; con `{"x":1}`, `400 invalid_json`;
    - sobre un `pending`: `409 nothing_to_adopt` con "No hay un registro que adoptar" (o el mensaje registrado).
  - `POST …/retry`: sin cuerpo y con `{}`, `200`; con `null` o `[]`, `400`.
  - Todas: un Bearer recibe `403 token_not_allowed`. Con Docker caído no cambia nada: los dominios no usan Docker.

  **Implementar:**
  - `body.go`: `func decodeEmptyBody(w, r) bool`. Lee con `http.MaxBytesReader` (`utils.MaxBodyBytes`); vacío (o solo espacios) es `true`; si no, rearma `r.Body` y llama a `utils.DecodeJSON(w, r, &struct{}{})`.
  - `domains.go`: los seis handlers. `handler.go` registra (todos `utils.AccessSession`):
  ```go
  const app = "/api/v1/projects/{p}/stages/{s}/apps/{a}"
  k.Handle("GET "+app+"/domains", utils.AccessSession, h.listDomains)
  k.Handle("POST "+app+"/domains", utils.AccessSession, h.createDomain)
  k.Handle("PATCH "+app+"/domains/{hostname}", utils.AccessSession, h.updateDomain)
  k.Handle("DELETE "+app+"/domains/{hostname}", utils.AccessSession, h.deleteDomain)
  k.Handle("POST "+app+"/domains/{hostname}/adopt", utils.AccessSession, h.adoptDomain)
  k.Handle("POST "+app+"/domains/{hostname}/retry", utils.AccessSession, h.retryDomain)
  ```
  - `errors.go` suma:
    - `k.RegisterError(service.ErrNothingToAdopt, 409, "nothing_to_adopt", "No hay un registro que adoptar")`;
    - `k.RegisterError(service.ErrHostnameTaken, 409, "conflict", "Ese hostname ya está en uso en muelle")`.
  - Al implementar: con las rutas registradas, `TestModuleRoutes` llamaba a un caso de uso nil (panic). Este commit ya arma los casos de uso en `module.go` (`DomainDeps` con `cloudflare.New(o.CloudflareURL)`) y cambia `TestModuleRoutes`: `…/domains` da `404 not_found` sin la app y `200 []` con ella. El Step 7 suma lo demás.

  **Commit:** `feat(ingress): los endpoints de dominios del contrato §3.2`

- [ ] **Step 7: Cableado.**

  **Tests primero:**
  - `module_test.go`:
    - `TestModuleRoutes`: `…/domains` ya no es `no_route`;
    - `TestModuleJobs`: `"proxy"` y `"certificados"`;
    - `TestModuleDomainsFlow`: `POST`, `GET`, `PATCH` y `DELETE` por HTTP con `cftest` (`Options.CloudflareURL`) y `caddytest`.
  - `container_test.go`, la línea de `TestNew`: `"sesiones vencidas, retención de auditoría, backup diario, proxy, certificados"`.

  **Implementar** `module.go`:
  - `New` exige `Audit`;
  - arma `cloudflare.New(o.CloudflareURL)`, `TLSProbe{Addr: o.CaddyTLSAddr, Roots: o.TLSRoots, Timeout: 5 s}`, `Ops`, `CertChecker` y los casos de uso;
  - `Run` suma `wg.Go(certs.Run)`.

  **Commit:** `feat(ingress): el módulo expone los dominios y verifica certificados`

- [ ] **Step 8: Integración — dominio por la API, ruta y certificado.**

  `TestIntegrationDomainsAndCertificates` (ranura 3):
  1. Con el módulo, el puente, el stage `s1`, la app `web`, su backend, sin token y `job()` → `synced`, un `POST {app}/domains` por `httptestutil` de `web.it3<id>.test` → `web:8080` da `201`, `pending` y `tls_mode http`.
  2. `eventually`: `get` da `200 "hola web"`. Prueba que crear aplica la ruta sin deploy.
  3. Las raíces de la CA interna salen de `GET /pki/ca/local` por el puente (`root_certificate`, PEM). Un `TLSProbe` con esas raíces y `Addr 127.0.0.1:<https>`:
     - `eventually`: `Valid`, `Issuer` empieza con "Caddy Local Authority" y `NotAfter` es futuro;
     - sin raíces, `Valid == false` con un `Problem`.
     La hoja interna dura 12 h: la regla de los 14 días ya está probada en el Step 1; acá se prueba la sonda contra Caddy real.
  4. `DELETE` del dominio da `204`, y `eventually`: `get` ya no da `200 "hola web"`, es decir, `status != 200 || body != "hola web"` (o un error de conexión).
     - No alcanza con "no da `200`": Caddy responde `200` vacío a un host que no tiene ruta, y el certificado de ese SNI puede seguir en su caché un momento después del `/load`. Así, un `200` vacío también cuenta como "sin la ruta".

  **Correr:** el comando de integración (los tres tests). Expected: PASS y nada `it3` sobrante.

  **Commit:** `test(ingress): integración de un dominio por la API, su ruta y la verificación del certificado`

- [ ] **Step 9: Cierre de T4.** Las verificaciones de "Convenciones" pasan, y la integración también.

---

### Task T5: Hooks de admin, el DNS del panel y el cierre

**Objetivo:**
- `CheckCloudflareToken` y `ProxySettingsChanged` reales (contrato §3.5): verificar el token, actualizar los registros cuando cambia la IP pública, recrear muelle-caddy después del `PATCH` y regenerar la configuración.
- El registro DNS del panel y `cloudflare_zones` en el job `"proxy"` (§3.4, §3.6).
- Los contratos de ingress con el contenedor completo, que nunca llegan a la API real (`WithIngress`, desde T2).
- La documentación queda al día.

**Files:**
- Create: `src/internal/ingress/app/usecases/{token,panel_dns,records_update,settings_changed}.go` + tests
- Modify: `src/internal/ingress/app/usecases/proxy_job.go` + test
- Modify: `src/internal/ingress/module.go`, `module_test.go`
- Create: `src/shared/infra/container/ingress_test.go`
- Modify: `README.md`, el contrato (§1.1, §1.2, §3 y §7 de ingress) y el diseño (§3.1, §3.2, §8 y §9), si hace falta

- [ ] **Step 1: `CheckCloudflareToken`.**

  **Tests primero** (`usecases/token_test.go`, con `cftest`):
  - Un token válido con 120 zonas (3 páginas) da `*zones == 120`.
  - Uno rechazado (ni verifica ni lista) da un `*validate.Error` con `Field == "cloudflare_token"`.
  - Uno de cuenta (`AddAccountToken`: no verifica, pero lista dos zonas) se acepta con `2`.
  - Uno `disabled` o `expired` (`SetTokenStatus`: `verify` da `200` con ese estado, y `/zones` da `401`) es `422` en `cloudflare_token`, con el estado en el mensaje (D31).
  - Uno válido sin zonas da `422` en `cloudflare_token`, con un mensaje que pide Zone:Read y DNS:Edit.
  - Uno válido al que `/zones` le da `403` es `422`.
  - Con `SetDown`, un `500` o un `429` en `/user/tokens/verify`: `errors.Is(err, entity.ErrCloudflareUnavailable)`.
  - Ningún mensaje contiene el token.

  **Implementar** `token.go`: `CheckCloudflareToken.Execute(ctx, token) (*int, error)`, con un plazo propio de 30 s sobre el `ctx` de la petición. La lógica:
  1. `VerifyToken`; si Cloudflare no está, `ErrCloudflareUnavailable`.
  2. `Zones`; lo mismo.
  3. Si el `verify` rechazó y `Zones` falló o dio 0, el token fue rechazado (`422`).
  4. Si `Zones` dio un `DNSError`, es `422` por los permisos.
  5. Con 0 zonas, `422`.
  6. Si no, `&n`.

  **Commit:** `feat(ingress): CheckCloudflareToken verifica el token y cuenta sus zonas (contrato §3.5)`

- [ ] **Step 2: El registro del panel y `cloudflare_zones`.**

  **Tests primero** (`usecases/panel_dns_test.go`). El `panel_domain` es `panel.tudominio.cl` y la zona `z1 tudominio.cl`:
  - `TestPanelDNSCreates`:
    - sin registros, crea el `A` (`proxied false`, `comment "muelle"`) y, con `public_ipv6`, el `AAAA`;
    - guarda `panel_cf_zone_id`, `panel_cf_record_id` y `panel_cf_record_id_aaaa`;
    - guarda `cloudflare_zones` con la cuenta;
    - devuelve `zoneChanged == true` la primera vez y `false` la segunda.
  - `TestPanelDNSForeign`: con un `A` ajeno, no crea nada y guarda la zona igual (decide el TLS del panel).
  - `TestPanelDNSForgets`:
    - un id guardado que ya no está en `cftest` se olvida y se crea uno nuevo;
    - un `A` guardado que ya no está, con el `AAAA` propio que sí está: el `AAAA` se borra, y se crean un `A` y un `AAAA` nuevos. El `AAAA` propio no cuenta como "otro registro de ese nombre" (D29);
    - uno de otro nombre (`viejo.tudominio.cl`, como si cambiara `MUELLE_PANEL_ORIGIN`) se olvida, **no se borra** (sigue en `cftest`), y se crea el del nombre nuevo.
  - `TestPanelDNSForgetsUnreadable` (D27), como después de cambiar el token a otra cuenta: `panel_cf_zone_id` es `zviejo` y `panel_cf_record_id` un id de esa zona; el token nuevo ve solo `z1`.
    - Con `FailNext("GET", "zviejo", 403, …)`: el id se olvida sin un `DELETE`, `panel_cf_zone_id` pasa a `z1` y se crea el `A` en `z1`. Lo mismo con un `400`.
    - Con `FailNext("GET", "zviejo", 503, …)` en esa lectura, no se olvida nada: `Execute` devuelve un error con `ErrCloudflareUnavailable`, y las claves quedan como estaban.
  - `TestPanelDNSNoPublicIP`: con token y zona, pero sin `public_ip`:
    - no hay `POST` ni `PATCH` en `cftest`;
    - un `A` propio queda con su contenido (como en §3.5: borrar `public_ip` no toca registros);
    - `panel_cf_zone_id` y `cloudflare_zones` se guardan igual.
  - `TestPanelDNSUpdatesContent`:
    - con otra `public_ip`, actualiza el contenido del `A` propio;
    - un `proxied: true` que el usuario puso en Cloudflare queda `true`.
  - `TestPanelDNSDropsAAAA`: sin `public_ipv6`, borra el `AAAA` propio y su clave.
  - `TestPanelDNSOutOfZone`: un `panel_domain` fuera de las zonas borra `panel_cf_zone_id`.
  - `TestPanelDNSNoToken`: no llama a `cftest`.
  - `TestPanelDNSReadsCurrent` (D30): con `public_ip` `203.0.113.99` guardado, `Execute` usa esa IP en el `A`.
  - `TestPanelDNSZonesCAS`: el token cambia mientras corre. Un `service.Settings` del test devuelve el token `t1` en `ProxySettings`, y delega lo demás (`PanelDNS`, `SavePanelDNS` y `SaveZones`) en el adaptador real, que tiene `t2` guardado. Entonces `cloudflare_zones` no se escribe.
  - `TestPanelDNSDeadline`: con `cftest.Hang(200ms)`, `Timing.DomainOp: 50ms` y un `ctx` sin plazo (como el del job), `Execute` vuelve con error en poco más de 50 ms, y libera el lock de `Ops` (un `Do` después no espera).

  **Implementar** `panel_dns.go`: `EnsurePanelDNS.Execute(ctx) (zoneChanged bool, err error)`, según D27, D28, D29 y D30.
  - Corre en `Ops.Do(ctx, Timing.DomainOp, …)` (D6): el `ctx` del job no tiene plazo, y sin eso podría tener el lock por minutos.
  - Dentro del lock, lee los ajustes actuales; sin token, vuelve sin llamar a Cloudflare.
  - Los ids guardados se validan con `Record(store.ZoneID, id)` antes de cambiar la zona. Un `ErrDNSNotFound` o un `*DNSError` olvidan el id (D27); un `ErrCloudflareUnavailable` vuelve con el error sin guardar nada.
  - Sin `public_ip`, no crea ni actualiza ningún registro; solo guarda la zona y `cloudflare_zones`.

  En `proxy_job.go`, después del `Sync` (D12): si hay token, `EnsurePanelDNS`; si `zoneChanged`, `Sync` otra vez. Su test `TestProxyJobCloudflare` cubre las dos cosas (dos `Load` cuando aparece la zona del panel; uno en la pasada siguiente).

  **Commit:** `feat(ingress): el registro DNS del panel y cloudflare_zones en el job proxy (contrato §3.4 y §3.6)`

- [ ] **Step 3: Los registros cuando cambia la IP pública.**

  **Tests primero** (`usecases/records_update_test.go`). Tres dominios: dos con registros propios (uno en `error` + `cloudflare`) y uno sin registros:
  - Un `public_ip` nuevo actualiza el `A` de los dos; el que estaba en error vuelve a `pending`; el que no tiene registros no se toca.
  - Si uno falla (`FailNext PATCH 400`), ese queda `error` + `cloudflare` y el otro se actualiza igual.
  - Un `A` propio que ya no está (404) limpia el id y deja `error` + `cloudflare` (así `retry` lo crea).
  - Lo mismo con un `AAAA` propio presente (D29): el `AAAA` se borra en `cftest`, **los dos ids** quedan vacíos, y el `error` + `cloudflare` se guarda (el `repo.Update` no choca con el `CHECK` de D19).
  - Borrar `public_ip` no hace ninguna llamada a `cftest`.
  - **Los ajustes de ese momento** (D30): el hook se llama con un `next` que trae `203.0.113.50`, pero la base ya tiene `203.0.113.60` (otro `PATCH` llegó entretanto). Los `A` quedan en `.60`.
  - **Plazos:** con `cftest.Hang(200ms)`, `Timing.DomainOp: 50ms` y dos dominios, cada uno queda `error` + `cloudflare`, y `Execute` vuelve en poco más de 100 ms. Con un `ctx` que vence en 60 ms (como el plazo del hook), el segundo dominio no espera otros 50 ms: su `opCtx` usa lo que queda del plazo del hook (D6).
  - Un `public_ipv6` nuevo crea el `AAAA` (con el `proxied` del dominio) de los que tienen `A` propio. Si ya hay un `AAAA` ajeno, `error` + `cloudflare`.
  - Borrar `public_ipv6` borra los `AAAA` propios y limpia los ids.
  - Sin token, los dominios con registros propios quedan `error` + `cloudflare` con el mensaje del token (D18).

  **Implementar** `records_update.go`: `UpdateManagedRecords.Execute(ctx, old, next shared.ProxySettings)`.
  - `old`/`next` solo deciden si hay algo que hacer: si no cambió `public_ip` ni `public_ipv6`, vuelve.
  - Lista los dominios con `cf_record_id`. Cada uno va en su propio `Ops.Do(ctx, Timing.DomainOp, …)`, así un hook largo no bloquea todo de una vez.
  - Dentro de cada `Do`, vuelve a leer el dominio (uno que ya no está se salta) y los ajustes actuales (D30), y lleva sus registros a esos ajustes:
    - el `A`, a `public_ip` si hay (sin `public_ip` no se toca);
    - el `AAAA`, a `public_ipv6` si hay (lo crea si falta); sin `public_ipv6`, se borra el propio;
    - si el `A` ya no está, D29;
    - sin token en los ajustes actuales, `error` + `cloudflare` con el mensaje del token, sin llamar a Cloudflare (D18).
  - Se guarda con `dbCtx`.

  **Commit:** `feat(ingress): los registros que maneja muelle siguen a la IP pública`

- [ ] **Step 4: `ProxySettingsChanged`.**

  **Antes de cablear:** `TestContractProxySettings` ya usa un `cftest` con su token y una zona, y `newTestModule` ya pasa `Options.CloudflareURL` (los dos desde T2 Step 5). Confírmalo con `git grep -n 'CloudflareURL\|WithIngress' -- '*_test.go'`. Si falta alguno, agrégalo en este mismo commit: si no, el `go test -race ./...` de antes del commit llamaría a la API real.

  **Tests primero** (`usecases/settings_changed_test.go`). Arman los casos de uso en el test: `ProxySettingsChanged` con su `Recreator`, `EnsureCaddy`, `Syncer`, `UpdateManagedRecords`, `EnsurePanelDNS` y `ProxyState`, sobre sqlite, `dockertest`, `caddytest`, `cftest` y el cliente de Cloudflare (como el fixture de T4 Step 5). No usan un `ingress.Module`: el paquete `ingress` importa `usecases`, y el test de `usecases` no puede importarlo de vuelta (ciclo). El cableado del módulo lo prueba `TestModuleProxySettingsChanged` (abajo).
  - Cambian los puertos: el estado queda `recreating` y no hay un `Load` nuevo antes de volver. Con `Run` corriendo (`RecreateDelay 20ms`), aparece un contenedor nuevo con los puertos nuevos y un `Load`.
  - **La recreación espera al hook** (D15, contrato §3.5): cambian los puertos y `public_ip` a la vez, con un token, el registro del panel en `cftest` y `cftest.Hang(100ms)`, más que `RecreateDelay 20ms`. `Run` corre, y `Execute` va en una goroutine.
    - Mientras `Execute` corre, `Snapshot` dice `recreating` y `Calls()` del fake no tiene `CreateContainer`.
    - Recién cuando `Execute` vuelve, `eventually`: aparece el contenedor nuevo, no antes de `RecreateDelay` después de la vuelta.
  - Cambia `caddy_image`: igual.
  - Cambia `acme_email`: un `Load` enseguida, con el email nuevo en la política.
  - Cambia `panel_allowed_cidrs`: un `Load` con el `403`.
  - Se quita el token: un `Load` sin la política `cloudflare`.
  - Un token nuevo: `EnsurePanelDNS` (`cftest` recibe el `POST` del registro del panel) y un `Load` con el panel en `cloudflare`.
  - Cambia `public_ip`: los registros de los dominios y el del panel se actualizan, y un `Load`.
  - El hook recibe un `ctx` ya cancelado y termina igual (`context.WithoutCancel`).
  - Con los puertos y el email en el mismo cambio, no hay un `Load` antes de la recreación (contrato §3.5).

  **Implementar** `settings_changed.go`. Corre con `Timing.Hook` y nunca devuelve error: los errores quedan en el log y en el estado.
  ```go
  // Execute aplica lo que cambió (next.ChangedKeys(old)), en este orden:
  //  1. puertos o imagen → release := recreator.Hold() y defer release(): GET
  //     /proxy dice recreating desde ya, y los 2 s de RecreateDelay empiezan
  //     recién cuando Execute vuelve (D15). No recrea acá;
  //  2. public_ip o public_ipv6 → UpdateManagedRecords (cada dominio con
  //     DomainOp, dentro del plazo del hook: D6);
  //  3. public_ip, public_ipv6 o un token nuevo (no vacío) → EnsurePanelDNS
  //     (lee los ajustes actuales: D30);
  //  4. si no hubo 1 y cambió public_ip, public_ipv6, acme_email, el token o
  //     panel_allowed_cidrs → syncer.Sync(ctx, false).
  func (u *ProxySettingsChanged) Execute(ctx context.Context, old, next shared.ProxySettings)
  ```
  En `module.go`, `CheckCloudflareToken` y `ProxySettingsChanged` llaman a estos casos de uso, y sus comentarios dejan de decir "esqueleto". `TestModuleStubs` desaparece: sus partes quedaron en tests propios.

  En `module_test.go`, `TestModuleProxySettingsChanged`: con `newTestModule`, un cambio de puertos deja `GET /api/v1/proxy` en `recreating`, y uno de `acme_email` manda un `Load`. Prueba el cableado; las reglas ya están en `settings_changed_test.go`.

  **Commit:** `feat(ingress): ProxySettingsChanged aplica los ajustes y agenda la recreación (contrato §3.5)`

- [ ] **Step 5: El contenedor completo, sin la API real de Cloudflare.**

  **Tests primero** (`src/shared/infra/container/ingress_test.go`, nuevo, `package container`):
  - `TestContractCloudflareTokenThroughSettings`:
    - con `New(…, WithDocker(dockertest.New()), WithIngress(ingress.Options{CloudflareURL: cf.URL()}))` y `cf` con el token `cf-token-1234567890abcdef` y dos zonas, el `PATCH /settings` del token da `200` con `cloudflare_zones: 2`;
    - un token que `cf` no conoce da `422` en `cloudflare_token`;
    - con `cf.SetDown(true)`, `502 cloudflare_unavailable`.
  - `TestContractPanelKeysHidden`:
    - con las claves `panel_cf_*` sembradas, `GET /settings` no tiene ninguna clave `panel_cf_*`;
    - `PATCH /settings` con `{"panel_cf_zone_id":"x"}` da `400 invalid_json`.
  - `TestContractDomainsWired`:
    - crear proyecto, stage y app de formulario por la API, y `POST …/domains` sin token, dan `201` con `tls_mode: http`;
    - borrar la app da `409 not_empty` (el contrato con catalog);
    - borrar el dominio y después la app da `204`.

  `WithIngress` y el `cftest` de `TestContractProxySettings` ya están desde T2 Step 5. Revisa otra vez con `git grep -n 'cloudflare_token\|CloudflareToken' -- 'src/shared/infra/container/*_test.go'` que ningún test del contenedor mande un token sin `WithIngress`.

  No hay código nuevo en el contenedor: si un test de estos falla, el arreglo va en ingress. Si de verdad hiciera falta tocar `container.go`, detente y repórtalo (`NEEDS_CONTEXT`).

  **Commit:** `test(container): los contratos de ingress con el contenedor completo, con Cloudflare falso`

- [ ] **Step 6: La documentación.**

  - **`README.md`:**
    - **"Estado":** una línea de la Fase 3: muelle-caddy, rutas, dominios con Cloudflare, certificados y `GET /proxy`.
    - **"Docker":** en desarrollo, sin dominio del panel o sin imagen de Caddy, muelle no crea muelle-caddy, y `GET /proxy` dice `disabled`.
    - **Una `caddy_image` propia** necesita Caddy ≥ 2.10 (antes, la admin API por socket unix rechaza el `Host` que manda muelle, caddy#6832) y el módulo `dns.providers.cloudflare` (contrato §3.4, D4).
    - **Imágenes privadas:** muelle baja las versiones nuevas de muelle-caddy con el registry `ghcr.io` de Ajustes → Registries, que siembra install. Si el token vence, `GET /proxy` lo dice, y se actualiza ahí (el README de la Fase 5 explica el token).
    - **Tests:** el comando de integración de ingress, que necesita un Swarm manager y las imágenes `caddy:2-alpine` y `alpine/socat:latest`. No se corre a la vez que la integración de la Fase 5 ("Al integrar").
    - **El bloque "Fase 3" de las notas:** tachar con `~~…~~` lo cumplido (`client_ip`, sin atajos, `CaddyCommand`/`CaddyEnv`, recrear y no rearrancar), y agregar lo que queda para el futuro:
      - la lista de rangos de Cloudflare se actualiza con cada versión (`caddy/ranges.go`);
      - la prueba manual de DNS-01 contra una zona de prueba;
      - el aislamiento entre stages por nombre lo prueba `TestIntegrationCaddyLifecycle` (T3); por IP (sin el nombre) no hay un test automático, y queda para la prueba manual;
      - verificar `Network.Subnets` de `muelle_panel` lo hace install.
  - **"Manual antes de migrar" (spec §15),** en el README como pasos:
    1. un token de una zona de prueba en Ajustes;
    2. crear un dominio, que queda `pending` → `active` en < 2 min (criterio §1.4);
    3. cambiar `proxied`;
    4. adoptar un registro existente;
    5. borrar el dominio y ver que el registro se va de Cloudflare.
  - **Contrato:**
    - Lo de las revisiones del plan ya está (`2249ccc`, y la versión mínima de Caddy, D27 y D32 de la segunda revisión): no lo repitas.
    - §1.1: "(hecho)" en las filas de ingress (`SyncRoutes`, `ProxyHook`, las lecturas de catalog y registries).
    - §1.2: solo el bloque **`ingress`** (Fase 3), si algo cambió. Nunca la lista de `deploy.Run`, que reescribe la Fase 2 ("Al integrar").
    - Si la implementación cambió algo visible para el panel, una fila en §7 y un aviso en el resumen de la tarea.
  - **Diseño:** §3.1, §3.2, §8 y §9, solo si algo quedó distinto de lo que dicen.

  **Commit** (separados): `docs: la Fase 3 en el README (estado, integración y notas)` y, si hubo cambios, `docs(contrato): la Fase 3 hecha en §1.1 …`.

- [ ] **Step 7: Cierre de la fase.**
  1. Las verificaciones de "Convenciones" pasan.
  2. `go test -tags integration -count=1 -run Integration -v ./src/internal/ingress/...` pasa (los tres tests).
  3. `docker ps -a --filter name=it3-`, `docker network ls --filter name=it3` y `docker volume ls --filter name=it3` no muestran nada.
  4. **Prueba de punta a punta con el binario** (manual, como en la Fase 1):
     - `go run . serve` con el `MUELLE_PANEL_ORIGIN` de desarrollo (una IP): `GET /api/v1/proxy` da `disabled`;
     - `POST …/domains` sin token da `201`, con `tls_mode: http` y `records`;
     - `PATCH /settings` con `{"cloudflare_token":"corto"}` da `422` en `cloudflare_token` ("no parece un token…"). admin rechaza el formato antes de llamar al hook, así que no sale nada a la red.
     - **Nunca** mandes un token bien formado (de 20 caracteres o más) al binario: iría a la API real de Cloudflare. La verificación del token ya la cubren los tests con `cftest`.
  5. El resumen para la revisión final lista:
     - los commits;
     - los cambios de contrato (con lo que el panel tiene que saber);
     - las preguntas abiertas al dueño.

---

## Preguntas para el dueño (el plan sigue la recomendación)

1. **Autoridad de certificados.** Con un emisor `acme` explícito, Caddy usa solo Let's Encrypt, sin el respaldo de ZeroSSL que trae por defecto (D16). *Recomendación:* solo Let's Encrypt en la v1. Es la CA conocida, con rate limits holgados para pocos dominios, y no necesita la cuenta EAB de ZeroSSL. Si se quiere el respaldo, es agregar un segundo emisor en `Render`.
2. **Dominios `http` con el proxy fuera de 80/443.** Durante la migración (§16, puertos 8080/8443), un dominio sin Cloudflare no puede emitir: HTTP-01 y TLS-ALPN-01 necesitan 80/443 hacia afuera. *Recomendación:* permitirlos igual. A los 10 min quedan en `error` + `certificate`, con un mensaje que dice que los puertos 80 y 443 tienen que llegar a muelle-caddy, y "Reintentar" funciona cuando se pasa a 80/443. Rechazarlos al crear obligaría a recrearlos después del cambio de puertos.
3. **El comentario `muelle` en los registros que crea muelle en Cloudflare** (D7, ya en el contrato §3.1). *Recomendación:* sí. Muestra en el panel de Cloudflare qué registros maneja muelle, no cuesta nada y no cambia el DNS.
