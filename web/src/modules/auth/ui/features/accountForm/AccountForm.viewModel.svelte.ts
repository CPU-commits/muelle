import { createForm } from '@tanstack/svelte-form'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import {
	isValidUsername,
	PASSWORD_MAX_BYTES,
	PASSWORD_MIN_LENGTH,
	passwordProblem,
} from '../../../domain'

export interface AccountFormValues {
	username: string
	password: string
	passwordRepeat: string
}

export const EMPTY_ACCOUNT_FORM: AccountFormValues = {
	username: '',
	password: '',
	passwordRepeat: '',
}

/** Los errores que devolvió el backend (422 `invalid`, 409 nombre tomado), por campo. */
export interface AccountServerErrors {
	username: string | null
	password: string | null
}

export const NO_SERVER_ERRORS: AccountServerErrors = { username: null, password: null }

// Validación de UX con mensajes traducidos. La regla de verdad vive en el dominio (el
// caso de uso vuelve a validar) y en el backend.
export function validateNewUsername(value: string): string | undefined {
	if (value.trim().length === 0) return m.common_required()
	if (!isValidUsername(value)) return m.auth_error_username_format()
	return undefined
}

export function validateNewPassword(value: string): string | undefined {
	switch (passwordProblem(value)) {
		case 'empty':
			return m.common_required()
		case 'short':
			return m.auth_error_password_short({ min: PASSWORD_MIN_LENGTH })
		case 'long':
			return m.auth_error_password_long({ max: PASSWORD_MAX_BYTES })
		default:
			return undefined
	}
}

export function validatePasswordRepeat(value: string, password: string): string | undefined {
	if (value.length === 0) return m.common_required()
	return value === password ? undefined : m.auth_error_password_mismatch()
}

export interface AccountFormParams {
	onSubmit: (values: AccountFormValues) => Promise<void> | void
}

function buildForm(params: AccountFormParams) {
	return createForm(() => ({
		defaultValues: EMPTY_ACCOUNT_FORM,
		onSubmit: async ({ value }) => {
			await params.onSubmit(value)
		},
	}))
}

export class AccountFormViewModel {
	readonly form: ReturnType<typeof buildForm>

	constructor(params: AccountFormParams) {
		this.form = buildForm(params)
	}
}
