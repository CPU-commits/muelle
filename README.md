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

Fase 3 (ingress): muelle crea y mantiene el contenedor muelle-caddy y le manda las rutas (los dominios de cada app y el panel, con TLS por Cloudflare o por HTTP y las IPs permitidas del panel); maneja los dominios con su DNS en Cloudflare (crear, adoptar, reintentar, la IP pública) y el registro del panel; verifica los certificados, y expone `GET /api/v1/proxy`. Ver "Proxy y dominios".

Fase 5 (distribución): el panel embebido en el binario (`-tags panel`), las imágenes `ghcr.io/cpu-commits/muelle` y `muelle-caddy` con su workflow de release, `muelle install`, `muelle settings reset-panel-ips` y la GitHub Action de deploy (`action/`).

Todavía no despliega nada.

## Estructura (spec §4.1)

```
main.go             subcomandos serve, install, genkey, settings y version
Dockerfile          la imagen de muelle (ver "Imágenes"); deploy/caddy/, la de muelle-caddy
action/             la GitHub Action de deploy (ver "GitHub Action")
src/
├── cmd/http/       ServeMux, middleware y errores → HTTP
├── cmd/http/panel/ sirve el panel embebido (-tags panel; ver "Con el panel")
├── cmd/cron/       ejecuta las tareas periódicas de los módulos
├── cmd/install/     muelle install (ver "Instalar")
├── cmd/settingscli/ muelle settings reset-panel-ips (ver "Recuperar el acceso")
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

## Instalar

**Requisitos:** un servidor Linux amd64 con Docker, con un Swarm en el que ese nodo es manager, o sin Swarm (install lo inicia). Los puertos del proxy (80 y 443, o los que elijas) tienen que estar libres.

**Las imágenes son privadas.** Primero, en el servidor, `docker login ghcr.io` con un token de GitHub (classic) que tenga solo el permiso `read:packages`:

```bash
docker login ghcr.io -u <tu-usuario-de-github>      # pega el token cuando lo pida
```

Después, install, montando el `config.json` de ese login (solo lectura) y el socket de Docker:

```bash
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock \
  -v ~/.docker/config.json:/root/.docker/config.json:ro \
  ghcr.io/cpu-commits/muelle:<versión> install \
  --panel-domain panel.tudominio.cl --email tu@correo.cl > install.txt
```

- Sin `:<versión>` se usa `:latest`, la última versión final.
- Si `docker login` guardó la credencial en un helper (`credsStore`, como en Docker Desktop o con `pass`), el `config.json` no la trae y el helper no corre dentro del contenedor: pasa la credencial con `--registry-user` y `--registry-password-stdin`, con el token en stdin (`-i`):

  ```bash
  docker run -i --rm -v /var/run/docker.sock:/var/run/docker.sock \
    ghcr.io/cpu-commits/muelle:<versión> install --panel-domain panel.tudominio.cl --email tu@correo.cl \
    --registry-user <tu-usuario-de-github> --registry-password-stdin < ghcr-token.txt > install.txt
  ```
- install baja con esa credencial la imagen de muelle y la de Caddy, y se la pasa a muelle, que la guarda como el registry `ghcr.io` en su primer arranque (Ajustes → Registries). Con ella, muelle baja la Caddy de la versión siguiente al actualizar, y tus apps pueden usar imágenes privadas de `ghcr.io`.

| Flag | Qué es |
|---|---|
| `--panel-domain <dominio>` | Obligatorio. El panel queda en `https://<dominio>` (con `:<puerto>` si `--https-port` no es 443) |
| `--email <correo>` | Obligatorio. El correo de Let's Encrypt |
| `--cloudflare-token <token>` | El token de la API de Cloudflare: muelle crea los registros DNS y valida los certificados por DNS-01. `-` lo lee de la primera línea de stdin (`docker run -i … --cloudflare-token - < cf-token.txt`), así no queda en el historial de la shell |
| `--public-ip <ipv4>` | La IP pública del servidor. Si no viene, install la detecta con `https://1.1.1.1/cdn-cgi/trace`; si no puede, avisa y sigue |
| `--public-ipv6 <ipv6>` | La IPv6 pública (opcional; no se detecta) |
| `--http-port`, `--https-port` | Los puertos del proxy (80 y 443) |
| `--caddy-image <imagen>` | Fija la imagen de muelle-caddy (avanzado). Sin él, Caddy sigue a la versión de muelle |
| `--advertise-addr <ip>` | La dirección de `docker swarm init`, si el Swarm no está iniciado y la máquina tiene varias IPs. Con un Swarm activo se ignora |
| `--registry-user <usuario>`, `--registry-password-stdin` | La credencial de las imágenes, si no viene del `config.json` (arriba). No se puede juntar con `--cloudflare-token -`: los dos leen stdin |
| `--reinstall` | Vuelve a desplegar después de un `docker stack rm muelle`, con la misma clave y la misma base (ver "Reinstalar") |

