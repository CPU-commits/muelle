import { defineConfig } from '@playwright/test'
import { MOCK_PORT, MOCK_URL, PROD_PORT } from './e2e/ports'

// Solo los flujos críticos (ver "Tests" en CLAUDE.md): el panel bajo la CSP, el acceso y el
// camino de un deploy. Corren contra el build real (el `200.html` de adapter-static),
// servido como lo va a servir Go: con el header de la CSP (scripts/serve-build.mjs). Hay dos
// builds, los dos en build-e2e/ para no pisar el ./build que se embebe:
// - `mock` (E2E_MOCK_PORT, 4173; el baseURL): con MSW, para recorrer el panel sin backend;
// - `prod` (E2E_PROD_PORT, 4174): el mismo build que `pnpm build`, para ver que arranca
//   bajo la CSP (e2e/csp.e2e.ts).
// `pnpm test:e2e` corre en un network namespace propio (scripts/isolated-net.mjs): los
// puertos son de esa corrida y varios worktrees no chocan. Con la red del host se cambian
// con E2E_MOCK_PORT y E2E_PROD_PORT (e2e/ports.ts).
export default defineConfig({
	webServer: {
		command: `pnpm build:e2e && node scripts/serve-build.mjs build-e2e/prod:${PROD_PORT} build-e2e/mock:${MOCK_PORT}`,
		url: `${MOCK_URL}/robots.txt`,
		// Nunca reusar sin pedirlo: con varios worktrees, lo que escucha en el puerto puede
		// ser el build de otro, y los tests pasarían (o fallarían) contra código ajeno. Con
		// el puerto ocupado, Playwright corta con un error claro. E2E_REUSE_SERVER=1 reusa un
		// `node scripts/serve-build.mjs …` propio que ya está corriendo.
		reuseExistingServer: process.env.E2E_REUSE_SERVER === '1',
		timeout: 180_000,
	},
	use: { baseURL: MOCK_URL },
	testDir: 'e2e',
	testMatch: '**/*.e2e.{ts,js}',
})
