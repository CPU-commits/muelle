import { expect, test, type Page } from '@playwright/test'

// Ajustes y Ajustes · Tokens (F1-identity-admin) sobre el build con MSW y la CSP. Sus rutas
// no tienen <ShellHeader>, pero se llega a ellas desde una que sí (el detalle de una app):
// el header tiene que volver a la nav de siempre sin errores.
const USERNAME = 'felipe'
const PASSWORD = 'una-contraseña-larga'

function watchErrors(page: Page): string[] {
	const errors: string[] = []
	page.on('pageerror', (error) => errors.push(error.message))
	page.on('console', (message) => {
		// Un 4xx de fetch es esperado (CLAUDE.md, "Cosas que conviene saber").
		if (message.type() === 'error' && !message.text().startsWith('Failed to load resource')) {
			errors.push(message.text())
		}
	})
	return errors
}

async function login(page: Page): Promise<void> {
	await page.goto('/login')
	await page.getByLabel('Usuario').fill(USERNAME)
	await page.getByLabel('Contraseña').fill(PASSWORD)
	await page.getByRole('button', { name: 'Continuar' }).click()
	await expect(page.getByRole('heading', { name: 'Proyectos' })).toBeVisible()
}

const sections = (page: Page) => page.getByRole('navigation', { name: 'Secciones', exact: true })
const settingsNav = (page: Page) => page.getByRole('navigation', { name: 'Secciones de ajustes' })
const appTabs = (page: Page) => page.getByRole('navigation', { name: 'Secciones de la app' })

