# muelle — Fase 2 (Deploy): Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Que muelle despliegue apps de formulario y compose por la API (y el panel): render del compose final con las restricciones de seguridad, cola con un worker por app, resolución de digests con las credenciales de los registries, `docker stack deploy`, espera de convergencia, rollback, log enmascarado y acotado, retención, reconciliación al arrancar, el estado y los logs en vivo de cada app, y el CRUD de registries. Todo según el contrato HTTP de las Fases 2 y 3.

**Architecture:** Spec §4.1 (hexagonal/DDD, módulos sin imports entre sí). La parte pura del compose (parsear con compose-go, la lista de permitidos de §6.3, los nombres de los servicios, el modelo de salida y su serialización) va en un paquete nuevo, `src/shared/domain/compose`, que usan catalog (al guardar) y deploy (al renderizar). El resto es del módulo `src/internal/deploy`: dominio puro (render, qué imagen se despliega, enmascarado, formato del log, regla de convergencia), repositorios SQLite (`deploys`, `registries`), casos de uso (encolar, correr, reconciliar, estado, logs), un despachador con un worker por app, y la API HTTP con SSE. Docker se usa solo por puertos propios de `deploy/domain/service`, implementados con el cliente compartido `src/shared/infra/out/docker`.

**Tech Stack:**
- Go 1.27, `net/http` y `encoding/json/v2` de la biblioteca estándar
- `modernc.org/sqlite` (sin CGO), con las migraciones embebidas (`0002_deploy.sql`, que se edita en el lugar)
- `github.com/compose-spec/compose-go/v2` **v2.16.1** (nueva: tiene `loader.WithSupportedAttributes`, la lista de permitidos que falla cerrada)
- `github.com/distribution/reference` v0.6.0 (ya está)
- El SDK de moby y el CLI `docker` por el cliente compartido (ya está)

**Spec:** [`docs/superpowers/specs/2026-10-09-muelle-design.md`](../specs/2026-10-09-muelle-design.md) (§5, §6, §7, §10, §12, §13, §15) y el contrato [`docs/superpowers/specs/2026-10-10-contrato-http-fases-2-3.md`](../specs/2026-10-10-contrato-http-fases-2-3.md) (§0, §1, §2, §6). Las referencias `§N` sin más son del diseño; `contrato §N`, del contrato. **El contrato manda**: cada endpoint, status, código de error y campo JSON de este plan sale de ahí, y si este plan y el contrato difieren, el contrato tiene razón (avisa en el reporte).

**Antes de este plan** (commit `docs(contrato): precisiones de la Fase 2…` en esta rama) se ajustaron el contrato y el diseño: la regla de "qué imagen se despliega" con la ref pedida (`requested_images`, contrato §2.2), el rollback (§2.7), el log (§2.5, §2.11), `since` y `follow` (§2.9), la reconciliación (§1.2), el bloqueo al borrar una app (§1.1), los servicios de una app en catalog (§2.12), la migración 0002 (§6) y §6.1–§6.3, §7 y §15 del diseño. Léelos antes de cada tarea.

---

## Etapas y paralelismo

| Tarea | Qué deja | Depende de |
|---|---|---|
| T1 | `shared/domain/compose` y la validación de catalog al guardar (más `services` en la respuesta de una app) | — |
| T2 | El dominio de deploy: entidades, las reglas de un registry (en `shared/domain/validate`), render con golden files, qué imagen se despliega, enmascarado, formato del log, regla de convergencia | T1 |
| T3 | La base y la API sin el worker: 0002, repositorios, registries (§2.10), encolar deploys y rollbacks, leerlos (§2.1–§2.5, §2.7), el ocultamiento de secretos en los tipos de Docker, y el registry inicial de install (`MUELLE_INIT_REGISTRY_FILE`, solo en el primer arranque de la base) | T2 |
| T4 | El worker: los pasos de §7, la cola por app, la reconciliación al arrancar, `RemoveApp`, y la integración con Swarm real | T3 |
| T5 | Los streams (log del deploy por SSE, logs de las apps con `since`), `GET …/status`, `Revalidate`, los contratos con el contenedor completo, el README y la prueba de punta a punta | T4 |

Las cinco tareas corren **en orden**, en la rama `fase-2-deploy` (worktree `/home/felipe/Escritorio/dev/muelle/.claude/worktrees/fase-2-deploy`). Cada una la hace un agente nuevo que solo lee este plan y el código, y cada una se revisa antes de empezar la siguiente. Cada tarea deja el árbol verde (gofmt, vet, `go test -race ./...`, `go mod tidy -diff`).

**En paralelo con otras líneas:** las Fases 3 (ingress, rama `fase-3-ingress`) y 5 (distribución, rama `fase-5-dist`) corren al mismo tiempo, cada una en su rama desde `fase-23-base`, y se mergean después. Por eso:
- solo se tocan los archivos que lista cada tarea;
- lo que haga falta en un paquete compartido va en un **archivo nuevo** con el nombre de la línea (`redact_deploy.go`, `fake_deploy.go`, `revalidate_deploy.go`, `registry_deploy.go`). Nunca se editan `docker.go`, `client.go`, `fake.go`, `kit.go`, `server.go` ni `container.go` (salvo lo que una tarea diga explícitamente);
- **`container.go`:** solo T3 lo edita, en dos lugares (la llamada a `deploy.New` y una llamada después de `ApplyStartupSettings`, T3 Step 8). La Fase 5 cambia la firma de `build` (suma `panelHandler`) y la `Config` del servidor: esas líneas no se tocan;
- **`src/settings`:** solo lo que dice T3 Step 8 (`MUELLE_INIT_REGISTRY_FILE`);
- **no se tocan**: `src/internal/ingress/**`, `web/`, `0003_ingress.sql`, los Dockerfiles, `action/`, `main.go`, los subcomandos `install` y `settings` de `main` (el paquete `src/settings` es otra cosa: ver arriba), ni el workflow de releases;
- en el README, el contrato y el diseño, solo las secciones de deploy (T5 dice cuáles).

## Convenciones (todas las tareas)

- **Dónde:** todos los comandos con `cd /home/felipe/Escritorio/dev/muelle/.claude/worktrees/fase-2-deploy` antes, o con `git -C <ese path>`. Nunca `git switch`/`checkout`, nunca otro worktree, nunca `git push`.
- **Shell:** la del usuario es fish. Lo que use sintaxis de bash va en `bash -c '…'`.
- **Go:** 1.27 (`go.mod`; `GOTOOLCHAIN=auto` baja el toolchain si la máquina tiene 1.26). Sin CGO.
- **Idioma:** comentarios, mensajes de error y commits en español con los acentos reales; identificadores en inglés.
- **Errores:**
  - Los comunes están en `shared/domain/repository` (`ErrNotFound` → 404 `not_found`, `ErrConflict` → 409 `conflict`) y `shared/domain/entity` (`ErrDockerUnavailable` → 503 `docker_unavailable`, `ErrNotImplemented` → 501). Ya están registrados en `cmd/http`.
  - La validación devuelve `*validate.Error` (422 `invalid`, mensaje `"<campo>: …"`).
  - Los del módulo van en `deploy/domain/service/errors.go` y se registran en `deploy/infra/in/http/mapper_errors.go` (`RegisterErrors(k)`), igual que en ingress.
- **Tiempo:** SQLite guarda milisegundos Unix (`sqlite.Millis`/`sqlite.Time`). El reloj es `package/clock` (`clock.NewManual` en los tests). Las horas que vienen de Docker tienen nanosegundos: en la API van **truncadas** al milisegundo (`t.UTC().Truncate(time.Millisecond)`), §10 del diseño.
- **Capas:** `domain` no importa `infra`, `database/sql` ni `net/http`, ni `shared/infra/out/docker`: los puertos de deploy usan sus propios tipos (`SwarmService`, `SwarmTask`, `LogLine`, `Credential`…) y el adaptador de `infra/out/service` los traduce desde `docker.*`. Sí puede importar `shared/domain/*` (también `compose`).
- **Tests:**
  - en el mismo paquete que el código, con `go test -race`;
  - SQLite real (`sqlitetest.New(t)`), sin mocks de la base;
  - Docker con el fake `dockertest` (y su `OnDeployStack` para simular lo que hace Swarm); nunca el Docker real fuera de los tests con tag `integration`;
  - los otros módulos, con fakes de sus puertos (`fakeEnv`, `fakeIngress`, como en `module_test.go`);
  - HTTP con `httptestutil` (`FakeAuth`); los streams, con `httptest.NewServer` sobre `httpserver.NewServer` (lo arma un helper del test de deploy).
- **TDD:** test → verificar que falla (y por qué) → implementar → verificar que pasa → commit.
- **Verificación antes de cada commit** (las cuatro):
  ```bash
  bash -c 'test -z "$(gofmt -l $(git ls-files "*.go"))"'   # nunca `gofmt -l .`: entra en .claude/worktrees
  go vet ./...
  go test -race ./...
  go mod tidy -diff
  ```
  Un archivo nuevo todavía no está en `git ls-files`: `git add` antes de correr gofmt, o pásalo a mano.
- **Commits:** chicos, en español, con el trailer como segundo `-m`:
  `git commit -m "feat(deploy): …" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"`.
- **Documentación de librerías** (compose-go, el SDK de moby, distribution/reference): `go doc` o Context7 (`resolve-library-id` y `query-docs`). No adivines una API. Las de compose-go que usa este plan se verificaron contra el código de v2.16.1 (`$(go env GOMODCACHE)/github.com/compose-spec/compose-go/v2@v2.16.1`).
- **Docker real (tests `//go:build integration`):**
  - todo lo que crean se llama `it2…` (proyecto `it2` + 4 hex al azar; sus stacks, servicios, redes y volúmenes empiezan con eso) y lleva la etiqueta `muelle.test=it2` donde se pueda;
  - se borra al terminar, también si el test falla (`t.Cleanup`), y nunca se toca algo sin ese prefijo. Un `-timeout` que vence no corre los `t.Cleanup`: lo barre el `TestMain` de la corrida siguiente, y hay un comando para hacerlo a mano (T4 Step 6);
  - nunca publican puertos del host. Si un test o la prueba de punta a punta necesita escuchar en el host, usa `127.0.0.1` en el rango **20000–20999**. Nunca 80, 443 ni 2019;
  - nunca usan los nombres fijos (`muelle`, `muelle_muelle`, `muelle-caddy`, `muelle_panel`, `muelle_master_key`, `muelle_data`…);
  - nunca `docker swarm init`/`leave`;
  - usan imágenes que ya están en el nodo (`nginx:alpine`, `alpine/socat:latest`): si faltan, el test se salta (`t.Skip`), no las baja;
  - solo se corren los de esta línea: `./src/shared/domain/compose/` y `./src/internal/deploy/...`. **Nunca** `-tags integration ./...` ni los de `shared/infra/out/docker` (los `ctr23` corren al integrar).
- **Alcance:** no modifiques archivos que la tarea no lista. Si algo compartido no alcanza, detente y repórtalo (`NEEDS_CONTEXT`) en vez de cambiarlo.

## Mapa de archivos

```
go.mod / go.sum                                   (T1) + compose-go v2.16.1
src/shared/domain/compose/                        (T1) paquete nuevo
├── compose.go        Validate, Load, Services, límites; carga en memoria con compose-go
├── allowlist.go      las rutas permitidas (tree.Path) y los motivos de cada rechazo (§6.3)
├── check.go          las reglas por valor sobre el modelo canónico (tipos de volumen, driver de logging, etiquetas, nombres)
├── interpolate.go    la sustitución estricta (${VAR} sin definir es error)
├── model.go          File, Service… (el compose final, con tags json)
├── convert.go        types.Project → File, solo con las claves conocidas
├── marshal.go        Marshal: JSON indentado, "$" → "$$" en los valores
└── *_test.go, integration_test.go (docker stack config), testdata/
src/internal/catalog/                             (T1) solo la validación del compose y services
├── domain/entity/app.go            ValidateCompose → compose.Validate
├── app/usecases/app.go             llama a checkServiceNames al crear y editar
├── app/usecases/service_names.go   (nuevo) nombres de servicio únicos por stage
└── app/dto/dto.go                  AppOutput.Services
src/shared/domain/validate/registry_deploy.go            (T2) RegistryHost, RegistryUsername, RegistryPassword (contrato §2.10)
src/shared/infra/out/sqlite/migrations/0002_deploy.sql   (T3) + requested_images, log_truncated
src/shared/infra/out/sqlite/migrations_test.go           (T3) sus defaults y CHECK
src/shared/infra/out/docker/redact_deploy.go             (T3) String/GoString/LogValue/MarshalJSON
src/settings/settings.go, settings_test.go               (T3) MUELLE_INIT_REGISTRY_FILE → Init.Registry (oculta la contraseña)
src/shared/infra/container/container.go                  (T3) solo deploy.New (InitRegistry) y la llamada a SeedInitRegistry
src/shared/infra/out/docker/dockertest/fake_deploy.go    (T5) PutContainer (health en los tests)
src/cmd/http/utils/revalidate_deploy.go                  (T5) Kit.Revalidate (contrato §0.3)
src/shared/infra/container/deploy_test.go                (T3–T5) el registry inicial (T3) y los contratos con el contenedor completo (T5)
src/internal/deploy/
├── module.go                                     (T3–T5) Deps (+ InitRegistry), New, RegisterRoutes, SeedInitRegistry, Run, RemoveApp
├── domain/entity/deploy.go, registry.go          (T2) Deploy, Status, Trigger; Registry (las reglas están en shared/domain/validate)
├── domain/repository/repository.go               (T3) DeployRepository, RegistryRepository
├── domain/service/
│   ├── ports.go          (existente; T3–T5 suman) Queue, Images, Cluster, ManagerNode; Stacks.Deploy
│   ├── errors.go         (T2–T5)
│   ├── formspec.go       (T2) el spec de formulario como lo lee deploy
│   ├── config.go         (T2) ReadConfig: servicios, refs de la configuración y modelo base
│   ├── render.go         (T2) Render: inyecciones de §6.2, destinos de dominio, entorno
│   ├── images.go         (T2) ChooseImages, ResolvedRef, ImageHost, PinnedDigest
│   ├── masker.go         (T2) Masker (contrato §2.5)
│   ├── deploylog.go      (T2) FormatLine, Sanitize, AppendPlan, JoinPieces, CutLine, ErrorLine
│   ├── convergence.go    (T2) Evaluate, AppliedState
│   └── testdata/render/*.input.json, *.golden.json
├── app/dto/                                      (T3–T5) (T3: InitRegistry, que oculta la contraseña)
├── app/usecases/
│   ├── remove_app.go     (existente; T4) bloquea, detiene el worker, marca failed, stack rm
│   ├── create_deploy.go, rollback.go, queries.go, registries.go   (T3)
│   ├── seed_registry.go                                           (T3) el registry inicial de install, una sola vez
│   ├── logwriter.go, hub.go                                       (T3) el log persistido y el aviso a los streams
│   ├── run_deploy.go, resume.go, dispatcher.go, manager_node.go   (T4)
│   └── status.go, app_logs.go                                     (T5)
├── infra/in/http/        (T3–T5) handler.go, deploys.go, registries.go, sse.go, streams.go, status.go, mapper_errors.go, res.go
├── infra/in/form/        (T3) deploy.go, registry.go (los cuerpos)
├── infra/out/repository/sqlite/deploys.go, registries.go    (T3)
├── infra/out/service/    apps.go, stacks.go (existentes); settings.go (T3: la marca init_registry_applied; T4: manager_node_id); docker.go (T4–T5)
├── integration_test.go   (T4–T5) //go:build integration, prefijo it2
└── integration_main_test.go (T4) TestMain: barre lo it2 de corridas anteriores
docs: README (T5, solo las notas de deploy)
```

## Decisiones (con su porqué)

Lo que el plan decide y la tarea no vuelve a discutir:

1. **La ref pedida, no quitar el digest** (contrato §2.2). Cada deploy guarda `requested_images` (la ref antes de resolver) y un deploy sin cuerpo repite la del deploy actual. Así nunca se pierde un digest fijado a propósito (override, configuración, rollback), y la afirmación "un rollback no se deshace con el próximo deploy sin cuerpo" es cierta y tiene su test (T2 y T4). La consecuencia, documentada: después de un rollback, para volver a seguir un tag que se mueve hace falta un override o cambiar la configuración (pregunta 1 al dueño).
   **Una ref con digest también se consulta al registry** (contrato §2.2: "solo verifica que exista y que haya acceso, y lo conserva"; §7 paso 2: si falla, Swarm no se toca). `Images.Digest` se llama para **todas** las refs, con `ResolveTimeout`; Docker busca una ref con digest por ese digest (`GET /distribution/{ref}/json`), así que verifica que el manifiesto exista y que la credencial llegue. Sin eso, un rollback a un digest que el registry ya borró llegaría a Swarm, y en un servicio con volumen (`stop-first`, §6.2) Swarm pararía la tarea vieja antes de descubrir que no puede bajar la nueva: caída hasta el rollback automático.
2. **compose-go v2.16.1 sin leer archivos.** El compose va en `ConfigFile.Content`, con `SkipInclude`, `SkipExtends`, `SkipResolveEnvironment`, `SkipResolveLabels`, `SkipNormalization`, `SkipDefaultValues`, `ResolvePaths: false` y un `WorkingDir` que no existe: compose-go nunca abre `include`, `extends.file`, `env_file` ni `label_file`, y esas claves quedan en el modelo para que la lista de permitidos las rechace. La lista de permitidos es doble: `loader.WithSupportedAttributes` sobre el modelo canónico (falla cerrada ante claves nuevas de la especificación) y un conversor propio que copia solo los campos conocidos al modelo de salida.
3. **Al guardar no se interpola** (catalog todavía no tiene las variables): se valida el modelo con `LoadModelWithContext` y `SkipInterpolation`, y las reglas por valor saltan los strings con `${`. El render repite todo ya interpolado y con el modelo tipado (`ModelToProject`).
4. **El compose final va en JSON** indentado con claves ordenadas: es YAML 1.2, `docker stack deploy` lo lee (verificado con `docker stack config -c -` en Docker 29.6.1, sin `version`), no tiene ambigüedades de tipos de YAML y el golden es estable. `$` → `$$` en todos los valores (no en las claves) al serializar.
5. **`dns_opt` se rechaza y `platform` se descarta:** el esquema del CLI de `docker stack deploy` no tiene ninguna de las dos (probado). Los jobs (`deploy.mode: *-job`) se rechazan: la v1 no tiene comandos de release.
6. **`muelle.deploy` solo en las etiquetas del servicio**, no en las de los contenedores: si no, cada deploy cambiaría la plantilla de las tareas y Swarm reiniciaría todos los servicios de la app (un `redis` con volumen se cortaría al desplegar el `api`). Para la convergencia de un servicio que ya existía y no cambió, se espera un margen de 5 s (`Timing.Settle`) sin `UpdateStatus` nuevo (§7 paso 6). El margen se cuenta desde que `docker stack deploy` **volvió**, no desde antes de llamarlo: con muchos servicios o un daemon lento el CLI puede tardar más de 5 s, y un servicio que sí cambió, cuyo `UpdateStatus` Swarm todavía no puso, se daría por convergido antes de que empiece el rollout. Pregunta 2 al dueño.
7. **Reconciliación por etiquetas** (contrato §1.2): un `running` que quedó del arranque anterior se resuelve en el worker de su app, mirando qué servicios llevan su `muelle.deploy`. Con Docker caído queda `running` y se reintenta cada 15 s; el apagado nunca cierra un deploy.
8. **Borrar una app bloquea sus deploys 2 min** (`409 conflict`), cancela el que corre (`failed`) y marca `failed` los `queued`, antes del `docker stack rm`. Si el `stack rm` falla, desbloquea enseguida. Sin el bloqueo, un deploy encolado entre el `stack rm` y el `DELETE` de la fila dejaría un stack huérfano.
   **El bloqueo lo cumple el despachador, no solo `CreateDeploy`:** `CreateDeploy` y `Rollback` miran `Accepting` antes de su `InTx`, así que un `POST` que pasó esa mirada puede insertar su `queued` después del `Block` (y después de `FailQueued`). Por eso, mientras una app está bloqueada: `Wake` no arranca un worker (solo marca `failed` sus `queued`, con "la app se borró"), y un worker nunca reclama (`Claim`) ni retoma: marca `failed` los `queued` y termina. Además, `RunDeploy` vuelve a mirar `Accepting(app.ID)` justo antes del paso 5 y, si es `false`, cierra `failed` sin tocar Swarm.
