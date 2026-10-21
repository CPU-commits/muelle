import { defineConfig } from '@playwright/test'
import { REAL_URL } from './e2e-real/env'

// `pnpm test:e2e:real`: el panel embebido en un `muelle serve` de verdad, con el Swarm local
// (web/e2e-real/). No usar directo: scripts/e2e-real.mjs construye el panel y el binario
// con -tags panel, toma el candado del Swarm de los tests de integración de Go y corre esto
// en el network namespace de scripts/isolated-net.mjs. globalSetup arranca muelle y su
// limpieza borra lo de Docker de la corrida (it7p<id>…), aunque los tests fallen.
export default defineConfig({
	testDir: 'e2e-real',
	testMatch: '**/*.real.ts',
	globalSetup: './e2e-real/globalSetup.ts',
	// Una sola base y un solo Swarm: nada en paralelo, y sin reintentos (el recorrido deja
	// usado el link de setup).
	workers: 1,
	fullyParallel: false,
	retries: 0,
	outputDir: 'test-results/real',
	reporter: [['list']],
	// Contra Go y Docker, no contra MSW: las respuestas tardan más.
	expect: { timeout: 15_000 },
	use: {
		baseURL: REAL_URL,
		trace: 'retain-on-failure',
		screenshot: 'only-on-failure',
	},
})
