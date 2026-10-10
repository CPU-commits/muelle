import { goto } from '$app/navigation'
import { resolve } from '$app/paths'
import { createMutation, useQueryClient } from '@tanstack/svelte-query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { acceptInviteMutation } from '../../../data'
import { InvalidInvite } from '../../../domain'
import { authErrorMessage } from '../../components/authMessages'
import {
	type AccountFormValues,
	type AccountServerErrors,
	accountServerErrors,
	NO_SERVER_ERRORS,
} from '../../features/accountForm'

// Aceptar una invitación (`/invite/<token>`): elegir usuario y contraseña. Una
// invitación es de un solo uso y vence en 24 h; si no sirve, el formulario se apaga.
export class InviteViewModel {
	formError = $state<string | null>(null)
	invalid = $state(false)
	serverErrors = $state.raw<AccountServerErrors>(NO_SERVER_ERRORS)

	readonly #token: () => string
	readonly #client = useQueryClient()
	readonly #accept = createMutation(() => acceptInviteMutation(this.#client))

	constructor(token: () => string) {
		this.#token = token
	}

	get isPending(): boolean {
		return this.#accept.isPending
	}

	submit = async (values: AccountFormValues): Promise<void> => {
		this.formError = null
		this.serverErrors = NO_SERVER_ERRORS
		try {
			await this.#accept.mutateAsync({
				token: this.#token(),
				username: values.username,
				password: values.password,
			})
			await goto(resolve('/2fa'))
		} catch (error) {
			const fieldErrors = accountServerErrors(error)
			if (fieldErrors) {
				this.serverErrors = fieldErrors
				return
			}
			if (error instanceof InvalidInvite) {
				this.invalid = true
				this.formError = m.auth_invite_invalid()
				return
			}
			this.formError = authErrorMessage(error)
		}
	}
}