9. **El log acotado** (contrato §2.5): una columna `log_truncated` (0002) guarda la marca, así sigue valiendo tras un reinicio. Las líneas se formatean y enmascaran en Go, y el repositorio aplica el tope en una transacción. El log se lee por bytes con `CAST(log AS BLOB)` (SQLite cuenta caracteres en un `TEXT`).
10. **Enmascarar las líneas partidas** (contrato §2.5): el cliente de Docker entrega en pedazos una línea de más de 16 KiB, y los pedazos que siguen vienen sin hora ni tarea; se unen (hasta 64 KiB) antes de enmascarar. El corte que hace el propio Docker en su `json-file` no se detecta (limitación aceptada, documentada en el contrato).
11. **SSE y `ReadTimeout`:** `newHTTPServer` (main.go) tiene `ReadTimeout: 30s`, y net/http pone ese plazo en la lectura de fondo de la conexión: al vencer, **cancela el contexto de la petición**. Cada handler SSE llama primero a `http.NewResponseController(w).SetReadDeadline(time.Time{})`. Un test con un `http.Server` de `ReadTimeout` corto lo prueba (T5).
12. **El entorno de un servicio cabe en `execve`:** cada `CLAVE=valor` ≤ 128 KiB y el total (+8 bytes por variable) ≤ 1,5 MiB; si no, error de render (el contenedor no arrancaría con `E2BIG`). secrets permite 1 MiB por dueño, así que stage + app pueden pasarse.
13. **Plazos por paso:** 60 s por imagen al resolver, 30 s para la red, 2 min para `docker stack deploy`, `deploy_timeout_s` para la convergencia (poll de 2 s), 30 s para `SyncRoutes`. Todos en `usecases.Timing`, que los tests acortan.
14. **`SyncRoutes`** devuelve `ErrNotImplemented` en esta rama hasta que se mergee la Fase 3: cualquier error es una línea `advertencia: no se pudo actualizar Caddy: …` y el deploy sigue (contrato §2.11).
15. **Los servicios de una app en catalog** (contrato §2.12, opcional y barato): `services: [{name, ports}]` con los puertos de `spec.port` o de `expose`. Sale de `compose.Services`, que lee el compose guardado sin validarlo.
16. **Las notas de la etapa de contratos** (`notes_phase2.md` y `contract_concerns.md` del scratchpad) ya no existían al escribir este plan. Lo que pedían está en el README ("Notas para las fases siguientes", Fase 2), en el texto de la tarea (los pendientes (a), el ocultamiento en `RegistryAuth`/`StackDeploy`, y (b), la regla de §2.2) y en el contrato; este plan los cubre todos.
17. **La sustitución estricta es propia, no envuelve `template.Substitute`** (T1). compose-go resuelve sin error dos casos que §5.2 regla 3 prohíbe: las variables dentro de un valor por defecto (`${A:-${B}}` llama al `Substitute` no estricto) y las que siguen en el mismo string a una con modificador (`${A:-x}-${B}`: el resto del string también va al no estricto). En los dos, una `B` sin definir sale `""` en silencio. `strictSubstitute` es un escáner propio, chico y con tests, que compose-go usa por `Interpolate.Substitute`; un `$` dentro de un valor por defecto es un error (al guardar y al renderizar).
18. **Los errores del worker nunca van crudos a `slog`** (§12). `RunDeploy.Execute` y `Resume` devuelven errores que pueden traer valores: compose-go pone el valor interpolado en sus errores de conversión (`parsing "<secreto>": invalid syntax`), `RenderError.Msg` puede llevar valores de variables, y la salida del CLI va sin enmascarar. El despachador registra solo la app, el id del deploy y el estado final, o el `error` ya guardado (`ErrorLine`, enmascarado); nunca `err.Error()` ni la salida del CLI. Lo mismo para los errores de la base dentro del worker (pueden envolver un error de render).
19. **El registry inicial de install se aplica solo en el primer arranque de la base** (cambio de requisito del dueño, 2026-10-15: las imágenes de muelle son privadas; contrato §2.10 y §5.2). Install deja montado el secreto `muelle_init_registry` y la variable `MUELLE_INIT_REGISTRY_FILE` en el servicio, igual que el del token de Cloudflare. Con una regla "si no hay un registry para ese host", un admin que borra en el panel la credencial de `ghcr.io` (porque se filtró) la vería volver con el próximo `docker service update` o reinicio del nodo. Por eso deploy guarda su propia marca, `init_registry_applied` en `settings`, en la misma transacción que crea el registry, y **siempre** la inserta en el primer arranque, haya o no archivo: una reinstalación sobre el mismo `muelle_data` no cambia las credenciales (como los demás `MUELLE_INIT_*`). No usa `init_applied` de admin: cuando deploy corre, admin ya la guardó en ese mismo arranque. Las reglas del host, el usuario y la contraseña pasan a `shared/domain/validate` (T2) para que `settings.Load` valide el archivo en cada arranque, como el del token, sin importar el dominio de un módulo.

---

### Task T1: `shared/domain/compose` y la validación de catalog al guardar

Un paquete puro que parsea un compose con compose-go, aplica la lista de permitidos y los rechazos de §6.3, interpola de forma estricta, convierte al modelo de salida y lo serializa. Catalog lo usa al guardar (con los nombres de servicio únicos por stage) y para mostrar los servicios de cada app.

**Files:**
- Modify: `go.mod`, `go.sum`
- Create: `src/shared/domain/compose/{compose,allowlist,check,interpolate,model,convert,marshal}.go`
- Create: `src/shared/domain/compose/{compose,reject,interpolate,marshal,services}_test.go`, `integration_test.go`, `testdata/kitchen-sink.yaml`
- Modify: `src/internal/catalog/domain/entity/app.go`, `entity_test.go`
- Create: `src/internal/catalog/app/usecases/service_names.go`
- Modify: `src/internal/catalog/app/usecases/app.go`, `usecases_test.go`, `src/internal/catalog/app/dto/dto.go`, `src/internal/catalog/module_test.go`, y el fixture de `src/internal/catalog/infra/out/repository/sqlite/repository_test.go` (`NewComposeApp` con `services: {}`, que ya no es un compose válido)

- [x] **Step 1: La dependencia**

```bash
go get github.com/compose-spec/compose-go/v2@v2.16.1
```

Expected: `go.mod` con `github.com/compose-spec/compose-go/v2 v2.16.1`. `go mod tidy` recién cuando haya código que la importe (si no, la quita).

- [x] **Step 2: La API del paquete** (`compose.go`, sin implementar todavía, para que compilen los tests):

```go
// Package compose es la parte pura del compose de una app (§6 del diseño):
// parsear con compose-go sin leer archivos, la lista de permitidos y los
// rechazos de §6.3, la interpolación estricta con las variables de la app, y
// el modelo y la serialización del compose final. Lo usan catalog (al
// guardar) y deploy (al renderizar), sin importarse.
package compose

const (
	// MaxBytes es el tamaño máximo de un compose (el mismo de catalog).
	MaxBytes = 256 << 10
	// MaxServices es el máximo de servicios de una app compose (§6.3).
	MaxServices = 32
	// Field es el campo de los errores de validación (contrato §0.2).
	Field = "compose_yaml"
)

// Validate valida un compose como lo guarda catalog: sin interpolar (todavía
// no hay variables), con la lista de permitidos, los rechazos de §6.3 y los
// nombres. Devuelve los nombres de sus servicios, ordenados. Los errores son
// *validate.Error en Field.
func Validate(src string) ([]string, error)

// Load parsea src, lo interpola solo con env (nunca con el entorno del
// proceso) y lo valida entero (§6.3), ya con los valores. project es el
// nombre del stack (naming.Stack), que compose-go exige. Devuelve el modelo
// de salida sin las inyecciones de §6.2, que hace deploy. env no se modifica.
func Load(ctx context.Context, src, project string, env map[string]string) (*File, error)

// ServiceInfo es un servicio como lo muestra catalog (contrato §2.12).
type ServiceInfo struct {
	Name  string
	Ports []int // de expose, ordenados; nunca nil
}

// Services lee los servicios de un compose ya guardado, sin validarlo ni
// interpolarlo. nil si no se puede leer.
func Services(src string) []ServiceInfo

// Marshal arma el compose final: JSON indentado con las claves ordenadas, y
// todo "$" de un valor (no de una clave) como "$$", porque docker stack
// deploy vuelve a interpolar (§6.3).
func Marshal(f *File) ([]byte, error)
```

- [x] **Step 3: Tests de `compose_test.go`** (escribir primero; todos con `errors.AsType[*validate.Error]` y `ve.Field == "compose_yaml"` cuando esperan un error):
  - `TestValidateKitchenSink`: `testdata/kitchen-sink.yaml` usa **todas** las claves permitidas de §6.3, en sus dos sintaxis cuando hay dos: `volumes` corto (`data:/data`) y largo (con `read_only` y `volume.nocopy`), `tmpfs` largo (`tmpfs.size`, `tmpfs.mode`) y como lista, `environment` como mapa y como lista (`- A=1`, `- B`), `labels` como mapa y como lista, `command`/`entrypoint` como string y como lista, `extra_hosts` (lista y mapa), `ulimits` (entero y `{soft, hard}`), `healthcheck` completo, `deploy` completo (`mode`, `replicas`, `placement` con `constraints`, `preferences` y `max_replicas_per_node`, `resources` con `cpus`/`memory` en `limits` y `reservations` (`limits.cpus: "0.5"` y `reservations.cpus: 0.1`), `restart_policy`, `update_config`, `rollback_config`, `endpoint_mode`, `labels`), `logging` (`json-file` con `options`), `x-common` con ancla y `<<: *common`, más `depends_on`, `restart`, `platform` y `version: "3.8"`. `Validate` no da error y devuelve los nombres ordenados.
  - `TestValidateInterpolationNotNeeded`: `image: ghcr.io/x/api:${TAG}`, `replicas: ${N:-1}` y `logging.driver: ${D}` pasan al guardar (todavía no hay variables). Lo que es error de sintaxis sin importar los valores sí falla al guardar (decisión 17): `${A:-${B}}` y `${A:-$B}` (un `$` en un valor por defecto), `${}` y `${A B}` → `compose_yaml: …` con la ruta.
  - `TestValidateSizeAndCount`: `""` y `"  \n"` → `compose_yaml: es obligatorio`; `MaxBytes+1` bytes → `compose_yaml: no puede superar 256 KiB`; `services: {}` → al menos un servicio; 33 servicios → a lo más 32; YAML roto → `compose_yaml: el compose no es válido: …`.
  - `TestValidateNames`: un servicio `Web`, `a_b` o de 17 caracteres, y un volumen `Data`, dan error y el mensaje nombra el servicio o volumen.
  - `TestValidateNeverReadsFiles`: crea un archivo en `t.TempDir()` con el contenido `MARCADOR-QUE-NO-DEBE-LEERSE: 1`, y un compose para cada clave que lee archivos (`include: [<path>]`, `extends: {file: <path>, service: x}`, `env_file: <path>`, `label_file: <path>`, `secrets: {s: {file: <path>}}` con el `secrets:` del servicio, `configs` igual). Cada uno da error, con el motivo de §6.3, y **ningún** mensaje contiene el marcador. Además, uno con `include` de un archivo que no existe da el error de la lista de permitidos, no "no such file".
  - `TestLoadUsesOnlyGivenEnv`: con `t.Setenv("MUELLE_TEST_SECRETO", "x")`, un compose con `${MUELLE_TEST_SECRETO}` y `env` vacío da el error de variable sin definir; `${COMPOSE_PROJECT_NAME}` vale el `project` (con `env` vacío y con `env` nil: la copia lo lleva siempre, Step 9); y después de `Load`, `env` no cambió (compose-go le agrega `COMPOSE_PROJECT_NAME` al mapa que recibe: hay que pasarle una copia). Un `env` que trae su propio `COMPOSE_PROJECT_NAME` no gana: vale el `project`.
  - `TestLoadModel`: convierte el kitchen sink con un `env` y verifica el `File`: `environment` como mapa (una variable sin valor, `- B` o `B:`, toma la de `env` como en compose con la shell, y si `env` no la tiene no va: `docker stack deploy` la tomaría del entorno de muelle; `TestLoadEnvironmentWithoutValue`), `command` como lista, montajes largos, etiquetas como mapa, duraciones como `"10s"`, memoria en bytes como string decimal (`"536870912"`), `cpus` como string (`"0.5"` y `"0.1"`: compose-go los lee como `float32`, y `0.1` en 64 bits saldría `"0.10000000149011612"`), `ulimits` con `soft`/`hard`, `extra_hosts` como `"host:ip"` ordenados, y sin `depends_on`, `restart`, `platform` ni `x-*`.
  - `TestLoadChecksInterpolatedValues`: `logging.driver: ${D}` con `D=syslog` y `image: ${IMG}` con `IMG="Mala Imagen"` dan error en `Load` (y pasan en `Validate`).

- [x] **Step 4: `reject_test.go`**, un caso por cada rechazo de §6.3 (tabla; cada fila: el compose mínimo `services: {web: {image: nginx, …}}` con la clave, y un pedazo del motivo esperado). Como mínimo:
  - publicar o construir: `ports`, `build`;
  - montar el host: `volumes: ["/var/run/docker.sock:/var/run/docker.sock"]`, `["./x:/x"]`, `type: bind`, `type: npipe`, `type: cluster`, `type: image`, un volumen anónimo (`type: volume` sin `source`), un `source` que no está en los `volumes` de primer nivel, `volume.subpath`; de primer nivel: `driver`, `driver_opts` (`{type: none, o: bind, device: /var/run}`), `external`, `name`; `devices`, `device_cgroup_rules`, `volumes_from`;
  - privilegios: `privileged`, `cap_add`, `security_opt`, `sysctls`, `pid`, `ipc`, `uts`, `userns_mode`, `cgroup`, `cgroup_parent`, `runtime`, `isolation`, `credential_spec`;
  - archivos de muelle: `secrets` y `configs` (de primer nivel y de servicio), `env_file`, `label_file`, `extends`, `include`;
  - red y nombres: `container_name`, `network_mode`, `links`, `external_links`, `networks` (de primer nivel y de servicio), `name` de primer nivel;
  - logging: `driver: syslog`, `gelf`, `fluentd`;
  - etiquetas: `muelle.app` en `labels`, en `deploy.labels` y en las de un volumen; `com.docker.stack.namespace` igual. Cada una como mapa (`muelle.app: x`), como lista con valor (`- muelle.app=x`) y como lista **sin `=`** (`- muelle.app`, `- com.docker.stack.namespace`): compose-go lee esa última como la clave con valor `""` (`Labels.DecodeMapstructure`), así que también es una etiqueta reservada;
  - lo que el CLI no soporta o la v1 no tiene: `dns_opt`, `deploy.mode: replicated-job` y `global-job`, `deploy.resources.limits.pids`, `deploy.resources.reservations.devices`, `use_api_socket`, `models`, `gpus`, `develop`, `pre_start`, `jobs` (de primer nivel);
  - una clave inventada: `foo` en el servicio, en el primer nivel y en `deploy` (`deploy.foo`). Estas no llegan a la lista de permitidos: las rechaza antes el esquema de compose-go (Step 9, `schemaMessage`), y el mensaje esperado es el genérico de `allowlist.go` con su ruta: `compose_yaml: services.web.foo: no está soportado…`, `compose_yaml: foo: …` y `compose_yaml: services.web.deploy.foo: …`. Todas las demás claves de esta lista existen en el esquema de v2.16.1 (verificado), así que sí llegan a la lista de permitidos;
  - `x-cualquier` en el servicio y en el primer nivel **sí** pasa.

  Lo que el esquema o `validation.Validate` de compose-go rechazan por la **combinación** de claves (por ejemplo `external` y `driver` juntos en un volumen: `volumes.data: conflicting parameters "external" and "driver" specified`) sale con el texto de compose-go detrás de `compose_yaml: el compose no es válido: `. Por eso cada fila de la tabla prueba **una** clave sola.

- [x] **Step 5: `interpolate_test.go`** (`TestStrictInterpolation`, con `Load`; y `TestStrictSubstitute`, tabla directa sobre `strictSubstitute` con un `Mapping` de mapa):
  - `${A}` y `$A` sin definir → error que nombra `A`; `${A:-x}` y `${A-x}` → `x`; `${A:-x}` con `A=""` → `x` y `${A-x}` con `A=""` → `""`; `${A:+y}` sin definir → `""` y definida → `y`; `${A:?falta A}` → error con "falta A"; `${A?}` con `A=""` → `""`;
  - `$$A` → el valor `$A` en el modelo (y `Marshal` lo vuelve a escribir `$$A`); `$1` y un `$` al final quedan literales (como en compose-go);
  - **lo que compose-go deja pasar** (decisión 17): `${A:-x}-${B}` con `B` sin definir → error que nombra `B`; `${A:-${B}}` y `${A:-$B}` → error de `$` en un valor por defecto, **aunque `B` esté definida**; `${A}${B}` con `B` sin definir → error;
  - `${}`, `${A B}`, `${1A}` y un `${A` sin cerrar → error de formato;
  - un valor por defecto con llaves (`${A:-{"x":1}}`) → `{"x":1}`, igual que compose-go (el cierre se busca contando llaves);
  - **la ruta en el mensaje** (con `Load`): `labels: {com.example.v: ${UNDEF}}` → `compose_yaml: services.web.labels.com.example.v: la variable UNDEF no está definida…` (compose-go guarda el `.` de una clave como `👻` en su `tree.Path`, y `Path.String()` lo devuelve: el mensaje nunca trae `👻`); con dos variables sin definir en servicios distintos, el mensaje es **uno** solo, el de la primera ruta en orden (compose-go los junta con `errors.Join`, ordenados por ruta); un `${UNDEF}` en `deploy.replicas` nombra `services.web.deploy.replicas`;
  - ningún caso escribe en logrus (no hay `WARN … variable is not set` en stderr).

- [x] **Step 6: `marshal_test.go` y `services_test.go`:**
  - `TestMarshalEscapesDollar`: un `$` en un valor de `environment`, en un argumento de `command`, en un valor de `labels` y en el `test` de un healthcheck sale como `$$`; una clave de `environment` con `$` queda igual.
  - `TestMarshalDeterministic`: dos `Marshal` del mismo `File` dan los mismos bytes; el resultado es JSON válido; no aparecen claves vacías (`networks`, `volumes` sin nada).
  - `TestServices`: nombres ordenados; `expose: ["80", 8080, "9000/tcp", "100-200", "${P}"]` → `[80, 8080, 9000]`; un servicio sin `expose` → `Ports: []`; un compose ilegible → `nil`.

- [x] **Step 7: `integration_test.go`** (`//go:build integration`, sin crear nada en Docker):
  - `TestMarshalAcceptedByDockerCLI`: `Load` del kitchen sink con un `env`, le agrega una red externa (`it2x_staging`, `external: true`) a `File.Networks` y a cada servicio, `Marshal`, y corre `docker stack config -c -` con el JSON por stdin, `cmd.Dir = t.TempDir()` y un entorno mínimo (`PATH`, `HOME`, `DOCKER_HOST` si está, `DOCKER_CONFIG=t.TempDir()`). Termina con código 0, y un `$$` del JSON sale como `$` en la salida. Se salta si `docker` no está en el `PATH`.

- [x] **Step 8: Verificar que fallan**

```bash
go test ./src/shared/domain/compose/
```

Expected: no compila o falla por las funciones vacías.

- [x] **Step 9: Implementar.** Lo que importa del diseño:

```go
// details arma la entrada de compose-go: el compose en memoria, nunca un
// archivo; un WorkingDir que no existe (nada debería resolverse contra él), y
// una copia de env (nunca nil) con COMPOSE_PROJECT_NAME = project. compose-go
// también lo agrega (el defer de projectName), pero a la copia: env no se toca.
func details(src, project string, env map[string]string) types.ConfigDetails {
	e := maps.Clone(env)
	if e == nil {
		e = map[string]string{}
	}
	e["COMPOSE_PROJECT_NAME"] = project
	return types.ConfigDetails{
		WorkingDir:  "/nonexistent/muelle-compose",
		ConfigFiles: []types.ConfigFile{{Filename: "compose.yaml", Content: []byte(src)}},
		Environment: e,
	}
}

// options son las del loader: ningún paso que lea archivos (§6.3, "compose-go
// no lee archivos"), el nombre del proyecto explícito, y la lista de
// permitidos, que deja sus hallazgos en *found. Con interpolate, las variables
// se buscan solo en d.Environment (la copia, que ya trae COMPOSE_PROJECT_NAME),
// nunca en el entorno del proceso ni en el env del que llama.
func options(d *types.ConfigDetails, project string, interpolate bool, found *[]loader.UnsupportedAttribute) []func(*loader.Options) {
	return []func(*loader.Options){
		func(o *loader.Options) {
			o.SetProjectName(project, true)
			o.SkipInclude, o.SkipExtends = true, true
			o.SkipResolveEnvironment, o.SkipResolveLabels = true, true
			o.SkipNormalization, o.SkipDefaultValues = true, true
			o.ResolvePaths = false
			o.SkipInterpolation = !interpolate
			if interpolate {
				o.Interpolate.LookupValue = d.LookupEnv // lee d.Environment
				o.Interpolate.Substitute = strictSubstitute
			}
		},
		loader.WithSupportedAttributes(allowed, func(f []loader.UnsupportedAttribute) { *found = f }),
	}
}
```

  `LoadModelWithContext` recibe `ConfigDetails` por valor, pero el mapa `Environment` es el mismo: `d.LookupEnv` ve lo que ve compose-go. `o.Interpolate` ya viene armado por `loader.ToOptions` (con su `TypeCastMapping`); solo se cambian `LookupValue` y `Substitute`.

