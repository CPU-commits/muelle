import { createMutation, useQueryClient } from '@tanstack/svelte-query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toast } from '@/shared/ui/stores'
import { updateSecurityMutation } from '../../../data'
import {
	cidrListAllows,
	cidrListText,
	type PanelAccess,
	parseCidrList,
	sameCidrList,
	type Settings,
	TotpRequiredFirst,
	type UpdateSecurityInput,
	WouldLockOut,
} from '../../../domain'
import { adminErrorMessage, adminFieldError } from '../../components/adminMessages'

// La tarjeta "Seguridad": exigir 2FA (Fase 1) y las IPs permitidas para el panel (Fase 3).
// Un solo "Guardar" manda lo que cambió.
export class SecuritySettingsViewModel {
	require2fa = $state(false)
	cidrs = $state('')
	cidrsError = $state<string | null>(null)
	/** El backend respondió `totp_required_first`: hay que configurar el 2FA propio antes. */
	totpRequiredFirst = $state(false)
	/**
	 * El backend respondió `would_lock_out`: la lista nueva deja fuera la IP con que ve esta
	 * sesión. Se pide confirmación y se reenvía con `confirmLockout`.
	 */
	confirmingLockout = $state(false)
	/** Por qué no se pudo guardar desde la confirmación; se muestra adentro. */
	lockoutError = $state<string | null>(null)

	readonly #settings: () => Settings
	readonly #client = useQueryClient()
	readonly #update = createMutation(() => updateSecurityMutation(this.#client))

	constructor(settings: () => Settings) {
		this.#settings = settings
		this.#load(settings())
	}

	/** `null`: el backend todavía no tiene la Fase 3. */
	get panelAccess(): PanelAccess | null {
		return this.#settings().panelAccess
	}

	get is2faDirty(): boolean {
		return this.require2fa !== this.#settings().require2fa
	}

	get areCidrsDirty(): boolean {
		const access = this.panelAccess
		return access !== null && !sameCidrList(this.cidrs, access.allowedCidrs)
	}

	get isDirty(): boolean {
		return this.is2faDirty || this.areCidrsDirty
	}

	get isSaving(): boolean {
		return this.#update.isPending
	}

	/** La IP con que muelle ve esta sesión; `''` si no la informa (Fase 1). */
	get clientIp(): string {
		return this.panelAccess?.clientIp ?? ''
	}

	/**
	 * Si con lo escrito la IP de esta sesión todavía entra; null si no se conoce. Es solo un
	 * aviso: quien decide si la lista deja fuera a la sesión es el backend (`would_lock_out`).
	 */
	get clientIpAllowed(): boolean | null {
		const ip = this.clientIp
		return ip === '' ? null : cidrListAllows(this.cidrs, ip)
	}

	get clientIpHint(): string | null {
		const ip = this.clientIp
		if (ip === '' || this.cidrs.trim() === '') return null
		return this.clientIpAllowed
			? m.admin_security_client_ip_in({ ip })
			: m.admin_security_client_ip_out({ ip })
	}

	get lockoutBody(): string {
		const ip = this.clientIp
		return ip === ''
			? m.admin_security_lockout_body_no_ip()
			: m.admin_security_lockout_body({ ip })
	}

	setRequire2fa = (value: boolean): void => {
		this.require2fa = value
		this.totpRequiredFirst = false
	}

	setCidrs = (value: string): void => {
		this.cidrs = value
		this.cidrsError = null
	}

	save = async (): Promise<void> => {
		if (!this.isDirty || this.#update.isPending) return
		if (this.areCidrsDirty) {
			try {
				parseCidrList(this.cidrs)
			} catch (error) {
				this.#fail(error)
				return
			}
		}
		const error = await this.#submit(false)
		if (error instanceof WouldLockOut) {
			// No se guardó nada (tampoco el 2FA): confirmar reenvía todo junto.
			this.lockoutError = null
			this.confirmingLockout = true
			return
		}
		if (error !== null) this.#fail(error)
	}

	confirmLockout = async (): Promise<void> => {
		if (this.#update.isPending) return
		this.lockoutError = null
		const error = await this.#submit(true)
		if (error === null) {
			this.confirmingLockout = false
			return
		}
		// Lo que tiene su lugar en la tarjeta (el 2FA propio, una línea mal) va ahí; el resto,
		// adentro de la confirmación, que queda abierta.
		const inCard =
			error instanceof TotpRequiredFirst ||
			adminFieldError(error)?.field === 'panelAllowedCidrs'
		if (inCard) {
			this.confirmingLockout = false
			this.#fail(error)
			return
		}
		this.lockoutError = adminErrorMessage(error)
	}

	cancelLockout = (): void => {
		if (this.#update.isPending) return
		this.confirmingLockout = false
		this.lockoutError = null
	}

	/** Guarda lo que cambió; devuelve el error si falló (o null). */
	async #submit(confirmLockout: boolean): Promise<unknown> {
		const input: UpdateSecurityInput = {
			...(this.is2faDirty ? { require2fa: this.require2fa } : {}),
			...(this.areCidrsDirty ? { allowedCidrs: this.cidrs } : {}),
			...(confirmLockout ? { confirmLockout: true } : {}),
		}
		try {
			this.#load(await this.#update.mutateAsync(input))
			toast.success(m.admin_security_saved())
			return null
		} catch (error) {
			return error
		}
	}

	#fail(error: unknown): void {
		if (error instanceof TotpRequiredFirst) {
			// El switch vuelve a lo guardado: exigirlo no se aplicó.
			this.require2fa = this.#settings().require2fa
			this.totpRequiredFirst = true
			return
		}
		const fieldError = adminFieldError(error)
		if (fieldError?.field === 'panelAllowedCidrs') {
			this.cidrsError = fieldError.message
			return
		}
		toast.error(m.admin_save_error(), adminErrorMessage(error))
	}

	#load(settings: Settings): void {
		this.require2fa = settings.require2fa
		if (settings.panelAccess) this.cidrs = cidrListText(settings.panelAccess.allowedCidrs)
	}
}
