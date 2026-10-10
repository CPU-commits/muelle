# muelle — Fase 4 (Panel): Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** El panel web de muelle (spec §11) en `web/`, una SPA embebida en el binario de Go, que implementa las 11 pantallas del diseño.

**Architecture:** Copia de la plantilla `~/Escritorio/dev/templates/svelte-clean-template`. Sus reglas son obligatorias y están en su `CLAUDE.md`, que se copia a `web/CLAUDE.md`. En resumen:
- 3 capas por módulo: `domain` / `data` / `ui`;
- el dominio es puro y las dependencias entran por puertos;
- los DTOs no salen de `data`;
- la UI no ve status HTTP;
- cada pantalla es un viewModel con runes + un `.svelte` con solo markup + `styles.css.ts`;
- entre módulos, solo el barrel;
- textos con Paraglide (`es` + `en`);
- estilos solo con tokens del tema.

Los módulos del frontend reflejan los del backend:

| Módulo | Contenido |
|---|---|
| `auth` | sesión, login, 2FA, setup, invitación |
| `catalog` | proyectos, stages, apps |
| `secrets` | variables |
| `identity` | usuarios, invitaciones, tokens |
| `admin` | settings, auditoría |
| `deploy` | Fase 2: estado, deploys, logs (con mocks) |
| `ingress` | Fase 3: dominios (con mocks) |

**Tech Stack:** el de la plantilla: Svelte 5, SvelteKit 2 (adapter-static, sin SSR), TypeScript strict, TanStack Query/Form, Zod 4, vanilla-extract, Paraglide, Vitest (node + browser), Playwright, MSW y pnpm. Agrega CodeMirror 6 para el editor YAML (App·Compose) y fuentes Geist servidas desde el propio binario, sin CDN.

**Spec:** `docs/superpowers/specs/2026-10-09-muelle-design.md` (§10 API, §10.1 auth, §11 panel).

**Diseño:** el canvas de Claude Design `https://claude.ai/artifact/1KK1CAWeGNG2rXK2C7hkVB`. Se lee con la herramienta Artifact (`action: "read"`, `url`, `path: "project/<Pantalla>.dc.html"`). Cada pantalla es un `.dc.html`, HTML con estilos inline y un bloque `DCLogic` con datos de ejemplo. Se reproduce la **apariencia** (layout, tipografía, colores, espaciado, estados), no el mecanismo: nada de `<x-dc>`, `sc-for` ni `DCLogic`.

## Sistema de diseño (sacado de `Main.dc.html`)

| Elemento | Valores |
|---|---|
| Esquema | oscuro (`color-scheme: dark`) |
| Fondos | página `#0A0A0B`; superficies `#101012`, `#0D0D0F`, `#141416` (hover de fila) y `#18181B` (botón secundario, avatar) |
| Texto | `#EDEDED` principal, `#A1A1A8` secundario, `#83838C` terciario, `#6E6E77` deshabilitado/iconos |
| Bordes | `#222225` suave, `#2E2E33` de controles; foco `#6E6E77` + `box-shadow 0 0 0 3px rgba(237,237,237,.08)`; `focus-visible` `2px solid #EDEDED` con offset de 2 px |
| Estados | ok `#46C37B`; aviso `#F5A524` (texto `#F5C26B`, borde `rgba(245,165,36,.3)`, fondo `rgba(245,165,36,.06)`); error `#F2555A` (texto `#F2777D`, borde `rgba(242,85,90,.3)`, fondo `rgba(242,85,90,.06)`); inactivo con anillo `#83838C` |
| Tipografía | Geist 400/500/600, base 14 px con line-height 1.43; Geist Mono para slugs técnicos, imágenes y redes (12-13 px); h1 24/32 600 con `letter-spacing -0.03em`; h2 15 600 |
| Controles | alto 36 (botones e inputs), 32 (botones chicos), 28 (acciones de fila); radio 6 px en controles y 8 px en tarjetas y avisos; badges de 20 px con radio 4 px o pastilla (999 px) |
| Botones | primario: fondo `#EDEDED` y texto `#0A0A0B` (hover `#FFF`); secundario: borde `#2E2E33`, fondo transparente (hover `#18181B`); ghost: sin borde (hover `#18181B`) |
| Layout | contenedor `max-width 1248px`, padding lateral `clamp(16px, 3vw, 24px)`; header con logo (SVG del muelle), avatar, y nav con tabs subrayados de 44 px (activo con `border-bottom 2px #EDEDED`) |
| Accesibilidad | mínimo 4.5:1, objetivos de ≥ 28 px en filas y ≥ 36 px en general, `aria-label` en botones de solo icono; funciona en ancho de teléfono (todo envuelve) |