- **`Validate`:** tamaño → `d := details(src, "compose", nil)` → `loader.LoadModelWithContext(ctx, d, options(&d, "compose", false, &found)...)` → el primer hallazgo de `found` (vienen ordenados por ruta) da el error, con el motivo de `allowlist.go` → `check(dict, false)`.
- **El orden dentro de compose-go** (v2.16.1, `loader.go`): `loadYamlFile` interpola, después corre el esquema (`schema.Validate`, que tiene `additionalProperties: false` en el primer nivel y `unevaluatedProperties: false` en los servicios) y `transform.Canonical`; `load` corre `validation.Validate` y **recién después** la lista de permitidos (`check.Report`). Un error del esquema vuelve antes y `found` queda vacío. La validación del esquema **queda prendida** (`SkipValidation = false`: es la que revisa los tipos al guardar, y sin ella `transform.Canonical` hace `panic` con formas inválidas, por ejemplo `networks: [1]`). Lo que hace `schemaMessage(err)`:
  - un error de clave desconocida del esquema sale con el mismo formato que un hallazgo: `compose_yaml: <ruta>: <motivo>`, con el motivo de `reasons` por el último segmento o el genérico. compose-go no exporta el error del esquema (`validationError`), así que se reconocen sus dos textos, verificados en v2.16.1: `validating compose.yaml: <ruta> false schema` (una clave de un servicio: `services.web.foo false schema`) y `validating compose.yaml: <ruta> additional properties '<k>'[, '<k2>'…] not allowed` (la ruta del padre, vacía en el primer nivel: `validating compose.yaml:  additional properties 'bar', 'foo' not allowed`; las claves vienen en el orden de un mapa de Go, así que se usa la primera **en orden alfabético**, `<ruta>.<k>` o solo `<k>`, para que el mensaje sea estable);
  - cualquier otro error (un tipo, `validation.Validate`, YAML roto) va tal cual: `compose_yaml: el compose no es válido: <texto de compose-go>`, con los saltos de línea como espacios;
  - un test de `schemaMessage` fija los dos textos de compose-go (si cambian al subir la versión, el test lo dice).
- **`Load`:** igual, con `d := details(src, project, env)` y `interpolate = true` siempre (un `env` nil cuenta como vacío); después `check(dict, true)`, `loader.ModelToProject(dict, loader.ToOptions(&d, opts), d)` (con `SkipResolveEnvironment` y `SkipResolveLabels`, no lee archivos; `SkipConsistencyCheck` queda en `false`: verifica que cada volumen usado esté declarado) y `convert(project)`.
- **`allowlist.go`:** `allowed []tree.Path` con las rutas del modelo **canónico** (el que ve la lista de permitidos, después de `transform.Canonical`). `*` vale para cualquier clave y también para los ítems de una lista. Cada mapa de claves libres necesita su `.*` (`labels.*`, `environment.*`, `logging.options.*`, `ulimits.*`, `volumes.*.labels.*`) y cada lista su `.*`; si no, cada clave del mapa sale como hallazgo. El kitchen sink del Step 3 es lo que prueba que la lista está completa. `reasons` asocia la clave rechazada (el último segmento con nombre de la ruta) a su motivo de §6.3 (por ejemplo `ports` → "publicar puertos no está soportado en la v1: los dominios van por Caddy"); lo que no está en `reasons`: "no está soportado: muelle solo acepta las claves de compose que conoce (§6.3)". El mensaje es `compose_yaml: <ruta>: <motivo>`, con la ruta del hallazgo sin los segmentos `[]` (por ejemplo `compose_yaml: services.web.ports: publicar puertos…`).
- **`check.go`** (sobre el `map[string]any` canónico): de 1 a 32 servicios; nombres de servicio y de volumen con `validate.VolumeName`; `image` obligatoria y, si ya no tiene `$`, `validate.ImageRef`; cada montaje es `type: volume` con un `source` declarado arriba, o `type: tmpfs`; `logging.driver` en `json-file`, `local` o `none` (vacío vale porque el render lo completa con `json-file`, T2 Step 6: el compose final siempre lleva un driver); ninguna etiqueta (`labels`, `deploy.labels`, las de un volumen) empieza con `muelle.` o `com.docker.stack.`. Las etiquetas no pasan por `transform.Canonical`: en el modelo pueden venir como mapa o como lista. En una lista, la clave de cada ítem es lo que va antes del **primer** `=`, o el string entero si no tiene `=` (`- muelle.app` es la clave `muelle.app` con valor `""`, como la lee `Labels.DecodeMapstructure`); `deploy.mode` vacío, `replicated` o `global`. Con `interpolated=false`, las reglas sobre un valor que contiene `$` se saltan (también `$VAR` es una variable), pero cada string con `$` pasa por `checkTemplate` (la sintaxis de `interpolate.go`, sin buscar valores): así un `${A:-${B}}` o un `${}` fallan al guardar. Además, para que guardar no acepte lo que el render (la verificación de consistencia de compose-go) rechaza: cada `depends_on` nombra un servicio del compose, `healthcheck.test` en lista empieza con `CMD`, `CMD-SHELL` o `NONE`, y `deploy.replicas` no es negativo.
- **`interpolate.go`** (decisión 17: propio, no envuelve `template.Substitute`):

```go
// strictSubstitute reemplaza las variables de s con m (§5.2 regla 3, §6.3).
// Es la Interpolate.Substitute del loader. Acepta lo mismo que compose-go:
// $$ (un $ literal), $NAME, ${NAME} y ${NAME<op>valor} con op en :- - :+ + :? ?
// (NAME es [_a-zA-Z][_a-zA-Z0-9]*); un $ seguido de otra cosa ($1, un $ al
// final) queda literal. El cierre de ${…} se busca contando llaves, con la regla
// de getFirstBraceClosingIndex de compose-go (se copia: no es exportada; sin
// su salto del carácter que sigue a cada {, que en un ${…} válido nunca es una
// llave: mismo resultado, y ${} da un mensaje más claro).
// Diferencias, a propósito (compose-go las resuelve sin error):
//   - una variable sin definir y sin op es *UndefinedError, también si viene
//     después de otra con op en el mismo string (${A:-x}-${B});
//   - un $ dentro del valor de un op es *TemplateError, aunque la variable
//     exista: ${A:-${B}} no se resuelve (compose-go lo haría sin ser estricto).
// :? y ? sin valor dan *RequiredError con el mensaje del usuario; ${}, ${A B},
// ${1A} y un ${ sin cerrar, *TemplateError. Nunca escribe con logrus.
func strictSubstitute(s string, m template.Mapping) (string, error)

// checkTemplate es strictSubstitute sin buscar valores (Validate): solo los
// *TemplateError.
func checkTemplate(s string) error
```

  **Cómo se saca la ruta.** compose-go envuelve nuestro error en `interpolation.pathError`, que **no es exportado** (ni él ni su campo `path`): la ruta solo está en su mensaje, `error while interpolating <ruta>: <nuestro error>` (`newPathError`; `<ruta>` es `tree.Path.String()`, con los `👻` vueltos a `.`). Además, `interpolation.Interpolate` junta todos los errores con `errors.Join`, anidados (un `Join` por nivel del árbol: el de `services` adentro del de primer nivel) y ordenados por ruta. Por eso `Load` no usa `errors.As` para la ruta, sino `interpolationError(err) (path string, inner error, ok bool)`:
  - recorre el árbol de `err` en profundidad y en orden (`Unwrap() error` y `Unwrap() []error`), y se queda con el **primer** nodo `e` cuyo `errors.Unwrap(e)` (el directo) es uno de nuestros tres tipos;
  - `path = strings.TrimSuffix(strings.TrimPrefix(e.Error(), "error while interpolating "), ": "+inner.Error())`, y `ok` solo si las dos partes coincidieron (así una ruta con `: `, por ejemplo la clave de una etiqueta, no se corta mal);
  - nuestros tres tipos **no** envuelven `*template.InvalidTemplateError`: si lo hicieran, compose-go usaría su otro mensaje (`invalid interpolation format for <ruta>.\nYou may need to escape…`) y el prefijo no coincidiría;
  - el mensaje es uno solo, el del primer error: los demás se descartan (el usuario corrige y vuelve a guardar);
  - la ruta va sin los segmentos `.[]` de las listas, como la de un hallazgo (`services.web.command`, no `services.web.command.[]`).

  Con eso:
  - `*UndefinedError` → `compose_yaml: <ruta>: la variable X no está definida: agrégala en las variables de la app o del stage, o usa ${X:-valor}`;
  - `*RequiredError` (`${X:?…}`) → `compose_yaml: <ruta>: falta la variable X: <motivo>`;
  - `*TemplateError` → `compose_yaml: <ruta>: <motivo>` (por ejemplo `un valor por defecto no puede tener $: ${A:-…}`; nunca el valor de una variable).
  - Si `interpolationError` no encuentra ninguno (`ok == false`), el error sigue a la regla de abajo.
  - Los demás errores de compose-go (los de conversión: `replicas: ${N}` con `N=abc` da `error while interpolating …: failed to cast to expected type: … parsing "abc"`) van tal cual detrás de `compose_yaml: `, con sus saltos de línea como espacios. Traen el **valor** interpolado: por eso un `RenderError` se enmascara antes de escribirse (como todo `Msg`) y nunca va crudo a `slog` (decisión 18).
- **`model.go`:** el compose final, solo con las claves de §6.3 y las que agrega §6.2, con tags `json` y `omitempty`/`omitzero`:

```go
type File struct {
	Services map[string]Service `json:"services"`
	Volumes  map[string]Volume  `json:"volumes,omitempty"`
	Networks map[string]Network `json:"networks,omitempty"`
}
type Network struct {
	Name     string `json:"name"`
	External bool   `json:"external"`
}
type Volume struct {
	Labels map[string]string `json:"labels,omitempty"`
}
type Service struct {
	Image           string            `json:"image"`
	Command         []string          `json:"command,omitempty"`
	Entrypoint      []string          `json:"entrypoint,omitempty"`
	Environment     map[string]string `json:"environment,omitempty"`
	Healthcheck     *Healthcheck      `json:"healthcheck,omitempty"`
	Deploy          *Deploy           `json:"deploy,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Logging         *Logging          `json:"logging,omitempty"`
	Networks        []string          `json:"networks,omitempty"`
	Volumes         []Mount           `json:"volumes,omitempty"`
	Tmpfs           []string          `json:"tmpfs,omitempty"`
	User            string            `json:"user,omitempty"`
	WorkingDir      string            `json:"working_dir,omitempty"`
	StopGracePeriod string            `json:"stop_grace_period,omitempty"`
	StopSignal      string            `json:"stop_signal,omitempty"`
	ReadOnly        bool              `json:"read_only,omitempty"`
	Init            *bool             `json:"init,omitempty"`
	Ulimits         map[string]Ulimit `json:"ulimits,omitempty"`
	CapDrop         []string          `json:"cap_drop,omitempty"`
	Hostname        string            `json:"hostname,omitempty"`
	Domainname      string            `json:"domainname,omitempty"`
	ExtraHosts      []string          `json:"extra_hosts,omitempty"`
	DNS             []string          `json:"dns,omitempty"`
	DNSSearch       []string          `json:"dns_search,omitempty"`
	Expose          []string          `json:"expose,omitempty"`
	Tty             bool              `json:"tty,omitempty"`
	StdinOpen       bool              `json:"stdin_open,omitempty"`
}
// Healthcheck, Deploy (Mode, Replicas *uint64, Labels, EndpointMode, Placement,
// Resources {Limits, Reservations *Resource{CPUs string, Memory string}},
// RestartPolicy, UpdateConfig, RollbackConfig *UpdateConfig{Parallelism
// *uint64, Delay, FailureAction, Monitor string, MaxFailureRatio *float64,
// Order}), Logging {Driver, Options}, Mount {Type, Source, Target, ReadOnly,
// Volume *{NoCopy}, Tmpfs *{Size int64, Mode uint32}}, Ulimit {Soft, Hard}.
```

  Las duraciones van como `time.Duration.String()` (`"30s"`), la memoria en bytes **como string decimal** (`"536870912"`: el esquema de `docker stack deploy` exige un string en `memory`, verificado con Docker 29.6.1; un entero da `must be a string`) y los `cpus` (compose-go los da como `types.NanoCPUs`, un `float32`) con `strconv.FormatFloat(float64(x), 'f', -1, 32)`: con 64 bits, `cpus: 0.1` saldría `"0.10000000149011612"`. (El `cpus` de un formulario es un `float64` del JSON: ahí va con 64, T2 Step 5.)
- **`marshal.go`:** `json.Marshal` → decodificar a `any` con `UseNumber` → recorrer y reemplazar `$` por `$$` en cada string **valor** (no en las claves de los mapas) → `json.MarshalIndent` con dos espacios y `\n` final.
- **`Services`:** `LoadModelWithContext` sin interpolar, con las mismas opciones de lectura, sin lista de permitidos y **con** la validación del esquema (no `SkipValidation`: sin ella, `transform.Canonical` hace `panic` con una forma inválida, verificado con `networks: [1]`, y las filas que guardó la Fase 1 solo pasaron por el chequeo de vacío y de tamaño). Devuelve los nombres y los puertos enteros de `expose` (`80`, `"8080"`, `"9000/tcp"`; los rangos, los `/udp` y los `${…}` se omiten), ordenados y sin repetir. Ante cualquier error, `nil`. `TestServices` suma un compose que el esquema rechaza (`networks: [1]`, `a: 1`) → `nil`, sin `panic`.
- compose-go puede escribir advertencias con logrus en stderr (por ejemplo, `version` obsoleta). No llevan valores de variables; no se cambia la configuración global de logrus.

- [x] **Step 10: Verificar y commit**

```bash
go mod tidy
go test -race ./src/shared/domain/compose/
go test -tags integration -run TestMarshalAcceptedByDockerCLI -count=1 ./src/shared/domain/compose/
```

Expected: `ok` en los dos (el de integración no crea nada en Docker). Con la verificación completa de las Convenciones:

```bash
git add go.mod go.sum src/shared/domain/compose
git commit -m "feat(compose): paquete compartido que valida un compose con compose-go (§6.3) sin leer archivos y arma el compose final" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

- [x] **Step 11: Catalog valida con el paquete (tests primero)**
  - `entity_test.go`: los fixtures `"a: 1"`, `"b: 2"` pasan a composes válidos (`"services:\n  web:\n    image: nginx\n"`…). Nuevo `TestValidateComposeSharedRules`: con `ports` → `wantInvalid(t, err, "compose_yaml")`; vacío y de 256 KiB + 1 → los mismos mensajes de antes.
  - `usecases_test.go`: los fixtures de compose pasan a válidos. El caso existente de una app de formulario con el slug de otra de formulario sigue esperando `ErrConflict` (409, del índice único), no un 422. Nuevo `TestServiceNamesUniquePerStage`:
    - una app compose con `web` y otra compose del mismo stage con `web` → 422 en `compose_yaml`, y el mensaje nombra el servicio y la app dueña;
    - una compose con un servicio `api` cuando ya existe la app de formulario `api` → 422 `compose_yaml` (el mensaje nombra la app `api`);
    - una app de formulario `web` cuando una compose del stage tiene `web` → 422 en `slug`, con el mensaje que nombra la app compose;
    - una app de formulario `api` cuando ya existe la de formulario `api` → `ErrConflict` (no 422: el chequeo de servicios no mira los slugs de formulario al crear una de formulario);
    - editar el compose de una app sin cambiar sus nombres → OK (no choca consigo misma);
    - los mismos nombres en otro stage → OK.
  - `module_test.go`: `POST` de una app compose con `ports` → 422 `invalid` con un mensaje que empieza con `compose_yaml: `; los casos que mandaban `"a: 1"` pasan a un compose válido.
- [x] **Step 12: Implementar.**
  - `entity.ValidateCompose(yaml) ([]string, error)` delega en `compose.Validate` (que ya revisa vacío y tamaño, con los mismos mensajes) y devuelve los nombres de los servicios, que usan los casos de uso. `MaxComposeBytes` queda como `compose.MaxBytes`.
  - `service_names.go`:

```go
// stageServices devuelve, para cada nombre de servicio del stage, la app que
// lo usa (§5.2 regla 2), salvo la app except. withForms dice si entran las apps
// de formulario (con su slug); las compose entran siempre, con los servicios de
// su compose (compose.Services: ya se validó al guardar). Corre dentro de la
// transacción del caso de uso.
func stageServices(ctx context.Context, apps repository.AppRepository, stageID, except int64, withForms bool) (map[string]string, error)

// checkServiceNames da un *validate.Error en field si un nombre ya lo usa otra app del stage.
func checkServiceNames(owned map[string]string, names []string, field string) error
```

  - `CreateApp`, dentro del `InTx` y después de leer el stage:
    - **de formulario:** los nombres son `[slug]` (campo `slug`) contra `stageServices(…, withForms = false)`, solo los servicios de las compose. Un slug que ya usa otra app de formulario sigue siendo `409 conflict` del índice único (`Apps.Create`), como hoy;
    - **compose:** los nombres de `compose.Validate` (campo `compose_yaml`) contra `stageServices(…, withForms = true)`: los slugs de las de formulario y los servicios de las otras compose.
  - `UpdateApp` con `compose_yaml`: dentro del `InTx`, con `except = app.ID` y `withForms = true`.
  - Mensajes: `el servicio "web" ya existe en la app "front" del stage: los servicios de un stage comparten la red` (`compose_yaml`) y, para `slug`, `"web" ya es un servicio de la app compose "front" del stage`.
- [x] **Step 13: Verificar y commit:** `feat(catalog): valida el compose al guardar con shared/domain/compose y los nombres de servicio únicos por stage`.
- [x] **Step 14: `services` en la respuesta (contrato §2.12; test primero).** En `module_test.go`, `TestAppServices`: `GET` de una app de formulario con `port: 3000` → `services: [{"name": "<slug>", "ports": [3000]}]`; de una compose con `web` (`expose: ["80"]`) y `worker` (sin `expose`) → `[{"name":"web","ports":[80]},{"name":"worker","ports":[]}]`; `GET …/apps` trae `services` en cada una; nunca `null`. Implementar en `dto.AppFrom`: `ServiceOutput{Name, Ports}` (`json:"name"`, `json:"ports"`), el puerto de un formulario sale de decodificar `{"port": N}` del spec guardado. Commit: `feat(catalog): los servicios de cada app y sus puertos en la respuesta (contrato §2.12)`.

**Integración en T1:** `go test -tags integration -run TestMarshalAcceptedByDockerCLI ./src/shared/domain/compose/` (solo `docker stack config`, sin efectos).

---

### Task T2: El dominio de deploy (puro)

Las entidades, el spec de formulario, el render con sus golden files, la regla de qué imagen se despliega, el enmascarado, el formato del log y la regla de convergencia. Todo funciones puras con tests de tabla; no toca la base ni Docker (salvo el test de integración con `docker stack config`).

**Files:**
- Create: `src/internal/deploy/domain/entity/{deploy,registry}.go` y sus tests
- Create: `src/shared/domain/validate/registry_deploy.go`, `registry_deploy_test.go` (las reglas de un registry: las usan deploy y `settings.Load`, T3 Step 8)
- Create: `src/internal/deploy/domain/service/{errors,formspec,config,render,images,masker,deploylog,convergence}.go` y sus tests
- Create: `src/internal/deploy/domain/service/testdata/render/*.input.json`, `*.golden.json`
- Create: `src/internal/deploy/domain/service/render_integration_test.go` (`//go:build integration`)
- Modify: `src/internal/deploy/domain/service/ports.go` (los tipos de Swarm que usa `Evaluate`)

- [ ] **Step 1: Entidades (tests primero, `entity_test.go`).**
  - `deploy.go`: `Status` (`queued`, `running`, `succeeded`, `failed`, `superseded`) con `Final() bool`; `Trigger` (`panel`, `api`, `rollback`); `Deploy`:

```go
type Deploy struct {
	ID, AppID int64
	Status    Status
	Trigger   Trigger
	// Images: mientras está queued, los overrides (en un rollback, las del
	// destino); desde el paso 2, las refs resueltas (contrato §2.1).
	Images map[string]string
	// RequestedImages y ConfigImages: las refs pedidas y las de la
	// configuración, que el paso 2 guarda (contrato §2.2). {} hasta entonces.
	RequestedImages, ConfigImages map[string]string
	ActorType    string // "user" o "token"
	ActorID      *int64
	ActorName    string
	SupersededBy *int64
	Error        *string
	CreatedAt    time.Time
	StartedAt    *time.Time
	FinishedAt   *time.Time
}
```

  - `registry.go`: `Registry{ID, Host, Username, Password, CreatedAt, UpdatedAt}`, con `String`, `GoString`, `LogValue` y `MarshalJSON` que ocultan `Password` (la entidad lleva la contraseña en claro en memoria). Sin las reglas de validación: van en `shared/domain/validate` (abajo).
  - `src/shared/domain/validate/registry_deploy.go` (archivo nuevo del paquete compartido; decisión 19: `settings.Load` también las usa, y `src/settings` no puede importar el dominio de un módulo): `RegistryHost(field, s)`, `RegistryUsername(field, s)` y `RegistryPassword(field, s)`, con la convención del paquete (`*validate.Error` en `field`, como `ImageRef(field, s)`) y los mensajes de contrato §2.10. Deploy las llama con `"host"`, `"username"` y `"password"`. Host canónico: `^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*(:[0-9]{1,5})?$`, puerto de 1 a 65535, hasta 255 caracteres; `index.docker.io`, `registry-1.docker.io` y `registry.hub.docker.com` → `<field>: para Docker Hub usa docker.io`. Usuario de 1 a 255 caracteres, sin caracteres de control. Contraseña de 1 a 4096 bytes, sin `\n` ni `\r`. **Ningún mensaje incluye el valor** (tampoco el usuario: un mensaje de `settings.Load` no debe dar pistas de la credencial).
  - Tests: en `registry_deploy_test.go`, `TestRegistryHost` (aceptados: `ghcr.io`, `docker.io`, `registry.example.com:5000`, `localhost:5000`; rechazados: `GHCR.io`, `https://ghcr.io`, `ghcr.io/`, `ghcr.io/x`, `ghcr.io:0`, `ghcr.io:70000`, `-x.io`, los tres alias con su mensaje), `TestRegistryUsername` (vacío, 256 caracteres, `\x00`), `TestRegistryPassword` (vacío, 4097 bytes, con `\n` o `\r`; 4096 pasa; ningún mensaje contiene la contraseña probada); en `entity_test.go`, `TestRegistryNeverPrintsPassword` (`%v`, `%+v`, `%#v`, `slog` JSON, `json.Marshal`: ninguno contiene la contraseña).
