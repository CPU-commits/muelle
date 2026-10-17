import { m } from '@/shared/ui/i18n/paraglide/messages'
import { describeError } from '@/shared/ui'
import { ValidationError } from '@/shared/domain'
import {
	CLOUDFLARE_TOKEN_MAX_LENGTH,
	CLOUDFLARE_TOKEN_MIN_LENGTH,
	CloudflareUnavailable,
	InvalidAcmeEmail,
	InvalidCidr,
	InvalidCloudflareToken,
	InvalidIpAddress,
	InvalidPort,
	InvalidRegistryCredential,
	InvalidRegistryHost,
	PortsDockerUnavailable,
	REGISTRY_HOST_MAX_LENGTH,
	REGISTRY_PASSWORD_MAX_BYTES,
	REGISTRY_USERNAME_MAX_LENGTH,
	type RegistryHostProblem,
	RegistryExists,
	RegistryNotFound,
	RequiredField,
	TotpRequiredFirst,
	WouldLockOut,
} from '../../../domain'

// La pantalla elige el mensaje por la CLASE del error de dominio, nunca por un status.
export function adminErrorMessage(error: unknown): string {
	if (error instanceof TotpRequiredFirst) return m.admin_security_totp_first()
	if (error instanceof WouldLockOut) return m.admin_security_lockout_title()
	// Antes que lo común: un `UnavailableError` cualquiera diría "muelle respondió con un error".
	if (error instanceof CloudflareUnavailable) return m.admin_cf_unavailable_title()
	if (error instanceof PortsDockerUnavailable) return m.admin_ports_docker_unavailable()
	if (error instanceof RegistryExists) return m.admin_error_registry_exists()
	if (error instanceof RegistryNotFound) return m.admin_error_registry_not_found()
	// Lo común (sin red, fase pendiente, una dirección que no es la del panel, demasiados
	// intentos…).
	return describeError(error)
}

const REGISTRY_HOST_MESSAGES: Record<RegistryHostProblem, () => string> = {
	empty: () => m.common_required(),
	too_long: () => m.admin_error_registry_host_long({ max: REGISTRY_HOST_MAX_LENGTH }),
	scheme: () => m.admin_error_registry_host_scheme(),
	path: () => m.admin_error_registry_host_path(),
	format: () => m.admin_error_registry_host_format(),
}

function cloudflareTokenMessage(error: InvalidCloudflareToken): string {
	switch (error.problem) {
		case 'placeholder':
			return m.admin_error_placeholder()
		case 'spaces':
			return m.admin_error_cf_token_spaces()
		case 'length':
			return m.admin_error_cf_token_length({
				min: CLOUDFLARE_TOKEN_MIN_LENGTH,
				max: CLOUDFLARE_TOKEN_MAX_LENGTH,
			})
	}
}

function registryCredentialMessage(error: InvalidRegistryCredential): string {
	if (error.field === 'username') {
		if (error.problem === 'too_long') {
			return m.admin_error_registry_username_long({ max: REGISTRY_USERNAME_MAX_LENGTH })
		}
		return error.problem === 'colon'
			? m.admin_error_registry_username_colon()
			: m.admin_error_registry_username_control()
	}
	return error.problem === 'too_long'
		? m.admin_error_registry_password_long({ max: REGISTRY_PASSWORD_MAX_BYTES })
		: m.admin_error_registry_password_newline()
}

export interface FieldError {
	readonly field: string
	readonly message: string
}

/**
 * Si el error es de un campo de un formulario de Ajustes, el campo y su mensaje: los del
 * dominio, traducidos; los 422 del backend, con su motivo tal cual. Si no, null.
 */
export function adminFieldError(error: unknown): FieldError | null {
	if (error instanceof InvalidIpAddress) {
		const message = error.version === 4 ? m.admin_error_ipv4() : m.admin_error_ipv6()
		return { field: error.field ?? 'ipv4', message }
	}
	if (error instanceof InvalidPort) {
		const message = error.problem === 'same' ? m.admin_error_port_same() : m.admin_error_port()
		return { field: error.field ?? 'httpPort', message }
	}
	if (error instanceof InvalidCidr) {
		return {
			field: 'panelAllowedCidrs',
			message: m.admin_error_cidr({ line: error.line, value: error.value }),
		}
	}
	if (error instanceof InvalidAcmeEmail) {
		const message =
			error.problem === 'placeholder' ? m.admin_error_placeholder() : m.admin_error_email()
		return { field: 'acmeEmail', message }
	}
	if (error instanceof InvalidRegistryHost) {
		return { field: 'host', message: REGISTRY_HOST_MESSAGES[error.problem]() }
	}
	if (error instanceof InvalidRegistryCredential) {
		return { field: error.field, message: registryCredentialMessage(error) }
	}
	if (error instanceof InvalidCloudflareToken) {
		return { field: 'cloudflareToken', message: cloudflareTokenMessage(error) }
	}
	if (error instanceof RegistryExists) {
		return { field: 'host', message: m.admin_error_registry_exists() }
	}
	if (error instanceof RequiredField) {
		return { field: error.field ?? '', message: m.common_required() }
	}
	if (error instanceof ValidationError && error.field) {
		return { field: error.field, message: error.message }
	}
	return null
}
