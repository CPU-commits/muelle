import { createMutation, useQueryClient } from '@tanstack/svelte-query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toast } from '@/shared/ui/stores'
import {
	replaceCloudflareTokenMutation,
	updateAcmeEmailMutation,
	updateProxyPortsMutation,
	updatePublicIpMutation,
} from '../../../data'
import { parseProxyPorts, type ProxySettings, type Settings } from '../../../domain'
import { adminErrorMessage, adminFieldError } from '../../components/adminMessages'

/** Los campos de las cuatro tarjetas, con el nombre que usan los errores del dominio. */
export type ProxyField =
	| 'ipv4'
	| 'ipv6'
	| 'acmeEmail'
	| 'httpPort'
	| 'httpsPort'
	| 'cloudflareToken'

type FieldErrors = Partial<Record<ProxyField, string>>

const PROXY_FIELDS: readonly string[] = [
	'ipv4',
	'ipv6',
	'acmeEmail',
	'httpPort',
	'httpsPort',
	'cloudflareToken',
]

function isProxyField(field: string): field is ProxyField {
	return PROXY_FIELDS.includes(field)
}

// "Proxy y DNS" de Ajustes (Fase 3): Cloudflare, IP pública, certificados y puertos. Cada
// tarjeta guarda lo suyo por separado; su botón se activa cuando algo cambió.
export class ProxySettingsViewModel {
	ipv4 = $state('')
	ipv6 = $state('')
	acmeEmail = $state('')
	httpPort = $state('')
	httpsPort = $state('')
	cloudflareToken = $state('')
	cloudflareOpen = $state(false)
	confirmingPorts = $state(false)
	errors = $state<FieldErrors>({})

	readonly #proxy: () => ProxySettings
	readonly #client = useQueryClient()
	readonly #ip = createMutation(() => updatePublicIpMutation(this.#client))
	readonly #acme = createMutation(() => updateAcmeEmailMutation(this.#client))
	readonly #ports = createMutation(() => updateProxyPortsMutation(this.#client))
	readonly #cloudflare = createMutation(() => replaceCloudflareTokenMutation(this.#client))

	constructor(proxy: () => ProxySettings) {
		this.#proxy = proxy
		const initial = proxy()
		this.#loadIp(initial)
		this.#loadAcme(initial)
		this.#loadPorts(initial)
	}

	get proxy(): ProxySettings {
		return this.#proxy()
	}

	get isIpDirty(): boolean {
		return (
			this.ipv4.trim() !== this.proxy.publicIpv4 ||
			this.ipv6.trim().toLowerCase() !== this.proxy.publicIpv6
		)
	}

	get isAcmeDirty(): boolean {
		return this.acmeEmail.trim().toLowerCase() !== this.proxy.acmeEmail
	}

	get arePortsDirty(): boolean {
		return (
			this.httpPort.trim() !== String(this.proxy.httpPort) ||
			this.httpsPort.trim() !== String(this.proxy.httpsPort)
		)
	}

	get isSavingIp(): boolean {
		return this.#ip.isPending
	}

	get isSavingAcme(): boolean {
		return this.#acme.isPending
	}

	get isSavingPorts(): boolean {
		return this.#ports.isPending
	}

	get isSavingCloudflare(): boolean {
		return this.#cloudflare.isPending
	}

	setField = (field: ProxyField, value: string): void => {
		this[field] = value
		this.#clearError(field)
	}

	saveIp = async (): Promise<void> => {
		if (!this.isIpDirty || this.#ip.isPending) return
		await this.#save(
			() => this.#ip.mutateAsync({ ipv4: this.ipv4, ipv6: this.ipv6 }),
			m.admin_ip_saved(),
			(proxy) => this.#loadIp(proxy),
		)
	}

	saveAcme = async (): Promise<void> => {
		if (!this.isAcmeDirty || this.#acme.isPending) return
		await this.#save(
			() => this.#acme.mutateAsync(this.acmeEmail),
			m.admin_cert_saved(),
			(proxy) => this.#loadAcme(proxy),
		)
	}

	/** Recrear Caddy corta los sitios: primero se valida y se pide confirmación. */
	askSavePorts = (): void => {
		if (!this.arePortsDirty) return
		try {
			parseProxyPorts(this.httpPort, this.httpsPort)
		} catch (error) {
			this.#fail(error)
			return
		}
		this.confirmingPorts = true
	}

	cancelPorts = (): void => {
		if (!this.#ports.isPending) this.confirmingPorts = false
	}

	savePorts = async (): Promise<void> => {
		if (this.#ports.isPending) return
		await this.#save(
			() => this.#ports.mutateAsync({ http: this.httpPort, https: this.httpsPort }),
			m.admin_ports_saved(),
			(proxy) => this.#loadPorts(proxy),
		)
		this.confirmingPorts = false
	}

	openCloudflare = (): void => {
		this.cloudflareToken = ''
		this.#clearError('cloudflareToken')
		this.cloudflareOpen = true
	}

	closeCloudflare = (): void => {
		if (this.#cloudflare.isPending) return
		this.cloudflareOpen = false
		this.cloudflareToken = ''
	}

	saveCloudflare = async (): Promise<void> => {
		if (this.#cloudflare.isPending) return
		const saved = await this.#save(
			() => this.#cloudflare.mutateAsync(this.cloudflareToken),
			m.admin_cf_saved(),
			() => {},
		)
		if (saved) {
			this.cloudflareOpen = false
			this.cloudflareToken = ''
		}
	}

	/**
	 * Guarda una tarjeta y, si salió bien, pone lo guardado en SU borrador: lo que se está
	 * editando en las otras tarjetas no se pierde.
	 */
	async #save(
		run: () => Promise<Settings>,
		success: string,
		apply: (proxy: ProxySettings) => void,
	): Promise<boolean> {
		try {
			const settings = await run()
			if (settings.proxy) apply(settings.proxy)
			toast.success(success)
			return true
		} catch (error) {
			this.#fail(error)
			return false
		}
	}

	#clearError(field: ProxyField): void {
		if (this.errors[field] === undefined) return
		const next = { ...this.errors }
		delete next[field]
		this.errors = next
	}

	#fail(error: unknown): void {
		const fieldError = adminFieldError(error)
		if (fieldError && isProxyField(fieldError.field)) {
			this.errors = { ...this.errors, [fieldError.field]: fieldError.message }
			return
		}
		toast.error(m.admin_save_error(), adminErrorMessage(error))
	}

	#loadIp(proxy: ProxySettings): void {
		this.ipv4 = proxy.publicIpv4
		this.ipv6 = proxy.publicIpv6
	}

	#loadAcme(proxy: ProxySettings): void {
		this.acmeEmail = proxy.acmeEmail
	}

	#loadPorts(proxy: ProxySettings): void {
		this.httpPort = String(proxy.httpPort)
		this.httpsPort = String(proxy.httpsPort)
	}
}