- [ ] **Step 2: Tipos de Swarm en `ports.go`** (para `Evaluate`; los usa T4):

```go
// SwarmService es un servicio de Swarm como lo lee deploy.
type SwarmService struct {
	ID, Name         string
	Labels           map[string]string // deploy.labels (muelle.deploy, …)
	Image            string
	Replicas         *uint64 // nil: global
	RunningTasks     uint64
	DesiredTasks     uint64
	MemoryLimitBytes int64
	Update           *UpdateStatus // nil: nunca se actualizó (o Swarm la reinició)
	CreatedAt        time.Time
}

type UpdateStatus struct {
	State, Message string // los Update* de shared/infra/out/docker
	StartedAt      *time.Time
}

type SwarmTask struct {
	ID, ServiceID, NodeID      string
	Slot                       int // 0 en un servicio global
	State, DesiredState, Err   string
	ContainerID                string
	CreatedAt, UpdatedAt       time.Time
}

// LogLine es una línea de log de una tarea; Time cero y TaskID vacío en el
// pedazo de una línea larga (contrato §2.5).
type LogLine struct {
	Time           time.Time
	Stream, TaskID string
	Text           string
}
```

- [ ] **Step 3: `images.go` (tests primero, `images_test.go`).**

```go
// ChooseImages elige la ref pedida de cada servicio de la configuración
// (contrato §2.2): el override; si no, la pedida del deploy actual si la ref
// de la configuración no cambió desde ese deploy; si no, la de la
// configuración. Los overrides de servicios que no están en config se ignoran
// (el caso de uso los avisa en el log). current puede ser nil.
func ChooseImages(config, overrides map[string]string, current *entity.Deploy) map[string]string

// ResolvedRef es la ref resuelta: la pedida con el tag explícito y el digest
// (`nginx` → `nginx:latest@sha256:…`); una pedida con digest queda igual.
func ResolvedRef(requested, digest string) (string, error)

// ImageHost es el host de la credencial: reference.Domain(ParseNormalizedNamed(ref)).
func ImageHost(ref string) (string, error)

// PinnedDigest es el digest de una ref que ya lo trae, o "". No es un atajo
// para no consultar el registry (decisión 1): el paso 2 lo usa solo para
// comprobar que Images.Digest devolvió ese mismo digest.
func PinnedDigest(ref string) string
```

  Tests:
  - `TestChooseImages` (tabla): primer deploy → la de la configuración; override → el override; sin cuerpo después de un deploy de CI con `x:sha-1` → `x:sha-1` (tag, se vuelve a resolver); después de uno por digest `x@sha256:A` → `x@sha256:A`; configuración con digest (`x:1.2@sha256:B`) → se mantiene en el siguiente sin cuerpo; la configuración cambió (`config_images` distinto) → la nueva de la configuración; un servicio nuevo del compose → el de la configuración; override de un servicio y los demás de arrastre.
  - `TestRollbackNotUndoneByBodylessDeploy`: D1 (`x:staging` → `x:staging@sha256:A`), D2 (`x:staging@sha256:B`), el rollback R pide `D1.Images` (con digest); `ChooseImages(config, {}, R)` → `x:staging@sha256:A`. Es la afirmación del contrato §2.2.
  - `TestResolvedRef`: `nginx` → `nginx:latest@d`; `ghcr.io/x/y:sha-1` → `ghcr.io/x/y:sha-1@d`; `ghcr.io/x/y@sha256:…` igual; `x:tag@sha256:…` igual; `docker.io/library/nginx:1` → `nginx:1@d` (forma familiar).
  - `TestImageHost`: `nginx` → `docker.io`; `ghcr.io/x` → `ghcr.io`; `localhost:5000/x` → `localhost:5000`; `registry.example.com:5000/a/b` → `registry.example.com:5000`.
- [ ] **Step 4: `masker.go` y `deploylog.go` (tests primero).**

```go
// Masker reemplaza los valores secretos por *** (contrato §2.5): cada valor
// de 8 bytes o más y, de uno de varias líneas, también cada línea (sin los
// espacios de los bordes) de 8 bytes o más; sin repetidos y del más largo al
// más corto, uno después del otro (strings.ReplaceAll en ese orden).
type Masker struct{ values []string }

func NewMasker(secretValues []string) Masker
func (m Masker) Mask(s string) string

const (
	LogCap       = 256 << 10 // 262 144 bytes (contrato §2.5)
	LogReserve   = 4 << 10   // para las líneas de cierre
	MaxLineBytes = 8 << 10   // una línea, sin el \n
	AppLineBytes = 16 << 10  // una línea de §2.9
	JoinMax      = 64 << 10  // los pedazos de una línea larga, unidos
	TruncatedMarker = "… (log truncado: se superaron los 256 KB)"
)

// FormatLine arma "<hora> <texto>\n" (una sola línea, contrato §2.5): hora UTC
// con tres decimales (2006-01-02T15:04:05.000Z), y el texto, en este orden,
// enmascarado (Mask), limpio (Sanitize) y cortado a MaxLineBytes con "…"
// (CutLine). Se enmascara antes de limpiar: un secreto de varias líneas cuyas
// líneas miden menos de 8 bytes solo se reconoce entero, con sus \n.
func FormatLine(m Masker, t time.Time, text string) string

// Sanitize deja el texto en una línea: quita los \r y cambia por un espacio
// los \n y los demás caracteres de control (C0, DEL y C1), salvo \t. Algunos
// textos que llegan al log traen \n (los errores de compose-go, los de Caddy
// cuando SyncRoutes exista); sin esto quedarían líneas sin hora.
func Sanitize(s string) string

// AppendPlan decide qué se agrega a un log de cur bytes (contrato §2.5). lines
// ya están formateadas; marker es TruncatedMarker formateada. Las normales
// entran mientras cur+len ≤ LogCap−LogReserve; la primera que no entra agrega
// marker (si truncated era false) y las siguientes se descartan. Las de cierre
// (final) entran mientras cur+len ≤ LogCap.
func AppendPlan(cur int, truncated bool, lines []string, final bool, marker string) (add string, nowTruncated bool)

// CutLine corta s a n bytes, terminando en "…" y sin partir una runa; los bytes
// de UTF-8 inválido salen como U+FFFD.
func CutLine(s string, n int) string

// JoinPieces une a la línea anterior del mismo stream cada pedazo (Time cero y
// TaskID vacío) que entrega el cliente de Docker, hasta JoinMax bytes; lo que
// pasa de ahí se descarta (contrato §2.5).
func JoinPieces(lines []LogLine) []LogLine

// SplitOutput parte la salida del CLI en líneas, sin las vacías del final.
func SplitOutput(out string) []string

// ErrorLine es el error de un deploy (contrato §2.1): enmascarado, después
// Sanitize (sin saltos de línea: van como espacios), y hasta 500 runas.
func ErrorLine(m Masker, msg string) string
```

  Tests:
  - `TestMaskerOrder` (`"abcdefgh"` y `"abcdefgh12"`: el más largo gana), `TestMaskerShortValues` (`1`, `true`, `3000` y uno de 7 bytes no se tocan; uno de 8 sí), `TestMaskerMultiline` (una clave PEM: el valor entero y cada línea ≥ 8 bytes; una línea de 2 bytes no), `TestMaskerEmpty`.
  - `TestFormatLine`: hora `2026-10-10T14:32:07.120Z` (tres decimales, UTC); `\r` quitado; un texto con `\n` adentro (el de un `InvalidTemplateError` de compose-go: `"invalid interpolation format for x.\nYou may need to escape any $ with another $."`) sale como **una** línea, con espacios, y el resultado tiene un solo `\n`, el del final; `\x1b`, `\x00` y `\x7f` → espacio; `\t` queda; un secreto de varias líneas con líneas de menos de 8 bytes (`"ab\ncdefg\nhi"`) queda como `***` (se enmascara antes de limpiar); un secreto que cruza el byte 8192 queda entero como `***` (se enmascara antes de cortar); la línea mide ≤ 8192 bytes + `\n` y es UTF-8 válido con una `ñ` justo en el corte.
  - `TestAppendPlan`: entra la normal que cabe; la primera que no cabe agrega la marca una sola vez; las siguientes normales se descartan; una de cierre entra en la reserva aunque el log esté truncado; una de cierre que pasa `LogCap` no entra.
  - `TestJoinPieces`: dos pedazos se unen a su línea; uno sin línea anterior queda solo; un stream distinto no se mezcla; el tope de 64 KiB.
  - `TestErrorLine`: 600 runas → 500; `\n` → espacio; enmascarado, también un secreto de varias líneas.
- [ ] **Step 5: `formspec.go` y `config.go` (tests primero).**

```go
// FormSpec es apps.spec (§6.1) como lo lee deploy: catalog ya lo validó y
// normalizó. Se decodifica con json/v2 sin rechazar campos (deploy no es dueño del formato).
type FormSpec struct { /* image, port, replicas, command, healthcheck{type, path, command, interval_s, timeout_s, retries, start_period_s}, volumes[{name, mount}], resources{memory_mb, cpus} */ }

func ParseFormSpec(raw []byte) (FormSpec, error)

// Config es la configuración de la app que leyó un deploy (paso 2): el
// modelo base, sin las inyecciones de §6.2, y la ref de cada servicio.
type Config struct {
	File       *compose.File
	Images     map[string]string // servicio → ref de la configuración (config_images)
	HasVolumes bool              // algún servicio monta un volumen nombrado
}

// ReadConfig arma la Config. Formulario: un servicio con el slug de la app,
// con todas las variables como environment (§5.2 regla 3), el command en
// forma exec, el healthcheck de §6.1, los volúmenes nombrados, replicas y
// resources.limits. Compose: compose.Load con env. Sus errores son *RenderError.
func ReadConfig(ctx context.Context, app App, env map[string]string) (Config, error)
```

  El healthcheck de un formulario (§6.1): `http` → `["CMD", "wget", "-q", "-O", "/dev/null", "http://127.0.0.1:<port><path>"]`; `bun-http` → `["CMD", "bun", "-e", "fetch('http://127.0.0.1:<port><path>').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"]`; `command` → `["CMD", …command]`; `none` → sin `healthcheck`. Tiempos como `"10s"`, `retries` tal cual. `memory_mb` → bytes (`MB << 20`) como string decimal (`Resource.Memory`, T1), `cpus` (un `float64` del JSON) → `strconv.FormatFloat(x, 'f', -1, 64)` (`0.1` → `"0.1"`).
- [ ] **Step 6: `render.go` (tests primero, con golden files).**

```go
type RenderInput struct {
	App           App
	DeployID      int64
	Images        map[string]string // servicio → ref resuelta (todas las de la Config)
	StorageNodeID string            // obligatorio si Config.HasVolumes
	Domains       []DomainTarget
}

type Rendered struct {
	Compose  []byte   // compose.Marshal del resultado; nunca se guarda (§12)
	Services []string // nombres cortos, ordenados
	Hosts    []string // hosts de las imágenes (ImageHost), para las credenciales del paso 5
}

// RenderError es un error de render: el deploy falla con "render: <Msg>" y
// Swarm no se toca. Msg puede traer valores de variables: se enmascara antes de escribirlo.
type RenderError struct{ Msg string }

// Render aplica §6.2 a la Config y verifica los destinos de dominio y el entorno (§6.3).
func Render(cfg Config, in RenderInput) (Rendered, error)
```

  Para cada servicio (§6.2, campo por campo):
  - `image` = `in.Images[svc]` (falta → error de programación, `RenderError`);
  - `networks` = `[naming.Network(p, s)]` y `File.Networks = {<red>: {Name: <red>, External: true}}`;
  - `logging`, campo por campo (§6.2): si falta, `{driver: json-file, options: {max-size: 10m, max-file: "3"}}`; si está sin `driver` (o con `driver: ""`, también uno que quedó vacío al interpolar), `driver = json-file`; con `json-file` o `local`, se agregan `max-size: 10m` y `max-file: "3"` si faltan, y las demás opciones del usuario quedan; con `none`, nada. El compose final **siempre** lleva un `driver` (y check lo da por hecho): si no, Docker usaría el driver por defecto del daemon (`journald`, `syslog` o `gelf` en algunos hosts) con las opciones libres del usuario, y se saltaría la regla de §6.3;
  - `deploy.update_config`: completa `parallelism: 1`, `order: start-first`, `failure_action: rollback`, `monitor: 30s` en lo que falta; `deploy.rollback_config`: `parallelism: 1`, `order: start-first`;
  - con volúmenes nombrados: `order` por defecto `stop-first` (en los dos), y `node.id == <StorageNodeID>` en `placement.constraints` salvo que ya haya una restricción sobre `node.id` o `node.hostname`;
  - `deploy.labels` += `naming.Labels(p, s, a, DeployID)`; `labels` (contenedor) += `naming.AppLabels(p, s, a)` (sin `muelle.deploy`, §6.2);
  - entorno: cada `CLAVE=valor` ≤ 128 KiB y el total (`len(k)+1+len(v)+1+8` por variable) ≤ 1,5 MiB; si no, `RenderError` con el servicio y el tamaño (nunca el valor);
  - destinos: cada `DomainTarget.Service` tiene que ser un servicio del compose (en un formulario, el slug), con `Port` de 1 a 65535; si no: `el dominio api.x.cl apunta al servicio web, que no está en la configuración de la app`.

  Golden files (`testdata/render/<caso>.input.json` → `<caso>.golden.json`; el input es un JSON con `kind`, `spec` o `compose`, `env`, `images`, `storage_node_id`, `domains`). `TestRenderGolden` recorre los casos y compara byte a byte; con `go test ./src/internal/deploy/domain/service -run TestRenderGolden -update` reescribe los golden (`var update = flag.Bool("update", false, "reescribe los golden")`). Casos:
  - `form-http` (puerto 3000, `/health`, 2 réplicas, recursos, una variable con `$`), `form-bun`, `form-command`, `form-none`;
  - `form-volumes` (dos volúmenes: `stop-first`, `node.id == node-1`, volúmenes de primer nivel);
  - `form-command-dollar` (`["sh", "-c", "echo $HOME"]` → `$$HOME`);
  - `compose-basic` (dos servicios, `${DB_URL}` interpolada, un `$$` del YAML que vuelve a salir como `$$`, `depends_on` y un ancla `x-` que no salen);
  - `compose-overrides` (`logging` propio con `local` y `max-size: 1m`, que gana y suma `max-file: "3"`; `update_config: {parallelism: 2}` que se completa con el resto, un servicio con volumen y `node.hostname == x` sin la restricción de muelle, y `order: start-first` propio respetado);
  - `compose-logging-options` (un servicio con `logging: {options: {max-size: 5m, compress: "true"}}` sin `driver` → `json-file`, `max-size: 5m` del usuario, `max-file: "3"` agregado y `compress` intacto; otro con `logging: {driver: none}` → sin opciones agregadas);
  - `compose-global` (`mode: global`, `cpus: 0.1` → `"0.1"`), `compose-volume-labels`.

  Además de comparar, `TestRenderGolden` decodifica cada golden y verifica que **todo** servicio lleva `logging.driver` en `json-file`, `local` o `none`, y que con `json-file` o `local` tiene `max-size` y `max-file`.

  `TestRenderErrors` (tabla): destino de dominio que no existe (el mensaje tiene el hostname y el servicio), una variable de 130 KiB, un total de 1,6 MiB, falta la imagen de un servicio, `StorageNodeID` vacío con volúmenes, destino distinto del slug en un formulario.
- [ ] **Step 7: `convergence.go` (tests primero).**

```go
// Verdict es el resultado de mirar los servicios una vez (§7 paso 6).
type Verdict struct {
	Done     bool              // todos convergieron
	Failed   bool              // alguno falló: Service y Reason dicen cuál y por qué
	Service  string
	Reason   string
	Progress map[string]string // nombre corto → "1/2 réplicas corriendo"
}

// Evaluate aplica la regla del paso 6 a los servicios esperados (nombres de
// Swarm) con lo que devolvió Swarm. appliedAt es la hora de justo antes de
// ejecutar docker stack deploy: un UpdateStatus de este deploy es uno con
// StartedAt ≥ appliedAt−2s. settleFrom es cuando docker stack deploy volvió
// (decisión 6): un servicio que ya existía y no tiene UpdateStatus de este
// deploy converge cuando pasó settle desde settleFrom (no desde appliedAt: el
// CLI puede tardar más que settle) y corren las deseadas.
func Evaluate(expected []string, services []SwarmService, appliedAt, settleFrom, now time.Time, settle time.Duration) Verdict

// AppliedState dice cuántos de los servicios esperados llevan muelle.deploy =
// deployID (contrato §1.2): AppliedNone, AppliedPartial o AppliedAll.
func AppliedState(expected []string, services []SwarmService, deployID int64) Applied
```

  `TestEvaluate` (tabla): nuevo y todas corriendo → listo; nuevo a medias → espera; update de este deploy `completed` → listo; `updating` → espera; `rollback_started`, `rollback_completed`, `rollback_paused` y `paused` → falla (con `Message`); update viejo `completed` antes del margen → espera, y después del margen con las deseadas corriendo → listo; `Update` nil en uno que ya existía → igual que el viejo; **el CLI tardó más que el margen** (`appliedAt = t0`, `settleFrom = t0+8s`, `settle = 5s`, un servicio que ya existía, sin `UpdateStatus` y con las deseadas corriendo): a `now = t0+9s` espera (contado desde `appliedAt` ya habría pasado), y a `now = t0+13s` → listo; `replicas: 0` → listo; global usa `DesiredTasks`; un servicio que todavía no aparece → espera; dos servicios, uno falla → falla con el primero por nombre. `TestAppliedState`: ninguno, algunos, todos.
- [ ] **Step 8: Integración (`render_integration_test.go`, `//go:build integration`, sin crear nada):** `TestRenderGoldensAcceptedByDockerCLI` corre `docker stack config -c -` con cada `*.golden.json` por stdin (mismo entorno mínimo que en T1) y espera código 0; en `form-command-dollar`, la salida trae `$HOME` (un solo `$`).
- [ ] **Step 9: Verificar y commits** (uno por grupo, cada uno verde):

```bash
go test -race ./src/internal/deploy/...
go test -tags integration -run TestRenderGoldensAcceptedByDockerCLI -count=1 ./src/internal/deploy/domain/service/
```

  1. `feat(deploy): entidades de deploy y registry, con la contraseña oculta al imprimirla, y las reglas de un registry en shared/domain/validate`
  2. `feat(deploy): qué imagen se despliega con la ref pedida (contrato §2.2), enmascarado y formato del log`
  3. `feat(deploy): render del compose final con las inyecciones de §6.2 y golden files`
  4. `feat(deploy): la regla de convergencia del paso 6 y el estado aplicado de un deploy`

**Integración en T2:** `TestRenderGoldensAcceptedByDockerCLI` (solo `docker stack config`).

---

### Task T3: La base, los registries y la API de los deploys (sin el worker)

La migración, los repositorios, el ocultamiento en los tipos de Docker, el CRUD de registries, los endpoints que encolan y leen deploys, y el registry inicial de install. Al terminar, `POST …/deploys` deja deploys `queued` que todavía nadie corre (el worker es T4): es un estado intermedio de la rama, con todo verde.

> **Cambio de requisito del dueño (2026-10-15): las imágenes de muelle son privadas.** Install (Fase 5) crea el secreto `muelle_init_registry` con un JSON `{"host","username","password"}` (la credencial de `ghcr.io`), lo monta en el servicio y pasa su ruta en `MUELLE_INIT_REGISTRY_FILE`. Esta tarea lo lee (`src/settings`) y lo guarda como un registry **solo en el primer arranque de la base** (decisión 19). Todo está en el Step 8.