**Qué hace:** revisa Docker, el Swarm y que no haya una instalación anterior; prueba los puertos del proxy con un contenedor temporal (`muelle-install-probe`, que borra siempre); inicia el Swarm si hace falta; crea la red `muelle_panel`, el secreto `muelle_master_key` (y `muelle_init_cloudflare_token` y `muelle_init_registry` si corresponde) y el stack `muelle`. Si algo falla antes de que el stack quede desplegado (o lo cortas con Ctrl-C), borra lo que creó. Nada cambia antes de la prueba de puertos: un puerto ocupado aborta sin dejar nada.

**Qué imprime:** por stdout, la **clave maestra, una sola vez**, y el link de setup para crear el primer usuario (de un solo uso, vence en 24 h); por stderr, el avance y los avisos. Con `> install.txt` guardas solo lo que hay que conservar: respalda la clave fuera del servidor (sin ella no se pueden descifrar los secretos ni los backups) y borra el archivo. Si muelle no muestra el link en 2 min, install termina con código 1, deja el stack desplegado y sugiere `docker service logs muelle_muelle` y `docker service ps --no-trunc muelle_muelle`.

Tres cuidados:
- **Siempre con `--rm`:** sin él, el contenedor del instalador queda parado con la clave maestra en `docker logs`. Si lo corriste sin `--rm`, `docker rm <contenedor>` después de guardar la clave.
- **Sin `-t`:** con una TTY, Docker junta stderr y stdout, y `> install.txt` guardaría también el avance (con `\r\n`). Para el token o la contraseña por stdin alcanza con `-i`.
- **Varias IPs:** si el Swarm no está iniciado y la máquina tiene varias direcciones, `docker swarm init` no elige una; install falla con el error de Docker y la pista de `--advertise-addr <ip>` (la IP privada por la que se van a hablar los nodos).

**Sin `--cloudflare-token`,** el registro DNS del panel tiene que existir de antemano y Let's Encrypt valida por HTTP-01, en el puerto 80 público: con `--http-port` distinto de 80, solo funciona si el 80 de tu router o firewall llega a ese puerto (install lo avisa).

**Junto a Dokploy** (diseño §16): su Traefik ocupa 80 y 443, así que install se corre con otros puertos y el token de Cloudflare (DNS-01 no necesita el 80):

```bash
docker run -i --rm -v /var/run/docker.sock:/var/run/docker.sock \
  -v ~/.docker/config.json:/root/.docker/config.json:ro \
  ghcr.io/cpu-commits/muelle:<versión> install --panel-domain panel.tudominio.cl --email tu@correo.cl \
  --cloudflare-token - --http-port 8080 --https-port 8443 < cf-token.txt > install.txt
```

El panel queda en `https://panel.tudominio.cl:8443`. Cuando apagues Dokploy, cambia los puertos del proxy a 80/443 en Ajustes: el panel pasa a `https://panel.tudominio.cl` con la misma sesión (el `:8443` deja de servir), sin tocar la configuración de muelle.

## Actualizar

```bash
docker service update --with-registry-auth --image ghcr.io/cpu-commits/muelle:<versión> muelle_muelle
```

- Con el `docker login ghcr.io` del servidor: `--with-registry-auth` le pasa a Swarm la credencial para bajar la imagen nueva.
- muelle-caddy sigue a muelle: al arrancar, muelle lo recrea con la Caddy de la versión nueva (unos segundos de corte en los sitios), que baja con el registry `ghcr.io` guardado. Con `caddy_image` fijo en Ajustes, Caddy no cambia.
- Si la versión nueva se cae al arrancar, Swarm vuelve sola a la anterior (`failure_action: rollback`, con `monitor: 30s`).
- **Si el token de GitHub vence:** actualízalo en Ajustes → Registries (lo usa muelle) y vuelve a hacer `docker login ghcr.io` en el servidor (lo usa `docker service update`).

## Reinstalar y empezar de cero

**Reinstalar** (volver a desplegar muelle con la misma base y la misma clave, por ejemplo después de borrar el stack), en este orden:
1. `docker stack rm muelle`;
2. esperar a que `docker ps -q -f label=com.docker.swarm.service.name=muelle_muelle` quede vacío: `docker stack rm` vuelve antes de que el contenedor termine, y un install en ese momento tendría dos muelle sobre la misma base;
3. el mismo `docker run … install` con `--reinstall`. Reusa el secreto `muelle_master_key` (no imprime una clave) y, si muelle ya tiene usuarios, lo dice en vez de mostrar un link. Los flags no cambian los ajustes de una base que ya arrancó: se cambian en el panel.

**Empezar de cero** (**se pierden los datos:** la base y sus backups de `muelle_data`, y los certificados de Caddy). Después de los pasos 1 y 2 de "Reinstalar":

```bash
docker rm -f muelle-caddy        # es un contenedor aparte: docker stack rm lo deja con los puertos tomados
docker volume rm muelle_data muelle_caddy_data muelle_caddy_config muelle_caddy_admin
docker network rm muelle_panel
docker secret rm muelle_master_key muelle_init_cloudflare_token muelle_init_registry   # los que existan
```

