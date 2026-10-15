# muelle — Fase 5 (distribución): Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Que muelle se pueda instalar, actualizar y usar desde CI sin compilar nada a mano: el panel (SvelteKit) servido por el binario, las imágenes `ghcr.io/cpu-commits/muelle` y `ghcr.io/cpu-commits/muelle-caddy` con el mismo tag `vX.Y.Z`, `muelle install`, `muelle settings reset-panel-ips` para recuperar el acceso, la GitHub Action de deploy, el workflow de release y la documentación de instalación, actualización y recuperación.

**Architecture:** Spec §4.1. La Fase 5 no agrega un módulo de `src/internal/`: lo suyo son adaptadores de entrada en `src/cmd/` (como `cmd/http` y `cmd/cron`), que usan los módulos y la infraestructura compartida sin que los módulos sepan de ellos.
- `src/cmd/http/panel`: un `http.Handler` que sirve el build embebido (`-tags panel`) y que `cmd/http.Server` usa para toda ruta fuera de `/api`.
- `src/cmd/install`: el subcomando `install`, que habla con Docker solo por `docker.Engine` (el fake `dockertest` en los tests).
- `src/cmd/settingscli`: `muelle settings reset-panel-ips`, que usa una función nueva de `admin` (en un archivo nuevo) y un puerto nuevo de Docker (`docker.ServiceRestarter`, fuera de `Engine`).
- Afuera de Go: el `Dockerfile` de muelle, `deploy/caddy/Dockerfile`, `.dockerignore`, `action/` (composite action en bash) y `.github/workflows/release.yml`.

**Tech Stack:**
- Go 1.27, `net/http`, `embed`, `io/fs`, `flag` y `encoding/json` de la librería estándar; sin dependencias nuevas.
- El cliente compartido `src/shared/infra/out/docker` (moby `client` v0.6.2, ya en `go.mod`) y su fake `dockertest`.
- Docker 29 (CLI y daemon), BuildKit; Node 24 LTS + pnpm 11.1.2 para el panel; Caddy 2.11.x + `xcaddy` + `github.com/caddy-dns/cloudflare`.
- GitHub Actions: composite action con `bash`, `curl` y `jq`; `docker/setup-buildx-action`, `docker/login-action` y `docker/build-push-action` (v7) en el release.

**Spec:** [`docs/superpowers/specs/2026-10-09-muelle-design.md`](../specs/2026-10-09-muelle-design.md) (§1 presupuesto de memoria, §3.3 instalación, §12 seguridad, §14 la action, §15 testing, §16 migración) y el contrato [`docs/superpowers/specs/2026-10-10-contrato-http-fases-2-3.md`](../specs/2026-10-10-contrato-http-fases-2-3.md), **§5 completo** (nombres, `MUELLE_INIT_*`, el stack, los pasos de install, `reset-panel-ips`, las imágenes y el panel embebido), §0, §1.2 y §4.1. Donde este plan y el contrato difieren, manda el contrato: avisa y para.

**Estado de partida** (rama `fase-5-dist`, desde `fase-23-base`): commits de documentación (el contrato, el diseño y este plan), nada de código.
- `083b0b5`, `f87d709` y `2eac145` dejaron el contrato §5 y el diseño §3.3 y §14 en el detalle que este plan implementa. Léelos antes de cada tarea: este plan los resume, no los repite entero.
- `b4a8d36`, de la revisión del plan, precisó el contrato §5.4 (deshacer ante cualquier error antes del deploy, la red revisada en el paso 3, la pista de `--advertise-addr`, y qué muestra install cuando ninguna tarea corre) y el diseño §3.3 paso 7.
- Las notas de la etapa de contratos (`notes_phase5.md`, `contract_concerns.md`) no estaban en el scratchpad al escribir el plan; lo que importaba de ellas ya está en el contrato §5 y en el README ("Notas para las fases siguientes", bloque "Fase 5 / install").

---

## Etapas y paralelismo

| Etapa | Tareas | Cómo corre |
|---|---|---|
| Fase 5 | T1 → T2 → T3 → T4 → T5 | Secuencial, en la rama `fase-5-dist` (worktree `.claude/worktrees/fase-5-dist`). Cada tarea la hace un agente nuevo y se revisa antes de la siguiente |
| Fases 2 y 3 | — | En paralelo, en `fase-2-deploy` y `fase-3-ingress`. Se integran después: esta rama no las ve ni las toca |

**Por qué T1..T5 en este orden:** T2 (las imágenes) necesita el paquete del panel de T1 para compilar con `-tags panel`. T4 (install) necesita la imagen de T2 para su test de integración, y esa imagen tiene que traer el `reset-panel-ips` de T3, que el test de T4 corre dentro del contenedor. T5 (la action y la documentación final) no depende de las otras salvo en el README.

**Lo compartido que toca esta fase** (contrato §1.3), siempre agregando y sin cambiar firmas:

| Lugar | Qué cambia | Tarea |
|---|---|---|
| `src/cmd/http/server.go` | Campo `Config.Panel` y tres líneas en `notMatched` | T1 |
| `src/shared/infra/container/container.go` | `panel.Embedded()` al principio de `New` (antes de tocar `DataDir`), un parámetro de `build` y `Panel:` en la config del servidor (≈8 líneas) | T1 |
| `src/shared/domain/naming` | Archivo nuevo `install.go` | T3, T4 |
| `src/shared/infra/out/docker` | Archivos nuevos `dist.go`, `client_dist.go`, `dockertest/fake_dist.go` y sus tests; **no** se toca `docker.go` ni `Engine`, y el fake no declara `PutContainer` (es de la Fase 2: ver "Nombres nuevos en paquetes compartidos") | T3, T4 |
| `src/internal/admin` | Archivo nuevo `reset_panel_ips.go` (y su test) | T3 |
| `main.go`, `main_test.go` | Subcomandos `version`, `settings` e `install`; el evento `listening` | T2, T3, T4 |
| `.github/workflows/ci.yaml` | Dos pasos nuevos al final; `workflow_call:` en el `on:`, para que `release.yml` corra el mismo job | T1, T2, T5 |
| `.gitignore` | Una línea | T1 |
| `README.md` | Secciones nuevas de instalación, uso y recuperación; la línea de la Fase 5 en "Estado" y en "Estructura"; el bloque de la Fase 5 en las notas | T1–T5 |

**Nombres nuevos en paquetes compartidos.** Las tres líneas agregan archivos nuevos al mismo paquete, y git los mezcla sin conflicto aunque declaren lo mismo: un método repetido recién rompe la compilación al integrar. Por eso esta fase solo declara estos nombres, todos de la Fase 5 y que ningún plan de las fases 2 y 3 usa (revisado en `fase-2-deploy` y `fase-3-ingress`):

| Paquete | Nombres de la Fase 5 |
|---|---|
| `docker` | `ServiceRestarter`, `(*Client).ForceUpdateService` |
| `dockertest` | `(*Fake).ForceUpdateService`, `(*Fake).PutNetwork` |
| `naming` | `LabelSwarmService`, `InstallProbeContainer`, `InstallNames`, `InstallNamesFor`, `InitRegistrySecret` |
| `cmd/http` | `Config.Panel`, `isAPIPath` (y el paquete nuevo `cmd/http/panel`) |
| `main` | `logListening` |

- `(*Fake).PutContainer` es de la Fase 2 (`dockertest/fake_deploy.go`). Esta fase no lo declara ni lo usa: sus tests crean los contenedores con `CreateContainer` (después de `SetImagePresent`), o con un stub local en el archivo de test. Si un test necesitara poner un contenedor tal cual, el helper se llama `PutContainerDist`, nunca `PutContainer`.
- El revisor de cada tarea que toca un paquete compartido lo comprueba con:
  ```bash
  cd /home/felipe/Escritorio/dev/muelle/.claude/worktrees/fase-5-dist
  for b in fase-2-deploy fase-3-ingress; do
    git diff fase-23-base..$b -- src main.go | grep -nE '^\+.*\b(ServiceRestarter|ForceUpdateService|PutNetwork|PutContainerDist|LabelSwarmService|InstallProbeContainer|InstallNames|InstallNamesFor|InitRegistrySecret|isAPIPath|logListening)\b'
  done                                                      # nada
  git grep -nE 'func \(f \*Fake\) PutContainer\(' -- src   # nada: PutContainer es de la Fase 2
  ```

**No se toca:** ningún otro archivo de `src/internal/**`; los tipos `RegistryAuth` y `StackDeploy`; `docker.go` y la interfaz `Engine`; las migraciones; `web/` (es del equipo del panel: se lee solo con `git -C /home/felipe/Escritorio/dev/muelle show fase-4-panel:<ruta>`, nunca con checkout).

## Convenciones (todas las tareas)

- **Dónde:** solo en `/home/felipe/Escritorio/dev/muelle/.claude/worktrees/fase-5-dist` (rama `fase-5-dist`). Cada comando empieza con `cd` a esa ruta, o usa `git -C <ruta>`. Nunca `git switch`/`checkout`, nunca otra rama ni otro worktree.
- **Shell:** la shell es fish. Para sintaxis de bash: `bash -c '…'`. Los bloques de este plan son bash.
- **Go:** 1.27, sin CGO. Comentarios, mensajes de error y commits en español; identificadores en inglés.
- **Verificación antes de cada commit** (todas tienen que pasar):
  ```bash
  cd /home/felipe/Escritorio/dev/muelle/.claude/worktrees/fase-5-dist
  test -z "$(gofmt -l $(git ls-files '*.go'))"   # nunca `gofmt -l .`: entra en .claude/worktrees
  go mod tidy -diff
  go vet ./...
  go test -race ./...
  ```
  Un archivo `.go` nuevo todavía no está en `git ls-files`: córrelo también con `gofmt -l <archivo>` antes del primer `git add`.
- **TDD:** test → verlo fallar (por la razón correcta) → implementar → verlo pasar → commit. Commits chicos.
- **Commits:** en español con los acentos reales, y el trailer como segundo `-m`:
  ```bash
  git commit -m "feat(panel): …" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
  ```
- **Nunca** `git push`, `docker swarm init` ni `docker swarm leave`.
- **Documentación de librerías** (moby, Caddy, xcaddy, GitHub Actions, las imágenes base): Context7 (`resolve-library-id` y después `query-docs`) o `go doc`. No adivines una API ni una versión: verifícala.
- **Tests:**
  - en el mismo paquete que el código (`package x`), salvo los de integración que usan `package x_test` si el paquete ya lo hace;
  - Docker siempre con `dockertest.New()` en los tests unitarios: nunca el Docker real;
  - SQLite real con `sqlitetest.New(t)` (o `sqlite.Open` + `Migrate` en un `t.TempDir()` cuando el test necesita la ruta del archivo);
  - `go test -race`.
- **Docker real (tests de integración, build tag `integration`) — línea de la Fase 5:**
  - todo lo que se crea empieza con **`it5`** (stacks, servicios, redes, secretos, volúmenes, contenedores, imágenes `it5-*`);
  - puertos del host solo en **22000–22999**; nunca 80, 443 ni 2019;
  - nunca los nombres fijos reales (`muelle`, `muelle_muelle`, `muelle-caddy`, `muelle_panel`, `muelle_data`, `muelle_master_key`, `muelle_caddy_*`, `muelle-install-probe`…): ni crearlos ni borrarlos;
  - cada test borra lo que creó (`t.Cleanup`), también si falla, y no toca nada sin el prefijo;
  - un contenedor de un solo uso a mano (`docker run --rm …` de los smoke tests) lleva `--name it5-<algo>` y nunca queda; nada de `docker … prune`;
  - se corren solo los de esta línea, con `-run` (los `ctr23` del paquete `docker` corren al integrar, no acá);
  - la máquina es un Swarm de un nodo manager con Docker 29.6.1 (cliente y daemon, `docker version`). El CLI 29.9.0 es solo el de la imagen de muelle (`docker:29.9.0-cli`, decisión 15). Si un test ve otra cosa que un Swarm activo con este nodo de manager, se salta (`t.Skip`), nunca inicia un Swarm.
- **Imágenes de prueba** (T2 y T4): se construyen desde un contexto temporal en el scratchpad de tu sesión, armado con el árbol de trabajo de esta rama más `web/` de `fase-4-panel` (que se queda en su rama: nunca se mergea acá):
  ```bash
  W=/home/felipe/Escritorio/dev/muelle/.claude/worktrees/fase-5-dist
  CTX=<tu scratchpad>/dist-ctx            # p. ej. /tmp/claude-1000/<proyecto>/<sesión>/scratchpad/dist-ctx
  rm -rf "$CTX" && mkdir -p "$CTX"
  (cd "$W" && git ls-files -co --exclude-standard -z | tar --null -T - -cf -) | tar -x -C "$CTX"
  git -C /home/felipe/Escritorio/dev/muelle archive fase-4-panel web | tar -x -C "$CTX"
  docker build --build-arg VERSION=v0.0.0-it5 -t it5-muelle:test "$CTX"
  docker build -t it5-muelle-caddy:test "$CTX/deploy/caddy"
  ```
- **Alcance:** no modifiques archivos que la tarea no lista. Si necesitas cambiar algo fuera de lo listado, para y repórtalo (status `NEEDS_CONTEXT`).

## Decisiones

Decididas en este plan (con la razón). Si una tarea encuentra que una no sirve, para y lo reporta.

1. **El panel se embebe solo con `-tags panel`.** El build de release copia `web/build` en `src/cmd/http/panel/build/` (ignorado por git) y compila con el tag; sin él, `Embedded()` devuelve un handler que responde `404` con una página corta. Así `go build ./...`, `go vet ./...` y `go test ./...` no necesitan Node ni el panel, y el único lugar con el panel es la imagen. CI compila además con el tag sobre un build de prueba de dos archivos, para que el archivo con `//go:embed` no se rompa sin que nadie lo vea.
2. **El panel se monta en `notMatched` de `cmd/http.Server`**, después del `405` del mux y antes del `no_route`, solo si la ruta limpia (`path.Clean`) no es `/api` ni `/api/…`. Así `/api/v1/…` sin ruta sigue siendo `no_route`, el `Origin` se revisa antes (como en toda petición) y no hace falta registrar un patrón `/` en el mux, que cambiaría el `405` de las rutas de `/api`.
3. **ETag solo fuera de `_app/immutable/`.** Esos archivos son chicos (el `200.html`, `version.json`, el favicon) y con `no-cache` el `304` ahorra la descarga. Los inmutables ya se cachean un año; hashearlos al arrancar leería todo el embed y lo dejaría residente (RSS, §1).
4. **`Content-Type` de una tabla propia** (contrato §5.5), no `mime.TypeByExtension`: Go completa su tabla chica con los archivos del sistema (`/etc/mime.types`), así que el resultado dependería de la imagen (`.woff2`, el charset de `.js`). Con `nosniff`, un tipo equivocado rompe el panel.
5. **Flags ocultos para los tests** (`--name-prefix`, `--image`, `--panel-origin`; contrato §5.1). El test de integración instala con un origen que es una IP: sin `panel_domain`, ingress queda `disabled` y el muelle del test no crea `muelle-caddy` ni `muelle_panel`. El test lo verifica sin crear ni borrar nada ajeno: mira esos dos nombres fijos y una red centinela propia (`it5`), nunca las redes de las otras líneas (T4 Step 10, "Guarda de lo ajeno").
6. **La prueba de puertos usa la imagen de muelle con `help`** (contrato §5.4): no hace falta otra imagen en el nodo, y es la misma que se va a desplegar.
7. **El stack es JSON armado con structs**, sin `version:`. Verificado con el CLI 29: `docker stack config -c -` lo acepta como 3.13 y convierte `$$` en `$`. Un test unitario lo pasa por `docker stack config` si el CLI está (sin daemon: funciona con `DOCKER_HOST` apuntando a un socket que no existe).
8. **install deshace lo que creó ante cualquier error o Ctrl-C antes de que `DeployStack` funcione** (contrato §5.4, paso 8), no solo si falla el deploy: un `CreateSecret` del token que falla con `muelle_master_key` ya creado dejaría un secreto cuya clave nunca se imprimió. **Si `docker stack deploy` falla, deshace también el stack** (`2eac145`): un CLI que creó el servicio y falló después dejaría un servicio con un secreto que nadie más conoce, y el secreto no se podría borrar (está en uso).
9. **Salida de install:** la clave y el link a stdout, el avance y los avisos a stderr (contrato §5.4). Los errores salen con `muelle install: <error>` y código 1; los flags que no sirven, con código 2.
10. **`reset-panel-ips` reinicia siempre** (también con la lista vacía, salvo `--no-restart`; contrato §5.4): el arranque reenvía la configuración a Caddy, que es lo que sirve si el problema era otro. No necesita la clave maestra: abre la base sin migrar y escribe una clave sin cifrar.
11. **`docker.ServiceRestarter` es una interfaz aparte**, en un archivo nuevo, y no entra en `Engine`: así no se toca `docker.go`, que las tres fases editan. El fake implementa el método en `fake_dist.go` sin campos nuevos en el struct (usa `Version` del servicio y el registro de llamadas). `fake_dist.go` declara solo `ForceUpdateService` y `PutNetwork`, nunca `PutContainer` (es de la Fase 2, ver "Nombres nuevos en paquetes compartidos").
12. **Los tests de la action son de Go** (`action/action_test.go`, un paquete con solo tests): corren el `deploy.sh` real con `bash` contra un muelle falso de `httptest`. Entran en `go test ./...` y en CI sin otra herramienta; se saltan si falta `bash`, `curl` o `jq`. El script lee dos variables solo para los tests (`MUELLE_ACTION_TEST_POLL` y `MUELLE_ACTION_TEST_RETRY_DELAYS`), que no se documentan como entradas: el nombre largo evita que una variable que el workflow define en el `env:` del job (un `MUELLE_POLL_INTERVAL` de otra cosa) cambie la action sin querer.
13. **La action solo habla con un origen `https://`** (o `http://localhost` y `http://127.0.0.1`, para probar en la misma máquina) y nunca pone un secreto en la línea de comandos (el `argv` de un proceso lo ve cualquiera en el runner, en `/proc/<pid>/cmdline`):
    - `url`, sin la `/` final, tiene que ser un origen entero, con una expresión **anclada** (con `LC_ALL=C`): `^(https://([A-Za-z0-9.-]+|\[[0-9A-Fa-f:]+\])(:[0-9]{1,5})?|http://(localhost|127\.0\.0\.1)(:[0-9]{1,5})?)$`. Un prefijo (`http://localhost*`) aceptaría `http://localhost.example.com` o `http://127.0.0.1.nip.io` y mandaría el token en claro; la expresión tampoco deja pasar una ruta, una query ni un `usuario@`;
    - el token se valida (`^[!-~]+$` con `LC_ALL=C`: solo ASCII visible, así un CR/LF no agrega headers y `::add-mask::`, que enmascara una sola línea, lo tapa entero) y va en un archivo de headers `0600` (`-H @archivo`);
    - `env-json` (en la práctica `toJson(secrets)`) llega solo por `INPUT_ENV_JSON` y `jq` lo lee con `jq -n 'env.INPUT_ENV_JSON | fromjson'`, nunca con `--arg`/`--argjson`; el cuerpo del `PUT` va en un archivo (`--data-binary @archivo`). Una variable de entorno tiene el mismo tope que un argumento (`MAX_ARG_STRLEN`, 128 KiB), así que el README dice que `env-json` tiene que quedar bajo ese tamaño;
    - `curl` corre siempre con `--globoff`: sin él, `[]` y `{}` de una URL (un `status_url`, un origen IPv6) son patrones;
    - el token solo se manda a `url`: el `status_url` tiene que empezar con `/api/v1/` (diseño §14);
    - lo que viene de muelle y va a `$GITHUB_OUTPUT` o a una URL se valida antes de usarlo: `id`, `next_offset` y `superseded_by` (si no es `null`) con `^[0-9]+$`, y `status` tiene que ser `queued`, `running`, `succeeded`, `failed` o `superseded` (contrato §2.1). `output` escribe una línea `clave=valor`: un valor con `\n` agregaría otras salidas. Si no sirve, `::error::` que nombra el campo (sin repetir el valor) y código 1.
