import { expect, type Page } from '@playwright/test'

// El usuario demo del mock, el login y la vigilancia de la página, para `csp.e2e.ts`.

/** El usuario demo de src/mocks/db/auth.ts (sin 2FA). */
export const USERNAME = 'felipe'
export const PASSWORD = 'una-contraseña-larga'

/**
 * Junta lo que no puede pasar en ninguna página que abra `page`: una violación de CSP, un
 * error sin atrapar o en la consola (un 4xx de fetch es esperado: CLAUDE.md, "Cosas que
 * conviene saber") y un pedido con los params vacíos (`/projects/undefined/…`). El test
 * termina con `expect(problems).toEqual([])`.
 */
export async function watch(page: Page): Promise<string[]> {
	const problems: string[] = []
	page.on('pageerror', (error) => problems.push(`pageerror: ${error.message}`))
	page.on('console', (message) => {
		const text = message.text()
		// Chrome anota cada bloqueo en la consola ("Refused to … Content Security Policy").
		if (/Content Security Policy/i.test(text)) problems.push(`csp: ${text}`)
		else if (message.type() === 'error' && !text.startsWith('Failed to load resource')) {
			problems.push(`console: ${text}`)
		}
	})
	page.on('request', (request) => {
		if (request.url().includes('/undefined')) problems.push(`pedido: ${request.url()}`)
	})
	// El documento también dispara `securitypolicyviolation`, aunque el código lo atrape (así
	// se ve la prueba de `new Function` de Zod). `exposeFunction` sobrevive a las navegaciones
	// y no lo frena la CSP.
	await page.exposeFunction('reportCspViolation', (text: string) => problems.push(`csp: ${text}`))
	await page.addInitScript(() => {
		document.addEventListener('securitypolicyviolation', (event) => {
			const report = (window as unknown as { reportCspViolation: (text: string) => void })
				.reportCspViolation
			report(
				`${event.effectiveDirective}: ${event.blockedURI} (${event.sourceFile}:${event.lineNumber})`,
			)
		})
	})
	return problems
}

/** Entra con el usuario demo y espera Proyectos. */
export async function login(page: Page): Promise<void> {
	await page.goto('/login')
	await page.getByLabel('Usuario').fill(USERNAME)
	await page.getByLabel('Contraseña').fill(PASSWORD)
	await page.getByRole('button', { name: 'Continuar' }).click()
	await expect(page.getByRole('heading', { name: 'Proyectos', level: 1 })).toBeVisible()
}
