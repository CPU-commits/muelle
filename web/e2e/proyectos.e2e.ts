import { expect, test, type Page } from '@playwright/test'

// La Fase 2 en Proyectos y en el Resumen, contra el build con MSW: el estado de cada app
// sale de `deploy` y lo que el catálogo sabe ("Cambios pendientes", "Sin deploys") sigue
// ahí. Los datos son los del mock (src/mocks/db/catalog.ts y deploy.ts).
const USERNAME = 'felipe'
const PASSWORD = 'una-contraseña-larga'

/** Los errores de la página (un 4xx de fetch es esperado: CLAUDE.md). */
function watchErrors(page: Page): string[] {
	const errors: string[] = []
	page.on('pageerror', (error) => errors.push(error.message))
	page.on('console', (message) => {
		if (message.type() === 'error' && !message.text().startsWith('Failed to load resource')) {
			errors.push(message.text())
		}
	})
	return errors
}

/** Cuántas veces se pidió `GET …/status` de cada app (por la ruta). */
function countStatusRequests(page: Page): Map<string, number> {
	const counts = new Map<string, number>()
	page.on('request', (request) => {
		const path = new URL(request.url()).pathname
		if (request.method() === 'GET' && path.startsWith('/api/v1/') && path.endsWith('/status')) {
			counts.set(path, (counts.get(path) ?? 0) + 1)
		}
	})
	return counts
}

async function login(page: Page): Promise<void> {
	await page.goto('/login')
	await page.getByLabel('Usuario').fill(USERNAME)
	await page.getByLabel('Contraseña').fill(PASSWORD)
	await page.getByRole('button', { name: 'Continuar' }).click()
	await expect(page.getByRole('heading', { name: 'Proyectos', level: 1 })).toBeVisible()
}

const row = (page: Page, project: string, stage: string, app: string) =>
	page.locator(`a[href="/projects/${project}/stages/${stage}/apps/${app}"]`)

const statusPath = (project: string, stage: string, app: string) =>
	`/api/v1/projects/${project}/stages/${stage}/apps/${app}/status`

test('Proyectos muestra el estado, la imagen que corre y el último deploy de cada app', async ({
	page,
}) => {
	const errors = watchErrors(page)
	await login(page)

	const api = row(page, 'auditfy', 'staging', 'api')
	await expect(api).toContainText('Corriendo · 2/2')
	// La imagen del deploy actual (la del spec es backend-app:sha-3f9c2e1).
	await expect(api).toContainText(/ghcr\.io\/auditfy\/api:/)
	await expect(api).toContainText(/#14\d · /)
	// Lo del catálogo sigue: la app tiene un cambio de después del deploy.
	await expect(api).toContainText('Cambios pendientes')

	const agents = row(page, 'aumi-agents', 'staging', 'agents')
	await agents.scrollIntoViewIfNeeded()
	await expect(agents).toContainText('Sin desplegar')
	await expect(agents).toContainText('Sin deploys')
	// Sin deploys, la imagen es lo del catálogo: los servicios del compose.
	await expect(agents).toContainText('gateway · runner')

	expect(errors).toEqual([])
})

test('Proyectos pide el estado una vez por app, y la de abajo recién cuando se ve', async ({
	page,
}) => {
	await page.setViewportSize({ width: 1280, height: 480 })
	const counts = countStatusRequests(page)
	await login(page)

	await expect(row(page, 'auditfy', 'staging', 'api')).toContainText('Corriendo')
	const last = statusPath('aumi-agents', 'production', 'agents')
	// Abajo de la pantalla: todavía no se pidió.
	await page.waitForTimeout(500)
	expect(counts.get(last) ?? 0).toBe(0)

	const agents = row(page, 'aumi-agents', 'production', 'agents')
	await agents.scrollIntoViewIfNeeded()
	await expect(agents).not.toContainText('—', { timeout: 5000 })
	await expect.poll(() => counts.get(last) ?? 0).toBe(1)

	// Volver arriba y abajo no vuelve a pedir nada: una vez por fila (contrato §2.8).
	await page.mouse.wheel(0, -5000)
	await page.mouse.wheel(0, 5000)
	await page.waitForTimeout(300)
	for (const [path, count] of counts) expect(count, path).toBe(1)
})
