import { createMutation, useQueryClient } from '@tanstack/svelte-query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toast } from '@/shared/ui/stores'
import { ingressKeys } from '@/modules/ingress'
import {
	removeCloudflareTokenMutation,
	replaceCloudflareTokenMutation,
	updateAcmeEmailMutation,
	updateProxyPortsMutation,
	updatePublicIpMutation,
} from '../../../data'
import {
	CloudflareUnavailable,
	movedPanelOrigin,
	parseCloudflareToken,
	parseProxyPorts,
	type ProxySettings,
	type Settings,
} from '../../../domain'
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

/** Un aviso adentro del modal del token: título y, si hace falta, qué hacer. */
export interface CloudflareNotice {
	readonly title: string
	readonly body: string | null
}

function currentOrigin(): string {
	return typeof location === 'undefined' ? '' : location.origin
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
	/** Por qué no se guardó el token, si no es un problema del campo (Cloudflare caído). */
	cloudflareError = $state.raw<CloudflareNotice | null>(null)
	confirmingRemoveCloudflare = $state(false)
	/** Por qué no se pudo quitar el token; se muestra adentro de la confirmación. */
	removeCloudflareError = $state<string | null>(null)
	confirmingPorts = $state(false)
	/**
	 * A dónde se mudó el panel después de cambiar el puerto HTTPS: el origen actual deja de
	 * responder cuando se recrea muelle-caddy. null si sigue donde está.
	 */
	movedTo = $state<string | null>(null)
	/** Por qué no se pudieron cambiar los puertos; se muestra adentro de la confirmación. */
	portsError = $state<string | null>(null)
	errors = $state<FieldErrors>({})

	readonly #proxy: () => ProxySettings
	readonly #client = useQueryClient()
	readonly #ip = createMutation(() => updatePublicIpMutation(this.#client))
	readonly #acme = createMutation(() => updateAcmeEmailMutation(this.#client))
	readonly #ports = createMutation(() => updateProxyPortsMutation(this.#client))
	readonly #cloudflare = createMutation(() => replaceCloudflareTokenMutation(this.#client))
	readonly #removeCloudflare = createMutation(() => removeCloudflareTokenMutation(this.#client))

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

	/** "4 zonas visibles · cifrado", o solo "cifrado" si el backend no sabe cuántas ve. */
	get cloudflareMeta(): string {
		const zones = this.proxy.cloudflare.zones
		if (zones === null) return m.admin_cf_encrypted()
		return zones === 1 ? m.admin_cf_zones_one() : m.admin_cf_zones({ count: zones })
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

	get isRemovingCloudflare(): boolean {
		return this.#removeCloudflare.isPending
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
		this.portsError = null
		this.confirmingPorts = true
	}

	cancelPorts = (): void => {
		if (this.#ports.isPending) return
		this.confirmingPorts = false
		this.portsError = null
	}

	/**
	 * El `PATCH` vuelve cuando el backend aplicó el cambio (hasta 2 min, contrato §3.5);
	 * mientras, la confirmación queda abierta diciendo que está aplicando.
	 */
	savePorts = async (): Promise<void> => {
		if (this.#ports.isPending) return
		this.portsError = null
		const before = this.proxy
		try {
			const settings = await this.#ports.mutateAsync({
				http: this.httpPort,
				https: this.httpsPort,
			})
			if (settings.proxy) {
				this.#loadPorts(settings.proxy)
				this.movedTo = this.#movedTo(settings.proxy, before)
			}
			this.#refreshIngress()
			toast.success(m.admin_ports_saved())
			this.confirmingPorts = false
		} catch (error) {
			const fieldError = adminFieldError(error)
			if (fieldError && isProxyField(fieldError.field)) {
				// Un puerto que el backend rechaza va a su campo, en la tarjeta.
				this.errors = { ...this.errors, [fieldError.field]: fieldError.message }
				this.confirmingPorts = false
				return
			}
			this.portsError = adminErrorMessage(error)
		}
	}

	openCloudflare = (): void => {
		this.cloudflareToken = ''
		this.cloudflareError = null
		this.#clearError('cloudflareToken')
		this.cloudflareOpen = true
	}

	closeCloudflare = (): void => {
		if (this.#cloudflare.isPending) return
		this.cloudflareOpen = false
		this.cloudflareToken = ''
		this.cloudflareError = null
	}

	saveCloudflare = async (): Promise<void> => {
		if (this.#cloudflare.isPending) return
		this.cloudflareError = null
		let token: string
		try {
			token = parseCloudflareToken(this.cloudflareToken)
		} catch (error) {
			// Todavía no salió: queda en el campo para corregirlo, como en un formulario.
			this.#fail(error)
			return
		}
		// Ya sale: el token no se queda en ningún lado, ni en el campo ni en el cache de
		// mutaciones. Si el backend lo rechaza, se pega de nuevo.
		this.cloudflareToken = ''
		try {
			await this.#cloudflare.mutateAsync(token)
			this.#refreshIngress()
			toast.success(m.admin_cf_saved())
			this.cloudflareOpen = false
		} catch (error) {
			const fieldError = adminFieldError(error)
			if (fieldError?.field === 'cloudflareToken') {
				// La forma, o Cloudflare lo rechazó (o no ve zonas): es el token.
				this.errors = { ...this.errors, cloudflareToken: fieldError.message }
				return
			}
			// Cloudflare caído no es culpa del token: se pega otra vez y se reintenta.
			this.cloudflareError =
				error instanceof CloudflareUnavailable
					? { title: m.admin_cf_unavailable_title(), body: m.admin_cf_unavailable_body() }
					: { title: adminErrorMessage(error), body: null }
		} finally {
			this.#cloudflare.reset()
		}
	}

	askRemoveCloudflare = (): void => {
		this.removeCloudflareError = null
		this.confirmingRemoveCloudflare = true
	}

	cancelRemoveCloudflare = (): void => {
		if (this.#removeCloudflare.isPending) return
		this.confirmingRemoveCloudflare = false
		this.removeCloudflareError = null
	}

	removeCloudflare = async (): Promise<void> => {
		if (this.#removeCloudflare.isPending) return
		this.removeCloudflareError = null
		try {
			await this.#removeCloudflare.mutateAsync()
			this.#refreshIngress()
			toast.success(m.admin_cf_removed())
			this.confirmingRemoveCloudflare = false
		} catch (error) {
			this.removeCloudflareError = adminErrorMessage(error)
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
	): Promise<void> {
		try {
			const settings = await run()
			if (settings.proxy) apply(settings.proxy)
			this.#refreshIngress()
			toast.success(success)
		} catch (error) {
			this.#fail(error)
		}
	}

	/**
	 * Lo que cambió puede cambiar los dominios y el proxy (contrato §3.5): los `records` se
	 * calculan con la IP pública de ahora, los registros que maneja muelle siguen a la IP (y
	 * un dominio en error de Cloudflare puede volver a `pending`), y la configuración de
	 * Caddy se regenera o se recrea el contenedor. Dominios los vuelve a pedir.
	 */
	#refreshIngress(): void {
		void this.#client.invalidateQueries({ queryKey: ingressKeys.all })
	}

	#movedTo(proxy: ProxySettings, before: ProxySettings): string | null {
		const origin = movedPanelOrigin(before, proxy)
		return origin === currentOrigin() ? null : origin
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
