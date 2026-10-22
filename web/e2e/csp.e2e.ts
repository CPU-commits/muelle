import { expect, test, type Page } from '@playwright/test'
import { login, watch } from './support'

// El panel bajo la CSP que manda el servidor (el `csp-header.txt` del build, ver
// svelte.config.js y scripts/serve-build.mjs): cada pantalla una vez, con MSW. Cualquier
// violación (un script en línea sin hash, un `style="…"` en el HTML, el `new Function` de
// Zod, un `data:`) rompe el test. Cada `goto` es un deep link: con assets relativos, el
// 200.html de una ruta anidada no carga nada. Una pantalla nueva se suma a SCREENS.

const APP = '/projects/auditfy/stages/staging/apps/api'

interface Screen {
	path: string
	heading: string
	level?: number
	/** Lo que hay que mirar además en esa pantalla. */
	check?: (page: Page) => Promise<void>
}

const SCREENS: Screen[] = [
	{ path: '/', heading: 'Proyectos', level: 1 },
	{ path: APP, heading: 'api', level: 1 },
	{ path: `${APP}/deploys`, heading: 'Deploys', level: 1 },
	{ path: `${APP}/logs`, heading: 'Logs', level: 1 },
	{ path: `${APP}/domains`, heading: 'Dominios', level: 1 },
	{ path: `${APP}/variables`, heading: 'Variables', level: 1 },
	{ path: `${APP}/configuration`, heading: 'Configuración', level: 1 },
	{
		path: '/projects/aumi-agents/stages/staging/apps/agents/configuration',
		heading: 'Configuración',
		level: 1,
		// El compose se edita en CodeMirror, que inyecta sus estilos: adentro de un shadow
		// root (ShadowHost), con adoptedStyleSheets, la CSP no los frena.
		check: async (page) => {
			await expect(page.locator('.cm-content')).toBeVisible()
			const display = await page
				.locator('.cm-scroller')
				.evaluate((element) => getComputedStyle(element).display)
			expect(display).toBe('flex')
		},
	},
	{ path: '/projects/auditfy/stages/staging/variables', heading: 'Variables del stage' },
	// `/settings` abre en Proxy y DNS.
	{ path: '/settings', heading: 'Cloudflare' },
	{ path: '/settings/security', heading: 'Seguridad' },
	{ path: '/settings/registries', heading: 'Registries', level: 2 },
	{ path: '/settings/users', heading: 'Usuarios', level: 2 },
	{ path: '/settings/tokens', heading: 'Tokens de API', level: 2 },
	{ path: '/settings/audit', heading: 'Auditoría', level: 2 },
	// El QR es un <svg> armado en el cliente.
	{ path: '/2fa', heading: 'Configurar 2FA' },
	{ path: '/invite', heading: 'Unirte a muelle' },
]

test('cada pantalla del panel carga bajo la CSP del header sin una sola violación', async ({
	page,
}) => {
	const problems = await watch(page)

	// Sin la CSP en el header, cero violaciones no diría nada.
	const response = await page.goto('/login')
	const header = response?.headers()['content-security-policy'] ?? ''
	expect(header).toContain("default-src 'self'")
	expect(header).toContain("frame-ancestors 'none'")
	await login(page)

	for (const { path, heading, level, check } of SCREENS) {
		await page.goto(path)
		await expect(page.getByRole('heading', { name: heading, level }), path).toBeVisible()
		// Con lo que carga con sus queries.
		await expect(page.getByRole('status').filter({ hasText: 'Cargando…' }), path).toHaveCount(0)
		await check?.(page)
	}

	expect(problems).toEqual([])
})