14. **Release:** un job de tests y después uno de imágenes, con las actions de terceros fijadas por SHA de commit (el job tiene `packages: write`). `:latest` solo para versiones finales de muelle. `workflow_dispatch` construye sin subir. El tag mayor de la action (`v0`) se mueve a mano (ver la pregunta 2).
15. **Imágenes base:** `node:24-alpine` (LTS) con `pnpm@11.1.2` instalado con `npm` (la versión de `packageManager`; sin corepack), `golang:1.27-alpine`, `docker:29.9.0-cli` (ya está en este nodo; trae el CLI 29.9.0, más nuevo que el 29.6.1 del nodo, y el CLI negocia la versión de la API con el daemon: lo confirma `TestIntegrationInstallFromImage`, que corre `docker stack deploy` desde la imagen) y `alpine:3.24.x`; Caddy `2.11.x` (builder y alpine). Todas con tag exacto y digest del índice; solo `linux/amd64` (contrato §5.4).
16. **`.dockerignore` es una lista de permitidos:** el contexto nunca lleva `.git`, `.claude` (otros worktrees), claves, bases ni `web/.env*` (un `.env` con `VITE_USE_MOCKS=1` metería MSW en la imagen). Además, la etapa de `pnpm` fija `VITE_USE_MOCKS=0` y falla si el build trae `mockServiceWorker.js`.
17. **El test de integración de install corre install dos veces:** en el proceso del test (con el cliente real, para verificar cada recurso) y una vez con `docker run … it5-muelle:test install`, que es como lo corre un usuario (el CLI de docker de la imagen, `/tmp`, los certificados).
18. **`muelle version`** imprime `main.version`: lo usa el smoke test de la imagen y lo cita el contrato §5.1.
19. **serve escribe `"event":"listening"`** en la línea de "muelle escuchando" (contrato §5.4): install la usa para saber que muelle atiende y, si no hubo `setup_url` antes, que ya tiene usuarios.
20. **El `PUT` de la action reintenta también un `429`**, en los mismos 3 intentos, porque el diseño §14 dice que "un `4xx` que no es `429` no se reintenta, en ningún paso" (el paso 1 del diseño lo dice ahora explícito). La espera es la mayor entre el retraso que toca y el `Retry-After` (muelle manda `60`, diseño §10), hasta 60 s: uno más largo no se espera y falla con el error de muelle, para no colgar el job. El `POST` sigue siendo de un solo intento (no es idempotente).

## Preguntas para el dueño

> **Respondida:** la 1 es NO, las imágenes son privadas. Ver "Cambio de requisito del dueño (2026-10-15)" justo abajo.

Son de política, no técnicas. El plan sigue la recomendación; si el dueño decide otra cosa, cambia solo el README (T5) y, en la 2, el workflow.

1. **¿Los paquetes de GHCR (`muelle` y `muelle-caddy`) son públicos?** Recomendación: sí. `docker run ghcr.io/cpu-commits/muelle install` no hace login, e ingress baja `muelle-caddy` sin credenciales salvo que haya un registry configurado para `ghcr.io` (contrato §3.4). GHCR crea los paquetes privados en el primer push, así que el README dice cómo cambiarlos a públicos una vez.
2. **¿El tag mayor de la action (`v0`) se mueve a mano o lo mueve el workflow?** Recomendación: a mano (`git tag -f v0 vX.Y.Z && git push -f origin v0`, en el README). Moverlo desde el workflow le daría `contents: write` a un job que corre en cada tag; a mano, el dueño decide cuándo un release llega a los repos que usan `@v0`.

### Cambio de requisito del dueño (2026-10-15): las imágenes son PRIVADAS

El dueño decidió que `ghcr.io/cpu-commits/muelle` y `ghcr.io/cpu-commits/muelle-caddy` sean **siempre privadas**. Esto reemplaza la pregunta 1 de arriba y todo lo que en este plan diga "públicas", "anónimo" o "hacer públicos los paquetes". Quién hace qué:

- **Install (T4) baja las imágenes con credenciales.** Las toma de una de dos fuentes, en este orden:
  1. `--registry-user <usuario>` y `--registry-password-stdin`. La contraseña o el token se leen de stdin; nunca van en un flag ni en una variable de entorno. No se pueden juntar con `--cloudflare-token -`, que también lee stdin: si vienen los dos, error claro.
  2. El `config.json` de Docker del host, montado en solo lectura con `-v ~/.docker/config.json:/root/.docker/config.json:ro`. Se lee `auths[<host>].auth` (base64 de `usuario:contraseña`) del host de la imagen (`reference.Domain`). Si para ese host hay `credsStore` o `credHelpers`, el helper no se puede correr dentro del contenedor: error que pide `--registry-user`/`--registry-password-stdin`.
- **Sin credenciales, o con un `denied`/`unauthorized` al bajar,** el error explica exactamente qué hacer: "las imágenes de muelle son privadas: haz `docker login ghcr.io` con un token de GitHub con permiso `read:packages` y monta `~/.docker/config.json` (o usa `--registry-user` y `--registry-password-stdin`)".
- **Paso 5:**
  - `PullImage(ref, cred)` de la imagen de muelle **y** de la de Caddy (`--caddy-image`, o `naming.CaddyImageFor(version)`), las dos con la credencial de su host. Así ingress encuentra muelle-caddy en el nodo en el primer arranque, sin credenciales.
  - Una imagen ya presente (`InspectImage` ok) no se baja.
- **El stack:** `DeployStack` recibe la credencial en `StackDeploy.Registries`, el equivalente de `--with-registry-auth`, para que Swarm pueda bajar muelle cuando recrea la tarea.
- **Que muelle la recuerde:**
  - Install crea el secreto `muelle_init_registry` (con el prefijo en los tests), con el JSON `{"host":"ghcr.io","username":"…","password":"…"}`.
  - El stack lo monta y agrega `MUELLE_INIT_REGISTRY_FILE=/run/secrets/muelle_init_registry`.
  - En el primer arranque, deploy (Fase 2, T3) lo guarda como un registry si no hay uno para ese host. Con eso, ingress baja la Caddy de la versión siguiente al actualizar, y las apps pueden usar imágenes de `ghcr.io`.
  - El nombre va como constante en un archivo nuevo de `naming` (p. ej. `install_registry.go`).
  - Se trata igual que el secreto del token de Cloudflare: entra en la lista de deshacer del paso 8, `RemoveSecret` del que quedó antes de crearlo, y lo maneja `--reinstall`. Además, el README explica cómo quitarlo del servicio después del primer arranque, igual que el del token.
- **Contrato §5.2 y §5.3:** agrega `MUELLE_INIT_REGISTRY_FILE`, el secreto `muelle_init_registry` y el formato del JSON, y anótalo en `contract_changes`.
- **Tests:**
  - unitarios con el fake: las credenciales de los flags y del `config.json` (con `credsStore` → error), el pull de las dos imágenes con su credencial, `StackDeploy.Registries`, el secreto y su deshacer, y que la contraseña no aparece en ninguna salida ni error;
  - integración: un `registry:2` con `htpasswd` en `127.0.0.1:22500`, con nombre `it5-registry` (Docker acepta 127.0.0.0/8 sin TLS). Se sube `it5-muelle:test` ahí. Install con `--image 127.0.0.1:22500/it5-muelle:test` y credenciales funciona; sin credenciales falla con el mensaje de arriba. Hay que limpiar el registry y sus imágenes.
- **README (T4 y T5):**
  - **"Instalar":** primero `docker login ghcr.io` en el servidor, con un token de GitHub (classic) con solo `read:packages`. Después el `docker run` con el `config.json` montado, o con `--registry-user` y `--registry-password-stdin`.
  - **"Actualizar":** `docker service update --with-registry-auth --image ghcr.io/cpu-commits/muelle:<v> muelle_muelle`. Muelle baja la Caddy nueva con el registry `ghcr.io` que guardó install.
  - **Si el token vence:** actualízalo en Ajustes → Registries y vuelve a hacer `docker login` en el servidor.
  - **"Publicar una versión":** los paquetes quedan privados; no hay que cambiar nada en GHCR.
  - **Si el repo de muelle es privado,** los otros repos solo pueden usar la action si en el repo de muelle se habilita Settings → Actions → General → Access ("Accessible from repositories owned by the user").
- **Release (T2):** sigue igual. `GITHUB_TOKEN` con `packages: write`, y el label `org.opencontainers.image.source` liga el paquete al repo. Ningún paso vuelve público un paquete.

## Mapa de archivos

```
main.go                                        (T2, T3, T4) version, settings, install; evento listening
main_test.go                                   (T2, T3, T4)
Dockerfile                                     (T2) pnpm → go (-tags panel) → alpine + docker CLI
.dockerignore                                  (T2) lista de permitidos
.gitignore                                     (T1) + /src/cmd/http/panel/build/
deploy/caddy/Dockerfile                        (T2) xcaddy + caddy-dns/cloudflare
action/
├── action.yml                                 (T5) composite: solo pasa las entradas por env
├── deploy.sh                                  (T5) PUT env, POST deploy, polling del log
└── action_test.go                             (T5) el script contra un muelle falso
.github/workflows/
├── ci.yaml                                    (T1, T2, T5) + -tags panel con un build de prueba, + workflow_call, + shellcheck
└── release.yml                                (T2) tests → imágenes en tags vX.Y.Z
src/
├── cmd/
│   ├── http/
│   │   ├── server.go                          (T1) Config.Panel; notMatched
│   │   ├── panel_mount_test.go                (T1)
│   │   └── panel/
│   │       ├── panel.go                       (T1) New(fs.FS): archivo o 200.html, CSP, caché, ETag
│   │       ├── embed_panel.go                 (T1) //go:build panel — //go:embed all:build
│   │       ├── embed_none.go                  (T1) //go:build !panel — 404 "no trae el panel"
│   │       ├── panel_test.go                  (T1)
│   │       ├── embed_none_test.go             (T1) //go:build !panel
│   │       └── embed_panel_test.go            (T1) //go:build panel
│   ├── settingscli/
│   │   ├── settingscli.go                     (T3) muelle settings reset-panel-ips
│   │   └── settingscli_test.go                (T3)
│   └── install/
│       ├── options.go                         (T4) flags, validación, Usage
│       ├── install.go                         (T4) Run: los pasos de §5.4
│       ├── stack.go                           (T4) el compose del stack (§5.3)
│       ├── setup.go                           (T4) seguir el log de la tarea actual
│       ├── publicip.go                        (T4) 1.1.1.1/cdn-cgi/trace
│       ├── *_test.go, testdata/*.golden.json  (T4)
│       └── integration_test.go                (T4) //go:build integration
├── internal/admin/
│   ├── reset_panel_ips.go                     (T3) ResetPanelAllowedCIDRs (desviación de §4.1, ver T3 Step 4)
│   └── reset_panel_ips_test.go                (T3)
└── shared/
    ├── domain/naming/
    │   ├── install.go                         (T3, T4) LabelSwarmService; InstallNames
    │   ├── install_registry.go                (T4) InitRegistrySecret
    │   └── install_test.go                    (T4)
    └── infra/
        ├── container/
        │   ├── container.go                   (T1) panel.Embedded() → Config.Panel
        │   └── panel_dist_test.go             (T1) //go:build !panel
        └── out/docker/
            ├── dist.go                        (T3) ServiceRestarter
            ├── client_dist.go                 (T3) (*Client).ForceUpdateService
            ├── client_dist_test.go            (T3)
            ├── integration_dist_test.go       (T3) //go:build integration, prefijo it5
            └── dockertest/
                ├── fake_dist.go               (T3, T4) ForceUpdateService, PutNetwork (PutContainer es de la Fase 2)
                └── fake_dist_test.go          (T3, T4)
README.md                                      (T1–T5)
```

---

### Task T1: El panel embebido (lado Go)

Un paquete que sirve el build de SvelteKit (`web/build`, en `fase-4-panel`) con la política exacta que escribió el build, montado en toda ruta fuera de `/api`. Sin `-tags panel`, todo sigue compilando y probándose sin Node.

**Lee antes:** contrato §5.5; `git -C /home/felipe/Escritorio/dev/muelle show fase-4-panel:web/CLAUDE.md` (sección "CSP"), `…:web/svelte.config.js` (qué escribe en `csp-header.txt`) y `…:web/scripts/serve-build.mjs` (cómo esperan los e2e que Go sirva el build: es la referencia de comportamiento).

**Files:**
- Create: `src/cmd/http/panel/panel.go`, `embed_panel.go`, `embed_none.go`
- Create: `src/cmd/http/panel/panel_test.go`, `embed_none_test.go`, `embed_panel_test.go`
- Modify: `src/cmd/http/server.go` (campo `Config.Panel`, `Server.panel`, `notMatched`)
- Create: `src/cmd/http/panel_mount_test.go`
- Modify: `src/shared/infra/container/container.go` (el principio de `New` y un parámetro de `build`)
- Create: `src/shared/infra/container/panel_dist_test.go` (`//go:build !panel`)
- Modify: `.gitignore`, `.github/workflows/ci.yaml`, `README.md`

- [x] **Step 1: Tests del handler** (`src/cmd/http/panel/panel_test.go`, `package panel`). Un helper `testBuild()` devuelve un `fstest.MapFS` con:
  - `200.html` (`<!doctype html><title>muelle</title>`), `csp-header.txt` (`default-src 'self'; script-src 'self' 'sha256-abc='; frame-ancestors 'none'` + `"\n"`);
  - `_app/immutable/entry/start.Bx1.js`, `_app/immutable/assets/geist.Cq2.woff2`, `_app/immutable/assets/app.D3.css`, `_app/version.json`, `favicon.svg`, `robots.txt`.

  Tests (con `httptest.NewRecorder` y `httptest.NewRequest`):
  - `TestNewValidatesBuild`: sin `200.html` → error que nombra `200.html`; sin `csp-header.txt` → error que lo nombra; CSP vacía, o con un `\n` en el medio → error. La del helper (con `\n` final) sirve, y el header sale sin el `\n`.
  - `TestServesBuildFiles` (tabla path → status, `Content-Type`, `Cache-Control`, body): el `.js` (`text/javascript; charset=utf-8`, `public, max-age=31536000, immutable`), el `.woff2` (`font/woff2`), el `.css`, `_app/version.json` (`application/json`, `no-cache`), `favicon.svg` (`image/svg+xml`, `no-cache`), `robots.txt` (`text/plain; charset=utf-8`).
  - `TestFallbackToShell`: `/`, `/login`, `/projects/a/stages/b/apps/c/deploys`, `/setup?token=x`, `/csp-header.txt` (no se sirve), `/_app/immutable/../../login` (se limpia), `/%2e%2e/%2e%2e/etc/passwd` y un directorio que existe fuera de `_app` (agrega `assets/x.png` al MapFS y pide `/assets` y `/assets/`) → `200`, el cuerpo del `200.html`, `text/html; charset=utf-8`, `no-cache`.
  - `TestAppMissingIs404`: `/_app/immutable/entry/viejo.js`, `/_app/nada`, `/_app`, `/_app/immutable/` → `404`, `Content-Type` que **no** es `text/html`, el cuerpo no es el del `200.html`, `Cache-Control: no-cache` (nunca `immutable` en un 404).
  - `TestSecurityHeadersEverywhere`: para un archivo, el fallback, un 404 de `/_app/…` y un 405: `Content-Security-Policy` igual al contenido de `csp-header.txt` sin espacios de los bordes, `X-Content-Type-Options: nosniff`, `Referrer-Policy: same-origin`.
  - `TestOnlyGetAndHead`: `POST`, `PUT`, `DELETE`, `PATCH` y `OPTIONS` en `/` → `405`, `Allow: GET, HEAD`, cuerpo `{"error":{"code":"method_not_allowed","message":"método no permitido"}}`. `HEAD /` → `200`, cuerpo vacío, `Content-Length` = largo del `200.html`.
  - `TestETagRevalidates`: `GET /` trae `ETag`; con `If-None-Match` igual → `304`. `favicon.svg` trae `ETag`; el `.js` de `_app/immutable/` no.
  - `TestContentType` (tabla sobre la función `contentType(name)`): `.html`, `.js`, `.mjs`, `.css`, `.json`, `.webmanifest`, `.map`, `.svg`, `.png`, `.jpg`, `.webp`, `.ico`, `.woff`, `.woff2`, `.txt`, `.wasm`, mayúsculas (`X.PNG`), y una extensión desconocida → `application/octet-stream`.

  Run: `go test ./src/cmd/http/panel/` → no compila (el paquete no existe). Es la falla esperada.

- [x] **Step 2: Implementar `panel.go`.** Esqueleto (el comportamiento es el del contrato §5.5):

  ```go
  // Package panel sirve el build del panel (web/, SvelteKit con adapter-static):
  // el archivo del build o el 200.html, siempre con la CSP que escribió el
  // build en csp-header.txt (contrato §5.5). cmd/http lo usa para toda ruta
  // fuera de /api.
  package panel

  // immutablePrefix son los archivos con hash en el nombre: nunca cambian.
  const immutablePrefix = "_app/immutable/"

  type handler struct {
  	fsys  fs.FS
  	csp   string
  	shell []byte
  	etags map[string]string // los archivos fuera de _app/immutable/ (decisión 3)
  }

  // New arma el handler sobre un build (la raíz tiene 200.html y
  // csp-header.txt). Falla si falta alguno, o si la CSP está vacía o no es una
  // sola línea: muelle no arranca con un panel que el navegador bloquearía.
  func New(fsys fs.FS) (http.Handler, error) {
  	// lee 200.html y csp-header.txt (strings.TrimSpace; error si queda "" o
  	// tiene \r o \n); recorre el build con fs.WalkDir saltando
  	// _app/immutable/ (fs.SkipDir) y guarda el ETag de cada archivo:
  	// `"` + hex(sha256[:16]) + `"`.
  }

  func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
  	hd := w.Header()
  	hd.Set("Content-Security-Policy", h.csp)
  	hd.Set("X-Content-Type-Options", "nosniff")
  	hd.Set("Referrer-Policy", "same-origin")
  	hd.Set("Cache-Control", "no-cache")
  	if r.Method != http.MethodGet && r.Method != http.MethodHead {
  		hd.Set("Allow", "GET, HEAD")
  		utils.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "método no permitido")
  		return
  	}
  	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/") // "" es la raíz
  	if f, ok := h.open(name); ok {
  		defer f.Close()
  		h.serveFile(w, r, name, f) // Content-Type de la tabla; immutable o ETag; http.ServeContent
  		return
  	}
  	if name == "_app" || strings.HasPrefix(name, "_app/") {
  		http.Error(w, "no existe", http.StatusNotFound) // un chunk viejo nunca recibe HTML
  		return
  	}
  	// el 200.html, con su ETag, por http.ServeContent (HEAD, Range, If-None-Match)
  }
  ```
  - `open(name)`: `false` si `name` es `""` o `csp-header.txt`, si `!fs.ValidPath(name)`, si no existe o si es un directorio.
  - `serveFile`: si el `fs.File` no es `io.ReadSeeker` (los de `embed` y `fstest` lo son), lo lee entero en un `bytes.Reader`. `http.ServeContent` con `time.Time{}` (sin `Last-Modified`).
  - `cmd/http/panel` puede importar `cmd/http/utils` (el JSON del 405 tiene que ser el mismo de toda la API).

- [x] **Step 3: Los dos `Embedded()`.**
  - `embed_none.go` (`//go:build !panel`): `func Embedded() (http.Handler, error)` devuelve un handler que, para cualquier método y ruta, responde `404` con `Content-Type: text/html; charset=utf-8`, `Content-Security-Policy: default-src 'none'`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: same-origin`, `Cache-Control: no-cache` y una página corta en español que diga que este binario se compiló sin `-tags panel` y que la API está en `/api/v1` (sin cuerpo en `HEAD`).
  - `embed_panel.go` (`//go:build panel`):
    ```go
    //go:embed all:build
    var build embed.FS // sin all: se pierde _app/ (web/CLAUDE.md, CSP)

    func Embedded() (http.Handler, error) {
    	sub, err := fs.Sub(build, "build")
    	if err != nil {
    		return nil, fmt.Errorf("panel: %w", err)
    	}
    	h, err := New(sub)
    	if err != nil {
    		return nil, fmt.Errorf("panel embebido: %w", err)
    	}
    	return h, nil
    }
    ```
  - `embed_none_test.go` (`//go:build !panel`): `TestNotEmbedded` — `GET /x` y `POST /` → `404` con los headers de arriba y un cuerpo que contiene `-tags panel`; `HEAD /` sin cuerpo.
  - `embed_panel_test.go` (`//go:build panel`): `TestEmbedded` — `Embedded()` sin error; `GET /` → `200` con `Content-Security-Policy` igual a `strings.TrimSpace` de `build/csp-header.txt` (leído con `os.ReadFile`).

  Run: `go test -race ./src/cmd/http/panel/` → PASS.

