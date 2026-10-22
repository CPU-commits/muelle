import { expect, test, type Page } from '@playwright/test'
import { PROD_URL } from './ports'
import { login, USERNAME, watch } from './support'

// El panel bajo la CSP que manda el servidor (el `csp-header.txt` del build, ver
// svelte.config.js y scripts/serve-build.mjs). Cualquier violación (un script en línea sin
// hash, un `style="…"` en el HTML, el `new Function` de Zod, un `data:`) rompe el test.
//
// Primero el build de producción, el mismo que embebe Go: arranca en un deep link (sus
// assets van con rutas absolutas bajo /_app/immutable/: con relativas, el 200.html de una
// ruta anidada no carga nada) y no trae el service worker de MSW. Después, cada pantalla del
// panel una vez, en el build con MSW y en un teléfono: ya que se abre, se mira también que
// no tenga scroll horizontal. Una pantalla nueva se suma a SCREENS.

const APP = '/projects/auditfy/stages/staging/apps/api'

interface Screen {
	path: string
	heading: string
	level?: number
	/** Lo que hay que mirar además en esa pantalla. */
	check?: (page: Page) => Promise<void>
}

const SCREENS: Screen[] = [
	{
		path: '/',
		heading: 'Proyectos',
		level: 1,
		// El menú de la cuenta va en el top layer (popover) y se posiciona con CSSOM.
		check: async (page) => {
			await page.getByRole('button', { name: `Cuenta: ${USERNAME}` }).click()
			await expect(page.getByRole('menuitem', { name: 'Cerrar sesión' })).toBeVisible()
			await page.keyboard.press('Escape')
		},
	},
	{ path: APP, heading: 'api', level: 1 },
	{ path: `${APP}/deploys`, heading: 'Deploys', level: 1 },
	{ path: `${APP}/logs`, heading: 'Logs', level: 1 },
	{ path: `${APP}/dominios`, heading: 'Dominios', level: 1 },
	{ path: `${APP}/variables`, heading: 'Variables', level: 1 },
	{ path: `${APP}/configuracion`, heading: 'Configuración', level: 1 },
	{
		path: '/projects/aumi-agents/stages/staging/apps/agents/configuracion',
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
	{ path: '/ajustes', heading: 'Ajustes', level: 1 },
	{ path: '/ajustes/tokens', heading: 'Tokens de API', level: 2 },
	// El QR es un <svg> armado en el cliente.
	{ path: '/2fa', heading: 'Configurar 2FA' },
	{ path: '/invite', heading: 'Unirte a muelle' },
]

test('el panel corre bajo la CSP del header sin una sola violación, y cada pantalla entra en un teléfono', async ({
	page,
	request,
}) => {
	const problems = await watch(page)

	await test.step('el build de producción arranca en un deep link', async () => {
		// Sin backend `/auth/me` es 401: el guard corrió y mandó al login.
		const response = await page.goto(`${PROD_URL}${APP}/dominios`)
		const header = response?.headers()['content-security-policy'] ?? ''
		expect(header).toContain("default-src 'self'")
		expect(header).toMatch(/script-src 'self' 'sha256-[^']+'/)
		expect(header).toContain("frame-ancestors 'none'")
		await expect(page).toHaveURL(`${PROD_URL}/login`)
		await expect(page.getByRole('heading', { name: 'Entrar a muelle' })).toBeVisible()
		await page.getByLabel('Usuario').fill(USERNAME)
		await expect(page.getByLabel('Usuario')).toHaveValue(USERNAME)

		// El archivo no existe: el servidor devuelve el `200.html` en su lugar.
		const worker = await request.get(`${PROD_URL}/mockServiceWorker.js`)
		expect(worker.headers()['content-type']).toContain('text/html')
	})

	await test.step('cada pantalla, en un teléfono', async () => {
		await page.setViewportSize({ width: 360, height: 740 })
		await login(page)
		for (const { path, heading, level, check } of SCREENS) {
			await page.goto(path)
			await expect(page.getByRole('heading', { name: heading, level }), path).toBeVisible()
			// Con lo que carga con sus queries.
			await expect(
				page.getByRole('status').filter({ hasText: 'Cargando…' }),
				path,
			).toHaveCount(0)
			const overflow = await page.evaluate(
				() => document.documentElement.scrollWidth - document.documentElement.clientWidth,
			)
			expect(overflow, `scroll horizontal en ${path}`).toBeLessThanOrEqual(0)
			await check?.(page)
		}
	})

	expect(problems).toEqual([])
})
