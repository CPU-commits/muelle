import { expect, test, type Page } from '@playwright/test'

// Resumen, Deploys, Logs y Dominios (F1-deploy-ingress) contra el build con MSW, bajo la
// CSP del header: cada test junta las violaciones de CSP y los errores de la consola, y
// termina sin ninguno. Los datos son los del mock (src/mocks/db/deploy.ts e ingress.ts).
const USERNAME = 'felipe'
const PASSWORD = 'una-contraseña-larga'
const APP = '/projects/auditfy/stages/staging/apps/api'

/** Las violaciones de CSP y los errores de la página (un 4xx de fetch es esperado). */
async function watch(page: Page): Promise<string[]> {
	const problems: string[] = []
	page.on('pageerror', (error) => problems.push(`pageerror: ${error.message}`))
	page.on('console', (message) => {
		const text = message.text()
		if (/Content Security Policy/i.test(text)) problems.push(`csp: ${text}`)
		else if (message.type() === 'error' && !text.startsWith('Failed to load resource')) {
			problems.push(`console: ${text}`)
		}
	})
	await page.exposeFunction('reportCspViolation', (text: string) => problems.push(`csp: ${text}`))
	await page.addInitScript(() => {
		document.addEventListener('securitypolicyviolation', (event) => {
			const report = (window as unknown as { reportCspViolation: (text: string) => void })
				.reportCspViolation
			report(`${event.effectiveDirective}: ${event.blockedURI}`)
		})
	})
	return problems
}

async function login(page: Page): Promise<void> {
	await page.goto('/login')
	await page.getByLabel('Usuario').fill(USERNAME)
	await page.getByLabel('Contraseña').fill(PASSWORD)
	await page.getByRole('button', { name: 'Continuar' }).click()
	await expect(page.getByRole('heading', { name: 'Proyectos' })).toBeVisible()
}