## Contrato de API

- **Fase 1 (existe):** el código del backend es la fuente de verdad. Los formatos de request y response están en:
  - `src/internal/*/infra/in/http/*` y `src/internal/*/infra/in/form/*`;
  - `src/internal/*/app/dto/*` y `src/internal/identity/infra/in/http/res.go`.
- **Errores:** siempre `{"error":{"code","message"}}`. Los códigos están en los `mapper_errors.go` de cada módulo y en `src/cmd/http/server.go`.
- **Sesión:** cookie `muelle_session` HttpOnly. El frontend nunca la lee. Usa `fetch` con `credentials: 'same-origin'`, y el navegador manda `Origin` solo.
- **Fases 2 y 3 (no existen):** el contrato es el del spec §10:
  - deploys, `log?offset`, rollback, status, logs SSE y dominios;
  - en Ajustes: registries, Cloudflare, IP pública, email ACME y `panel_allowed_cidrs`.

  Los tipos se derivan de §5.1, §7, §8 y §9 del spec. Con MSW funcionan en `pnpm dev:mock`. Contra el backend real, el repositorio traduce el 404 de esas rutas a un `UnavailableError` de dominio, y la pantalla muestra un estado vacío: "Llega con la Fase 2" o "Llega con la Fase 3".

## Etapas

| Etapa | Tareas | Cómo corre |
|---|---|---|
| F0 Base | F0 | 1 agente, en la rama `fase-4-panel` |
| F1 Pantallas | F1-catalog, F1-secrets, F1-identity-admin, F1-deploy-ingress | **en paralelo**, cada una en su worktree desde `fase-4-panel` (tag `panel-f0`) |
| F2 Integración | F2 | secuencial, en `fase-4-panel`, después de mergear F1 |

**Regla de paralelismo:** cada tarea F1 solo crea o modifica archivos en:
- `web/src/modules/<su módulo>/`;
- `web/src/routes/(app)/…` de sus pantallas;
- sus mocks en `web/src/mocks/handlers/<módulo>.ts` y `web/src/mocks/db/<módulo>.ts`;
- sus claves de i18n, con el prefijo `<módulo>_` y solo agregando líneas.

`messages/*.json` es el único archivo que comparten; al mergear se combinan las claves. Si una tarea necesita cambiar `shared/`, se detiene y lo reporta.

---

### Task F0: Base del panel

**Files:**
- Create: todo `web/`.
- Create: `docs/superpowers/plans/2026-10-10-muelle-fase4-panel.md`, copia de este plan.
- Modify: `.gitignore` del repo (`web/node_modules`, `web/.svelte-kit`, `web/build`, `web/test-results`).

1. **Copiar la plantilla** a `web/`, sin `node_modules`, `.svelte-kit`, `build`, `test-results`, `.git`, `.env`, `Dockerfile`, `nginx.conf.template`, `.dockerignore` ni `.github`. Después:
   - el nombre del paquete pasa a ser `muelle-panel`;
   - el README pasa a ser un `web/README.md` corto;
   - `web/CLAUDE.md` es el de la plantilla, más una sección "muelle" con las notas de este plan.
