import { defineConfig } from '@playwright/test'

// Los e2e corren contra el build real (el `200.html` de adapter-static), servido como lo
// va a servir Go: con el header de la CSP (scripts/serve-build.mjs). Hay dos builds, los
// dos en build-e2e/ para no pisar el ./build que se embebe:
// - `mock` (puerto 4173, el baseURL): con MSW, para recorrer el panel sin backend;
// - `prod` (puerto 4174): el mismo build que `pnpm build`, para ver que arranca bajo la
//   CSP (e2e/csp.e2e.ts).
const MOCK_URL = 'http://127.0.0.1:4173'

export default defineConfig({
	webServer: {
		command:
			'pnpm build:e2e && node scripts/serve-build.mjs build-e2e/prod:4174 build-e2e/mock:4173',
		url: `${MOCK_URL}/robots.txt`,
		reuseExistingServer: !process.env.CI,
		timeout: 180_000,
	},
	use: { baseURL: MOCK_URL },
	testDir: 'e2e',
	testMatch: '**/*.e2e.{ts,js}',
})
