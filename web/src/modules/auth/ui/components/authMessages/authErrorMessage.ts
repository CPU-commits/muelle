import { m } from '@/shared/ui/i18n/paraglide/messages'
import { describeError } from '@/shared/ui/utils/errorMessage'
import {
	AccountLocked,
	AlreadySetUp,
	InvalidCode,
	InvalidCredentials,
	InvalidInvite,
	InvalidPassword,
	InvalidUsername,
	MalformedCode,
	NoPendingTotp,
	PASSWORD_MAX_BYTES,
	PASSWORD_MIN_LENGTH,
	SessionExpired,
	TooManyAttempts,
	UsernameTaken,
} from '../../../domain'

/** El texto de un problema con la contraseña de una cuenta nueva. */
export function passwordMessage(error: InvalidPassword): string {
	switch (error.problem) {
		case 'empty':
			return m.common_required()
		case 'short':
			return m.auth_error_password_short({ min: PASSWORD_MIN_LENGTH })
		case 'long':
			return m.auth_error_password_long({ max: PASSWORD_MAX_BYTES })
	}
}

// La pantalla elige el mensaje por la CLASE del error de dominio, nunca por un status. Lo
// que no es de auth (backend caído, bad_origin, dos cookies) lo explica `describeError`.
export function authErrorMessage(error: unknown): string {
	if (error instanceof InvalidCredentials) return m.auth_error_invalid_credentials()
	if (error instanceof AccountLocked) return m.auth_error_locked()
	if (error instanceof TooManyAttempts) return m.auth_error_rate_limited()
	if (error instanceof InvalidCode) return m.auth_error_invalid_code()
	if (error instanceof MalformedCode) return m.auth_error_code_length()
	if (error instanceof SessionExpired) return m.auth_error_session_expired()
	if (error instanceof NoPendingTotp) return m.auth_error_no_pending_totp()
	if (error instanceof InvalidInvite) return m.auth_invite_invalid()
	if (error instanceof AlreadySetUp) return m.auth_setup_already()
	if (error instanceof UsernameTaken) return m.auth_error_username_taken()
	if (error instanceof InvalidUsername) return m.auth_error_username_format()
	if (error instanceof InvalidPassword) return passwordMessage(error)
	return describeError(error)
}