2. **Borrar el módulo `example`**, sus rutas y sus mocks. **Mantener y adaptar** `auth` y todo `shared`.
3. **Tema.** Los tokens del sistema de diseño de arriba van en `shared/ui/themes`: un tema oscuro que reemplaza al claro. **Fuentes:** Geist y Geist Mono empaquetadas (paquete npm `geist` o `@fontsource/*`), sin pedidos a Google.
4. **Átomos, moléculas y organismos** restilizados según el diseño:
   - Button (primary, secondary, ghost, danger; tamaños 36, 32 y 28), Input, Select, Textarea, Badge (neutral, warn, error, ok, mono), StatusDot (ok, warn, bad, idle) y Spinner;
   - Banner (warn, error), Tabs (nav subrayada), Modal, ConfirmDialog, EmptyState, ErrorState, Field, Table, Toaster y PageHeader;
   - AppShell: header con el logo SVG de `Main.dc.html`, botón de cuenta con menú (2FA y cerrar sesión), y nav con Proyectos y Ajustes;
   - un componente `Mono` para texto técnico.
5. **`shared/data/http`.**
   - Base `/api/v1`, `credentials: 'same-origin'` y JSON.
   - Parseo de `{"error":{code,message}}` a `ApiError` (code, status, message).
   - Los 401 en rutas autenticadas van al login.
   - `config/local.json` y Vite: proxy de `/api` a `http://127.0.0.1:18080`; en desarrollo, `MUELLE_PANEL_ORIGIN=http://localhost:5173`.
6. **Módulo `auth`** adaptado a muelle:
   - **Entidad:** `Me` (`username`, `totpEnabled`, `needsTotpSetup`).
   - **Casos de uso:** `getMe`, `login` (devuelve si pide 2FA), `completeTotp`, `logout`, `setup` (token de la URL), `acceptInvite`, `beginTotpSetup` (secreto + URL otpauth) y `enableTotp`.
   - **Errores de dominio** según los códigos del backend:
     - `invalid_credentials`, `locked`, `rate_limited`, `invalid_code`;
     - `mfa_required`, `totp_setup_required`, `invalid_invite`, `already_set_up`.
   - **Pantallas** (diseño de `Login.dc.html`):
     - `/login`: contraseña y después código;
     - `/setup?token=`: primer usuario;
     - `/invite/[token]`: aceptar invitación;
     - `/2fa`: configurar 2FA con QR, que es obligatoria si `needsTotpSetup`.
     - Las variantes que el diseño no tiene se arman con el mismo layout.
   - **Guard en `(app)/+layout.ts`:** consulta `getMe`. Sin sesión va a `/login`; con `needsTotpSetup` va a `/2fa`.
   - **QR:** la URL otpauth se muestra como QR con una librería chica (`qrcode`, render a SVG), y además el secreto en texto.
7. **Rutas vacías** que llenan las tareas F1, cada una con un placeholder `EmptyState`:
   - `(app)/+page.svelte` (Proyectos);
   - `(app)/projects/[p]/stages/[s]/variables`;
   - `(app)/projects/[p]/stages/[s]/apps/[a]/+layout.svelte`, con tabs Resumen, Configuración, Variables, Dominios, Deploys y Logs, más un breadcrumb `proyecto / stage / app` armado con los parámetros de la ruta (el header lo enriquece catalog);
   - `…/apps/[a]/{+page (resumen),configuracion,variables,dominios,deploys,logs}`;
   - `(app)/ajustes` y `(app)/ajustes/tokens`.
8. **MSW:** `src/mocks/` organizado como `handlers/<módulo>.ts` + `db/<módulo>.ts`, con un `handlers/index.ts` que los junta. Mocks de `auth` con un usuario demo `felipe` / `una-contraseña-larga` y 2FA desactivada.
9. **Tests:**
   - los de `auth` (casos de uso con fake, mapper e integración contra MSW);
   - el viewModel de Login en browser;
   - un e2e de login con MSW.
10. **`pnpm install && pnpm verify`** pasa, y `pnpm build` genera `web/build`.
11. **Commit** en `fase-4-panel` y tag `panel-f0`.