Los stacks y las redes de las apps (`docker stack ls`, `docker network ls --filter label=muelle.project`) no se tocan: bórralos aparte si también los quieres sacar.

## Después de instalar

**Leer la clave maestra** (si no la guardaste; también vive en el secreto Swarm):

```bash
docker exec "$(docker ps -q -f label=com.docker.swarm.service.name=muelle_muelle | head -n1)" cat /run/secrets/muelle_master_key
```

**Quitar el token y la credencial de install del servicio** (opcional). muelle los guardó en su base en el primer arranque, y después ya no los usa como ajustes, pero los relee en cada arranque: cada secreto se quita junto con su variable, en un solo `docker service update` (reinicia muelle):

```bash
docker service update --secret-rm muelle_init_cloudflare_token --env-rm MUELLE_INIT_CLOUDFLARE_TOKEN_FILE muelle_muelle
docker secret rm muelle_init_cloudflare_token
docker service update --secret-rm muelle_init_registry --env-rm MUELLE_INIT_REGISTRY_FILE muelle_muelle
docker secret rm muelle_init_registry
```

**El origen del panel** sale de los ajustes: `https://<dominio del panel>`, con `:<puerto HTTPS del proxy>` si no es 443. install guarda solo el dominio (`MUELLE_PANEL_DOMAIN`), así que al cambiar los puertos en Ajustes el panel, sus links de invitación y el `Origin` que acepta muelle pasan al puerto nuevo desde que responde el cambio, y el puerto viejo deja de pasar.

**Detrás de otro proxy** (el panel en un origen que muelle no conoce): agrega ese origen, que se acepta además del de los ajustes y pasa a ser el de los links (reinicia muelle, no los sitios):

```bash
docker service update --env-add MUELLE_PANEL_ORIGIN=https://panel.otro-proxy.cl muelle_muelle
```

Si instalaste con una versión que escribía `MUELLE_PANEL_ORIGIN` (con el puerto de entonces), ese origen sigue pasando aunque cambies el puerto. Para que el panel siga a los ajustes: `docker service update --env-rm MUELLE_PANEL_ORIGIN --env-add MUELLE_PANEL_DOMAIN=panel.tudominio.cl muelle_muelle`.

## GitHub Action

`action/` es una composite action (diseño §14) que reemplaza los workflows `sync-env` y `deploy`: guarda las variables de la app, crea el deploy y sigue su log hasta el resultado. Es bash con `curl` y `jq`, que vienen en los runners de GitHub. Necesita la API de deploys (Fase 2): contra un muelle sin ella falla con `404 no_route`.

```yaml
deploy:
  needs: migrate
  runs-on: ubuntu-latest
  environment: ${{ github.ref_name == 'master' && 'production' || 'staging' }}
  steps:
    - uses: CPU-commits/muelle/action@v0
      with:
        url: ${{ secrets.MUELLE_URL }}
        token: ${{ secrets.MUELLE_TOKEN }}
        app: auditfy/${{ github.ref_name == 'master' && 'production' || 'staging' }}/api
        image: ghcr.io/aumi-app/backend-app:sha-${{ github.sha }}
        env-json: ${{ toJson(secrets) }}
        env-exclude: EXPORT_PASSPHRASE
```

| Entrada | Qué es |
|---|---|
| `url` | Obligatoria. El origen del panel, sin ruta: `https://panel.tudominio.cl` (con `:<puerto>` si no es 443). Solo `https://`, salvo `http://localhost` o `http://127.0.0.1`: el token nunca viaja en claro |
| `token` | Obligatoria. Un token de API (`mk_…`) con alcance a la app (abajo) |
| `app` | Obligatoria. `proyecto/stage/app` |
| `image` | La imagen de una app de formulario. Sin `image` ni `images-json`, muelle redespliega las imágenes actuales (contrato §2.2) |
| `images-json` | Las imágenes de una app compose: un objeto JSON `servicio → imagen`, solo con los servicios que cambian. No va junto con `image` |
| `env-json` | Las variables de la app: un objeto JSON de strings; en la práctica `${{ toJson(secrets) }}` |
| `env-exclude` | Claves de `env-json` que no se mandan, separadas por coma. Siempre se excluyen `GITHUB_TOKEN`, `MUELLE_URL` y `MUELLE_TOKEN`, sin distinguir mayúsculas (`toJson(secrets)` trae el token del runner como `github_token`) |
| `env-secret` | `true` (por defecto) guarda las variables como secretas: el panel no vuelve a mostrar su valor y el log las enmascara. `false` las deja visibles |
| `timeout` | Segundos de espera, contados desde que se creó el deploy (600) |

**Salidas:** `deploy-id` y `status` (el estado final, o el último que vio si se pasó el timeout).