- [x] **Step 4: Montarlo en `cmd/http`.** Test primero, `src/cmd/http/panel_mount_test.go` (`package http`), con un `Server` propio que tiene `Config.Panel` = un stub que responde `200 panel` y guarda cada ruta que recibe, más `GET /fuera-de-api` y `GET /api/v1/ping` registradas:
  - `TestPanelMount`:
    - `GET /x/y`, `GET /`, `GET /apix`, `GET /API/v1` → el stub (y vio esa ruta);
    - `GET /api`, `/api/`, `/api/v1/nada`, `/api/v2`, `/x/../api/v1/nada` → `404 no_route` en JSON, sin llamar al stub;
    - `GET /fuera-de-api` → la ruta registrada; `POST /fuera-de-api` con `Origin` válido → `405` del mux (su `Allow` incluye `GET`), sin el stub;
    - `POST /x` sin `Origin` → `403 bad_origin` sin el stub; con `Origin: https://panel.test` → el stub;
    - las respuestas del stub no llevan `Cache-Control: no-store` (eso es solo de `/api/`).
  - `TestPanelNilKeepsNoRoute`: con `Panel` nil, `GET /otra-cosa` sigue siendo `404 no_route` (ya lo cubre `TestUnmatchedRouteIsJSON`; este test lo deja explícito para el panel).

  Implementación en `server.go` (solo esto):
  ```go
  	// Panel, opcional, sirve el panel (cmd/http/panel) en toda ruta fuera de
  	// /api que no tiene una ruta registrada (contrato §5.5). Con nil, esas
  	// rutas responden 404 no_route, como las de /api.
  	Panel http.Handler
  ```
  `Server` guarda `panel`; en `notMatched`, entre el `405` y el `no_route`:
  ```go
  	if s.panel != nil && !isAPIPath(r.URL.Path) {
  		s.panel.ServeHTTP(w, r)
  		return
  	}
  ```
  con `isAPIPath(p)`: `c := path.Clean(p); c == "/api" || strings.HasPrefix(c, "/api/")`. Actualiza el comentario de `ServeHTTP` (una línea: fuera de `/api`, el panel).

- [x] **Step 5: Conectarlo en el contenedor.** Test primero, `src/shared/infra/container/panel_dist_test.go`, con `//go:build !panel` en la primera línea: con `-tags panel` y un build en su lugar (el flujo "Con el panel" del README, o un revisor que corre los tests con el tag), `GET /` da `200` y el test no aplica. `TestPanelNotEmbedded` — con `newTestContainer(t)`, `GET /` por `c.Server.ServeHTTP` → `404`, `text/html`, cuerpo con `-tags panel`; `GET /api/v1/nada` → `404 no_route`.

  Después, en `container.go`, el panel se lee **al principio de `New`**, justo después de `loadCipher` y antes de `docker.New`, `MkdirAll` y `sqlite.Open`: un embed roto (sin `csp-header.txt`) termina sin haber escrito nada en `DataDir`, igual que una clave que falta.
  ```go
  	// El panel se lee antes de tocar DataDir, como la clave: con un embed
  	// roto, muelle no arranca y la base queda como estaba.
  	panelHandler, err := panel.Embedded()
  	if err != nil {
  		return nil, fmt.Errorf("container: %w", err)
  	}
  ```
  `build` recibe `panelHandler http.Handler` como último parámetro y pone `Panel: panelHandler,` en el `httpserver.Config`. En el comentario de `New`, "La clave se lee antes de tocar DataDir" pasa a "La clave y el panel embebido se leen antes de tocar DataDir". Nada más en `container.go`.

- [x] **Step 6: `.gitignore`, CI y README.**
  - `.gitignore`: `/src/cmd/http/panel/build/`.
  - `.github/workflows/ci.yaml`, un paso nuevo al final del job `test` (después de `build`):
    ```yaml
          - name: panel embebido (-tags panel, con un build de prueba)
            run: |
              mkdir -p src/cmd/http/panel/build/_app/immutable
              printf '<!doctype html><title>muelle</title>\n' > src/cmd/http/panel/build/200.html
              printf "default-src 'self'; frame-ancestors 'none'\n" > src/cmd/http/panel/build/csp-header.txt
              go vet -tags panel ./...
              go test -race -tags panel ./src/cmd/http/... ./src/shared/infra/container/
    ```
    (T1 amplió el `go test` del borrador, que era solo `./src/cmd/http/panel/`, a los mismos paquetes del Step 7: así CI prueba también el contenedor con el panel embebido.)
  - `README.md`, en "Desarrollo local", una subsección **"Con el panel"**: sin `-tags panel` las rutas del panel responden la página de "no trae el panel"; para probarlo, construir `web/` (`pnpm install && pnpm build` en la carpeta del panel), copiar `web/build/.` a `src/cmd/http/panel/build/` y `go run -tags panel . serve`; o `pnpm dev` en `web/`, que proxea `/api` a un `muelle serve` local (web/README.md).

- [x] **Step 7: Verificar.** Las cuatro verificaciones de las convenciones, y además el tag con el mismo build de prueba que CI (bórralo al final; está ignorado, pero no lo dejes):
  ```bash
  cd /home/felipe/Escritorio/dev/muelle/.claude/worktrees/fase-5-dist
  bash -c 'B=src/cmd/http/panel/build; mkdir -p $B/_app/immutable &&
    printf "<!doctype html><title>muelle</title>\n" > $B/200.html &&
    printf "default-src '"'"'self'"'"'; frame-ancestors '"'"'none'"'"'\n" > $B/csp-header.txt &&
    go vet -tags panel ./... && go test -race -tags panel ./src/cmd/http/... ./src/shared/infra/container/; s=$?; rm -rf $B; exit $s'
  ```
  Expected: todo PASS (con el tag, `panel_dist_test.go` no entra y el resto del contenedor pasa con el panel de prueba); `git status` no muestra `src/cmd/http/panel/build/`.

- [x] **Step 8: Commits** (uno por pieza):
  - `feat(panel): servir el build del panel embebido con su CSP, el 200.html de respaldo y la caché por tipo de archivo`
  - `feat(http): montar el panel en toda ruta fuera de /api, después del 405 del mux`
  - `ci: compilar y probar con -tags panel sobre un build de prueba`
  - `docs(readme): probar muelle con el panel embebido`

**Integración:** ninguna (no toca Docker). El build real del panel se prueba en T2, dentro de la imagen.

---

### Task T2: Imágenes, `muelle version` y el workflow de release

Los dos `Dockerfile`, el `.dockerignore` y el workflow que publica las imágenes en los tags `vX.Y.Z`. La imagen de muelle trae el panel embebido, el binario estático con su versión y el CLI de `docker` que necesita `docker stack deploy`.

**Lee antes:** contrato §5.1 (nombres y versión) y §5.4 ("Imágenes" y "Publicación"); diseño §3.1 y §12; `git -C /home/felipe/Escritorio/dev/muelle show fase-4-panel:web/package.json` (`packageManager`, scripts `codegen`, `build`, `prepare`), `…:web/.npmrc`, `…:web/pnpm-workspace.yaml` y `…:web/.gitignore`.

**Files:**
- Modify: `main.go`, `main_test.go` (subcomando `version`)
- Create: `Dockerfile`, `.dockerignore`, `deploy/caddy/Dockerfile`, `.github/workflows/release.yml`
- Modify: `README.md` (sección "Imágenes"), `.github/workflows/ci.yaml` (`workflow_call:`, Step 7)

- [x] **Step 1: `muelle version`.** Tests en `main_test.go`:
  - `TestRunVersion`: `run([]string{"version"}, …)` → código 0, stdout `dev\n`, stderr vacío.
  - En `TestRunUsage`, agregar `{[]string{"version", "extra"}, 2}`.

  Implementación: `case "version"` en `run` (sin argumentos; imprime `version`), y la línea en `usage` (`version  imprime la versión de muelle`). Reordena el `switch` para que cada comando valide sus argumentos en su propio `case` (T3 y T4 agregan `settings` e `install`, que sí reciben argumentos). Commit: `feat: muelle version imprime la versión del binario`.

- [x] **Step 2: Versiones y digests.** Para cada imagen base, fija el tag exacto (con parche) y el digest del índice multi-arquitectura:
  ```bash
  for img in node:24-alpine golang:1.27-alpine docker:29.9.0-cli alpine:3.24 caddy:2.11-builder-alpine caddy:2.11-alpine; do
    docker buildx imagetools inspect "$img" | sed -n '1,3p'   # Name y Digest del índice
  done
  docker run --rm node:24-alpine node --version                 # el tag exacto: node:24.X.Y-alpine3.ZZ
  # sin buildx: docker pull <img> && docker inspect --format '{{index .RepoDigests 0}}' <img>
  ```
  Usa el tag más específico que publique cada imagen (`node:24.X.Y-alpine3.ZZ`, `golang:1.27.X-alpine3.ZZ`, `alpine:3.24.X`, `caddy:2.11.X-builder-alpine` y `caddy:2.11.X-alpine`, con la misma `2.11.X` en las dos). Para `github.com/caddy-dns/cloudflare`, la última versión: `go list -m -versions github.com/caddy-dns/cloudflare` (o Context7 / las releases del repo). Confirma con Context7 (`/caddyserver/caddy-docker`) cómo se usa `xcaddy build` en el builder.

  **Hecho (2026-10-15):** `node:24.22.0-alpine3.24`, `golang:1.27.2-alpine3.24`, `docker:29.9.0-cli`, `alpine:3.24.2`, `caddy:2.11.7-builder-alpine` y `caddy:2.11.7-alpine` (cada uno igual a su tag corto), y `caddy-dns/cloudflare@v0.2.4`. Además, la línea `# syntax` de los dos `Dockerfile` fija el frontend: `docker/dockerfile:1.27.1@sha256:…` (sin el digest, BuildKit bajaría `docker/dockerfile:1` sin fijar en cada build).

- [x] **Step 3: `.dockerignore`** (lista de permitidos, decisión 16):
  ```
  # El contexto lleva solo lo que construye la imagen de muelle: nunca .git,
  # .claude (otros worktrees), claves, bases ni el .env del panel.
  *
  !go.mod
  !go.sum
  !main.go
  !src/
  !web/
  src/cmd/http/panel/build/
  **/*_test.go
  **/testdata/
  web/node_modules/
  web/build/
  web/build-e2e/
  web/.svelte-kit/
  web/test-results/
  web/playwright-report/
  web/src/shared/ui/i18n/paraglide/
  web/.env
  web/.env.*
  ```
  Hecho con lo mismo más `**/*.key`, `**/muelle_master_key`, `**/*.db*` y `**/.env*` dentro de `src/` y `web/` (lo que ignora git), `web/coverage/` y `web/project.inlang/cache/`. Comprobado con un build que lista el contexto: con un `web/.env.production`, una clave, una base, `node_modules`, `web/build` y `src/cmd/http/panel/build` puestos a propósito, ninguno entra.

- [x] **Step 4: `Dockerfile`** (raíz). Esqueleto; reemplaza `<…>` con lo del Step 2:
  ```dockerfile
  # syntax=docker/dockerfile:1
  # Imagen de muelle (contrato §5.4): el panel (web/) construido con pnpm, el
  # binario estático con el panel embebido (-tags panel) y el CLI de docker
  # para docker stack deploy. Desde la raíz del repo, con web/ presente:
  #   docker build --build-arg VERSION=v0.1.0 -t ghcr.io/cpu-commits/muelle:v0.1.0 .

  FROM node:<24.X.Y-alpine3.ZZ>@sha256:<…> AS web
  WORKDIR /web
  # Nunca un build con MSW: el proceso manda sobre cualquier .env (que tampoco entra al contexto).
  ENV VITE_USE_MOCKS=0 CI=true
  RUN npm install -g pnpm@11.1.2
  COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml web/.npmrc ./
  # El prepare (pnpm codegen) falla acá porque todavía no están las fuentes; el
  # script lo tolera, y el codegen corre de nuevo abajo.
  RUN pnpm install --frozen-lockfile
  COPY web/ ./
  RUN pnpm codegen && pnpm build \
   && test -f build/200.html && test -f build/csp-header.txt && test ! -e build/mockServiceWorker.js

  FROM golang:<1.27.X-alpine3.ZZ>@sha256:<…> AS go
  WORKDIR /src
  COPY go.mod go.sum ./
  RUN go mod download
  COPY main.go ./
  COPY src/ ./src/
  COPY --from=web /web/build/ ./src/cmd/http/panel/build/
  ARG VERSION=dev
  RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -tags panel \
        -ldflags "-s -w -X main.version=${VERSION}" -o /out/muelle .

  FROM docker:29.9.0-cli@sha256:<…> AS docker-cli

  FROM alpine:<3.24.X>@sha256:<…>
  # Alpine trae /tmp con 1777 (DeployStack escribe ahí su DOCKER_CONFIG) y cat
  # (para releer la clave maestra, §12 del diseño). Corre como root: usa docker.sock.
  RUN apk add --no-cache ca-certificates
  COPY --from=docker-cli /usr/local/bin/docker /usr/local/bin/docker
  COPY --from=go /out/muelle /usr/local/bin/muelle
  ARG VERSION=dev
  LABEL org.opencontainers.image.source="https://github.com/CPU-commits/muelle" \
        org.opencontainers.image.title="muelle" \
        org.opencontainers.image.version="${VERSION}" \
        org.opencontainers.image.description="Deploy manager ultraligero sobre Docker Swarm"
  ENTRYPOINT ["muelle"]
  CMD ["serve"]
  ```
  Si `pnpm codegen` o `pnpm build` piden otra cosa (una variable, un archivo), resuélvelo en el `Dockerfile`, nunca editando `web/`; si no se puede, para y repórtalo.

  **Cambios al esqueleto:** `go build` usa `GOOS=${TARGETOS} GOARCH=${TARGETARCH}` (los `ARG` que define BuildKit) en vez de `linux`/`amd64` fijos: el release los pasa como `linux/amd64` igual, y un build local en otra arquitectura no mete un binario amd64 en una imagen arm64. La CSP se comprueba con `test -s` (no vacía), y la línea `# syntax` va fijada (Step 2). `pnpm codegen` y `pnpm build` no pidieron nada más.

- [x] **Step 5: Construir y probar la imagen de muelle.** Arma el contexto como en las convenciones y construye `it5-muelle:test` con `VERSION=v0.0.0-it5`. Después, con nombres `it5` y el puerto 22080 solo en loopback (es un test: en producción el puerto de muelle nunca se publica). Abajo se omite por brevedad, pero cada `docker run --rm` (también los de los Steps 2 y 6) lleva su `--name it5-<algo>` (convenciones):
  ```bash
  docker run --rm it5-muelle:test version                          # v0.0.0-it5
  docker run --rm it5-muelle:test help | head -3                   # el uso
  docker run --rm --entrypoint docker it5-muelle:test --version    # Docker version 29.9.0, …
  docker run --rm --entrypoint sh it5-muelle:test -c 'stat -c %a /tmp; test -s /etc/ssl/certs/ca-certificates.crt && echo ca-ok'
  #                                                                # 1777 y ca-ok
  (umask 077; docker run --rm it5-muelle:test genkey > "$CTX/it5.key")
  docker run -d --name it5-smoke -p 127.0.0.1:22080:8080 \
    -e MUELLE_PANEL_ORIGIN=http://127.0.0.1:22080 -e MUELLE_SECURE_COOKIES=false \
    -v "$CTX/it5.key:/run/secrets/muelle_master_key:ro" --tmpfs /data it5-muelle:test
  # espera a que responda (hasta 20 s), y después:
  curl -sS -D - -o /dev/null http://127.0.0.1:22080/
  #   200, Content-Type text/html, Cache-Control: no-cache, Referrer-Policy: same-origin, y un
  #   Content-Security-Policy con frame-ancestors 'none' y el mismo script-src (con su sha256-)
  #   que el <meta http-equiv="content-security-policy"> del HTML (curl -s …/ | grep -o 'content-security-policy[^>]*')
  curl -sS -D - -o /dev/null "http://127.0.0.1:22080$(curl -s http://127.0.0.1:22080/ | grep -o '/_app/immutable/[^"]*\.js' | head -1)"
  #   200, text/javascript; charset=utf-8, Cache-Control: public, max-age=31536000, immutable
  curl -sS http://127.0.0.1:22080/api/v1/nada                       # {"error":{"code":"no_route",…}}
  curl -sS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:22080/_app/immutable/no-existe.js   # 404
  curl -sS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:22080/mockServiceWorker.js
  #   200 con el 200.html (no existe en el build), nunca el worker de MSW
  docker logs it5-smoke | head -5                                    # el JSON de arranque, con event=setup_url
  docker stats --no-stream --format '{{.MemUsage}}' it5-smoke        # < 40MiB (§1 del diseño); anótalo para el Step 6
  docker rm -f it5-smoke && rm -f "$CTX/it5.key"
  docker image ls it5-muelle:test                                    # anota el tamaño en el commit
  ```
  Si el RSS pasa de 40 MiB, no lo arregles en esta tarea: anótalo en el reporte con el valor.

  **Resultado (2026-10-15, build sin caché):** todo como arriba. El binario y el `docker` son estáticos; `/tmp` 1777; la CSP del header es la del `<meta>` más `frame-ancestors 'none'`; `POST /` con `Origin` da `405` con `Allow: GET, HEAD`. Memoria de `it5-smoke` después de cargar el panel: 14,3 MiB en `docker stats` (VmRSS 25,7 MiB). Imagen: 102 MB en disco, 30,8 MB comprimida.

- [x] **Step 6: `deploy/caddy/Dockerfile`** y su prueba:
  ```dockerfile
  # syntax=docker/dockerfile:1
  # muelle-caddy (contrato §3.4 y §5.4): Caddy con el módulo de DNS de
  # Cloudflare, en versiones fijas. Sin CMD ni configuración propios: ingress
  # crea el contenedor con `caddy run --resume` y CADDY_ADMIN en el socket.
  FROM caddy:<2.11.X>-builder-alpine@sha256:<…> AS builder
  RUN xcaddy build v<2.11.X> --with github.com/caddy-dns/cloudflare@<vA.B.C>

  FROM caddy:<2.11.X>-alpine@sha256:<…>
  COPY --from=builder /usr/bin/caddy /usr/bin/caddy
  LABEL org.opencontainers.image.source="https://github.com/CPU-commits/muelle" \
        org.opencontainers.image.title="muelle-caddy"
  ```
  ```bash
  docker build -t it5-muelle-caddy:test "$CTX/deploy/caddy"
  docker run --rm it5-muelle-caddy:test caddy version                                   # v2.11.X
  docker run --rm it5-muelle-caddy:test caddy list-modules | grep -x dns.providers.cloudflare
  # Arranca como lo crea ingress (naming.CaddyCommand y CaddyEnv), sin publicar puertos:
  docker run -d --name it5-caddy-smoke -e CADDY_ADMIN=unix//run/caddy-admin/admin.sock \
    -e XDG_CONFIG_HOME=/config -e XDG_DATA_HOME=/data --tmpfs /run/caddy-admin \
    it5-muelle-caddy:test caddy run --resume
  docker exec it5-caddy-smoke curl -s --unix-socket /run/caddy-admin/admin.sock http://localhost/config/
  #   null (Caddy vacío, con la admin API en el socket)
  docker stats --no-stream --format '{{.MemUsage}}' it5-caddy-smoke  # la de Caddy vacío
  docker rm -f it5-caddy-smoke
  ```
  El otro objetivo de §1 del diseño es muelle más Caddy bajo 100 MB: en el reporte (y en el commit de la imagen de Caddy) anota la memoria de `it5-caddy-smoke`, la de `it5-smoke` del Step 5 y la suma. Es una cota de abajo (Caddy sin sitios ni certificados); si la suma ya pasa de 100 MB, no lo arregles en esta tarea: anótalo con los valores.

  **Hecho:** además del esqueleto, el `Dockerfile` corre `caddy list-modules | grep -qx dns.providers.cloudflare` (el build falla sin el módulo) y toma `ARG VERSION` para la etiqueta `org.opencontainers.image.version`, que si no quedaría con la versión de Caddy de la imagen base (el release le pasa el mismo `VERSION` que a muelle). **Resultado:** `v2.11.7`, el módulo está, `/config/` responde `null` por el socket y no hay admin API en TCP. Memoria de `it5-caddy-smoke`: 20,0 MiB (`docker stats`; VmRSS 47,6 MiB). Con muelle: 34,3 MiB (`docker stats`) o 73,3 MiB (VmRSS), bajo los 100 MB. Imagen: 163 MB en disco, 43 MB comprimida.

