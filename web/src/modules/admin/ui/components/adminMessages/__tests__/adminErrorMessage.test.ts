import { describe, expect, it } from 'vitest'
import { UnavailableError, ValidationError } from '@/shared/domain'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import {
	CloudflareUnavailable,
	InvalidAcmeEmail,
	InvalidCloudflareToken,
	InvalidRegistryCredential,
	InvalidRegistryHost,
	PortsDockerUnavailable,
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

	it('Docker caído al probar los puertos dice que no se guardó nada, no "muelle respondió con un error"', () => {
		expect(adminErrorMessage(new PortsDockerUnavailable())).toBe(
			m.admin_ports_docker_unavailable(),
		)
		expect(m.admin_ports_docker_unavailable()).toMatch(/Docker/)
		expect(m.admin_ports_docker_unavailable({}, { locale: 'en' })).toMatch(/Docker/)
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
		expect(messageOf('too_long')).toEqual({
			field: 'host',
			message: m.admin_error_registry_host_long({ max: 255 }),
		})
		expect(m.admin_error_registry_host_long({ max: 255 })).toBe('Hasta 255 caracteres.')
	})

	it('{ y } en el email o el token: Caddy los lee como placeholder', () => {
		expect(adminFieldError(new InvalidAcmeEmail('placeholder'))).toEqual({
			field: 'acmeEmail',
			message: m.admin_error_placeholder(),
		})
		expect(adminFieldError(new InvalidAcmeEmail())).toEqual({
			field: 'acmeEmail',
			message: m.admin_error_email(),
		})
		expect(adminFieldError(new InvalidCloudflareToken('placeholder'))).toEqual({
			field: 'cloudflareToken',
			message: m.admin_error_placeholder(),
		})
	})

	it('el 422 de { y } del backend se muestra con su motivo, en su campo (contrato §4.1)', () => {
		const reason = 'no puede tener { ni }: Caddy los toma como placeholders'
		expect(adminFieldError(new ValidationError(reason, 'acmeEmail'))).toEqual({
			field: 'acmeEmail',
			message: reason,
		})
		expect(adminFieldError(new ValidationError(reason, 'cloudflareToken'))).toEqual({
			field: 'cloudflareToken',
			message: reason,
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
		expect(adminFieldError(new InvalidRegistryCredential('username', 'colon'))).toEqual({
			field: 'username',
			message: m.admin_error_registry_username_colon(),
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
