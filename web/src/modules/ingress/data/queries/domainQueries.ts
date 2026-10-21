import { type QueryClient, type QueryKey, queryOptions } from '@tanstack/svelte-query'
import { ingressUseCases } from '../../container'
import {
	type AddDomainInput,
	type AppDomain,
	type AppRef,
	type ChangeTargetInput,
	type DomainRef,
	hasProvisioning,
	hasWaitingDns,
	proxySettling,
	replaceDomain,
	type SetProxiedInput,
	sortDomains,
} from '../../domain'

export const ingressKeys = {
	all: ['ingress'] as QueryKey,
	domains: (app: AppRef): QueryKey => ['ingress', app.project, app.stage, app.app, 'domains'],
	proxy: ['ingress', 'proxy'] as QueryKey,
}

/** Mientras el proxy se está arreglando solo (desincronizado, recreándose), cada cuánto mirar. */
export const PROXY_SETTLING_REFRESH_MS = 10_000

/** El estado de muelle-caddy, para el aviso "proxy desincronizado". */
export function proxyQuery() {
	return queryOptions({
		queryKey: ingressKeys.proxy,
		queryFn: ({ signal }) => ingressUseCases.getProxyStatus(signal),
		staleTime: 10_000,
		refetchInterval: (query) =>
			proxySettling(query.state.data) ? PROXY_SETTLING_REFRESH_MS : false,
		refetchIntervalInBackground: false,
	})
}

export interface ProxyFollowOptions {
	/** Si todavía falta una respuesta que diga cómo terminó (se lee en cada vuelta). */
	readonly waiting: () => boolean
	/** Si se dejó de esperar porque pasó el plazo, sin esa respuesta (se lee en cada vuelta). */
	readonly expired: () => boolean
}

/**
 * `GET /proxy` para quien espera cómo termina una recreación de muelle-caddy (Ajustes,
 * después de cambiar los puertos): lo mismo que `proxyQuery`, y además pregunta cada 10 s
 * mientras `waiting()` diga que sí, aunque la última respuesta haya fallado o sea de antes
 * del cambio. El panel puede pasar por el mismo muelle-caddy que se recrea.
 *
 * Pasado el plazo (`expired()`), un último pedido que falló corta: TanStack guarda la
 * respuesta de antes, y si decía `recreating` se seguiría preguntando cada 10 s para siempre.
 */
export function proxyFollowQuery({ waiting, expired }: ProxyFollowOptions) {
	return queryOptions({
		...proxyQuery(),
		refetchInterval: (query) => {
			if (waiting()) return PROXY_SETTLING_REFRESH_MS
			if (expired() && query.state.status === 'error') return false
			return proxySettling(query.state.data) ? PROXY_SETTLING_REFRESH_MS : false
		},
	})
}

/** Mientras muelle esté resolviendo un dominio (registro, certificado), la lista se refresca sola. */
export const PENDING_REFRESH_MS = 5_000

/**
 * Un `pending` que espera un registro manual: muelle lo verifica cada minuto (contrato
 * §3.1) y lo pasa a `active` cuando el certificado sale, o a `error` a los 10 min. La
 * lista lo sigue, más despacio; Verificar lo mira ahora.
 */
export const WAITING_DNS_REFRESH_MS = 30_000

export function domainsQuery(app: AppRef) {
	return queryOptions({
		queryKey: ingressKeys.domains(app),
		queryFn: ({ signal }) => ingressUseCases.listDomains(app, signal),
		staleTime: 10_000,
		// Con la pestaña oculta no se refresca (refetchIntervalInBackground: false).
		refetchInterval: (query) => {
			const domains = query.state.data ?? []
			if (hasProvisioning(domains)) return PENDING_REFRESH_MS
			return hasWaitingDns(domains) ? WAITING_DNS_REFRESH_MS : false
		},
		refetchIntervalInBackground: false,
	})
}

/**
 * Cada cambio de un dominio reenvía la configuración de Caddy: el estado del proxy puede
 * haber cambiado (una ruta que Caddy no acepta lo deja desincronizado).
 */
function refreshAfterChange(client: QueryClient, app: AppRef): Promise<void> {
	return Promise.all([
		client.invalidateQueries({ queryKey: ingressKeys.domains(app) }),
		client.invalidateQueries({ queryKey: ingressKeys.proxy }),
	]).then(() => undefined)
}

/** Pone en el cache el dominio que devolvió el backend y pide la lista de nuevo. */
function storeDomain(client: QueryClient, app: AppRef, domain: AppDomain): Promise<void> {
	client.setQueryData<readonly AppDomain[]>(ingressKeys.domains(app), (current) => {
		if (!current) return current
		const exists = current.some((item) => item.hostname === domain.hostname)
		return exists ? replaceDomain(current, domain) : sortDomains([...current, domain])
	})
	return refreshAfterChange(client, app)
}

export function addDomainMutation(client: QueryClient) {
	return {
		mutationFn: (input: AddDomainInput) => ingressUseCases.addDomain(input),
		onSuccess: (domain: AppDomain, input: AddDomainInput) =>
			storeDomain(client, input.app, domain),
	}
}

interface ProxiedContext {
	readonly previous: readonly AppDomain[] | undefined
}

/** El switch se mueve enseguida; si el backend falla, vuelve a como estaba. */
export function setProxiedMutation(client: QueryClient) {
	return {
		mutationFn: (input: SetProxiedInput) => ingressUseCases.setProxied(input),
		onMutate: async (input: SetProxiedInput): Promise<ProxiedContext> => {
			const key = ingressKeys.domains(input.app)
			await client.cancelQueries({ queryKey: key })
			const previous = client.getQueryData<readonly AppDomain[]>(key)
			client.setQueryData<readonly AppDomain[]>(key, (current) =>
				current?.map((domain) =>
					domain.hostname === input.hostname
						? { ...domain, proxied: input.proxied }
						: domain,
				),
			)
			return { previous }
		},
		onError: (_error: unknown, input: SetProxiedInput, context: ProxiedContext | undefined) => {
			if (context?.previous) {
				client.setQueryData(ingressKeys.domains(input.app), context.previous)
			}
		},
		onSuccess: (domain: AppDomain, input: SetProxiedInput) =>
			storeDomain(client, input.app, domain),
	}
}

export function changeTargetMutation(client: QueryClient) {
	return {
		mutationFn: (input: ChangeTargetInput) => ingressUseCases.changeTarget(input),
		onSuccess: (domain: AppDomain, input: ChangeTargetInput) =>
			storeDomain(client, input.app, domain),
	}
}

export function removeDomainMutation(client: QueryClient) {
	return {
		mutationFn: (ref: DomainRef) => ingressUseCases.removeDomain(ref),
		onSuccess: (_result: void, ref: DomainRef) => {
			client.setQueryData<readonly AppDomain[]>(ingressKeys.domains(ref.app), (current) =>
				current?.filter((domain) => domain.hostname !== ref.hostname),
			)
			return refreshAfterChange(client, ref.app)
		},
	}
}

export function adoptRecordMutation(client: QueryClient) {
	return {
		mutationFn: (ref: DomainRef) => ingressUseCases.adoptRecord(ref),
		onSuccess: (domain: AppDomain, ref: DomainRef) => storeDomain(client, ref.app, domain),
	}
}

export function retryDomainMutation(client: QueryClient) {
	return {
		mutationFn: (ref: DomainRef) => ingressUseCases.retryDomain(ref),
		onSuccess: (domain: AppDomain, ref: DomainRef) => storeDomain(client, ref.app, domain),
	}
}