- [x] **Step 7: `.github/workflows/release.yml`.** No lo corras nunca (ni con `act`). Fija `actions/checkout`, `actions/setup-go` y las tres de `docker/` por SHA de commit, con la versión en un comentario (`git ls-remote --tags https://github.com/docker/build-push-action | sort -V -k2 | tail -3`; en un tag anotado, la línea `^{}` es el commit). Confirma las entradas con Context7 (`/docker/build-push-action`, `/docker/login-action`).
  ```yaml
  name: Release

  # Construye y sube ghcr.io/cpu-commits/muelle y muelle-caddy con el tag de git
  # (contrato §5.4). Con workflow_dispatch construye sin subir.
  on:
    push:
      tags: ['v[0-9]+.[0-9]+.[0-9]+*']
    workflow_dispatch:

  permissions:
    contents: read

  jobs:
    test:
      runs-on: ubuntu-latest
      steps:
        # los mismos pasos que ci.yaml: checkout, setup-go, gofmt, go mod tidy -diff, vet, test -race
    images:
      needs: test
      runs-on: ubuntu-latest
      permissions:
        contents: read
        packages: write
      steps:
        - uses: actions/checkout@<sha> # vN
        - name: versión y tags
          id: v
          run: |
            # Solo un push de un tag sube; workflow_dispatch nunca (aunque se elija un tag).
            if [[ "$GITHUB_EVENT_NAME" == push && "$GITHUB_REF" == refs/tags/* ]]; then
              v="$GITHUB_REF_NAME"; push=true
              [[ "$v" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || { echo "::error::el tag $v no es vX.Y.Z[-sufijo]"; exit 1; }
            else
              v=dev; push=false
            fi
            muelle="ghcr.io/cpu-commits/muelle:$v"
            [[ "$push" == true && "$v" != *-* ]] && muelle="$muelle,ghcr.io/cpu-commits/muelle:latest"
            { echo "version=$v"; echo "push=$push"; echo "muelle=$muelle"; echo "caddy=ghcr.io/cpu-commits/muelle-caddy:$v"; } >> "$GITHUB_OUTPUT"
        - uses: docker/setup-buildx-action@<sha> # vN
        - if: steps.v.outputs.push == 'true'
          uses: docker/login-action@<sha> # vN
          with:
            registry: ghcr.io
            username: ${{ github.actor }}
            password: ${{ secrets.GITHUB_TOKEN }}
        - uses: docker/build-push-action@<sha> # v7.x
          with:
            context: .
            platforms: linux/amd64
            build-args: VERSION=${{ steps.v.outputs.version }}
            tags: ${{ steps.v.outputs.muelle }}
            push: ${{ steps.v.outputs.push == 'true' }}
            cache-from: type=gha,scope=muelle
            cache-to: type=gha,mode=max,scope=muelle
        - uses: docker/build-push-action@<sha> # v7.x
          with:
            context: deploy/caddy
            platforms: linux/amd64
            tags: ${{ steps.v.outputs.caddy }}
            push: ${{ steps.v.outputs.push == 'true' }}
            cache-from: type=gha,scope=caddy
            cache-to: type=gha,mode=max,scope=caddy
  ```
  - Ningún `${{ … }}` con datos del evento va dentro de un `run:` (el tag se lee de `$GITHUB_REF_NAME`).
  - Verifica la sintaxis con `actionlint` si está en la máquina; si no, revisa el YAML a mano (indentación, `on:`, `needs:`). No instales herramientas nuevas para esto.

  **Cambios al esqueleto:**
  - El job `test` es `uses: ./.github/workflows/ci.yaml`, y `ci.yaml` suma `workflow_call:` a su `on:` (una línea). Así el release corre exactamente los pasos de CI, también los que agregue T5 (shellcheck), en vez de una copia que se desincroniza.
  - El checkout va con `persist-credentials: false`.
  - Las dos imágenes llevan `org.opencontainers.image.revision` (el `github.sha`), y muelle-caddy también recibe `VERSION` (Step 6).
  - Versiones fijadas: `actions/checkout` v7.0.1, `docker/setup-buildx-action` v4.4.1, `docker/login-action` v4.6.0 y `docker/build-push-action` v7.4.0 (los cuatro tags son livianos: el SHA es el commit). `actions/setup-go` no hace falta en este archivo.
  - `actionlint` no está en la máquina: el YAML se revisó con el parser de Ruby, y el script del paso `versión y tags` se corrió en bash con eventos simulados (tag final, `-rc.1`, tags que no sirven y `workflow_dispatch`).

- [x] **Step 8: README**, sección **"Imágenes"** (después de "Tests"): qué trae cada imagen, que se publican con el mismo tag `vX.Y.Z` (`:latest` solo muelle, solo versiones finales), cómo construirlas en local desde la raíz con `web/` presente (`docker build --build-arg VERSION=… .` y `docker build deploy/caddy`), y que hasta integrar el panel (`fase-4-panel`) el contexto se arma con su `web/` aparte.

- [x] **Step 9: Verificar y commits.** Las cuatro verificaciones de Go. Commits:
  - `build: imagen de muelle con el panel embebido, el binario estático y el CLI de docker`
  - `build: imagen de muelle-caddy con xcaddy y el DNS de Cloudflare, en versiones fijas`
  - `ci: workflow de release que publica las dos imágenes en los tags vX.Y.Z`
  - `docs(readme): las imágenes de muelle y cómo construirlas`

**Integración:** los smoke tests del Step 5 y el Step 6, a mano y con nombres `it5`. Al terminar no queda ningún contenedor `it5-*`. Las imágenes `it5-muelle:test` e `it5-muelle-caddy:test` quedan (T4 reconstruye la primera; T5 borra las dos). Borra `$CTX`.

---

### Task T3: Recuperar el acceso (`muelle settings reset-panel-ips`)

El subcomando que vacía `panel_allowed_cidrs` desde dentro del contenedor y reinicia el propio servicio, más el puerto de Docker que lo reinicia.

**Lee antes:** contrato §4.1 ("Recuperar el acceso") y §5.4 (`reset-panel-ips`, después de `f87d709`); `src/internal/admin/app/usecases/settings.go` (cómo `PatchSettings` guarda la lista y su auditoría); `src/shared/infra/out/docker/client.go` (`wrap`) y `client_test.go` (`newFakeAPIClient`).

**Files:**
- Create: `src/shared/domain/naming/install.go` (por ahora, `LabelSwarmService`)
- Create: `src/shared/infra/out/docker/dist.go`, `client_dist.go`, `client_dist_test.go`, `integration_dist_test.go`
- Create: `src/shared/infra/out/docker/dockertest/fake_dist.go`, `fake_dist_test.go`
- Create: `src/internal/admin/reset_panel_ips.go`, `reset_panel_ips_test.go`
- Create: `src/cmd/settingscli/settingscli.go`, `settingscli_test.go`
- Modify: `main.go`, `main_test.go`, `README.md`

- [x] **Step 1: `naming.LabelSwarmService`.** En `src/shared/domain/naming/install.go` (archivo nuevo, para no tocar `naming.go`):
  ```go
  // LabelSwarmService la pone Swarm en cada contenedor de una tarea, con el
  // nombre del servicio. muelle settings reset-panel-ips la lee en su propio
  // contenedor para saber qué servicio reiniciar (contrato §5.1 y §5.4).
  const LabelSwarmService = "com.docker.swarm.service.name"
  ```

- [x] **Step 2: `docker.ServiceRestarter`, tests primero.**
  - `client_dist_test.go` (`package docker`, con `newFakeAPIClient`):
    - `TestForceUpdateService`: `GET /services/muelle_muelle` responde un `swarm.Service` con `ID: "svc1"`, `Version.Index: 7`, `Spec.Name`, `Spec.Labels`, `Spec.TaskTemplate.ContainerSpec.Image` y `ForceUpdate: 2`. `POST /services/svc1/update` responde `{}`. Asserts: el update llegó con la query `version=7` y `registryAuthFrom=spec` (las credenciales que el servicio ya tiene, como `docker service update` sin `--with-registry-auth`: con las imágenes privadas, sin ellas Swarm no podría bajar la imagen al recrear la tarea), y el cuerpo (un `swarm.ServiceSpec`) trae `TaskTemplate.ForceUpdate == 3` y el resto del spec igual (nombre, etiquetas, imagen).
    - `TestForceUpdateServiceNotFound`: `GET` responde `404` → `errors.Is(err, ErrNotFound)`, y no hubo `POST`.
  - `dockertest/fake_dist_test.go`: `TestFakeForceUpdateService` (`Version` sube en 1 y `Calls()` registra `ForceUpdateService <nombre>`; un servicio que no existe → `ErrNotFound`; `SetUnavailable(true)` → `ErrUnavailable`; sin Swarm activo → `ErrUnavailable`). Nada de `PutContainer` ni de su test: es de la Fase 2 (`fake_deploy.go`), y declararlo acá rompería la compilación al integrar.

  Run: `go test ./src/shared/infra/out/docker/...` → no compila. Implementación:
  - `dist.go`:
    ```go
    // ServiceRestarter reinicia las tareas de un servicio sin cambiar su spec,
    // como docker service update --force (Fase 5: muelle settings
    // reset-panel-ips, contrato §5.4). No es parte de Engine: lo pide solo
    // quien lo usa.
    type ServiceRestarter interface {
    	// ForceUpdateService sube TaskTemplate.ForceUpdate y actualiza el
    	// servicio con la versión que leyó: Swarm reemplaza sus tareas según su
    	// update_config. ErrNotFound si no existe.
    	ForceUpdateService(ctx context.Context, nameOrID string) error
    }
    ```
  - `client_dist.go`:
    ```go
    var _ ServiceRestarter = (*Client)(nil)

    func (c *Client) ForceUpdateService(ctx context.Context, nameOrID string) error {
    	res, err := c.api.ServiceInspect(ctx, nameOrID, client.ServiceInspectOptions{})
    	if err != nil {
    		return wrap("inspeccionando el servicio "+nameOrID, err)
    	}
    	spec := res.Service.Spec
    	spec.TaskTemplate.ForceUpdate++
    	_, err = c.api.ServiceUpdate(ctx, res.Service.ID, client.ServiceUpdateOptions{
    		Version: res.Service.Version, Spec: spec, RegistryAuthFrom: swarm.RegistryAuthFromSpec,
    	})
    	return wrap("reiniciando el servicio "+nameOrID, err)
    }
    ```
    (Sin `QueryRegistry`: no vuelve a resolver la imagen; el stack se desplegó con `--resolve-image=never`.)
  - `fake_dist.go` (sin campos nuevos en `Fake`, decisión 11):
    ```go
    var _ docker.ServiceRestarter = (*Fake)(nil)

    // ForceUpdateService sube Version del servicio, como un update del daemon.
    // Las tareas nuevas las pone el test con SetTasks, si las necesita.
    func (f *Fake) ForceUpdateService(ctx context.Context, nameOrID string) error // begin, swarm, findService, Version++, UpdatedAt = Now()
    ```
    (El contenedor de una tarea, con sus etiquetas, los tests lo crean con `CreateContainer`: ver el Step 5.)

- [x] **Step 3: Integración del puerto** (`integration_dist_test.go`, `//go:build integration`, `package docker_test`). Reusa `realClient`, `requireLocalImage`, `serviceImage`, `eventually` y `removeNetworkEventually` del archivo de `ctr23`, pero con su propio prefijo:
  ```go
  const distPrefix = "it5"
  func distName(t *testing.T) string // "it5" + 8 hex de crypto/rand
  ```
  `TestIntegrationDistForceUpdate`:
  - despliega con `DeployStack` el stack `distName(t)` con un servicio `app` (`serviceImage`, `entrypoint: ["sh", "-c", "trap 'exit 0' TERM; sleep 600 & wait"]`, que termina enseguida con el `SIGTERM` de Swarm, `deploy.labels: {muelle.test: it5}`, `memory: 32M`) en la red por defecto del stack;
  - `t.Cleanup`: `RemoveStack`, esperar (60 s cada uno) a que no queden servicios con `com.docker.stack.namespace=<stack>` ni los contenedores de las tareas que vio el test, `removeNetworkEventually(<stack>_default)` y esperar a que la red ya no exista (el nodo la sigue viendo unos segundos);
  - espera la tarea corriendo; `InspectContainer(task.ContainerID[:12])` (como `os.Hostname` dentro del contenedor) trae `Labels[naming.LabelSwarmService] == <stack>_app`;
  - `ForceUpdateService(<stack>_app)` → `Version` sube y, antes de 90 s, hay una tarea corriendo con otro id;
  - `ForceUpdateService("it5-no-existe")` → `ErrNotFound`.

  ```bash
  cd /home/felipe/Escritorio/dev/muelle/.claude/worktrees/fase-5-dist
  go test -tags integration -count=1 -run '^TestIntegrationDist' -v ./src/shared/infra/out/docker/
  docker ps -a --filter name=it5 --format '{{.Names}}'; docker network ls --filter name=it5 -q   # nada
  ```

- [x] **Step 4: `admin.ResetPanelAllowedCIDRs`, test primero** (`src/internal/admin/reset_panel_ips_test.go`, `package admin`, con `newEnv(t)` de `module_test.go`):
  - `TestResetPanelAllowedCIDRs` (tabla sobre el valor guardado antes): sin la clave → `Changed: false`; `[]`, `""` y `null` → `Changed: false` (los tres son "sin restricción" para `entity.DecodeProxySettings`, y no se tocan); `["190.5.6.0/24","2001:db8::/32"]` → `Changed: true`, `Previous` con los dos; `basura` (no es JSON) → `Changed: true`, `Previous` nil. Después de cada `Changed: true`, el valor guardado es `[]`.
  - `TestResetPanelAllowedCIDRsAudit`: con cambio, la última fila de `audit_events` es `actor_type = 'system'`, `action = 'settings.update'`, `target = 'settings'`, `details = {"panel_allowed_cidrs":[],"via":"cli"}`. Sin cambio, no se agrega ninguna fila (cuenta antes y después).
  - `TestResetPanelAllowedCIDRsKeepsOtherSettings`: con `proxy_https_port = 8443` guardado antes, sigue igual; y `GET /api/v1/settings` (con `e.c` y la sesión `sin`) devuelve `panel_allowed_cidrs: []`.

  Implementación, en `src/internal/admin/reset_panel_ips.go` (archivo nuevo, en el paquete del módulo; no toca otro archivo de `admin`):
  ```go
  // ResetResult es lo que hizo ResetPanelAllowedCIDRs.
  type ResetResult struct {
  	Changed  bool     // false: la lista ya estaba vacía y no se escribió nada
  	Previous []string // los rangos que tenía; nil si el valor guardado no se entendía
  }

  // ResetPanelAllowedCIDRs deja panel_allowed_cidrs en [] (muelle settings
  // reset-panel-ips, contrato §4.1 y §5.4), en una transacción con su evento
  // settings.update (actor system: el ctx no trae Principal). No necesita la
  // clave maestra: la lista no va cifrada. Un valor que no es una lista JSON
  // también se reemplaza.
  func ResetPanelAllowedCIDRs(ctx context.Context, db *sqlite.DB, rec audit.Recorder) (ResetResult, error)
  ```
  Dentro de `db.InTx`: `adminrepo.NewSettingsRepository(db).Get(ctx, shared.SettingPanelAllowedCIDRs)`; `ErrNotFound` o una lista vacía → nada; si no, `Set(…, []byte("[]"), false)` y `rec.Record(ctx, "settings.update", "settings", map[string]any{shared.SettingPanelAllowedCIDRs: []string{}, "via": "cli"})`.

  **Desviación de §4.1 (aceptada, a revisar al integrar):** en el diseño, la lógica de un caso de uso va en `app/usecases` (un struct con `Execute`) y la raíz del módulo solo la expone. Esta función lee y escribe el repositorio y la auditoría en la raíz (`package admin`) porque el alcance de esta línea es **un solo archivo nuevo** en `src/internal/admin` (más su test), y `app/usecases` es de los dueños de admin. Al integrar, el dueño de admin puede moverla a `usecases.ResetPanelAllowedCIDRs` (con `Execute(ctx) (ResetResult, error)`) y dejar `admin.ResetPanelAllowedCIDRs` como fachada de dos líneas, sin cambiar su firma ni `settingscli`. El comentario de la función lo dice en una línea (`// Vive acá y no en app/usecases solo por el alcance de la Fase 5: ver el plan, T3 Step 4.`).

- [x] **Step 5: `settingscli`, tests primero** (`src/cmd/settingscli/settingscli_test.go`). Un helper arma:
  - un `DataDir` temporal con `muelle.db` migrada (`sqlite.Open` + `Migrate`, y un `Set` del valor inicial con el repositorio de admin o un `INSERT` directo);
  - un `dockertest.Fake` con el servicio `muelle_muelle` (`PutService`) y el contenedor de su tarea, creado con lo que el fake ya tiene: `SetImagePresent("ghcr.io/cpu-commits/muelle:v0.1.0")` y `CreateContainer(ctx, docker.ContainerSpec{Name: "muelle_muelle.1.abc", Image: …, Labels: map[string]string{naming.LabelSwarmService: "muelle_muelle"}})`, que devuelve el id (no `PutContainer`, que es de la Fase 2);
  - `Deps` con `Hostname` que devuelve **ese id** (en Swarm, `os.Hostname` es el id corto del contenedor, e `InspectContainer` lo encuentra igual) y un `Docker` que cuenta cuántas veces se llamó.
  - `TestRunUsage`: `nil` → 2 (uso en stderr); `["x"]` → 2; `["reset-panel-ips", "extra"]` → 2; `["reset-panel-ips", "--nope"]` → 2; `["-h"]`, `["--help"]`, `["help"]` y `["reset-panel-ips", "-h"]` → 0 (uso en stdout). Ninguno lee `DataDir`, `Hostname` ni arma Docker.
  - `TestResetClearsAndRestarts`: lista `["190.5.6.0/24"]` → 0; stdout nombra el rango quitado y `reiniciando muelle_muelle`, y ese aviso ya estaba en stdout cuando se llamó a `ForceUpdateService` (un `Engine` del test que envuelve al fake lo mira); la base queda en `[]`; `Calls()` tiene `ForceUpdateService muelle_muelle`; Docker se armó y se cerró una vez.
  - `TestResetEmptyListStillRestarts`: lista vacía → 0; stdout dice que ya estaba vacía y que reinicia igual; `ForceUpdateService` se llamó; ninguna fila nueva en `audit_events`.
  - `TestResetNoRestart`: `--no-restart` → 0; `Docker` nunca se llamó; stdout trae `docker service update --force muelle_muelle`.
  - `TestResetUsesServiceFromLabel`: la etiqueta dice `it5abc_muelle` (y el fake tiene ese servicio) → reinicia ese, y el mensaje lo nombra.
  - `TestResetRestartFails`: (a) el contenedor no tiene la etiqueta (otro `CreateContainer` sin `Labels`, y `Hostname` devuelve su id); (b) `Hostname` devuelve un id que no está entre los contenedores; (c) `SetUnavailable(true)`; además `Hostname` con error, `Docker()` con error, `ForceUpdateService` que falla con `ErrNotFound`, y uno que falla con el servicio de la etiqueta `it5abc_muelle` (el comando a mano lo nombra) → 1 en todos, stderr trae `docker service update --force <servicio>`, y la base igual quedó en `[]`.
  - `TestResetOutsideContainer`: `DataDir` devuelve error → 1, stderr con `docker exec`; un `DataDir` sin `muelle.db` → 1, stderr con `docker exec`, y **no** se creó el archivo.

  Implementación (`settingscli.go`):
  ```go
  // Package settingscli es `muelle settings <subcomando>`. Por ahora solo
  // reset-panel-ips, que recupera el acceso al panel (contrato §4.1 y §5.4).
  // Corre dentro del contenedor de muelle, con su entorno.
  package settingscli

  const Usage = `uso: muelle settings <subcomando>

  subcomandos:
    reset-panel-ips [--no-restart]   vacía la lista de IPs permitidas del panel y reinicia muelle

  Corre dentro del contenedor de muelle:
    docker exec "$(docker ps -q -f label=com.docker.swarm.service.name=muelle_muelle | head -n1)" muelle settings reset-panel-ips`

  // Engine es lo que usa de Docker: su propio contenedor y reiniciar su servicio.
  type Engine interface {
  	InspectContainer(ctx context.Context, nameOrID string) (docker.Container, error)
  	docker.ServiceRestarter
  }

  type Deps struct {
  	DataDir  func() (string, error)  // MUELLE_DATA_DIR (main: settings.Load)
  	Hostname func() (string, error)  // os.Hostname: en una tarea de Swarm, el id corto del contenedor
  	Docker   func() (e Engine, closeFn func() error, err error) // se arma recién al reiniciar
  	Log      *slog.Logger            // los errores de la auditoría
  }

  // Run ejecuta `muelle settings <args>` y devuelve el código de salida:
  // 0 bien, 1 si algo falló, 2 si los argumentos no sirven.
  func Run(ctx context.Context, args []string, d Deps, stdout, stderr io.Writer) int
  ```
  `reset-panel-ips`, en orden: flags (`flag.NewFlagSet` con `ContinueOnError` y salida descartada; `--no-restart`); `DataDir`; `os.Stat(<dir>/muelle.db)` (si falta, o no es un archivo, no abre nada: `sqlite.Open` lo crearía); `sqlite.Open` sin `Migrate`; `admin.ResetPanelAllowedCIDRs` con `sqlite.NewAuditRecorder(db, clock.System{}, d.Log)`; el mensaje; con `--no-restart`, el comando y 0; si no, `Docker()`, `Hostname()`, `InspectContainer(hostname)`, la etiqueta, **primero** el mensaje `reiniciando <servicio>…` (Swarm puede cortar este `docker exec` al detener la tarea) y después `ForceUpdateService`. Cada error dice qué pasó y el comando a mano: `docker service update --force <servicio>` (con el de la etiqueta si se leyó, si no `naming.MuelleService`). Las llamadas a Docker tienen un plazo de 30 s; un `closeFn` nil no se llama. Los mensajes dependen de `Changed`: con la lista ya vacía no dicen "Caddy sigue filtrando", sino que el reinicio le reenvía la configuración a Caddy.

