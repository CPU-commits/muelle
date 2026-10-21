import { describe, expect, it } from 'vitest'
import type { RegistryHost } from '../../../domain'
import { auditEventDtoSchema, registryDtoSchema, settingsDtoSchema } from '../../dtos/adminDto'
import {
	auditEventFromDto,
	newRegistryPayload,
	registryCredentialsPayload,
	registryFromDto,
	settingsFromDto,
	settingsPatchPayload,
} from '../adminMapper'

const PHASE_3 = {
	require_2fa: true,
	cloudflare_token_set: true,
	cloudflare_zones: 4,
	cloudflare_origin_ca: false,
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
	it('GET /settings trae siempre todas las claves (contrato §4): sin una, no es del contrato', () => {
		expect(settingsDtoSchema.safeParse(PHASE_3).success).toBe(true)
		expect(settingsDtoSchema.safeParse({ require_2fa: false }).success).toBe(false)
		for (const key of Object.keys(PHASE_3) as (keyof typeof PHASE_3)[]) {
			const missing: Partial<typeof PHASE_3> = { ...PHASE_3 }
			delete missing[key]
			expect(settingsDtoSchema.safeParse(missing).success, key).toBe(false)
		}
		// cloudflare_zones sí puede ser null (no se pudo consultar).
		expect(settingsDtoSchema.safeParse({ ...PHASE_3, cloudflare_zones: null }).success).toBe(
			true,
		)
		// cloudflare_origin_ca nunca es null, y es un bool (contrato §4).
		for (const value of [null, 'true', 1]) {
			const dto = { ...PHASE_3, cloudflare_origin_ca: value }
			expect(settingsDtoSchema.safeParse(dto).success, String(value)).toBe(false)
		}
	})

	it('arma cada sección', () => {
		expect(settingsFromDto(settingsDtoSchema.parse(PHASE_3))).toEqual({
			require2fa: true,
			proxy: {
				cloudflare: { tokenSet: true, zones: 4, originCa: false },
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

	it('los certificados de Cloudflare (Origin CA) van con el token, en cloudflare.originCa', () => {
		const dto = settingsDtoSchema.parse({ ...PHASE_3, cloudflare_origin_ca: true })
		expect(settingsFromDto(dto).proxy.cloudflare).toEqual({
			tokenSet: true,
			zones: 4,
			originCa: true,
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
				confirmLockout: true,
			}),
		).toEqual({
			public_ip: '203.0.113.24',
			public_ipv6: '',
			acme_email: 'a@b.cl',
			proxy_http_port: 8080,
			proxy_https_port: 8443,
			panel_allowed_cidrs: ['10.0.0.0/8'],
			cloudflare_token: 'cf',
			confirm_lockout: true,
		})
		expect(settingsPatchPayload({})).toEqual({})
		// Quitar el token es mandar "".
		expect(settingsPatchPayload({ cloudflareToken: '' })).toEqual({ cloudflare_token: '' })
	})

	it('un evento de auditoría pierde los details en el borde', () => {
		const dto = auditEventDtoSchema.parse({
			id: 9,
			actor_type: 'token',
			actor_id: 3,
			actor_name: 'github-actions',
			action: 'env.patch',
			target: 'app:auditfy/staging/api',
			details: { set: ['DATABASE_URL'] },
			at: '2026-10-10T19:36:53.770Z',
		})

		expect(dto).not.toHaveProperty('details')
		expect(auditEventFromDto(dto)).toEqual({
			id: 9,
			actor: { kind: 'token', id: 3, name: 'github-actions' },
			action: 'env.patch',
			target: 'app:auditfy/staging/api',
			at: new Date('2026-10-10T19:36:53.770Z'),
		})
	})

	it('traduce cada tipo de actor, con el nombre copiado al escribir el evento (contrato §4.2)', () => {
		const base = { id: 1, action: 'a', target: 't', at: '2026-10-10T12:00:00-03:00' }
		const actor = (actor_type: string, actor_id: number | null, actor_name: string) =>
			auditEventFromDto({ ...base, actor_type, actor_id, actor_name }).actor

		expect(actor('user', 1, 'felipe')).toEqual({ kind: 'user', id: 1, name: 'felipe' })
		expect(actor('token', 5, 'github-actions')).toEqual({
			kind: 'token',
			id: 5,
			name: 'github-actions',
		})
		expect(actor('system', null, 'system')).toEqual({ kind: 'system' })
		expect(actor('cron', null, 'cron')).toEqual({ kind: 'other', type: 'cron' })
		// Un evento de antes de la migración 0004 cuyo autor ya no estaba: "".
		expect(actor('token', 9, '')).toEqual({ kind: 'token', id: 9, name: '' })
	})

	it('actor_name viene siempre y es un string (contrato §4.2): sin él no es un evento', () => {
		const event = {
			id: 1,
			actor_type: 'user',
			actor_id: 1,
			action: 'auth.login',
			target: 'user:felipe',
			at: '2026-10-10T12:00:00Z',
		}
		expect(auditEventDtoSchema.safeParse(event).success).toBe(false)
		expect(auditEventDtoSchema.safeParse({ ...event, actor_name: null }).success).toBe(false)
		expect(auditEventDtoSchema.safeParse({ ...event, actor_name: '' }).success).toBe(true)
	})

	it('client_ip es siempre un string (contrato §4): ya no admite null', () => {
		expect(settingsDtoSchema.safeParse({ ...PHASE_3, client_ip: null }).success).toBe(false)
		expect(settingsFromDto(settingsDtoSchema.parse(PHASE_3)).panelAccess?.clientIp).toBe(
			'198.51.100.17',
		)
	})

	it('un registry trae sus fechas', () => {
		expect(
			registryFromDto(
				registryDtoSchema.parse({
					host: 'ghcr.io',
					username: 'bot',
					created_at: '2026-10-01T12:00:00.000Z',
					updated_at: '2026-10-10T19:36:53.77Z',
				}),
			),
		).toEqual({
			host: 'ghcr.io',
			username: 'bot',
			createdAt: new Date('2026-10-01T12:00:00.000Z'),
			updatedAt: new Date('2026-10-10T19:36:53.770Z'),
		})
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
