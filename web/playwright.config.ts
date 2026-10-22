import { defineConfig } from '@playwright/test'
import { MOCK_PORT, MOCK_URL } from './e2e/ports'

// Solo lo que ningún otro test ve (ver "Tests" en CLAUDE.md): cada pantalla del panel bajo
// la CSP. Corre contra el build real con MSW (el `200.html` de adapter-static, en
// build-e2e/ para no pisar el ./build que se embebe), servido como lo va a servir Go: con
// el header de la CSP (scripts/serve-build.mjs), en E2E_MOCK_PORT (4173).
// `pnpm test:e2e` corre en un network namespace propio (scripts/isolated-net.mjs): el
// puerto es de esa corrida y varios worktrees no chocan. Con la red del host se cambia con
// E2E_MOCK_PORT (e2e/ports.ts).
export default defineConfig({
	webServer: {
		command: `pnpm build:e2e && node scripts/serve-build.mjs build-e2e/mock:${MOCK_PORT}`,
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