- [x] **Step 6: `main.go`.** Tests en `main_test.go`:
  - en `TestRunUsage`: `{"settings"}` → 2, `{"settings", "x"}` → 2, `{"settings", "-h"}` → 0. Estos muestran `settingscli.Usage`, no el uso de `main`: la tabla gana un campo `use` (vacío es `usage`);
  - `TestRunSettingsOutsideContainer`: con `serveEnv` y después `t.Setenv("MUELLE_PANEL_ORIGIN", "")`, `run({"settings", "reset-panel-ips"})` → 1, stderr con `MUELLE_PANEL_ORIGIN` y `docker exec`, y no se creó `DataDir`.

  Implementación: `case "settings"` en `run`, con `signal.NotifyContext` y `settingscli.Run(ctx, extra, settingscli.Deps{…}, stdout, stderr)`: `DataDir` llama a `settings.Load()` y devuelve `s.DataDir`; `Hostname: os.Hostname`; `Docker` arma `docker.New(docker.Options{})`; `Log` es un `slog` de texto a stderr. Agrega `settings` al `usage`. (El error de `settings.Load` ya dice qué variable falta; `settingscli` le agrega la pista de `docker exec`.)

- [x] **Step 7: README**, sección **"Recuperar el acceso"** (nueva, después de "Solo detrás de Caddy"):
  - si la lista de IPs permitidas te deja fuera: el `docker exec … muelle settings reset-panel-ips` del contrato §5.4, qué hace (vacía la lista, reinicia muelle, unos segundos sin panel) y `--no-restart`;
  - si el problema son los puertos del proxy (muelle-caddy no arranca en los nuevos): el `docker run --rm -v muelle_data:/data alpine …sqlite3 … UPDATE settings …` del contrato §4.1 con los puertos anteriores (probado sobre un volumen `it5`), y después `docker service update --force muelle_muelle` (o esperar el job `"proxy"`). `muelle settings reset-panel-ips` también reinicia, pero de paso vacía la lista de IPs: el README lo ofrece solo si la lista también te dejó fuera;
  - en "Estructura", `settings` en la línea de `main.go` y `cmd/settingscli/`.

- [x] **Step 8: Verificar y commits.**
  - `feat(docker): reiniciar un servicio como docker service update --force (ServiceRestarter)`
  - `feat(admin): vaciar la lista de IPs permitidas del panel desde la línea de comandos`
  - `feat: muelle settings reset-panel-ips recupera el acceso al panel y reinicia muelle`
  - `docs(readme): recuperar el acceso al panel`

**Integración:** `TestIntegrationDistForceUpdate` (Step 3), solo con `-run '^TestIntegrationDist'`. `reset-panel-ips` dentro del contenedor de verdad se prueba en T4, después de instalar.

---

### Task T4: `muelle install`

> **Antes de empezar:** lee "Cambio de requisito del dueño (2026-10-15): las imágenes son PRIVADAS" (antes del Mapa de archivos). Es parte de esta tarea y cambia los pasos 5 y 8, los flags, el compose del stack, los tests y el README.

El instalador de §5.4 del contrato: valida, revisa Docker y una instalación anterior, detecta la IP pública, baja la imagen, prueba los puertos, inicia el Swarm si hace falta, crea la red, los secretos y el stack, imprime la clave y espera el link de setup.

**Lee antes, entero:** contrato §5.1–§5.4 (orden, instalación anterior, puertos, red, clave, link de setup, salida); diseño §3.3, §12 y §16. `main.go` (`logSetupURL`, y la línea de "muelle escuchando"). `dockertest/fake.go` (`OnDeployStack`, `PutStackService`, `SetTasks`, `AppendTaskLogs`, `BusyHostPort`, `SetInfo`, `SetImagePresent`, `CreateContainer`, `FailNext`). `FailNext` falla la **próxima** llamada a un método, sea cual sea su argumento: para fallar una llamada en particular (el segundo `CreateSecret`), el test envuelve el fake en un tipo propio de `install_test.go` que embebe `*dockertest.Fake` y reemplaza ese método (abajo).

**Files:**
- Modify: `src/shared/domain/naming/install.go`; Create: `src/shared/domain/naming/install_registry.go`, `install_test.go`
- Modify: `src/shared/infra/out/docker/dockertest/fake_dist.go`, `fake_dist_test.go` (`PutNetwork`)
- Create: `src/cmd/install/options.go`, `install.go`, `stack.go`, `setup.go`, `publicip.go`
- Create: `src/cmd/install/options_test.go`, `install_test.go`, `stack_test.go`, `setup_test.go`, `publicip_test.go`, `testdata/stack.golden.json`, `testdata/stack_token.golden.json`
- Create: `src/cmd/install/integration_test.go`
- Modify: `main.go`, `main_test.go`, `README.md`

- [x] **Step 1: Los nombres** (`naming/install.go`, y `InitRegistrySecret = "muelle_init_registry"` en el archivo nuevo `naming/install_registry.go`), test primero (`install_test.go`):
  - `TestInstallNamesForMuelle`: `InstallNamesFor(MuelleStack)` es exactamente `{MuelleStack, MuelleService, PanelNetwork, DataVolume, CaddyAdminVolume, MasterKeySecret, InitCloudflareTokenSecret, InitRegistrySecret, InstallProbeContainer, CaddyContainer}`.
  - `TestInstallNamesForPrefix`: con `it5abc` → `it5abc`, `it5abc_muelle`, `it5abc_panel`, `it5abc_data`, `it5abc_caddy_admin`, `it5abc_master_key`, `it5abc_init_cloudflare_token`, `it5abc_init_registry`, `it5abc-install-probe`, `it5abc-caddy`.
  ```go
  // InstallProbeContainer es el contenedor temporal con que install prueba
  // los puertos del proxy (contrato §5.4); install lo borra siempre.
  const InstallProbeContainer = "muelle-install-probe"

  // InstallNames son los nombres de lo que crea o mira install. Con el
  // prefijo "muelle" son los fijos de este paquete; otro prefijo es solo
  // para los tests de integración (--name-prefix, contrato §5.1).
  type InstallNames struct {
  	Stack, Service, PanelNetwork, DataVolume, CaddyAdminVolume     string
  	MasterKeySecret, InitCloudflareTokenSecret, InitRegistrySecret string
  	ProbeContainer, CaddyContainer                                 string
  }

  func InstallNamesFor(prefix string) InstallNames
  ```

- [x] **Step 2: `PutNetwork` en el fake** (`fake_dist.go`): `func (f *Fake) PutNetwork(n docker.Network)` la deja tal cual (con sus `Subnets`, `Driver`, `Scope` y `Attachable`; sin ID se le asigna uno). Test `TestFakePutNetwork` en `fake_dist_test.go` (`InspectNetwork` la devuelve igual; `CreateNetwork` con ese nombre da `ErrConflict`).

- [x] **Step 3: Flags, tests primero** (`options_test.go`):
  - `TestParseArgsDefaults`: `--panel-domain panel.example.com --email a@example.com` → puertos 80/443, `NamePrefix` `muelle`, sin `Reinstall`.
  - `TestParseArgsRequired`: sin `--panel-domain` (y sin `--panel-origin`) → error que nombra `--panel-domain`; sin `--email` → nombra `--email`.
  - `TestParseArgsInvalid` (tabla; cada error nombra su flag): `--panel-domain Panel.Example.com`, `--email x`, `--public-ip 1.2.3`, `--public-ipv6 2001:0db8::1`, `--http-port 0`, `--https-port 70000`, los dos puertos iguales (nombra `--https-port`), `--cloudflare-token corto`, un token con un espacio, `--caddy-image -x`, `--image 'a b'`, `--name-prefix It5`, `--name-prefix it5_x`, `--advertise-addr -x`, un argumento suelto. En los casos del token (`corto`, con un espacio, y uno de 600 caracteres), el error **no contiene el valor** (`strings.Contains(err.Error(), token)` es falso): dice qué no cumple, nunca lo cita.
  - `TestParseArgsTokenFromStdin`: `--cloudflare-token -` con stdin `"<token de 40>\nsobra\n"` → la primera línea sin espacios; stdin vacío → error; una primera línea que no sirve (con un espacio en el medio) → error que tampoco la contiene.
  - `TestParseArgsRegistry` (imágenes privadas): `--registry-user bot --registry-password-stdin` con stdin `"<contraseña con un espacio>\r\nsobra\n"` → la primera línea, sin el `\r\n` y sin recortar los espacios; `--registry-user` sin `--registry-password-stdin` (o al revés) → error que nombra el que falta; stdin vacío, una línea vacía, una contraseña con un caracter de control o de más de 4096 bytes, un usuario de más de 255 → error que nombra el flag y nunca cita la contraseña; `--registry-password-stdin` junto con `--cloudflare-token -` → error (los dos leen stdin).
  - `TestOptionsHidesSecrets` (como `TestInitDoesNotPrintToken` de `settings`): unas `Options` con `CloudflareToken` y `RegistryPassword`; ni `%v`, `%+v`, `%#v` ni `fmt.Sprint` de `o` y de `&o`, ni un log de `slog` con `"opts", o` (JSON y texto), contienen el token ni la contraseña; `o.String()` trae `***` y el resto de los campos (`PanelDomain`, `RegistryUser`).
  - `TestParseArgsPanelOrigin`: `--panel-origin https://127.0.0.1:22443` sin `--panel-domain` sirve; `https://x/y`, `ftp://x`, `x` → error.
  - `TestParseArgsAdvertiseAddr`: sirven `10.0.0.5`, `10.0.0.5:2377`, `[2001:db8::1]:2377`, `eth0`; no `-x` ni `a b`.
  - `TestParseArgsHelp`: `-h` → `flag.ErrHelp`.
  - `TestUsage`: `Usage` nombra cada flag público y ninguno oculto (`name-prefix`, `--image `, `panel-origin`).
  - `TestPanelOriginFromFlags` (la función que arma `MUELLE_PANEL_ORIGIN`): dominio + 443 → `https://d`; dominio + 8443 → `https://d:8443`; `--panel-origin` manda.

  `options.go`:
  ```go
  type Options struct {
  	PanelDomain, Email, PublicIP, PublicIPv6, AdvertiseAddr string
  	HTTPPort, HTTPSPort                                      int
  	CloudflareToken                                          string // ya leído si vino "-"
  	CaddyImage                                               string
  	Reinstall                                                bool
  	// Las credenciales de las imágenes privadas ("Cambio de requisito del
  	// dueño"); la contraseña ya leída de stdin.
  	RegistryUser, RegistryPassword string
  	// Ocultos, solo para los tests de integración (contrato §5.1).
  	NamePrefix, Image, PanelOrigin string
  }

  // String es Options con el token y la contraseña como "***" (si hay), como
  // settings.Init: unas Options en un %+v de un error o de un log de test no
  // filtran nada.
  func (o Options) String() string {
  	if o.CloudflareToken != "" {
  		o.CloudflareToken = "***"
  	}
  	if o.RegistryPassword != "" {
  		o.RegistryPassword = "***"
  	}
  	type plain Options // sin métodos: no vuelve a llamar a String
  	return fmt.Sprintf("%+v", plain(o))
  }

  // GoString es String para %#v, y LogValue para slog (el JSONHandler
  // serializaría el struct).
  func (o Options) GoString() string { return o.String() }
  func (o Options) LogValue() slog.Value { return slog.StringValue(o.String()) }

  // ParseArgs lee los flags de `muelle install` (stdin solo para
  // --cloudflare-token -) y los valida sin tocar Docker. Un error es de uso
  // (código 2); con -h devuelve flag.ErrHelp.
  func ParseArgs(args []string, stdin io.Reader) (Options, error)

  const Usage = `uso: muelle install --panel-domain <dominio> --email <correo> [flags]
  …(un flag por línea, en español, solo los públicos)…`
  ```
  Stdin se lee al final, solo si el resto de los flags ya sirve (un flag que no sirve no deja a install esperando stdin). La credencial del registry sigue las reglas de `POST /registries` (contrato §2.10), con las que muelle valida `MUELLE_INIT_REGISTRY_FILE` al arrancar: usuario de 1 a 255 caracteres, contraseña de 1 a 4096 bytes, los dos sin caracteres de control (la contraseña puede tener espacios: no se recorta). Validación con `validate.Hostname`, `Email`, `IPv4`, `IPv6`, `Port` e `ImageRef` usando el flag como campo (`validate.Email("--email", …)` da `--email: …`); el token con `^[\x21-\x7e]{20,512}$` (el mismo formato que `PATCH /settings`), con un error fijo que no lo cita (`--cloudflare-token: tiene que tener de 20 a 512 caracteres ASCII visibles, sin espacios`; nunca un `%q` del valor, tampoco el de stdin), y lo mismo para la contraseña del registry; el prefijo con `^[a-z][a-z0-9]{0,15}$`; `--advertise-addr` con `netip.ParseAddr`, `netip.ParseAddrPort` o un nombre de interfaz `^[A-Za-z0-9][A-Za-z0-9._-]{0,14}(:[0-9]{1,5})?$`; `--panel-origin` con `url.Parse` (esquema `http` o `https`, host, sin ruta ni query; muelle lo valida entero al arrancar).

- [x] **Step 4: El stack, tests primero** (`stack_test.go`):
  - `TestComposeGolden` (tres casos, con `-update` para regenerar): (a) prefijo `muelle`, sin token, con `public_ip`, `public_ipv6` y `caddy_image` → `testdata/stack.golden.json`; (b) prefijo `it5abc`, con token, sin las opcionales → `testdata/stack_token.golden.json`; (c) prefijo `muelle`, puertos 8080/8443, con el secreto del registry (`MUELLE_INIT_REGISTRY_FILE` y `{source: init_registry, target: muelle_init_registry}`) → `testdata/stack_registry.golden.json`.
  - `TestServiceEnv`: las variables exactas del caso (b), sin `MUELLE_SECURE_COOKIES`; en el (c), `MUELLE_INIT_REGISTRY_FILE` y ninguna opcional vacía. Revisa los dos golden a mano contra el YAML del contrato §5.3 antes del commit: `image`, `environment` (las fijas, `MUELLE_INIT_PROXY_*` siempre, las opcionales solo con valor, `MUELLE_INIT_CLOUDFLARE_TOKEN_FILE` solo con token), los tres montajes, el secreto con `target: muelle_master_key` (y el del token), `networks: [panel]`, `logging`, `deploy` (`replicas: 1`, las dos restricciones con el id del nodo, `update_config` y `rollback_config` `stop-first`), y los bloques de primer nivel con `name:` (`external: true` en secretos y red).
  - `TestComposeEscapesDollar`: con un email `a$b@example.com` el JSON trae `a$$b@example.com` y ningún `$` suelto.
  - `TestComposeAcceptedByDockerCLI`: si `exec.LookPath("docker")` y `docker stack config --help` funcionan (si no, `t.Skip`), corre `docker stack config -c -` con el compose del caso (b) más el email con `$`, con `DOCKER_HOST=unix:///nonexistent.sock` (no necesita daemon) → código 0, y la salida trae `a$b@example.com`, `name: it5abc_data` y `node.id == node-1`.

  `stack.go`: structs con tags `json` para el compose (servicio, montaje `{type, source, target}`, secreto `{source, target}`, logging, deploy con placement y los dos update config, y los bloques de primer nivel), escritos con un `json.Encoder` de `encoding/json` con `SetIndent` y `SetEscapeHTML(false)` (los mapas salen ordenados: es determinista). El `$` se escapa como `$$` sobre el JSON ya escrito (`bytes.ReplaceAll`): `encoding/json` no escapa el `$`, que solo aparece dentro de los strings, así que equivale a escapar **todo** string del documento sin poder olvidar ninguno (decisión 7). Sin `version:`. La entrada es un `stackInput` (nombres, imagen, origen, IP pública, nodo, y si monta el secreto del token y el del registry, porque la credencial puede venir del `config.json` y no de las `Options`). Las variables:
  ```go
  // serviceEnv son las variables del servicio de muelle (contrato §5.2): las
  // fijas, MUELLE_INIT_PROXY_* siempre, y las demás solo con valor.
  func serviceEnv(in stackInput) map[string]string
  ```

- [x] **Step 5: La IP pública, tests primero** (`publicip_test.go`, con `httptest`):
  - `TestDetectPublicIP`: un cuerpo `fl=1\nip=203.0.113.24\nts=1\n` → `203.0.113.24`.
  - `TestDetectPublicIPRejects`: `ip=2001:db8::1` (IPv6) → error; sin línea `ip=` → error; una IPv4 no canónica o basura → error; la línea `ip=` después de los 4 KiB → error (solo se leen las líneas enteras dentro del tope: una `ip=` cortada podría parecer otra IP); un `500` → error; un servidor que no responde dentro del plazo (el test pasa un `ctx` de 100 ms) → error.
  ```go
  const traceURL = "https://1.1.1.1/cdn-cgi/trace"

  // detectPublicIP lee la IPv4 pública de la línea ip= de url, con a lo más
  // 5 s (o el plazo de ctx, si es menor) y 4 KiB de respuesta (contrato §5.4 paso 4).
  func detectPublicIP(ctx context.Context, client *http.Client, url string) (string, error)
  ```