### Task F1-catalog: Proyectos, app (header), Configuración y Compose

**Diseño:** `Main.dc.html`, `App-Configuracion.dc.html`, `App-Compose.dc.html`, y el header y los tabs de las pantallas `App-*.dc.html`.

**API:** `GET/POST /projects`, `GET/PATCH/DELETE /projects/{p}`, stages y apps (Fase 1).

1. **Módulo `catalog`:** entidades `Project`, `Stage` y `App` (`kind` service|compose, `spec`, `composeYaml`, `pendingChanges`, `lastDeployedAt`), con sus casos de uso, DTOs, mappers y queries.
2. **Proyectos** (`(app)/+page.svelte`):
   - lista de proyectos → stages → apps, como en el diseño, con buscador;
   - "Nuevo proyecto", "Nueva app en stage" y "Nuevo stage" en modales;
   - opciones de proyecto (renombrar, borrar con confirmación; `not_empty` → mensaje claro);
   - el link "Variables · N" va a la página de variables del stage;
   - el estado ("Corriendo 2/2") y el último deploy son de la Fase 2: quedan como un slot que llena F2. Por ahora muestran el `kind` y, si es service, la imagen del spec.
3. **Header de la app** en `apps/[a]/+layout.svelte`: nombre, kind, imagen y badge de cambios pendientes, según el diseño.
4. **Configuración** (form app): formulario TanStack Form + Zod que replica las reglas de `appspec` del backend:
   - imagen, puerto, réplicas, comando y healthcheck (`http`, `bun-http`, `command`, `none`, con sus campos);
   - volúmenes, recursos y `deploy_timeout_s`;
   - errores 422 del backend mapeados a sus campos; aviso de `none` sin cero downtime (§6.1).
5. **Compose:** editor YAML con CodeMirror 6 (resaltado YAML y tema oscuro con los tokens). Guarda con PATCH `compose_yaml`; error 422 → mensaje.
6. **Tests:** dominio, mappers, integración contra MSW y viewModels (lista y form).

### Task F1-secrets: Variables

**Diseño:** `App-Variables.dc.html`.

**API:** `GET/PUT/PATCH …/stages/{s}/env` y `…/apps/{a}/env` (Fase 1).

1. **Módulo `secrets`:** entidad `EnvVar` (`key`, `value` o `null` si es secreta, `secret`, `updatedAt`), con sus casos de uso `getEnv`, `replaceEnv` y `patchEnv`.
2. **Pantalla de variables de app:**
   - las del stage en solo lectura, con link a la página del stage;
   - las de la app, editables: agregar, editar, borrar y marcar como secreta;
   - los secretos se ven como `••••` y no se pueden leer, solo reemplazar;
   - "Pegar .env": un textarea que parsea `KEY=VALUE`, maneja comillas y comentarios, y valida las claves como el backend (`validate.EnvKey`; prefijo `COMPOSE_` reservado). Previsualiza los cambios y aplica con PATCH.
   - Avisa que cambiar variables deja la app con "cambios pendientes".
3. **Pantalla de variables de stage** (`stages/[s]/variables`): lo mismo, con el dueño stage.
4. **Errores:** 422 por campo y 403 `out_of_scope` (no debería pasar con sesión).
5. **Tests** de dominio (parser de `.env`), mappers, integración y viewModel.

### Task F1-identity-admin: Ajustes y Tokens

**Diseño:** `Ajustes.dc.html` y `Ajustes-Tokens.dc.html`.

**API:**
- Fase 1: `GET/PATCH /settings` (`require_2fa`), `GET /audit`, `GET /users`, `DELETE /users/{u}`, `DELETE /users/{u}/totp`, `POST /invites`, y `GET/POST/DELETE /tokens`.
- Fases 2/3, con mocks: registries, Cloudflare, IP pública, email ACME y `panel_allowed_cidrs`.

