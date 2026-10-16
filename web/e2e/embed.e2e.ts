import { expect, test } from '@playwright/test'
import { PROD_URL } from './ports'

// El build de producción tal como lo sirve el binario de Go con `-tags panel` (contrato
// §5.5, `src/cmd/http/panel/panel.go`), copiado a mano en `scripts/serve-build.mjs`.
//
// Lo que prueba del BUILD: que pide todo con rutas absolutas bajo `/_app/immutable/` (lo que
// necesita panel.go para servir el mismo `200.html` en cualquier ruta, y para darle un año de
// caché) y que `csp-header.txt` trae `frame-ancestors 'none'`. El resto (los headers, el 404
// de un chunk, el 405) lo contesta serve-build.mjs: esos tests DOCUMENTAN lo que hace
// panel.go, no lo verifican. Si serve-build.mjs se aleja de panel.go, siguen pasando; lo que
// sí lo verifica son los tests de Go del handler (`panel_test.go`).

test('el shell sale en cualquier ruta, con los headers del panel y sin caché', async ({
	request,
}) => {
	for (const path of ['/', '/projects/auditfy/stages/staging/apps/api/dominios']) {
		const response = await request.get(`${PROD_URL}${path}`)
		expect(response.status()).toBe(200)
		const headers = response.headers()
		expect(headers['content-type']).toBe('text/html; charset=utf-8')
		expect(headers['content-security-policy']).toContain("frame-ancestors 'none'")
		expect(headers['x-content-type-options']).toBe('nosniff')
		// Los links con secretos (/setup?token=, /invite/<token>) nunca salen del origen.
		expect(headers['referrer-policy']).toBe('same-origin')
		expect(headers['cache-control']).toBe('no-cache')
	}
})

test('el shell pide todo con rutas absolutas bajo /_app/immutable/ (un año de caché)', async ({
	request,
}) => {
	const shell = await (await request.get(`${PROD_URL}/`)).text()
	const assets = [...shell.matchAll(/(?:href|src)="([^"]+)"/g)].map((match) => match[1] ?? '')
	expect(assets.length).toBeGreaterThan(0)
	// Absolutas: el mismo 200.html sirve /projects/…/dominios sin que se rompan.
	for (const asset of assets) expect(asset).toMatch(/^\/_app\/immutable\//)

	const script = assets.find((asset) => asset.endsWith('.js')) ?? ''
	const response = await request.get(`${PROD_URL}${script}`)
	expect(response.status()).toBe(200)
	expect(response.headers()['content-type']).toBe('text/javascript; charset=utf-8')
	expect(response.headers()['cache-control']).toBe('public, max-age=31536000, immutable')
})

test('un chunk que no existe es 404 en texto, y csp-header.txt no se sirve', async ({
	request,
}) => {
	// Un chunk viejo nunca recibe HTML (con nosniff, el navegador lo rechazaría peor).
	const chunk = await request.get(`${PROD_URL}/_app/immutable/chunks/no-existe.js`)
	expect(chunk.status()).toBe(404)
	expect(chunk.headers()['content-type'] ?? '').not.toContain('text/html')

	// La CSP va como header; el archivo no es parte del panel.
	const csp = await request.get(`${PROD_URL}/csp-header.txt`)
	expect(csp.headers()['content-type']).toBe('text/html; charset=utf-8')
	expect(await csp.text()).toContain('<!doctype html>')
})

test('solo GET y HEAD: otro método es 405 en JSON, con Allow', async ({ request }) => {
	const response = await request.post(`${PROD_URL}/projects`)
	expect(response.status()).toBe(405)
	expect(response.headers()['allow']).toBe('GET, HEAD')
	expect(await response.json()).toEqual({
		error: { code: 'method_not_allowed', message: 'método no permitido' },
	})
})
