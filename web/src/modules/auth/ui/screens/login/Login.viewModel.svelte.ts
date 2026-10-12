import { goto } from '$app/navigation'
import { resolve } from '$app/paths'
import { createForm } from '@tanstack/svelte-form'
import { createMutation, useQueryClient } from '@tanstack/svelte-query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { completeTotpMutation, loginMutation, logoutMutation } from '../../../data'
import { normalizeTotpCode, SessionExpired, TOTP_CODE_LENGTH } from '../../../domain'
import { authErrorMessage } from '../../components/authMessages'

export type LoginStep = 'password' | 'code'

export interface LoginFormValues {
	username: string
	password: string
}

const EMPTY_LOGIN_FORM: LoginFormValues = { username: '', password: '' }

// Validación de UX: solo que no estén vacíos. La regla de verdad la aplica el backend, y
// un usuario con otro formato no existe: responde igual que una contraseña mala.
export function validateRequiredField(value: string): string | undefined {
	return value.trim().length === 0 ? m.common_required() : undefined
}

// El login en dos pasos de `Login.dc.html`: contraseña y, si la cuenta tiene 2FA, el
// código. `initialStep` es 'code' cuando se recarga la página con una sesión que ya
// espera el código (el guard de /login lo detecta).
export class LoginViewModel {
	step = $state<LoginStep>('password')
	code = $state('')
	formError = $state<string | null>(null)

	readonly #client = useQueryClient()
	readonly #login = createMutation(() => loginMutation(this.#client))
	readonly #totp = createMutation(() => completeTotpMutation(this.#client))
	readonly #logout = createMutation(() => logoutMutation(this.#client))

	readonly form = createForm(() => ({
		defaultValues: EMPTY_LOGIN_FORM,
		onSubmit: async ({ value }) => {
			await this.#submitPassword(value)
		},
	}))

	// Se lee una sola vez, al montar: después el paso lo maneja el viewModel.
	constructor(initialStep: () => LoginStep = () => 'password') {
		this.step = initialStep()
	}

	get isPending(): boolean {
		return this.#login.isPending || this.#totp.isPending
	}

	get isCodeComplete(): boolean {
		return normalizeTotpCode(this.code).length === TOTP_CODE_LENGTH
	}

	setCode = (value: string): void => {
		this.code = value
		this.formError = null
	}

	submitCode = async (): Promise<void> => {
		if (this.#totp.isPending || !this.isCodeComplete) return
		this.formError = null
		try {
			await this.#totp.mutateAsync(this.code)
			await this.#enter()
		} catch (error) {
			if (error instanceof SessionExpired) {
				// Cinco códigos malos borran la sesión pendiente: de vuelta a la contraseña.
				this.#toPassword(m.auth_error_session_expired())
				return
			}
			this.code = ''
			this.formError = authErrorMessage(error)
		}
	}

	// La sesión que esperaba el código no sirve más: se descarta (el backend puede
	// contestar 401 mfa_required, que cuenta como cerrada) y se vuelve a la contraseña.
	backToPassword = (): void => {
		this.#logout.mutate(undefined, { onError: () => {} })
		this.#toPassword(null)
	}

	#toPassword(message: string | null): void {
		this.step = 'password'
		this.code = ''
		this.formError = message
	}

	async #submitPassword(value: LoginFormValues): Promise<void> {
		this.formError = null
		try {
			const outcome = await this.#login.mutateAsync(value)
			if (outcome.mfaRequired) {
				this.step = 'code'
				this.code = ''
				return
			}
			await this.#enter()
		} catch (error) {
			this.formError = authErrorMessage(error)
		}
	}

	// El guard de `(app)` decide el resto: si el panel exige 2FA y la cuenta no la tiene,
	// manda a /2fa.
	async #enter(): Promise<void> {
		await goto(resolve('/'))
	}
}