**Qué hace:**
1. Valida las entradas, antes de tocar muelle.
2. Si viene `env-json`, `PUT …/apps/<app>/env`, que **reemplaza todas las variables de la app**: una que no viene se borra (las del stage no se tocan). Reintenta hasta 3 veces si muelle no responde o responde 5xx o 429.
3. `POST …/deploys`, una sola vez, porque no es idempotente. Si muelle no responde, falla sin reintentar: el deploy puede haberse creado, revísalo en el panel.
4. Imprime el log del deploy a medida que avanza y termina según el resultado:
   - `succeeded`: el paso sigue;
   - `failed`: un error con el motivo que dio muelle, y el paso falla;
   - `superseded` (otro deploy más nuevo de la misma app lo reemplazó antes de que corriera): un aviso con el id del nuevo, y el paso sigue;
   - el timeout: un error y el paso falla, pero **el deploy sigue en muelle**.

Un error de muelle se muestra con su `code` y su `message` (`403 out_of_scope: …`).

**Cuidados:**
- **El tamaño de `env-json`:** llega al script como una variable de entorno del paso, y Linux no deja pasar una de más de 128 KiB (el paso no arrancaría). Con `toJson(secrets)` son todos los secretos del repo y del environment juntos.
- **Un token por stage:** el alcance de un token es un proyecto, o un proyecto y un stage (`POST /tokens` con `project` y `stage`), y solo puede leer, cambiar variables, desplegar y hacer rollback ahí (diseño §10.1). Guarda el de cada stage en el secreto `MUELLE_TOKEN` de su environment de GitHub: así el job de staging no puede desplegar producción.
- **Lo que imprime la app** no se lee como comandos del runner (`::error::`, `::add-mask::`…): el log va entre `::stop-commands::` y su cierre. Los secretos ya vienen enmascarados por muelle, y GitHub tapa además los `secrets.*`.
- **El token** nunca va en la línea de comandos de un proceso del runner: `curl` lo lee de un archivo `0600` que se borra al terminar, y `jq` lee `env-json` del entorno.
- **La versión:** `@v0` sigue al último release de la serie 0 (se mueve a mano, ver "Publicar una versión"); `@vX.Y.Z` queda fija.
- **Si el repo de muelle es privado,** los repos que usan la action necesitan acceso: en el repo de muelle, Settings → Actions → General → Access, "Accessible from repositories owned by the user". Solo los repos privados del mismo dueño pueden usarla, y sus colaboradores externos ven los logs de la action.

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
| `MUELLE_PANEL_DOMAIN` | — | Dominio del panel, sin esquema ni puerto (lo escribe install). El origen del panel sale de él y del puerto HTTPS de los ajustes: es el `Origin` que se acepta (CSRF) y el de los links de setup e invitaciones |
| `MUELLE_PANEL_ORIGIN` | (obligatoria sin `MUELLE_PANEL_DOMAIN`) | Un origen exacto más, en forma canónica (con el puerto si no es el por defecto): en desarrollo, o detrás de otro proxy. Se compara con el header `Origin` (CSRF) y, si está, arma los links de setup e invitaciones. Sin `MUELLE_PANEL_DOMAIN`, su host (si es un dominio) es el dominio del panel |
| `MUELLE_LISTEN` | `:8080` | Dirección HTTP. No publiques este puerto: solo se llega a muelle por el proxy (ver abajo) |
| `MUELLE_DATA_DIR` | `/data` | Carpeta de `muelle.db` y `backups/` |
| `MUELLE_MASTER_KEY_FILE` | `/run/secrets/muelle_master_key` | Clave maestra en base64 (32 bytes), con permisos 0600 |
| `MUELLE_SECURE_COOKIES` | `true` | `false` solo en desarrollo por http; con un `MUELLE_PANEL_ORIGIN` https, o con `MUELLE_PANEL_DOMAIN`, muelle no arranca |

### Primer usuario

Al arrancar sin usuarios, muelle imprime en el log un link de setup de un solo uso (vence en 24 h):

```
{"level":"INFO","msg":"todavía no hay usuarios: abre este link para crear el primero (vence en 24 h)","event":"setup_url","url":"http://127.0.0.1:18080/setup?token=<TOKEN>"}
```

La página `/setup` llega con el panel (Fase 4); mientras tanto, el usuario se crea con `curl`, con el `token` del link. Toda petición que modifica algo con la cookie de sesión tiene que traer el header `Origin` igual a `MUELLE_PANEL_ORIGIN` (o al origen de los ajustes, con `MUELLE_PANEL_DOMAIN`; si no, `403 bad_origin`):

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

### Con el panel

Compilado sin `-tags panel` (`go run . serve`, `go build`), muelle no trae el panel: toda ruta fuera de `/api` responde `404` con una página que lo dice, y la API anda igual. La imagen de muelle sí lo trae. Para probar el panel en local hay dos caminos:

- **Embebido, como en la imagen:** construir el panel (`web/`, Fase 4; Node 24 y pnpm), copiar el build en `src/cmd/http/panel/build/` (ignorado por git) y compilar con el tag:

  ```bash
  (cd web && pnpm install && VITE_USE_MOCKS=0 pnpm build)
  rm -rf src/cmd/http/panel/build && mkdir -p src/cmd/http/panel/build
  cp -R web/build/. src/cmd/http/panel/build/
  MUELLE_PANEL_ORIGIN=http://127.0.0.1:18080 \
  MUELLE_SECURE_COOKIES=false \
  MUELLE_LISTEN=127.0.0.1:18080 \
  MUELLE_DATA_DIR=/tmp/muelle-data \
  MUELLE_MASTER_KEY_FILE=/tmp/muelle.key \
  go run -tags panel . serve                  # el panel en http://127.0.0.1:18080/
  ```

  muelle sirve el archivo del build o, si no existe, el `200.html`, siempre con la CSP de `csp-header.txt` (contrato §5.5). Si al build le falta `200.html` o `csp-header.txt`, muelle no arranca; con el tag y sin `src/cmd/http/panel/build/`, no compila. Después de cada `pnpm build` hay que volver a copiarlo: el hash de la CSP cambia en cada build. `VITE_USE_MOCKS=0` le gana a un `VITE_USE_MOCKS=1` de los `web/.env*`, que metería los mocks (MSW) en el build.
- **`pnpm dev` en `web/`:** Vite proxea `/api` a un `muelle serve` local, con `MUELLE_PANEL_ORIGIN=http://localhost:5173` (ver `web/README.md`). No hace falta el tag.

### Docker

Crear y borrar un stage crea y borra su red overlay, y borrar una app hace `docker stack rm` (con el CLI `docker` en el `PATH`). Para eso, el Docker local tiene que ser manager de un Swarm (`docker swarm init`); si no, esas peticiones responden `503 docker_unavailable` y no cambian nada. El resto de la API anda sin Docker. Sin un Swarm, esos caminos se prueban con los tests, que usan el Docker en memoria (`dockertest`, que `container.WithDocker` inyecta).

## Proxy y dominios

ingress crea muelle-caddy y lo mantiene: el job `"proxy"`, al arrancar y cada 5 minutos, asegura el contenedor y las redes, y le reenvía la configuración. Cambiar los puertos del proxy en Ajustes lo recrea 2 s después de responder el `PATCH` (contrato §3.5).