1. **Módulos `identity`** (usuarios, invitaciones, tokens) **y `admin`** (settings, auditoría).
2. **Ajustes:** todas las secciones del diseño.
   - **Exigir 2FA:** el error `totp_required_first` se muestra como "configura tu 2FA primero", con link a `/2fa`.
   - **Usuarios:** listar, eliminar con confirmación, resetear 2FA de otro usuario.
   - **Invitar:** muestra la URL una sola vez, con botón de copiar.
   - **Auditoría:** paginada con `before`.
   - **Secciones de Fase 2/3:** con mocks y `UnavailableError` en el backend real.
3. **Tokens:**
   - listar, con nombre, proyecto/stage, último uso y vencimiento;
   - crear: modal con proyecto y stage elegidos de listas que vienen del barrel de `catalog`, o de su propio datasource si catalog todavía no está mergeado; al crear se muestra el token una sola vez, con copiar y el aviso de que no se vuelve a mostrar;
   - revocar con confirmación.
4. **Tests** de dominio, mappers, integración y viewModels.

### Task F1-deploy-ingress: Resumen, Deploys, Logs y Dominios (con mocks)

**Diseño:** `App-Resumen.dc.html`, `App-Deploys.dc.html`, `App-Logs.dc.html` y `App-Dominios.dc.html`.

**API (no existe todavía):** spec §7 (deploys), §8 (proxy), §9 (DNS) y §10 (endpoints).

1. **Módulo `deploy`:**
   - **Estado de app** (`GET …/apps/{a}/status`): réplicas, tareas, CPU% y RAM. Se refresca cada 5 s, solo con el tab visible.
   - **Desplegar:** con override de imagen opcional.
   - **Rollback.**
   - **Historial:** `GET …/deploys`.
   - **Log de deploy:** SSE `…/deploys/{id}/log/stream`, con fallback a polling `?offset`.
   - **Logs en vivo:** SSE `…/logs?service&tail&follow`.
2. **Módulo `ingress`:** dominios (`GET/POST …/domains`, `PATCH/DELETE …/domains/{hostname}`, `adopt`, `retry`), con el estado de DNS y del certificado y `proxied`.
3. **MSW:** handlers con datos realistas que recorren los estados (deploy `queued` → `running` → `succeeded`/`failed`, log en streaming, dominio en `error` con "Adoptar").
4. **Backend real:** un 404 de estas rutas → `UnavailableError` → `EmptyState` "Llega con la Fase 2" (o 3), sin errores en consola.
5. **Tests:** dominio, mappers, integración contra MSW (también el `UnavailableError`) y viewModels.

### Task F2: Integración

1. **Mergear** las cuatro ramas F1 en `fase-4-panel`. En `messages/*.json`, combinar las claves y ordenarlas.
2. **Proyectos:** llenar el slot de estado y último deploy con el barrel de `deploy`. Si devuelve `UnavailableError`, se muestra "—".
3. **Go:**
   - `web/embed.go` (paquete `web`, `//go:embed all:build`), con un `web/build/.keep` versionado para que `go build` funcione sin el panel compilado;
   - `src/cmd/http` sirve el SPA: las rutas que no son `/api/` entregan archivos de `build` con cache, y `200.html` como fallback;
   - si no hay build, responde un 503 en texto claro, "panel no compilado: cd web && pnpm build";
   - headers de seguridad para el panel: `Content-Security-Policy` (`default-src 'self'`; `style-src 'self' 'unsafe-inline'` solo si hace falta; `img-src 'self' data:`; `connect-src 'self'`; `frame-ancestors 'none'`), `Referrer-Policy: same-origin` y `X-Frame-Options: DENY`;
   - tests del handler.
4. **CI:** un job de `web` (`pnpm install --frozen-lockfile`, `pnpm verify`, `pnpm build`) antes del build de Go, que embebe el panel.
5. **e2e real:** Playwright contra `muelle serve` con el panel embebido. Recorre setup → login → crear proyecto, stage y app → editar variables → crear token.
6. **README:** cómo desarrollar el panel (`pnpm dev` con el backend y `pnpm dev:mock`).
