import { describe, expect, it } from 'vitest'
import { ValidationError } from '@/shared/domain'
import {
	InvalidIpAddress,
	InvalidPort,
	makeReplaceCloudflareToken,
	makeUpdateAcmeEmail,
	makeUpdateProxyPorts,
	makeUpdatePublicIp,
	RequiredField,
} from '../..'
import {
	fakeProxySettings,
	fakeSettings,
	InMemorySettingsRepository,
} from '../../repositories/__fakes__/inMemoryAdminRepository'

// Las tarjetas de "Proxy y DNS" (Fase 3): cada una valida lo suyo y manda solo sus campos.
describe('ajustes de proxy y DNS', () => {
	it('IP pública: IPv4 obligatoria, IPv6 opcional', async () => {
		const settings = new InMemorySettingsRepository()
		const update = makeUpdatePublicIp({ settings })

		await update({ ipv4: '203.0.113.30', ipv6: '' })
		await expect(update({ ipv4: '', ipv6: '' })).rejects.toBeInstanceOf(InvalidIpAddress)
		await expect(update({ ipv4: '203.0.113.30', ipv6: 'nope' })).rejects.toBeInstanceOf(
			InvalidIpAddress,
		)

		expect(settings.patches).toEqual([{ publicIpv4: '203.0.113.30', publicIpv6: '' }])
	})

	it('email ACME: un email válido, en minúsculas', async () => {
		const settings = new InMemorySettingsRepository()
		const update = makeUpdateAcmeEmail({ settings })

		await update(' Ops@TuDominio.cl ')
		const error = await update('ops@').catch((caught: unknown) => caught)

		expect(settings.patches).toEqual([{ acmeEmail: 'ops@tudominio.cl' }])
		expect(error).toBeInstanceOf(ValidationError)
		expect((error as ValidationError).field).toBe('acmeEmail')
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

	it('token de Cloudflare: no vacío, sin espacios en los bordes', async () => {
		const settings = new InMemorySettingsRepository({
			settings: fakeSettings({
				proxy: fakeProxySettings({ cloudflare: { tokenSet: false, zones: null } }),
			}),
		})
		const replace = makeReplaceCloudflareToken({ settings })

		await expect(replace('   ')).rejects.toBeInstanceOf(RequiredField)
		await expect(replace(' cf-token ')).resolves.toMatchObject({
			proxy: { cloudflare: { tokenSet: true } },
		})
		expect(settings.patches).toEqual([{ cloudflareToken: 'cf-token' }])
	})
})
