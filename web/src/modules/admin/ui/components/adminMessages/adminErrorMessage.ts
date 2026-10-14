import { m } from '@/shared/ui/i18n/paraglide/messages'
import { describeError } from '@/shared/ui'
import { ValidationError } from '@/shared/domain'
import {
	InvalidAcmeEmail,
	InvalidCidr,
	InvalidIpAddress,
	InvalidPort,
	InvalidRegistryHost,
	RegistryExists,
	RegistryNotFound,
	RequiredField,
	TotpRequiredFirst,
} from '../../../domain'

// La pantalla elige el mensaje por la CLASE del error de dominio, nunca por un status.
export function adminErrorMessage(error: unknown): string {
	if (error instanceof TotpRequiredFirst) return m.admin_security_totp_first()
	if (error instanceof RegistryExists) return m.admin_error_registry_exists()
	if (error instanceof RegistryNotFound) return m.admin_error_registry_not_found()
	// Lo común (sin red, fase pendiente, MUELLE_PANEL_ORIGIN mal, demasiados intentos…).
	return describeError(error)
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
		return { field: 'acmeEmail', message: m.admin_error_email() }
	}
	if (error instanceof InvalidRegistryHost) {
		return { field: 'host', message: m.admin_error_registry_host() }
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