**Files:**
- Modify: `src/shared/infra/out/sqlite/migrations/0002_deploy.sql`, `migrations_test.go`
- Create: `src/shared/infra/out/docker/redact_deploy.go`, `redact_deploy_test.go`
- Create: `src/internal/deploy/domain/repository/repository.go`
- Create: `src/internal/deploy/infra/out/repository/sqlite/{deploys,registries}.go` y tests
- Create: `src/internal/deploy/infra/out/service/settings.go`, `settings_test.go` (Step 8: la marca; T4 le suma `manager_node_id`)
- Modify: `src/internal/deploy/domain/service/{ports,errors}.go`
- Create: `src/internal/deploy/app/dto/{deploy,registry}.go` (o en `dto.go`)
- Create: `src/internal/deploy/app/usecases/{create_deploy,rollback,queries,registries,logwriter,hub,seed_registry}.go` y tests
- Create: `src/internal/deploy/infra/in/http/{handler,deploys,registries,mapper_errors,res}.go`, `src/internal/deploy/infra/in/form/{deploy,registry}.go`
- Modify: `src/internal/deploy/module.go`, `module_test.go`
- Modify (Step 8, lo mínimo): `src/settings/settings.go`, `src/settings/settings_test.go`
- Modify (Step 8, solo dos lugares): `src/shared/infra/container/container.go`
- Create (Step 8): `src/shared/infra/container/deploy_test.go` (T5 le suma los contratos)

- [ ] **Step 1: La migración (test primero).** En `migrations_test.go`, `TestDeploysSchema` suma: los defaults `requested_images = '{}'` y `log_truncated = 0`, y que `log_truncated = 2` falla. En `0002_deploy.sql`, en `deploys`:

```sql
  -- JSON servicio → ref pedida, antes de resolver (contrato §2.2): un deploy
  -- sin cuerpo repite la del deploy actual. Tiene digest solo si se fijó a
  -- propósito (override, configuración, rollback). {} hasta el paso 2.
  requested_images TEXT    NOT NULL DEFAULT '{}',
  …
  -- 1 cuando el log llegó al tope y ya tiene la marca (contrato §2.5).
  log_truncated    INTEGER NOT NULL DEFAULT 0 CHECK (log_truncated IN (0, 1)),
```

  Commit: `feat(sqlite): 0002 suma requested_images y log_truncated a deploys`.
- [ ] **Step 2: Ocultar secretos en los tipos de Docker (contrato §2.10; test primero).** `redact_deploy_test.go`, `TestSecretsNeverPrinted`: con `a := RegistryAuth{Host: "ghcr.io", Username: "bot", Password: "s3cr3t-pass"}` y `d := StackDeploy{Name: "p_s_a", Compose: []byte("MARCADOR-DEL-COMPOSE"), Registries: []RegistryAuth{a}}`, ninguna de estas salidas contiene `s3cr3t-pass` ni `MARCADOR-DEL-COMPOSE`: `fmt.Sprintf` con `%v`, `%+v`, `%#v` y `%s` de `a`, `&a`, `[]RegistryAuth{a}`, `d` y `&d`; `fmt.Errorf("%v", d).Error()`; `slog` con `slog.NewJSONHandler` y `slog.NewTextHandler` y `slog.Any("x", a)`, `slog.Any("x", d)`; `encoding/json` y `encoding/json/v2` `Marshal` de los dos. Y sí aparecen `ghcr.io`, `bot` y `p_s_a` (y el tamaño del compose). En `redact_deploy.go` (archivo nuevo; no se toca `docker.go`):

```go
// Los tipos que llevan secretos se imprimen sin ellos (contrato §2.10):
// RegistryAuth sin la contraseña, StackDeploy sin el compose (trae los valores
// de las variables) y con sus registries ocultos.
func (a RegistryAuth) String() string        // {Host:ghcr.io Username:bot Password:[oculta]}
func (a RegistryAuth) GoString() string      // docker.RegistryAuth{…}
func (a RegistryAuth) LogValue() slog.Value  // grupo host, username, password="[oculta]"
func (a RegistryAuth) MarshalJSON() ([]byte, error)
func (d StackDeploy) String() string         // {Name:p_s_a Compose:(1234 bytes) Registries:[…]}
func (d StackDeploy) GoString() string
func (d StackDeploy) LogValue() slog.Value
func (d StackDeploy) MarshalJSON() ([]byte, error)
```

  `MarshalJSON` serializa un struct anónimo (sin recursión). Una contraseña vacía sale `""`. Verificar que `dockerConfig`, `encodeRegistryAuth` y el fake siguen usando los campos (no cambian). Commit: `feat(docker): RegistryAuth y StackDeploy no imprimen la contraseña ni el compose`.
- [ ] **Step 3: Repositorios (tests primero, con `sqlitetest.New` y una app sembrada por SQL).**

```go
type DeployRepository interface {
	// Create inserta d como queued (ID, CreatedAt); Images son los overrides.
	// repository.ErrNotFound si la app ya no existe (sqlite.IsForeignKey: un
	// DELETE de catalog entre la lectura de la app y este INSERT), no un 500.
	Create(ctx context.Context, d *entity.Deploy) error
	// SupersedeQueued pasa a superseded los queued de la app salvo by, con
	// superseded_by = by y finished_at = at (contrato §2.2). Devuelve sus ids,
	// para avisarles a los streams después del commit (Hub.Notify); nunca nil.
	SupersedeQueued(ctx context.Context, appID, by int64, at time.Time) ([]int64, error)
	Get(ctx context.Context, appID, id int64) (entity.Deploy, error) // ErrNotFound si no es de esa app
	GetByID(ctx context.Context, id int64) (entity.Deploy, error)
	List(ctx context.Context, appID int64) ([]entity.Deploy, error) // del más nuevo al más viejo; nunca nil
	Current(ctx context.Context, appID int64) (*entity.Deploy, error) // el succeeded más reciente, o nil
	PreviousSucceeded(ctx context.Context, appID, before int64) (*entity.Deploy, error)
	NextQueued(ctx context.Context, appID int64) (*entity.Deploy, error)
	Running(ctx context.Context, appID int64) (*entity.Deploy, error)
	// ActiveApps son las apps con un deploy queued o running (Run, contrato §1.2).
	ActiveApps(ctx context.Context) ([]int64, error)
	// Claim pasa un queued a running con started_at = at; false si ya no estaba queued.
	Claim(ctx context.Context, id int64, at time.Time) (bool, error)
	SetImages(ctx context.Context, id int64, requested, config, resolved map[string]string) error
	// AppendLog agrega lines (ya formateadas) con AppendPlan, en una transacción
	// que lee el largo en bytes y log_truncated y guarda los dos.
	AppendLog(ctx context.Context, id int64, lines []string, final bool, marker string) error
	// ReadLog devuelve desde offset hasta max bytes en líneas completas, con
	// el largo total, el estado y superseded_by. ErrOffsetBeyond si offset > largo.
	ReadLog(ctx context.Context, appID, id int64, offset, max int) (LogChunk, error)
	// Finish cierra un running (o un queued, con failed): status, error y finished_at.
	Finish(ctx context.Context, id int64, status entity.Status, errMsg *string, at time.Time) error
	// FailQueued pasa a failed los queued de la app, con el motivo como error.
	// Devuelve sus ids, como SupersedeQueued, para el Hub.Notify; nunca nil.
	FailQueued(ctx context.Context, appID int64, reason string, at time.Time) ([]int64, error)
	// ApplyRetention borra lo que no guarda la retención (contrato §2.3) y dice cuántos.
	ApplyRetention(ctx context.Context, appID int64) (int, error)
}

type LogChunk struct {
	Chunk        []byte
	Length       int
	Status       entity.Status
	SupersededBy *int64
}

type RegistryRepository interface {
	List(ctx context.Context) ([]entity.Registry, error) // por host, sin la contraseña
	Get(ctx context.Context, host string) (entity.Registry, error)
	Create(ctx context.Context, r *entity.Registry) error // ErrConflict si el host ya está
	Update(ctx context.Context, r *entity.Registry) error // reemplaza usuario y contraseña
	Delete(ctx context.Context, host string) error
	// Credentials descifra las credenciales de esos hosts (los que no tienen, no vienen).
	Credentials(ctx context.Context, hosts []string) (map[string]entity.Registry, error)
}
```

  SQL que importa:
  - `ReadLog`: `SELECT substr(CAST(log AS BLOB), ?+1, ?), length(CAST(log AS BLOB)), status, superseded_by FROM deploys WHERE id = ? AND app_id = ?` (por bytes: en un `TEXT`, `substr` cuenta caracteres); el `Chunk` se corta en el último `\n`.
  - `AppendLog`: `UPDATE deploys SET log = log || ?, log_truncated = ? WHERE id = ?`, dentro del `InTx` (si el ctx ya trae una transacción, la usa).
  - Retención (contrato §2.3): borra los de la app que no son `queued` ni `running`, no están entre los 20 de id más alto, y no son el actual ni el `succeeded` anterior al actual.
  - `Create` de registry: `INSERT … RETURNING id` con la contraseña vacía y después `UPDATE` con `Cipher.Encrypt(password, "registries:password:<id>")`, en la misma transacción.

  Tests: `TestDeployCreateAndSupersede` (dos queued → el viejo superseded con `superseded_by` y `finished_at`, sin `started_at`, y `SupersedeQueued` devuelve su id; un running no se toca; sin queued → `[]`), `TestDeployCreateAppGone` (`Create` con un `app_id` que no existe → `repository.ErrNotFound`, no el error de la FK), `TestCurrentAndPrevious` (ignora failed y superseded), `TestClaim` (solo un queued; dos `Claim` a la vez: uno gana), `TestAppendLogCap` (líneas hasta pasar 252 KiB: marca una sola vez, normales descartadas, de cierre en la reserva; reabrir el repositorio mantiene `log_truncated`), `TestReadLog` (offsets en bytes con `ñ` y `…`, solo líneas completas, máximo 64 KiB, `ErrOffsetBeyond` más allá del largo, `""` al final), `TestRetention` (25 deploys, varios failed: quedan los 20 nuevos + el actual + el anterior + queued/running; `superseded_by` de un borrado queda en NULL), `TestFailQueued` (devuelve los ids que cerró), `TestActiveApps`, `TestRegistriesRoundTrip` (la columna guarda el cifrado, no la contraseña; descifra con su AAD; `Credentials` solo de los hosts pedidos; `ErrConflict` con host repetido; `Update`; `Delete`; `ErrNotFound`).
  Commit: `feat(deploy): repositorios SQLite de deploys y registries, con el log acotado y la retención`.
- [ ] **Step 4: El log persistido y el aviso a los streams** (`logwriter.go`, `hub.go`; tests primero):

```go
// Hub avisa a los streams que un deploy cambió (líneas nuevas o estado).
// Wait devuelve un canal que se cierra con el próximo Notify de ese deploy;
// Notify lo cierra y lo olvida (sin pérdidas: quien espera se suscribe antes de leer).
type Hub struct{ /* mu, chans map[int64]chan struct{} */ }
func (h *Hub) Wait(id int64) <-chan struct{}
func (h *Hub) Notify(id int64)

// LogWriter escribe el log de un deploy: formatea y enmascara (FormatLine),
// guarda (AppendLog) y recién después avisa (Notify): lo que se ve por SSE ya
// está en ?offset (contrato §2.5).
type LogWriter struct{ /* repo, hub, clock */ }
func (w *LogWriter) For(id int64, m service.Masker) *DeployLog
func (l *DeployLog) Line(format string, args ...any) error
func (l *DeployLog) Lines(texts []string) error // una sola escritura (la salida del CLI, 50 líneas de una tarea)
func (l *DeployLog) Final(format string, args ...any) error
```

  Tests: `TestHubNotify` (un `Wait` antes de `Notify` se despierta; uno después espera el siguiente), `TestLogWriterMasksAndNotifies` (un secreto del masker no llega a la base; `Notify` después de guardar).
- [ ] **Step 5: Puertos y errores.** En `ports.go`:

```go
// Queue es la cola de deploys (la implementa el despachador de T4).
type Queue interface {
	Wake(appID int64)            // hay un queued nuevo en esa app
	Accepting(appID int64) bool  // false mientras la app se está borrando (contrato §1.1)
}
```

  En `errors.go`: `ErrNoPreviousDeploy`, `ErrAppRemoving` y `ErrOffsetBeyond` (lo devuelve `ReadLog`; el caso de uso lo traduce a `validate.Errorf("offset", "pasa el largo del log")`), más los de T2. `module.go` usa, hasta T4, una `Queue` que siempre acepta y no hace nada al despertar.
- [ ] **Step 6: Casos de uso (tests primero, con repositorios reales, `clock.NewManual` y la auditoría en una base de prueba).**
  - `CreateDeploy.Execute(ctx, dto.CreateDeploy{Project, Stage, App, Image *string, Images map[string]string}) (dto.Queued, error)`:
    1. `Apps.Get` (404);
    2. `!Queue.Accepting` → `ErrAppRemoving`;
    3. validación (contrato §2.2): los dos → `image: manda image o images, no los dos`; compose con `image` → `image: una app compose recibe images, por servicio`; cada ref con `validate.ImageRef(field, ref)` (`image` o `images.<servicio>`); un servicio que no está en la configuración actual (formulario: el slug; compose: `compose.Services`) → `images.<servicio>: el servicio no existe en la app`;
    4. el principal del ctx: sesión → `trigger: panel`, `actor_type: user`, `actor_name` = username; token → `api`, `token`, el nombre del token;
    5. `InTx`: `Create` (queued, `Images` = overrides; `ErrNotFound` si la app se borró entre medio: 404) y `SupersedeQueued`; **después del commit**, `Hub.Notify` de cada id que devolvió `SupersedeQueued` (contrato §2.6: el stream de un deploy manda `status` "cada vez que cambia", no al próximo heartbeat);
    6. auditoría `deploy.create`, target `app:<p>/<s>/<a>`, details `{"deploy": id, "trigger": t, "images": {…}}`;
    7. `Queue.Wake`. Devuelve `{id, status: "queued", status_url}`. **No llama a Docker** (contrato §2.2).
    - `CheckApp(ctx, dto.AppRef) error`: solo el paso 1 (`Apps.Get`), para que el handler responda el 404 antes de leer el cuerpo (Step 7).
  - `Rollback.Execute`: app; `Accepting`; `InTx`: `Current` y `PreviousSucceeded(current.ID)` (sin alguno → `ErrNoPreviousDeploy`), `Create` con `trigger: rollback` e `Images` = las del anterior, `SupersedeQueued`; después del commit, `Hub.Notify` de los reemplazados; auditoría con `trigger: rollback`; `Wake`. También tiene `CheckApp`.
  - **La regla del aviso**, para todo lo que cambia el estado de un deploy (aquí, en T4 y en T5): lo que pasa dentro de un `InTx` se avisa con `Hub.Notify` recién después del commit, a cada id que cambió (`SupersedeQueued` y `FailQueued` devuelven los suyos). Nunca se avisa de algo que todavía no está en la base.
  - `ListDeploys`, `GetDeploy`, `ReadLog` (64 KiB).
  - Registries: `List`, `Create`, `Update`, `Delete`, con auditoría `registry.create|update|delete`, target `registry:<host>`, details `{"username": …}` en create y update (nunca la contraseña).
  - Tests: `TestCreateDeployValidation` (tabla con los mensajes exactos), `TestCreateDeploySupersedes` (además, un `Hub.Wait(<id del viejo>)` tomado antes del `POST` se cierra, y el del nuevo no), `TestCreateDeployActor` (sesión y token), `TestCreateDeployAudit`, `TestCreateDeployRejectedWhileRemoving`, `TestRollback` (0 y 1 succeeded → 409; con 2 → las imágenes del anterior), `TestRegistryAuditNoPassword`.
- [ ] **Step 7: HTTP (tests primero, `httptestutil` con `FakeAuth` con una sesión y dos tokens: uno del proyecto y uno de otro proyecto).** Rutas, todas `AccessToken` salvo las de registries:

| Ruta | Handler |
|---|---|
| `POST /api/v1/projects/{p}/stages/{s}/apps/{a}/deploys` | crear (202, `Location` = `status_url`) |
| `GET  …/apps/{a}/deploys` | lista (`[]` si no hay) |
| `GET  …/apps/{a}/deploys/{id}` | uno (un `id` no numérico → 404) |
| `GET  …/apps/{a}/deploys/{id}/log` | polling (`offset`) |
| `POST …/apps/{a}/rollback` | rollback (202) |
| `GET/POST /api/v1/registries`, `PATCH/DELETE /api/v1/registries/{host}` | `AccessSession` |

  - El orden de los errores (contrato §2.2): app (404) → cuerpo (400) → validación (422). `utils.DecodeJSON` escribe el 400 él mismo, así que el handler **resuelve la app antes de leer el cuerpo**: `CreateDeploy.CheckApp(ctx, dto.AppRef)` (`Apps.Get`: 404) → leer y decodificar el cuerpo (400) → `Execute` (que vuelve a leer la app dentro: es barato y cubre un borrado entre medio). El rollback igual (`Rollback.CheckApp`).
  - El cuerpo de `POST …/deploys` es opcional: el handler lee hasta 1 MiB y, si está vacío (o solo espacios), es `{}`; si no, `utils.DecodeJSON` sobre esos bytes (con `r.Body` vuelto a armar con `io.NopCloser(bytes.NewReader(b))`).
  - **El rollback igual** (contrato §2.7: "sin cuerpo, o `{}`"): `DecodeJSON` solo **no** sirve, porque con un cuerpo vacío responde `400` ("el cuerpo está vacío o incompleto"), y el panel (`deployHttpDataSource.rollback`, rama `fase-4-deploy-ingress`) hace el `POST` **sin cuerpo**. El handler lee hasta 1 MiB; vacío o solo espacios → sigue; si no, `DecodeJSON` en un `struct{}` sobre el `r.Body` vuelto a armar (`{}` pasa; un campo, `[]` o JSON roto → `400`). Un helper común (`readOptionalBody`) sirve a los dos.
  - `offset` (contrato §2.5): ausente es 0; no entero o negativo → `422` `offset: tiene que ser un entero ≥ 0`; más allá del largo → `422` `offset: pasa el largo del log`.
  - Las respuestas (`res.go`) tienen **todos** los campos de contrato §2.1, con `null` cuando corresponde, fechas truncadas al milisegundo e `images` nunca `null`. El registry: `{host, username, created_at, updated_at}`, nunca la contraseña.
  - `mapper_errors.go`: `ErrNoPreviousDeploy` → `409 no_previous_deploy` ("no hay un deploy exitoso anterior al actual"); `ErrAppRemoving` → `409 conflict` ("la app se está borrando").
  - Tests: `TestCreateDeployHTTP` (202, `Location`, cuerpo; vacío y `{}` iguales; `400` con un campo desconocido o `[]`; los 422 con su texto; 404 de una app que no existe, también con un cuerpo roto (`{"x":`) o con `[]`: el 404 gana al 400), `TestRollbackHTTPOrder` (app que no existe con cuerpo `[]` → 404), `TestDeploysListAndGet` (forma de cada campo; un id de otra app → 404), `TestLogPolling` (los casos de `offset`, `chunk`, `next_offset`, `status`, `superseded_by`; fin con `chunk: ""`), `TestRollbackHTTP` (409 `no_previous_deploy`; con dos `succeeded`: **sin cuerpo** → 202, como lo manda el panel; `{}` → 202; `"  \n"` → 202; `{"x":1}` y `[]` → 400 `invalid_json`), `TestTokenScopes` (el token del proyecto crea deploys y lee; el de otro proyecto → 403 `out_of_scope`; un token en `/registries` → 403 `token_not_allowed`), `TestRegistriesHTTP` (201, 200, 204, 404, 409, los 422 de §2.10 con `host: para Docker Hub usa docker.io`, `registry.example.com%3A5000` en la ruta, ordenados por host, sin contraseña en ninguna respuesta).
  - `module_test.go`: `TestModuleRoutesAndStubs` deja de esperar `no_route` en `/deploys`, `/deploys/1`, `/deploys/1/log` y `/registries`; siguen sin existir `…/status`, `…/logs` y `…/log/stream` (T5).
- [ ] **Step 8: El registry inicial de install** (decisión 19; el contrato ya lo dice en §1.1, §1.2 y §2.10; la Fase 5 documenta la variable, el secreto y el JSON en §5.2 y §5.3; tests primero en cada parte).

  **8a. `src/settings`** (lo mínimo; copia cómo se lee `MUELLE_INIT_CLOUDFLARE_TOKEN_FILE`):

