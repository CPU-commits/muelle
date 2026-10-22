<p align="center">
  <img src="web/src/shared/ui/assets/favicon.svg" width="72" height="72" alt="">
</p>

<h1 align="center">muelle</h1>

<p align="center">
  Despliega contenedores en tu propio servidor con Docker Swarm: panel web, API, secretos cifrados, stages y dominios con HTTPS automático.
</p>

---

muelle es un deploy manager self-hosted. Cubre lo que se usa a diario de un PaaS: desplegar imágenes que ya se construyeron en CI, guardar sus variables y secretos, separar cada proyecto en stages (staging, production) y servir cada app en sus dominios con certificados automáticos. No trae más que eso: en el servidor no hay Postgres, Redis ni Node. Son un binario de Go con SQLite, que corre como servicio de Swarm, y un Caddy aparte que hace de proxy.

- **Apps de formulario o compose.** Una app es una imagen con su puerto, réplicas, healthcheck, volúmenes y límites de CPU y memoria, o un compose con varios servicios. Las dos se despliegan igual.
- **Proyectos, stages y apps.** Cada stage tiene su propia red: staging no ve a production, ni un proyecto a otro.
- **Variables y secretos** por stage y por app. Los secretos se guardan cifrados y salen enmascarados en los logs.
- **Deploys sin corte.** La versión nueva arranca antes de bajar la anterior y, si no pasa el healthcheck, la anterior sigue sirviendo. El log se ve en vivo, y hay historial y rollback.
- **Dominios con HTTPS.** Con un token de Cloudflare, muelle crea el registro DNS y Caddy saca el certificado de Let's Encrypt. Opcionalmente, certificados de Cloudflare Origin CA para el modo Full (strict).
- **Un panel web** con varios usuarios, 2FA, invitaciones, IPs permitidas, auditoría y un backup diario de la base.
- **Una API REST** con tokens que se limitan a un proyecto o a un stage, y que pueden ser de solo lectura.
- **Una GitHub Action** que guarda las variables de la app, despliega y sigue el log hasta el resultado.
- **Un servidor MCP** para que Claude Code lea el estado, los deploys y los logs de tus apps y, con un token que puede escribir, despliegue.

**Lo que no hace:** construir imágenes desde git (se construyen en CI), publicar puertos TCP, respaldar bases de datos, administrar nodos, abrir una terminal en los contenedores, mandar notificaciones, guardar métricas históricas, roles por proyecto ni entornos de preview.

## Cómo funciona

```
             Internet :80 / :443 (+443/udp)   (publicados directo en el host: llega la IP real del cliente)
                      │
              ┌───────▼────────┐   socket unix en un volumen   ┌──────────────────┐
              │  muelle-caddy  │◄────────── (admin API) ────────│      muelle      │
              │  (contenedor   │                                │   panel + API    │
              │   aparte)      │                                │ (servicio Swarm) │
              └─┬────┬──────┬──┘                                └────────┬─────────┘
                │    │      │  red muelle_panel (solo Caddy y muelle)     │ docker.sock
                │    │      └─────────────────────────────────────────────┤ /data (SQLite)
                │    │ conectado a la red de cada stage
     red        │    └───────────────────┐ red
     tienda_staging                      │ tienda_production
   ┌────────────┴─────────┐   ┌──────────┴───────────┐
   │ tienda_staging_api   │   │ tienda_production_   │   (stacks que despliega muelle)
   │ worker, redis …      │   │ api, worker, redis … │
   └──────────────────────┘   └──────────────────────┘
```

Las dos piezas viven en el nodo manager:

- **muelle** es el servicio del stack `muelle`. Sirve el panel y la API (`/api/v1`), guarda todo en SQLite (volumen `muelle_data`) y maneja Docker por `docker.sock`. Su puerto nunca se publica: solo se llega a muelle a través de Caddy. La clave maestra vive en el secreto Swarm `muelle_master_key`.
- **muelle-caddy** es un contenedor aparte, no un servicio de Swarm, con los puertos del proxy publicados en el host. muelle arma la configuración completa de Caddy desde la base y se la manda por la admin API, en un socket unix. Es un contenedor y no un servicio porque así se conecta en caliente a la red de cada stage nuevo (a un servicio, agregarle una red lo recrea y cortaría todos los sitios). Reiniciar muelle no corta los sitios.

**Redes.** Cada stage es una red overlay `<proyecto>_<stage>`, con sus apps y con Caddy, que es el único conectado a más de una. En `muelle_panel` solo están Caddy y muelle: las apps no llegan a muelle. Caddy enruta por el nombre completo de cada servicio (`proyecto_stage_app_servicio`), así el `api` de staging nunca resuelve al de production.

**Un deploy** (`POST …/apps/{app}/deploys`, desde el panel, la API, la action o Claude Code):

1. **Se encola.** Cada app despliega de a uno, y apps distintas en paralelo. Si llega otro deploy mientras uno espera, el que esperaba queda `superseded`.
2. **Resuelve las imágenes.** Pide el digest de cada una con las credenciales de su registry: el deploy fija exactamente la imagen que se pidió, y un rollback vuelve exactamente a la anterior.
3. **Arma el compose final.** Le pone la red del stage, la rotación de logs, `update_config` con `order: start-first` y `failure_action: rollback`, las etiquetas de muelle y las variables ya resueltas. Valida el compose con una lista de permitidos: una app no puede montar rutas del host ni `docker.sock`, pedir privilegios, ni leer los archivos o los secretos de muelle.
4. **Aplica y espera.** Corre `docker stack deploy` y espera a que Swarm converja; con healthcheck, una tarea cuenta solo cuando está sana. Si Swarm vuelve atrás o se pasa el timeout, el deploy falla con el motivo y la versión anterior sigue sirviendo.
5. **Actualiza las rutas.** Le reenvía a Caddy la configuración, por si cambiaron los servicios que son destino de un dominio.

El log de cada paso se ve en vivo, con los secretos tapados. Un rollback es un deploy nuevo con las imágenes del último que salió bien. Un deploy sin imagen repite las del actual. Un deploy que no cambia nada no reinicia los contenedores. Si muelle se reinicia con un deploy en curso, al arrancar lo cierra según el estado real de Swarm.

