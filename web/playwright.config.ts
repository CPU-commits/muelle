import { defineConfig } from '@playwright/test'

// Los e2e corren contra el build real (el `200.html` de adapter-static servido por
// `vite preview`) y con MSW prendido, para no depender de un backend.
export default defineConfig({
	webServer: {
		command: 'VITE_USE_MOCKS=1 pnpm build && pnpm preview',
		port: 4173,
		reuseExistingServer: !process.env.CI,
	},
	testDir: 'e2e',
	testMatch: '**/*.e2e.{ts,js}',
})
