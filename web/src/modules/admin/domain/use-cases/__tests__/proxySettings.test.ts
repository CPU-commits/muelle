import { describe, expect, it } from 'vitest'
import type { CloudflareStatus } from '../..'
import {
	InvalidAcmeEmail,
	InvalidCloudflareToken,
	InvalidIpAddress,
	InvalidPort,
	makeRemoveCloudflareToken,
	makeReplaceCloudflareToken,
	makeSetCloudflareOriginCa,
	makeUpdateAcmeEmail,
	makeUpdateProxyPorts,
	makeUpdatePublicIp,
	OriginCaNeedsToken,
	RequiredField,
} from '../..'
import {
	fakeProxySettings,
	fakeSettings,
	InMemorySettingsRepository,
} from '../../repositories/__fakes__/inMemoryAdminRepository'

// Las tarjetas de "Proxy y DNS" (Fase 3): cada una valida lo suyo y manda solo sus campos.
describe('ajustes de proxy y DNS', () => {
	it('IP pública: cada una válida o vacía (vacía la borra)', async () => {
		const settings = new InMemorySettingsRepository()
		const update = makeUpdatePublicIp({ settings })

		await update({ ipv4: '203.0.113.30', ipv6: '' })
		await update({ ipv4: '', ipv6: '' })
		await expect(update({ ipv4: '203.0.113.30', ipv6: 'nope' })).rejects.toBeInstanceOf(
			InvalidIpAddress,
		)

		expect(settings.patches).toEqual([
			{ publicIpv4: '203.0.113.30', publicIpv6: '' },
			{ publicIpv4: '', publicIpv6: '' },
		])
	})

	it('email ACME: un email válido, en minúsculas', async () => {
		const settings = new InMemorySettingsRepository()
		const update = makeUpdateAcmeEmail({ settings })

		await update(' Ops@TuDominio.cl ')
		await update('  ')
		const error = await update('ops@').catch((caught: unknown) => caught)

		// Vacío borra el email (contrato §4.1).
		expect(settings.patches).toEqual([{ acmeEmail: 'ops@tudominio.cl' }, { acmeEmail: '' }])
		expect(error).toBeInstanceOf(InvalidAcmeEmail)
		expect((error as InvalidAcmeEmail).field).toBe('acmeEmail')
	})

	it('email ACME: sin { ni }, que Caddy lee como placeholder (contrato §3.4)', async () => {
		const settings = new InMemorySettingsRepository()
		const update = makeUpdateAcmeEmail({ settings })

		const error = await update('ops{x}@tudominio.cl').catch((caught: unknown) => caught)

		// No sale: el backend también lo rechaza (422, §4.1), pero acá con el texto del panel.
		expect(error).toMatchObject({ name: 'InvalidAcmeEmail', problem: 'placeholder' })
		expect(settings.patches).toEqual([])
	})

	it('puertos: los dos, distintos', async () => {
		const settings = new InMemorySettingsRepository()
		const update = makeUpdateProxyPorts({ settings })

		await expect(update({ http: '8080', https: '8443' })).resolves.toMatchObject({
			proxy: { httpPort: 8080, httpsPort: 8443 },
		})
		await expect(update({ http: '80', https: '80' })).rejects.toBeInstanceOf(InvalidPort)
		expect(settings.patches).toHaveLength(1)
	})

	it('token de Cloudflare: con forma de token, sin espacios en los bordes', async () => {
		const settings = new InMemorySettingsRepository({
			settings: fakeSettings({
				proxy: fakeProxySettings({
					cloudflare: { tokenSet: false, zones: null, originCa: false },
				}),
			}),
		})
		const replace = makeReplaceCloudflareToken({ settings })
		const token = 'cf_' + 'x'.repeat(37)

		await expect(replace('   ')).rejects.toBeInstanceOf(RequiredField)
		await expect(replace('cf-token')).rejects.toBeInstanceOf(InvalidCloudflareToken)
		await expect(replace(` ${token} `)).resolves.toMatchObject({
			proxy: { cloudflare: { tokenSet: true } },
		})
		expect(settings.patches).toEqual([{ cloudflareToken: token }])
	})

	it('quitar el token manda "" y deja sin token ni zonas', async () => {
		const settings = new InMemorySettingsRepository()
		const current = settings.settings.proxy.cloudflare

		await expect(makeRemoveCloudflareToken({ settings })(current)).resolves.toMatchObject({
			proxy: { cloudflare: { tokenSet: false, zones: null, originCa: false } },
		})
		expect(settings.patches).toEqual([{ cloudflareToken: '' }])
	})

	it('quitar el token con los certificados de Cloudflare encendidos los apaga en el mismo cambio', async () => {
		const settings = withCloudflare({ tokenSet: true, zones: 4, originCa: true })
		const current = settings.settings.proxy.cloudflare

		await expect(makeRemoveCloudflareToken({ settings })(current)).resolves.toMatchObject({
			proxy: { cloudflare: { tokenSet: false, zones: null, originCa: false } },
		})
		// Sin el false, el backend lo rechaza (contrato §4.1).
		expect(settings.patches).toEqual([{ cloudflareToken: '', cloudflareOriginCa: false }])
	})
})

