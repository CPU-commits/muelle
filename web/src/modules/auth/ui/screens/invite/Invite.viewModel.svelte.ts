import { goto, replaceState } from '$app/navigation'
import { resolve } from '$app/paths'
import { createMutation, useQueryClient } from '@tanstack/svelte-query'
import { onMount } from 'svelte'
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

	readonly #token: string | null
	readonly #client = useQueryClient()
	readonly #accept = createMutation(() => acceptInviteMutation(this.#client))

	// El token se lee UNA vez y se saca de la URL (queda /invite): no queda en el historial,
	// en una captura ni en el Referer. Si se recarga, /invite dice que falta el link.
	constructor(token: () => string | null) {
		this.#token = token()
		if (this.hasToken) onMount(() => replaceState(resolve('/invite'), {}))
	}

	get hasToken(): boolean {
		return (this.#token ?? '').trim().length > 0
	}

	get canSubmit(): boolean {
		return this.hasToken && !this.invalid
	}

	get isPending(): boolean {
		return this.#accept.isPending
	}

	submit = async (values: AccountFormValues): Promise<void> => {
		this.formError = null
		this.serverErrors = NO_SERVER_ERRORS
		try {
			await this.#accept.mutateAsync({
				token: this.#token ?? '',
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
