import { m } from '@/shared/ui/i18n/paraglide/messages'
import { describeError } from '@/shared/ui'
import { ValidationError } from '@/shared/domain'
import {
	CannotDeleteSelf,
	InvalidTokenExpiry,
	InvalidTokenName,
	MissingTokenProject,
	TOKEN_MAX_DAYS,
	TOKEN_NAME_MAX_LENGTH,
	TokenNotFound,
	TokenScopeNotFound,
	UserNotFound,
} from '../../../domain'

// La pantalla elige el mensaje por la CLASE del error de dominio, nunca por un status.
export function identityErrorMessage(error: unknown): string {
	if (error instanceof CannotDeleteSelf) return m.identity_error_cannot_delete_self()
	if (error instanceof UserNotFound) return m.identity_error_user_not_found()
	if (error instanceof TokenNotFound) return m.identity_tokens_already_revoked()
	if (error instanceof TokenScopeNotFound) return m.identity_error_scope_not_found()
	// Lo común (sin red, un 5xx, una dirección que no es la del panel, demasiados
	// intentos…).
	return describeError(error)
}

/** Los campos del formulario de un token. */
export type TokenFormField = 'name' | 'project' | 'stage' | 'expiresOn'

const TOKEN_FIELDS: readonly string[] = ['name', 'project', 'stage', 'expiresOn']

function isTokenField(field: string | undefined): field is TokenFormField {
	return field !== undefined && TOKEN_FIELDS.includes(field)
}

/**
 * Si el error es de un campo del formulario de tokens, ese campo y su mensaje: los del
 * dominio, traducidos; los 422 del backend, con su motivo tal cual. Si no, null.
 */
export function tokenFieldError(error: unknown): { field: TokenFormField; message: string } | null {
	if (error instanceof InvalidTokenName) {
		const message =
			error.problem === 'empty'
				? m.common_required()
				: error.problem === 'long'
					? m.identity_error_token_name_long({ max: TOKEN_NAME_MAX_LENGTH })
					: m.identity_error_token_name_invisible()
		return { field: 'name', message }
	}
	if (error instanceof MissingTokenProject) {
		return { field: 'project', message: m.identity_error_token_project() }
	}
	if (error instanceof InvalidTokenExpiry) {
		const message =
			error.problem === 'past'
				? m.identity_error_token_expiry_past()
				: error.problem === 'too_far'
					? m.identity_error_token_expiry_far({ days: TOKEN_MAX_DAYS })
					: m.identity_error_token_expiry_invalid()
		return { field: 'expiresOn', message }
	}
	if (error instanceof TokenScopeNotFound) {
		return { field: 'project', message: m.identity_error_scope_not_found() }
	}
	if (error instanceof ValidationError && isTokenField(error.field)) {
		return { field: error.field, message: error.message }
	}
	return null
}
