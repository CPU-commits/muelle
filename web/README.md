# muelle-panel

El panel web de muelle: una SPA en Svelte 5 + SvelteKit (sin SSR) que el binario de Go
sirve junto a la API `/api/v1`. Sale de la plantilla `svelte-clean-template`: clean
architecture en 3 capas por módulo. Las reglas, la arquitectura y los contratos
compartidos están en [CLAUDE.md](./CLAUDE.md).

## Desarrollo

Con el backend real (la sesión es una cookie: el panel y la API comparten origen gracias al
proxy de Vite):

```bash
# en la raíz del repo
go run . genkey > /tmp/muelle.key
MUELLE_PANEL_ORIGIN=http://localhost:5173 \
MUELLE_SECURE_COOKIES=false \
MUELLE_LISTEN=127.0.0.1:18080 \
MUELLE_DATA_DIR=/tmp/muelle-data \
MUELLE_MASTER_KEY_FILE=/tmp/muelle.key \
go run . serve           # imprime el link de setup: http://localhost:5173/setup?token=…

# en web/
pnpm install
pnpm dev                 # http://localhost:5173, /api → http://127.0.0.1:18080
```

`MUELLE_BACKEND_URL` cambia a dónde proxea `pnpm dev` (por defecto, `devProxy` de
`config/local.json`).

Sin backend, con MSW:

```bash
pnpm dev:mock            # usuario felipe / una-contraseña-larga, código de 2FA 123456
```

## Comandos

```bash
pnpm dev / dev:mock   # servidor de desarrollo, con el backend o con MSW
pnpm build            # → ./build (200.html + _app), lo que embebe el binario
pnpm preview          # sirve ./build
pnpm check            # codegen + svelte-check
pnpm lint             # prettier --check + eslint
pnpm format           # prettier --write
pnpm test             # vitest: node (dominio, mappers, MSW) + Chromium (viewModels)
pnpm test:e2e         # playwright: construye build-e2e/ (con MSW y sin) y lo sirve con la CSP;
                      # no toca ./build. Corre en su propio network namespace, como pnpm test
pnpm verify           # lint + check + test, lo mismo que CI
```