```go
// En Init, un campo nuevo:
	// Registry es el registry inicial de install: el contenido de
	// MUELLE_INIT_REGISTRY_FILE (un secreto Swarm). nil si no está. deploy lo
	// guarda solo en el primer arranque de la base (SeedInitRegistry).
	Registry *InitRegistry

// InitRegistry es la credencial de un registry que install le pasa a muelle
// (las imágenes de muelle son privadas). La contraseña va en claro: String,
// GoString, LogValue y MarshalJSON la ocultan, también dentro de Init y de
// Settings.
type InitRegistry struct{ Host, Username, Password string }
```

  - `loadInit`, después del token: si `MUELLE_INIT_REGISTRY_FILE` no está vacía, lee el archivo (más de 16 KiB es un error) y lo decodifica con json/v2 estricto en un struct con `json:"host"`, `json:"username"` y `json:"password"`: `jsonv2.Unmarshal(b, &v, jsonv2.RejectUnknownMembers(true))` rechaza los campos desconocidos y repetidos y lo que sigue al objeto; un valor que no es un objeto (`[]`) también es error, y `null` deja los campos vacíos, que rechazan las reglas. Después, `validate.RegistryHost`, `RegistryUsername` y `RegistryPassword` (T2) con los campos `MUELLE_INIT_REGISTRY_FILE.host`, `.username` y `.password`. Como el token, se lee y valida **en cada arranque** (contrato §5.2): un archivo roto no deja arrancar, aunque deploy ya no lo use.
  - **Ningún error trae la contraseña ni el contenido del archivo.** El de lectura nombra la variable y la ruta. El de decodificación **no** incluye el error de json/v2, que puede citar un pedazo del archivo: `MUELLE_INIT_REGISTRY_FILE: <ruta> no es un JSON {"host", "username", "password"} (sin otros campos)`. Los de las reglas son los de `validate`, que no incluyen el valor (T2).
  - `Init.String` no cambia: el `%+v` de su copia sin métodos llama a `InitRegistry.String` en el campo (fmt usa los métodos de un campo exportado, también de un puntero), y `%#v` a `GoString`. `InitRegistry.String` → `{Host:ghcr.io Username:bot Password:***}`; `MarshalJSON` serializa un struct anónimo con `"password": "***"` (`""` si está vacía).
  - Tests (`settings_test.go`; `clearEnv` suma la variable):
    - `TestLoadInitRegistry`: un archivo válido → `Init.Registry` con los tres valores; sin la variable → `nil`;
    - en `TestLoadRejectsBadInit`, con la contraseña `s3cr3t-pass`: un archivo que no existe, un JSON sin cerrar que la trae (`{"host":"ghcr.io","username":"bot","password":"s3cr3t-pass"`), un campo de más, `[]`, `host` `GHCR.io` y `index.docker.io`, un usuario vacío y una contraseña con `\n`. Cada uno da un error que nombra `MUELLE_INIT_REGISTRY_FILE` y **nunca** contiene `s3cr3t-pass`;
    - `TestInitDoesNotPrintToken` suma `Registry` con esa contraseña al `Init` de prueba: `%v`, `%+v`, `%#v`, `fmt.Sprint` y `slog` (JSON y texto) de `Init`, `&Init`, `Settings` y `&Settings` no la contienen; aparte (el token no tiene `MarshalJSON`, y ese test lo busca en todas las salidas), `json.Marshal(s.Init.Registry)` y `json.Marshal(s)` tampoco.

  **8b. deploy:**
  - `app/dto/registry.go`: `InitRegistry{Host, Username, Password}`, con `String`, `GoString`, `LogValue` y `MarshalJSON` que ocultan la contraseña (con su test, como `TestRegistryNeverPrintsPassword`). En `module.go`, `type InitRegistry = dto.InitRegistry` (como `admin.InitialSettings`) y `Deps.InitRegistry *InitRegistry`, opcional: `nil` es "no hay nada que sembrar".
  - La clave: `SettingInitRegistryApplied = "init_registry_applied"` en `domain/entity/registry.go` (solo la usa deploy; contrato §1.1). El puerto, en `ports.go`: `Marks interface{ MarkOnce(ctx context.Context, key string) (bool, error) }`.
  - `infra/out/service/settings.go`: `NewDeploySettings(db)` implementa `Marks` con `INSERT INTO settings (key, value, encrypted) VALUES (?, 'true', 0) ON CONFLICT (key) DO NOTHING` y `RowsAffected() == 1`, con la transacción del ctx (`db.Conn(ctx)`). Test: el primero inserta, el segundo no. T4 suma aquí `manager_node_id`.
  - `app/usecases/seed_registry.go`:

```go
// SeedInitRegistry guarda el registry inicial de install solo en el primer
// arranque de la base (decisión 19, contrato §2.10). En un InTx:
// Marks.MarkOnce(SettingInitRegistryApplied), siempre, haya o no un registry
// inicial. Si la marca ya estaba, no hace nada más: un registry que el admin
// borró en el panel no vuelve. Si la insertó y r no es nil, lo crea como
// Registries.Create (cifrado con registries:password:<id>), con la auditoría
// registry.create, target registry:<host>, details {"username": …, "source":
// "install"} y actor system (el ctx del arranque no trae un principal). Si
// ese host ya tiene un registry (ErrConflict), no lo toca. r ya viene
// validado (settings.Load). Si algo falla, el InTx deshace la marca y el
// próximo arranque reintenta. Sus errores son de la base: la contraseña va
// cifrada antes del UPDATE, así que nunca la traen.
func (u *SeedInitRegistry) Execute(ctx context.Context, r *dto.InitRegistry) error
```

  - `module.go`: `func (m *Module) SeedInitRegistry(ctx context.Context) error`, con `Deps.InitRegistry`.
  - Tests (`seed_registry_test.go`, con la base real):
    - primer arranque con un registry → se crea (la columna tiene el cifrado, `Credentials` lo descifra), la marca queda, y hay un `registry.create` con actor `system` y sin la contraseña en `details`;
    - **después de un `Registries.Delete` (lo que hace el panel), otro `Execute` con el mismo registry no lo vuelve a crear**;
    - un segundo `Execute` con otra contraseña para el mismo host no cambia nada;
    - primer arranque **sin** registry → la marca igual queda, y un `Execute` posterior con uno no lo crea (una reinstalación sobre la misma base);
    - un `Registries` del test que envuelve al real y falla en `Create` → `Execute` devuelve el error, la marca no quedó, y el error no trae la contraseña.

  **8c. `container.go`** (solo estos dos lugares: la firma de `build` y la `Config` del servidor son de la Fase 5):

```go
	var initRegistry *deploy.InitRegistry
	if r := s.Init.Registry; r != nil {
		initRegistry = &deploy.InitRegistry{Host: r.Host, Username: r.Username, Password: r.Password}
	}
	c.Deploy = deploy.New(deploy.Deps{
		DB: db, Cipher: ciph, Clock: clk, Audit: rec, Log: log, Docker: engine,
		Env: c.Secrets, Ingress: c.Ingress, InitRegistry: initRegistry,
	})
	…
	// Justo después de ApplyStartupSettings, y por eso después de
	// VerifyMasterKey: nunca se cifra con una clave que no se verificó.
	if err := c.Deploy.SeedInitRegistry(ctx); err != nil {
		return nil, fmt.Errorf("container: guardando el registry inicial: %w", err)
	}
```

  Test en `src/shared/infra/container/deploy_test.go` (archivo nuevo; T5 le suma los contratos): `TestInitRegistrySeededOnce`, con `testSettings(t)` más `s.Init.Registry`: el primer `New` deja el registry (`SELECT` por `c.DB`: está, y `password` no es la contraseña en claro); se borra como lo haría el panel (el caso de uso `Delete` de registries o un `DELETE` por SQL); `Close` y otro `New` sobre el mismo `DataDir` con el mismo `s` → la tabla sigue vacía.

- [ ] **Step 9: Verificar y commits:**
  1. `feat(deploy): registries por HTTP (contrato §2.10)`
  2. `feat(deploy): encolar deploys y rollbacks y leerlos por HTTP (contrato §2.1–§2.5 y §2.7)`
  3. `feat(settings): MUELLE_INIT_REGISTRY_FILE, el registry inicial de install, con la contraseña oculta`
  4. `feat(deploy): el registry inicial de install se guarda solo en el primer arranque de la base`

**Integración en T3:** ninguna (no usa Docker real).

---

### Task T4: El worker: los pasos de §7, la cola por app, la reconciliación y el borrado

**Files:**
- Modify: `src/internal/deploy/domain/service/ports.go` (`Images`, `Credential`, `Cluster`, `ManagerNode`; `Stacks.Deploy`)
- Create: `src/internal/deploy/infra/out/service/docker.go` y tests; Modify: `stacks.go`, `settings.go` (de T3: suma `manager_node_id`)
- Create: `src/internal/deploy/app/usecases/{run_deploy,resume,dispatcher,manager_node}.go`, `timing.go`, y sus tests con `swarmsim_test.go`
- Modify: `src/internal/deploy/app/usecases/remove_app.go`, `src/internal/deploy/module.go`, `module_test.go`
- Create: `src/internal/deploy/integration_test.go` y `integration_main_test.go` (los dos `//go:build integration`)

- [ ] **Step 1: Puertos y adaptadores (tests primero, con `dockertest`).**

```go
// Credential es la credencial de un registry para el paso 2 y el 5; se
// imprime sin la contraseña (String, GoString, LogValue, MarshalJSON).
type Credential struct{ Host, Username, Password string }

type Images interface {
	// Digest resuelve ref en el registry. ErrImageNotFound, ErrImageDenied o
	// un error que envuelve entity.ErrDockerUnavailable.
	Digest(ctx context.Context, ref string, cred *Credential) (string, error)
}

// Stacks (existente) suma:
	// Deploy ejecuta docker stack deploy y devuelve su salida (para el log, sin enmascarar).
	Deploy(ctx context.Context, stack string, compose []byte, creds []Credential) (string, error)

type Cluster interface {
	AppServices(ctx context.Context, project, stage, app string) ([]SwarmService, error) // naming.AppLabels
	Service(ctx context.Context, name string) (SwarmService, error)                     // ErrSwarmNotFound
	Tasks(ctx context.Context, serviceID string) ([]SwarmTask, error)                   // de la más nueva a la más vieja
	TaskLogs(ctx context.Context, taskID string, tail int) ([]LogLine, error)          // sin follow
	LocalNodeID(ctx context.Context) (string, error)                                   // Info().NodeID
}

// ManagerNode es settings.manager_node_id (contrato §1.1; la clave es entity.SettingManagerNodeID).
type ManagerNode interface {
	Get(ctx context.Context) (string, error)              // "" si no está
	SetIfMissing(ctx context.Context, id string) (string, error) // devuelve el que quedó
}
```

  `ErrImageNotFound`, `ErrImageDenied`, `ErrSwarmNotFound` y `ErrAppRemoved` (la causa con que `RemoveApp` cancela un worker) van en `errors.go`. `infra/out/service/docker.go` implementa `Images`, `Cluster` y `Stacks.Deploy` con `docker.Engine`: traduce `docker.ErrNotFound` → `ErrImageNotFound` (en `Digest`) o `ErrSwarmNotFound`, `docker.ErrDenied` → `ErrImageDenied`, y deja pasar `ErrUnavailable` (es `entity.ErrDockerUnavailable`). `settings.go` (el adaptador de T3, que ya tiene `MarkOnce`) suma `ManagerNode`: `SELECT value FROM settings WHERE key = ?` y `INSERT … ON CONFLICT(key) DO NOTHING` + lectura, con `encrypted = 0`. Tests: los mapeos de error con `FailNext`/`DenyImage`, `AppServices` filtra por etiquetas, `TaskLogs` con `tail`, `SetIfMissing` no pisa.
- [ ] **Step 2: El simulador de Swarm para los tests** (`swarmsim_test.go`, solo test): `simulate(f *dockertest.Fake, o simOpts)` pone `f.OnDeployStack`, que lee el JSON del compose y, por cada servicio, hace `f.PutStackService(stack, docker.Service{…})` con el nombre `<stack>_<servicio>`, `Labels` = sus `deploy.labels`, `Image`, `Replicas`, `RunningTasks`/`DesiredTasks`, `CreatedAt` (el anterior si ya existía) y `Update`:
  - si el servicio ya existía y su plantilla cambió (imagen, env, command… cualquier cosa fuera de `deploy.labels`): `{State: o.state (completed por defecto), StartedAt: ahora}`;
  - si solo cambió `deploy.labels`: el `Update` anterior, sin tocar las tareas;
  - si es nuevo: `nil`.

  `simOpts` permite: `state` (`rollback_completed` con `Message`), `running` (cuántas corren), una tarea fallida con `Err` y N líneas de log (`AppendTaskLogs`), un `block chan struct{}` para retener el deploy, `delay` (cuánto tarda el CLI simulado) y `noUpdate` (un servicio cambiado queda sin `UpdateStatus`, como en Swarm antes de que empiece el rollout).

  **Dónde se retiene un deploy:** `block` lo retiene **dentro** de `OnDeployStack`, que no recibe el ctx: cancelar el worker no lo suelta (`Stop` esperaría sus 15 s) y, al soltarlo, el stack igual queda aplicado. Sirve solo para los tests que no cancelan (`TestDispatcherOneAtATimePerApp`, `TestDispatcherSupersede`, que cierran `block` al final). Los que cancelan (`RemoveApp`, el apagado) retienen el deploy en el **paso 6**: `running: 0` y un `deploy_timeout_s` largo, así la convergencia espera y el ctx sí la corta.
- [ ] **Step 3: `RunDeploy` (tests primero).**

```go
// Timing son los plazos del deploy (decisión 13); los tests los acortan.
type Timing struct {
	Poll, Settle, ResolveTimeout, NetworkTimeout, StackTimeout, RoutesTimeout, RetryEvery time.Duration
}

func DefaultTiming() Timing // 2s, 5s, 60s, 30s, 2m, 30s, 15s

// RunDeploy corre un deploy ya reclamado (running) hasta cerrarlo (§7).
// Gate es el despachador (service.Queue): se fija después de armarlo.
type RunDeploy struct { /* Apps, Deploys, Registries, Env, Domains, Ingress, Images, Stacks, Cluster, Manager, Gate, Log *LogWriter, Hub, Tx, Clock, Timing */ }

// Execute corre los pasos 2 a 8. Si ctx se cancela por el apagado, vuelve sin
// cerrar el deploy (queda running, contrato §1.2); si se cancela con la causa
// ErrAppRemoved, lo cierra failed con "la app se borró durante el deploy".
func (u *RunDeploy) Execute(ctx context.Context, d entity.Deploy) error
```

  El orden (contrato §2.11). Cada `fail(motivo)` hace, en un `InTx`, `Deploys.AppendLog` de la línea `failed: <motivo>` (formateada con `FormatLine`, `final = true`) y `Finish(failed, ErrorLine(m, motivo))`; **después del commit**, `Hub.Notify`; y al final la retención. Así un stream nunca ve el estado final antes que la última línea, ni un aviso de algo que todavía no está en la base (el `LogWriter`, que avisa al escribir, se usa solo fuera de una transacción):
  1. `app := Apps.GetByID`; `vars, secrets := Env.ResolveEnv` (Docker no se toca si falla); `m := NewMasker(secrets)`; `Line("deploy #%d de %s/%s/%s (%s · %s)")`.
  2. `Line("resolviendo imágenes")`; `cfg := ReadConfig(ctx, app, vars)` (error → `fail("render: " + msg)`); los overrides de un rollback que no están en `cfg` se omiten con una línea; `requested := ChooseImages(cfg.Images, d.Images, Deploys.Current(app))`; credenciales con `Registries.Credentials(hosts)`; por servicio, en orden, **siempre** `Images.Digest(ref, cred)` con `ResolveTimeout`, también si la ref ya trae digest (decisión 1: verifica que exista y que la credencial llegue). Los errores, los mismos para una ref con tag o con digest: no existe → `la imagen <ref> no existe en el registry`; sin acceso → `sin acceso a <ref>: revisa la credencial de <host> en Ajustes → Registries`; Docker caído → `Docker no responde`. Con una ref fijada (`PinnedDigest(ref) != ""`) se conserva ese digest, y si `Digest` devolvió otro → `fail("el registry devolvió otro digest para <ref>")` (no debería pasar: Docker la busca por su digest). Después, `Line("  %s: %s → %s", svc, ref, digest[:19]+"…")`; `SetImages(requested, cfg.Images, resolved)`.
  3. El nodo del manager (contrato §1.2: si `Run` no pudo guardarlo, "lo reintenta en el primer deploy"): en **todo** deploy, si `Manager.Get` da `""`, `Cluster.LocalNodeID` y `Manager.SetIfMissing` (gana el que quedó). En una app sin volúmenes, un error ahí es solo una línea (`advertencia: no se pudo guardar el nodo del manager: Docker no responde`) y se sigue; con volúmenes es `fail`. Si `cfg.HasVolumes` y `app.StorageNodeID == ""`: `Apps.SetStorageNode` con ese id (gana el que quedó). `Render(cfg, …)` con `Domains.ByApp` (error → `fail("render: …")`); `Line("render: 1 servicio")` o `"render: N servicios"`.
  4. `Ingress.EnsureStageNetwork` con `NetworkTimeout` (error → `fail`); `Line("red %s lista")`.
  5. Si `!Gate.Accepting(app.ID)` (la app se empezó a borrar mientras corrían los pasos 2 a 4; decisión 8) → `fail("la app se borró durante el deploy")`, sin tocar Swarm. `appliedAt := Clock.Now()` (justo antes del CLI: el umbral de "un `UpdateStatus` de este deploy"); `Line("docker stack deploy %s")`; `out, err := Stacks.Deploy(…, credenciales de rendered.Hosts)` con `StackTimeout`; `settleFrom := Clock.Now()` (cuando el CLI volvió: desde aquí corre el margen, decisión 6); `Lines(SplitOutput(m.Mask(out)))`: la salida **entera** se enmascara antes de partirla en líneas, porque un secreto de varias líneas cuyas líneas miden menos de 8 bytes solo se reconoce entero, con sus `\n` (`FormatLine` vuelve a enmascarar cada línea, y no hace daño); error → `fail("docker stack deploy falló: …")`.
  6. `Line("esperando convergencia (hasta %d s)")`; cada `Poll`, `Cluster.AppServices` y `Evaluate(expected, …, appliedAt, settleFrom, Clock.Now(), Settle)`; una línea cada vez que cambia el progreso de un servicio; Docker caído: una línea "Docker no responde; reintentando" y se sigue. Falla → `Status.Err` de la última tarea fallida del servicio (`Tasks`: la primera `failed`/`rejected`, o con `Err`) y sus últimas 50 líneas (`TaskLogs` → `JoinPieces` → `Lines` con `"    | "`), y `fail("<servicio>: Swarm hizo rollback: <Message>")` (o `paused`…). Timeout (`app.DeployTimeoutS`) → por cada servicio sin converger, su progreso y el `Err` de su última tarea fallida, y `fail("timeout esperando convergencia")`.
  7. `Ingress.SyncRoutes` con `RoutesTimeout`: `Line("rutas de Caddy actualizadas")` o `Line("advertencia: no se pudo actualizar Caddy: %v")` (también con `ErrNotImplemented`).
  8. `InTx`: `Deploys.AppendLog` de `succeeded en N s` (`final = true`), `Finish(succeeded)` y `Apps.MarkDeployed(app.ID, *d.StartedAt)`; después del commit, `Hub.Notify`; al final, `ApplyRetention`.

  Las escrituras a la base después de una cancelación por `ErrAppRemoved` usan `context.WithoutCancel(ctx)`.

  Tests (con el simulador, `fakeEnv` con un secreto, `fakeIngress` que cuenta llamadas y cuyo `SyncRoutes` devuelve `entity.ErrNotImplemented`, `clock` del sistema y `Timing` corto; las apps sembradas por SQL con `deploy_timeout_s` chico):
  - `TestRunDeployFormApp`: `succeeded`; el log trae las líneas de contrato §2.11 en orden, con la advertencia de Caddy; `images` = `x:1@sha256:…`; `requested_images` y `config_images` guardados; `last_deployed_at` = `started_at`; el compose de `StackDeploys()` lleva `muelle.deploy=<id>` en `deploy.labels` y no en `labels`; ninguna columna de la base contiene el compose.
  - `TestRunDeployMasks`: un secreto en la salida del CLI (`OnDeployStack` que falla con un mensaje que lo trae) y en el error → `***` en el log y en `error`. Otro caso con un secreto **de varias líneas, cada una de menos de 8 bytes** (`"abc\ndefg\nhij"`, 12 bytes) que aparece entero en la salida del CLI: el log no trae `defg` (con `SplitOutput` antes de `Mask` saldría a la vista).
  - `TestRunDeploySlowCLI` (decisión 6): un segundo deploy con otra imagen y un `OnDeployStack` que tarda el doble de `Settle` y deja el servicio cambiado **sin** `UpdateStatus` (como Swarm justo antes de empezar el rollout; `simOpts` suma `delay` y `noUpdate`). El test anota cuándo volvió `OnDeployStack` (`t1`): el deploy cierra `succeeded` con `finished_at ≥ t1 + Settle`, nunca antes.
  - `TestRunDeployImageErrors`: no existe, sin acceso, Docker caído → `failed` con su motivo; `DeployStack` nunca se llamó; `images` siguen siendo los overrides. Cada caso, con una ref con tag y con una ref con digest (`x@sha256:…` sin `SetDigest`, y con `DenyImage`): los mismos mensajes.
  - `TestRunDeployPinnedDigestVerified`: un override `x:1@sha256:A` con `SetDigest("x:1@sha256:A", "sha256:A")` → `DistributionCalls()` trae esa ref (con la credencial de su host) y `images` = `x:1@sha256:A`; un digest fijado que ya no existe en el registry (sin `SetDigest`) → `failed` "la imagen … no existe en el registry" y `DeployStack` nunca se llamó; `SetDigest` con otro digest → `failed` "el registry devolvió otro digest…".
  - `TestRunDeployStopsIfRemoving`: con un `Gate` que deja de aceptar la app después del paso 4 → `failed` "la app se borró durante el deploy" y `DeployStack` nunca se llamó.
  - `TestRunDeployStoresManagerNode`: sin `manager_node_id` en `settings` y una app **sin** volúmenes → `succeeded` y `settings.manager_node_id = node-1`; con `FailNext` en el `Info` de ese paso → `succeeded` con la advertencia en el log.
  - `TestRunDeployCredentialsByExactHost`: con un registry `ghcr.io`, `DistributionCalls()` muestra la credencial para `ghcr.io/x` y anónimo para `nginx`; uno de `registry.example.com:5000` no sirve para `registry.example.com/x`; `DeployStack` recibe solo las credenciales de los hosts del compose.
  - `TestRunDeployRenderAndNetworkErrors`: variable sin definir y `EnsureStageNetwork` con error → `failed`, Swarm sin tocar.
  - `TestRunDeployStackError`: `FailNext("DeployStack", …)` → `failed` con la salida en el log.
  - `TestRunDeployRollbackDetected`: `rollback_completed` + tarea con `Err` y 60 líneas de log → `failed`, el log trae el `Err` y exactamente las últimas 50.
  - `TestRunDeployTimeout`: ninguna réplica corre → `failed` "timeout esperando convergencia", con el progreso del servicio.
  - `TestRunDeployUnchangedRedeploy`: un segundo deploy sin cambios → `succeeded` después de `Settle`, sin `UpdateStatus` nuevo y con las mismas tareas.
  - `TestRunDeployStorageNode`: formulario con volumen → `storage_node_id` = `manager_node_id` (o el del nodo local) y `node.id == …` en el compose.
  - `TestRollbackFlow`: D1 y D2 con `x:staging` y `SetDigest` distinto en cada uno (`A` y `B`). Primero, sin `SetDigest` de la ref fijada (el registry ya no tiene `A`), el rollback R1 → `failed` "la imagen x:staging@sha256:<A> no existe en el registry" y `StackDeploys()` sigue con dos. Después, `SetDigest("x:staging@sha256:<A>", "sha256:<A>")` y otro rollback R2 (el destino sigue siendo D1: R1 no cuenta) → `succeeded` con `x:staging@sha256:<A>`, y `DistributionCalls()` trae esa ref; un deploy sin cuerpo después → el mismo (contrato §2.2), aunque el tag ahora resuelva a otro digest.
