import { m } from '@/shared/ui/i18n/paraglide/messages'
import { ValidationError } from '@/shared/domain'
import { InvalidPassword, InvalidUsername, UsernameTaken } from '../../../domain'
import { passwordMessage } from '../../components/authMessages'
import type { AccountServerErrors } from './AccountForm.viewModel.svelte.ts'

// Si el error es de un campo del formulario (nombre tomado, un 422 del backend o la
// validación del dominio), su mensaje por campo; si no, null y la pantalla lo muestra
// arriba del formulario.
export function accountServerErrors(error: unknown): AccountServerErrors | null {
	if (error instanceof UsernameTaken) {
		return { username: m.auth_error_username_taken(), password: null }
	}
	if (error instanceof InvalidPassword) {
		return { username: null, password: passwordMessage(error) }
	}
	if (error instanceof InvalidUsername) {
		return { username: m.auth_error_username_format(), password: null }
	}
	if (error instanceof ValidationError && error.field === 'username') {
		return { username: m.auth_error_username_format(), password: null }
	}
	if (error instanceof ValidationError && error.field === 'password') {
		return { username: null, password: error.message }
	}
	return null
}
