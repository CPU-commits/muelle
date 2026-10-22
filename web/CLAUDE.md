# Instrucciones del proyecto

SPA Svelte 5 + SvelteKit (sin SSR), con clean architecture de 3 capas por módulo.
Antes de tocar código, leé la sección [Arquitectura](#arquitectura) de este archivo y, para
lo propio de muelle, la sección [muelle](#muelle).

## Reglas que no se negocian

1. **El dominio es puro.** `src/modules/*/domain/**` no importa Svelte, `$app/*`,
   TanStack, `@/shared/data` ni `@/shared/ui` (stores, estilos, i18n). Si necesitás algo de
   eso en el dominio, es que la lógica está en el lugar equivocado.

2. **Las dependencias entran por puertos.** El dominio declara interfaces en
   `domain/repositories/`; la capa `data` las implementa; `container.ts` las conecta.
   Ningún caso de uso importa un repositorio concreto.

3. **Los DTOs no salen de `data`.** La forma del backend vive en `data/dtos/` y se traduce
   en `data/mappers/`. Si un `created_at` o un `snake_case` aparece en la UI, hay un mapper
   faltante.

4. **La UI no ve status HTTP.** Los repositorios traducen `ApiError` a errores de dominio
   (`NotFoundError`, `ForbiddenError`, …). Los viewModels miran `error.code`, nunca `error.status`.

5. **Estado y markup separados.** Toda pantalla es `X.viewModel.svelte.ts` (clase con
   `$state`/`$derived`, queries, mutations y navegación) + `XScreen.svelte` (sólo markup) +
   `styles.css.ts` (vanilla-extract). Si hay un `$state`, `$derived` o `$effect` en el
   `<script>` de una pantalla, va al viewModel.

6. **Entre módulos, sólo el barrel.** Se importa `@/modules/<x>`, nunca
   `@/modules/<x>/data/...`.

7. **Los casos de uso son `async`.** Aunque sólo deleguen: garantiza que un error de
   validación llegue como promesa rechazada y no como throw síncrono.

8. **Nada de SSR.** `ssr = false` en el layout raíz y `adapter-static` con fallback. No hay
   `+page.server.ts`, `+server.ts` ni `hooks.server.ts`: si hace falta lógica de servidor,
   va en el backend.

9. **`shared/` tiene las mismas 3 capas que un módulo: `domain/`, `data/`, `ui/`.** Nada de
   `shared/utils/`, `shared/helpers/` ni carpetas sueltas que mezclen capas: un helper va en
   la capa que lo usa (`shared/ui/utils/format.ts`, `shared/data/http/parse.ts`).
   `shared/data` no importa `shared/ui`.

Estas reglas se revisan a mano en code review: no hay un verificador automático.

## Convenciones

- **Nombres de archivo**: `camelCase.ts` para lógica, `PascalCase.svelte` para componentes,
  `X.viewModel.svelte.ts` y `styles.css.ts` colocados junto a la pantalla.
- **Módulos con runes** (`.svelte.ts`) se importan **con la extensión completa**:
  `import { XViewModel } from './X.viewModel.svelte.ts'`.
- **ViewModels**: una clase instanciada en el `<script>` de la pantalla
  (`const vm = new XViewModel()`). `useQueryClient`, `createQuery` y `createMutation` sólo
  funcionan durante la inicialización del componente.
- **Acciones de un viewModel**: arrow functions de clase (`save = async () => {…}`), porque
  la pantalla las pasa como callback (`onclick={vm.save}`) y un método perdería el `this`.
- **Props reactivas hacia un viewModel**: como getter (`new XViewModel(() => id)`), nunca
  el valor suelto, o el viewModel queda congelado con el valor inicial.
- **Una query (o mutation) cuyas opciones leen esos getters se crea en el `constructor`**,
  no en el inicializador del campo. Los inicializadores corren ANTES del cuerpo del
  constructor, y `createQuery` evalúa las opciones en el acto: `this.#app` todavía es
  `undefined`. Las que no leen props (`createQuery(() => meQuery())`) pueden ir en el campo.

    ```ts
    readonly #app: () => AppRef
    readonly #deploys: CreateQueryResult<Deploy[], Error>

    constructor(app: () => AppRef) {
    	this.#app = app
    	this.#deploys = createQuery(() => deploysQuery(this.#app()))
    }
    ```

- **Entidades en estado**: `$state.raw`. Son inmutables, no hace falta volverlas un proxy.
- **Navegación**: siempre con `resolve()` de `$app/paths` (`goto(resolve('/x'))`,
  `href={resolve('/x/[id]', { id })}`). eslint lo exige.
- **Links con query string** (`?deploy=7`): `resolve()` no arma queries y eslint marca
  `resolve(…) + '?…'` en un `<a href>`. Se usa `withQuery` de `@/shared/ui` como `href` de un
  componente de `shared/ui` (`TextLink`, `LinkButton`, `ListRow`, los items de `Tabs`, `Menu`
  o `Breadcrumb`), sin `eslint-disable`:
  `<TextLink href={withQuery(resolve('/(app)/projects/[p]/stages/[s]/apps/[a]/deploys', params), { deploy: id })}>`.
  Para leerlo, `page.url.searchParams.get('deploy')` de `$app/state`. Un `goto` con query
  no tiene salida limpia (eslint mira el argumento): si hace falta, es un link.
- **Casos de uso**: un archivo por operación, factory `makeXxx(deps)` que devuelve la función.
- **Cache keys y opciones de query**: todas en `data/queries/`, nunca inline en un viewModel.
- **Textos**: siempre desde `import { m } from '@/shared/ui/i18n/paraglide/messages'`,
  nunca hardcodeados en la UI. En muelle cada módulo tiene sus archivos de mensajes (ver
  [i18n](#i18n-prefijo-por-módulo)): agregá la clave en el `es.json` **y** el `en.json`.
- **Estilos**: sólo tokens de `theme` (`@/shared/ui/themes/theme.css`), nunca un hex suelto.
- **Errores de dominio**: heredan de las clases de `@/shared/domain`, con `name` propio.
- **Entidades inmutables**: las operaciones devuelven copia (`{ ...example, name }`).
- **`exactOptionalPropertyTypes` está activo**: para un campo opcional hay que OMITIR la
  clave (`...(x ? { x } : {})`), no asignarle `undefined`.

## Tests

- **Los tests viven junto a lo que prueban**, en `__tests__/` al lado del archivo, uno por
  archivo y con su mismo nombre: `domain/use-cases/login.ts` →
  `domain/use-cases/__tests__/login.test.ts`. No hay carpeta `tests/` por módulo.
- Dominio, casos de uso y mappers: `*.test.ts`, corren en node contra el fake en memoria del
  puerto (`domain/repositories/__fakes__/`), sin mockear `fetch`.
- Integración de un módulo: `<modulo>/__tests__/*.integration.test.ts` en node contra MSW
  (`@/mocks/server`), entrando por `container.ts`.
- ViewModels: `__tests__/X.viewModel.svelte.test.ts` junto a la pantalla, corren en Chromium
  (el `.svelte.test.ts` es lo que los manda al proyecto browser de vitest). Se montan con un
  harness (en el mismo `__tests__/`) que hace
  `setQueryClientContext(createQueryClient({ retry: false }))` (de `@/shared/data/query`:
  las mismas opciones que la app) e instancia el viewModel, mockeando el **container** del
  módulo, no la red. Ver
  `src/modules/auth/ui/screens/login/__tests__/` (también mockea `$app/navigation`: `goto`
  no existe fuera del runtime de SvelteKit).
- E2E: `e2e/*.e2e.ts` contra el build con MSW servido con la CSP (`pnpm test:e2e`, ver
  [CSP](#csp)).
- E2E contra el backend de verdad: `e2e-real/*.real.ts`, el panel embebido en un
  `muelle serve` con el Swarm local (`pnpm test:e2e:real`, ver
  [E2E contra muelle](#e2e-contra-muelle)). Lo de node puro que usa (los códigos TOTP de
  `totp.ts`, la carpeta de `runDir.ts` y la limpieza de `teardown.ts`) se prueba en
  `e2e-real/*.test.ts`, que corre en el proyecto server de vitest
  con `pnpm test`, sin muelle ni Swarm.
- Componentes: `__tests__/X.svelte.test.ts` junto al componente, en Chromium, con un
  harness cuando hace falta un contexto (ver `molecules/menu/__tests__/`). Un harness que
  mide posiciones importa `@/shared/ui/themes/global.css` (sin el `box-sizing` global las
  medidas no son las del panel).
- Memoria: el Chromium de vitest corre con `--expose-gc`. Para lo que muestra un secreto (una
  URL de invitación, un token), un test guarda `WeakRef`s de lo que se cerró y pide
  `expect(await aliveAfterGc(refs)).toBe(0)` (`shared/ui/components/__tests__/memory.ts`:
  GC mayor asincrónico, con reintentos acotados). Ver "no retiene lo que mostró" en
  `Modal.svelte.test.ts` y "no deja vivo el `<dialog>`" en `CopyButton.svelte.test.ts`. Dos
  cosas retienen y no son del componente: el último evento que Svelte delegó (un clic en otra
  parte de la página lo suelta) y un `expect.element(…)` que encontró el elemento (usar
  `expect.poll` sobre el texto).

## Antes de decir que terminaste

```bash
pnpm verify     # lint + check + test
```

Si tocaste `messages/` o `project.inlang/`, corré `pnpm codegen` antes.

---

## Arquitectura

### El mapa

```
src/
  routes/          rutas file-based de SvelteKit. Sólo CABLEAN pantallas y guards.
  modules/         una carpeta por dominio de negocio (el corazón).
  shared/          las MISMAS 3 capas que un módulo, y nada más.
    domain/        dominio compartido: Brand, Email, errores, Paginated.
    data/          infraestructura de datos: config, cliente http, QueryClient.
    ui/            design system (atoms / molecules / organisms) y todo lo de la vista:
      themes/      tokens de vanilla-extract, paleta, fuentes, estilos globales.
      stores/      estado cliente global con runes: toasts.
      i18n/        locale + paraglide generado (`i18n/paraglide`).
      utils/       helpers de la vista: formateo con Intl, errores de formularios.
      assets/      favicon y estáticos importados.
  mocks/           MSW: handlers/<módulo>.ts + db/<módulo>.ts.
  hooks.client.ts  arranque de la app (el `main.tsx` de una SPA).
```

Alias: `@` → `src/`.

### Anatomía de un módulo

```
src/modules/<módulo>/
  domain/                        PURO. Sin Svelte, sin fetch, sin estilos.
    entities/                    entidades + invariantes (inmutables)
    value-objects/               ids branded, enums con máquina de estados
    repositories/                PUERTOS (interfaces), no implementaciones
      __fakes__/                 el fake en memoria del puerto, para los tests
    use-cases/                   una operación de negocio por archivo
    errors/                      errores de dominio del módulo
    index.ts
  data/                          ADAPTADORES. Traduce backend <-> dominio.
    dtos/                        forma exacta del backend + schema Zod
    mappers/                     DTO <-> entidad (el único que conoce ambas)
    datasources/                 rutas y verbos HTTP
    repositories/                implementa el puerto; traduce errores
    queries/                     cache keys + queryOptions + mutations
    index.ts
  container.ts                   COMPOSITION ROOT: inyecta el repo en los casos de uso
  ui/
    screens/<pantalla>/          XScreen.svelte + X.viewModel.svelte.ts + styles.css.ts
    features/<bloque>/           bloques compuestos reutilizables
    components/<pieza>/          piezas chicas del módulo
    index.ts
  __tests__/                     integración del módulo entero contra MSW
  index.ts                       superficie pública del módulo
```

El módulo de referencia es `src/modules/auth`.

### El flujo completo

```
XScreen.svelte  ->  X.viewModel.svelte.ts  ->  data/queries  ->  caso de uso  ->  PUERTO
                                                                                    |
                                                          data/repositories (adaptador)
                                                                                    |
                                                  data/datasources -> shared/data/http -> backend
                                                                                    |
                                                             data/mappers (DTO -> entidad)
```

La UI nunca ve un DTO ni un status HTTP. El dominio nunca ve un `fetch`.

### Reglas de dependencia

| Capa               | Puede importar                                  | No puede importar                                                          |
| ------------------ | ----------------------------------------------- | -------------------------------------------------------------------------- |
| `modules/*/domain` | `@/shared/domain`, `zod`                        | Svelte, `$app/*`, TanStack, `@/shared/data`, `@/shared/ui`, su `data`/`ui` |
| `modules/*/data`   | su `domain`, `@/shared/domain`, `@/shared/data` | Svelte, `$app/*`, `@/shared/ui`, su `ui`                                   |
| `modules/*/ui`     | su `domain`, su `data`, `@/shared/*`, `$app/*`  | tripas de otros módulos                                                    |
| `routes`           | `@/modules/<x>` (barrel), `@/shared/*`          | `domain`/`data`/`ui` de un módulo directamente                             |
| `shared/ui`        | `@/shared/domain`, `@/shared/data`              | `modules/`                                                                 |
| `shared/data`      | `@/shared/domain`                               | `modules/`, `@/shared/ui`                                                  |
| `shared/domain`    | nada                                            | todo lo demás                                                              |

### Configuración por entorno

`config/local.json` y `config/production.json` guardan la config **pública** de cada
entorno, validada con Zod al arrancar (`src/shared/data/config.ts`). En muelle `apiUrl` es
`''` en los dos: el panel y la API comparten origen. `devProxy` (solo local) es a dónde
proxea `pnpm dev`. Nada de secretos: todo lo que empieza con `VITE_` entra al bundle.

---

## muelle

El panel de muelle (spec §11): una SPA que el binario de Go sirve junto a la API
`/api/v1`. Plan: `docs/superpowers/plans/2026-10-10-muelle-fase4-panel.md`. Diseño: el
canvas de Claude Design `https://claude.ai/artifact/1KK1CAWeGNG2rXK2C7hkVB`
(`project/<Pantalla>.dc.html`): se copia la apariencia, no el mecanismo. Los textos de la
UI van en español neutro con **tú** ("Usa tu usuario", "Ingresa el código"), como el diseño.

### Comandos de desarrollo

```bash
pnpm install
pnpm dev:mock     # sin backend: MSW contesta en el navegador
pnpm dev          # con un `muelle serve` local (ver web/README.md)
pnpm verify       # antes de decir que terminaste
pnpm test:e2e     # playwright contra build-e2e/ (con y sin MSW, bajo la CSP); no toca ./build
                  # aislado de la red como pnpm test (ver "Cosas que conviene saber")
pnpm test:e2e:real  # el panel embebido en un muelle serve de verdad, con el Swarm local
                    # (ver "E2E contra muelle"): construye ./build y el binario con -tags panel
pnpm build        # el ./build que embebe el binario (sin MSW ni mockServiceWorker.js)
```

Con el backend real: `MUELLE_PANEL_ORIGIN=http://localhost:5173 MUELLE_SECURE_COOKIES=false
MUELLE_LISTEN=127.0.0.1:18080 … go run . serve` en la raíz y `pnpm dev` acá. El backend
compara el `Origin` de cada escritura con los permitidos (contrato §0.1): sin
`MUELLE_PANEL_DOMAIN` y con `localhost` (que no es un dominio) no hay dominio del panel, y el
único que pasa es el de `MUELLE_PANEL_ORIGIN`. El proxy de Vite lo deja pasar tal cual.
`MUELLE_BACKEND_URL` cambia el destino del proxy.

Datos de `pnpm dev:mock` (`src/mocks/db/auth.ts`):

| Qué                  | Valor                                                                        |
| -------------------- | ---------------------------------------------------------------------------- |
| Usuario / contraseña | `felipe` / `una-contraseña-larga` (sin 2FA al empezar)                       |
| Código de 2FA válido | `123456` (el mock no calcula TOTP)                                           |
| Invitación           | `/invite/invitacion-demo`                                                    |
| Setup                | `/setup?token=setup-demo` (responde `already_set_up` mientras haya usuarios) |

El estado del mock vive en `sessionStorage` de la pestaña: sobrevive a un F5. Para empezar
de cero, cerrá la pestaña.

### Cambios en panel-f0-fix (para el rebase de F1)

Lo que cambió en `shared/` desde `panel-f0` y puede pedir un ajuste al rebasar una rama F1.
Lo nuevo y aditivo (props, helpers, componentes) está en las tablas de abajo.

- `BreadcrumbItem` es una unión: `BreadcrumbLink` (`{ label, href?, initial? }`, igual que
  antes) o `BreadcrumbMenu` (`{ kind: 'menu', … }`). Un tipo propio que extendía
  `BreadcrumbItem` ahora extiende `BreadcrumbLink`.
- `Button loading` ya no pone `disabled`: pone `aria-disabled` y el clic no hace nada. Un
  test que miraba el atributo `disabled` mira `aria-disabled` (`toBeDisabled()` de vitest
  acepta los dos), o que el clic no llame nada.
- `LoadingState`: el `role="status"` está en el contenedor y el `Spinner` de adentro es
  decorativo; un test que buscaba el status en el spinner lo busca en el `LoadingState`.
- `toast.error` ya no se va solo ni lleva `role="alert"`: hay una sola región
  `aria-live="polite"` para todos los avisos. Los de éxito e info se van a los 4 s.
- `Modal onClose` también se llama cuando el navegador cierra el `<dialog>` por su cuenta
  (un segundo Escape): el dueño tiene que aceptar cerrarlo ahí también, o usar
  `dismissible={false}` (modales de un solo uso).
- MSW: una ruta sin mock contesta `noRoute()` (404 `no_route`), como el backend, y el
  repositorio la lee como `UnexpectedError` (una ruta mal armada); `notFound()` (404
  `not_found`) es un recurso que no existe.
- Tema: nuevo `text.faint` (texto atenuado legible; lo usa `Mono tone="disabled"`) y
  `dangerHover` es `red-700`; el color `red-550` de la paleta ya no existe.
- Tests de viewModels: el `QueryClient` del harness sale de
  `createQueryClient({ retry: false })` (`@/shared/data/query`), con las opciones de la app
  (entre ellas `notifyOnChangeProps: 'all'`).
- `UnavailableError` de un 5xx o un timeout tiene `reason`, y su texto en `describeError`
  cambió (ver [HTTP y errores](#http-y-errores)); `shouldRetry` no reintenta timeouts.

### Paralelismo de F1

Cada tarea F1 crea o modifica **solo**:

- `src/modules/<su módulo>/`;
- sus rutas (tabla de abajo) y lo que cuelgue de ellas;
- `src/mocks/handlers/<módulo>.ts` y `src/mocks/db/<módulo>.ts` (ya existen, vacíos, y
  `handlers/index.ts` ya los importa);
- `messages/<módulo>/es.json` y `messages/<módulo>/en.json` (ya existen y ya están en
  `project.inlang`).

`package.json` ya trae lo de F1: CodeMirror 6 (`codemirror`, `@codemirror/state`,
`@codemirror/view`, `@codemirror/language`, `@codemirror/commands`, `@codemirror/lang-yaml`,
`@lezer/highlight`) y `qrcode`. Si una tarea necesita
cambiar `shared/`, `package.json` o una ruta ajena, se detiene y lo reporta.

### Rutas

| URL                                                             | Archivo (`src/routes/…`)                                 | Dueño                                                                          |
| --------------------------------------------------------------- | -------------------------------------------------------- | ------------------------------------------------------------------------------ |
| `/login`, `/setup?token=`, `/invite/[token]`, `/invite`, `/2fa` | `login/`, `setup/`, `invite/[token]/`, `invite/`, `2fa/` | F0 (`auth`)                                                                    |
| `/`                                                             | `(app)/+page.svelte`                                     | F1-catalog (Proyectos)                                                         |
| `/projects/[p]/stages/[s]/apps/[a]` (layout)                    | `(app)/projects/[p]/stages/[s]/apps/[a]/+layout.svelte`  | F0 lo arma; F1-catalog cambia los items del breadcrumb por selectores (sin h1) |
| `/projects/[p]/stages/[s]/apps/[a]` (Resumen)                   | `…/apps/[a]/+page.svelte`                                | F1-deploy-ingress                                                              |
| `…/apps/[a]/configuracion`                                      | `…/apps/[a]/configuracion/+page.svelte`                  | F1-catalog (formulario o editor de compose)                                    |
| `…/apps/[a]/variables`                                          | `…/apps/[a]/variables/+page.svelte`                      | F1-secrets                                                                     |
| `…/apps/[a]/dominios`                                           | `…/apps/[a]/dominios/+page.svelte`                       | F1-deploy-ingress                                                              |
| `…/apps/[a]/deploys`                                            | `…/apps/[a]/deploys/+page.svelte`                        | F1-deploy-ingress                                                              |
| `…/apps/[a]/logs`                                               | `…/apps/[a]/logs/+page.svelte`                           | F1-deploy-ingress                                                              |
| `/projects/[p]/stages/[s]/variables`                            | `(app)/projects/[p]/stages/[s]/variables/+page.svelte`   | F1-secrets                                                                     |
| `/ajustes`                                                      | `(app)/ajustes/+page.svelte`                             | F1-identity-admin                                                              |
| `/ajustes/tokens`                                               | `(app)/ajustes/tokens/+page.svelte`                      | F1-identity-admin                                                              |

- Todo lo de `(app)` pasa por el guard de `(app)/+layout.ts`: sin sesión va a `/login`, con
  `needsTotpSetup` a `/2fa`. Las páginas reciben `params` tipados (`let { params }: PageProps = $props()`).
- Los placeholders de F0 (`EmptyState` "En construcción") se reemplazan enteros por
  `<XScreen … />` del barrel del módulo.
- **Quién dibuja el header de una app.** El layout `apps/[a]/+layout.svelte` solo llena el
  `<ShellHeader>` (breadcrumb y tabs) y no pone nada en `main`: ni h1 ni encabezado. Cada
  página dibuja su propio encabezado, como en el diseño:
    - Resumen (F1-deploy-ingress) tiene el header grande: nombre de la app, kind, imagen,
      cambios pendientes y las acciones de deploy;
    - Configuración, Variables, Dominios, Deploys y Logs tienen el h1 de su sección
      ("Variables", "Deploys").
- F1-catalog exporta desde su barrel la query del resumen de una app (nombre, kind, imagen,
  cambios pendientes) y un componente chico con esos datos, para que las páginas los usen
  sin entrar a las tripas de `catalog`. En el layout cambia los items del breadcrumb por
  selectores (`{ kind: 'menu', … }`, ver `Breadcrumb`).
- La página de variables del stage pone su breadcrumb con `<ShellHeader>` igual:
  `<StageBreadcrumb>` del barrel de `catalog`, con los selectores de proyecto y stage (cambiar
  de uno lleva a las variables del otro stage).
- **Los snippets de `<ShellHeader>` pueden ver `params` vacíos.** Se pintan en el header
  del AppShell, fuera de su ruta. Al navegar afuera, SvelteKit cambia los `params` y
  desmonta la ruta en el mismo cambio, y el snippet llega a evaluarse una vez con los params
  de la ruta nueva (a `/` no hay `p`, `s` ni `a`; a las variables del stage no hay `a`):
    - lo que tire esa evaluación (un `params.p.charAt(0)`, un `resolve()` sin `a`) lo ataja
      el AppShell con un `<svelte:boundary>`, sin aviso: en el mismo cambio la ruta suelta
      el slot. Un error con la ruta todavía montada sí va a la consola y deja el logo;
    - lo que NO se puede atajar son los efectos: un componente del snippet que hace una
      query con los params tiene que llevar `enabled: Boolean(p && s)` (o recibir los datos
      ya cargados), o sale un pedido a `/projects/undefined`;
    - `e2e/shell.e2e.ts` recorre app → Proyectos, app → variables del stage y los tabs: si
      tu ruta tiene un `<ShellHeader>`, sumala ahí.

### i18n: prefijo por módulo

- Cada clave empieza con el nombre del módulo: `catalog_…`, `secrets_…`, `identity_…`,
  `admin_…`, `deploy_…`, `ingress_…`, `auth_…`. Las de F0 son `common_…`, `nav_…`,
  `pagination_…` y `not_found_…` (en `messages/es.json` / `en.json`): se pueden usar, no se
  agregan. Si falta algo genérico, va con el prefijo del módulo.
- Las claves de un módulo van en `messages/<módulo>/{es,en}.json` (`project.inlang` mezcla
  todos los archivos: el prefijo evita choques). Las dos lenguas, siempre.
- Después de tocar mensajes, `pnpm codegen` (también lo corre `pnpm check`).
- Claves útiles de F0: `common_save`, `common_cancel`, `common_delete`, `common_edit`,
  `common_create`, `common_back`, `common_close`, `common_retry`, `common_loading`,
  `common_search`, `common_options`, `common_optional`, `common_copy`, `common_copied`,
  `common_required`, `common_error_generic`, `common_unavailable`, `common_load_more`,
  `common_no_more`, `common_forbidden` y `common_rate_limited`. Para el texto de un error cualquiera, mejor `describeError(error)`.

### Tema

`import { theme } from '@/shared/ui/themes/theme.css'`, solo en `styles.css.ts`:

| Token                       | Qué                                                                                                                                                                                                                                                                       |
| --------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `colors.background.*`       | `page` #0A0A0B, `surface` #101012 (tarjetas, filas), `sunken` #0D0D0F (pies, filas atenuadas), `hover` #141416, `raised` #18181B, `divider` #1A1A1D, `inverse`/`inverseHover` (botón primario), `danger` #C7343A/`dangerHover` #B32D33 (los dos ≥ 4.5:1 con texto blanco) |
| `colors.borders.*`          | `subtle` #222225, `control` #2E2E33, `strong` #3A3A40, `focus` #6E6E77, `outline` #EDEDED, `danger`                                                                                                                                                                       |
| `colors.text.*`             | `primary` #EDEDED, `secondary` #A1A1A8, `tertiary` #83838C, `faint` #808089 (lo más tenue que se lee: ≥ 4.5:1; lo usa `Mono tone="disabled"`), `disabled` #6E6E77 (controles deshabilitados, no texto que hay que leer), `inverse`, `strong`, `separator`, `onDanger`     |
| `colors.status.*`           | `ok`, `warn`/`warnText`/`warnBorder`/`warnSurface`, `error`/`errorText`/`errorBorder`/`errorSurface`/`errorDivider`/`errorSunken`, `idle`                                                                                                                                 |
| `space[n]`                  | en px, la clave es el valor: 0, 2, 4, 6, 8, 10, 12, 14, 16, 20, 24, 28, 32, 40, 48, 80                                                                                                                                                                                    |
| `typography.fontSize[n]`    | 11, 12, 13, 14 (base), 15, 16, 18, 20, 24 (h1), 28 (nombre de app)                                                                                                                                                                                                        |
| `typography.fontFamily`     | `sans` (Geist), `mono` (Geist Mono)                                                                                                                                                                                                                                       |
| `typography.fontWeight`     | `regular` 400, `medium` 500, `semibold` 600                                                                                                                                                                                                                               |
| `typography.letterSpacing`  | `tighter` -0.03em (h1), `tight` -0.02em, `snug` -0.01em                                                                                                                                                                                                                   |
| `radius`                    | `xs` 4 (badge), `sm` 5, `md` 6 (controles), `lg` 8 (tarjetas, avisos), `xl` 12 (diálogos), `full`                                                                                                                                                                         |
| `control`                   | alturas: `xs` 28, `sm` 32, `md` 36, `lg` 40, `xl` 48, `tab` 44, `badge` 20, `badgeLg` 24                                                                                                                                                                                  |
| `shadows`                   | `focusRing`, `modal`, `menu`                                                                                                                                                                                                                                              |
| `layout`                    | `maxWidth` 1248px, `gutter` clamp(16px, 3vw, 24px), `authWidth` 360px                                                                                                                                                                                                     |
| `zIndex`, `transition.fast` |                                                                                                                                                                                                                                                                           |

Geist y Geist Mono están empaquetadas (`themes/fonts.ts`, `@fontsource-variable/*`): nada
de Google Fonts ni CDN. El foco con teclado es el global (`2px solid` claro, offset 2px);
los controles muestran el suyo (borde `focus` + `focusRing`).

### Componentes (`@/shared/ui`)

Todos se importan del barrel: `import { Button, Card } from '@/shared/ui'`. Los que reciben
un `href` lo esperan **ya resuelto** con `resolve()`.

**Átomos**

| Componente   | Props                                                                                                                                                                                                                                                                                                                                  |
| ------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Button`     | `variant` `primary`·`secondary`·`ghost`·`danger` (primary) · `size` `xs` 28·`sm` 32·`md` 36·`lg` 40 (md) · `loading` (spinner, `aria-disabled` y el clic no hace nada; NO es `disabled`, así no pierde el foco ni el nombre) · `iconOnly` (pasá `aria-label`) · `fullWidth` · atributos de `<button>` (`type` = `button`) · `children` |
| `LinkButton` | `href` · `variant` · `size` · `iconOnly` · `fullWidth` · atributos de `<a>` · `children`. Para una acción que navega ("Desplegar" → Deploys)                                                                                                                                                                                           |
| `Input`      | `inputSize` `sm` 32·`md` 36·`lg` 40 · `mono` (Geist Mono 13) · `bind:value` · atributos de `<input>`                                                                                                                                                                                                                                   |
| `Select`     | `inputSize` · `bind:value` · `children` (`<option>`) · atributos de `<select>`                                                                                                                                                                                                                                                         |
| `Textarea`   | `mono` · `bind:value` · atributos de `<textarea>`                                                                                                                                                                                                                                                                                      |
| `TextLink`   | `href` (ya resuelto; con query, `withQuery(resolve(…), {…})`) · atributos de `<a>` · `children`. Un link dentro de un texto, con la clase `textLink`                                                                                                                                                                                   |
| `Checkbox`   | `label` o `children` · `bind:checked` · atributos de `<input>`                                                                                                                                                                                                                                                                         |
| `Switch`     | `bind:checked` · atributos de `<input>` (`role="switch"`). Va dentro del `<label>` de la pantalla, o con `aria-label`                                                                                                                                                                                                                  |
| `Badge`      | `tone` `neutral`·`ok`·`warn`·`error`·`mono` ("compose")·`strong` ("nuevo") · `shape` `tag`·`pill` · `size` `sm` 20·`md` 24 · `dot` (`true` o un `StatusTone`) · `children`                                                                                                                                                             |
| `StatusDot`  | `status` `ok`·`warn`·`bad`·`idle` (anillo) · `size` 6·8 · `pulse` ("en vivo") · `label` (si el punto es la única pista)                                                                                                                                                                                                                |
| `Mono`       | `tone` `inherit`·`primary`·`secondary`·`tertiary`·`disabled` · `size` `inherit`·11·12·13·14 · `truncate` · `title` · `children`. Para slugs, imágenes, digests, redes, claves                                                                                                                                                          |
| `Icon`       | `name` (`plus`, `search`, `more`, `chevronRight`, `chevronDown`, `chevronUpDown`, `arrowLeft`, `arrowUpRight`, `copy`, `check`, `x`, `lock`, `download`, `upload`, `refresh`, `trash`, `logOut`, `shield`, `globe`, `alert`, `clock`, `external`) · `size` (16) · `label`                                                              |
| `Avatar`     | `name` · `size` `sm` 28·`md` 32                                                                                                                                                                                                                                                                                                        |
| `Logo`       | `size` (22)                                                                                                                                                                                                                                                                                                                            |
| `Spinner`    | `size` `sm`·`md`·`lg` · `label` · `decorative` (sin `role="status"`: cuando otro elemento ya anuncia la carga)                                                                                                                                                                                                                         |
| `ShadowHost` | `mount(container, root)` (devuelve la limpieza; estable, del viewModel) · `class` y atributos de `<div>` (el borde y el tamaño van en el host). Un shadow root para librerías que inyectan `<style>` (CodeMirror): ver [CSP](#csp)                                                                                                     |

**Moléculas**

| Componente         | Props                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| ------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Field`            | `label` · `hint` · `error` · `optional` ("· opcional") · `children(a11y)`: `{#snippet children(a11y)}<Input {...a11y} />{/snippet}`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `SearchInput`      | `label` (el `aria-label`) · `inputSize` · `bind:value` · atributos de `<input>`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `CodeInput`        | `value` · `label` · `length` (6) · `disabled` · `focusOnMount` · `oninput(value)` · `oncomplete(value)` · el `a11y` de `Field`. Se queda solo con los dígitos: pegar "123 456" funciona                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `Banner`           | `tone` `info`·`warn`·`error` · `title` (en 500) · `body` (gris) · `dot` · `action` (snippet: botones a la derecha)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `Card`             | `title` · `description` · `headingLevel` 2·3 · `tone` `default`·`danger`·`highlight` · `flush` (encabezado en franja y contenido al borde, para tablas o `<dl>`) · `aside` (snippet, solo `flush`) · `note` (texto del pie) · `footer` (snippet: botones del pie, `size="sm"`) · `id` · `children`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `List` / `ListRow` | `List`: `label` · `children`. `ListRow`: `href` (la fila entera es un link) · `muted` · `height` `sm` 44·`md` 52·`lg` 56 · `class` · `children`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `Table`            | `framed` (true; `false` dentro de una `Card flush`) · `minWidth` · `caption` · `children` (`<thead>`/`<tbody>` planos). Clases: `cellEnd`, `cellActions`, `cellMono`, `rowMuted`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `Tabs`             | `label` · `items: NavItem[]` (`{ href, label, active }`). La nav subrayada del header                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `SubNav`           | `label` · `items: NavItem[]` (el `active` lo pone quien arma los items: en Ajustes y Configuración, el hash de la URL). `SubNavLayout`: lo mismo + `children` (nav de 180 px a la izquierda, contenido a la derecha: Ajustes, Configuración). Con la nav al costado (el layout desde 820 px) es sticky y, si no entra en la pantalla, scrollea sola; arriba del contenido (teléfono) se va con la página. El layout es el contenedor de esa query (`container-type: inline-size`): va donde un padre le dé el ancho, no adentro de algo que se ajusta al contenido                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `Breadcrumb`       | `label` · `homeHref` · `homeLabel` · `items: BreadcrumbItem[]`: un link `{ label, href?, initial? }` (`kind: 'link'` opcional) o un selector `{ kind: 'menu', label, ariaLabel, items: MenuEntry[], initial? }` (el botón con chevron del diseño que abre un `Menu`: "auditfy ⌄" → los otros proyectos). El último item es la página actual                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `Menu`             | `label` · `items: MenuEntry[]` (`{ kind: 'action', id, label, onSelect, icon?, tone?: 'danger', disabled? }`, `{ kind: 'link', id, label, href, icon?, current? }`, `{ kind: 'separator', id }`; `current`: el lugar donde se está, con `aria-current="page"` y la marca ✓, como el proyecto actual en un selector del breadcrumb. Si algún item tiene ícono, los que no tienen dejan el hueco y todos los textos se alinean) · `variant` `ghost` (los ⋯ de una fila) · `bare` · `trigger` (snippet) · `triggerClass` (reemplaza la clase del `variant`: el botón queda con la forma que le dé esa clase, como el selector del breadcrumb) · `align` `start`·`end` · `current` (el `aria-current` del botón) · `header` (snippet). El panel va en el top layer (`popover`): no lo recorta el `overflow` de una `List`, `Card` o `Table`; se da vuelta hacia arriba si no entra abajo y no se sale de la pantalla; si no entra de ningún lado (20 proyectos en un teléfono), va del lado con más lugar y scrollea adentro. Elegir una acción devuelve el foco al botón ANTES de `onSelect`: un `ConfirmDialog` abierto desde un item se lo devuelve al botón al cerrarse |
| `Modal`            | `open` · `title` · `description` · `size` `sm`·`md`·`lg` · `onClose` (Escape, clic en el fondo, o cualquier cierre del `<dialog>`) · `dismissible` (true) · `children` · `footer` (snippet). **Modales de un solo uso** (la URL de una invitación, un token nuevo, lo que no se vuelve a mostrar): `dismissible={false}`. Ni Escape ni el fondo lo cierran, y si el navegador lo cierra igual (Chrome deja frenar un solo Escape sin activación del usuario) se reabre con el foco donde estaba; solo lo cierra un botón del dueño que pone `open` en false. `<dialog>` nativo: foco atrapado y devuelto al que lo abrió; si ese elemento ya no está (la fila que se borró), al h1 de `main` o a `main`. Una selección de texto que termina sobre el fondo no lo cierra. Nunca pasa del alto de la pantalla: con contenido largo scrollea el cuerpo y el pie queda a la vista                                                                                                                                                                                                                                                                                           |
| `ConfirmDialog`    | `open` · `title` · `body` · `confirmLabel` · `cancelLabel` · `loading` · `tone` `danger`·`primary` · `error` (un `Banner` de error adentro del diálogo: el `not_empty` de borrar un proyecto) · `children` (contenido extra bajo el `body`) · `onConfirm` · `onCancel`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `CopyButton`       | `value` · `label` · `target` (id del elemento que muestra el valor) · `iconOnly` · `variant` · `size`. Si no se puede copiar (HTTP sin `localhost`, permiso denegado) dice "No se pudo copiar", muestra un toast y selecciona el texto de `target` para copiarlo a mano: pasalo siempre que el valor esté en pantalla. Adentro de un `Modal` también funciona (el plan B copia desde el `<dialog>`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `SegmentedControl` | `label` o `labelledBy` · `name` · `options: ChoiceOption<T>[]` (`{ value, label, disabled? }`) · `value` · `disabled` · `onChange`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `RadioGroup`       | lo mismo que `SegmentedControl` + `direction` `row`·`column`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `EmptyState`       | `title` · `body` · `icon` · `action` (snippet)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `ErrorState`       | `title` · `body` · `retryLabel` · `onRetry`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `LoadingState`     | `label` (se anuncia una sola vez)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `PageHeader`       | `title` (el h1) · `subtitle` · `meta` ("10 apps") · `titleSize` `md` 24·`lg` 28 · `badges` · `details` · `actions` (snippets)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `Pagination`       | `page` · `totalPages` · `label` · `prevLabel` · `nextLabel` · `onPageChange`. Para listas con total (`Paginated<T>`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `LoadMore`         | `hasMore` · `loading` · `onLoadMore` · `label` ("Cargar más") · `doneLabel` (lo que se ve al final; sin esto, nada). La variante por cursor de `Pagination`: ver [Paginación](#paginación)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `Toaster`          | va en el layout raíz; se usa con `toast.success/error/info(title, description?)` de `@/shared/ui/stores`. Los avisos se van a los 4 s (en pausa con el puntero o el foco encima); `toast.error` se queda hasta que se cierra. Una sola región `aria-live` polite                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `LocaleSwitcher`   | `label`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |

**Organismos**

| Componente    | Props                                                                                                                                                                                                                      |
| ------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `AppShell`    | El marco con sesión. No se usa directo: lo monta `SessionShell` (barrel de `auth`) en `(app)/+layout.svelte`                                                                                                               |
| `ShellHeader` | `breadcrumb` · `nav` (snippets). No dibuja nada donde está: mientras está montado, reemplaza el logo y/o la nav del header del AppShell. Sus snippets pueden ver `params` vacíos al salir de la ruta (ver [Rutas](#rutas)) |
| `AuthLayout`  | `appName` · `host` · `width` `sm` 360·`md` 440 · `children`. Las pantallas de acceso                                                                                                                                       |

Clases y helpers: `srOnly`, `textLink` (link dentro de un texto), `truncate`, `firstError`,
`formatDate`, `formatDateTime`, `formatNumber`, `formatRelative`, `withQuery(href, query)` (ver
[Convenciones](#convenciones)) y `describeError(error)`
(el texto para el usuario de cualquier error de `@/shared/domain`: "muelle no acepta cambios
desde esta dirección", "No se pudo contactar a muelle", …; los errores
propios de un módulo los traduce el módulo y caen acá para el resto).

### Paginación

- **Con total** (`Paginated<T>` de `@/shared/domain`, páginas numeradas): `<Pagination>`.
- **Por cursor** (`/audit?before=`): `CursorPage<T>` de `@/shared/domain`
  (`{ items, nextCursor: string | null }`, con `emptyCursorPage`, `hasNextPage` y
  `flattenCursorPages`). El repositorio devuelve una página; el viewModel usa
  `createInfiniteQuery` con `initialPageParam: null` y
  `getNextPageParam: (page) => page.nextCursor ?? undefined`, junta
  las páginas con `flattenCursorPages(query.data?.pages)` y pone al pie
  `<LoadMore hasMore={query.hasNextPage} loading={query.isFetchingNextPage} onLoadMore={() => query.fetchNextPage()} />`.

### HTTP y errores

- `http.get/post/put/patch/delete(path, { query, body, signal, timeoutMs })` de
  `@/shared/data/http/client`, con `path` sin `/api/v1` (`http.get('/projects')`). Manda la
  cookie (`credentials: 'same-origin'`) y JSON; un 204 devuelve `null`.
- Toda petición se corta a los 30 s (`timeoutMs` para otro valor; un stream SSE no pasa por
  `http`). El corte llega como `UnavailableError` con `reason: 'timeout'`; un `signal`
  propio que se aborta sigue siendo un `AbortError`.
- El backend siempre responde `{"error":{"code","message"}}`: llega como
  `ApiError(status, code, message)`.
- Un 401 en una ruta con sesión limpia el cache y manda a `/login`; un 403
  `totp_setup_required`, a `/2fa`. No hay que manejarlos en las pantallas.
  (`redirectOn401: false` es solo para los endpoints de acceso de `auth`.)
- Los repositorios traducen con `withDomainErrors(() => ds.algo(), MAP)` (o `toDomainError`)
  de `@/shared/data/http/domainErrors`, con un `DomainErrorMap`:
    - `codes`: errores propios por `error.code` (`not_empty` → `ProjectNotEmpty`). Ganan siempre.
    - `notFound`: el error para un 404 (`ProjectNotFound`).
    - `fields`: nombre de dominio de los campos de un 422 (`{ deploy_timeout_s: 'deployTimeoutSeconds' }`).
      El 422 `invalid` trae `"<campo>: <motivo>"`; sale como `ValidationError` con `field` y
      `message` = el motivo. El viewModel lo pone en el campo del formulario.
    - Siempre, en cualquier endpoint: 403 `bad_origin` → `BadOriginError` y 400
      `ambiguous_session` → `AmbiguousSessionError` (dos cookies de sesión). `describeError`
      les da el texto que dice qué hacer. `bad_origin` es una escritura desde una dirección
      que no es la del panel (contrato §0.1): la de Ajustes, `https://<panel_domain>` con el
      puerto HTTPS si no es 443, o `MUELLE_PANEL_ORIGIN` si alguien la puso a mano (detrás de
      otro proxy, o en desarrollo). install ya no escribe `MUELLE_PANEL_ORIGIN`
      (`MUELLE_PANEL_DOMAIN`, §5.2): el texto la nombra solo para el caso del otro proxy.
    - Después de cambiar el puerto HTTPS desde la dirección en que muelle-caddy servía el
      panel, el backend ya no acepta escrituras con ese `Origin` (§0.1 y §3.5). Ajustes llama
      `holdWrites(<origen nuevo>)` del cliente: desde esa página ningún `POST`, `PUT`,
      `PATCH` ni `DELETE` sale, y tira `PanelMovedError` (un `BadOriginError` con `origin`;
      `describeError` dice a dónde seguir y, si allá no responde, que recargue esta página).
      Las lecturas siguen. `releaseWrites()` las deja salir otra vez (muelle volvió a los
      puertos anteriores). El freno es de la página, no del viewModel que lo puso:
      `heldWritesOrigin()` dice a dónde se mudó, y Ajustes lo suelta al ver la vuelta atrás
      de esa dirección aunque sea otra visita a la pantalla. Fuera de Ajustes nadie lo
      suelta: las otras pantallas no siguen `GET /proxy` (Dominios y Deploys lo piden, pero
      `GET /proxy` no trae el dominio del panel para armar la dirección que falló), y por eso
      el texto pide recargar, que suelta el freno.
    - Sin mapa: 401 `UnauthorizedError`, 403 `ForbiddenError`, 404 `NotFoundError`,
      409 `ConflictError`, 423/429 `RateLimitedError`, 5xx, red y timeout `UnavailableError`,
      el resto `UnexpectedError`. Una respuesta con otra forma es `ContractError`
      (`parseOrThrow`).
- `shouldRetry` (el `retry` de las queries) reintenta dos veces solo lo que puede andar
  la próxima: red y 5xx. Un 4xx, un `ContractError`, una ruta que no existe o un timeout
  (ya esperó 30 s), no.
- `UnavailableError.reason` dice por qué: `network` (no hubo respuesta), `server` (un 5xx)
  o `timeout`. `describeError` solo pide "revisa la conexión" con `network`; un 5xx dice
  "muelle respondió con un error".
- Las queries avisan cualquier cambio del resultado (`notifyOnChangeProps: 'all'` en
  `createQueryClient`). Con el modo por defecto de TanStack solo avisan lo que ya se leyó, y
  un viewModel que mira `data` antes que `error` se quedaba cargando para siempre.

### Las fases 2, 3 y 5: el backend entero

Las Fases 2 (deploys, status, logs, rollback, registries y los `services` de una app), 3
(dominios, `GET /proxy` y, en Ajustes, Cloudflare, IP pública, email ACME, puertos e IPs
permitidas) y 5 (install y el panel embebido) están en este mismo árbol que el panel. La
única fuente para §0–§5 es el contrato,
`docs/superpowers/specs/2026-10-10-contrato-http-fases-2-3.md`, resumido en
`src/modules/{deploy,ingress}/README.md`, sin diferencias (también los certificados de
Cloudflare, Origin CA, de §3.7: el diseño es
`docs/superpowers/specs/2026-10-21-origin-ca-design.md`, §11). `pnpm test:e2e:real` recorre el
panel contra ese backend (ver [E2E contra muelle](#e2e-contra-muelle)).

El panel va embebido en el binario que lo sirve (contrato §5.5): las rutas del panel y las
del backend llegan juntas, y el panel no tiene caminos para un backend más viejo (ni
`<ComingSoon>`, ni campos opcionales "por si falta la fase"). Un 404 `no_route` o un 405
`method_not_allowed` es una ruta mal armada del panel: `toDomainError` lo convierte en
`UnexpectedError` (no en el `notFound` del módulo, que diría que el recurso no existe), y la
pantalla muestra un error. `shouldRetry` no lo reintenta. Con `pnpm dev:mock`, una ruta de
`/api/v1` sin handler contesta lo mismo (el `http.all` del final de `handlers/index.ts`).

Los **campos** que las Fases 2 y 3 sumaron a endpoints que ya existían son obligatorios en
los DTO, como en el contrato: una respuesta sin ellos es un `ContractError`. Son los de
proxy y DNS de `GET /settings` (§4: todas las claves, siempre, también
`cloudflare_origin_ca`; `Settings.proxy` y `panelAccess` nunca son null), `cert_source` y
`origin_ca` de un dominio y `panel_certificate` de `GET /proxy` (§3.1, §3.3 y §3.7: la clave
va siempre, el objeto puede ser null), los `services` de una app de catalog (§2.12: `[]` es un
compose guardado que no se puede leer, y ahí Dominios sugiere solo los destinos que ya usan
sus dominios), `requested_images` y `routes_warning` de un deploy (§2.1: lo fijado en un
digest sale solo de `requested_images`, `pinnedImages`), `ports_rollback` de `GET /proxy`
(§3.3, null si no hubo vuelta atrás), `created_by` de un token (un username, o null si se
borró el usuario) y el `actor_name` de cada evento de `GET /audit` (§4.2). La Auditoría
muestra el nombre que trae el evento (el username, el del token; `system` es "sistema"),
copiado al escribirlo, así sigue aunque se borre el usuario o el token. Sin nombre (`""`: un
evento de antes de la migración 0004 cuyo autor ya no estaba), el tipo y el id ("token
#9"). `caddy_image` no es un ajuste (§4.1: solo lo fija install): Ajustes no lo muestra ni
lo manda, y el mock lo rechaza.

**Lo que un módulo usa de otro** (siempre por el barrel):

- Proyectos (`catalog`) no importa `deploy`: la ruta `(app)/+page.svelte` le pasa
  `appDeploy`, un `AppRowDeployFactory` que crea un `AppDeployState` (barrel de `deploy`) por
  fila. Cada fila lo crea al montarse y le pasa `enabled`, que se prende una sola vez cuando
  la fila entra en la pantalla (`IntersectionObserver`, `whenVisible`): el estado se pide una
  vez por fila (contrato §2.8) y las filas que no se ven no piden nada. Usa el cache de
  `appStatusQuery` (5 s) y `deploysQuery` (10 s), el mismo del Resumen y Deploys, sin
  refrescos ni reintentos. Si falla, la fila dice "—" y la imagen es la
  del spec (o los servicios del compose).
- **Pendiente:** el "Variables · N" de cada stage en Proyectos (el slot `stageVariablesCount`
  de `ProjectsScreen` y la clave `catalog_stage_variables_count`) no está conectado. No hay
  un endpoint que cuente las variables de un stage, y pedir `GET …/stages/{s}/env` por cada
  stage traería al cache los valores de las que no son secretas solo para contarlas. Queda un
  `TODO(stageVariablesCount)` en `(app)/+page.svelte`: se conecta cuando el backend dé el
  número.
- Dominios pide los destinos al catálogo: `appTargetsQuery(ref)` de `catalog` (un
  `servicio:puerto` por cada puerto que declara cada servicio, comparte el cache de
  `appQuery`).
- Resumen y Deploys piden la app al catálogo (`appQuery` de `catalog`): su `kind` decide si
  el diálogo de deploy manda `image` o `images`, y sus `services` son los campos del diálogo
  (contra esos valida el backend). Si el catálogo no carga, salen de Swarm, como antes.
  Resumen también saca de ahí "Cambios sin desplegar" (con link a Configuración) y el
  healthcheck del spec, y vuelve a pedir la app una vez cuando el deploy actual es más nuevo
  que su `last_deployed_at` (un deploy exitoso lo adelanta en la misma transacción). Deploys,
  cuando un deploy que sigue en vivo termina `succeeded`, marca viejos el árbol y la app en
  el cache del catálogo (`refetchType: 'none'`): Proyectos y el Resumen dejan de mostrar los
  cambios pendientes al volver, sin esperar a que venza el cache. Un test de viewModel de
  esas pantallas mockea también `@/modules/catalog/container`.
- Deploys pide el estado del proxy a `ingress` (`proxyQuery` y `proxyNotice` del barrel)
  solo cuando el deploy elegido trae `routes_warning` (el paso 7 no pudo actualizar Caddy,
  contrato §2.1): la línea del log no trae el error (§2.11) y el motivo sale de
  `GET /proxy`. Decide por el campo, nunca por el texto del log (que no es un contrato). Su
  test de viewModel mockea también `@/modules/ingress/container`.
- Variables (`secrets`), después de guardar un cambio de verdad, marca viejo todo
  `catalogKeys.all` con `refetchType: 'none'`: el backend dejó con cambios pendientes a la
  app o a todas las apps del stage (§5.2 regla 4 del diseño), y Proyectos, el Resumen y
  Configuración lo vuelven a pedir al mostrarse (no antes: el árbol del breadcrumb, que está
  a la vista, no muestra la marca). Un guardado que no cambió nada no lo toca.
- Ajustes, después de guardar la IP pública, el email ACME, el token, los puertos o la lista
  de IPs permitidas, invalida `ingressKeys.all`: los `records` de cada dominio se calculan
  con los ajustes de ahora, y todos regeneran (o recrean) la configuración de Caddy, así
  que el estado del proxy puede cambiar (contrato §3.5). Guardar solo el 2FA no la toca.
  Ajustes pide `GET /proxy` al abrirse (`proxyFollowQuery`, `proxyNotice` y el tipo
  `PortsRollback` del barrel de `ingress`) y, después de cambiar los puertos, lo sigue hasta
  que dice cómo terminó, aunque falle (2 min como mucho: pasado el plazo, un pedido que
  falla corta, aunque la última respuesta guardada diga `recreating`). Antes, `resetQueries` saca lo que
  había en el cache: el `PATCH` limpió `ports_rollback`. Un cambio que falla (un `422`,
  `PanelMovedError`) no guardó nada: el seguimiento del anterior sigue.
    - Los certificados de Cloudflare (Origin CA, contrato §3.7): el switch de la tarjeta de
      Cloudflare guarda `cloudflare_origin_ca` y, si sale bien, invalida `ingressKeys.all`
      (los dominios recalculan `cert_source` y `origin_ca`, y cambia `GET /proxy`). Con la
      opción encendida, la tarjeta lee `panel_certificate` de ese mismo `GET /proxy` (con
      `originCaNotice` y los tipos `PanelCertificate` y `OriginCaState` del barrel) para la
      línea "Panel: …" y el aviso de su falla (sin Reintentar: el del panel se reintenta
      solo), y `proxyFollowQuery` lo vuelve a pedir cada 10 s mientras el del panel dice
      `issuing`.
    - Si el panel sigue en el mismo origen (solo cambió el HTTP, o se entró por otra
      dirección que la de Caddy), dice "Recreando muelle-caddy…" y después si quedó
      escuchando o el error. Pasados los 2 min, si `GET /proxy` no respondió desde el cambio
      o su último pedido falló, que no se pudo confirmar y que recargue en un momento
      (`recreation.kind === 'unknown'`); si responde y todavía dice `recreating`, sigue en
      "Recreando muelle-caddy…" (se sigue preguntando hasta que diga cómo terminó).
    - Si cambió el HTTPS y se entró por la dirección de Caddy, esa dirección deja de
      responder: el aviso lleva a la nueva, no salen más escrituras (`holdWrites`) y se
      sigue preguntando solo por si muelle vuelve atrás (la dirección vuelve a responder).
    - Con `ports_rollback` (muelle-caddy no arrancó con los puertos nuevos y muelle volvió a
      los anteriores, contrato §3.5) la tarjeta lo dice con el motivo y cuándo (`GET /proxy`
      lo trae hasta el próximo cambio de puertos o un reinicio de muelle; cerrado, el aviso de
      esa vuelta no vuelve a salir, pero lo demás se atiende igual), saca el aviso de la
      dirección que falló (también uno guardado, al recargar), deja salir las escrituras
      frenadas por la mudanza a esa dirección (también si se salió de Ajustes y se volvió),
      pone los puertos de antes en los campos (si el cambio fue desde acá, o si los ajustes
      del cache todavía son los que fallaron) y, si el panel está en una dirección de Caddy,
      dice que sigue en la anterior (`https://<panel_domain>[:<puerto anterior>]`); desde la
      que falló, ofrece volver. Entrando por otro proxy, una IP o `MUELLE_PANEL_ORIGIN`, no
      nombra ni ofrece la de Caddy: esa dirección nunca se usó. El panel no
      puede saber si la dirección nueva responde (la CSP no deja pedir a otro origen): lo
      sabe por `ports_rollback`, en una dirección que sí responde. Por eso el aviso de la
      dirección nueva dice qué hacer si no responde: volver a la anterior.
- En MSW: `db/ingress.ts` lee el token, las IP, el dominio del panel y los puertos de
  `db/admin.ts` (un `PATCH` de los puertos llama a `ingressDb.recreateProxy`, e
  `ingressDb.settle` termina la recreación: con el 9443, muelle vuelve a los puertos
  anteriores y escribe `ports_rollback`; el 8081 lo rechaza el `PATCH` con `422`, salvo sin
  dominio del panel: con el proxy deshabilitado no se prueba nada), y el
  `DELETE` de una app (`handlers/catalog.ts`) mira los dominios de `db/ingress.ts` con
  `ingressDb.list` (una app con dominios es `409 not_empty`). `list` siembra los tres
  dominios del diseño la primera vez que se piden los de una app, así que una app del mock
  los tiene aunque nadie haya abierto Dominios; una app creada en el mock empieza sin
  ninguno (`ingressDb.startEmpty`). Con `cloudflare_origin_ca` encendido en `db/admin.ts`,
  `db/ingress.ts` calcula `cert_source` y `origin_ca` de cada dominio y del panel (las reglas,
  con `falla-`, `gris-` y `renueva-`, en el README de ingress), y el `PATCH` de los ajustes
  responde los `422` y el `502` del contrato con un token que contiene `sin-ssl` o
  `ssl-lento`. Los mocks de deploy e ingress leen el tipo y los
  servicios de una app del catálogo (`catalogDb.configured`, en `db/catalog.ts` junto a
  `servicesOf` y `composeServices`): deploy valida `image`/`images` contra la configuración
  y saca de ahí la ref de un servicio que el deploy actual no tiene (§2.2), e ingress exige
  el slug como destino solo en una app de formulario. Lo que corre en Swarm (estado, logs)
  son los servicios del deploy actual. Al revés, el `last_deployed_at` del catálogo sigue
  al deploy actual del mock de deploy (`deployDb.lastDeployedAt`). `handlers/secrets.ts`
  pregunta al catálogo si el stage o la app existen (si no, `404`) y, al cambiar sus
  variables, adelanta su `config_changed_at` (`catalogDb.touchConfig`: la app, o todas las
  del stage), así quedan con cambios pendientes como en el backend. Un test de integración
  que toque uno de esos resetea también el otro.

### Sesión y `auth`

- La sesión es la cookie `muelle_session` (HttpOnly): el panel nunca la lee ni guarda nada
  de la sesión en localStorage. Ahí solo van comodidades de la vista: el idioma (Paraglide) y
  el aviso de Ajustes "La dirección del panel ahora es …" después de cambiar el puerto HTTPS
  (`proxySettings/panelMoveNotice.ts`, hasta que se cierra; "Ir al panel" lo lleva al origen
  nuevo con `?panel_moved=1`, porque allá el localStorage es otro). El aviso nombra la
  dirección anterior (`panelOrigin` de antes del `PATCH`, contrato §3.5): a dónde volver si
  la nueva no responde, si se entró por ella (por otro proxy, una IP o
  `MUELLE_PANEL_ORIGIN` no se usó nunca, y no la nombra). Allá no se sabe cuál era, y
  tampoco la nombra. Los dos orígenes pasan por
  `parsePanelOrigin` al leerlos. También va ahí el `at` de la vuelta a los puertos anteriores
  cuyo aviso se cerró (`proxySettings/rollbackNotice.ts`); los dos usan `noticeStorage.ts`.
- Para saber quién es el usuario: `createQuery(() => meQuery())` con `meQuery` del barrel
  de `auth`. Devuelve `Me`: `username`, `totpEnabled` y `needsTotpSetup`. El guard ya lo dejó
  en el cache.
- El barrel también exporta `authKeys`, `loadSessionState` (para guards), `landingFor` y
  `SessionShell`.
- Entrar (login, código, setup, invitación) y salir vacían todo el cache de TanStack
  Query: nada de la sesión anterior queda a la vista. Activar el 2FA invalida `me`.
- Cerrar sesión solo cuenta como hecho si sale bien o si el backend dice 401 (la sesión ya
  no existía). Cualquier otro error (red, 5xx, `bad_origin`) muestra un toast y deja al
  usuario donde estaba: la cookie sigue viva.
- "Volver" en el paso del código cierra la sesión a medias (el 401 `mfa_required` de esa
  respuesta es lo esperado).
- `/setup?token=` e `/invite/<token>` sacan el token de la URL apenas lo leen
  (`replaceState` a `/setup` e `/invite`): no queda en el historial ni en un pantallazo.
- Un 401 en `/2fa` es una sesión que se cayó: manda a `/login` como en el resto del panel.

### MSW

```
src/mocks/
  api.ts            api('/projects'), apiError(status, code, message), invalid(campo, motivo),
                    notFound() (404 not_found), noRoute() (404 no_route), noContent()
  session.ts        requireSession(): la respuesta 401/403 si no hay sesión completa, o undefined;
                    sessionUser()
  db/persist.ts     loadPersisted / savePersisted (sessionStorage en el navegador)
  db/<módulo>.ts    los datos del módulo
  handlers/<módulo>.ts
  handlers/index.ts junta todos (ya importa los de F1) y termina con un http.all que
                    contesta 404 no_route a cualquier /api/v1 sin handler
```

Un handler típico:

```ts
http.get(api('/projects'), () => requireSession() ?? HttpResponse.json(catalogDb.projects)),
http.post(api('/projects'), async ({ request }) => {
	const denied = requireSession()
	if (denied) return denied
	const body = (await request.json()) as { slug?: string }
	if (!body.slug) return invalid('slug', 'es obligatorio')
	…
}),
```

- Los errores, con los mismos `code` que el backend (los `mapper_errors.go` de cada módulo y
  `src/cmd/http/server.go`).
- Cada `db/<módulo>.ts` exporta un objeto con `reset()`; los tests de integración lo llaman
  en `beforeEach`, junto con `authDb.reset()`, y hacen
  `server.listen({ onUnhandledRequest: 'error' })`. Ojo: por el `http.all` del final, una
  ruta de `/api/v1` que el test no mockeó no dispara `onUnhandledRequest`, contesta
  `no_route`; el test lo ve como `UnexpectedError` (no como el `notFound` del mapa).
- Los usuarios y las invitaciones viven en `db/auth.ts` (`authDb.state.users`,
  `authDb.addInvite(token)`): el mock de `identity` los usa en vez de tener otra lista.
- `server.use(…)` dentro de un test para un caso puntual (un 422, un 404, `HttpResponse.error()`).

### Cosas que conviene saber

- Las rutas `(app)/…` y sus `+page.ts` corren solo en el navegador (`ssr = false`).
- `@tanstack/svelte-form` 1.33.5 (la última estable) avisa `state_proxy_equality_mismatch`
  en la consola de `pnpm dev`; es interno de la librería y no aparece en el build.
- Chrome anota en la consola cada respuesta 4xx de `fetch` ("Failed to load resource"): un
  401 de `/auth/me` en `/login` es esperado, no un error de la app.
- `pnpm test:e2e` construye en `build-e2e/` (ignorado): `mock` con MSW y `prod` igual al de
  `pnpm build`, y los sirve con `scripts/serve-build.mjs` (una copia a mano de las reglas de
  `panel.go`, sin `ETag` ni `Range`: ver [CSP](#csp)). `./build` no se toca.
- **Puertos de los e2e:** `mock` en `E2E_MOCK_PORT` (4173, el `baseURL`) y `prod` en
  `E2E_PROD_PORT` (4174). Salen de `e2e/ports.ts`: un test que necesite una URL la importa
  de ahí (`PROD_URL`, `MOCK_URL`) o usa rutas relativas al `baseURL`, nunca un puerto
  escrito a mano. Con el aislamiento de red (abajo), los puertos son del namespace de cada
  corrida y varios worktrees no chocan. Sin él (`MUELLE_TEST_HOST_NET=1`, o donde
  `unshare -rn` no anda), cada uno con los suyos:
  `E2E_MOCK_PORT=4183 E2E_PROD_PORT=4184 pnpm test:e2e`. Si un puerto está ocupado,
  Playwright corta con "already used" en vez de reusar lo que escucha ahí (que puede ser el
  build de otro worktree): elegí otro par. `E2E_REUSE_SERVER=1` reusa un servidor propio
  que ya dejaste corriendo (y corre con la red del host, que es donde escucha).
- `static/mockServiceWorker.js` solo queda en un build con `VITE_USE_MOCKS=1`: el adapter
  (`svelte.config.js`) lo borra de cualquier otro.
- **`pnpm test` y `pnpm test:e2e` corren en un network namespace propio**, con solo `lo`
  (`scripts/isolated-net.mjs`, con `unshare -rn`; en los e2e, el servidor del build que
  levanta Playwright también queda adentro). Chromium corta todas las peticiones en
  curso, también las de localhost, con `ERR_NETWORK_CHANGED` cada vez que cambia una IP del
  host, y Docker cambia una cada vez que arranca o borra un contenedor (otro worktree con
  tests de integración alcanza). En los tests de navegador salían archivos con "Failed to
  fetch dynamically imported module" (0 tests) y, si el corte caía en el arranque de un
  tester, vitest esperaba para siempre al 0 % de CPU. Adentro del namespace Chromium no ve
  esos cambios (y el puerto 63315 es siempre el propio). Si `unshare -rn` no anda (macOS,
  un CI que lo bloquea) avisa y corre con la red del host; `MUELLE_TEST_HOST_NET=1` lo
  saltea. `pnpm test:watch` no se aísla.
- Si igual una corrida se queda sin progreso, `scripts/vitest-stall-watchdog.mjs` (un
  reporter) la corta a los 120 s con la lista de archivos que no terminaron:
  `testTimeout`/`hookTimeout` no cubren un tester que no arrancó.

### E2E contra muelle

`e2e-real/` recorre el panel embebido en un `muelle serve` de verdad, con el Docker de
verdad: lo que `e2e/` prueba contra MSW, acá contra Go y Swarm, bajo la CSP que manda Go.
Un solo recorrido, en orden (`journey.real.ts`): el setup del primer usuario con el link del
log, el 2FA con un código calculado de la clave que muestra el panel (`totp.ts`, RFC 6238;
cada código de un paso nuevo, como exige el backend; sus tests, `totp.test.ts`, corren con
`pnpm test`), salir y entrar con el código, un
proyecto, un stage y una app de formulario (`nginx:alpine`), una variable secreta (que
llega al servicio de Swarm), un deploy seguido en Deploys hasta "Exitoso", el Resumen con el
estado de Swarm, los Logs en vivo, un segundo deploy y el rollback al primero, la Auditoría
con el nombre del usuario (`actor_name`) y borrar la app, el stage y el proyecto. Junta las
violaciones de CSP y los errores de la consola, como los de `e2e/`.

- **Necesita** un Docker que sea manager de un Swarm activo (`docker swarm init`: el script
  nunca lo crea; en CI lo hace el job), Go, el Chromium de Playwright y red para que el
  daemon baje `nginx:alpine` y consulte su digest. Con un `muelle-caddy` en ese Docker no
  corre: crear un stage lo conectaría a la red del stage.
- `scripts/e2e-real.mjs` construye el panel como la imagen (`VITE_USE_MOCKS=0 pnpm build`,
  con `200.html`, la CSP y sin `mockServiceWorker.js`), lo copia a
  `src/cmd/http/panel/build/` y compila muelle con `-tags panel` en una carpeta temporal
  (`E2E_REAL_BIN=<binario>` se salta los dos pasos). Toma el candado del Swarm de los tests
  de integración de Go (`flock` sobre `$TMPDIR/muelle-it-swarm.lock`, el de
  `dockertest/swarmlock`) y corre `playwright.real.config.ts` en el network namespace de
  `scripts/isolated-net.mjs`: muelle llega a Docker por su socket, y Chromium no ve los
  cambios de red que hace Docker al arrancar contenedores. Lo que sigue a `--` va a
  Playwright: `pnpm test:e2e:real -- --headed`.
- `e2e-real/globalSetup.ts` arranca muelle con una base nueva en una carpeta temporal:
  `MUELLE_LISTEN=127.0.0.1:24080` (otro con `E2E_REAL_PORT`, del 24000 al 24999),
  `MUELLE_PANEL_ORIGIN` en esa dirección, `MUELLE_SECURE_COOKIES=false` y su clave maestra
  (0600); ninguna otra `MUELLE_*` del entorno pasa. La carpeta y la clave las prepara
  `runDir.ts` (con `runDir.test.ts`): si `muelle genkey` falla, borra la carpeta, porque la
  limpieza de abajo todavía no está armada. Sin dominio del panel el proxy queda
  deshabilitado: no hay muelle-caddy ni rutas. El link de setup sale del log
  (`event: setup_url`) y llega a los tests por el entorno (`realRun()` de `env.ts`).
- Todo lo de Docker se llama `it7p<id>…` (el slug del proyecto; `id`, 4 hex al azar). La
  limpieza de `globalSetup.ts` corre al final aunque el test falle o se corte con Ctrl-C:
  apaga muelle, borra los stacks, las redes y los volúmenes de `it7p<id>` y deja el log de
  muelle en `test-results/real-muelle.log` (con la traza de Playwright en
  `test-results/real/`). El log y el borrado de la carpeta temporal (la clave maestra y la
  base) van aunque falle el CLI de docker (`teardown.ts`, con sus tests en
  `teardown.test.ts`); lo que quedó de Docker (o el fallo del CLI) se avisa después. La
  corrida también falla si el log de muelle trae un error interno: un 500 (`error interno`,
  aunque el panel no lo mostrara) o uno del despachador de deploys (`deploy: error
interno`, que no es una respuesta), con un solo error que nombra eso y lo de Docker; un
  pedido que el panel cortó al cambiar de pantalla no cuenta (el backend no lo registra
  como error). Al empezar, borra lo `it7p…` de más de 30 min (una corrida que murió con
  SIGKILL).
- Con un SIGKILL a Playwright la limpieza no corre: quedan el muelle de la corrida y su
  carpeta (`$TMPDIR/muelle-it7-*`). Ese muelle no tiene el candado del Swarm (node le pone
  CLOEXEC a lo heredado: el fd de `flock` lo tiene solo el node de `isolated-net.mjs`, que sale
  y lo suelta), así que no frena a los tests de Go; y si `unshare -rn` anduvo, escucha en el
  network namespace de su corrida y no choca con la próxima. Se mata con
  `pkill -f 'muelle-it7-bin-.*/muelle serve'` (o con la ruta de `E2E_REAL_BIN`).
- Un test nuevo va en `e2e-real/*.real.ts`, con los mismos selectores que `e2e/` (roles y
  textos en español). Lo que necesite de Docker, por `e2e-real/docker.ts` y solo con nombres
  `it7p<id>`.

### CSP

El panel corre bajo una CSP estricta: nada en línea ni `eval`, todo del mismo origen.

- **La política** está en `svelte.config.js` (`kit.csp`, modo `hash`): `default-src`,
  `script-src`, `style-src`, `img-src`, `font-src`, `connect-src` y `worker-src` en
  `'self'`, `object-src 'none'`, `base-uri 'none'` y `form-action 'self'`. El build la
  escribe como `<meta http-equiv>` en el `200.html`, con el hash del script de arranque de
  SvelteKit, y `style-src-attr 'unsafe-hashes'` con el hash del único `style` en línea que
  hay (el `#svelte-announcer` de SvelteKit).
- **Lo que manda Go** (la Fase 5, `src/cmd/http/panel/panel.go`, contrato §5.5) como
  header `Content-Security-Policy` en cada respuesta del panel: el contenido de
  `build/csp-header.txt`, que el build escribe con la política del `<meta>` más
  `frame-ancestors 'none'` (un `<meta>` no puede llevarlo). El hash cambia en cada build: Go
  lo lee del embed al arrancar (y no arranca si falta, si está vacío o si no es una sola
  línea). Un header con otra política no sirve: el navegador aplica las dos, y sin el hash
  el script de arranque no corre.
- **Cómo se embebe:** el `Dockerfile` construye `web/` (`pnpm codegen && pnpm build`, con
  `VITE_USE_MOCKS=0`, y verifica `build/200.html`, `build/csp-header.txt` y que no esté
  `mockServiceWorker.js`), lo copia en `src/cmd/http/panel/build/` y compila con `-tags panel`
  (`//go:embed all:build`: sin `all:` se pierde `_app/`). Sin el tag, toda ruta del panel es
  un `404` corto. El panel va en la raíz (`/`, sin `paths.base`) y todas sus rutas absolutas.
- **Qué responde Go**: el archivo del build si existe; si no, `200.html` (`/` también). Un
  `/_app/…` que no existe es `404` en texto, y `csp-header.txt` no se sirve. Solo `GET` y
  `HEAD` (otro método, `405` en JSON). En todas: la CSP, `nosniff`,
  `Referrer-Policy: same-origin` (la misma del `<meta name="referrer">` de `app.html`; no
  `no-referrer`, que haría que un `<form>` nativo mande `Origin: null`) y `Cache-Control` de
  un año `immutable` en `/_app/immutable/…` y `no-cache` en el resto (el `200.html` y
  `_app/version.json` revalidan con su `ETag`). El `Content-Type` sale de una tabla propia,
  por la extensión en minúscula, y `http.ServeContent` atiende `HEAD`, `Range` y las
  condiciones (`If-None-Match` → `304`).
- **`scripts/serve-build.mjs` lo copia a mano** para los e2e, sin el `ETag`, el `304` ni los
  `Range` (los e2e no los usan). Nada verifica que diga lo mismo que `panel.go`: si cambia
  uno, se cambia el otro. `e2e/embed.e2e.ts` prueba lo que es del build (rutas absolutas
  bajo `/_app/immutable/`, `frame-ancestors` en `csp-header.txt`); sus tests de headers, 404
  y 405 documentan lo que hace `panel.go` contra el mock, y lo de verdad lo prueba
  `panel_test.go`.
- **Qué rompe la CSP** (lo atrapa `e2e/csp.e2e.ts`, que falla con cualquier violación):
    - un `style="…"` fijo en un template (va una clase de `styles.css.ts`; `style:prop={x}`
      sí se puede: Svelte lo aplica por CSSOM);
    - `{@html}` con `<script>` o atributos `on…`, `eval`, `new Function`;
    - imágenes o fuentes `data:` o de otro origen (Vite no mete assets en línea:
      `assetsInlineLimit: 0`);
    - crear un schema de Zod antes de `@/shared/data/zodSetup` (Zod sin JIT: con JIT prueba
      un `new Function` que la CSP reporta). Es el primer import de `hooks.client.ts`: que
      siga primero.
- **Librerías que inyectan estilos** (CodeMirror 6 con su `style-mod`, y cualquiera que
  haga `document.createElement('style')`): en el documento la CSP bloquea ese `<style>` y
  el editor queda sin estilos. Adentro de un shadow root, CodeMirror usa
  `adoptedStyleSheets` (hojas construidas por JS), que la CSP no frena. Se monta con
  `ShadowHost` de `@/shared/ui`:

    ```svelte
    <!-- XScreen.svelte -->
    <ShadowHost class={styles.editor} mount={vm.mountEditor} />
    ```

    ```ts
    // X.viewModel.svelte.ts: estable (arrow function del viewModel)
    mountEditor: ShadowMount = (container, root) => {
    	const view = new EditorView({ parent: container, root, doc: this.yaml, extensions: [...] })
    	return () => view.destroy()
    }
    ```

    - `root` es obligatorio: sin él, CodeMirror vuelve a poner el `<style>` en el documento;
    - el nombre accesible va en el editor, no en el host:
      `EditorView.contentAttributes.of({ 'aria-label': m.catalog_compose_label() })`;
    - adentro no llegan las clases de vanilla-extract, pero sí las variables del tema:
      `EditorView.theme({ '&': { color: theme.colors.text.primary } })` funciona, porque
      `theme.*` son `var(--…)` declaradas en `:root`. Ese tema va en un `editorTheme.ts`
      junto a la pantalla: es la excepción a "`theme` solo en `styles.css.ts`";
    - el borde, el alto y el radio van en `class` del host;
    - `ShadowHost.svelte.test.ts` lo prueba bajo `style-src 'self'` (y muestra que sin
      shadow root falla). Si otra librería no usa `adoptedStyleSheets`, no sirve: avisar.

- Una pantalla nueva con algo raro (CodeMirror, un gráfico) se suma al recorrido de
  `e2e/csp.e2e.ts`.
- `pnpm dev` aplica la misma política como header, con `'unsafe-inline'` en los estilos
  (Vite los inyecta así). Una violación de scripts aparece igual en la consola.

---

## Svelte MCP

You are able to use the Svelte MCP server, where you have access to comprehensive Svelte 5 and SvelteKit documentation. Here's how to use the available tools effectively:

### 1. list-sections

Use this FIRST to discover all available documentation sections. Returns a structured list with titles, use_cases, and paths.
When asked about Svelte or SvelteKit topics, ALWAYS use this tool at the start of the chat to find relevant sections.

### 2. get-documentation

Retrieves full documentation content for specific sections. Accepts single or multiple sections.
After calling the list-sections tool, you MUST analyze the returned documentation sections (especially the use_cases field) and then use the get-documentation tool to fetch ALL documentation sections that are relevant for the user's task.

### 3. svelte-autofixer

Analyzes Svelte code and returns issues and suggestions.
You MUST use this tool whenever writing Svelte code before sending it to the user. Keep calling it until no issues or suggestions are returned.

### 4. playground-link

Generates a Svelte Playground link with the provided code.
After completing the code, ask the user if they want a playground link. Only call this tool after user confirmation and NEVER if code was written to files in their project.
