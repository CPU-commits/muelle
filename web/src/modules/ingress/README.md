# Módulo `ingress` (Fase 3): el contrato que usa el panel

La fuente es el contrato HTTP de las Fases 2 y 3:
`docs/superpowers/specs/2026-10-10-contrato-http-fases-2-3.md` (§0 y §3, con los
certificados de Cloudflare de §3.7), en este mismo árbol, con la Fase 3 terminada.
Este archivo resume lo que la pantalla Dominios lee y manda, y cómo se comporta con cada
respuesta. Si el contrato cambia, se cambian este README, `data/dtos/domainDto.ts` y los
mocks (`src/mocks/{db,handlers}/ingress.ts`).

## Convenciones

- Base `/api/v1`, JSON en snake_case, fechas RFC 3339 en UTC.
- Errores: `{"error": {"code": "…", "message": "…"}}`; el panel decide por el `code`.
- **Solo con sesión**: un token de API recibe `403 token_not_allowed`.
- `{app}` es `/projects/{p}/stages/{s}/apps/{a}`. `{hostname}` va url-encoded y tiene que
  ser de esa app: si no, `404 not_found`, aunque exista en otra.
- Crear, editar y borrar aplican al instante el DNS y la ruta de Caddy, sin deploy. Un fallo
  de Caddy no hace fallar la petición (se ve en `GET /proxy`).

## Errores

| Respuesta                                | Qué ve el panel                                                                                                                                                               |
| ---------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `404 no_route`, `405 method_not_allowed` | Una ruta mal armada del panel: `UnexpectedError` (el panel va con el backend entero)                                                                                          |
| `404 not_found` (un dominio)             | `DomainNotFound`. Al borrar: otra persona ya lo borró, se cierra el diálogo y se pide la lista                                                                                |
| `409 conflict` (crear)                   | `HostnameTaken`: "Ese hostname ya está en uso en muelle", en el campo                                                                                                         |
| `409 nothing_to_adopt`                   | `NothingToAdopt`: "No hay un registro que adoptar"                                                                                                                            |
| `422 invalid`                            | En el campo: `hostname`, o `target_service` / `target_port` (el campo "destino"), o `proxied`                                                                                 |
| `502 cloudflare_unavailable`             | `CloudflareUnavailable` (en el diálogo de borrar): Cloudflare no respondió o rechazó el borrado, no se borró nada; para borrarlo solo en muelle, se quita el token en Ajustes |
| `503 docker_unavailable`                 | `DockerUnavailable`: "Docker no responde en el servidor de muelle."                                                                                                           |