- [ ] **Step 4: El despachador y la reconciliación (tests primero).**

```go
// Dispatcher es la cola por app (§7 paso 1, contrato §1.2): un worker por app
// con trabajo, un deploy a la vez por app, apps distintas en paralelo.
// Implementa service.Queue.
type Dispatcher struct{ /* mu, workers, blocked map[int64]time.Time, pending, started, ctx, wg */ }

func (d *Dispatcher) Start(ctx context.Context, apps []int64) // los de ActiveApps más los Wake previos
// Wake: antes de Start, solo lo anota. Con la app bloqueada no arranca un
// worker: marca failed sus queued ("la app se borró"; decisión 8) y avisa
// (Hub.Notify) a cada id que devolvió FailQueued, en una goroutine del wg con
// el ctx de Start.
func (d *Dispatcher) Wake(appID int64)
func (d *Dispatcher) Accepting(appID int64) bool               // false mientras está bloqueada
func (d *Dispatcher) Block(appID int64, dur time.Duration)
func (d *Dispatcher) Unblock(appID int64)
// Stop cancela el worker de la app con la causa ErrAppRemoved y espera a que
// termine, a lo más 15 s: si no terminó, devuelve ErrWorkerBusy (el worker
// sigue cancelado y cierra solo; Stop no lo abandona en el mapa).
func (d *Dispatcher) Stop(ctx context.Context, appID int64) error
func (d *Dispatcher) Wait()                                    // después de cancelar el ctx de Start
```

  El worker: `for { si la app está bloqueada → FailQueued(app, "la app se borró"), Hub.Notify de sus ids, y termina; si hay un running (Running) → Resume; si no, NextQueued → Claim → RunDeploy; si no hay nada y no hubo Wake mientras trabajaba → termina }`. La mirada a `blocked` va **antes de cada** `Claim` y de cada `Resume`, bajo el mismo `mu` que `Block`, así un `queued` que un `POST` insertó después del `Block` nunca se reclama (decisión 8).

  `Resume` (contrato §1.2): `Line("muelle se reinició durante el deploy")` (una vez); sin `requested_images` → `failed` "muelle se reinició antes de aplicar el deploy; Swarm no cambió". Si no, con los servicios esperados (las claves de `requested_images`, con `naming.Service`) y `Cluster.AppServices`:
  1. **Primero, un rollback de Swarm:** si algún servicio esperado tiene un `UpdateStatus` en `rollback_started`, `rollback_completed`, `rollback_paused` o `paused` con `StartedAt ≥ started_at − 2 s` (es de este deploy), se cierra como la falla del paso 6 (el `Err` y las 50 líneas de su última tarea fallida; `failed` "<servicio>: Swarm hizo rollback: <Message>"). Un rollback de Swarm vuelve a la `PreviousSpec`, etiquetas incluidas: si muelle se reinició mientras Swarm volvía atrás, los servicios llevan otra vez el `muelle.deploy` anterior, y sin esta mirada `AppliedState` diría `AppliedNone` y el deploy se cerraría con "Swarm no cambió", que es falso.
  2. Si no, `AppliedState`: `AppliedNone` → `failed` "muelle se reinició antes de aplicar el deploy; Swarm no cambió"; `AppliedPartial` → `failed` "muelle se reinició a mitad de docker stack deploy"; `AppliedAll` → el paso 6 con `appliedAt = started_at`, `settleFrom` = la hora en que `Resume` empieza a esperar (no se sabe cuándo volvió el CLI cortado: contar desde ahí es lo conservador) y el resto como un deploy normal.
  3. Docker caído (en cualquiera de las dos lecturas) → queda `running`, el worker espera `RetryEvery` y reintenta.

  **Qué registra el despachador en `slog`** (decisión 18): `deploy cerrado` con `app` (el id), `deploy` (el id) y `status`; si falló, `error` = el `deploys.error` guardado (ya pasó por `ErrorLine`: enmascarado). Un error que devuelven `Execute`, `Resume` o la base se registra solo como `deploy: error interno` con `app` y `deploy`, **nunca** con `err.Error()` ni con la salida del CLI: pueden traer valores de variables.

  `manager_node.go`: `StoreManagerNode(ctx)` guarda `LocalNodeID` si falta; con Docker caído registra una advertencia y sigue, y el próximo deploy lo reintenta (Step 3, paso 3).

  `module.go`: `Run(ctx)` = `StoreManagerNode` → `ActiveApps` → `Dispatcher.Start` → `<-ctx.Done()` → `Dispatcher.Wait()`.

  Tests: `TestDispatcherOneAtATimePerApp` (dos apps a la vez sí, la misma app nunca: con `block`), `TestDispatcherSupersede` (tres `POST` mientras uno corre: corre solo el último), `TestDispatcherWakeBeforeStart`, `TestDispatcherBlockedApp` (con la app bloqueada, un `queued` insertado por el repositorio, como lo haría un `CreateDeploy` que pasó `Accepting` antes del `Block`, y un `Wake` → el deploy queda `failed` "la app se borró", nunca `running`, y `StackDeploys()` está vacío; después de `Unblock`, un deploy nuevo sí corre), `TestRunShutdownLeavesRunning` (cancelar `Run` durante el paso 6: queda `running`, `Run` vuelve; otro módulo sobre la misma base con los servicios ya etiquetados lo cierra `succeeded` con "muelle se reinició durante el deploy"), `TestResumeNeverApplied`, `TestResumePartial`, `TestResumeAfterSwarmRollback` (un `running` cuyos servicios llevan el `muelle.deploy` **anterior** y un `UpdateStatus` `rollback_completed` con `StartedAt` después de su `started_at` → `failed` "…Swarm hizo rollback…", no "Swarm no cambió"; con un `rollback_completed` de **antes** de `started_at` → sí "Swarm no cambió"), `TestResumeDockerDown` (queda `running` y lo cierra cuando `SetUnavailable(false)`), `TestManagerNodeStored` (después de `Run`, `settings.manager_node_id = node-1`; con Docker caído no se guarda y `Run` igual corre), `TestRetentionAfterClose`, `TestDispatcherNeverLogsSecrets` (un `slog.NewJSONHandler` sobre un buffer; un deploy que falla en el render con `replicas: ${N}` y `N` secreta de valor `no-es-un-numero-secreto`, y otro con `OnDeployStack` que falla con un mensaje que trae el secreto → el buffer tiene `app`, `deploy` y `status`, y nunca el secreto).
- [ ] **Step 5: `RemoveApp` (test primero).** `RemoveApp.Execute`: `Apps.Get` (si no existe, solo el `stack rm`); `Block(app.ID, 2*time.Minute)`; `Stop`; `FailQueued(app.ID, "la app se borró")` y `Hub.Notify` de sus ids; `Stacks.Remove` (si falla: `Unblock` y devuelve el error). Un `queued` que llega después de `FailQueued` lo cierra el despachador (Step 4, decisión 8).
  - **Si `Stop` vence sus 15 s** (`ErrWorkerBusy`: el worker sigue vivo, por ejemplo en una escritura a la base), `RemoveApp` **no sigue**: `Unblock` y devuelve el error, sin `FailQueued` ni `stack rm`. Seguir con un worker vivo podría dejar un stack huérfano (el worker aplicaría el stack después del `stack rm`). La app no se borra (catalog no borra la fila si `RemoveApp` falla) y el usuario reintenta. `ErrWorkerBusy` va en `errors.go` y en `mapper_errors.go`: `409 conflict` "un deploy de la app todavía no se detiene; vuelve a intentar en unos segundos" (el `Kit` es uno solo, así que el `DELETE` de catalog lo responde igual). El deploy de ese worker cierra después como cualquiera cancelado con `ErrAppRemoved` ("la app se borró durante el deploy"), aunque al final la app no se borró: es raro y se acepta.
  - `TestRemoveAppStopsWorker`: con un deploy retenido en el **paso 6** (`running: 0`, no con `block`: ver Step 2) y otro `queued`, `RemoveApp` deja el primero `failed` ("la app se borró durante el deploy") y el segundo `failed`; `POST …/deploys` → 409 `conflict`; con `FailNext("RemoveStack", docker.ErrUnavailable)`, vuelve a aceptar deploys enseguida.
  - `TestRemoveAppRaceWithCreate` (el orden que deja un stack huérfano sin la decisión 8): un `CreateDeploy` que pasó `Accepting` y todavía no insertó (el test lo parte en dos: primero `RemoveApp` completo, con su `Block`, `Stop`, `FailQueued` y `stack rm`; después el `InTx` del `Create` y el `Wake`) → el deploy termina `failed` "la app se borró" y `StackDeploys()` no tiene ningún deploy de esa app después del `RemoveStack` (`Calls()` lo muestra en orden).
  - `TestRemoveAppStopTimeout`: con un `Dispatcher` cuyo plazo de `Stop` el test acorta (un campo, 15 s por defecto) y un worker retenido donde el ctx no llega (`block` en `OnDeployStack`, Step 2) → `RemoveApp` devuelve `ErrWorkerBusy`, la app vuelve a aceptar deploys (`Accepting` true), `RemovedStacks()` está vacío y el `queued` de la app sigue `queued`. Al soltar `block`, el worker termina.
  - Los tests de `TestRemoveApp` y `TestRemoveAppDockerDown` existentes siguen pasando.
- [ ] **Step 6: Integración con Swarm real** (`integration_test.go`, `//go:build integration`, paquete `deploy`):
  - **Helpers:**
    - `itClient(t)`: el cliente real, o `t.Skip` si no hay un Swarm activo con este nodo como manager;
    - `requireImage(t, ref)`: `t.Skip` si falta `nginx:alpine` o `alpine/socat:latest`, **o si no tiene `RepoDigests`** (una imagen construida o cargada con `docker load` no tiene digest de registry, y `localDigests` no tendría qué devolver);
    - `itProject(t)`: `it2` + 4 hex;
    - `itModule(t, …)`: la base (`sqlitetest`), el proyecto, el stage `staging` y la app sembrados por SQL, un `fakeEnv`, un `itIngress` que crea la red `naming.Network(p, "staging")` overlay attachable con `muelle.test=it2` y cuyo `SyncRoutes` no hace nada, y un `localDigests` que envuelve al cliente real y en `DistributionDigest` devuelve el digest de `InspectImage(ref).RepoDigests` (así no sale a internet, y como `--resolve-image=never` deja la ref con digest, Swarm no baja la imagen porque ya está en el nodo). Una ref que ya trae digest (un redeploy, un rollback) también pasa por ahí, como en producción (decisión 1): el wrapper la busca sin el tag (`InspectImage("<repo>@sha256:…")`, la forma de `RepoDigests`) y devuelve ese mismo digest; si no está en el nodo, `docker.ErrNotFound`;
    - `t.Cleanup` (también si el test falla): `docker stack rm` de cada stack `it2…` del test, esperar hasta 90 s a que no quede ningún servicio con `muelle.project=<p>`, borrar la red (reintentando hasta 60 s mientras `ErrConflict`) y los volúmenes `<p>_…` (reintentando hasta 60 s).
    - **`TestMain` (en `integration_main_test.go`, con el mismo `//go:build integration`; el paquete no tiene otro `TestMain`):** un `-timeout` que vence hace `panic` y no corre los `t.Cleanup`, así que antes de `m.Run()` barre lo que dejó una corrida anterior. Solo toca lo que es de esta línea **y** tiene más de 30 min (más que el `-timeout` de 25 min: nunca lo de una corrida que sigue viva). La edad: la de un servicio sale de `docker.Service.CreatedAt` (`ListServices`); `docker.Network` y `docker.Volume` **no** traen la hora de creación, así que el `TestMain` la lee con el CLI, con `exec` en el test: `docker network inspect --format '{{json .Created}}' <nombre>` (con `json`: `{{.Created}}` solo sale en el formato de `time.Time.String()`, `2026-10-15 18:32:47.3 -0300 -03`) y `docker volume inspect --format '{{.CreatedAt}}' <nombre>` (ya es RFC 3339); verificado con Docker 29. Uno que no se puede leer o parsear no se toca. No se agrega un método al cliente compartido para esto:
      - los stacks cuyos servicios se llaman `it2…` y llevan `muelle.project` con valor `it2…` (los pone el render): `docker stack rm`, y esperar a que se vayan sus contenedores;
      - las redes `it2…` con la etiqueta `muelle.test=it2` (las crea `itIngress`);
      - los volúmenes `it2…` cuya `com.docker.stack.namespace` empieza con `it2`.
      Lo que encuentra lo dice con `fmt.Fprintf(os.Stderr, …)`. Si Docker no está, no hace nada (los tests se saltan solos).
    - El comentario del archivo trae el comando para limpiar a mano (el de abajo).
  - **Casos:**
    - `TestIntegrationFormAppDeploy`: `nginx:alpine`, puerto 80, healthcheck `http` en `/`, 1 réplica → `succeeded`; el servicio corre 1/1, y un redeploy sin cambios → `succeeded` con el mismo id de tarea (no reinició).
    - `TestIntegrationComposePrune`: `web` (`nginx:alpine`) + `side` (`alpine/socat` con `socat TCP-LISTEN:9000,fork SYSTEM:echo`) → `succeeded`; el mismo compose sin `side` → `succeeded` y `side` ya no existe (`--prune`).
    - `TestIntegrationFailingHealthcheckRollsBack`: v1 con healthcheck `["CMD", "wget", "-q", "-O", "/dev/null", "http://127.0.0.1/"]`, `interval: 1s`, `retries: 2`, `start_period: 1s` y `update_config: {monitor: 5s}` → `succeeded`; v2 con `/no-existe` → `failed` con el rollback detectado y el `Err` de la tarea en el log; el servicio vuelve a llevar `muelle.deploy` de v1.
    - `TestIntegrationVolumeStopFirst`: formulario con un volumen → `docker service inspect --format '{{.Spec.UpdateConfig.Order}} {{.Spec.TaskTemplate.Placement.Constraints}}'` (exec en el test) da `stop-first` y la restricción **tal como la escribe el render**, `node.id == <id>` (con espacios: Swarm guarda las restricciones textuales; si se prefiere, comparar sin espacios en los dos lados), y el volumen `<stack>_data` existe.
    - `TestIntegrationResumeAfterRestart`: arranca un deploy con un healthcheck de `interval: 5s` (así la convergencia tarda), cancela `Run` cuando el log dice "esperando convergencia", verifica que quedó `running`, y un `Run` nuevo sobre la misma base lo cierra `succeeded` con "muelle se reinició durante el deploy".

```bash
go test -tags integration -run Integration -count=1 -timeout 20m ./src/internal/deploy/
bash -c 'left=$( { docker service ls --format "{{.Name}}"; docker network ls --format "{{.Name}}"; docker volume ls --format "{{.Name}}"; } | grep "^it2" ); if [ -z "$left" ]; then echo limpio; else echo "quedó:"; echo "$left"; fi'
```

  Expected: `ok`, y `limpio` (no quedó ningún servicio, red ni volumen `it2`).

  Si quedó algo (un `-timeout`, un `kill`), se limpia a mano. Solo toca lo que empieza con `it2`:

```bash
bash -c '
docker stack ls --format "{{.Name}}" | grep "^it2" | xargs -r -n1 docker stack rm
for i in $(seq 60); do docker ps -a --format "{{.Label \"com.docker.stack.namespace\"}}" | grep -q "^it2" || break; sleep 2; done
docker network ls --format "{{.Name}}" | grep "^it2" | xargs -r docker network rm
docker volume ls --format "{{.Name}}" | grep "^it2" | xargs -r docker volume rm'
```
- [ ] **Step 7: Verificar y commits:**
  1. `feat(deploy): puertos y adaptadores de Docker para el deploy (imágenes, stacks y Swarm)`
  2. `feat(deploy): los pasos de un deploy (§7): imágenes, render, red, stack deploy, convergencia y cierre`
  3. `feat(deploy): cola por app con un worker y la reconciliación al arrancar (contrato §1.2)`
  4. `feat(deploy): borrar una app detiene su worker y no acepta deploys mientras se borra`
  5. `test(deploy): integración con Swarm real (prefijo it2)`

**Integración en T4:** el comando de arriba (solo `./src/internal/deploy/`).

---

### Task T5: Streams, estado y logs de las apps, y cierre de la fase

**Files:**
- Create: `src/cmd/http/utils/revalidate_deploy.go`, `revalidate_deploy_test.go`
- Create: `src/shared/infra/out/docker/dockertest/fake_deploy.go`, `fake_deploy_test.go`
- Modify: `src/internal/deploy/domain/service/ports.go` (`Cluster` suma `ServiceLogs`, `NodeHostname`, `ContainerHealth`, `ContainerStats`; `LogStream`, `LogOptions`, `Stats`), `infra/out/service/docker.go`
- Create: `src/internal/deploy/app/usecases/{status,app_logs}.go` y tests
- Create: `src/internal/deploy/infra/in/http/{sse,streams,status}.go` y tests
- Modify: `src/internal/deploy/module.go` (rutas, el ctx de los streams), `module_test.go`
- Modify: `src/shared/infra/container/deploy_test.go` (lo creó T3 con `TestInitRegistrySeededOnce`)
- Modify: `src/internal/deploy/integration_test.go`, `README.md` (solo las partes de deploy)

- [ ] **Step 1: `Kit.Revalidate` (contrato §0.3; test primero).**

```go
// Revalidate repite las comprobaciones de Handle con la credencial de r (la
// cookie o el Bearer con que se abrió un stream), sin escribir la respuesta:
// el alcance del token sobre {p}/{s} de la ruta (como lectura), y que la
// sesión siga completa. nil si sigue valiendo; si no, entity.ErrUnauthenticated,
// entity.ErrForbidden o el error del Authenticator.
func (k *Kit) Revalidate(r *http.Request, a Access) error
```

  Revalida con el mismo `Authenticator` de `Handle` (`AuthenticateSession` / `AuthenticateToken`), y esos **tocan** `last_seen_at` de la sesión y `last_used_at` del token (a lo más una vez por minuto, `identity` `TouchEvery`). Consecuencia, aceptada y documentada en el comentario de `Revalidate` y en el README (Step 10): un stream abierto mantiene viva una sesión que si no vencería por las 12 h sin uso (`SessionIdle`); el tope de 7 días (`SessionMax`) sigue valiendo, y el stream dura a lo más 1 h (el panel reconecta). No se agrega un método "sin tocar" al `Authenticator`: sería editar `identity` y la interfaz compartida.

  Tests (con el `Authenticator` falso de `utils_test.go`): sesión vigente → nil; la sesión ya no existe → error; token revocado → error; token de otro proyecto → `ErrForbidden`; sesión con `MFAPending` → error; un token con `AccessSession` → `ErrForbidden`. Commit: `feat(http): Kit.Revalidate para los streams (contrato §0.3)`.
