import { expect, test, type Page } from '@playwright/test'
import { PROD_URL } from './ports'

// El panel bajo la CSP que manda el servidor (el `csp-header.txt` del build, ver
// svelte.config.js y scripts/serve-build.mjs). Cualquier violación (un script en línea
// sin hash, un `style="…"` en el HTML, el `new Function` de Zod, un `data:`) rompe el test.

const USERNAME = 'felipe'
const PASSWORD = 'una-contraseña-larga'

/** Junta las violaciones de CSP de todas las páginas que abra `page`. */
async function watchCsp(page: Page): Promise<string[]> {
	const violations: string[] = []
	// Chrome anota cada bloqueo en la consola ("Refused to … Content Security Policy").
	page.on('console', (message) => {
		if (/Content Security Policy/i.test(message.text())) violations.push(message.text())
	})
	// Y el documento dispara `securitypolicyviolation`, aunque el código lo atrape (así
	// se ve la prueba de `new Function` de Zod). `exposeFunction` sobrevive a las
	// navegaciones y no lo frena la CSP.
	await page.exposeFunction('reportCspViolation', (text: string) => violations.push(text))
	await page.addInitScript(() => {
		document.addEventListener('securitypolicyviolation', (event) => {
			const report = (window as unknown as { reportCspViolation: (text: string) => void })
				.reportCspViolation
			report(
				`${event.effectiveDirective}: ${event.blockedURI} (${event.sourceFile}:${event.lineNumber})`,
			)
		})
	})
	return violations
}

test('el build de producción arranca bajo la CSP del header', async ({ page }) => {
	const violations = await watchCsp(page)

	const response = await page.goto(`${PROD_URL}/`)
	const header = response?.headers()['content-security-policy'] ?? ''
	expect(header).toContain("default-src 'self'")
	expect(header).toMatch(/script-src 'self' 'sha256-[^']+'/)
	expect(header).toContain("frame-ancestors 'none'")

	// Sin backend `/auth/me` es 401: el guard tiene que haber corrido y mandado al login.
	await expect(page).toHaveURL(/\/login$/)
	await expect(page.getByRole('heading', { name: 'Entrar a muelle' })).toBeVisible()
	await page.getByLabel('Usuario').fill(USERNAME)
	await expect(page.getByLabel('Usuario')).toHaveValue(USERNAME)

	expect(violations).toEqual([])
})

test('el build de producción no trae el service worker de MSW', async ({ request }) => {
	// El archivo no existe: el servidor devuelve el `200.html` en su lugar.
	const response = await request.get(`${PROD_URL}/mockServiceWorker.js`)
	expect(response.headers()['content-type']).toContain('text/html')
})

test('recorrer el panel no viola la CSP', async ({ page }) => {
	const violations = await watchCsp(page)

	await page.goto('/login')
	await page.getByLabel('Usuario').fill(USERNAME)
	await page.getByLabel('Contraseña').fill(PASSWORD)
	await page.getByRole('button', { name: 'Continuar' }).click()
	await expect(page.getByRole('heading', { name: 'Proyectos' })).toBeVisible()

	await page.goto('/projects/auditfy/stages/staging/apps/api/logs')
	await expect(page.getByRole('navigation', { name: 'Secciones de la app' })).toBeVisible()

	// El menú de la cuenta va en el top layer (popover) y se posiciona con CSSOM.
	await page.getByRole('button', { name: `Cuenta: ${USERNAME}` }).click()
	await expect(page.getByRole('menuitem', { name: 'Cerrar sesión' })).toBeVisible()
	await page.keyboard.press('Escape')

	// El QR de /2fa es un <svg> armado en el cliente.
	await page.goto('/2fa')
	await expect(
		page.getByRole('img', { name: 'Código QR para la app de autenticación' }),
	).toBeVisible()

	expect(violations).toEqual([])
})
