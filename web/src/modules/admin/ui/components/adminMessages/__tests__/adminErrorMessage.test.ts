import { describe, expect, it } from 'vitest'
import { UnavailableError, ValidationError } from '@/shared/domain'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import {
	CloudflareUnavailable,
	InvalidCloudflareToken,
	InvalidRegistryCredential,
	InvalidRegistryHost,
	WouldLockOut,
} from '../../../../domain'
import { adminErrorMessage, adminFieldError } from '../adminErrorMessage'

describe('adminErrorMessage', () => {
	it('Cloudflare caído no es un problema del token: dice que se reintente', () => {
		expect(adminErrorMessage(new CloudflareUnavailable())).toBe(m.admin_cf_unavailable_title())
		expect(adminFieldError(new CloudflareUnavailable())).toBeNull()
		// Otro 5xx sigue con el texto de siempre.
		expect(adminErrorMessage(new UnavailableError('x', { reason: 'server' }))).not.toBe(
			m.admin_cf_unavailable_title(),
		)
	})

	it('would_lock_out tiene su texto', () => {
		expect(adminErrorMessage(new WouldLockOut())).toBe(m.admin_security_lockout_title())
	})
})

describe('adminFieldError', () => {
	it('explica qué sobra en el host de un registry', () => {
		const messageOf = (problem: InvalidRegistryHost['problem']) =>
			adminFieldError(new InvalidRegistryHost(problem))

		expect(messageOf('empty')).toEqual({ field: 'host', message: m.common_required() })
		expect(messageOf('scheme')).toEqual({
			field: 'host',
			message: m.admin_error_registry_host_scheme(),
		})
		expect(messageOf('path')).toEqual({
			field: 'host',
			message: m.admin_error_registry_host_path(),
		})
		expect(messageOf('format')).toEqual({
			field: 'host',
			message: m.admin_error_registry_host_format(),
		})
	})

	it('los límites de usuario y contraseña de un registry', () => {
		expect(adminFieldError(new InvalidRegistryCredential('username', 'too_long'))).toEqual({
			field: 'username',
			message: m.admin_error_registry_username_long({ max: 255 }),
		})
		expect(adminFieldError(new InvalidRegistryCredential('username', 'control'))).toEqual({
			field: 'username',
			message: m.admin_error_registry_username_control(),
		})
		expect(adminFieldError(new InvalidRegistryCredential('password', 'too_long'))).toEqual({
			field: 'password',
			message: m.admin_error_registry_password_long({ max: 4096 }),
		})
		expect(adminFieldError(new InvalidRegistryCredential('password', 'newline'))).toEqual({
			field: 'password',
			message: m.admin_error_registry_password_newline(),
		})
	})

	it('la forma del token de Cloudflare', () => {
		expect(adminFieldError(new InvalidCloudflareToken('length'))).toEqual({
			field: 'cloudflareToken',
			message: m.admin_error_cf_token_length({ min: 20, max: 512 }),
		})
		expect(adminFieldError(new InvalidCloudflareToken('spaces'))).toEqual({
			field: 'cloudflareToken',
			message: m.admin_error_cf_token_spaces(),
		})
	})

	it('un 422 del backend (Cloudflare rechazó el token) va al campo con su motivo', () => {
		expect(
			adminFieldError(new ValidationError('Cloudflare rechazó el token', 'cloudflareToken')),
		).toEqual({ field: 'cloudflareToken', message: 'Cloudflare rechazó el token' })
	})
})
