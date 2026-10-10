import { goto } from '$app/navigation'
import { resolve } from '$app/paths'
import { createMutation, useQueryClient } from '@tanstack/svelte-query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { setupMutation } from '../../../data'
import { AlreadySetUp, InvalidInvite, MissingInviteToken } from '../../../domain'
import { authErrorMessage } from '../../components/authMessages'
import {
	type AccountFormValues,
	type AccountServerErrors,
	accountServerErrors,
	NO_SERVER_ERRORS,
} from '../../features/accountForm'

// El primer usuario de muelle, con el link que imprime `muelle serve`
// (`<panel>/setup?token=…`). Después va a /2fa, que recomienda (o exige) activar 2FA.
export class SetupViewModel {
	formError = $state<string | null>(null)
	/** El setup ya se hizo: el formulario no sirve, hay que entrar o pedir invitación. */
	alreadySetUp = $state(false)
	serverErrors = $state.raw<AccountServerErrors>(NO_SERVER_ERRORS)

	readonly #token: () => string | null
	readonly #client = useQueryClient()
	readonly #setup = createMutation(() => setupMutation(this.#client))

	constructor(token: () => string | null) {
		this.#token = token
	}

	get hasToken(): boolean {
		return (this.#token() ?? '').trim().length > 0
	}

	get isPending(): boolean {
		return this.#setup.isPending
	}

	get canSubmit(): boolean {
		return this.hasToken && !this.alreadySetUp
	}

	submit = async (values: AccountFormValues): Promise<void> => {
		this.formError = null
		this.serverErrors = NO_SERVER_ERRORS
		try {
			await this.#setup.mutateAsync({
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
			if (error instanceof AlreadySetUp) {
				this.alreadySetUp = true
				this.formError = m.auth_setup_already()
				return
			}
			if (error instanceof MissingInviteToken) {
				this.formError = m.auth_setup_missing_token()
				return
			}
			this.formError =
				error instanceof InvalidInvite
					? m.auth_setup_invalid_token()
					: authErrorMessage(error)
		}
	}
}