test('Resumen → Desplegar con otra imagen → Deploys sigue el log en vivo', async ({ page }) => {
	const problems = await watch(page)
	await login(page)

	await page.goto(APP)
	await expect(page.getByRole('heading', { level: 1, name: 'api' })).toBeVisible()
	await expect(page.getByRole('table', { name: 'Tareas de Swarm de la app' })).toBeVisible()

	await page.getByRole('button', { name: 'Desplegar' }).first().click()
	const dialog = page.getByRole('dialog', { name: 'Desplegar api' })
	await expect(dialog).toBeVisible()
	const image = dialog.getByLabel(/^Imagen/)

	// La misma regla que el backend: un repositorio con mayúsculas no pasa.
	await image.fill('Ghcr.io/Foo/Bar:Latest')
	await dialog.getByRole('button', { name: 'Desplegar' }).click()
	await expect(dialog.getByText(/No es una referencia de imagen válida/)).toBeVisible()

	await image.fill('ghcr.io/auditfy/api:sha-e2e0001')
	await dialog.getByRole('button', { name: 'Desplegar' }).click()
	await expect(page).toHaveURL(/\/apps\/api\/deploys$/)

	// Abre el más reciente (el recién encolado) y su log llega en vivo.
	const panel = page.getByRole('region', { name: /^Log del deploy #\d+$/ })
	await expect(panel).toBeVisible()
	await expect(page.getByRole('button', { pressed: true })).toContainText('sha-e2e0001')
	await expect(panel.getByRole('listitem').nth(2)).toBeVisible({ timeout: 15_000 })

	expect(problems).toEqual([])
})

test('Desplegar una compose que nunca se desplegó: una imagen para un servicio del catálogo', async ({
	page,
}) => {
	const problems = await watch(page)
	await login(page)

	// aumi-agents/staging/agents: Swarm no tiene nada; los servicios salen del catálogo.
	await page.goto('/projects/aumi-agents/stages/staging/apps/agents')
	await expect(page.getByRole('heading', { level: 1, name: 'agents' })).toBeVisible()
	await page.getByRole('button', { name: 'Desplegar' }).first().click()
	const dialog = page.getByRole('dialog', { name: 'Desplegar agents' })
	await expect(dialog.getByLabel(/^Imagen de runner/)).toBeVisible()
	await expect(dialog.getByLabel(/^Imagen de scheduler/)).toBeVisible()

	await dialog.getByLabel(/^Imagen de gateway/).fill('ghcr.io/aumi-app/agents-gateway:e2e')
	await dialog.getByRole('button', { name: 'Desplegar' }).click()
	await expect(page).toHaveURL(/\/apps\/agents\/deploys$/)
	const panel = page.getByRole('region', { name: /^Log del deploy #\d+$/ })
	await expect(panel.getByText(/agents-gateway:e2e/).first()).toBeVisible({ timeout: 15_000 })

	expect(problems).toEqual([])
})

test('Deploys: ?deploy= abre ese deploy, y uno que no existe avisa', async ({ page }) => {
	const problems = await watch(page)
	await login(page)

	await page.goto(`${APP}/deploys?deploy=140`)
	await expect(page.getByRole('heading', { level: 2, name: '#140' })).toBeVisible()
	await expect(page.getByRole('region', { name: 'Log del deploy #140' })).toBeVisible()

	await page.goto(`${APP}/deploys?deploy=9999`)
	await expect(
		page.getByText('El deploy #9999 ya no existe: se muestra el más reciente.'),
	).toBeVisible()
	await expect(page.getByRole('heading', { level: 2, name: /^#\d+$/ })).toBeVisible()

	// Elegir otro del historial no cambia la URL (el ?deploy= es para entrar).
	await page.getByRole('button', { name: /#141/ }).click()
	await expect(page.getByRole('heading', { level: 2, name: '#141' })).toBeVisible()
	await expect(page).toHaveURL(/\?deploy=9999$/)

	expect(problems).toEqual([])
})

test('Logs: llegan en vivo, la pausa junta las nuevas y reanudar las suma', async ({ page }) => {
	const problems = await watch(page)
	await login(page)

	await page.goto(`${APP}/logs`)
	await expect(page.getByRole('heading', { level: 1, name: 'Logs' })).toBeVisible()
	const lines = page.getByRole('region', { name: 'Líneas de log' }).getByRole('listitem')
	await expect(lines.first()).toBeVisible()

	await page.getByRole('button', { name: 'Pausar' }).click()
	const resume = page.getByRole('button', { name: 'Reanudar' })
	await expect(resume).toBeVisible()
	// El mock escribe una línea cada ~1 s.
	await expect(page.getByText(/^\d+ línea(s)? nueva(s)? en pausa$/)).toBeVisible({
		timeout: 5_000,
	})
	await expect(page.getByRole('status').filter({ hasText: 'Hay líneas nuevas' })).toHaveCount(1)

	await resume.click()
	await expect(page.getByRole('button', { name: 'Pausar' })).toBeVisible()
	await expect(page.getByText(/en pausa$/)).toHaveCount(0)

	// Filtrar por texto deja solo lo que coincide.
	await page.getByRole('searchbox', { name: 'Filtrar logs' }).fill('GET /health')
	await expect(lines.first()).toContainText('GET /health')

	expect(problems).toEqual([])
})

test('Dominios: agregar con Proxied, ver cómo se emite el certificado y eliminar', async ({
	page,
}) => {
	const problems = await watch(page)
	await login(page)

	await page.goto(`${APP}/dominios`)
	await expect(page.getByRole('heading', { level: 1, name: 'Dominios' })).toBeVisible()
	const list = page.getByRole('list', { name: 'Dominios de la app' })
	await expect(list.getByRole('article').first()).toBeVisible()

	const hostname = 'e2e.tudominio.cl'
	await page.getByRole('textbox', { name: 'Hostname' }).fill(hostname)
	const proxied = page.getByRole('switch', { name: 'Proxied', exact: true })
	await expect(proxied).not.toBeChecked()
	await proxied.check()
	await page.getByRole('button', { name: 'Agregar' }).click()

	const card = list.getByRole('article', { name: hostname })
	await expect(card).toBeVisible()
	await expect(card.getByText('Emitiendo certificado')).toBeVisible()
	// La lista se refresca sola mientras muelle provisiona (~3 s en el mock).
	await expect(card.getByText('Válido')).toBeVisible({ timeout: 15_000 })
	await expect(card.getByRole('switch', { name: `Proxied para ${hostname}` })).toBeChecked()

	await card.getByRole('button', { name: `Opciones de ${hostname}` }).click()
	await page.getByRole('menuitem', { name: 'Eliminar' }).click()
	const confirm = page.getByRole('dialog', { name: `Eliminar ${hostname}` })
	await confirm.getByRole('button', { name: 'Eliminar' }).click()
	await expect(card).toHaveCount(0)

	expect(problems).toEqual([])
})

test('Dominios: el aviso "proxy desincronizado" cuando Caddy no acepta la configuración', async ({
	page,
}) => {
	const problems = await watch(page)
	await login(page)

	await page.goto(`${APP}/dominios`)
	await expect(page.getByRole('list', { name: 'Dominios de la app' })).toBeVisible()
	await expect(page.getByText('El proxy está desincronizado.')).toHaveCount(0)

	// El mock: un hostname con `caddy-rechaza` deja a Caddy sin aceptar la configuración.
	await page.getByRole('textbox', { name: 'Hostname' }).fill('caddy-rechaza.tudominio.cl')
	await page.getByRole('button', { name: 'Agregar' }).click()

	await expect(page.getByText('El proxy está desincronizado.')).toBeVisible()
	await expect(page.getByText(/Caddy rechazó la configuración/)).toBeVisible()
	// Lo que lo arregla (los puertos, el registry de la imagen de Caddy…) está en Ajustes.
	await page.getByRole('link', { name: 'Abrir Ajustes' }).click()
	await expect(page).toHaveURL(/\/ajustes$/)

	expect(problems).toEqual([])
})

test('Dominios de una compose ofrece los servicios del catálogo con sus puertos', async ({
	page,
}) => {
	const problems = await watch(page)
	await login(page)

	// aumi-agents/staging/agents: gateway declara `expose: ["8080"]` (contrato §2.12).
	await page.goto('/projects/aumi-agents/stages/staging/apps/agents/dominios')
	await expect(page.getByRole('list', { name: 'Dominios de la app' })).toBeVisible()
	await expect(page.getByRole('combobox', { name: 'Servicio y puerto de destino' })).toHaveValue(
		'gateway:8080',
	)

	expect(problems).toEqual([])
})

test('las cuatro pantallas entran en un teléfono sin scroll horizontal', async ({ page }) => {
	const problems = await watch(page)
	await page.setViewportSize({ width: 375, height: 812 })
	await login(page)

	const pages = [
		{ path: '', heading: 'api', content: page.getByRole('table').first() },
		{
			path: '/deploys',
			heading: 'Deploys',
			content: page.getByRole('region', { name: /^Log del deploy/ }),
		},
		{
			path: '/logs',
			heading: 'Logs',
			content: page
				.getByRole('region', { name: 'Líneas de log' })
				.getByRole('listitem')
				.first(),
		},
		{ path: '/dominios', heading: 'Dominios', content: page.getByRole('article').first() },
	]
	for (const { path, heading, content } of pages) {
		await page.goto(`${APP}${path}`)
		await expect(page.getByRole('heading', { level: 1, name: heading })).toBeVisible()
		// Que haya contenido (no solo el "Cargando…") antes de medir.
		await expect(content).toBeVisible()
		const overflow = await page.evaluate(
			() => document.documentElement.scrollWidth - document.documentElement.clientWidth,
		)
		expect(overflow, `scroll horizontal en ${path || 'Resumen'}`).toBeLessThanOrEqual(0)
	}

	expect(problems).toEqual([])
})
