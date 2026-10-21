import { expect, type Page, test } from '@playwright/test'

// El catálogo de punta a punta contra el build con MSW (servido con la CSP): crear un
// proyecto, un stage y una app, editar su configuración (con el aviso de cambios sin
// guardar), cambiar de app con los selectores del breadcrumb y editar un compose (el
// editor, CodeMirror, se baja recién ahí).
const USERNAME = 'felipe'
const PASSWORD = 'una-contraseña-larga'

interface Watch {
	errors: string[]
	csp: string[]
}

async function watch(page: Page): Promise<Watch> {
	const found: Watch = { errors: [], csp: [] }
	page.on('pageerror', (error) => found.errors.push(error.message))
	page.on('console', (message) => {
		const text = message.text()
		if (/Content Security Policy/i.test(text)) found.csp.push(text)
		// Un 4xx de fetch es esperado (CLAUDE.md, "Cosas que conviene saber").
		else if (message.type() === 'error' && !text.startsWith('Failed to load resource')) {
			found.errors.push(text)
		}
	})
	await page.exposeFunction('reportCspViolation', (text: string) => found.csp.push(text))
	await page.addInitScript(() => {
		document.addEventListener('securitypolicyviolation', (event) => {
			const report = (window as unknown as { reportCspViolation: (text: string) => void })
				.reportCspViolation
			report(`${event.effectiveDirective}: ${event.blockedURI}`)
		})
	})
	return found
}

async function login(page: Page): Promise<void> {
	await page.goto('/login')
	await page.getByLabel('Usuario').fill(USERNAME)
	await page.getByLabel('Contraseña').fill(PASSWORD)
	await page.getByRole('button', { name: 'Continuar' }).click()
	await expect(page.getByRole('heading', { name: 'Proyectos', level: 1 })).toBeVisible()
}

const card = (page: Page, id: string) => page.locator(`section#${id}`)

test('crear proyecto, stage y app, y configurarla', async ({ page }) => {
	const found = await watch(page)
	await login(page)

	// Proyecto: el slug se sugiere a partir del nombre.
	await page.getByRole('button', { name: 'Nuevo proyecto' }).click()
	const projectDialog = page.getByRole('dialog', { name: 'Nuevo proyecto' })
	await projectDialog.getByLabel('Nombre').fill('Tienda Online')
	await expect(projectDialog.getByLabel('Slug')).toHaveValue('tienda-online')
	await projectDialog.getByRole('button', { name: 'Crear' }).click()
	await expect(page.getByRole('heading', { name: 'Tienda Online', level: 2 })).toBeVisible()

	// Stage, desde las opciones del proyecto.
	await page.getByRole('button', { name: 'Opciones de tienda-online' }).click()
	await page.getByRole('menuitem', { name: 'Nuevo stage' }).click()
	await page.getByRole('dialog').getByLabel('Slug').fill('qa')
	await page.getByRole('dialog').getByRole('button', { name: 'Crear' }).click()
	await expect(page.getByRole('heading', { name: 'qa', level: 3 })).toBeVisible()

	// App de formulario: con una imagen inválida no se crea; corregida, lleva a Configuración.
	await page.getByRole('button', { name: 'Nueva app en tienda-online/qa' }).click()
	const appDialog = page.getByRole('dialog')
	await appDialog.getByLabel('Slug').fill('web')
	await appDialog.getByLabel('Imagen').fill('Nginx:latest')
	await appDialog.getByLabel('Puerto').fill('80')
	await appDialog.getByRole('button', { name: 'Crear' }).click()
	await expect(
		appDialog.getByText('No es una referencia válida de Docker', { exact: false }),
	).toBeVisible()
	await appDialog.getByLabel('Imagen').fill('nginx:1.27-alpine')
	await appDialog.getByRole('button', { name: 'Crear' }).click()
	await expect(page).toHaveURL(/\/projects\/tienda-online\/stages\/qa\/apps\/web\/configuracion$/)
	await expect(page.getByRole('heading', { name: 'Configuración', level: 1 })).toBeVisible()

	// Una tarjeta se guarda sola.
	await page.getByLabel('Réplicas').fill('2')
	await card(page, 'c-ejecucion').getByRole('button', { name: 'Guardar' }).click()
	await expect(page.getByText('Cambios guardados')).toBeVisible()

	// Con un cambio sin guardar, irse pregunta: "no" se queda, "sí" se va. Un solo handler:
	// el `click` puede terminar antes de que llegue el evento del diálogo.
	await page.getByLabel('Réplicas').fill('3')
	const tabs = page.getByRole('navigation', { name: 'Secciones de la app' })
	const asked: string[] = []
	let leave = false
	page.on('dialog', (dialog) => {
		asked.push(dialog.message())
		void (leave ? dialog.accept() : dialog.dismiss())
	})
	await tabs.getByRole('link', { name: 'Logs' }).click()
	await expect.poll(() => asked.length).toBe(1)
	await expect(page).toHaveURL(/\/configuracion$/)
	await expect(page.getByLabel('Réplicas')).toHaveValue('3')
	leave = true
	await tabs.getByRole('link', { name: 'Logs' }).click()
	await expect(page).toHaveURL(/\/apps\/web\/logs$/)
	expect(asked).toEqual([
		'Tienes cambios sin guardar. ¿Salir igual y perderlos?',
		'Tienes cambios sin guardar. ¿Salir igual y perderlos?',
	])

	expect(found.errors).toEqual([])
	expect(found.csp).toEqual([])
})