**Dominios.** Cada dominio apunta a un servicio y un puerto de una app.
- Si cae en una zona que ve tu token de Cloudflare, muelle crea su registro `A` (y `AAAA`), con o sin el proxy de Cloudflare, y Caddy valida el certificado por DNS-01.
- Si no, el registro se crea a mano y el certificado sale por HTTP-01.
- muelle no pisa un registro que ya existe y que no creó: lo puedes adoptar desde el panel.
- Verifica el certificado que sirve Caddy para cada dominio, y avisa si es inválido o vence en menos de 14 días.

**Seguridad.** muelle tiene `docker.sock`, que equivale a ser root en el host. Por eso:
- Los secretos van cifrados con AES-256-GCM, ligados a su fila: un valor copiado a otra fila no se descifra. La clave maestra nunca toca la base ni los logs, y muelle no arranca con una clave distinta de la que cifró la base.
- Las contraseñas van con Argon2id. Hay bloqueos progresivos por usuario e IP, 2FA con TOTP (que se puede exigir a todos) y una lista opcional de IPs permitidas para el panel.
- La sesión es una cookie `HttpOnly` y `SameSite=Strict` que vence a las 12 h sin uso, y toda petición que cambia algo tiene que traer el `Origin` exacto del panel.
- Los tokens de API (`mk_…`) se guardan como hash y se limitan a un proyecto o a un stage. Uno de solo lectura no puede escribir, y no existe un token que escriba en todos los proyectos.
- Cada cambio queda en la auditoría, y la base se respalda a diario (`/data/backups`, los últimos 7).
- Dos límites aceptados: quien tiene acceso a Docker en el host ve las variables con `docker inspect` (ya es root), y las credenciales de los registries son de todo muelle, una por host.

## Instalar

**Requisitos:** un servidor Linux amd64 con Docker. Si ya tiene un Swarm, el nodo tiene que ser manager; si no, install lo inicia. Los puertos del proxy (80 y 443, u otros que elijas) tienen que estar libres.

**Las imágenes son privadas.** Primero, en el servidor, haz `docker login ghcr.io` con un token de GitHub (classic) que tenga solo el permiso `read:packages` y acceso a los paquetes:

```bash
docker login ghcr.io -u <tu-usuario-de-github>      # pega el token cuando lo pida
```

Después corre install, montando el `config.json` de ese login (solo lectura) y el socket de Docker:

```bash
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock \
  -v ~/.docker/config.json:/root/.docker/config.json:ro \
  ghcr.io/cpu-commits/muelle:<versión> install \
  --panel-domain panel.tudominio.cl --email tu@correo.cl > install.txt
```

- Sin `:<versión>` se usa `:latest`, la última versión final.
- Si `docker login` guardó la credencial en un helper (`credsStore`, como en Docker Desktop o con `pass`), el `config.json` no la trae y el helper no corre dentro del contenedor. En ese caso, pasa la credencial con `--registry-user` y `--registry-password-stdin`, con el token en stdin (`-i`):

  ```bash
  docker run -i --rm -v /var/run/docker.sock:/var/run/docker.sock \
    ghcr.io/cpu-commits/muelle:<versión> install --panel-domain panel.tudominio.cl --email tu@correo.cl \
    --registry-user <tu-usuario-de-github> --registry-password-stdin < ghcr-token.txt > install.txt
  ```
- install baja con esa credencial la imagen de muelle y la de Caddy, y se la pasa a muelle, que la guarda como el registry `ghcr.io` en su primer arranque (Ajustes → Registries). Con ella, muelle baja la Caddy de cada versión nueva al actualizar, y tus apps pueden usar imágenes privadas de `ghcr.io`.

| Flag | Qué es |
|---|---|
| `--panel-domain <dominio>` | Obligatorio. El panel queda en `https://<dominio>` (con `:<puerto>` si `--https-port` no es 443) |
| `--email <correo>` | Obligatorio. El correo de Let's Encrypt |
| `--cloudflare-token <token>` | El token de la API de Cloudflare: muelle crea los registros DNS y valida los certificados por DNS-01. Con `-`, lo lee de la primera línea de stdin (`docker run -i … --cloudflare-token - < cf-token.txt`), así no queda en el historial de la shell |
| `--public-ip <ipv4>` | La IP pública del servidor. Si no viene, install la detecta con `https://1.1.1.1/cdn-cgi/trace`; si no puede, avisa y sigue |
| `--public-ipv6 <ipv6>` | La IPv6 pública (opcional; no se detecta) |
| `--http-port`, `--https-port` | Los puertos del proxy (80 y 443) |
| `--caddy-image <imagen>` | Fija la imagen de muelle-caddy (avanzado). Sin él, Caddy sigue a la versión de muelle |
| `--advertise-addr <ip>` | La dirección de `docker swarm init`, si el Swarm no está iniciado y la máquina tiene varias IPs. Con un Swarm activo se ignora |
| `--registry-user <usuario>`, `--registry-password-stdin` | La credencial de las imágenes, si no viene del `config.json` (arriba). No se puede juntar con `--cloudflare-token -`: los dos leen stdin |
| `--reinstall` | Vuelve a desplegar después de un `docker stack rm muelle`, con la misma clave y la misma base (ver "Reinstalar y empezar de cero") |

**Qué hace.** Revisa Docker, el Swarm y que no haya una instalación anterior, y prueba los puertos del proxy con un contenedor temporal (`muelle-install-probe`, que siempre borra). Nada cambia antes de esa prueba: un puerto ocupado aborta sin dejar nada. Después inicia el Swarm si hace falta, crea la red `muelle_panel`, el secreto `muelle_master_key` (y `muelle_init_cloudflare_token` y `muelle_init_registry` si corresponde) y el stack `muelle`. Si algo falla antes de que el stack quede desplegado (o lo cortas con Ctrl-C), borra lo que creó. En su primer arranque, muelle crea muelle-caddy y la ruta del panel.