function withCloudflare(cloudflare: CloudflareStatus): InMemorySettingsRepository {
	return new InMemorySettingsRepository({
		settings: fakeSettings({ proxy: fakeProxySettings({ cloudflare }) }),
	})
}

// Los certificados de Cloudflare (Origin CA, contrato §3.7 y §4.1).
describe('certificados de Cloudflare', () => {
	it('encenderlos sin token es OriginCaNeedsToken en el campo, sin llamar al backend', async () => {
		const settings = withCloudflare({ tokenSet: false, zones: null, originCa: false })
		const set = makeSetCloudflareOriginCa({ settings })

		const error = await set(true, settings.settings.proxy.cloudflare).catch(
			(caught: unknown) => caught,
		)

		expect(error).toBeInstanceOf(OriginCaNeedsToken)
		expect((error as OriginCaNeedsToken).field).toBe('cloudflareOriginCa')
		expect(settings.patches).toEqual([])
	})

	it('con token, encenderlos y apagarlos manda solo cloudflareOriginCa', async () => {
		const settings = withCloudflare({ tokenSet: true, zones: 4, originCa: false })
		const set = makeSetCloudflareOriginCa({ settings })

		await expect(set(true, settings.settings.proxy.cloudflare)).resolves.toMatchObject({
			proxy: { cloudflare: { tokenSet: true, zones: 4, originCa: true } },
		})
		await expect(set(false, settings.settings.proxy.cloudflare)).resolves.toMatchObject({
			proxy: { cloudflare: { originCa: false } },
		})
		expect(settings.patches).toEqual([
			{ cloudflareOriginCa: true },
			{ cloudflareOriginCa: false },
		])
	})

	it('apagarlos sin token se puede (quedaron encendidos sin token, a mano)', async () => {
		const settings = withCloudflare({ tokenSet: false, zones: null, originCa: true })

		await expect(
			makeSetCloudflareOriginCa({ settings })(false, settings.settings.proxy.cloudflare),
		).resolves.toMatchObject({ proxy: { cloudflare: { originCa: false } } })
		expect(settings.patches).toEqual([{ cloudflareOriginCa: false }])
	})

	it('el fake rechaza como el backend: quitar el token sin apagarlos es un 422 del campo', async () => {
		const settings = withCloudflare({ tokenSet: true, zones: 4, originCa: true })

		const error = await settings
			.patch({ cloudflareToken: '' })
			.catch((caught: unknown) => caught)

		expect(error).toMatchObject({ name: 'ValidationError', field: 'cloudflareOriginCa' })
		expect(settings.settings.proxy.cloudflare.tokenSet).toBe(true)
	})
})
