import { afterAll, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { api } from '@/mocks/api'
import { adminDb } from '@/mocks/db/admin'
import { authDb, MOCK_USERNAME } from '@/mocks/db/auth'
import { ingressDb } from '@/mocks/db/ingress'
import { server } from '@/mocks/server'

// El `PATCH /settings` del mock con los certificados de Cloudflare (Origin CA): las reglas y
// los mensajes del contrato (§4.1), letra por letra, directo contra el endpoint.
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
beforeEach(() => {
	authDb.reset({ session: { username: MOCK_USERNAME, mfaPending: false, badCodes: 0 } })
	adminDb.reset()
	ingressDb.reset()
})
afterAll(() => server.close())

/** Un token con la forma de uno de Cloudflare (`sin-ssl`, `ssl-lento`: ver handlers/admin.ts). */
const token = (tag: string) => `cf_token_${tag}_de_cuarenta_caracteres`.padEnd(40, 'x')

async function patch(body: Record<string, unknown>) {
	const response = await fetch(api('/settings'), {
		method: 'PATCH',
		headers: { 'content-type': 'application/json' },
		body: JSON.stringify(body),
	})
	return { status: response.status, body: (await response.json()) as Record<string, unknown> }
}

const invalid = (message: string) => ({ error: { code: 'invalid', message } })

const PERMISSION =
	'el token no tiene el permiso Zone → SSL and Certificates → Edit en la zona tudominio.cl (lo necesitan los certificados de Cloudflare)'

describe('PATCH /settings: cloudflare_origin_ca', () => {
	it('con token, la enciende y la apaga; GET /settings la trae', async () => {
		expect(await patch({ cloudflare_origin_ca: true })).toMatchObject({
			status: 200,
			body: { cloudflare_origin_ca: true },
		})
		expect(adminDb.state.cloudflareOriginCa).toBe(true)
		expect(adminDb.state.audit.at(-1)?.details).toEqual({ cloudflare_origin_ca: true })

		expect(await patch({ cloudflare_origin_ca: false })).toMatchObject({
			status: 200,
			body: { cloudflare_origin_ca: false },
		})
		const settings = await fetch(api('/settings')).then((r) => r.json())
		expect(settings).toMatchObject({ cloudflare_origin_ca: false })
	})

	it('es un bool: otro tipo es 400 invalid_json', async () => {
		expect(await patch({ cloudflare_origin_ca: 'true' })).toMatchObject({
			status: 400,
			body: { error: { code: 'invalid_json' } },
		})
	})

	it('mandar el valor que ya tiene no es un cambio: no va a la auditoría', async () => {
		const before = adminDb.state.audit.length

		expect((await patch({ cloudflare_origin_ca: false })).status).toBe(200)

		expect(adminDb.state.audit).toHaveLength(before)
	})

	it('encenderla sin token es 422, y no guarda nada', async () => {
		adminDb.state.cloudflareToken = null

		expect(await patch({ cloudflare_origin_ca: true })).toEqual({
			status: 422,
			body: invalid(
				'cloudflare_origin_ca: necesita el token de Cloudflare: configúralo antes de activarla',
			),
		})
		expect(adminDb.state.cloudflareOriginCa).toBe(false)
	})

	it('encenderla y quitar el token a la vez es la misma regla', async () => {
		expect(await patch({ cloudflare_origin_ca: true, cloudflare_token: '' })).toMatchObject({
			status: 422,
			body: invalid(
				'cloudflare_origin_ca: necesita el token de Cloudflare: configúralo antes de activarla',
			),
		})
		expect(adminDb.state.cloudflareToken).not.toBeNull()
	})

	it('quitar el token con la opción encendida pide mandarla en false en el mismo cambio', async () => {
		adminDb.state.cloudflareOriginCa = true

		expect(await patch({ cloudflare_token: '' })).toEqual({
			status: 422,
			body: invalid(
				'cloudflare_origin_ca: sin token no se pueden usar los certificados de Cloudflare: mándala en false en el mismo cambio para quitar el token',
			),
		})
		expect(adminDb.state.cloudflareToken).not.toBeNull()

		expect(await patch({ cloudflare_token: '', cloudflare_origin_ca: false })).toMatchObject({
			status: 200,
			body: { cloudflare_token_set: false, cloudflare_origin_ca: false },
		})
		expect(adminDb.state.audit.at(-1)?.details).toEqual({
			cloudflare_token: 'removed',
			cloudflare_origin_ca: false,
		})
	})

	it('encendida sin token (a mano), "" no quita nada y la regla no aplica', async () => {
		adminDb.state.cloudflareToken = null
		adminDb.state.cloudflareOriginCa = true

		expect((await patch({ cloudflare_token: '' })).status).toBe(200)
	})

	it('un token sin el permiso (sin-ssl) es 422 en cloudflare_origin_ca al encenderla', async () => {
		adminDb.state.cloudflareToken = token('sin-ssl')

		expect(await patch({ cloudflare_origin_ca: true })).toEqual({
			status: 422,
			body: invalid(
				`cloudflare_origin_ca: ${PERMISSION}: agrégalo al token en Cloudflare, o deja la opción apagada`,
			),
		})
		expect(adminDb.state.cloudflareOriginCa).toBe(false)
	})

	it('y en cloudflare_token si cambia el token con la opción encendida', async () => {
		adminDb.state.cloudflareOriginCa = true
		const before = adminDb.state.cloudflareToken

		const response = await patch({ cloudflare_token: token('sin-ssl') })

		expect(response).toEqual({
			status: 422,
			body: invalid(
				`cloudflare_token: ${PERMISSION}: agrégalo al token en Cloudflare, o apaga antes los certificados de Cloudflare`,
			),
		})
		// El mensaje nunca trae el token, y no se guarda nada.
		expect(JSON.stringify(response.body)).not.toContain(token('sin-ssl'))
		expect(adminDb.state.cloudflareToken).toBe(before)
	})

	it('con la opción apagada, un token sin el permiso se guarda: no se comprueba', async () => {
		expect((await patch({ cloudflare_token: token('sin-ssl') })).status).toBe(200)
	})

	it('si Cloudflare no responde al comprobar el permiso (ssl-lento) es 502, y no guarda nada', async () => {
		adminDb.state.cloudflareToken = token('ssl-lento')

		expect(await patch({ cloudflare_origin_ca: true })).toMatchObject({
			status: 502,
			body: { error: { code: 'cloudflare_unavailable' } },
		})
		expect(adminDb.state.cloudflareOriginCa).toBe(false)
	})

	it('apagarla nunca llama a Cloudflare', async () => {
		adminDb.state.cloudflareToken = token('ssl-lento')
		adminDb.state.cloudflareOriginCa = true

		expect((await patch({ cloudflare_origin_ca: false })).status).toBe(200)
	})
})