- [x] **Step 6: El link de setup, tests primero** (`setup_test.go`, con el fake; `poll` de 10 ms):
  - `TestWaitSetupURL`: el servicio con una tarea `running` cuyas líneas son texto suelto, un JSON con `"event":"setup_url","url":"https://p/setup?token=A"` y `"event":"listening"` → `https://p/setup?token=A`.
  - `TestWaitSetupURLCurrentTask`: una tarea `shutdown` (desired `shutdown`) con el link A y otra `running` con el B y `listening` → B. Una tarea con desired `running` pero `state: starting` no cuenta hasta que corre.
  - `TestWaitSetupURLTaskReplaced`: la tarea t1 corre con el link A y sin `listening`; 100 ms después el test hace `SetTasks(t1 shutdown, t2 running)` y `AppendTaskLogs(t2, link B, listening)` → B.
  - `TestWaitSetupURLNoTaskYet`: el servicio sin tareas; 100 ms después aparece la tarea con su log → el link.
  - `TestWaitSetupURLHasUsers`: solo `listening` → `hasUsers` y sin error.
  - `TestWaitSetupURLShowsErrors`: una tarea `running` con la línea `{"level":"ERROR","msg":"muelle terminó con error","err":"MUELLE_INIT_ACME_EMAIL: …"}` → stderr la muestra; sin más líneas, termina por el plazo.
  - `TestWaitSetupURLFailedTask` (lo que pasa de verdad con una clave equivocada o un `--panel-origin` que no sirve: muelle sale en milisegundos, la tarea queda `failed` y Swarm reintenta cada ~5 s, así que casi nunca se la ve `running`): el servicio solo tiene una tarea `failed` con desired `shutdown`, `ContainerID: "c1"`, `Err: "task: non-zero exit (1)"`, y en su log (`AppendTaskLogs`) la línea `ERROR` de arriba; ninguna tarea `running`; plazo de 300 ms → antes de `ErrSetupTimeout`, stderr trae una línea `aviso:` con el id de la tarea, `failed` y su `Err`, y la línea `ERROR` (su `err`). Cada una sale **una sola vez**, aunque el bucle vio la tarea muchas veces (cuenta las apariciones en stderr).
  - `TestWaitSetupURLRejectedTask` (la tarea nunca corre: el bind de `/var/run/docker.sock` no existe, con Docker rootless; una restricción que no se cumple deja la tarea `pending` con `Err` "no suitable node", que sale por la misma regla): una tarea `rejected` con desired `shutdown`, sin `ContainerID`, `Err: "invalid mount config for type \"bind\": bind source path does not exist: /var/run/docker.sock"` → stderr trae `rejected` y ese `Err` una sola vez, y en `Calls()` no hay ningún `TaskLogs` (sin contenedor no hay log).
  - `TestWaitSetupURLPendingErrLater`: una tarea `pending` sin `Err` no escribe nada; cuando aparece su `Err` ("no suitable node"), sale una sola vez.
  - `TestWaitSetupURLFailedThenRunning`: la tarea t1 `failed` (con su `ERROR` en el log) y, 100 ms después, `SetTasks(t1, t2 running)` con el link B y `listening` en t2 → B, y stderr trae el `ERROR` de t1 una vez.
  - `TestWaitSetupURLStreamEnds` (el stream de la tarea actual se corta antes de `listening`: un `io.EOF`, o el daemon que lo cierra): el fake envuelto en `endsOnce{Fake, task: "t1"}`, un tipo de `setup_test.go` que embebe `*dockertest.Fake` y cuya **primera** llamada a `TaskLogs` de t1 con `Follow` le pide al fake el mismo log con `Follow: false` (así el stream termina con `io.EOF` después de las líneas que ya hay) y lo avisa por un canal. t1 corre con el link A y una línea `ERROR`, sin `listening`; cuando se cierra el primer stream, el test hace `AppendTaskLogs(t1, listening)` → A y sin error (nunca `ErrSetupTimeout`); `Calls()` trae dos `TaskLogs t1`, y la línea `ERROR` sale una sola vez en stderr aunque el log se leyó dos veces.
  - `TestWaitSetupURLTimeout`: plazo de 200 ms sin líneas → `errors.Is(err, ErrSetupTimeout)` y el mensaje trae `docker service logs <servicio>` y `docker service ps --no-trunc <servicio>`.
  - `TestWaitSetupURLCanceled`: un Ctrl-C (el `ctx` de quien llama, cancelado) devuelve `context.Canceled`, no `ErrSetupTimeout`.

  `setup.go` (contrato §5.4, "El link de setup"):
  ```go
  var ErrSetupTimeout = errors.New("muelle no mostró el link de setup a tiempo")

  // waitSetup sigue el log de la tarea actual del servicio (la running más
  // nueva) hasta "event":"listening". Devuelve el último setup_url de esa
  // tarea, o hasUsers si no hubo ninguno. Si la tarea actual cambia, cierra el
  // stream y lee la nueva desde el principio; si el stream se corta antes de
  // listening, vuelve a leer la misma desde el principio. Mientras no hay una tarea
  // corriendo, muestra en stderr por qué: el estado y el Err de la tarea más
  // nueva, y las líneas ERROR de su log si ya terminó. Las líneas de nivel
  // ERROR de cualquier tarea van a stderr, una vez cada una.
  func waitSetup(ctx context.Context, e docker.Services, service string, timeout, poll time.Duration, stderr io.Writer) (setupURL string, hasUsers bool, err error)
  ```
  Forma:
  - un `context.WithTimeout`, y un bucle que cada vez lee **todas** las tareas del servicio (`ListTasks` con `Service` y sin `DesiredState`; vienen de la más nueva a la más vieja);
  - **hay una tarea corriendo** (desired `running` y state `running`, la más nueva): `TaskLogs(task, LogOptions{Tail: -1, Follow: true})`, con una goroutine que cada `poll` vuelve a buscar la tarea actual y cierra el stream si cambió (o si se acabó el plazo). Cada línea se decodifica como JSON (las que no son JSON se ignoran). Al salir de una tarea sin `listening`, vuelve al bucle;
    - **si el stream termina antes de `listening`** (`io.EOF`, u otro error porque el daemon lo cerró) y la tarea sigue siendo la actual, el bucle la vuelve a seguir **desde el principio** (otra vez `Tail: -1`), después de esperar `poll`. Una tarea actual nunca se salta por haberla leído antes: el `setup_url` que vale es el último de la lectura que llegó a `listening`;
  - **no hay ninguna corriendo**: toma la tarea más nueva del servicio, con cualquier desired state (si no hay ninguna, espera `poll`):
    - si tiene `Err`, o su estado es final (`failed`, `rejected`, `shutdown`, `complete`, `orphaned`), escribe una vez por tarea `aviso: la tarea <id> de <servicio> está <state>: <Err>` (sin `: <Err>` si está vacío; una tarea `pending` cuyo `Err` aparece después, como "no suitable node", sale cuando aparece);
    - si tiene `ContainerID` y su estado es final, lee su log **una vez** (`TaskLogs` con `Tail: -1` y `Follow: false`) y escribe sus líneas `ERROR`;
    - después espera `poll` y vuelve al bucle;
  - lo que se anota para que nada salga dos veces aunque el bucle pase muchas veces por la misma tarea:
    - los `aviso:`, por id de tarea;
    - el log de una tarea que ya terminó, leído una vez sin `Follow`, por id (ya no cambia);
    - las líneas `ERROR` escritas en stderr, por tarea y texto de la línea (el id y la línea cruda), así una tarea seguida dos veces no las repite;
    - el log seguido de la tarea actual, en cambio, **no** se anota: se puede volver a leer entero cuantas veces haga falta;
  - con el plazo vencido, `ErrSetupTimeout`, con el mensaje "revisa `docker service logs <servicio>` y `docker service ps --no-trunc <servicio>`" (la segunda muestra el `Err` entero de cada tarea).

- [x] **Step 7: `Run`, tests primero** (`install_test.go`, con el fake). Un helper `newInstall(t)` arma `dockertest.New()`, `SetImagePresent` de `ghcr.io/cpu-commits/muelle:v0.1.0` y de `ghcr.io/cpu-commits/muelle-caddy:v0.1.0` (install baja las dos, "Cambio de requisito del dueño"), un `Config` con `Version: "v0.1.0"`, `DetectIP` que devuelve `203.0.113.24` (y cuenta llamadas), `SetupTimeout: 2 * time.Second`, `Poll` y `NetworkWait` cortos, `DockerConfig` en un archivo que no existe (nunca el `config.json` de quien corre los tests) y stdout/stderr en buffers; y un `muelleStarts(f, names, lines...)` para `OnDeployStack`, que crea el servicio con `PutStackService` (una tarea `running`) y le agrega esas líneas con `AppendTaskLogs` (por defecto, `setup_url` + `listening`). El fake registra también las llamadas con que el test prepara el nodo (`CreateSecret`, `CreateNetwork`…): `h.calls()` son solo las de la corrida.
  - `TestRunFreshInstall`: termina sin error. En `Calls()`, en este orden relativo (puede haber otras llamadas entre medio, como `InspectNetwork`): `Info`; las búsquedas de una instalación anterior; `InspectImage`; el contenedor de prueba (`RemoveContainer`, `CreateContainer muelle-install-probe <imagen>`, `StartContainer`, `RemoveContainer`); `CreateNetwork muelle_panel`; `CreateSecret muelle_master_key`; `DeployStack muelle`. La red es overlay y attachable. Stdout trae la clave (base64 de 32 bytes, igual a `f.SecretData("muelle_master_key")`) **antes** del link. El compose desplegado (`f.StackDeploys()`) tiene `node.id == node-1` y la imagen de la versión.
  - `TestRunPullsMissingImage`: sin la imagen en el nodo → `PullImage` antes del contenedor de prueba.
  - `TestRunDevNeedsImage`: `Version: "dev"` sin `Image` → error que sugiere `--image`, y nada creado; con `Image: "it5-muelle:test"` (presente) funciona y usa esa.
  - `TestRunDockerDown`: `SetUnavailable(true)` → error con `/var/run/docker.sock`.
  - `TestRunSwarmStates`: worker (`active`, `IsManager: false`), `pending`, `error` y `locked` → error, sin nada creado.
  - `TestRunInitsSwarm`: `SetInfo` con `SwarmState: inactive` y sin `NodeID`, más `--advertise-addr 10.0.0.5` → `InitSwarm 10.0.0.5` después del contenedor de prueba y antes de `CreateNetwork`; la restricción usa el id que devolvió; no se buscó ningún servicio ni secreto (con el Swarm inactivo no hay), pero sí la red (`InspectNetwork muelle_panel`, que funciona sin Swarm).
  - `TestRunInitSwarmFails`: Swarm inactivo, sin `--advertise-addr`, y `FailNext("InitSwarm", errors.New("could not choose an IP address to advertise since this system has multiple addresses"))` → error que trae el de Docker y sugiere `--advertise-addr <ip>` (si la máquina tiene varias IPs); el contenedor de prueba no quedó y no hubo `CreateNetwork`, `CreateSecret` ni `DeployStack`.
  - `TestRunAdvertiseAddrIgnored`: Swarm activo + `--advertise-addr` → aviso en stderr, sin `InitSwarm`.
  - `TestRunAlreadyInstalled`: `PutService(muelle_muelle)` → `errors.Is(err, ErrAlreadyInstalled)`, también con `Reinstall`; el mensaje sugiere `docker service update --image`.
  - `TestRunPreviousDataNeedsReinstall`: con el secreto `muelle_master_key`, o con el volumen `muelle_data` (`PutVolume`) → `ErrPreviousData`, el mensaje ofrece `--reinstall`; nada creado.
  - `TestRunReinstall`: secreto y volumen existen, `Reinstall` → sin `CreateSecret muelle_master_key`, stdout sin clave y con el link, stderr con "la base ya existe".
  - `TestRunReinstallWithoutSecret`: `Reinstall` sin el secreto → error.
  - `TestRunReinstallSkipsProbe`: `Reinstall` + un `muelle-caddy` creado con `CreateContainer` (después de `SetImagePresent` de su imagen) + `BusyHostPort(443)` → sin contenedor de prueba, y termina bien.
  - `TestRunTokenSecret`: con token → `CreateSecret muelle_init_cloudflare_token` con el token como dato; el compose monta el secreto y trae `MUELLE_INIT_CLOUDFLARE_TOKEN_FILE`. Si el secreto ya existía, primero `RemoveSecret`.
  - `TestRunReinstallKeepsTokenSecret`: `Reinstall` sin token y con el secreto del token → no se borra ni se monta.
  - `TestRunPortsBusy`: `BusyHostPort(443)` → `errors.Is(err, ErrPortsBusy)`, el mensaje sugiere `--http-port`/`--https-port`, el contenedor de prueba no quedó, y no hubo `InitSwarm`, `CreateNetwork`, `CreateSecret` ni `DeployStack`. Sin `muelle-caddy`, el mensaje no lo nombra. Variante (quedó el proxy de una instalación borrada a medias): además un `muelle-caddy` creado con `CreateContainer` (después de `SetImagePresent` de su imagen) → el mensaje dice que ese contenedor tiene tomados los puertos y trae `docker rm -f muelle-caddy`.
  - `TestProbeSpec` (sobre la función que arma el contenedor de prueba): `Cmd: ["help"]`, la imagen, y los puertos `http/tcp → 80`, `https/tcp → 443`, `https/udp → 443`, sin red ni reinicio.
  - `TestRunRemovesStaleProbe`: un `muelle-install-probe` que quedó (creado antes con `CreateContainer` y la imagen de muelle, que `newInstall` ya dejó presente) se borra antes, y la instalación sigue.
  - `TestRunHTTP01Warning`: sin token y `--http-port 8080` → aviso de HTTP-01 en stderr; con token, sin aviso.
  - `TestRunPublicIP`: sin `--public-ip`, la detectada va en `MUELLE_INIT_PUBLIC_IP`; si `DetectIP` falla, aviso y la variable no va; con `--public-ip`, `DetectIP` no se llama.
  - `TestRunExistingNetwork`: una `muelle_panel` overlay y attachable creada antes se reusa (un solo `CreateNetwork`, el del test) y, si después `DeployStack` falla, no se borra.
  - `TestRunWrongNetwork`:
    - una `muelle_panel` `bridge` (también con el Swarm inactivo, que es cuando más sorprende) y una overlay no attachable → error en el paso 3: en `Calls()` no hay `PullImage`, `CreateContainer`, `InitSwarm`, `CreateNetwork` ni `CreateSecret`;
    - una overlay attachable con subred pública (`PutNetwork` con `203.0.113.0/24`) → error en el paso 8, antes de crear secretos (el contenedor de prueba ya se borró).
  - `TestRunDeployFailsRollsBack`: `FailNext("DeployStack", …)` → error con la salida del CLI; se borraron los secretos y la red creados en la corrida; stdout sin clave; sin `RemoveStack` (el servicio no existe). Variante: `OnDeployStack` crea el servicio y devuelve error → además `RemoveStack muelle`, y los secretos se borran después de que el servicio ya no existe. Variantes con volúmenes (la tarea alcanzó a arrancar: el daemon creó los volúmenes y muelle una base con una clave que nadie vio):
    - `OnDeployStack` además hace `PutVolume(muelle_data)` y `PutVolume(muelle_caddy_admin)` → los dos se borran: `RemoveVolume` de cada uno después de `RemoveStack muelle` en `Calls()`, y `ListVolumes` ya no los trae;
    - con `Reinstall`, el secreto y un `muelle_data` que existían antes de `Run`, más el mismo `OnDeployStack` → `muelle_data` sigue (no hay `RemoveVolume muelle_data`), y `muelle_caddy_admin`, que creó esta corrida, se borra;
    - `FailNext("RemoveVolume", errors.New("x"))` (un error que no es `ErrConflict`: no se reintenta) → `Run` devuelve el error del deploy, y stderr trae `docker volume rm <el volumen que no se pudo borrar>`.
  - `TestRunUndoOnTokenSecretFailure`: con token, el fake envuelto en `failSecret{Fake, name: "muelle_init_cloudflare_token"}` (un tipo de `install_test.go` cuyo `CreateSecret` falla solo con ese nombre y si no llama al del fake) → error; ya se habían creado la red y `muelle_master_key`, y después de `Run` ninguno de los dos existe (`InspectNetwork` y `InspectSecret` dan `ErrNotFound`); stdout no trae la clave; sin `DeployStack`. Variante (un Ctrl-C en ese momento): el `CreateSecret` del token cancela el `ctx` de `Run` y devuelve `context.Canceled` → lo mismo.
  - `TestRunUndoOnLeftoverTokenSecret`: con token, el secreto del token que quedó de una corrida cortada y `FailNext("RemoveSecret", …)` (el único `RemoveSecret` antes del deploy es el de ese secreto) → error; la red y `muelle_master_key` creados en esta corrida se borraron; stdout sin clave.
  - `TestRunUndoNetworkWithoutSubnets`: el fake envuelto en `noSubnets{Fake}` (su `InspectNetwork` devuelve la red sin `Subnets`, como una red recién creada que todavía no las tiene) y `NetworkWait` de 100 ms → error que dice que la red no tiene subred todavía; la red creada en esta corrida se borró (`RemoveNetwork muelle_panel` en `Calls()`, y ya no está); no se creó ningún secreto.
  - `TestRunSetupTimeoutKeepsStack`: `OnDeployStack` crea el servicio sin líneas y `SetupTimeout` de 200 ms → `ErrSetupTimeout`; la clave ya salió en stdout; no hubo `RemoveStack`.
  - `TestRunHasUsers`: las líneas son solo `listening` → stdout dice que muelle ya tiene usuarios y el origen; sin error.
  - `TestRunNamePrefix`: `NamePrefix: "it5abc"` → todo lo creado y buscado lleva el prefijo (`it5abc_panel`, `it5abc_master_key`, `it5abc` el stack, `it5abc-install-probe`), ningún argumento de una llamada es un nombre fijo, y espera el link en `it5abc_muelle`.
  - Las imágenes privadas ("Cambio de requisito del dueño"; `registry_test.go` y `install_test.go`):
    - `TestRegistryHost`, y `TestResolveCredentials*`: con `--registry-user`, la credencial del host de la imagen de muelle (sin leer el `config.json`); del `config.json`, `auths[<host>].auth` (base64 de `usuario:contraseña`; Docker Hub con su clave larga `https://index.docker.io/v1/`); sin el archivo, ninguna; un helper (`credHelpers[<host>]`, o `credsStore` con una entrada vacía de ese host en `auths`) → error que pide `--registry-user` y `--registry-password-stdin`; con `credsStore` y sin login en ese host, sin credencial y sin error (la imagen puede ser pública); un `config.json` que no es JSON, un `auth` que no es base64 o no es `usuario:contraseña`, o con un caracter de control → error que nombra el `config.json` y nunca cita la contraseña.
    - `TestRunRegistryFromFlags` y `TestRunRegistryFromDockerConfig` (sin las imágenes en el nodo; un `recordPulls` de `install_test.go` anota la credencial de cada `PullImage`): las dos imágenes se bajan con la credencial de `ghcr.io`; el secreto `muelle_init_registry` es el JSON `{"host","username","password"}`; el compose lo monta y trae `MUELLE_INIT_REGISTRY_FILE`; `StackDeploy.Registries` es esa credencial; la contraseña no sale en stdout, stderr ni el compose.
    - `TestRunRegistryHelperFails`: el error del helper sale antes de tocar Docker. `TestRunRegistryNeedsBoth`: unas `Options` con el usuario y sin la contraseña (o al revés) → error, sin tocar Docker.
    - `TestRunPullDenied` (`DenyImage` de la imagen de muelle), sin credencial y con una: el error dice `docker login ghcr.io`, `read:packages`, `~/.docker/config.json`, `--registry-user` y `--registry-password-stdin`, sin la contraseña; nada creado.
    - `TestRunRegistrySecretUndo`: si `DeployStack` falla, el secreto del registry se borra. `TestRunReinstallRegistrySecret`: con `--reinstall` y el secreto que quedó, sin credencial no se toca ni se monta; con credencial se borra y se crea de nuevo.

  `install.go`:
  ```go
  var (
  	ErrAlreadyInstalled = errors.New("muelle ya está instalado")
  	ErrPreviousData     = errors.New("hay datos de una instalación anterior")
  	ErrPortsBusy        = errors.New("los puertos del proxy están ocupados")
  )

  // Config son las dependencias de Run; los tests acortan los plazos y cambian la red.
  type Config struct {
  	Version        string        // main.version
  	Docker         docker.Engine
  	Stdout, Stderr io.Writer
  	DetectIP       func(ctx context.Context) (string, error) // nil: detectPublicIP con traceURL
  	SetupTimeout   time.Duration // 0: 2 min
  	Poll           time.Duration // 0: 1 s
  	NetworkWait    time.Duration // 0: 5 s (las subredes de una red recién creada)
  	// DockerConfig es el config.json de las credenciales si no vinieron por
  	// flags; vacío es $DOCKER_CONFIG/config.json o ~/.docker/config.json (en
  	// la imagen, /root/.docker/config.json, montado con -v).
  	DockerConfig string
  }

  // Run instala muelle con los pasos del contrato §5.4, en ese orden: nada
  // cambia antes de la prueba de puertos. Devuelve nil si imprimió el link de
  // setup (o muelle ya tenía usuarios). Los errores ya traen el mensaje para
  // quien corre install.
  func Run(ctx context.Context, o Options, c Config) error
  ```
  Puntos de diseño (todos del contrato):
  - **Paso 3** (instalación anterior): con el Swarm activo, `InspectService(names.Service)`, `InspectSecret(names.MasterKeySecret)` y, si vino token o con `Reinstall`, `InspectSecret(names.InitCloudflareTokenSecret)`; con el Swarm inactivo no se busca ningún servicio ni secreto (el daemon respondería `503`). Siempre `ListVolumes(nil)` buscando `names.DataVolume` y `names.CaddyAdminVolume`, y se anota si existían (el deshacer del paso 8 borra solo los que creó esta corrida); con `Reinstall`, además `InspectContainer(names.CaddyContainer)`. Un error que no es `ErrNotFound` aborta.
  - **Paso 3, la red:** siempre `InspectNetwork(names.PanelNetwork)`, que funciona sin Swarm. Si existe y no es overlay, attachable y de scope `swarm`, aborta acá, antes de bajar la imagen, probar los puertos o iniciar el Swarm (una `bridge` local con ese nombre y el Swarm inactivo abortaría recién en el paso 8, con el Swarm ya iniciado). `ErrNotFound` es que hay que crearla en el paso 8. Las subredes se revisan en el paso 8, para la red que existía y para la recién creada.
  - **Antes del paso 2** (no tocan Docker, como la validación del paso 1): la imagen, `o.Image` o `naming.MuelleImageFor(c.Version)` (vacía → error que pide una versión publicada o `--image`), la de Caddy (`o.CaddyImage` o `naming.CaddyImageFor(c.Version)`; vacía con `dev`) y las credenciales (`resolveCredentials`, `registry.go`). Así un `dev` sin `--image` o un `config.json` con un helper abortan sin llamar a Docker.
  - **Paso 5** (imágenes): para la de muelle y la de Caddy, `InspectImage` y, con `ErrNotFound`, `PullImage(ref, <la credencial de su host, o nil>)`. Un pull que falla sin credencial, o con un `denied`/`unauthorized`, trae `privateHint`.
  - **Paso 6** (puertos): borra un contenedor de prueba que quedó (ignora `ErrNotFound`), lo crea con `probeSpec(…)`, lo arranca; `ErrPortInUse` → `ErrPortsBusy`. El contenedor de prueba se borra siempre (`force`) con `context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)`.
    - Con `ErrPortsBusy`, si `InspectContainer(names.CaddyContainer)` lo encuentra, el mensaje agrega que ese contenedor (de una instalación anterior: `docker stack rm` no lo borra) tiene tomados los puertos, con `docker rm -f <names.CaddyContainer>`. Así el error no esconde la causa detrás de "usa otros puertos".
  - **Paso 7** (Swarm): si `InitSwarm` falla, el error trae el de Docker y, si no vino `--advertise-addr`, la pista "si la máquina tiene varias IPs, repite con `--advertise-addr <ip>`" (Docker no elige una sola: "could not choose an IP address to advertise").
  - **Paso 8, el orden:** la red (crearla si no existe; esperar sus subredes hasta `NetworkWait` y revisar que sean privadas), `muelle_master_key` (salvo con `Reinstall`), el secreto del token si vino (`RemoveSecret` del que quedó, y `CreateSecret`), el de `muelle_init_registry` si hay una credencial para el host de la imagen de muelle (igual que el del token), y `DeployStack` con esa credencial en `StackDeploy.Registries`.
  - **El aviso de HTTP-01** no sale si el volumen de la base ya existe: los ajustes de install no se aplican sobre esa base.
  - **Paso 8 con deshacer** (contrato §5.4: "install no cambia nada hasta saber que va a poder terminar"): una lista de funciones de limpieza que se agregan a medida que se crea cada cosa (la red si se creó acá, cada secreto creado, y el stack con sus volúmenes, abajo). Corre en orden inverso ante **cualquier** error o cancelación del `ctx` desde la primera creación hasta que `DeployStack` funciona: las subredes de la red que no llegan o son públicas, un `CreateSecret` o el `RemoveSecret` del secreto del token que quedó, `DeployStack`, o un Ctrl-C entre medio. Así nunca queda un `muelle_master_key` cuya clave no se imprimió (la próxima corrida abortaría con `ErrPreviousData`, y `--reinstall` reusaría una clave que nadie vio).
    - Forma: `Run` (o la función del paso 8) tiene el error como resultado con nombre y un `defer` que, si el error no es nil y `DeployStack` todavía no funcionó, corre la lista con `context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)`. Con el `ctx` cancelado, cada llamada siguiente falla, y el `defer` limpia igual.
    - Justo antes de `DeployStack` se agrega a la lista (queda primera al correr) un paso que hace `InspectService(names.Service)`: si existe (el CLI alcanzó a crearlo), `RemoveStack(names.Stack)` y espera (hasta 30 s) a que `InspectService` dé `ErrNotFound` antes de seguir con los secretos, que no se pueden borrar mientras un servicio los usa; con `ErrNotFound`, nada.
    - En ese mismo paso, **después** de que `InspectService` da `ErrNotFound` (el servicio se borró, o nunca existió), se borran `names.DataVolume` y `names.CaddyAdminVolume` si no existían en el paso 3. Si la tarea alcanzó a arrancar, el daemon los creó y muelle inicializó en `names.DataVolume` una base cifrada con una clave que nadie vio: la próxima corrida abortaría con `ErrPreviousData`, y `--reinstall` tampoco serviría (el secreto ya se borró). `RemoveVolume` (sin `force`) reintenta mientras dé `ErrConflict` (el contenedor tarda en terminar; hasta 30 s), y `ErrNotFound` es que no se creó. Un volumen que existía antes de esta corrida (con `Reinstall`, `muelle_data`) nunca se borra.
    - La red se borra reintentando mientras dé `ErrConflict`. Lo que no se pudo borrar se nombra en stderr con el comando (`docker secret rm …`, `docker network rm …`, `docker stack rm …`, `docker volume rm …`).
    - Una red que ya existía antes de esta corrida nunca se borra (`TestRunExistingNetwork`).
  - **Paso 9:** la clave (si se generó) en stdout, después `waitSetup`, después el link (o el aviso de usuarios). Los textos van como constantes del paquete; la clave y el link en su propia línea, con cuatro espacios adelante, para copiarlos fácil.
  - **Avance y avisos:** una línea por paso en stderr (`Docker 29.6.1, Swarm activo (nodo …)`, `IP pública: … (detectada)`, `puertos 80 y 443 libres`, `red muelle_panel creada`…); los avisos empiezan con `aviso: `.