test('de una app a Ajustes y a Tokens: el header vuelve a la nav de siempre', async ({ page }) => {
	const errors = watchErrors(page)
	await login(page)

	await page.goto('/projects/auditfy/stages/staging/apps/api/logs')
	await expect(appTabs(page)).toBeVisible()
	await page.goto('/ajustes')
	await expect(page.getByRole('heading', { name: 'Ajustes', level: 1 })).toBeVisible()
	await expect(appTabs(page)).toHaveCount(0)
	await expect(
		sections(page).getByRole('link', { name: 'Ajustes', exact: true }),
	).toHaveAttribute('aria-current', 'page')

	await settingsNav(page).getByRole('link', { name: 'Usuarios' }).click()
	await expect(page).toHaveURL(/\/ajustes#usuarios$/)
	await expect(settingsNav(page).getByRole('link', { name: 'Usuarios' })).toHaveAttribute(
		'aria-current',
		'page',
	)

	await settingsNav(page).getByRole('link', { name: 'Tokens de API' }).click()
	await expect(page).toHaveURL(/\/ajustes\/tokens$/)
	await expect(page.getByRole('heading', { name: 'Tokens de API', level: 2 })).toBeVisible()

	await sections(page).getByRole('link', { name: 'Proyectos', exact: true }).click()
	await expect(page.getByRole('heading', { name: 'Proyectos' })).toBeVisible()

	expect(errors).toEqual([])
})

test('la URL de una invitación no se pierde con Escape, ni con varios seguidos', async ({
	page,
}) => {
	const errors = watchErrors(page)
	await login(page)
	await page.goto('/ajustes#usuarios')

	await page.getByRole('button', { name: 'Invitar' }).click()
	const dialog = page.getByRole('dialog', { name: 'Invitación creada' })
	const url = dialog.getByLabel('Link de invitación')
	await expect(url).toHaveValue(/\/invite\/[\w-]+$/)
	const invite = await url.inputValue()

	for (let press = 0; press < 6; press += 1) await page.keyboard.press('Escape')

	await expect(dialog).toBeVisible()
	await expect(url).toHaveValue(invite)
	await dialog.getByRole('button', { name: 'Listo' }).click()
	await expect(dialog).toHaveCount(0)

	expect(errors).toEqual([])
})

test('una lista de IPs que te deja fuera pide confirmar, y confirmada se guarda', async ({
	page,
}) => {
	const errors = watchErrors(page)
	await login(page)
	await page.goto('/ajustes')

	// El mock ve al navegador como 198.51.100.17.
	const security = page.getByRole('region', { name: 'Seguridad', exact: true })
	const cidrs = security.getByLabel('IPs permitidas para el panel')
	await cidrs.fill('10.0.0.0/8')
	await expect(security.getByText(/198\.51\.100\.17, no queda incluida/)).toBeVisible()
	await security.getByRole('button', { name: 'Guardar' }).click()

	// Lo decide el backend (409 would_lock_out): cancelar no guarda nada.
	const dialog = page.getByRole('dialog', { name: 'Te quedarías fuera del panel' })
	await expect(dialog).toBeVisible()
	await expect(dialog.getByText(/desde 198\.51\.100\.17/)).toBeVisible()
	// Cómo volver a entrar desde el servidor (contrato §5.4), para copiarlo.
	await expect(dialog.getByText(/muelle settings reset-panel-ips/)).toBeVisible()
	await dialog.getByRole('button', { name: 'Cancelar' }).click()
	await expect(dialog).toHaveCount(0)
	await expect(cidrs).toHaveValue('10.0.0.0/8')

	await security.getByRole('button', { name: 'Guardar' }).click()
	await dialog.getByRole('button', { name: 'Guardar igual' }).click()
	await expect(dialog).toHaveCount(0)
	await expect(page.getByText('Seguridad guardada')).toBeVisible()
	await expect(security.getByRole('button', { name: 'Guardar' })).toBeDisabled()

	// Quedó guardado: sobrevive a recargar (el mock vive en sessionStorage).
	await page.reload()
	await expect(
		page
			.getByRole('region', { name: 'Seguridad', exact: true })
			.getByLabel('IPs permitidas para el panel'),
	).toHaveValue('10.0.0.0/8')

	expect(errors).toEqual([])
})

test('cambiar los puertos espera a que se apliquen, diciendo que está aplicando', async ({
	page,
}) => {
	const errors = watchErrors(page)
	await login(page)
	await page.goto('/ajustes')

	const ports = page.getByRole('region', { name: 'Puertos del proxy' })
	await ports.getByLabel('HTTP', { exact: true }).fill('8080')
	await ports.getByRole('button', { name: 'Guardar' }).click()

	const dialog = page.getByRole('dialog', { name: '¿Cambiar los puertos?' })
	await dialog.getByRole('button', { name: 'Cambiar puertos' }).click()
	// El mock tarda un poco en "aplicar", como el backend (que puede tardar hasta 2 min).
	await expect(dialog.getByText('Aplicando los puertos…')).toBeVisible()
	await expect(dialog).toHaveCount(0)
	await expect(page.getByText('Puertos guardados')).toBeVisible()
	await expect(ports.getByLabel('HTTP', { exact: true })).toHaveValue('8080')

	// Solo cambió el HTTP: el panel sigue acá y sigue a muelle-caddy (GET /proxy) mientras
	// se recrea. El mock tarda unos segundos; el panel pregunta cada 10.
	await expect(ports.getByText('Recreando muelle-caddy con los puertos nuevos…')).toBeVisible()
	await expect(ports.getByText('muelle-caddy ya escucha en los puertos nuevos.')).toBeVisible({
		timeout: 20_000,
	})
	await expect(ports.getByText(/La dirección del panel ahora es/)).toHaveCount(0)

	expect(errors).toEqual([])
})

test('cambiar el HTTPS avisa a dónde se muda el panel, y el aviso sobrevive a recargar hasta cerrarlo', async ({
	page,
}) => {
	const errors = watchErrors(page)
	await login(page)
	await page.goto('/ajustes')

	const ports = page.getByRole('region', { name: 'Puertos del proxy' })
	await ports.getByLabel('HTTPS', { exact: true }).fill('8443')
	await ports.getByRole('button', { name: 'Guardar' }).click()

	// Antes de confirmar: a dónde va el panel y que ese puerto tiene que estar abierto.
	const dialog = page.getByRole('dialog', { name: '¿Cambiar los puertos?' })
	await expect(dialog.getByText('El panel pasa a https://panel.tudominio.cl:8443')).toBeVisible()
	await expect(dialog.getByText(/Abre el puerto 8443 en el firewall/)).toBeVisible()
	await dialog.getByRole('button', { name: 'Cambiar puertos' }).click()
	await expect(dialog).toHaveCount(0)

	const notice = 'La dirección del panel ahora es https://panel.tudominio.cl:8443'
	await expect(ports.getByText(notice)).toBeVisible()
	// El origen permitido sigue al puerto HTTPS (contrato §0.1): no hay nada que tocar en el
	// servidor.
	await expect(ports.getByText(/MUELLE_PANEL_ORIGIN/)).toHaveCount(0)
	await expect(ports.getByRole('link', { name: 'Ir al panel' })).toHaveAttribute(
		'href',
		'https://panel.tudominio.cl:8443/ajustes?panel_moved=1',
	)

	await page.reload()
	const reloaded = page.getByRole('region', { name: 'Puertos del proxy' })
	await expect(reloaded.getByText(notice)).toBeVisible()

	await reloaded.getByRole('button', { name: 'Entendido' }).click()
	await expect(reloaded.getByText(notice)).toHaveCount(0)
	await page.reload()
	await expect(page.getByRole('region', { name: 'Puertos del proxy' })).toBeVisible()
	await expect(page.getByText(notice)).toHaveCount(0)

	expect(errors).toEqual([])
})

test('un puerto ocupado en el host (422) queda en su campo y no se guarda nada', async ({
	page,
}) => {
	const errors = watchErrors(page)
	await login(page)
	await page.goto('/ajustes')

	// El mock tiene el 8081 ocupado: el PATCH lo prueba antes de guardar (contrato §3.5).
	const ports = page.getByRole('region', { name: 'Puertos del proxy' })
	const httpPort = ports.getByLabel('HTTP', { exact: true })
	await httpPort.fill('8081')
	await ports.getByRole('button', { name: 'Guardar' }).click()
	const dialog = page.getByRole('dialog', { name: '¿Cambiar los puertos?' })
	await dialog.getByRole('button', { name: 'Cambiar puertos' }).click()

	await expect(dialog).toHaveCount(0)
	await expect(
		ports.getByText('el puerto 8081 del host ya está en uso: libéralo o elige otro'),
	).toBeVisible()
	await expect(httpPort).toHaveAttribute('aria-invalid', 'true')
	await expect(page.getByText('Puertos guardados')).toHaveCount(0)
	await expect(ports.getByText('Recreando muelle-caddy con los puertos nuevos…')).toHaveCount(0)

	await page.reload()
	await expect(
		page.getByRole('region', { name: 'Puertos del proxy' }).getByLabel('HTTP', { exact: true }),
	).toHaveValue('80')

	expect(errors).toEqual([])
})

test('si muelle-caddy no arranca con el HTTPS nuevo, muelle vuelve al anterior y Ajustes ofrece esa dirección', async ({
	page,
}) => {
	const errors = watchErrors(page)
	await login(page)
	await page.goto('/ajustes')

	// El 9443 pasa la prueba del PATCH, pero otro lo toma antes de recrear muelle-caddy.
	const ports = page.getByRole('region', { name: 'Puertos del proxy' })
	await ports.getByLabel('HTTPS', { exact: true }).fill('9443')
	await ports.getByRole('button', { name: 'Guardar' }).click()
	const dialog = page.getByRole('dialog', { name: '¿Cambiar los puertos?' })
	await dialog.getByRole('button', { name: 'Cambiar puertos' }).click()
	await expect(dialog).toHaveCount(0)
	const notice = 'La dirección del panel ahora es https://panel.tudominio.cl:9443'
	await expect(ports.getByText(notice)).toBeVisible()

	// El panel sigue GET /proxy (cada 10 s): muelle volvió a 80/443 (ports_rollback).
	await expect(
		ports.getByText('muelle volvió a los puertos anteriores (HTTP 80, HTTPS 443).'),
	).toBeVisible({ timeout: 25_000 })
	await expect(
		ports.getByText(/HTTPS 9443: los puertos 80 o 9443 del host ya están en uso/),
	).toBeVisible()
	await expect(
		ports.getByText(/La dirección del panel sigue siendo https:\/\/panel\.tudominio\.cl\./),
	).toBeVisible()
	await expect(
		ports.getByRole('link', { name: 'Volver a la dirección anterior' }),
	).toHaveAttribute('href', 'https://panel.tudominio.cl/ajustes')
	await expect(ports.getByText(notice)).toHaveCount(0)
	await expect(ports.getByLabel('HTTPS', { exact: true })).toHaveValue('443')

	// Lo sigue diciendo al recargar (GET /proxy lo trae hasta el próximo cambio de puertos),
	// sin el aviso de la dirección que falló.
	await page.reload()
	const reloaded = page.getByRole('region', { name: 'Puertos del proxy' })
	await expect(
		reloaded.getByText('muelle volvió a los puertos anteriores (HTTP 80, HTTPS 443).'),
	).toBeVisible()
	await expect(reloaded.getByText(notice)).toHaveCount(0)
	await expect(reloaded.getByLabel('HTTPS', { exact: true })).toHaveValue('443')

	expect(errors).toEqual([])
})

test('Cloudflare caído (502) es un aviso en el modal para reintentar; un token mal formado se queda en el campo', async ({
	page,
}) => {
	const errors = watchErrors(page)
	await login(page)
	await page.goto('/ajustes')

	const cloudflare = page.getByRole('region', { name: 'Cloudflare' })
	await cloudflare.getByRole('button', { name: 'Reemplazar token' }).click()
	const dialog = page.getByRole('dialog', { name: 'Token de Cloudflare' })
	const token = dialog.getByLabel('Token de API')

	// La forma la mira el panel antes de mandarlo: no sale, y el campo no se borra.
	await token.fill('corto')
	await dialog.getByRole('button', { name: 'Guardar' }).click()
	await expect(
		dialog.getByText('Un token de Cloudflare tiene entre 20 y 512 caracteres.'),
	).toBeVisible()
	await expect(token).toHaveValue('corto')

	// Con "caido", el mock responde como Cloudflare caído: 502 cloudflare_unavailable.
	await token.fill('cf-token-de-prueba-caido-0123456789')
	await dialog.getByRole('button', { name: 'Guardar' }).click()
	await expect(dialog.getByText('Cloudflare no respondió')).toBeVisible()
	await expect(dialog.getByText(/El token no se guardó\. Pégalo otra vez/)).toBeVisible()
	// Ya salió: no se queda en el campo.
	await expect(token).toHaveValue('')

	await token.fill('cf-token-de-prueba-nuevo-0123456789')
	await dialog.getByRole('button', { name: 'Guardar' }).click()
	await expect(dialog).toHaveCount(0)
	await expect(page.getByText('Token de Cloudflare guardado')).toBeVisible()

	expect(errors).toEqual([])
})

test('el token de Cloudflare se puede quitar', async ({ page }) => {
	const errors = watchErrors(page)
	await login(page)
	await page.goto('/ajustes')

	const cloudflare = page.getByRole('region', { name: 'Cloudflare' })
	await expect(cloudflare.getByText('Token configurado')).toBeVisible()
	await cloudflare.getByRole('button', { name: 'Quitar token' }).click()
	const dialog = page.getByRole('dialog', { name: '¿Quitar el token de Cloudflare?' })
	await dialog.getByRole('button', { name: 'Quitar token' }).click()

	await expect(dialog).toHaveCount(0)
	await expect(cloudflare.getByText('Sin token', { exact: true })).toBeVisible()
	await expect(cloudflare.getByRole('button', { name: 'Quitar token' })).toHaveCount(0)
	await expect(cloudflare.getByRole('button', { name: 'Configurar token' })).toBeVisible()

	expect(errors).toEqual([])
})