- [ ] **Step 2: `dockertest` para el health** (`fake_deploy.go`, archivo nuevo del mismo paquete): `func (f *Fake) PutContainer(c docker.Container)` guarda el contenedor por nombre (con `ID`, `Health`, `Running`), para que `InspectContainer` y `ContainerStats` lo encuentren. Test en `fake_deploy_test.go`. Commit: `test(dockertest): PutContainer para el health de las tareas`.
- [ ] **Step 3: El SSE común (`sse.go`; tests primero).**

```go
// sseConfig son los límites de contrato §0.3; los tests los acortan.
type sseConfig struct {
	Heartbeat       time.Duration // 15 s
	RevalidateEvery int           // cada 4 heartbeats (60 s)
	MaxDuration     time.Duration // 1 h
	WriteTimeout    time.Duration // 30 s, antes de cada escritura
	MaxPerPrincipal int           // 8, sumando los dos tipos de stream
}

// streams cuenta los abiertos por principal: "s:"+SessionHash o "t:"+id del token.
type streams struct{ /* mu, open map[string]int */ }
func (s *streams) acquire(p entity.Principal) (release func(), ok bool)

// openSSE abre el stream: quita el plazo de lectura de la conexión (el
// ReadTimeout del servidor cancelaría el ctx de la petición: decisión 11), pone
// Content-Type text/event-stream, Cache-Control: no-cache (pisa el no-store de
// cmd/http) y X-Accel-Buffering: no, responde 200 y hace flush.
func openSSE(w http.ResponseWriter, r *http.Request, cfg sseConfig) (*sse, error)
func (s *sse) event(name, id, data string) error // data de una sola línea
func (s *sse) comment(text string) error         // ": ping"
```

  Cada escritura hace `SetWriteDeadline(now + WriteTimeout)` (si devuelve `http.ErrNotSupported`, se ignora) y `Flush`. El ctx de cada stream se cancela con el de la petición, con el de `Module.Run` (el módulo guarda un ctx que `Run` cancela al terminar) y con `MaxDuration`. `ErrTooManyStreams` → `429 rate_limited` (con `Retry-After`, lo pone el `Kit`), antes de abrir.

  Tests (`httptest.NewServer` sobre `httpserver.NewServer` con el `FakeAuth`): `TestSSEHeaders`; `TestSSEHeartbeat` (`: ping`); `TestSSESurvivesReadTimeout` (un `http.Server` con `ReadTimeout: 200ms` y el stream sigue vivo 1 s después); `TestSSERevalidate` (con un `Authenticator` **propio del test**, `lockedAuth`: los mapas de sesiones y tokens detrás de un `sync.Mutex`, con `Revoke(token)`. `httptestutil.FakeAuth` tiene mapas sin candado, y borrar una sesión mientras la goroutine del stream llama a `AuthenticateSession` es una carrera que `-race` detecta: nunca se modifica un `FakeAuth` con un servidor corriendo. Se revoca la sesión y el stream se cierra al próximo chequeo; igual con un token); `TestSSELimitPerPrincipal` (8 abiertos con la misma sesión; el noveno → 429 `rate_limited` con `Retry-After`; otra sesión sí abre); `TestSSEClosesOnModuleStop`.
- [ ] **Step 4: El log del deploy en vivo** (`GET …/deploys/{id}/log/stream`, contrato §2.6; tests primero). Desde dónde: `Last-Event-ID`, si no `?offset`, si no 0, con la misma validación que el polling (422 antes de abrir), y 404 si el deploy no es de la app. Después: `status` si no terminó; el bucle es `wait := Hub.Wait(id)` (antes de leer) → `ReadLog` → un `log` por línea con `id` = el offset después de esa línea → un `status` si cambió → si es final y no quedan líneas, el `status` final y cerrar; si no, esperar `wait`, el heartbeat (que manda `: ping` y **también vuelve a leer**: es la red de seguridad si se perdiera un aviso) o el fin del ctx. Tests: `TestDeployLogStreamLive` (las líneas llegan a medida que el `LogWriter` escribe, y el `status` final va después de la última); `TestDeployLogStreamFinished` (un deploy terminado: las líneas que faltan, el `status` final y cierra, sin `status` al conectar); `TestDeployLogStreamResume` (`Last-Event-ID` gana sobre `?offset`); `TestDeployLogStreamSuperseded` (con el heartbeat del test en 1 min, así solo el aviso puede despertarlo: un stream abierto sobre un `queued`, y otro `POST …/deploys` de la misma app → el stream manda `status` `superseded` y cierra enseguida, por el `Hub.Notify` de `CreateDeploy`, T3 Step 6; igual con un `queued` que cierra `RemoveApp` como `failed`); los 404 y 422 en JSON.
- [ ] **Step 5: `GET …/status`** (contrato §2.8; tests primero, con el fake: servicios con etiquetas de la app, tareas en `node-1` y en otro nodo, `PutContainer` con `Health`, `SetStats`). `GetStatus.Execute`: la app (404); `AppServices`; por servicio: nombre corto (sin el prefijo `<stack>_`), `image`, `replicas_desired` (`Replicas` o `DesiredTasks`), `replicas_running`, `memory_limit_bytes` (null si 0), `update_state` (null si no hay), `tasks` (orden de §2.8; `slot` null en uno global; `health` con `ContainerHealth` solo si la tarea corre en el nodo local; `node` con `NodeHostname` en caché por 5 min; `since` truncada al milisegundo; `error` null si vacío y, si no, **enmascarado** con los secretos de `Env.ResolveEnv` de la app, como el log (contrato §2.8): `Task.Err` puede traer un argumento de `command` ya interpolado, y el endpoint acepta tokens. `GetStatus` recibe el `EnvResolver`; lo que se guarda en la caché ya va enmascarado); medidas con `ContainerStats` en paralelo (hasta 8 a la vez) con un plazo total de 2 s, solo para las tareas que corren en el nodo local; sumas del servicio (null si no se midió ninguna). Caché por app: lo estructural 2 s y las medidas 5 s; una tarea que no estaba en la medición guardada sale con medidas null. **Una sola medición a la vez por app** ("comparten la misma medición", contrato §2.8): con la caché vencida, el primer `GET` mide y los que llegan mientras tanto esperan su resultado (un `inflight` por app: un canal que se cierra al terminar, bajo el mismo `mu` de la caché; cada uno espera también su propio ctx). Igual para lo estructural. Así, cinco pestañas que piden a la vez con la caché fría hacen una sola ronda de `ContainerStats`, no cinco. Con Docker caído, 503 (y el error no queda en la caché). Tests: `TestStatusShape` (todos los campos con su tipo y sus null), `TestStatusMasksTaskError` (una tarea con `Err` que trae un secreto de la app → `***` en `tasks[].error`), `TestStatusOrder`, `TestStatusCaches` (con `clock.NewManual` y `Calls()` del fake: dos `GET` seguidos llaman una sola vez a `ContainerStats`; pasados 2 s se vuelven a leer los servicios pero no las medidas; pasados 5 s, también las medidas), `TestStatusConcurrentColdRequests` (con un `Cluster` del test que envuelve al adaptador y retiene `ContainerStats` en un canal: cinco `Execute` a la vez con la caché fría → una sola llamada por tarea; al soltarlo, los cinco tienen las mismas medidas), `TestStatusNeverDeployed` (`services: []`), `TestStatusDockerDown`.
- [ ] **Step 6: Los logs de una app** (`GET …/logs`, contrato §2.9; tests primero). En orden: la app (404); la query (`service` obligatorio y slug con `validate.VolumeName("service", …)`; si viene `since`, `time.Parse(time.RFC3339Nano, …)` y entonces `LogOptions{Since, Tail: 5000}`, y `tail` **no se valida** (contrato §2.9, "tail no se mira": `since=…&tail=abc` abre el stream); si no, `tail` de 0 a 5000 con 200 por defecto; `follow` vacío, `0` o `1`); el servicio con `Cluster.Service(naming.Service(p, s, a, service))` y todas las `naming.AppLabels` (si no, 404); `ServiceLogs` con su `ID` (503 con Docker caído); los slots de las tareas con `Tasks`. **El mapa de slots se renueva:** un stream con `follow` dura hasta 1 h, y un deploy en ese rato crea tareas nuevas. Una línea con un `TaskID` que no está en el mapa vuelve a llamar a `Tasks` (una vez por id desconocido: un id que sigue sin aparecer, o un `Tasks` que falla, queda anotado y sale como `<servicio>.<7 del id>` sin volver a preguntar). Ya abierto: una goroutine lee `Next()`; los pedazos se unen (`JoinPieces`, con un pendiente que sale cuando llega la línea siguiente o a los 200 ms sin nada); cada línea sale como `log` con `{"time", "task", "stream", "line"}` (`time` truncada al milisegundo; `task` `<servicio>.<slot>` o `<servicio>.<7 del id>`; `line` enmascarada con los secretos de `ResolveEnv` y cortada con `CutLine(…, AppLineBytes)`); `io.EOF` → `end` `eof`; `MaxDuration` → `end` `timeout`; `Close` del stream siempre. Tests: `TestAppLogsQuery` (cada 422 con su campo; `since` y `tail` juntos → manda `since`, y `LogOptions.Tail` es 5000; `since` válido con `tail=abc` y con `tail=99999` → 200, el stream abre; sin `since`, `tail=abc` → 422 `tail`; `since=ayer` → 422 `since`), `TestAppLogsLookup` (un servicio con ese nombre sin las etiquetas → 404; nunca se llama a Docker con el parámetro tal cual: `Calls()` muestra el nombre completo), `TestAppLogsSinceInclusive` (líneas con la misma hora que `since` llegan), `TestAppLogsMasksAndJoins` (un secreto partido en dos pedazos de 16 KiB sale enmascarado), `TestAppLogsNewTaskSlot` (con `follow`: después de abrir, `SetTasks` suma una tarea de slot 2 y `AppendServiceLogs` una línea suya → sale como `<servicio>.2`, no con el id; `Calls()` muestra un `ListTasks` más, y una segunda línea de esa tarea no lo repite), `TestAppLogsFollowEnd` (sin `follow`: las líneas, `end` `eof` y cierra; con `follow`, quitar el servicio termina con `end` `eof`).
- [ ] **Step 7: El módulo completo.** `RegisterRoutes` registra todas las rutas de contrato §1 y los errores (`RegisterErrors`). `module_test.go`: `TestModuleRoutesAndStubs` se reemplaza por `TestModuleRoutes`, que verifica que ninguna ruta **de deploy** de contrato §1 (las tres primeras filas: deploys, log, log/stream, rollback, status, logs y registries, con cada método) responde `no_route` ni `method_not_allowed`. Las de ingress (dominios, `/proxy`) no se miran: son de la Fase 3 y en esta rama responden `no_route`. Y `TestRun` (vuelve al cancelar). Commit: `feat(deploy): el log del deploy y los logs de las apps en vivo (SSE), y el estado de las apps (contrato §2.6, §2.8 y §2.9)`.
- [ ] **Step 8: Contratos con el contenedor completo** (`src/shared/infra/container/deploy_test.go`, con `newTestContainer`, un simulador mínimo propio en el `OnDeployStack` del `dockertest.Fake` del contenedor (el de T4 es de otro paquete de test y no se importa), digests con `SetDigest`, y `c.Run(ctx)` en una goroutine que el test cancela y espera):
  - `TestContractDeployFlow`: proyecto, stage y app de formulario por HTTP; un token del proyecto; `POST …/deploys` con el Bearer → 202; el polling de `…/log` hasta `succeeded`, con **una de las dos** líneas del paso 7: `rutas de Caddy actualizadas` o `advertencia: no se pudo actualizar Caddy:` (hoy sale la segunda; cuando se mergee la Fase 3 y `SyncRoutes` exista, puede salir la primera, y el test no se rompe); `GET …/deploys` con `images` resueltas; `GET` de la app (catalog) con `last_deployed_at` y `pending_changes: false`; `GET …/status`; `POST …/rollback` → 409 `no_previous_deploy`; un segundo deploy y el rollback → 202 y `succeeded` (con `SetDigest` también para la ref con digest a la que vuelve el rollback: el paso 2 la verifica, decisión 1).
  - `TestContractDeleteAppStopsDeploys`: con un deploy retenido **en el paso 6** (el simulador deja las réplicas en 0 y la app tiene un `deploy_timeout_s` largo; no en `OnDeployStack`, que no ve el ctx: `Stop` esperaría sus 15 s y el stack quedaría aplicado igual), `DELETE` de la app → 204; ya no hay filas de deploys de esa app ni stack.
  - `TestContractDeployScopes`: un token de `p/staging` lee y despliega en su stage; en otro stage del mismo proyecto → 403; en otro proyecto → 403 `out_of_scope`; `/registries` con token → 403 `token_not_allowed`.
  - `TestContractDeployLogStream`: `httptest.NewServer(c.Server)` y el SSE del log hasta el `status` final.

  Commit: `test(container): contratos del deploy con el contenedor completo`.
- [ ] **Step 9: Integración** (agregar a `integration_test.go`): `TestIntegrationStatusAndLogs`: con `nginx:alpine` desplegado, `GetStatus` da 1/1, `health: healthy`, `cpu_percent` y `memory_bytes` medidos (la tarea corre en este nodo); `GET …/logs` sin `follow` trae las líneas de arranque de nginx y `end` `eof`; con `since` = la hora de la última, vuelve a llegar esa línea (inclusivo).

```bash
go test -tags integration -run Integration -count=1 -timeout 25m ./src/internal/deploy/
```

  Expected: `ok`, y después el chequeo de T4 Step 6 da `limpio` (ningún servicio, red ni volumen `it2`).

- [ ] **Step 10: README** (solo lo de deploy):
  - "Estado": cambiar "Todavía no despliega nada." por una línea de la Fase 2 (despliega apps de formulario y compose, con log, rollback, registries, estado y logs en vivo; las rutas de Caddy llegan con la Fase 3).
  - "Docker": un deploy necesita el CLI `docker` en el `PATH`, un Swarm del que el nodo sea manager y llegar al registry de cada imagen.
  - Una subsección "Desplegar una app con curl" con el flujo de la prueba de punta a punta (abajo).
  - "Notas para las fases siguientes", bloques de la Fase 2: tachar (`~~…~~`) lo que se cumplió y agregar lo que queda: la advertencia de `SyncRoutes` hasta la Fase 3; el corte de 16 KiB del `json-file` de Docker (decisión 10); el margen de 5 s de la convergencia, que se podría reemplazar comparando `PreviousSpec` con un método nuevo del cliente; las advertencias de compose-go en stderr; que un stream abierto mantiene viva la sesión (Step 1: `Revalidate` toca `last_seen_at`).
  - **Reemplazar, no tachar**, la nota "Un deploy sin cuerpo le quita el digest a una ref con tag, pero una ref sin tag (`…@sha256:…`) queda fija…" del bloque "Fase 2, worker de deploy": ya no es la regla (contrato §2.2 la cambió), y tacharla diría que se cumplió. Queda: "Un deploy sin cuerpo repite la ref **pedida** del deploy actual (`requested_images`): un tag se vuelve a resolver y un digest fijado a propósito (override, configuración o rollback) se mantiene; un rollback no se deshace con el próximo deploy sin cuerpo, y para volver a seguir un tag hace falta un override o cambiar la configuración (contrato §2.2)".

  Commit: `docs: README de la Fase 2 (deploy)`.
- [ ] **Step 11: Prueba de punta a punta** (manual, sin commit; necesita red para que `DistributionDigest` consulte Docker Hub). Es un solo script de bash en el scratchpad (nunca en el repo), así el `trap` limpia aunque un paso falle. Usa el `ingress` real de esta rama: `EnsureStageNetwork` y `RemoveStageNetwork` conectan y desconectan `muelle-caddy` si existe, y ese nombre fijo no se toca nunca, así que si existe la prueba **no corre**. Y el `DELETE` del stage no borra la red si las tareas de la app todavía se están yendo (lo dejaría para la reconciliación de la Fase 3, que no está en esta rama): antes de borrarlo se espera a que no quede nada del proyecto.

```bash
#!/usr/bin/env bash
# e2e-fase2.sh (en el scratchpad). Correr con: bash e2e-fase2.sh
set -euo pipefail
cd /home/felipe/Escritorio/dev/muelle/.claude/worktrees/fase-2-deploy

# Antes de empezar. Sin `grep -q` en un pipe: con pipefail, el SIGPIPE del
# que escribe haría falsa la condición aunque grep encontrara algo.
caddy=$(docker ps -a --format '{{.Names}}' | grep -x muelle-caddy || true)
if [ -n "$caddy" ]; then
  echo "hay un muelle-caddy en este Docker: crear o borrar el stage lo conectaría o desconectaría; no se corre" >&2; exit 1
fi
left=$( { docker service ls --format '{{.Name}}'; docker network ls --format '{{.Name}}'; docker volume ls --format '{{.Name}}'; } | grep '^it2e2e' || true)
if [ -n "$left" ]; then
  echo "quedó algo it2e2e de una corrida anterior: limpiarlo primero (T4 Step 6)" >&2; echo "$left" >&2; exit 1
fi
if (exec 3<>/dev/tcp/127.0.0.1/20080) 2>/dev/null; then echo "127.0.0.1:20080 está ocupado" >&2; exit 1; fi

# Espera hasta 90 s a que no queden servicios ni contenedores del proyecto
# (los contenedores de las tareas llevan muelle.project, §6.2).
wait_gone() {
  for _ in $(seq 45); do
    if [ -z "$(docker service ls -q --filter label=muelle.project=it2e2e)" ] &&
       [ -z "$(docker ps -aq --filter label=muelle.project=it2e2e)" ]; then return 0; fi
    sleep 2
  done
  echo "todavía quedan servicios o contenedores it2e2e" >&2; return 1
}

D=$(mktemp -d)
cleanup() {
  if [ -f "$D/pid" ]; then kill "$(cat "$D/pid")" 2>/dev/null || true; wait 2>/dev/null || true; fi
  docker stack ls --format '{{.Name}}' | grep '^it2e2e_' | xargs -r -n1 docker stack rm || true
  wait_gone || true
  # Respaldo: la red del stage, si el DELETE no la borró (anotarlo en el reporte).
  if docker network inspect it2e2e_staging >/dev/null 2>&1; then docker network rm it2e2e_staging || true; fi
  docker volume ls --format '{{.Name}}' | grep '^it2e2e_' | xargs -r docker volume rm || true
  rm -rf "$D"
}
trap cleanup EXIT

(umask 077; go run . genkey > "$D/key")
go build -o "$D/muelle" .
MUELLE_PANEL_ORIGIN=http://127.0.0.1:20080 MUELLE_SECURE_COOKIES=false MUELLE_LISTEN=127.0.0.1:20080 \
MUELLE_DATA_DIR="$D/data" MUELLE_MASTER_KEY_FILE="$D/key" "$D/muelle" serve > "$D/log.json" 2>&1 &
echo $! > "$D/pid"

# … los pasos con curl (abajo): `curl -fsS` para que set -e corte, salvo los que
# esperan un error (el 409 del rollback), que miran `-w '%{http_code}'` …

wait_gone                      # antes del DELETE del stage
# curl … -X DELETE …/projects/it2e2e/stages/staging
if docker network inspect it2e2e_staging >/dev/null 2>&1; then echo "AVISO: el DELETE del stage dejó la red"; fi
```

  Los pasos, con el link de setup del log (como en "Primer usuario con curl" del README): crear el usuario; proyecto `it2e2e`, stage `staging` (crea la red `it2e2e_staging`), app de formulario `web` con `nginx:alpine`, puerto 80 y healthcheck `http` en `/`; un token; `POST …/deploys` con el Bearer; el polling de `…/log` hasta `succeeded`; `GET …/status`; `curl -N --max-time 5 '…/logs?service=web&tail=10'`; `POST …/rollback` → 409; cambiar una variable y desplegar otra vez; rollback → `succeeded`; `ps -o rss= -p $(cat $D/pid)` en reposo (< 40 MB, §1); `DELETE` de la app; `wait_gone`; `DELETE` del stage. Al terminar (el `trap` también corre si algo falla), `docker service ls`, `docker network ls` y `docker volume ls` no tienen nada `it2e2e`, el proceso está muerto y `$D` borrado. Anotar el resultado en el reporte de la tarea, también si hizo falta el respaldo de la red.

**Integración en T5:** el comando del Step 9, y la prueba de punta a punta del Step 11 (manual).

---

## Preguntas abiertas para el dueño (el plan sigue la recomendación)

1. **Después de un rollback, un deploy sin cuerpo repite sus digests** (contrato §2.2): "Desplegar" en el panel no vuelve a seguir `:staging` hasta que haya un override o cambie la configuración. Recomendación: mantenerlo; es lo que promete el contrato ("un rollback no se deshace con el próximo deploy sin cuerpo"), y evita que un clic vuelva a la versión rota.
2. **Un deploy que no cambia nada de un servicio no lo reinicia** (`muelle.deploy` solo en el servicio, decisión 6): "Desplegar" sin cambios no reinicia los contenedores. Recomendación: mantenerlo (un deploy de compose no corta un `redis` que no cambió); si hace falta, una acción "Reiniciar" explícita más adelante (`docker service update --force`).