- **En desarrollo** (un `MUELLE_PANEL_ORIGIN` con una IP o `localhost`, que deja `panel_domain` vacío, o un binario sin versión y sin `caddy_image`), muelle no crea muelle-caddy ni toca los puertos del host, y `GET /api/v1/proxy` dice `disabled`. Los dominios y su DNS en Cloudflare funcionan igual.
- **El token de Cloudflare** (Ajustes) necesita los permisos Zone:Read y DNS:Edit sobre las zonas de tus dominios. muelle lo verifica al guardarlo, y con él crea los registros de los dominios y el del panel, y valida los certificados por DNS-01.
- **Una `caddy_image` propia** (`MUELLE_INIT_CADDY_IMAGE`) tiene que ser Caddy 2.10 o más nuevo, con el módulo `dns.providers.cloudflare` (contrato §3.4). Antes de la 2.10, la admin API por socket unix revisa el header `Host` y rechaza el que manda muelle (caddy#6832), y todo `POST /load` falla. La imagen `muelle-caddy` de cada versión cumple las dos cosas.
- **Imágenes privadas:** muelle baja la muelle-caddy de cada versión nueva con el registry `ghcr.io` de Ajustes → Registries, que siembra install. Si su token vence, muelle-caddy sigue con la imagen anterior y el error de `GET /api/v1/proxy` dice que hay que actualizar la credencial en Ajustes → Registries (el token de GitHub con `read:packages` que explica la parte de install, Fase 5).

**Antes de migrar los sitios (spec §15), a mano, contra una zona de prueba:**
1. Guarda en Ajustes un token que vea solo la zona de prueba, y la IP pública del servidor.
2. Crea un dominio de esa zona: tiene que pasar de `pending` a `active` en menos de 2 minutos (criterio §1.4), con el certificado emitido por DNS-01.
3. Cambia su `proxied` y mira en Cloudflare que el registro cambió.
4. Crea a mano en Cloudflare un registro de otro nombre, agrega ese nombre como dominio (queda en `error` con `record_exists`) y adóptalo.
5. Borra el dominio y mira que su registro se fue de Cloudflare.

**Tests de integración de ingress**, con Caddy real (necesitan un Swarm activo en el que este nodo es manager, y las imágenes `caddy:2-alpine` y `alpine/socat:latest`; si falta algo, se saltan):

```bash
go test -tags integration -count=1 -run Integration -v ./src/internal/ingress/...
```

Solo crean contenedores, redes y volúmenes con el prefijo `it3`, en los puertos 21000–21999, y los borran al terminar; nunca usan 80, 443 ni 2019, ni la API real de Cloudflare. No se corren a la vez que la integración de la Fase 5: su test de install falla si mientras corre desaparece una red con la etiqueta `muelle.project`, y estos crean y borran redes de stage.

## Solo detrás de Caddy (o de otro proxy)

**muelle nunca debe quedar expuesto directo.** Para limitar los logins por IP usa la IP del cliente, y confía en el header `X-Real-IP` cuando la conexión viene de loopback o de una red privada (RFC 1918, ULA), que es por donde llega Caddy (spec §8). Si un cliente puede conectarse a muelle sin pasar por el proxy desde una de esas redes, escribe su propio `X-Real-IP` y se salta el límite por IP.

- El `MUELLE_LISTEN=:8080` por defecto escucha en todas las interfaces: **no publiques ese puerto** (ni con `-p`/`ports:` de Docker ni en el firewall del host). En producción muelle está en la red `muelle_panel`, a la que solo llega Caddy.
- El proxy tiene que **pisar** `X-Real-IP` con la IP real del cliente, nunca reenviar la que mande el cliente. En un Caddyfile es `header_up X-Real-IP {client_ip}`; en la configuración JSON que manda muelle a muelle-caddy, `{http.vars.client_ip}` (el atajo `{client_ip}` no existe en JSON; contrato §3.4).
- En desarrollo, escucha en `127.0.0.1`, como en el ejemplo de arriba.

## Recuperar el acceso

Si guardaste una lista de IPs permitidas del panel (`panel_allowed_cidrs`, en Ajustes) que te deja fuera, por ejemplo porque cambió tu IP, vacíala desde el servidor, dentro del contenedor de muelle:

```bash
docker exec "$(docker ps -q -f label=com.docker.swarm.service.name=muelle_muelle | head -n1)" muelle settings reset-panel-ips
```

- Deja la lista en `[]` (cualquier IP) y reinicia muelle, como `docker service update --force muelle_muelle`: al arrancar, muelle le manda a Caddy la configuración sin la lista. El panel y los streams se cortan unos segundos; un deploy en curso se resuelve con la reconciliación del arranque.
- El cambio queda en la auditoría como `settings.update`, con actor `system` y `"via": "cli"`. No necesita la clave maestra.
- Si la lista ya estaba vacía, no cambia nada y lo dice (el problema es otro: mira los puertos del proxy, abajo), pero reinicia igual: ese arranque le reenvía la configuración a Caddy.
- `--no-restart` solo cambia la base e imprime el comando para reiniciar a mano. Mientras tanto, Caddy sigue filtrando con la lista anterior, hasta que muelle le reenvíe la configuración (el job `"proxy"`, cada 5 min).
- Si no puede reiniciar (Docker no responde, o el contenedor no es una tarea de Swarm), lo dice, imprime el comando para reiniciar a mano y termina con código 1: la base ya quedó cambiada. Fuera del contenedor de muelle (sin su entorno o sin `muelle.db` en `MUELLE_DATA_DIR`) no toca nada, termina con 1 y muestra el `docker exec` de arriba. Con argumentos que no sirven, termina con 2.

**Si el problema son los puertos del proxy.** Si cambiaste `proxy_http_port` o `proxy_https_port` a unos que estaban ocupados, muelle-caddy no arranca en los nuevos ni vuelve solo a los anteriores, y sin Caddy no se llega al panel. Vuelve a poner los de antes directo en la base. La imagen de muelle no trae `sqlite3`, así que se usa un contenedor aparte sobre el mismo volumen (SQLite en modo WAL admite la escritura con muelle corriendo). Cambia `80` y `443` por los puertos que tenías:

```bash
docker run --rm -v muelle_data:/data alpine sh -c \
  "apk add -q sqlite && sqlite3 /data/muelle.db \"UPDATE settings SET value='80' WHERE key='proxy_http_port'; UPDATE settings SET value='443' WHERE key='proxy_https_port'\""
```

Después reinicia muelle desde el host (`docker service update --force muelle_muelle`), y al arrancar recrea muelle-caddy con esos puertos. También puedes esperar al job `"proxy"` (a lo más 5 min). `muelle settings reset-panel-ips` también reinicia, pero además vacía la lista de IPs permitidas: úsalo si también te dejó fuera.

## Tests

```bash
gofmt -l $(git ls-files '*.go')   # no debe imprimir nada
go mod tidy -diff
go vet ./...
go test -race ./...
```

Los tests de `action/` corren el `deploy.sh` de la GitHub Action con bash, `curl` y `jq` contra un muelle falso (se saltan si falta alguno); CI corre además `shellcheck action/deploy.sh`.

Los tests de `src/shared/infra/container` arman muelle completo con los cuatro módulos (matriz de permisos de los tokens, contratos entre módulos, fechas). La prueba de punta a punta con el binario real y `curl` (spec §15) por ahora es manual: los pasos de "Primer usuario" de arriba, más los endpoints de la fase.

## Imágenes

Dos imágenes, solo para `linux/amd64` (contrato §5.4):

| Imagen | Qué trae |
|---|---|
| `ghcr.io/cpu-commits/muelle` | El binario estático (sin CGO) con el panel embebido y su versión (`muelle version`), el CLI de `docker` 29.9.0 para `docker stack deploy` y `ca-certificates`, sobre Alpine (que trae el `/tmp` y el `cat` que usa muelle). Corre como root, porque usa `docker.sock`. `ENTRYPOINT ["muelle"]` y `CMD ["serve"]`, sin `EXPOSE`: el puerto de muelle nunca se publica |
| `ghcr.io/cpu-commits/muelle-caddy` | Caddy 2.11.7 con el módulo de DNS de Cloudflare (`dns.providers.cloudflare`, compilado con `xcaddy`). No trae configuración: ingress la arranca con `caddy run --resume` y la admin API en un socket |

- **Versiones fijas:** las imágenes base, el frontend de BuildKit (la línea `# syntax`) y el módulo de Cloudflare van con versión exacta y, las imágenes, con su digest.
- **Publicación:** `.github/workflows/release.yml` las construye y las sube a GHCR al empujar un tag `vX.Y.Z[-sufijo]`, después de los mismos tests de CI. Las dos llevan el mismo tag, que es también la versión del binario. `:latest` existe solo para muelle y solo para las versiones finales (sin `-sufijo`). Con `workflow_dispatch` (Actions → Release → Run workflow) construye sin subir, para probar los `Dockerfile`.
- **Paquetes privados:** las dos son privadas. Para bajarlas hace falta `docker login ghcr.io` con un token de GitHub con permiso `read:packages`.

Para construirlas en local, desde la raíz del repo:

```bash
docker build --build-arg VERSION=v0.1.0 -t ghcr.io/cpu-commits/muelle:v0.1.0 .
docker build --build-arg VERSION=v0.1.0 -t ghcr.io/cpu-commits/muelle-caddy:v0.1.0 deploy/caddy
docker run --rm ghcr.io/cpu-commits/muelle:v0.1.0 version            # v0.1.0
```

La imagen de muelle construye el panel desde `web/` (Node 24 y pnpm 11.1.2, dentro de la imagen) y compila con `-tags panel`. La etapa del panel construye con `VITE_USE_MOCKS=0`, y el build de la imagen falla si al panel le falta el `200.html` o la CSP, o si trae el service worker de MSW. El `.dockerignore` es una lista de permitidos: al contexto solo entran `go.mod`, `go.sum`, `main.go`, `src/` y `web/`, sin tests, claves, bases, `.env`, `node_modules` ni builds.

Mientras el panel siga en su rama (`fase-4-panel`), `web/` no está en esta, y el contexto se arma aparte, con el árbol de trabajo más el `web/` de esa rama:

```bash
CTX=$(mktemp -d)
git ls-files -co --exclude-standard -z | tar --null -T - -cf - | tar -x -C "$CTX"
git archive fase-4-panel web | tar -x -C "$CTX"
docker build --build-arg VERSION=v0.0.0-dev -t muelle:dev "$CTX"
```

## Publicar una versión

Lo hace el dueño del repo, desde `main` con CI en verde:

1. Crear y subir el tag (con sufijo, `v0.2.0-rc.1`, es una prerelease):

   ```bash
   git tag -a vX.Y.Z -m "muelle vX.Y.Z"
   git push origin vX.Y.Z
   ```
2. `release.yml` corre los mismos tests que CI y después construye y sube `ghcr.io/cpu-commits/muelle:vX.Y.Z` y `ghcr.io/cpu-commits/muelle-caddy:vX.Y.Z`. `:latest` (solo muelle) se mueve solo con las versiones finales.
3. Los paquetes quedan **privados**: no hay que cambiar nada en GHCR. El label `org.opencontainers.image.source` los liga al repo.
4. Cuando quieras que el release llegue a los repos que usan la action con `@v0`, mueve el tag mayor (nunca a una prerelease):

   ```bash
   git tag -f v0 'vX.Y.Z^{}'          # el commit del release
   git push -f origin v0
   ```

Para probar los `Dockerfile` sin publicar nada: Actions → Release → Run workflow (`workflow_dispatch`) construye las dos imágenes sin subirlas.

## Convenciones

Comentarios y mensajes de error en español; identificadores en inglés.

## Notas para las fases siguientes

Hallazgos de la revisión de código que las próximas fases deben respetar.

**Fase 2, render**
- Escapar `$` como `$$` en todo string que venga del spec de formulario (`command`, `healthcheck.command`, imagen, mounts, valores de env).
- Generar el healthcheck `http` en forma exec (sin shell).
- Fijar el nombre del proyecto compose explícitamente y usar variables de entorno solo para interpolación (las claves `COMPOSE_*` están reservadas).
- La lista de permitidos y de rechazos de §6.3: entre otras, `secrets:`/`configs:` (con `file:` o `external` montarían la clave maestra en la app), volúmenes con `driver`/`driver_opts`/`external`/`name` (un `driver_opts` con `o: bind` monta `docker.sock`), `env_file`/`extends`/`include` y etiquetas `muelle.*` propias. Un test por cada rechazo.
- Interpolar solo con las variables de `ResolveEnv` (nunca `os.Environ()`), y escapar todo `$` como `$$` en el compose final, porque `docker stack deploy` vuelve a interpolar.
- Comprobar que el tamaño del env mezclado stage+app cabe en el `ARG_MAX` de Linux.
- `secrets.ResolveEnv` ya está conectado al deploy (puerto `EnvResolver`), igual que `docker stack rm` al borrar una app.
- Reemplazar `validateCompose` por la validación real de compose-go, incluyendo nombres de servicio únicos por stage. La parte pura (parsear, las restricciones de §6.3, los nombres de los servicios) va en un paquete de `shared/domain`, para que catalog la use al guardar y deploy al renderizar sin importarse.

**Fase 2, worker de deploy**
- Las goroutines nunca deben usar el ctx de una transacción después de que `InTx` retorna; derivar de un ctx externo (`context.WithoutCancel`).
- La retención guarda siempre el deploy actual y el destino del rollback, además de los 20 más nuevos (contrato §2.3).
- Enmascarar también cada línea de un secreto de varias líneas, y nada de menos de 8 bytes (contrato §2.5).
- Los SSE revisan la credencial cada 60 s y tienen límites por principal y de duración (contrato §0.3).
- Un deploy sin cuerpo le quita el digest a una ref con tag, pero una ref sin tag (`…@sha256:…`) queda fija: sin el digest, Docker la leería como `:latest` (contrato §2.2).

**Fase 2, estado y logs**
- `GET …/logs` acepta `since` en vez de `tail` (inclusivo, hasta 5000 líneas), con `time` truncado al milisegundo: el panel lo manda al reconectar (contrato §2.9).
- El servicio de los logs se busca por `naming.Service(p, s, a, service)` y con las etiquetas de la app, nunca con el parámetro tal cual: Docker también acepta un id o un prefijo de id (contrato §2.9).
- Las medidas de `GET …/status` (`ContainerStats`) se guardan 5 s por app (contrato §2.8).

**Fase 3**
- Crear un stage ya crea la red overlay y conecta Caddy (etapa de contratos).
- El pool de direcciones de la overlay debe ser privado (RFC1918/ULA) para que se pueda confiar en `X-Real-IP` (`docker.Network.Subnets`). Verificarlo en `muelle_panel` le toca a install: ingress no lo revisa.
- ~~Las IPs permitidas del panel van con el matcher `client_ip` de Caddy, no `remote_ip` (contrato §3.4).~~
- ~~La configuración de Caddy es JSON: placeholders completos (`{http.vars.client_ip}`), nunca los atajos del Caddyfile. El golden lo verifica.~~
- ~~muelle-caddy se crea con `naming.CaddyCommand()` y `naming.CaddyEnv()`, nunca con el CMD de la imagen (la admin API quedaría en TCP, sin el socket).~~
- ~~Docker 29 arranca sin puertos un contenedor cuyo arranque falló con `ErrPortInUse`: muelle-caddy se recrea, nunca se rearranca (`Container.PublishedPorts`).~~
- Los rangos de IP de Cloudflare de `trusted_proxies` (`ingress/infra/out/service/caddy/ranges.go`) son una lista estática: se revisan con cada versión contra cloudflare.com/ips-v4 y ips-v6.
- La emisión por DNS-01 con el módulo de Cloudflare no tiene un test automático (la imagen oficial de Caddy no lo trae, y no se usa la API real): se prueba a mano contra una zona de prueba, con los pasos de "Proxy y dominios".
- El aislamiento entre stages por nombre lo prueba `TestIntegrationCaddyLifecycle`; por IP (sin pasar por el nombre del servicio) no hay un test automático, y queda para la prueba manual.

**Fase 5 / install**
- ~~La imagen de muelle necesita un `/tmp` escribible; la prueba de puertos incluye 443/udp y usa `MuelleImageFor(version)`, bajándola si falta; el link de setup se lee de la tarea actual (contrato §5.4).~~ Hecho en la Fase 5.
- Al integrar, considerar el prefijo `__Host-` en el nombre de la cookie de sesión cuando las cookies son Secure (los archivos de identity y `cmd/http/utils`).
- El test de integración de install (`src/cmd/install/integration_test.go`) corre un muelle de verdad con `docker.sock` y una base nueva, y cuenta con que, con ingress `disabled` (`Enabled` falso, contrato §5.1), **ningún** módulo hace nada con Docker por su cuenta: ingress no crea `muelle-caddy` ni `muelle_panel` (la Fase 3 lo cumple: con `disabled` no hace nada), y ningún job de deploy toca redes, servicios ni stacks sin un deploy. Al integrar, una fase que agregue un job de arranque que limpie o recree recursos de Docker tiene que respetarlo. El test lo verifica sin mirar lo de las otras líneas: falla si aparecen `muelle-caddy` o `muelle_panel`, o si desaparece su red centinela `<prefijo>_sentinel` (con la forma de una red de stage huérfana: `naming.Network` y `naming.StageLabels` de un stage que no está en la base).
- Con `--name-prefix` (solo para tests), muelle-caddy, que tiene nombres fijos, no comparte la red ni el socket con ese muelle.
- Un `GET /healthz` (pendiente de la Fase 1, abajo) permitiría un healthcheck en el stack de muelle.

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
