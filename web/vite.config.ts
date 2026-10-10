import { paraglideVitePlugin } from '@inlang/paraglide-js'
import { playwright } from '@vitest/browser-playwright'
import { sveltekit } from '@sveltejs/kit/vite'
import { vanillaExtractPlugin } from '@vanilla-extract/vite-plugin'
import { loadEnv } from 'vite'
import { defineConfig } from 'vitest/config'
import { resolve } from 'node:path'
import devtoolsJson from 'vite-plugin-devtools-json'
import localConfig from './config/local.json'

export default defineConfig(({ mode }) => {
	const env = loadEnv(mode, process.cwd(), '')
	// `pnpm dev` proxea /api a un `muelle serve` local (ver web/README.md). El panel y la
	// API quedan en el mismo origen, así que la cookie de sesión y el `Origin` que revisa
	// el backend funcionan igual que en producción. Con `pnpm dev:mock` MSW contesta antes
	// de que la petición llegue al proxy.
	const backend = env.MUELLE_BACKEND_URL || localConfig.devProxy

	return {
		resolve: {
			alias: {
				'@': resolve('./src'),
			},
		},
		server: {
			proxy: {
				'/api': { target: backend, changeOrigin: false },
			},
		},
		plugins: [
			sveltekit(),
			devtoolsJson(),
			paraglideVitePlugin({
				project: './project.inlang',
				outdir: './src/shared/ui/i18n/paraglide',
				// El locale vive en localStorage. En una SPA no hay servidor que pueda
				// negociarlo por Accept-Language ni reescribir la URL, así que no hay
				// `reroute` ni middleware: la elección del usuario persiste en el cliente
				// y el fallback es el locale base.
				strategy: ['localStorage', 'baseLocale'],
				emitTsDeclarations: true,
			}),
			vanillaExtractPlugin(),
		],
		test: {
			expect: { requireAssertions: true },
			projects: [
				{
					// Componentes y viewModels: navegador real. Un viewModel llama a
					// `createQuery`, que lee el QueryClient del contexto de Svelte, así que
					// necesita estar montado en un componente de verdad.
					extends: './vite.config.ts',
					test: {
						name: 'client',
						browser: {
							enabled: true,
							provider: playwright(),
							instances: [{ browser: 'chromium', headless: true }],
						},
						include: ['src/**/*.svelte.{test,spec}.{js,ts}'],
					},
				},
				{
					// Dominio, casos de uso, mappers e integración contra MSW: node. Nada de
					// esto toca el DOM, y corre en milisegundos. En node una ruta relativa no
					// tiene contra qué resolverse: el cliente HTTP y los handlers de MSW
					// usan este origen.
					extends: './vite.config.ts',
					test: {
						name: 'server',
						environment: 'node',
						env: { VITE_API_URL: 'http://localhost' },
						// Node 25+ trae localStorage/sessionStorage experimentales y avisa al
						// tocarlos sin archivo. Acá sobran: los tests de node no tienen navegador.
						execArgv: ['--no-experimental-webstorage'],
						include: ['src/**/*.{test,spec}.{js,ts}'],
						exclude: ['src/**/*.svelte.{test,spec}.{js,ts}'],
					},
				},
			],
		},
	}
})