**Qué imprime.** Por stdout, la **clave maestra, una sola vez**, y el link de setup para crear el primer usuario (de un solo uso, vence en 24 h); por stderr, el avance y los avisos. Con `> install.txt` guardas solo lo que hay que conservar: respalda la clave fuera del servidor (sin ella no se pueden descifrar los secretos ni los backups) y borra el archivo. Si muelle no muestra el link en 2 min, install termina con código 1, deja el stack desplegado y sugiere `docker service logs muelle_muelle` y `docker service ps --no-trunc muelle_muelle`.

Tres cuidados:
- **Siempre con `--rm`:** sin él, el contenedor del instalador queda parado con la clave maestra en `docker logs`. Si lo corriste sin `--rm`, haz `docker rm <contenedor>` después de guardar la clave.
- **Sin `-t`:** con una TTY, Docker junta stderr y stdout, y `> install.txt` guardaría también el avance. Para el token o la contraseña por stdin alcanza con `-i`.
- **Varias IPs:** si el Swarm no está iniciado y la máquina tiene varias direcciones, `docker swarm init` no elige una. install falla con el error de Docker y sugiere `--advertise-addr <ip>`, la IP privada por la que se van a hablar los nodos.

**Sin `--cloudflare-token`,** el registro DNS del panel tiene que existir de antemano, y Let's Encrypt valida por HTTP-01, en el puerto 80 público. Con un `--http-port` distinto de 80, eso solo funciona si el 80 de tu router o firewall llega a ese puerto (install lo avisa).

**Junto a otro proxy que ya ocupa 80 y 443** (por ejemplo, mientras migras desde Dokploy): instala con otros puertos y el token de Cloudflare, porque DNS-01 no necesita el 80:

```bash
docker run -i --rm -v /var/run/docker.sock:/var/run/docker.sock \
  -v ~/.docker/config.json:/root/.docker/config.json:ro \
  ghcr.io/cpu-commits/muelle:<versión> install --panel-domain panel.tudominio.cl --email tu@correo.cl \
  --cloudflare-token - --http-port 8080 --https-port 8443 < cf-token.txt > install.txt
```

El panel queda en `https://panel.tudominio.cl:8443`. Cuando apagues el otro proxy, cambia los puertos del proxy a 80 y 443 en Ajustes: el panel pasa a `https://panel.tudominio.cl` con la misma sesión, sin tocar la configuración de muelle.

## Usarlo