## El dominio

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
		{ "type": "A", "name": "api.tudominio.cl", "content": "203.0.113.24" },
		{ "type": "AAAA", "name": "api.tudominio.cl", "content": "2001:db8::24" }
	],
	"created_at": "2026-10-01T12:00:00Z",
	"updated_at": "2026-10-01T12:00:00Z"
}
```

| Campo                              | Qué                                                                                                                                                                                                                                                                          |
| ---------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `hostname`                         | Único en todo muelle, en forma canónica (minúsculas, sin punto final, sin comodines)                                                                                                                                                                                         |
| `target_service`, `target_port`    | El servicio (nombre corto) y el puerto al que va el tráfico                                                                                                                                                                                                                  |
| `proxied`                          | Detrás del proxy de Cloudflare. Solo tiene efecto si muelle maneja el registro: fuera de toda zona, o sin token, queda `false` aunque el `POST` mande `true`                                                                                                                 |
| `tls_mode`                         | `cloudflare` (DNS-01) · `http` (HTTP-01, fuera de Cloudflare)                                                                                                                                                                                                                |
| `cf_zone_id?`                      | La zona; null si no cae en ninguna o no se pudo consultar                                                                                                                                                                                                                    |
| `cf_record_id?`                    | El registro `A` que maneja muelle; **null si no lo maneja**. El del `AAAA` no se expone                                                                                                                                                                                      |
| `status`                           | `pending` · `active` · `error`                                                                                                                                                                                                                                               |
| `last_error_code?`                 | Solo con `error`, y **siempre** con `error` (la migración 0003 lo garantiza): `record_exists` → Adoptar; `cloudflare` o `certificate` → Reintentar. El panel decide **solo por el código**, nunca por el texto. Uno desconocido es un error cualquiera                       |
| `last_error?`                      | El mensaje en español, que el panel muestra                                                                                                                                                                                                                                  |
| `cert_issuer?`, `cert_expires_at?` | Lo último que vio la verificación en un certificado **válido** (la organización del emisor, o su CN): "Let's Encrypt · vence 8 ene 2027"; sin los dos, "Certificado pendiente"                                                                                               |
| `cert_source`                      | **Calculado**: qué certificado le pide muelle a Caddy ahora. `letsencrypt` (ACME, DNS-01 o HTTP-01) · `cloudflare_origin_ca` (uno de Cloudflare Origin CA que emitió muelle, §3.7). Es `cloudflare_origin_ca` si y solo si `origin_ca.status` es `active`. Obligatorio       |
| `origin_ca`                        | **Calculado**: el certificado de Cloudflare Origin CA del dominio (abajo), o null con la opción apagada en Ajustes, sin token o con el proxy deshabilitado. Obligatorio (la clave; el valor puede ser null)                                                                  |
| `records`                          | **Calculados con los ajustes actuales** (no se guardan): un `A` con la IP pública y un `AAAA` con la IPv6, si las hay; `[]` sin IP pública. `name` es el hostname. Con `cf_record_id` son los que maneja muelle; sin zona, los que hay que crear a mano (tabla con "copiar") |
| `created_at`, `updated_at`         | `updated_at` cambia con cada cambio del dominio (crearlo, un `PATCH` que cambia algo, adoptarlo, un reintento que rehace el DNS y cada cambio de `status`); una verificación que solo renueva el certificado no lo mueve                                                     |

### Estados

- **Crear**, con token y el hostname en una zona: se crean `A` (y `AAAA`) con el `proxied`
  pedido y queda `pending`. Si ya hay un registro de ese nombre que muelle no creó (un `A`,
  un `AAAA` o un `CNAME`): `error` + `record_exists`, sin `cf_record_id` pero **con**
  `cf_zone_id` y `tls_mode: cloudflare` (el certificado sale por DNS-01 igual). Si
  Cloudflare falla, o no hay IP pública: `error` + `cloudflare` (el `POST` igual responde
  `201`).
- **Sin token o sin zona**: `tls_mode: http`, `pending`, `proxied: false`; el registro se
  crea a mano.
- **`pending` → `active`**: cuando la verificación ve un certificado válido que vence en más
  de 14 días. **Corre 30 s después de crear, adoptar o reintentar; cada minuto mientras
  sigue `pending`; y cada hora para todos.**
- **Un `pending` sin certificado válido 10 min después** de su último cambio (`updated_at`)
  pasa a `error` + `certificate`. **`active` → `error`** + `certificate` si el certificado es
  inválido o vence en menos de 14 días; **`error` + `certificate` → `active`** cuando la
  verificación de cada hora ve uno válido.
- Si muelle no llega a Caddy, o el proxy no está `synced`, la verificación no cambia nada.

| Estado del panel      | Cuándo                                           | Acción                                                          |
| --------------------- | ------------------------------------------------ | --------------------------------------------------------------- |
| Válido                | `active`                                         | Proxied (si `cf_record_id`), Editar, Reintentar en el menú      |
| Emitiendo certificado | `pending` y muelle lo resuelve (no es manual)    | La lista se refresca cada 5 s mientras haya uno                 |
| Esperando DNS         | `pending` fuera de Cloudflare (`tls_mode: http`) | **Verificar** (= retry: verifica ahora) y la tabla de `records` |
| Registro existente    | `error` + `record_exists`, sin `cf_record_id`    | **Adoptar registro**; "Volver a mirar el DNS" en el menú        |
| Error                 | otro `error`                                     | `last_error` y **Reintentar**                                   |

Un dominio "Esperando DNS" se refresca cada 30 s: muelle lo verifica cada minuto mientras
siga `pending`, así que pasa solo a `active` (o a `error` + `certificate` a los 10 min).
**Verificar** lo mira ahora.

### Certificados de Cloudflare (Origin CA, contrato §3.7)

Opcionales, apagados por defecto (`cloudflare_origin_ca` en Ajustes). Con la opción
encendida y un token con **Zone → SSL and Certificates → Edit**, cada dominio de Cloudflare
con el proxy (nube naranja), también el del panel, usa un certificado de Cloudflare Origin
CA en vez de Let's Encrypt (para el modo SSL Full (strict)). **Solo Cloudflare confía en
él, los navegadores no.** Los de DNS only y los de fuera de Cloudflare siguen con Let's
Encrypt. Lo emite y lo renueva muelle, en segundo plano.

`origin_ca` trae siempre sus cinco claves:

| Campo        | Qué                                                                                                                                                                                       |
| ------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `status`     | `active` · `issuing` · `failed` · `dns_only` · `not_eligible` (abajo). Uno desconocido es un `ContractError`                                                                              |
| `error_code` | `permission` · `cloudflare` · `rejected`, solo con `failed` y con `active` (una renovación que falló). Uno que el panel no conoce se lee como null y se muestra como una falla cualquiera |
| `error`      | El mensaje en español, que el panel muestra tal cual (como oración)                                                                                                                       |
| `retry_at`   | Cuándo se reintenta; después de un Reintentar, la hora del pedido (en el pasado) hasta que termina la pasada                                                                              |
| `expires_at` | El vencimiento del que guardó muelle, se sirva o no; con `active`, nunca null                                                                                                             |

| `status`           | Cuándo                                                                                          | Qué muestra Dominios (`originCaNotice`)                                                                                                   |
| ------------------ | ----------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| `not_eligible`     | `tls_mode: http`, sin zona o sin `cf_record_id`; o en Cloudflare no hay registros de ese nombre | Nada                                                                                                                                      |
| `dns_only`         | `proxied: false` en muelle, o la nube gris en Cloudflare                                        | La pista: "DNS only: usa Let's Encrypt…", o, con `proxied: true` en muelle, "En Cloudflare el registro está en DNS only (nube gris)…"     |
| `active`           | Caddy sirve el de Origin CA                                                                     | "Cloudflare Origin CA · vence {expires_at}" ("pronto" a menos de 30 días), con "Solo Cloudflare confía en este certificado" en el `title` |
| `active` + `error` | Falló una renovación y el actual todavía sirve                                                  | Lo de arriba y un pie en tono `warn` con el `error` y **Reintentar**                                                                      |
| `failed`           | Califica, pero la última emisión falló: Let's Encrypt mientras tanto                            | Un pie en tono `warn` con el `error` y **Reintentar**. El dominio **no** pasa a `error`                                                   |
| `issuing`          | muelle lo está emitiendo, o todavía no leyó sus registros: Let's Encrypt mientras tanto         | La pista "Emitiendo el certificado de Cloudflare…"; la lista se refresca cada 5 s mientras haya uno (`hasProvisioning`)                   |

- El texto del certificado sale de `origin_ca.expires_at` (el que guardó muelle) con
  `cert_source: cloudflare_origin_ca`: `cert_expires_at` es el de la verificación, que puede
  ir hasta una hora atrasado después del cambio, y el texto de `cert_issuer` de Cloudflare
  no está verificado.
- Un dominio en `error` muestra su propio pie (con Reintentar, que también reintenta el de
  Cloudflare): el aviso de Origin CA va cuando el dominio no tiene otro.
- **Reintentar** (`retry`) también reinicia la espera del de Cloudflare que falló: la
  respuesta trae `retry_at` en la hora del pedido, y el resultado se ve en la lista después.
- Cambiar Proxied lo deja `dns_only` desde la respuesta (`false`) o `issuing` (`true`, o
  `failed` si tenía una falla guardada).

## Endpoints

| Endpoint                              | Cuerpo                                                            | Respuesta                     |
| ------------------------------------- | ----------------------------------------------------------------- | ----------------------------- |
| `GET {app}/domains`                   | —                                                                 | `200 [dominio]`, por hostname |
| `POST {app}/domains`                  | `{hostname, target_service, target_port, proxied}`                | `201 dominio`                 |
| `PATCH {app}/domains/{hostname}`      | `{proxied}` o `{target_service, target_port}`: solo lo que cambia | `200 dominio`                 |
| `DELETE {app}/domains/{hostname}`     | —                                                                 | `204`                         |
| `POST {app}/domains/{hostname}/adopt` | — (vacío o `{}`; otra cosa es `400 invalid_json`)                 | `200 dominio`                 |
| `POST {app}/domains/{hostname}/retry` | — (vacío o `{}`; otra cosa es `400 invalid_json`)                 | `200 dominio`                 |

- **Crear**: el panel normaliza el hostname (minúsculas, sin punto final) y lo valida
  antes. En una app de formulario, `target_service` tiene que ser el slug de la app (en una
  compose lo verifica el render del deploy); si no, `422 target_service`. `proxied` va siempre, y **en `false` por defecto** (como el contrato): el
  formulario tiene el switch "Proxied" al lado de "Agregar". Un hostname que ya usa
  cualquier app: `409 conflict`. Tampoco puede ser el dominio del panel (`422 hostname`).
- **Editar**: `proxied` sobre un dominio sin `cf_record_id` es `422` (el panel no ofrece el
  switch en ese caso). Si Cloudflare falla (o ya no hay token), el valor se guarda, el
  dominio queda en `error` + `cloudflare` y responde `200`; si sale bien y estaba en
  `error`/`cloudflare`, vuelve a `pending`. El switch es optimista: se mueve enseguida y
  vuelve si el backend falla.
- **Borrar**: con `cf_record_id`, borra primero los registros en Cloudflare (`A` y `AAAA`).
  Si Cloudflare no responde **o rechaza el borrado** (un token sin permiso):
  `502 cloudflare_unavailable` y **no se borra nada**; el diálogo de confirmación queda
  abierto con el error, que dice cómo borrarlo solo en muelle (quitar el token en Ajustes).
  Sin token, borra solo en muelle.
- **Adoptar**: solo con `error` + `record_exists` (si no, `409 nothing_to_adopt`). Apunta el
  registro existente a la IP pública, **conserva su `proxied` y lo copia al dominio**
  (adoptar no cambia hacia dónde va el tráfico; para cambiarlo, un `PATCH` después) y deja
  el dominio `pending`. Un `CNAME` pasa a ser un `A`. Sin token o sin IP pública, con
  varios `A`/`CNAME` de ese nombre, o si Cloudflare falla: `200` con el dominio en `error` +
  `cloudflare` y el motivo (el aviso de Adoptar lo muestra). Sin IP pública, la tarjeta ya
  dice que falta configurarla en Ajustes.
- **Reintentar** (sincrónico, hasta 20 s): con `cloudflare` rehace el DNS; con
  `record_exists` **vuelve a mirar y, si el registro ajeno ya no está, lo crea**; con
  `pending`, `certificate` o `active` verifica el certificado ahora (un `certificate` que
  sigue sin certificado vuelve a `pending` con otros 10 min, y muelle fuerza a Caddy a
  reintentar la emisión); sin zona, la vuelve a buscar. Responde con el estado resultante.

## `GET /proxy` → 200 (solo con sesión)

El estado de muelle-caddy, para el aviso "proxy desincronizado" (spec §8 y §13). Lo muestra
Dominios, arriba del formulario.

```json
{
	"status": "synced",
	"error": null,
	"synced_at": "2026-10-10T14:20:41Z",
	"caddy": {
		"image": "ghcr.io/cpu-commits/muelle-caddy:v0.1.0",
		"running": true,
		"http_port": 80,
		"https_port": 443
	},
	"ports_rollback": null,
	"panel_certificate": {
		"hostname": "panel.tudominio.cl",
		"cert_source": "letsencrypt",
		"origin_ca": null
	}
}
```

| `status` (y `caddy.running`) | Aviso de Dominios                                                                         |
| ---------------------------- | ----------------------------------------------------------------------------------------- |
| `synced`, corriendo          | Ninguno                                                                                   |
| `synced`, `running: false`   | "muelle-caddy no está corriendo"                                                          |
| `out_of_sync`                | "El proxy está desincronizado", con el `error` y un link a Ajustes; muelle reintenta solo |
| `recreating`                 | "Se está recreando el proxy": cambiaron los puertos en Ajustes (nunca la imagen, §3.5)    |
| `disabled`                   | "El proxy de muelle está apagado": falta el dominio del panel o la imagen de Caddy        |

- `error` viene solo con `out_of_sync`: el último fallo del contenedor (un puerto ocupado, la
  imagen que no baja, el registry sin acceso a la imagen de Caddy) o, si no hay, el de
  Caddy al cargar la configuración (con el token como `***`). Lo que lo arregla está en
  Ajustes (puertos, registries, email, token): el aviso trae el link. Al arrancar muelle,
  hasta el primer envío a Caddy, dice `out_of_sync` con "muelle todavía no le mandó la
  configuración a Caddy".
- `synced_at` es null hasta el primer envío; sin contenedor, `caddy.image` es null y
  `running` false. Con `disabled` no se consulta Docker (también responde sin Docker).
- `ports_rollback` (`portsRollback`): la última vez que muelle volvió a los puertos
  anteriores porque muelle-caddy no arrancó con los nuevos (§3.5), o null:
  `{attempted: {http_port, https_port}, restored: {…}, error, at}`. Vive en la memoria de
  muelle (se pierde al reiniciarlo) y el próximo cambio de puertos la limpia. Viene siempre
  (null si no hubo). Dominios no la usa; Ajustes sí.
- `panel_certificate` (`panelCertificate`): el certificado del panel, o null sin dominio del
  panel. `hostname` es el dominio del panel, y `cert_source` y `origin_ca` tienen la forma y
  las reglas de los de un dominio, salvo que el panel no tiene Reintentar: su falla se
  reintenta sola (su mensaje lo dice). Con `disabled` también viene, y `origin_ca` es null.
  Obligatorio. Dominios no lo usa; Ajustes sí (la línea "Panel: …" de la tarjeta de
  Cloudflare).
- Mientras dice `out_of_sync` o `recreating` se pregunta de nuevo cada 10 s (con la
  pestaña visible). Cada cambio de un dominio (crear, editar, borrar, adoptar,
  reintentar) lo vuelve a pedir: una ruta nueva puede dejarlo desincronizado.
- Si falla (Docker caído, `503 docker_unavailable`, o muelle sin responder) o mientras carga,
  no se muestra nada: ni un error (los dominios tienen sus propios avisos).
- Ajustes no es de este módulo, pero usa `proxyFollowQuery` y `proxyNotice` del barrel: lo
  pide al abrirse (muestra `ports_rollback`) y, después de cambiar los puertos, sigue
  `GET /proxy` mientras dice `recreating` (aunque falle, 2 min como mucho: pasado el plazo
  con el último pedido fallado, `proxyFollowQuery` deja de preguntar), y muestra si muelle-caddy quedó
  escuchando, el `error` si no arrancó, que no se pudo confirmar (pasado el plazo sin una
  respuesta que lo diga) o, si muelle volvió a los puertos anteriores, por qué y la
  dirección anterior del panel (contrato §3.5). Con los certificados de Cloudflare
  encendidos, muestra `panel_certificate` (lo lee con `originCaNotice`), y
  `proxyFollowQuery` también pregunta cada 10 s mientras el del panel dice `issuing`.

## Refresco y avisos

- La lista se refresca cada 5 s mientras muelle está resolviendo algo ("Emitiendo
  certificado", o el de Cloudflare `issuing`) y cada 30 s mientras hay uno "Esperando DNS";
  nunca con la pestaña oculta.
- Cambiar la IP pública o el token en Ajustes cambia los `records` y puede sacar dominios
  de `error` + `cloudflare`, y encender o apagar los certificados de Cloudflare cambia
  `cert_source` y `origin_ca`: Ajustes invalida estas queries (`ingressKeys.all`).
- Los dominios con error se anuncian a un lector de pantalla en **una** región para toda la
  pantalla ("2 dominios tienen errores: …"), que cambia solo si cambia cuáles fallan.

## Lo de ingress que se ve en otras pantallas

Dos reglas de ingress (contrato §7, revisión del 2026-10-17) que el panel muestra fuera de
Dominios:

- **Crear un stage** (catalog) con una red de Docker ajena del mismo nombre:
  `409 network_conflict` (§0.2, §1.1 y §3.4; `ErrNotStageNetwork`). Ya hay una red con el
  nombre de la del stage (`<proyecto>_<stage>`) que no es overlay attachable: muelle no la
  usa ni la borra, y el stage no se guarda. Una overlay attachable con ese nombre sí se usa,
  aunque no la haya creado muelle. catalog lo muestra como `StageNetworkConflict`, nombrando
  la red que hay que borrar o renombrar en el servidor. En el mock, un stage con `ajena` en
  el slug.
- **Ajustes**: un email ACME o un token de Cloudflare con `{` o `}` (Caddy los toma como
  placeholders, §3.4): `PATCH /settings` responde `422` en `acme_email` o `cloudflare_token`
  y no guarda nada (§4.1; el del token, después del de su forma, y sin el token en el
  mensaje). Ajustes los frena antes de mandarlos, con su texto
  (`InvalidAcmeEmail('placeholder')`, `InvalidCloudflareToken('placeholder')`); si igual
  vuelve el `422`, su motivo va al campo, como cualquier `422` de Ajustes. El mock responde
  lo mismo. Si llegan a los ajustes por otro lado (install, un `UPDATE` a mano),
  `GET /proxy` dice `out_of_sync` con el motivo.

## Superficie pública (barrel)

- `DomainsScreen` (props `project`, `stage`, `app` y, opcional, `targets`: los destinos
  posibles como `"api:3000"`). Sin `targets`, los pide al catálogo (`appTargetsQuery` del
  barrel de `catalog`): un `servicio:puerto` por cada puerto que declara cada servicio de
  la app (`services` de `GET {app}`, contrato §2.12). Siempre se suman los que ya usan los
  dominios.
- `domainsQuery`, `proxyQuery`, `proxyFollowQuery`, `ingressKeys`, `ingressUseCases`,
  `domainState`, `originCaNotice`, `proxyNotice` y los tipos `AppDomain`, `DomainState`,
  `Hostname`, `OriginCaState`, `PanelCertificate`, `PortsRollback`, `ProxyStatus`,
  `ProxyNotice` y `ProxyPorts`.

## Mocks (`pnpm dev:mock`)

La primera vez que se abre una app se siembran los tres del diseño:
`{app}-{stage}.tudominio.cl` (válido), `webhooks-{stage}.tudominio.cl` (registro existente,
con Proxied prendido en Cloudflare: Adoptar lo copia) y `{app}.{stage}.otro-dominio.com`
(fuera de Cloudflare, esperando DNS). Las IP públicas son `203.0.113.24` y `2001:db8::24`.

- Un hostname bajo `tudominio.cl` cae en la zona: `pending` y a los ~3 s `active`.
    - Con `existe` en la primera etiqueta: el registro ya existía (Adoptar). Con
      `existe-tmp`, el registro ajeno desaparece a los 5 s y "Volver a mirar el DNS" lo
      crea.
    - Con `cf-error`: Cloudflare responde con error al crearlo (Reintentar).
    - Con `cf-intermitente`: Cloudflare falla al cambiar Proxied o al adoptar (`200` con el
      dominio en `error` + `cloudflare`).
    - Con `cf-caido`: Cloudflare no responde al borrarlo (`502`, no se borra).
    - Con `caddy-rechaza`: Caddy no acepta la configuración y `GET /proxy` dice
      `out_of_sync`.
- En una app de formulario (según el catálogo del mock, aunque nunca se haya desplegado),
  el destino tiene que ser el servicio de la app: otro es `422 target_service`. En una
  compose no se verifica al guardar.
- Cualquier otro hostname queda fuera de Cloudflare (y su `proxied` en `false`) hasta que se
  toca Verificar.
- El token de Cloudflare, las IP públicas y el dominio del panel son los de Ajustes del mock
  (`db/admin.ts`): sin token todo queda fuera de Cloudflare; `records` sigue a las IP de
  ahora (sin IPv6 no hay `AAAA`, sin IP pública `[]`); adoptar sin token o sin IP queda en
  `error` + `cloudflare`; sin dominio del panel, `GET /proxy` dice `disabled`.
- Cambiar los puertos en Ajustes recrea muelle-caddy: `GET /proxy` dice `recreating` unos 4 s
  (sin contenedor: `image: null`, `running: false`) y después `synced`. El puerto `8081`
  está ocupado en el host del mock: el `PATCH` lo prueba antes de guardar y responde `422`
  en ese campo, sin guardar nada (§3.5). El `9443` pasa la prueba pero otro lo toma antes de
  recrear: al terminar la recreación (`ingressDb.settle`, que corren `GET /proxy` y
  `GET`/`PATCH /settings`) muelle vuelve a los puertos anteriores, con `ports_rollback` y
  `settings.ports_rollback` (actor `system`) en la auditoría.
- Un `PATCH` de Proxied que sale bien saca un `error` + `cloudflare` (vuelve a `pending`).
  Sin token (se quitó en Ajustes), se guarda y el dominio queda en `error` + `cloudflare`
  con "agrega el token de Cloudflare en Ajustes", como en el backend.
- **Certificados de Cloudflare** (`cloudflare_origin_ca` de Ajustes, apagados al empezar):
  con la opción encendida, el token y el dominio del panel (si no, `origin_ca` es null),
  durante la primera pasada (~3 s desde que se encendió, o desde que se creó el dominio) un
  dominio que califica dice `issuing` y después `active`, con 90 días. Uno manejado sin
  Proxied es `dns_only`; uno que muelle no maneja, o fuera de Cloudflare, `not_eligible`. Por
  la primera etiqueta del hostname (también la del dominio del panel, en
  `panel_certificate`):
    - `falla-`: la emisión falla por el permiso del token (`failed` + `permission`, con el
      mensaje del backend; se reintenta en 6 h o con Reintentar, que pone `retry_at` en la
      hora del pedido);
    - `gris-`: en Cloudflare el registro está en DNS only (`dns_only` con Proxied prendido
      en muelle);
    - `renueva-`: falló la renovación y el actual todavía sirve (`active` con el aviso).
- En Ajustes, el token elige la respuesta de Cloudflare al encenderlos: con `sin-ssl` le
  falta el permiso (`422` en `cloudflare_origin_ca`, o en `cloudflare_token` si se cambia el
  token con ellos encendidos) y con `ssl-lento` Cloudflare no responde (`502`). Sin token,
  encenderlos es `422`, y quitar el token con ellos encendidos sin mandarlos en `false`,
  también (contrato §4.1, con los mensajes del contrato).

## Diferencias con el contrato

Ninguna.
