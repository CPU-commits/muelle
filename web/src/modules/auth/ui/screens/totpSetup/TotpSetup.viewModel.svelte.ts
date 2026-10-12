import { goto } from '$app/navigation'
import { resolve } from '$app/paths'
import { createMutation, useQueryClient } from '@tanstack/svelte-query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toast } from '@/shared/ui/stores'
import { UnavailableError } from '@/shared/domain'
import { beginTotpSetupMutation, enableTotpMutation, logoutMutation } from '../../../data'
import {
	groupSecret,
	type Me,
	NoPendingTotp,
	normalizeTotpCode,
	SessionExpired,
	TOTP_CODE_LENGTH,
	type TotpSetup,
} from '../../../domain'
import { authErrorMessage } from '../../components/authMessages'

// /2fa: generar el secreto, mostrarlo como QR y confirmarlo con un código. Es obligatoria
// si el panel exige 2FA (`needsTotpSetup`); si no, se puede saltar o volver.
export class TotpSetupViewModel {
	code = $state('')
	formError = $state<string | null>(null)
	setup = $state.raw<TotpSetup | null>(null)

	readonly #me: () => Me
	readonly #client = useQueryClient()
	readonly #begin = createMutation(() => beginTotpSetupMutation())
	readonly #enable = createMutation(() => enableTotpMutation(this.#client))
	readonly #logout = createMutation(() => logoutMutation(this.#client))

	constructor(me: () => Me) {
		this.#me = me
		// Sin 2FA, el secreto se genera al entrar. Con 2FA activa, solo si lo pide: generar
		// uno nuevo no apaga el actual, pero confirmarlo sí lo reemplaza.
		if (!me().totpEnabled) void this.start()
	}

	get isRequired(): boolean {
		return this.#me().needsTotpSetup
	}

	get alreadyEnabled(): boolean {
		return this.#me().totpEnabled
	}

	get isStarting(): boolean {
		return this.#begin.isPending
	}

	get isEnabling(): boolean {
		return this.#enable.isPending
	}

	get secretGroups(): string {
		return this.setup ? groupSecret(this.setup.secret) : ''
	}

	get isCodeComplete(): boolean {
		return normalizeTotpCode(this.code).length === TOTP_CODE_LENGTH
	}

	setCode = (value: string): void => {
		this.code = value
		this.formError = null
	}

	start = async (): Promise<void> => {
		this.formError = null
		this.code = ''
		try {
			this.setup = await this.#begin.mutateAsync()
		} catch (error) {
			this.formError =
				error instanceof UnavailableError ? m.common_unavailable() : m.auth_2fa_load_error()
		}
	}

	submit = async (): Promise<void> => {
		if (this.#enable.isPending || !this.isCodeComplete) return
		this.formError = null
		try {
			await this.#enable.mutateAsync(this.code)
			toast.success(m.auth_2fa_enabled_ok())
			await goto(resolve('/'))
		} catch (error) {
			if (error instanceof SessionExpired) {
				await goto(resolve('/login'))
				return
			}
			this.code = ''
			if (error instanceof NoPendingTotp) this.setup = null
			this.formError = authErrorMessage(error)
		}
	}

	logout = async (): Promise<void> => {
		this.formError = null
		try {
			await this.#logout.mutateAsync()
		} catch {
			this.formError = m.auth_error_logout()
			return
		}
		await goto(resolve('/login'))
	}
}