- [ ] **Step 8: `main.go`.** Tests en `main_test.go`:
  - `TestRunInstallUsage`: `{"install", "--nope"}` → 2, stderr con `install.Usage`; `{"install", "-h"}` → 0, stdout con `install.Usage`; `{"install"}` → 2 (falta `--panel-domain`).
  - `TestLogListening`: `logListening(log, "[::]:8080")` escribe una línea `INFO` con `"event":"listening"` y `"addr":"[::]:8080"`.

  Implementación: `case "install"`: `install.ParseArgs(extra, os.Stdin)` (`flag.ErrHelp` → uso a stdout y 0; otro error → `muelle install: <err>` + uso a stderr y 2), `signal.NotifyContext`, `docker.New(docker.Options{})` (y `Close`), `install.Run(ctx, o, install.Config{Version: version, Docker: cli, Stdout: stdout, Stderr: stderr})`; un error → `muelle install: <err>` en stderr y 1. En `serve`, la línea de "muelle escuchando" pasa a `logListening(log, ln.Addr().String())`:
  ```go
  // logListening deja en el log que serve empezó a atender, con
  // event=listening: install la espera después del setup_url (contrato
  // §5.4). El mensaje se puede cambiar; event no.
  func logListening(log *slog.Logger, addr string) {
  	log.Info("muelle escuchando", "event", "listening", "addr", addr)
  }
  ```
  Agrega `install` al `usage` de `main.go`.

- [ ] **Step 9: Verificar lo unitario y commitear** antes de la integración, en el orden de los steps (cada commit compila: `Run` usa `waitSetup` y `detectPublicIP`):
  - `feat(naming): los nombres de install, con el prefijo de los tests`
  - `test(dockertest): PutNetwork deja una red tal cual, con sus subredes`
  - `feat(install): flags y validación de muelle install`
  - `feat(install): el stack de muelle como JSON, con $ escapado`
  - `feat(install): detectar la IP pública con 1.1.1.1/cdn-cgi/trace`
  - `feat(install): seguir el log de la tarea actual hasta el link de setup, y mostrar por qué no arranca`
  - `feat(install): los pasos de muelle install, la prueba de puertos y deshacer lo creado ante cualquier error antes del deploy`
  - `feat: muelle install, y serve avisa con event=listening cuando empieza a atender`

- [ ] **Step 10: Integración** (`src/cmd/install/integration_test.go`, `//go:build integration`, `package install`). Necesita la imagen de **esta** rama (trae `settings reset-panel-ips` y el evento `listening`): reconstruye `it5-muelle:test` con el contexto de las convenciones antes de correr. El test se salta si la imagen no está (`MUELLE_IT_IMAGE`, por defecto `it5-muelle:test`) o si el nodo no es manager de un Swarm activo.
  - **Guarda de lo ajeno** (decisión 5): el muelle del test tiene `docker.sock`, así que el test cuenta con que, con ingress `disabled`, ningún módulo hace nada con Docker por su cuenta (el plan de la Fase 3 lo cumple: "Con `disabled`, no hace nada"; los jobs de deploy no tienen deploys en una base nueva). Para que una fase que lo rompa se note, el test mira **solo** dos nombres fijos y una red propia, nunca las redes de las otras líneas: los tests de las fases 2 y 3 crean y borran redes con `muelle.project` en este mismo daemon y en paralelo (`it2e2e_staging`, `it3<id>_s2`, las huérfanas de la Fase 3), y al integrar `go test -tags integration ./...` corre a la vez los binarios de varios paquetes. Una guarda que anotara esas redes fallaría al azar. Helper `guardForeign(t, c, names)`, para los dos tests que arrancan un muelle (`TestIntegrationInstall` y `TestIntegrationInstallFromImage`), llamado **después** de `cleanupInstall` y antes de instalar:
    - anota si existen la red `muelle_panel` y el contenedor `muelle-caddy` (nunca los crea ni los borra);
    - crea una red **centinela** con la forma exacta de una red de stage que el job `"proxy"` tomaría por huérfana (D13 del plan de la Fase 3): `naming.Network(p, "sentinel")` (`<p>_sentinel`), overlay y attachable, con `naming.StageLabels(p, "sentinel")`. Ese stage no está en la base nueva del muelle del test, así que una limpieza de redes de stage que corriera con `disabled` la tomaría. Tiene el prefijo `it5`: ningún test de las otras líneas la toca (la Fase 3 filtra las huérfanas con `NetworkPrefix` `it3<id>`);
    - registra un `t.Cleanup` que falla si `muelle_panel` o `muelle-caddy` aparecieron durante el test, o si la centinela ya no existe (`InspectNetwork` da `ErrNotFound`). Como los `Cleanup` corren al revés, este corre antes que el de `cleanupInstall`: al final del test, con el muelle todavía arriba. La centinela la borra después `cleanupInstall`.
    - Es de mejor esfuerzo: la Fase 3 borra una huérfana la segunda vez seguida que la ve (cada 5 min), así que un ingress que corriera con `disabled` lo delatan `muelle-caddy` o `muelle_panel` (los crea en la primera pasada), y la centinela atrapa una limpieza que borre enseguida.
  - **Limpieza por prefijo** (helper `cleanupInstall(t, c, names)`, registrado antes de `guardForeign` y de instalar): `RemoveStack`, esperar a que el servicio no exista (60 s), borrar el contenedor de prueba (`force`), la red (reintentando con `ErrConflict`, 60 s), la red centinela `<p>_sentinel`, los dos secretos y los dos volúmenes (`RemoveVolume`, reintentando mientras dé `ErrConflict`, 60 s). Ignora `ErrNotFound`; cualquier otro error es `t.Errorf`.
  - `TestIntegrationInstall` (subtests en orden, sobre el mismo prefijo `it5` + 6 hex; puertos 22080 y 22443):
    1. install en el proceso del test, con el cliente real: `Options{PanelOrigin: "https://127.0.0.1:22443", Email: "it5@example.com", PublicIP: "203.0.113.10", HTTPPort: 22080, HTTPSPort: 22443, Image: <imagen>, NamePrefix: <p>}`, `Config{Version: "dev"}`. **Sin token** (con un token, el muelle del test podría llamar a Cloudflare). Asserts: sin error; stdout con una clave base64 de 32 bytes y un link que empieza con `https://127.0.0.1:22443/setup?token=`; existen el secreto `<p>_master_key` y la red `<p>_panel` (overlay, attachable, scope `swarm`, subredes privadas); el servicio `<p>_muelle` tiene una tarea corriendo; el contenedor de prueba no existe.
    2. `docker exec <contenedor de la tarea> muelle settings reset-panel-ips --no-restart` (con `exec.Command`) → código 0, salida con "ya estaba vacía" y `docker service update --force <p>_muelle`.
    3. `docker exec <contenedor> muelle settings reset-panel-ips` → código 0 (o la sesión cortada por el reinicio: acepta un código distinto de 0 solo si la salida ya traía "reiniciando"); antes de 90 s hay una tarea corriendo con otro id.
    4. install de nuevo, sin `Reinstall` → `ErrAlreadyInstalled`; no se creó nada nuevo.
    5. Antes de `RemoveStack(<p>)`, anota el `ContainerID` de cada tarea de `<p>_muelle` que lo tenga (`ListTasks` con `Service` y sin `DesiredState`). `RemoveStack(<p>)`, esperar a que el servicio no exista **y** a que cada uno de esos contenedores dé `ErrNotFound` en `InspectContainer` (60 s): `docker stack rm` vuelve antes de que el contenedor termine (hasta el `stop_grace_period`, 10 s por defecto), y un install en ese momento tendría dos muelle sobre la misma `<p>_data`, justo lo que evita el `stop-first` del contrato. (Si un contenedor de una tarea vieja queda `exited` y Swarm no lo borra, basta con que ninguno tenga `Running`: anótalo en el reporte.) Después, install sin `Reinstall` → `ErrPreviousData`; con `Reinstall` → sin error, stdout **sin** clave y con un link nuevo (distinto del primero: cada arranque sin usuarios genera otro), stderr con "la base ya existe".
  - `TestIntegrationInstallPortsBusy` (otro prefijo; puertos 22180 y 22543): el test escucha en `0.0.0.0:22180` con `net.Listen`; install → `ErrPortsBusy`; después no existen `<p>_master_key`, `<p>_panel`, `<p>_muelle` ni `<p>-install-probe`.
  - `TestIntegrationInstallFromImage` (otro prefijo; puertos 22081 y 22444): `docker run --rm --name <p>-installer -v /var/run/docker.sock:/var/run/docker.sock <imagen> install --name-prefix <p> --image <imagen> --panel-origin https://127.0.0.1:22444 --email it5@example.com --public-ip 203.0.113.10 --http-port 22081 --https-port 22444` con `exec.CommandContext` (plazo de 4 min) → código 0, stdout con la clave y el link. Usa `cleanupInstall` y `guardForeign` como `TestIntegrationInstall`, y la limpieza incluye además `docker rm -f <p>-installer`.

  ```bash
  cd /home/felipe/Escritorio/dev/muelle/.claude/worktrees/fase-5-dist
  # reconstruir la imagen con esta rama (ver Convenciones), y después:
  MUELLE_IT_IMAGE=it5-muelle:test go test -tags integration -count=1 -timeout 15m -run '^TestIntegrationInstall' -v ./src/cmd/install/
  docker ps -a --filter name=it5 -q; docker service ls --filter name=it5 -q; docker network ls --filter name=it5 -q
  docker secret ls --filter name=it5 -q; docker volume ls --filter name=it5 -q      # nada en los cinco
  ```
  Commit: `test(install): instalar de verdad con el prefijo it5, reinstalar, recuperar el acceso dentro del contenedor y los puertos ocupados`.

- [ ] **Step 11: README** (secciones nuevas, antes de "Desarrollo local"; contrato §5.2–§5.4 y diseño §3.3 y §16):
  - **"Instalar"**: requisitos (Linux amd64, Docker con Swarm o sin iniciar, los puertos del proxy libres); el comando del diseño §3.3 con `:<versión>`; la tabla de flags; el token por stdin (`docker run -i … --cloudflare-token -`); qué imprime (la clave una sola vez, el link de 24 h) y `> install.txt`; la migración desde Dokploy con `--http-port 8080 --https-port 8443` (diseño §16); el aviso de HTTP-01 sin token. Además, tres cuidados:
    - **siempre con `--rm`**: sin él, el contenedor del instalador queda parado con la clave maestra en `docker logs`. Si se corrió sin `--rm`, `docker rm <contenedor>` después de guardar la clave;
    - **sin `-t`**: con una TTY, Docker junta stderr y stdout, y `> install.txt` guardaría también el avance (con `\r\n`). Para el token por stdin alcanza con `-i`;
    - **varias IPs**: si el Swarm no está iniciado y la máquina tiene varias direcciones, `docker swarm init` no elige una; install falla con el error de Docker y la pista de `--advertise-addr <ip>` (la IP privada por la que se van a hablar los nodos).
  - **"Actualizar"**: `docker service update --image ghcr.io/cpu-commits/muelle:<v> muelle_muelle`; Caddy sigue a muelle salvo con `caddy_image`; `failure_action: rollback`.
  - **"Reinstalar"**, en este orden:
    1. `docker stack rm muelle`;
    2. esperar a que `docker ps -q -f label=com.docker.swarm.service.name=muelle_muelle` quede vacío: `docker stack rm` vuelve antes de que el contenedor termine, y un install en ese momento tendría dos muelle sobre la misma base;
    3. `install --reinstall …` (reusa la clave; los flags no cambian una base que ya arrancó).
  - **"Empezar de cero"** (con la advertencia de que se pierden los datos: la base y los backups de `muelle_data`, y los certificados de Caddy), después de los pasos 1 y 2 de "Reinstalar", la limpieza entera:
    - `docker rm -f muelle-caddy`: es un contenedor aparte, con `restart: unless-stopped`, y `docker stack rm` lo deja corriendo con los puertos del proxy tomados (un install nuevo fallaría con "puertos ocupados");
    - `docker volume rm muelle_data muelle_caddy_data muelle_caddy_config muelle_caddy_admin`;
    - `docker network rm muelle_panel`;
    - `docker secret rm muelle_master_key muelle_init_cloudflare_token muelle_init_registry` (los que existan).

    Los stacks y las redes de las apps (`docker stack ls`, `docker network ls --filter label=muelle.project`) no se tocan: se borran aparte si también se quieren sacar.
  - **"Leer la clave maestra"**: `docker exec "$(docker ps -q -f label=com.docker.swarm.service.name=muelle_muelle | head -n1)" cat /run/secrets/muelle_master_key`.
  - **"Quitar el token de install del servicio"** y **"Cambiar `MUELLE_PANEL_ORIGIN`"**: los dos `docker service update` del contrato §5.2.

  Commit: `docs(readme): instalar, actualizar y reinstalar muelle`.

**Integración:** los tres tests de `src/cmd/install/integration_test.go`, con `-run '^TestIntegrationInstall'` y la imagen reconstruida. Al terminar no queda nada `it5` en Docker salvo las imágenes `it5-muelle:test` e `it5-muelle-caddy:test`. Borra `$CTX`.

---

### Task T5: GitHub Action, CI y documentación final

> **Antes de empezar:** lee "Cambio de requisito del dueño (2026-10-15): las imágenes son PRIVADAS". En "Publicar una versión" ya no se hacen públicos los paquetes, y el README de la action dice cómo usarla si el repo de muelle es privado.

La composite action que reemplaza `sync-env` y `deploy` de los workflows actuales (diseño §14), sus tests, `shellcheck` en CI y el README de la action y de cómo publicar una versión.

**Lee antes:** diseño §14 (con lo que agregó `f87d709`); contrato §2.2 (`POST …/deploys`), §2.4, §2.5 (`…/log?offset=N`) y §0.2 (errores); la ruta `PUT …/apps/{a}/env` en `src/internal/secrets/infra/in/http/env.go` (cuerpo `{"vars", "secret"}`; responde `204`).

**Files:**
- Create: `action/action.yml`, `action/deploy.sh` (modo `100755`), `action/action_test.go`
- Modify: `.github/workflows/ci.yaml`, `README.md`

