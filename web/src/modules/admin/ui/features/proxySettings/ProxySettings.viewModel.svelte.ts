import { resolve } from '$app/paths'
import { untrack } from 'svelte'
import { createMutation, createQuery, useQueryClient } from '@tanstack/svelte-query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { withQuery } from '@/shared/ui'
import { toast } from '@/shared/ui/stores'
import { ingressKeys, proxyFollowQuery, proxyNotice, type ProxyStatus } from '@/modules/ingress'
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
	panelOrigin,
	parseCloudflareToken,
	parseProxyPorts,
	type ProxyPorts,
	type ProxySettings,
	type Settings,
} from '../../../domain'
import { adminErrorMessage, adminFieldError } from '../../components/adminMessages'
import { clearPanelMove, loadPanelMove, PANEL_MOVED_PARAM, savePanelMove } from './panelMoveNotice'

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

/** A dónde se mudaría el panel con los puertos que se están confirmando. */
export interface PortsMove {
	readonly origin: string
	/** El puerto HTTPS nuevo: el que tiene que estar abierto en el firewall. */
	readonly port: number
}

/**
 * Cómo va muelle-caddy después de cambiar los puertos sin mudarse de origen (contrato §3.5):
 * se recrea con los puertos nuevos, quedó escuchando, o no pudo (un puerto ocupado: el
 * `error` de `GET /proxy`, y muelle reintenta con los mismos puertos).
 */
export type PortsRecreation =
	| { readonly kind: 'recreating' }
	| { readonly kind: 'done' }
	| { readonly kind: 'failed'; readonly error: string | null }

/** Un aviso adentro del modal del token: título y, si hace falta, qué hacer. */
export interface CloudflareNotice {
	readonly title: string
	readonly body: string | null
}

/**
 * Cuánto se espera, después de cambiar los puertos, una respuesta de `GET /proxy` que diga
 * cómo terminó la recreación (el `PATCH` ya volvió; recrear tarda unos segundos más, y si no
 * arranca, volver a los puertos anteriores, otros tantos). Pasado eso, el aviso se va.
 */
export const PORTS_WATCH_TIMEOUT_MS = 2 * 60_000

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
	 * A dónde se mudó el panel después de cambiar el puerto HTTPS (si se entró por el origen
	 * anterior, deja de responder cuando se recrea muelle-caddy). Queda guardado hasta que
	 * se cierra el aviso: sobrevive a recargar. null si no hay aviso.
	 */
	movedTo = $state<string | null>(loadPanelMove())
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
	/** Se prende al cambiar los puertos si el panel sigue en este origen. */
	#watchingProxy = $state(false)
	/** Pasó `PORTS_WATCH_TIMEOUT_MS` sin una respuesta que diga cómo terminó. */
	#watchExpired = $state(false)
	/** Cuándo se guardaron: un estado del proxy de antes (en el cache) no dice nada. */
	#watchingSince = 0
	// `GET /proxy` (ingress): lo vuelve a pedir mientras no diga cómo terminó, aunque falle.
	// `waiting` lo llama TanStack adentro del efecto que suscribe la query: leído con
	// `untrack`, o cada respuesta haría que se vuelva a suscribir, y eso cancela el pedido
	// en curso (y descarta su respuesta).
	readonly #proxyStatus = createQuery(() =>
		proxyFollowQuery({
			enabled: this.#watchingProxy,
			waiting: () => untrack(() => this.#awaitingProxy),
		}),
	)

	constructor(proxy: () => ProxySettings) {
		this.#proxy = proxy
		const initial = proxy()
		this.#loadIp(initial)
		this.#loadAcme(initial)
		this.#loadPorts(initial)
		$effect(() => {
			if (!this.#watchingProxy) return
			const timer = setTimeout(() => (this.#watchExpired = true), PORTS_WATCH_TIMEOUT_MS)
			return () => clearTimeout(timer)
		})
	}

	/** El último `GET /proxy` pedido después de guardar los puertos, si llegó alguno. */
	get #freshProxy(): ProxyStatus | undefined {
		const query = this.#proxyStatus
		return query.dataUpdatedAt >= this.#watchingSince ? query.data : undefined
	}

	/** Todavía no llegó un estado que diga cómo terminó la recreación (y no pasó el plazo). */
	get #awaitingProxy(): boolean {
		if (!this.#watchingProxy || this.#watchExpired) return false
		const fresh = this.#freshProxy
		return fresh === undefined || proxyNotice(fresh) === 'recreating'
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

	/**
	 * Si los puertos que se están confirmando mudan el panel (cambia el HTTPS y Caddy lo
	 * sirve): a dónde, y qué puerto tiene que estar abierto para volver a entrar. null si no
	 * se muda, o si los puertos todavía no son válidos.
	 */
	get portsMove(): PortsMove | null {
		let ports: ProxyPorts
		try {
			ports = parseProxyPorts(this.httpPort, this.httpsPort)
		} catch {
			return null
		}
		const next = { ...this.proxy, httpPort: ports.http, httpsPort: ports.https }
		const origin = movedPanelOrigin(this.proxy, next)
		return origin === null ? null : { origin, port: ports.https }
	}

	/**
	 * muelle-caddy después de cambiar los puertos, si el panel sigue en este origen (contrato
	 * §3.5: solo el HTTP, o se entró por otra dirección). null si no se cambiaron, si se mudó
	 * (este origen deja de responder) o sin el proxy de muelle (`disabled`): los puertos ya se
	 * guardaron, esto es solo el seguimiento. Mientras `GET /proxy` no diga cómo terminó
	 * (también si falla: el panel puede pasar por el mismo muelle-caddy que se recrea), dice
	 * `recreating`, hasta `PORTS_WATCH_TIMEOUT_MS`.
	 */
	get recreation(): PortsRecreation | null {
		if (!this.#watchingProxy) return null
		const fresh = this.#freshProxy
		if (this.#awaitingProxy) return { kind: 'recreating' }
		if (fresh === undefined) return null
		const notice = proxyNotice(fresh)
		if (notice === null) return { kind: 'done' }
		if (notice === 'disabled' || notice === 'recreating') return null
		return { kind: 'failed', error: fresh.error }
	}

	/** Si el aviso apunta a otro origen: "Ir al panel" lleva allá. */
	get canGoToMovedPanel(): boolean {
		return this.movedTo !== null && this.movedTo !== currentOrigin()
	}

	/** Ajustes del origen nuevo, con el aviso (allá el localStorage es otro). */
	get movedPanelHref(): string {
		return withQuery(`${this.movedTo ?? ''}${resolve('/(app)/ajustes')}`, {
			[PANEL_MOVED_PARAM]: 1,
		})
	}

	dismissMove = (): void => {
		this.movedTo = null
		clearPanelMove()
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
		this.#watchingProxy = false
		this.#watchExpired = false
		try {
			const settings = await this.#ports.mutateAsync({
				http: this.httpPort,
				https: this.httpsPort,
			})
			if (settings.proxy) {
				this.#loadPorts(settings.proxy)
				const moved = this.#movedTo(settings.proxy, before)
				if (moved !== null) {
					this.movedTo = moved
					savePanelMove(moved)
				}
				// Si se entró por el origen que sirve Caddy y cambió el HTTPS, este origen deja
				// de responder: no hay nada que seguir desde acá. Si no, se sigue la recreación.
				this.#watchingSince = Date.now()
				this.#watchingProxy = moved === null || currentOrigin() !== panelOrigin(before)
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