1. Abre el link de setup y crea el primer usuario. Desde Ajustes → Usuarios puedes invitar a otros; todos tienen los mismos permisos.
2. En Ajustes, el token de Cloudflare y la IP pública (si no los pasaste a install), y, si quieres, exigir 2FA y limitar el panel a tus IPs.
3. Crea un proyecto, sus stages y una app: de formulario (imagen, puerto, healthcheck…) o compose.
4. Carga sus variables (o pega un `.env`), agrega sus dominios y despliega.
5. Para desplegar desde CI, crea un token del stage en Ajustes → Tokens y usa la [GitHub Action](#github-action).

**El healthcheck** es lo que da el deploy sin corte: Swarm lo corre **dentro** del contenedor, y no manda tráfico a la versión nueva hasta que pasa. En una app de formulario puede ser:
- `http`: `wget` contra `http://127.0.0.1:<puerto><ruta>` (la imagen tiene que traer `wget`);
- `bun-http`: lo mismo con `bun`;
- `command`: un comando libre;
- `none`: sin healthcheck (o el `HEALTHCHECK` de la imagen). Entonces no hay garantía de cero downtime.

**Un compose** se escribe como siempre, con algunos límites. muelle pone la red del stage, así que un compose no define las suyas ni publica `ports:`. Las imágenes vienen de un registry, sin `build:`. Los volúmenes son volúmenes nombrados, sin bind mounts. Los secretos llegan como variables, sin `secrets:` ni `configs:`. Un `${VAR}` se interpola solo con las variables del stage y de la app. Si el compose usa algo que no se permite, el panel lo dice al guardar.

## Actualizar

```bash
docker service update --with-registry-auth --image ghcr.io/cpu-commits/muelle:<versión> muelle_muelle
```

- `--with-registry-auth` le pasa a Swarm la credencial del `docker login ghcr.io` del servidor, para bajar la imagen nueva.
- muelle-caddy sigue a muelle: al arrancar, muelle lo recrea con la Caddy de la versión nueva (unos segundos de corte en los sitios), que baja con el registry `ghcr.io` guardado. Si fijaste la imagen al instalar (`--caddy-image`), Caddy no cambia.
- Si la versión nueva se cae al arrancar, Swarm vuelve sola a la anterior (`failure_action: rollback`, con `monitor: 30s`).
- **Si el token de GitHub vence:** actualízalo en Ajustes → Registries (lo usa muelle) y vuelve a hacer `docker login ghcr.io` en el servidor (lo usa `docker service update`).

## GitHub Action

`action/` es una composite action que guarda las variables de la app, crea el deploy y sigue su log hasta el resultado. Es bash con `curl` y `jq`, que vienen en los runners de GitHub.

```yaml
deploy:
  runs-on: ubuntu-latest
  environment: ${{ github.ref_name == 'main' && 'production' || 'staging' }}
  steps:
    - uses: CPU-commits/muelle/action@v0
      with:
        url: ${{ secrets.MUELLE_URL }}
        token: ${{ secrets.MUELLE_TOKEN }}
        app: tienda/${{ github.ref_name == 'main' && 'production' || 'staging' }}/api
        image: ghcr.io/tu-org/api:sha-${{ github.sha }}
        env-json: ${{ toJson(secrets) }}
        env-exclude: DEPLOY_KEY
```

| Entrada | Qué es |
|---|---|
| `url` | Obligatoria. El origen del panel, sin ruta: `https://panel.tudominio.cl` (con `:<puerto>` si no es 443). Solo `https://`, salvo `http://localhost` o `http://127.0.0.1`: el token nunca viaja en claro |
| `token` | Obligatoria. Un token de API (`mk_…`) con alcance a la app (abajo) |
| `app` | Obligatoria. `proyecto/stage/app` |
| `image` | La imagen de una app de formulario. Sin `image` ni `images-json`, muelle redespliega las imágenes actuales |
| `images-json` | Las imágenes de una app compose: un objeto JSON `servicio → imagen`, solo con los servicios que cambian. No va junto con `image` |
| `env-json` | Las variables de la app: un objeto JSON de strings; en la práctica, `${{ toJson(secrets) }}` |
| `env-exclude` | Claves de `env-json` que no se mandan, separadas por coma. Siempre se excluyen `GITHUB_TOKEN`, `MUELLE_URL` y `MUELLE_TOKEN`, sin distinguir mayúsculas |
| `env-secret` | `true` (por defecto) guarda las variables como secretas: el panel no vuelve a mostrar su valor y el log las enmascara. `false` las deja visibles |
| `timeout` | Segundos de espera, contados desde que se creó el deploy (600) |

**Salidas:** `deploy-id` y `status` (el estado final, o el último que vio si se pasó el timeout).

**Qué hace:**
1. Valida las entradas, antes de tocar muelle.
2. Si viene `env-json`, `PUT …/apps/<app>/env`, que **reemplaza todas las variables de la app**: una que no viene se borra (las del stage no se tocan). Reintenta hasta 3 veces si muelle no responde, o responde 5xx o 429.
3. `POST …/deploys`, una sola vez, porque no es idempotente. Si muelle no responde, falla sin reintentar: el deploy puede haberse creado, así que revísalo en el panel.
4. Imprime el log del deploy a medida que avanza y termina según el resultado:
   - `succeeded`: el paso sigue;
   - `failed`: un error con el motivo que dio muelle, y el paso falla;
   - `superseded` (un deploy más nuevo de la misma app lo reemplazó antes de que corriera): un aviso con el id del nuevo, y el paso sigue;
   - el timeout: un error y el paso falla, pero **el deploy sigue en muelle**.

Un error de muelle se muestra con su `code` y su `message` (`403 out_of_scope: …`).

**Cuidados:**
- **Un token por stage:** un token se limita a un proyecto, o a un proyecto y un stage, y solo puede leer, cambiar variables, desplegar y hacer rollback ahí. Guarda el de cada stage en el secreto `MUELLE_TOKEN` de su environment de GitHub: así el job de staging no puede desplegar production. Un token de solo lectura no sirve para la action (`403 read_only_token`).
- **El tamaño de `env-json`:** llega al script como una variable de entorno del paso, y Linux no deja pasar una de más de 128 KiB (el paso no arrancaría). Con `toJson(secrets)` van todos los secretos del repo y del environment juntos.
- **Lo que imprime la app** no se lee como comandos del runner (`::error::`, `::add-mask::`…): el log va entre `::stop-commands::` y su cierre. Los secretos ya vienen enmascarados por muelle, y GitHub tapa además los `secrets.*`.
- **El token** nunca va en la línea de comandos de un proceso del runner: `curl` lo lee de un archivo `0600` que se borra al terminar, y `jq` lee `env-json` del entorno.
- **La versión:** `@v0` sigue al último release de la serie 0; `@vX.Y.Z` queda fija.

## Claude Code (MCP)

muelle sirve un servidor MCP en `https://panel.tudominio.cl/api/v1/mcp` (el origen del panel, con `:<puerto>` si no es 443). Con él, Claude Code lee el estado de las apps, sus deploys, sus logs, sus variables (sin los secretos) y sus dominios. Con un token que no es de solo lectura, además despliega, hace rollback y cambia variables no secretas. No hay nada que instalar: es HTTP, con un token de API.

**El token**, en Ajustes → Tokens de API:
- para depurar, uno **de solo lectura**: de un proyecto, de un stage, o "Todos los proyectos (solo lectura)", que lee todo muelle (el panel le pone un vencimiento de 90 días, que puedes cambiar);
- para que Claude Code despliegue, uno de escritura **de un stage** (`tienda/staging`), nunca de production. No existe un token de escritura de todos los proyectos.

**Agregarlo.** El panel muestra las dos formas al crear el token, con la URL de tu panel. Con el comando:

```bash
claude mcp add --transport http --scope user muelle https://panel.tudominio.cl/api/v1/mcp --header "Authorization: Bearer mk_…"
```

`--scope user` lo deja para todos tus proyectos. Claude Code guarda el token en claro en `~/.claude.json`, y el comando queda en el historial de la shell. Para compartirlo con tu equipo sin el token, usa un `.mcp.json` en la raíz del repositorio:

```json
{
  "mcpServers": {
    "muelle": {
      "type": "http",
      "url": "https://panel.tudominio.cl/api/v1/mcp",
      "headers": { "Authorization": "Bearer ${MUELLE_TOKEN}" }
    }
  }
}
```

Cada persona exporta el suyo antes de abrir Claude Code (`export MUELLE_TOKEN=mk_…`), y Claude Code pide aprobar el servidor de un `.mcp.json` la primera vez. Para no tener el token ni en un archivo ni en el entorno, sirve `headersHelper` (un comando que imprime los headers; ver la documentación de MCP de Claude Code). Con `/mcp`, en Claude Code, se ve si conectó y qué herramientas tiene.

**Herramientas:**

| Herramienta | Qué hace |
|---|---|
| `list_projects`, `list_stages`, `list_apps` | Lo que puede ver el token |
| `get_app` | La configuración de una app: el spec, los servicios y, si se pide, el compose |
| `get_env` | Las variables del stage y de la app. Los valores secretos nunca salen |
| `get_app_status` | Las réplicas, las tareas de cada servicio con su error, la CPU y la memoria |
| `get_app_logs` | Los logs de un servicio, sin follow: `tail`, `since` (`15m`, o una fecha), `contains` y `stream` |
| `list_deploys`, `get_deploy` | El historial, y un deploy con el final de su log. `wait_s` espera a que termine |
| `read_deploy_log` | El log completo de un deploy, por partes |
| `list_domains` | Los dominios de la app, con el estado de su DNS y de su certificado (de Let's Encrypt o de Cloudflare, y por qué no salió el de Cloudflare) |
| `deploy` (escribe) | Despliega, con `image` o `images` opcionales, como la API |
| `rollback` (escribe) | Vuelve al deploy anterior |
| `set_env` (escribe) | Guarda variables **no secretas**: no pisa una secreta, ni la tapa con una de la app. Se aplican en el próximo deploy |
| `unset_env` (escribe) | Borra variables |

Las cuatro que escriben solo están con un token que no es de solo lectura.

**Permisos:** para que las lecturas no pregunten, `/permissions` → Allow → `mcp__muelle` (o, en `settings.json`, `"permissions": {"allow": ["mcp__muelle"]}`). Las que escriben piden confirmación **siempre**, en cada llamada y en cualquier modo, aunque haya una regla `allow`: muelle las marca con `anthropic/requiresUserInteraction`.

**Un ejemplo:** "¿por qué falla el deploy de api en staging?". Claude Code sigue las instrucciones del servidor: `list_deploys` (el último `failed`), `get_deploy` (su error y el final del log), `get_app_status` (las tareas y su error) y `get_app_logs` (los logs del servicio).

**Cuidados:**
- **Los logs son texto de las apps**, y lo puede escribir cualquiera que les mande pedidos: Claude puede leer instrucciones plantadas ahí. muelle lo marca en cada respuesta, pero revisa lo que apruebas, sobre todo la `image` de un `deploy`.
- **Los secretos** no se muestran ni se escriben por MCP. Los logs salen enmascarados, con los límites de siempre: un valor de menos de 8 bytes, o uno que la app imprime transformado (en base64, por partes), no se tapa.
- **Lo que devuelven las herramientas queda en la transcripción** de Claude Code, en tu máquina, y pasa por la API de Anthropic.
- **La auditoría** marca lo que se hizo por MCP con `"via": "mcp"` y el nombre del token. Para revocarlo, Ajustes → Tokens.
- **Límites:** cada respuesta llega hasta 40 KiB (no hace falta subir `MAX_MCP_OUTPUT_TOKENS`). Si no entra todo, la herramienta recorta lo más viejo (las líneas, los deploys, las tareas históricas) y dice cuánto faltó, en vez de fallar. Hasta 8 llamadas a la vez por token; los logs y la espera de un deploy cuentan en los 8 streams del token.

**Si no conecta:**
- `401`: el token no existe, se revocó o venció, o `MUELLE_TOKEN` no está exportada en la shell en que abriste Claude Code;
- `403` sin cuerpo: tu IP no está en las IPs permitidas del panel (Ajustes). Suma la de tu oficina o tu VPN;
- `403 bad_origin`: el cliente manda un `Origin` que no es el del panel (Claude Code no lo manda);
- si Claude Code no logra negociar el protocolo: `MCP_PROTOCOL_NEGOTIATION=legacy claude`. muelle habla 2026-07-28 (con `server/discover`, sin sesiones) y, con `initialize`, 2025-11-25 y los anteriores.

Para probarlo sin Claude Code, ver el `curl` del [contrato de la API](docs/superpowers/specs/2026-10-10-contrato-http-fases-2-3.md) (§8.2).

## Proxy y dominios

El job `"proxy"` de muelle, al arrancar y cada 5 minutos, asegura el contenedor muelle-caddy y sus redes y le reenvía la configuración. Si una escritura a Caddy falla, el panel muestra "proxy desincronizado" y muelle reintenta. Caddy valida cada configuración antes de aplicarla y, si falla, conserva la anterior.

- **Cambiar los puertos del proxy** en Ajustes recrea muelle-caddy 2 s después de guardar. Antes, muelle prueba que los puertos nuevos estén libres en el host (uno ocupado es un `422` y no cambia nada); si igual muelle-caddy no arranca con ellos, muelle vuelve a los anteriores y lo dice en `GET /api/v1/proxy` y en la auditoría.
- **El token de Cloudflare** (Ajustes) necesita los permisos Zone:Read y DNS:Edit sobre las zonas de tus dominios y, si activas los certificados de Cloudflare, también Zone → SSL and Certificates → Edit. El usuario dueño del token tiene que tener API Access. muelle lo verifica al guardarlo. Los registros que crea llevan el comentario `muelle`.
- **La IP real del cliente:** Caddy confía en los rangos de Cloudflare y lee `CF-Connecting-IP`, así que con el proxy de Cloudflare encendido, la lista de IPs permitidas y los límites por IP ven la IP del cliente, no la de Cloudflare.
- **Una `caddy_image` propia** (`--caddy-image`) tiene que ser Caddy 2.10 o más nuevo, con el módulo `dns.providers.cloudflare`. La imagen `muelle-caddy` de cada versión cumple las dos cosas.
- **En desarrollo** (un `MUELLE_PANEL_ORIGIN` con una IP o `localhost`, sin dominio del panel), muelle no crea muelle-caddy ni toca los puertos del host, y `GET /api/v1/proxy` dice `disabled`. Los dominios y su DNS en Cloudflare funcionan igual.

**Certificados de Cloudflare (Origin CA)**, en Ajustes → Cloudflare, apagados por defecto:
- Son opcionales y sirven para el modo SSL **Full (strict)** de Cloudflare. muelle no cambia el modo SSL de la zona: con "Flexible" hay un bucle de redirects.
- **Solo Cloudflare confía en ellos, los navegadores no.**
- Los usan los dominios de Cloudflare con el proxy (nube naranja) cuyo registro maneja muelle, y el del panel si cae en una de tus zonas y su registro tiene el proxy. muelle lee el `proxied` de los registros cada 5 minutos.
- Los dominios en DNS only y los que están fuera de Cloudflare siguen con Let's Encrypt. Un dominio que pierde el proxy, o todos al apagar la opción, vuelven a Let's Encrypt.
- Duran 90 días, y muelle los renueva cuando faltan 30. Si no puede renovar uno, cuando le faltan 14 días vuelve a Let's Encrypt.
- muelle revoca cada uno cuando deja de usarse, después de que Caddy cargó una configuración sin él.
- Sus claves quedan cifradas en la base, y en claro en el autosave de Caddy (volumen `muelle_caddy_config`), como el token de Cloudflare.
- Con el panel en Origin CA hay que entrar por Cloudflare: directo al servidor (por ejemplo con `/etc/hosts`), el navegador no confía en el certificado. Para volver a Let's Encrypt, apaga el proxy del registro del panel en Cloudflare; el cambio se aplica en a lo más 5 minutos.
- Al apagarlos, Cloudflare puede responder `525` un momento, mientras Caddy consigue el de Let's Encrypt.

## Operación

### Después de instalar

**Leer la clave maestra** (si no la guardaste; también vive en el secreto Swarm):

```bash
docker exec "$(docker ps -q -f label=com.docker.swarm.service.name=muelle_muelle | head -n1)" cat /run/secrets/muelle_master_key
```

**Quitar el token y la credencial de install del servicio** (opcional). muelle los guardó en su base en el primer arranque y ya no los usa como ajustes, pero los relee en cada arranque. Cada secreto se quita junto con su variable, en un solo `docker service update` (reinicia muelle):

```bash
docker service update --secret-rm muelle_init_cloudflare_token --env-rm MUELLE_INIT_CLOUDFLARE_TOKEN_FILE muelle_muelle
docker secret rm muelle_init_cloudflare_token
docker service update --secret-rm muelle_init_registry --env-rm MUELLE_INIT_REGISTRY_FILE muelle_muelle
docker secret rm muelle_init_registry
```

**El origen del panel** sale de los ajustes: `https://<dominio del panel>`, con `:<puerto HTTPS del proxy>` si no es 443. install guarda solo el dominio (`MUELLE_PANEL_DOMAIN`), así que al cambiar los puertos en Ajustes, el panel, sus links de invitación y el `Origin` que acepta muelle pasan al puerto nuevo, y el viejo deja de servir.

**Detrás de otro proxy** (el panel en un origen que muelle no conoce): agrega ese origen. Se acepta además del de los ajustes y pasa a ser el de los links (reinicia muelle, no los sitios):

```bash
docker service update --env-add MUELLE_PANEL_ORIGIN=https://panel.otro-proxy.cl muelle_muelle
```

### Solo detrás de Caddy

**muelle nunca debe quedar expuesto directo.** Para limitar los logins por IP usa la IP del cliente, y confía en el header `X-Real-IP` cuando la conexión viene de loopback o de una red privada (RFC 1918, ULA), que es por donde llega Caddy. Si un cliente puede conectarse a muelle sin pasar por el proxy desde una de esas redes, puede escribir su propio `X-Real-IP` y saltarse el límite por IP.

- El `MUELLE_LISTEN=:8080` por defecto escucha en todas las interfaces: **no publiques ese puerto** (ni con `-p`/`ports:` de Docker ni en el firewall del host). En producción, muelle está en la red `muelle_panel`, a la que solo llega Caddy.
- Si pones otro proxy delante, tiene que **pisar** `X-Real-IP` con la IP real del cliente, nunca reenviar la que mande el cliente. En un Caddyfile es `header_up X-Real-IP {client_ip}`.

### Recuperar el acceso

Si la lista de IPs permitidas del panel (Ajustes) te deja fuera, por ejemplo porque cambió tu IP, vacíala desde el servidor, dentro del contenedor de muelle:

```bash
docker exec "$(docker ps -q -f label=com.docker.swarm.service.name=muelle_muelle | head -n1)" muelle settings reset-panel-ips
```

- Deja la lista vacía (cualquier IP) y reinicia muelle (`docker service update --force muelle_muelle`): al arrancar, muelle le manda a Caddy la configuración sin la lista. El panel y los streams se cortan unos segundos, y un deploy en curso se resuelve con la reconciliación del arranque.
- El cambio queda en la auditoría como `settings.update`, con actor `system` y `"via": "cli"`. No necesita la clave maestra.
- Si la lista ya estaba vacía, no cambia nada y lo dice (el problema es otro: mira los puertos del proxy, abajo), pero reinicia igual, y ese arranque le reenvía la configuración a Caddy.
- `--no-restart` solo cambia la base e imprime el comando para reiniciar a mano. Mientras tanto, Caddy sigue filtrando con la lista anterior, hasta que muelle le reenvíe la configuración (el job `"proxy"`, cada 5 min).
- Si no puede reiniciar (Docker no responde, o el contenedor no es una tarea de Swarm), lo dice, imprime el comando para reiniciar a mano y termina con código 1: la base ya quedó cambiada.

**Si el problema son los puertos del proxy.** Si muelle-caddy no arranca en los puertos nuevos ni vuelve solo a los anteriores, sin Caddy no se llega al panel. Corrígelos directo en la base. La imagen de muelle no trae `sqlite3`, así que se usa un contenedor aparte sobre el mismo volumen (SQLite en modo WAL admite la escritura con muelle corriendo). Cambia `80` y `443` por los anteriores u otros libres:

```bash
docker run --rm -v muelle_data:/data alpine sh -c \
  "apk add -q sqlite && sqlite3 /data/muelle.db \"UPDATE settings SET value='80' WHERE key='proxy_http_port'; UPDATE settings SET value='443' WHERE key='proxy_https_port'\""
```

Después reinicia muelle desde el host (`docker service update --force muelle_muelle`), y al arrancar recrea muelle-caddy con esos puertos.

### Reinstalar y empezar de cero

**Reinstalar** (volver a desplegar muelle con la misma base y la misma clave, por ejemplo después de borrar el stack), en este orden:
1. `docker stack rm muelle`;
2. esperar a que `docker ps -q -f label=com.docker.swarm.service.name=muelle_muelle` quede vacío: `docker stack rm` vuelve antes de que el contenedor termine, y un install en ese momento tendría dos muelle sobre la misma base;
3. el mismo `docker run … install`, con `--reinstall`. Reusa el secreto `muelle_master_key` (no imprime una clave) y, si muelle ya tiene usuarios, lo dice en vez de mostrar un link. Los flags no cambian los ajustes de una base que ya arrancó: esos se cambian en el panel.

**Empezar de cero** (**se pierden los datos:** la base, sus backups y los certificados de Caddy). Después de los pasos 1 y 2 de "Reinstalar":

```bash
docker rm -f muelle-caddy        # es un contenedor aparte: docker stack rm lo deja con los puertos tomados
docker volume rm muelle_data muelle_caddy_data muelle_caddy_config muelle_caddy_admin
docker network rm muelle_panel
docker secret rm muelle_master_key muelle_init_cloudflare_token muelle_init_registry   # los que existan
```

Los stacks y las redes de las apps (`docker stack ls`, `docker network ls --filter label=muelle.project`) no se tocan: bórralos aparte si también los quieres sacar.

## Desarrollo

```
main.go              subcomandos serve, install, genkey, settings y version
Dockerfile           la imagen de muelle; deploy/caddy/, la de muelle-caddy
action/              la GitHub Action de deploy
web/                 el panel: SvelteKit sin SSR, que el binario embebe con -tags panel
src/
├── cmd/http/        ServeMux, middleware y errores → HTTP; panel/ sirve el panel embebido
├── cmd/cron/        las tareas periódicas de los módulos
├── cmd/mcp/         el servidor MCP (POST /api/v1/mcp) y el kit con que los módulos registran sus herramientas
├── cmd/install/     muelle install
├── cmd/settingscli/ muelle settings reset-panel-ips
├── internal/
│   ├── identity/    usuarios, sesiones, 2FA, invitaciones y tokens de API
│   ├── catalog/     proyectos, stages y apps
│   ├── secrets/     variables cifradas de stage y app
│   ├── admin/       ajustes, auditoría, backup y limpieza periódica
│   ├── deploy/      deploys, registries, estado y logs de las apps
│   └── ingress/     redes de stage, dominios, Cloudflare y muelle-caddy
├── shared/          dominio común, nombres de Docker, el cliente de Docker y container.New, que arma todo
├── package/         interfaces finas: cipher, clock, audit
└── settings/        carga y validación de MUELLE_*
```

Go con `net/http` de la biblioteca estándar y SQLite en Go puro (sin CGO), con arquitectura hexagonal. Cada módulo tiene `domain/`, `app/` e `infra/` (in/out), y los módulos no se importan entre sí: se hablan por puertos que arma `container.New`. Comentarios y mensajes de error en español; identificadores en inglés.

### Correrlo en local

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

Al arrancar sin usuarios, muelle imprime en el log un link de setup de un solo uso (vence en 24 h):

```
{"level":"INFO","msg":"todavía no hay usuarios: abre este link para crear el primero (vence en 24 h)","event":"setup_url","url":"http://127.0.0.1:18080/setup?token=<TOKEN>"}
```

| Variable | Por defecto | Qué es |
|---|---|---|
| `MUELLE_PANEL_DOMAIN` | — | Dominio del panel, sin esquema ni puerto (lo escribe install). El origen del panel sale de él y del puerto HTTPS de los ajustes: es el `Origin` que se acepta (CSRF) y el de los links de setup e invitaciones |
| `MUELLE_PANEL_ORIGIN` | (obligatoria sin `MUELLE_PANEL_DOMAIN`) | Un origen exacto más, en forma canónica (con el puerto si no es el por defecto): en desarrollo, o detrás de otro proxy. Si está, también arma los links de setup e invitaciones |
| `MUELLE_LISTEN` | `:8080` | Dirección HTTP. No publiques este puerto (ver "Solo detrás de Caddy") |
| `MUELLE_DATA_DIR` | `/data` | Carpeta de `muelle.db` y `backups/` |
| `MUELLE_MASTER_KEY_FILE` | `/run/secrets/muelle_master_key` | Clave maestra en base64 (32 bytes), con permisos 0600 |
| `MUELLE_SECURE_COOKIES` | `true` | `false` solo en desarrollo por http; con un `MUELLE_PANEL_ORIGIN` https, o con `MUELLE_PANEL_DOMAIN`, muelle no arranca |

**El panel.** Compilado sin `-tags panel` (`go run . serve`, `go build`), muelle no trae el panel: toda ruta fuera de `/api` responde `404`, y la API funciona igual. Para probar el panel hay dos caminos:

- **Embebido, como en la imagen** (Node 24 y pnpm): construir el panel, copiar el build en `src/cmd/http/panel/build/` (ignorado por git) y compilar con el tag. Después de cada `pnpm build` hay que volver a copiarlo, porque el hash de la CSP cambia en cada build.

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
- **`pnpm dev` en `web/`:** Vite proxea `/api` a un `muelle serve` local, con `MUELLE_PANEL_ORIGIN=http://localhost:5173`. Sin backend, `pnpm dev:mock` usa MSW. Ver [`web/README.md`](web/README.md).

**Docker.** Crear un stage crea su red overlay, y desplegar o borrar una app usa `docker stack deploy` y `docker stack rm`. Para eso, el Docker local tiene que ser manager de un Swarm (`docker swarm init`) y el CLI `docker` tiene que estar en el `PATH`; si no, esas peticiones responden `503 docker_unavailable` y no cambian nada. El resto de la API funciona sin Docker.

**Con `curl`.** Toda petición que cambia algo con la cookie de sesión tiene que traer el header `Origin` del panel (si no, `403 bad_origin`); los tokens de API (`Authorization: Bearer mk_…`) no lo necesitan:

```bash
O=http://127.0.0.1:18080
curl -c cookies -H "Origin: $O" -H 'Content-Type: application/json' \
  -d '{"token":"<TOKEN>","username":"admin","password":"una-contraseña-larga"}' \
  $O/api/v1/auth/setup                      # 201; la sesión queda en ./cookies
curl -b cookies -H "Origin: $O" -H 'Content-Type: application/json' \
  -d '{"slug":"tienda","name":"Tienda"}' $O/api/v1/projects
curl -b cookies -H "Origin: $O" -H 'Content-Type: application/json' \
  -d '{"slug":"staging"}' $O/api/v1/projects/tienda/stages          # crea la red tienda_staging
curl -b cookies -H "Origin: $O" -H 'Content-Type: application/json' \
  -d '{"slug":"web","kind":"service","spec":{"image":"nginx:alpine","port":80,"healthcheck":{"type":"http","path":"/"}}}' \
  $O/api/v1/projects/tienda/stages/staging/apps
curl -b cookies -H "Origin: $O" -H 'Content-Type: application/json' \
  -d '{"name":"ci","project":"tienda"}' $O/api/v1/tokens   # el token en claro se muestra una sola vez

T=mk_…
A=$O/api/v1/projects/tienda/stages/staging/apps/web
curl -H "Authorization: Bearer $T" -X POST $A/deploys               # 202 {"id":1,"status":"queued",…}
curl -N -H "Authorization: Bearer $T" $A/deploys/1/log/stream       # el log en vivo (SSE)
curl -H "Authorization: Bearer $T" $A/status                        # servicios, tareas, CPU y memoria
curl -H "Authorization: Bearer $T" -X POST $A/rollback              # 409 no_previous_deploy hasta el segundo deploy
```

La API completa, con sus errores, está en el [contrato de la API](docs/superpowers/specs/2026-10-10-contrato-http-fases-2-3.md).

### Tests

```bash
gofmt -l $(git ls-files '*.go')   # no debe imprimir nada
go mod tidy -diff
go vet ./...
go test -race ./...
```

- **De integración** (build tag `integration`): usan el Docker real y necesitan un Swarm activo en el que este nodo es manager; si no lo hay, se saltan. Cada paquete crea solo recursos con su prefijo (`it2`, `it3`…), en puertos altos, y borra lo suyo; nunca usan 80, 443 ni la API real de Cloudflare. Los que usan el Swarm corren de a uno aunque `go test` los lance a la vez.

  ```bash
  go test -tags integration -race -count=1 -timeout 40m ./...
  ```

  `TestIntegrationEndToEnd` (en `src/shared/infra/container`) recorre muelle completo por la API contra el Swarm: un deploy seguido por SSE, el estado y los logs, un dominio que muelle-caddy enruta a la app, un redeploy y el rollback. `TestIntegrationMCP` hace lo mismo con el servidor MCP.
- **La GitHub Action:** los tests de `action/` corren `deploy.sh` con bash, `curl` y `jq` contra un muelle falso; CI corre además `shellcheck action/deploy.sh`.
- **El panel** (`web/`): `pnpm verify` (lint, svelte-check y vitest), `pnpm build` y `pnpm test:e2e` (Playwright: cada pantalla bajo la CSP). `pnpm test:e2e:real` construye muelle con el panel y recorre, contra un `muelle serve` de verdad y el Swarm local, el camino crítico: entrar, crear una app, desplegarla hasta que corre y hacer rollback.

Los e2e y las pruebas de punta a punta cubren solo lo que, si se rompe, deja a todos sin muelle: una feature nueva se prueba en su paquete o su módulo, no les suma pasos (ver "Tests" en [`web/CLAUDE.md`](web/CLAUDE.md)).

### Imágenes

Dos imágenes, solo para `linux/amd64`:

| Imagen | Qué trae |
|---|---|
| `ghcr.io/cpu-commits/muelle` | El binario estático (sin CGO) con el panel embebido y su versión (`muelle version`), el CLI de `docker` 29.9.0 para `docker stack deploy` y `ca-certificates`, sobre Alpine. Corre como root, porque usa `docker.sock`. `ENTRYPOINT ["muelle"]` y `CMD ["serve"]`, sin `EXPOSE`: el puerto de muelle nunca se publica |
| `ghcr.io/cpu-commits/muelle-caddy` | Caddy 2.11.7 con el módulo de DNS de Cloudflare (`dns.providers.cloudflare`, compilado con `xcaddy`). No trae configuración: muelle la arranca con `caddy run --resume` y la admin API en un socket |

Las imágenes base, el frontend de BuildKit y el módulo de Cloudflare van con versión exacta y, las imágenes, con su digest. La imagen de muelle construye el panel dentro (con `VITE_USE_MOCKS=0`), y su build falla si al panel le falta el `200.html` o la CSP, o si trae el service worker de MSW. El `.dockerignore` es una lista de permitidos: al contexto solo entran `go.mod`, `go.sum`, `main.go`, `src/` y `web/`.

Para construirlas en local, desde la raíz del repo:

```bash
docker build --build-arg VERSION=v0.1.0 -t ghcr.io/cpu-commits/muelle:v0.1.0 .
docker build --build-arg VERSION=v0.1.0 -t ghcr.io/cpu-commits/muelle-caddy:v0.1.0 deploy/caddy
docker run --rm ghcr.io/cpu-commits/muelle:v0.1.0 version            # v0.1.0
```

### Publicar una versión

Desde `main` con CI en verde:

1. Crear y subir el tag (con sufijo, como `v0.2.0-rc.1`, es una prerelease):

   ```bash
   git tag -a vX.Y.Z -m "muelle vX.Y.Z"
   git push origin vX.Y.Z
   ```
2. `release.yml` corre los mismos tests que CI y después construye y sube `ghcr.io/cpu-commits/muelle:vX.Y.Z` y `ghcr.io/cpu-commits/muelle-caddy:vX.Y.Z`. `:latest` (solo muelle) se mueve solo con las versiones finales. Los paquetes quedan privados.
3. Para que el release llegue a los repos que usan la action con `@v0`, mueve el tag mayor (nunca a una prerelease):

   ```bash
   git tag -f v0 'vX.Y.Z^{}'          # el commit del release
   git push -f origin v0
   ```

Para probar los `Dockerfile` sin publicar nada: Actions → Release → Run workflow construye las dos imágenes sin subirlas.

## Documentación

- [Diseño](docs/superpowers/specs/2026-10-09-muelle-design.md): las decisiones, la arquitectura, el modelo de datos, el render del compose, el flujo de deploy, el proxy, el DNS y la seguridad.
- [Contrato de la API](docs/superpowers/specs/2026-10-10-contrato-http-fases-2-3.md): cada endpoint, sus errores, los puertos entre módulos y la configuración de install.
- [Servidor MCP](docs/superpowers/specs/2026-10-21-mcp-design.md) y [certificados de Cloudflare (Origin CA)](docs/superpowers/specs/2026-10-21-origin-ca-design.md).
- [El panel](web/README.md).

## Licencia

[MIT](LICENSE)