test('los selectores del breadcrumb cambian de app en el mismo tab', async ({ page }) => {
	const found = await watch(page)
	await login(page)

	await page.goto('/projects/auditfy/stages/staging/apps/api/configuracion')
	const location = page.getByRole('navigation', { name: 'Ubicación' })

	await location.getByRole('button', { name: 'Stage staging, cambiar' }).click()
	// El stage actual lleva la marca y aria-current; los otros, no.
	await expect(page.getByRole('menuitem', { name: 'staging', exact: true })).toHaveAttribute(
		'aria-current',
		'page',
	)
	await expect(page.getByRole('menuitem', { name: 'production' })).not.toHaveAttribute(
		'aria-current',
	)
	await page.getByRole('menuitem', { name: 'production' }).click()
	await expect(page).toHaveURL(/\/auditfy\/stages\/production\/apps\/api\/configuracion$/)

	await location.getByRole('button', { name: 'App api, cambiar' }).click()
	await page.getByRole('menuitem', { name: 'redis' }).click()
	await expect(page).toHaveURL(/\/auditfy\/stages\/production\/apps\/redis\/configuracion$/)

	await location.getByRole('button', { name: 'Proyecto auditfy, cambiar' }).click()
	await page.getByRole('menuitem', { name: 'extractor' }).click()
	await expect(page).toHaveURL(/\/extractor\/stages\/production\/apps\/api\/configuracion$/)

	expect(found.errors).toEqual([])
})

test('las variables de un stage tienen los mismos selectores: llevan a las del otro', async ({
	page,
}) => {
	const found = await watch(page)
	await login(page)

	await page.goto('/projects/auditfy/stages/staging/variables')
	await expect(page.getByRole('heading', { name: 'Variables del stage' })).toBeVisible()
	const location = page.getByRole('navigation', { name: 'Ubicación' })

	await location.getByRole('button', { name: 'Stage staging, cambiar' }).click()
	await page.getByRole('menuitem', { name: 'production' }).click()
	await expect(page).toHaveURL(/\/auditfy\/stages\/production\/variables$/)
	await expect(
		location.getByRole('button', { name: 'Stage production, cambiar' }),
	).toHaveAttribute('aria-current', 'page')

	await location.getByRole('button', { name: 'Proyecto auditfy, cambiar' }).click()
	await page.getByRole('menuitem', { name: 'extractor' }).click()
	await expect(page).toHaveURL(/\/extractor\/stages\/production\/variables$/)
	await expect(page.getByRole('heading', { name: 'Variables del stage' })).toBeVisible()

	// "Proyectos", arriba del menú del proyecto, vuelve a la lista.
	await location.getByRole('button', { name: 'Proyecto extractor, cambiar' }).click()
	await page.getByRole('menuitem', { name: 'Proyectos' }).click()
	await expect(page.getByRole('heading', { name: 'Proyectos', level: 1 })).toBeVisible()

	expect(found.errors).toEqual([])
})

test('el compose se edita en CodeMirror, sin violar la CSP', async ({ page }) => {
	const found = await watch(page)
	await login(page)

	await page.goto('/projects/aumi-agents/stages/staging/apps/agents/configuracion')
	const editor = page.locator('.cm-content')
	await expect(editor).toBeVisible()

	// Con estilos: los de CodeMirror (el scroller es flex) y el tema del panel (Geist Mono,
	// el fondo de página), aplicados adentro del shadow root sin violar la CSP.
	const styles = await page.locator('.cm-scroller').evaluate((element) => {
		const scroller = getComputedStyle(element)
		const editorBox = getComputedStyle(element.closest('.cm-editor') as Element)
		return {
			display: scroller.display,
			font: scroller.fontFamily,
			background: editorBox.backgroundColor,
		}
	})
	expect(styles.display).toBe('flex')
	expect(styles.font).toContain('Geist Mono')
	expect(styles.background).toBe('rgb(10, 10, 11)')
	await expect(page.locator('.cm-lineNumbers .cm-gutterElement').nth(1)).toHaveText('1')

	// Un servicio repetido no se guarda: se marca la línea.
	// `insertText` escribe tal cual, sin la indentación automática del editor.
	await editor.click()
	await page.keyboard.press('Control+Home')
	await page.keyboard.press('End')
	await page.keyboard.insertText('\n  gateway:\n    image: otra')
	await page.getByRole('button', { name: 'Guardar', exact: true }).click()
	await expect(page.getByRole('alert').getByText(/No se guardó/)).toBeVisible()
	await expect(page.getByText(/El servicio «gateway» está repetido/)).toBeVisible()

	// Descartar vuelve al guardado; un cambio válido se guarda.
	await page.getByRole('button', { name: 'Descartar' }).click()
	await editor.click()
	await page.keyboard.press('Control+End')
	await page.keyboard.insertText('# revisado\n')
	await page.getByRole('button', { name: 'Guardar', exact: true }).click()
	await expect(page.getByText('Compose guardado')).toBeVisible()

	expect(found.errors).toEqual([])
	expect(found.csp).toEqual([])
})
