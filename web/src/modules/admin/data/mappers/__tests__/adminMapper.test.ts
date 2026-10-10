import { describe, expect, it } from 'vitest'
import type { RegistryHost } from '../../../domain'
import { auditEventDtoSchema, settingsDtoSchema } from '../../dtos/adminDto'
import {
	auditEventFromDto,
	newRegistryPayload,
	registryCredentialsPayload,
	settingsFromDto,
	settingsPatchPayload,
} from '../adminMapper'

const PHASE_3 = {
	require_2fa: true,
	cloudflare_token_set: true,
	cloudflare_zones: 4,
	public_ip: '203.0.113.24',
	public_ipv6: '2001:db8::24',
	acme_email: 'ops@tudominio.cl',
	panel_domain: 'panel.tudominio.cl',
	proxy_http_port: 80,
	proxy_https_port: 443,
	panel_allowed_cidrs: ['198.51.100.0/24'],
	client_ip: '198.51.100.17',
}

describe('adminMapper', () => {
	it('la Fase 1 solo trae require_2fa: lo demás queda pendiente (null)', () => {
		expect(settingsFromDto(settingsDtoSchema.parse({ require_2fa: false }))).toEqual({
			require2fa: false,
			proxy: null,
			panelAccess: null,
		})
	})

	it('con la Fase 3, arma cada sección', () => {
		expect(settingsFromDto(settingsDtoSchema.parse(PHASE_3))).toEqual({
			require2fa: true,
			proxy: {
				cloudflare: { tokenSet: true, zones: 4 },
				publicIpv4: '203.0.113.24',
				publicIpv6: '2001:db8::24',
				acmeEmail: 'ops@tudominio.cl',
				panelDomain: 'panel.tudominio.cl',
				httpPort: 80,
				httpsPort: 443,
			},
			panelAccess: { allowedCidrs: ['198.51.100.0/24'], clientIp: '198.51.100.17' },
		})
	})

	it('manda solo las claves presentes, con los nombres del backend', () => {
		expect(settingsPatchPayload({ require2fa: true })).toEqual({ require_2fa: true })
		expect(
			settingsPatchPayload({
				publicIpv4: '203.0.113.24',
				publicIpv6: '',
				acmeEmail: 'a@b.cl',
				httpPort: 8080,
				httpsPort: 8443,
				panelAllowedCidrs: ['10.0.0.0/8'],
				cloudflareToken: 'cf',
			}),
		).toEqual({
			public_ip: '203.0.113.24',
			public_ipv6: '',
			acme_email: 'a@b.cl',
			proxy_http_port: 8080,
			proxy_https_port: 8443,
			panel_allowed_cidrs: ['10.0.0.0/8'],
			cloudflare_token: 'cf',
		})
		expect(settingsPatchPayload({})).toEqual({})
	})

	it('un evento de auditoría pierde los details en el borde', () => {
		const dto = auditEventDtoSchema.parse({
			id: 9,
			actor_type: 'token',
			actor_id: 3,
			action: 'env.patch',
			target: 'app:auditfy/staging/api',
			details: { set: ['DATABASE_URL'] },
			at: '2026-10-10T19:36:53.770Z',
		})

		expect(dto).not.toHaveProperty('details')
		expect(auditEventFromDto(dto)).toEqual({
			id: 9,
			actor: { kind: 'token', id: 3 },
			action: 'env.patch',
			target: 'app:auditfy/staging/api',
			at: new Date('2026-10-10T19:36:53.770Z'),
		})
	})

	it('traduce cada tipo de actor', () => {
		const base = { id: 1, action: 'a', target: 't', at: '2026-10-10T12:00:00-03:00' }
		const actor = (actor_type: string, actor_id: number | null) =>
			auditEventFromDto({ ...base, actor_type, actor_id }).actor

		expect(actor('user', 1)).toEqual({ kind: 'user', id: 1 })
		expect(actor('system', null)).toEqual({ kind: 'system' })
		expect(actor('cron', null)).toEqual({ kind: 'other', type: 'cron' })
	})

	it('arma los cuerpos de registries', () => {
		expect(
			newRegistryPayload({ host: 'ghcr.io' as RegistryHost, username: 'u', password: 'p' }),
		).toEqual({ host: 'ghcr.io', username: 'u', password: 'p' })
		expect(registryCredentialsPayload({ username: 'u', password: 'p' })).toEqual({
			username: 'u',
			password: 'p',
		})
	})
})
