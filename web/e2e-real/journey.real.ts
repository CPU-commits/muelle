import { expect, type Page, test } from '@playwright/test'
import { serviceEnv, stackServices } from './docker'
import { realRun } from './env'
import { TotpClock } from './totp'

// El panel embebido en un `muelle serve` de verdad (-tags panel), con un Swarm de verdad:
// lo que los e2e de `e2e/` prueban contra MSW, acá contra Go y Docker. globalSetup.ts arranca
// muelle con una base nueva y deja el link de setup; todo lo de Docker se llama it7p<id>…
// y globalSetup.ts lo borra al final, aunque el test falle.
//
// Un solo recorrido, en orden: cada paso usa lo que dejó el anterior (el usuario, el
// proyecto, los deploys). Sin dominio del panel no hay muelle-caddy: el proxy queda
// deshabilitado y los deploys no le mandan rutas.
const PASSWORD = 'una-contraseña-larga-it7'
const STAGE = 'qa'
const APP = 'web'
const IMAGE = 'nginx:alpine'
const DEPLOY_TIMEOUT = 180_000

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

test('de la instalación al borrado: setup, 2FA, catálogo, variables, deploys, rollback y auditoría', async ({
	page,
}) => {
	test.setTimeout(15 * 60_000)
	const run = realRun()
	const { project, username } = run
	const appPath = `/projects/${project}/stages/${STAGE}/apps/${APP}`
	const stack = `${project}_${STAGE}_${APP}`
	const service = `${stack}_${APP}`
	const totp = new TotpClock()
	const nextCode = async (secret: string): Promise<string> => {
		const wait = totp.waitMs()
		if (wait > 0) await page.waitForTimeout(wait + 500)
		return totp.next(secret).code
	}
	const problems = await watch(page)
	const tabs = page.getByRole('navigation', { name: 'Secciones de la app' })

	let secret = ''
	await test.step('setup del primer usuario con el link del log', async () => {
		await page.goto(run.setupUrl)
		await expect(page.getByRole('heading', { name: 'Configurar muelle' })).toBeVisible()
		// El token sale de la URL apenas se lee (replaceState a /setup).
		await expect(page).toHaveURL(/\/setup$/)
		await page.getByLabel('Usuario').fill(username)
		await page.getByLabel('Contraseña', { exact: true }).fill(PASSWORD)
		await page.getByLabel('Repetir contraseña').fill(PASSWORD)
		await page.getByRole('button', { name: 'Crear usuario' }).click()
		await expect(page).toHaveURL(/\/2fa$/)
	})

	await test.step('2FA con un código calculado de la clave que muestra el panel', async () => {
		await expect(page.getByRole('heading', { name: 'Configurar 2FA' })).toBeVisible()
		const shown = page.locator('#totp-secret')
		await expect(shown).toHaveText(/[A-Z2-7]{4}/)
		secret = (await shown.innerText()).replace(/\s/g, '')
		await page.getByLabel('Código de 6 dígitos').fill(await nextCode(secret))
		await expect(page.getByText('2FA activada')).toBeVisible()
		await expect(page.getByRole('heading', { name: 'Proyectos', level: 1 })).toBeVisible()
	})

	await test.step('cerrar sesión y entrar con contraseña y código', async () => {
		await page.getByRole('button', { name: `Cuenta: ${username}` }).click()
		await page.getByRole('menuitem', { name: 'Cerrar sesión' }).click()
		await expect(page).toHaveURL(/\/login$/)
		await page.getByLabel('Usuario').fill(username)
		await page.getByLabel('Contraseña').fill(PASSWORD)
		await page.getByRole('button', { name: 'Continuar' }).click()
		await expect(page.getByRole('heading', { name: 'Verificación en dos pasos' })).toBeVisible()
		await page.getByLabel('Código de 6 dígitos').fill(await nextCode(secret))
		await expect(page.getByRole('heading', { name: 'Proyectos', level: 1 })).toBeVisible()
	})

	await test.step('proyecto, stage y app de formulario', async () => {
		// Sin proyectos, el estado vacío trae otro botón igual.
		await page.getByRole('button', { name: 'Nuevo proyecto' }).first().click()
		const projectDialog = page.getByRole('dialog', { name: 'Nuevo proyecto' })
		await projectDialog.getByLabel('Nombre').fill(`it7 ${run.id}`)
		await projectDialog.getByLabel('Slug').fill(project)
		await projectDialog.getByRole('button', { name: 'Crear' }).click()
		await expect(page.getByRole('heading', { name: `it7 ${run.id}`, level: 2 })).toBeVisible()

		await page.getByRole('button', { name: `Opciones de ${project}` }).click()
		await page.getByRole('menuitem', { name: 'Nuevo stage' }).click()
		await page.getByRole('dialog').getByLabel('Slug').fill(STAGE)
		await page.getByRole('dialog').getByRole('button', { name: 'Crear' }).click()
		await expect(page.getByRole('heading', { name: STAGE, level: 3 })).toBeVisible()

		await page.getByRole('button', { name: `Nueva app en ${project}/${STAGE}` }).click()
		const appDialog = page.getByRole('dialog')
		await appDialog.getByLabel('Slug').fill(APP)
		await appDialog.getByLabel('Imagen').fill(IMAGE)
		await appDialog.getByLabel('Puerto').fill('80')
		await appDialog.getByRole('button', { name: 'Crear' }).click()
		await expect(page).toHaveURL(new RegExp(`${appPath}/configuracion$`))
		await expect(page.getByRole('heading', { name: 'Configuración', level: 1 })).toBeVisible()
	})

	await test.step('una variable secreta de la app', async () => {
		await tabs.getByRole('link', { name: 'Variables' }).click()
		await expect(page.getByRole('heading', { name: 'Variables', level: 1 })).toBeVisible()
		await page.getByRole('textbox', { name: 'Clave' }).fill('IT7_GREETING')
		await page.getByRole('textbox', { name: 'Valor' }).fill(`hola-${run.id}`)
		await page.getByRole('button', { name: 'Agregar', exact: true }).click()
		const row = page
			.getByRole('list', { name: 'De la app' })
			.getByRole('listitem')
			.filter({ has: page.getByText('IT7_GREETING', { exact: true }) })
		await expect(row).toContainText('valor secreto')
		await expect(page.getByText('Hay cambios sin desplegar.')).toBeVisible()
	})

	const deployAndWait = async (id: number): Promise<void> => {
		await expect(page).toHaveURL(new RegExp(`${appPath}/deploys`))
		const log = page.getByRole('region', { name: `Log del deploy #${id}` })
		await expect(log).toBeVisible()
		const selected = page.getByRole('heading', { level: 2, name: `#${id}` })
		await expect(selected).toBeVisible()
		await expect(
			page.getByRole('button', { pressed: true }).filter({ hasText: 'Exitoso' }),
		).toContainText(`#${id}`, { timeout: DEPLOY_TIMEOUT })
		await expect(log.getByRole('listitem').first()).toBeVisible()
	}

	await test.step('primer deploy desde el Resumen, seguido en Deploys hasta Exitoso', async () => {
		await tabs.getByRole('link', { name: 'Resumen' }).click()
		await expect(page.getByRole('heading', { level: 1, name: APP })).toBeVisible()
		await expect(page.getByText('Esta app todavía no tiene un deploy')).toBeVisible()
		await page.getByRole('button', { name: 'Desplegar' }).first().click()
		const dialog = page.getByRole('dialog', { name: `Desplegar ${APP}` })
		await dialog.getByRole('button', { name: 'Desplegar' }).click()
		await deployAndWait(1)
		// La variable llegó al servicio de Swarm.
		expect(serviceEnv(service)).toContain(`IT7_GREETING=hola-${run.id}`)
	})

	await test.step('el Resumen muestra el estado de Swarm', async () => {
		await tabs.getByRole('link', { name: 'Resumen' }).click()
		await expect(page.getByText('Corriendo').first()).toBeVisible({ timeout: 60_000 })
		await expect(page.getByRole('table', { name: 'Tareas de Swarm de la app' })).toContainText(
			'Running',
			{ ignoreCase: true },
		)
		await expect(page.getByText(IMAGE).first()).toBeVisible()
	})

	await test.step('los logs en vivo de la app', async () => {
		await tabs.getByRole('link', { name: 'Logs' }).click()
		await expect(page.getByRole('heading', { level: 1, name: 'Logs' })).toBeVisible()
		const lines = page.getByRole('region', { name: 'Líneas de log' }).getByRole('listitem')
		// nginx escribe al arrancar ("… ready for start up").
		await expect(lines.filter({ hasText: /start up|nginx/ }).first()).toBeVisible({
			timeout: 60_000,
		})
	})

	await test.step('segundo deploy y rollback al primero', async () => {
		await tabs.getByRole('link', { name: 'Deploys' }).click()
		await page.getByRole('button', { name: 'Desplegar' }).first().click()
		await page
			.getByRole('dialog', { name: `Desplegar ${APP}` })
			.getByRole('button', { name: 'Desplegar' })
			.click()
		await deployAndWait(2)

		await tabs.getByRole('link', { name: 'Resumen' }).click()
		await page.getByRole('button', { name: 'Rollback' }).click()
		const dialog = page.getByRole('dialog', { name: 'Rollback a #1' })
		await dialog.getByRole('button', { name: 'Hacer rollback' }).click()
		await expect(page.getByText('Rollback en cola: deploy #3')).toBeVisible()
		await tabs.getByRole('link', { name: 'Deploys' }).click()
		await page.getByRole('button', { name: /#3/ }).click()
		await deployAndWait(3)
		await expect(page.getByRole('button', { name: /#3/ })).toContainText(
			`rollback · ${username}`,
		)
	})

	await test.step('la Auditoría nombra al usuario en cada evento', async () => {
		await page.goto('/ajustes#auditoria')
		const audit = page.getByRole('table', {
			name: 'Eventos de auditoría, del más nuevo al más viejo',
		})
		await expect(audit).toBeVisible()
		const row = (action: string) =>
			audit
				.getByRole('row')
				.filter({ has: page.getByRole('cell', { name: action, exact: true }) })
		await expect(row('project.create')).toContainText(username)
		await expect(row('project.create')).toContainText(project)
		await expect(row('deploy.create').first()).toContainText(username)
		await expect(row('deploy.create')).toHaveCount(3)
		await expect(row('user.totp_enable')).toContainText(username)
	})

	await test.step('borrar la app, el stage y el proyecto', async () => {
		await page.goto(`${appPath}/configuracion`)
		await page.getByRole('button', { name: 'Eliminar', exact: true }).click()
		await page
			.getByRole('dialog', { name: `¿Eliminar la app ${APP}?` })
			.getByRole('button', { name: 'Eliminar' })
			.click()
		await expect(page.getByText(`App ${APP} eliminada`)).toBeVisible()
		// docker stack rm: Swarm baja los servicios.
		await expect.poll(() => stackServices(stack), { timeout: 60_000 }).toEqual([])

		await page.goto('/')
		await page.getByRole('button', { name: `Opciones del stage ${project}/${STAGE}` }).click()
		await page.getByRole('menuitem', { name: 'Eliminar stage' }).click()
		await page
			.getByRole('dialog', { name: `¿Eliminar el stage ${STAGE} de ${project}?` })
			.getByRole('button', { name: 'Eliminar' })
			.click()
		await expect(page.getByText(`Stage ${STAGE} eliminado`)).toBeVisible()

		await page.getByRole('button', { name: `Opciones de ${project}` }).click()
		await page.getByRole('menuitem', { name: 'Eliminar proyecto' }).click()
		await page
			.getByRole('dialog', { name: `¿Eliminar el proyecto ${project}?` })
			.getByRole('button', { name: 'Eliminar' })
			.click()
		await expect(page.getByText(`Proyecto ${project} eliminado`)).toBeVisible()
		await expect(page.getByRole('heading', { name: `it7 ${run.id}`, level: 2 })).toHaveCount(0)
	})

	expect(problems).toEqual([])
})