- [ ] **Step 1: Tests primero** (`action/action_test.go`, `package action`: un paquete con solo tests, decisión 12). Se saltan si falta `bash`, `curl` o `jq` (`exec.LookPath`).
  - Un muelle falso con `httptest.NewServer` que guarda cada petición (método, ruta con query, header `Authorization`, cuerpo) y responde según el test: `PUT …/env` (`204`, como muelle), `POST …/deploys` (`202 {"id":145,"status":"queued","status_url":"/api/v1/projects/auditfy/stages/staging/apps/api/deploys/145"}`), `GET …/deploys/145/log?offset=N` (una secuencia de respuestas por offset) y `GET …/deploys/145` (el deploy, con su `error`).
  - `runAction(t, srvURL, inputs map[string]string) (stdout string, outputs map[string]string, code int)`: corre `bash deploy.sh` en el directorio del paquete con un entorno mínimo (`PATH`, `HOME`), las entradas como `INPUT_*`, `GITHUB_OUTPUT` en un archivo temporal, `MUELLE_ACTION_TEST_POLL=0.01` y `MUELLE_ACTION_TEST_RETRY_DELAYS="0 0"`; lee las salidas `clave=valor` del archivo.

  Tests:
  - `TestActionYAMLPassesInputsByEnv`: lee `action.yml` como texto: `using: composite`; declara `url`, `token`, `app`, `image`, `images-json`, `env-json`, `env-exclude`, `env-secret` (default `"true"`), `timeout` (default `"600"`) y las salidas `deploy-id` y `status`; cada entrada llega como `INPUT_<NOMBRE>: ${{ inputs.<nombre> }}` en `env:` (con `-` → `_` y en mayúsculas: `INPUT_IMAGES_JSON: ${{ inputs.images-json }}`); y la línea `run:` no tiene `${{`.
  - `TestDeploySucceeded`: el log llega en dos partes y después `chunk: ""` con `succeeded` → código 0; stdout trae las líneas del log; `deploy-id=145` y `status=succeeded`; `Authorization: Bearer <token>` en cada petición; `POST` con cuerpo `{"image":"…"}`.
  - `TestDeployBodies`: sin `image` ni `images-json` → `{}`; con `images-json` → `{"images":{…}}`; con los dos → código 1 con `::error::` y **ninguna** petición.
  - `TestDeployFailed`: termina `failed` → `::error::` con el `error` del `GET` del deploy, código 1, `status=failed`.
  - `TestDeploySuperseded`: termina `superseded` con `superseded_by: 146` → `::warning::` con `#146`, código 0.
  - `TestDeployTimeout`: `timeout: 1` y el log siempre `running` y vacío → `::error::` con "timeout", código 1, `status=running`.
  - `TestStopCommandsWrapTheLog`: un `chunk` con `::error::de la app` sale después de `::stop-commands::<tok>` y antes de `::<tok>::`; `<tok>` son 32 hex, distinto en dos corridas; el `::warning::`/`::error::` propio va después del cierre.
  - `TestEnvPutFiltersExcluded`: `env-json` con `github_token`, `MUELLE_URL`, `Muelle_Token`, `EXPORT_PASSPHRASE`, `DB_URL` y `env-exclude: " export_passphrase ,,"` → el `PUT` lleva solo `DB_URL`, con `"secret": true`; con `env-secret: false`, `"secret": false`.
  - `TestEnvPutRetries`: el `PUT` responde `503` dos veces y después `204` → sigue; tres `503` → código 1 sin `POST`. Un `422` no se reintenta. Un `429` con `Retry-After: 1` y después `204` → sigue, con dos `PUT`, y la corrida tardó al menos 1 s (los retrasos de prueba son `0 0`: esperó lo que pidió muelle). Un `429` con `Retry-After: 61` → código 1 con el error de muelle y un solo `PUT` (más de 60 s no se espera). Un `429` sin `Retry-After`, o con una fecha HTTP, usa el retraso que toca.
  - `TestPostNotRetried`: el `POST` responde `503` → código 1, un solo `POST`. Sin respuesta (el servidor cierra la conexión con `Hijack`) → código 1 con "revísalo en el panel", un solo `POST`.
  - `TestPollRetriesTransient`: el `GET …/log` responde `503`, después `429`, después el log → termina bien.
  - `TestErrorsShowCodeAndMessage`: un `404 no_route` en el `POST` → `::error::` que dice que ese muelle todavía no tiene la API de deploys; un `403 out_of_scope` → `::error::` con `out_of_scope` y el `message`.
  - `TestInputsValidated` (sin ninguna petición, código 1): `url` `http://muelle.example.com` (no es https ni localhost), `http://localhost.example.com` y `http://127.0.0.1.nip.io` (empiezan como localhost: un prefijo los dejaría pasar y el token iría en claro), `https://muelle.example.com/panel` (con ruta), `https://u:p@muelle.example.com` (con `usuario@`) y `https://muelle.example.com?x=1`, `app` `a/b` y `a/b/c/d` y `A/b/c`, `env-json` `[1]` y `{"a":1}`, `images-json` `"x"`, `env-secret` `quizás`, `timeout` `0` y `abc`, y un `token` vacío, con un espacio, con `"mk_x\r\nX-Evil: 1"` (agregaría un header) o con un carácter no ASCII. El `::error::` del token no repite el token. Y los que sí sirven (sin servidor: no hay `env-json`, así que la primera petición es el `POST`, que no conecta): `https://[::1]:1/` (con la `/` final) y `http://localhost:1` terminan con el error de "revísalo en el panel", no con el de `url`.
  - `TestOutputsValidated` (lo que responde muelle no agrega salidas): el `POST` responde `202` con `"id": "145\nstatus=succeeded"` → código 1 con un `::error::` que nombra `id`, y `$GITHUB_OUTPUT` no trae ninguna línea `status=` ni `deploy-id=`; un `status_url` correcto pero el log con `"status": "succeeded\nx=y"` → código 1, sin la salida `x`; un `next_offset` `"1&offset=0"` → código 1 sin pedir esa URL; un `superseded_by` `"146\n::error::x"` → código 1.
  - `TestEnvJSONLarge`: un `env-json` de ~100 KiB (una variable con un valor largo; bajo el tope de 128 KiB de una variable de entorno) → el `PUT` lo lleva entero. Junto con el test que sigue, cubre que `env-json` no se copie a un argumento.
  - `TestEnvJSONErrorHidesContent`: un `env-json` que no es JSON (`{"DB_URL": "postgres://u:clave-secreta@db` sin cerrar) → código 1, y ni stdout ni stderr traen `clave-secreta`.
  - `TestSecretsStayOffArgv` (lee `deploy.sh` como texto): `INPUT_ENV_JSON` aparece solo dentro de `env.INPUT_ENV_JSON` de un filtro de `jq` (nunca como `$INPUT_ENV_JSON`: así nunca se pasa con `--arg`/`--argjson` ni a otro comando), e `INPUT_TOKEN` solo en su validación, en el `printf` del archivo de headers y en el `::add-mask::` (los dos son builtins de bash: no hacen `exec`); cada `curl` lleva `--globoff`; y las únicas expansiones `$MUELLE_…`/`${MUELLE_…}` son las de `MUELLE_ACTION_TEST_POLL` y `MUELLE_ACTION_TEST_RETRY_DELAYS` (un `MUELLE_POLL_INTERVAL` del `env:` de un job no cambia nada; los nombres `MUELLE_URL` y `MUELLE_TOKEN` del filtro de `env-json` son strings de `jq`, no variables).
  - `TestCurlGlobOff`: el servidor falso responde un `status_url` `/api/v1/projects/auditfy/stages/staging/apps/api/deploys/[1-2]` → el script pide esa ruta literal (con `[1-2]`), una sola vez por consulta; sin `--globoff`, curl pediría dos URLs y el `%{http_code}` saldría dos veces. Si `net/http` del servidor falso no acepta los corchetes en la ruta, usa `{145}` (otro patrón de curl) y anótalo.
  - `TestStatusURLMustBeLocal`: un `status_url` `https://otro.example.com/api/v1/…` → código 1, y ninguna petición fuera del servidor falso.
  - `TestEscapesWorkflowCommands`: un `error` del deploy con `%` y `\n` sale como `%25` y `%0A` en el `::error::`.

- [ ] **Step 2: `action/action.yml`:**
  ```yaml
  name: muelle deploy
  description: Sincroniza las variables de una app de muelle, la despliega y sigue el log hasta el resultado.
  inputs:
    url:
      description: "Origen del panel de muelle (https://panel.tudominio.cl)"
      required: true
    token:
      description: "Token de API de muelle (mk_…), con alcance a la app"
      required: true
    app:
      description: "proyecto/stage/app"
      required: true
    image:
      description: "Imagen de una app de formulario"
      required: false
      default: ""
    # images-json, env-json, env-exclude: igual, con default ""
    env-secret:
      description: "Guardar las variables como secretas (true o false)"
      required: false
      default: "true"
    timeout:
      description: "Segundos de espera, desde que se creó el deploy"
      required: false
      default: "600"
  outputs:
    deploy-id:
      description: "Id del deploy"
      value: ${{ steps.deploy.outputs.deploy-id }}
    status:
      description: "Estado final, o el último que vio si se pasó el timeout"
      value: ${{ steps.deploy.outputs.status }}
  runs:
    using: composite
    steps:
      - id: deploy
        shell: bash
        # Las entradas van solo por env: dentro de run: una entrada podría inyectar comandos (diseño §14).
        env:
          INPUT_URL: ${{ inputs.url }}
          INPUT_TOKEN: ${{ inputs.token }}
          INPUT_APP: ${{ inputs.app }}
          INPUT_IMAGE: ${{ inputs.image }}
          INPUT_IMAGES_JSON: ${{ inputs.images-json }}
          INPUT_ENV_JSON: ${{ inputs.env-json }}
          INPUT_ENV_EXCLUDE: ${{ inputs.env-exclude }}
          INPUT_ENV_SECRET: ${{ inputs.env-secret }}
          INPUT_TIMEOUT: ${{ inputs.timeout }}
        run: bash "$GITHUB_ACTION_PATH/deploy.sh"
  ```
  Las descripciones van entre comillas: varias llevan `:` o `,`.

- [ ] **Step 3: `action/deploy.sh`** (bash, `set -euo pipefail`). Esqueleto; los nombres de las funciones son sugeridos, el comportamiento es el del diseño §14:
  ```bash
  #!/usr/bin/env bash
  # muelle/action (diseño §14): sincroniza las variables, crea el deploy y sigue
  # su log. Las entradas llegan solo como variables de entorno (INPUT_*).
  set -euo pipefail
  export LC_ALL=C   # las expresiones regulares comparan bytes: [!-~] es solo ASCII visible

  poll="${MUELLE_ACTION_TEST_POLL:-3}"                 # solo para los tests
  read -r -a retry_delays <<< "${MUELLE_ACTION_TEST_RETRY_DELAYS:-2 4}"   # entre los 3 intentos del PUT; solo para los tests

  escape() { local s=$1; s=${s//'%'/'%25'}; s=${s//$'\r'/'%0D'}; s=${s//$'\n'/'%0A'}; printf '%s' "$s"; }
  fail()   { end_log; printf '::error::%s\n' "$(escape "$1")"; exit 1; }
  # output solo recibe valores ya validados (decisión 13): deploy-id con ^[0-9]+$ y status de
  # la lista de estados. Un valor de muelle con \n agregaría otras salidas.
  output() { printf '%s=%s\n' "$1" "$2" >> "${GITHUB_OUTPUT:-/dev/null}"; }
  # want_id CAMPO VALOR: fail (nombra el campo, no repite el valor) si no es ^[0-9]+$.
  # want_status VALOR: fail si no es queued|running|succeeded|failed|superseded.

  # 1. Validar las entradas, sin tocar muelle: url (se le quita la / final y tiene que ser
  #    un origen entero, con la expresión anclada de la decisión 13:
  #      re='^(https://([A-Za-z0-9.-]+|\[[0-9A-Fa-f:]+\])(:[0-9]{1,5})?|http://(localhost|127\.0\.0\.1)(:[0-9]{1,5})?)$'
  #    nunca un patrón de prefijo como http://localhost*), token (re='^[!-~]+$'; [[ $INPUT_TOKEN =~ $re ]]: sin espacios, CR/LF
  #    ni no-ASCII; el error no lo repite), app (tres slugs: ^[a-z][a-z0-9-]*$ cada uno),
  #    image xor images-json, images-json y env-json objetos de strings, leídos SIEMPRE del
  #    entorno y nunca de un argumento (decisión 13):
  #      jq -en 'env.INPUT_ENV_JSON | fromjson | type == "object" and all(.[]; type == "string")' >/dev/null 2>&1
  #    (el stderr de jq se descarta: el error de fromjson cita el texto entero, que serían los secretos;
  #    el ::error:: dice solo "env-json no es un objeto JSON de strings"), env-secret true|false,
  #    timeout entero > 0.

  tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
  umask 077
  printf 'Authorization: Bearer %s\n' "$INPUT_TOKEN" > "$tmp/auth"   # builtin: nunca en un argv
  echo "::add-mask::$INPUT_TOKEN"                                     # una sola línea: el token ya se validó

  # request MÉTODO RUTA [ARCHIVO]: el cuerpo queda en $tmp/body, los headers en $tmp/headers
  # (para el Retry-After de un 429) y el status en $status (000 sin respuesta).
  request() {
    local args=(-sS --globoff -o "$tmp/body" -D "$tmp/headers" -w '%{http_code}' --max-time 30 -X "$1" -H @"$tmp/auth" -H 'Accept: application/json')
    [[ -n "${3:-}" ]] && args+=(-H 'Content-Type: application/json' --data-binary @"$3")
    status=$(curl "${args[@]}" "$url$2" 2>"$tmp/curl.err") || status=000
  }
  # api_error QUÉ: "QUÉ: <status> <code>: <message>", o lo de no_route (diseño §14).

  base="/api/v1/projects/$project/stages/$stage/apps/$app"

  # 2. Si hay env-json: filtrar (GITHUB_TOKEN, MUELLE_URL, MUELLE_TOKEN y env-exclude, sin
  #    distinguir mayúsculas; jq con ascii_upcase e index([$k]) == null), PUT $base/env con
  #    {"vars": …, "secret": …}; hasta 3 intentos con 000, 5xx o 429, con retry_delays entre ellos
  #    (decisión 20). Con un 429, la espera es la mayor entre el retraso que toca y el Retry-After
  #    (segundos enteros, sin \r; muelle manda 60); un Retry-After de más de 60 no se espera:
  #    fail api_error enseguida. Sin Retry-After, o con una fecha HTTP, el retraso que toca.
  #    Otro 4xx no se reintenta.
  #    El cuerpo lo arma jq desde el entorno y va a un archivo, nunca a un argumento:
  #      jq -n --arg secret "$secret" --arg exclude "$INPUT_ENV_EXCLUDE" \
  #        '(env.INPUT_ENV_JSON | fromjson) as $vars | …' > "$tmp/env.json"
  #    (--arg solo con datos que no son secretos: env-exclude y true/false.)
  # 3. POST $base/deploys con {} | {"image"} | {"images"}, una vez: 000 → fail "…revísalo en el
  #    panel"; si no es 202 → fail api_error. id (want_id), status (want_status) y status_url
  #    (tiene que empezar con /api/v1/). output deploy-id; deadline = ahora + timeout.
  # 4. Polling: tok=$(head -c 16 /dev/urandom | od -An -tx1 | tr -d ' \n'); echo "::stop-commands::$tok".
  #    GET "$status_url/log?offset=$offset": 000, 5xx o 429 → sleep $poll y seguir; otro no-200 → fail.
  #    jq -j .chunk (lo imprime tal cual); next_offset (want_id: va en la URL), status (want_status)
  #    y superseded_by (want_id si no es null). Si avanzó, pedir enseguida;
  #    si no y el status es final, salir; si no, sleep $poll. Pasado el deadline: fail "timeout …".
  # 5. end_log ("::$tok::", una sola vez); output status; succeeded → 0; superseded → ::warning:: con
  #    superseded_by → 0; failed → GET $status_url, fail con .error.
  ```
  `chmod +x action/deploy.sh` y `git add --chmod=+x` (el modo queda `100755`).

  ```bash
  cd /home/felipe/Escritorio/dev/muelle/.claude/worktrees/fase-5-dist
  go test -race -count=1 -v ./action/
  # shellcheck: si no está instalado, con su imagen (red para bajarla una vez). La imagen no es
  # it5: si no estaba antes en el nodo, se borra al terminar (nunca con prune).
  bash -c 'if command -v shellcheck >/dev/null; then shellcheck action/deploy.sh; exit; fi
    img=koalaman/shellcheck:stable; had=0; docker image inspect "$img" >/dev/null 2>&1 && had=1
    docker run --rm --name it5-shellcheck -v "$PWD/action:/mnt:ro" "$img" /mnt/deploy.sh; s=$?
    [ "$had" = 1 ] || docker rmi "$img" >/dev/null; exit $s'
  ```
  Expected: PASS y shellcheck sin avisos (o con un `# shellcheck disable=SCxxxx` justificado en un comentario). `docker image ls koalaman/shellcheck` queda como estaba antes.

- [ ] **Step 4: CI**, un paso al final del job `test` de `ci.yaml` (después del de `-tags panel`): `- name: shellcheck` con `run: shellcheck action/deploy.sh` (viene en `ubuntu-latest`). Los tests de la action ya corren en `go test -race ./...`.

- [ ] **Step 5: README:**
  - **"GitHub Action"**: el ejemplo de auditfy del diseño §14 con `@v0`; las entradas y salidas (`url` es solo el origen: `https://panel…`, sin ruta; `http://` solo con `localhost` o `127.0.0.1`); que `env-json` reemplaza todas las variables de la app (`PUT`) y tiene que quedar bajo 128 KiB (es una variable de entorno del paso, y Linux no deja pasar una más grande); los tokens de API con alcance mínimo (un token por proyecto o stage, contrato §0.1); `env-exclude`; qué pasa con `failed`, `superseded` y el timeout.
  - **"Publicar una versión"**: CI en verde; `git tag -a vX.Y.Z -m …` y `git push origin vX.Y.Z` (los hace el dueño); `release.yml` construye y sube las dos imágenes (`:latest` solo las finales); la primera vez, hacer públicos los paquetes `muelle` y `muelle-caddy` en GHCR (pregunta 1); mover el tag mayor de la action: `git tag -f v0 vX.Y.Z && git push -f origin v0` (pregunta 2); `workflow_dispatch` para probar los `Dockerfile` sin subir.
  - **"Estado"** y **"Estructura"**: las fases 2 y 3 también editan esas dos secciones, así que se tocan **solo las líneas de la Fase 5** (agregar las propias, sin reescribir ni reordenar las de las otras fases): al integrar habrá conflictos triviales ahí, que se resuelven dejando las líneas de las tres. "Estado": una línea de la Fase 5 (install, `reset-panel-ips`, imágenes, panel embebido, la action, el release). "Estructura": `version`, `install` y `settings` en la línea de `main.go`; `cmd/install`, `cmd/settingscli` y `cmd/http/panel`; `action/` y `deploy/caddy/`.
  - **"Notas para las fases siguientes"**, bloque **"Fase 5 / install"**: tachar lo que se cumplió (`/tmp` de la imagen, la prueba de puertos con 443/udp y la imagen de la versión, el link de la tarea actual) y dejar:
    - el prefijo `__Host-` de la cookie, al integrar (archivos de identity y `cmd/http/utils`);
    - el test de integración de install corre un muelle de verdad con `docker.sock` y una base nueva, y cuenta con que, con ingress `disabled` (`Enabled` falso, contrato §5.1), **ningún** módulo hace nada con Docker por su cuenta: ingress no crea `muelle-caddy` ni `muelle_panel` (la Fase 3 lo cumple: con `disabled` no hace nada), y ningún job de deploy toca redes, servicios ni stacks sin un deploy. Al integrar, una fase que agregue un job de arranque que limpie o recree recursos de Docker tiene que respetarlo. El test lo verifica sin mirar lo de las otras líneas: falla si aparecen `muelle-caddy` o `muelle_panel`, o si desaparece su red centinela `<p>_sentinel` (con la forma de una red de stage huérfana: `naming.Network` y `naming.StageLabels` de un stage que no está en la base);
    - con `--name-prefix`, muelle-caddy (nombres fijos) no comparte la red ni el socket con ese muelle: es solo para tests;
    - un `GET /healthz` (pendiente de la Fase 1) permitiría un healthcheck en el stack de muelle.

- [ ] **Step 6: Verificar, limpiar y commits.** Las cuatro verificaciones de las convenciones. Borra las imágenes de prueba: `docker rmi it5-muelle:test it5-muelle-caddy:test`. Nada de `docker image prune` ni `docker builder prune`: alcanzarían imágenes y caché de otras líneas. Commits:
  - `feat(action): la GitHub Action de deploy: variables, deploy y log hasta el resultado`
  - `ci: shellcheck del script de la action`
  - `docs(readme): la GitHub Action, publicar una versión y el estado de la Fase 5`

**Integración:** no hay Docker en esta tarea; los tests de la action usan un servidor `httptest` local.

---

## Al cerrar la fase

- `git log fase-23-base..HEAD` muestra los commits de T1..T5 encima de los de documentación; `git status` limpio.
- Nada `it5` en Docker (`docker ps -a`, `service ls`, `network ls`, `secret ls`, `volume ls` e `image ls` filtrados por `it5`).
- El revisor de la fase corre además `go vet -tags panel ./...` y `go test -tags panel ./...` con el build de prueba (T1, Step 7), los tests de integración de esta línea (T3 Step 3 y T4 Step 10) y la comprobación de "Nombres nuevos en paquetes compartidos" (vacía: ningún método o constante de esta fase está también en `fase-2-deploy` o `fase-3-ingress`, y esta rama no declara `PutContainer`).
