import { type QueryClient, type QueryKey, queryOptions } from '@tanstack/svelte-query'
import { ingressUseCases } from '../../container'
import {
	type AddDomainInput,
	type AppDomain,
	type AppRef,
	type ChangeTargetInput,
	type DomainRef,
	replaceDomain,
	type SetProxiedInput,
	sortDomains,
} from '../../domain'

export const ingressKeys = {
	all: ['ingress'] as QueryKey,
	domains: (app: AppRef): QueryKey => ['ingress', app.project, app.stage, app.app, 'domains'],
}

/** Mientras un dominio esté en `pending`, la lista se refresca sola cada tanto. */
export const PENDING_REFRESH_MS = 5_000

export function domainsQuery(app: AppRef) {
	return queryOptions({
		queryKey: ingressKeys.domains(app),
		queryFn: ({ signal }) => ingressUseCases.listDomains(app, signal),
		staleTime: 10_000,
		// Con la pestaña oculta TanStack no refresca (refetchIntervalInBackground: false).
		refetchInterval: (query) =>
			(query.state.data ?? []).some((domain) => domain.status === 'pending')
				? PENDING_REFRESH_MS
				: false,
		// Ver deploy/data/queries: el resultado "tracked" se pierde cambios de `error` si
		// un viewModel solo leyó `data`.
		notifyOnChangeProps: 'all',
	})
}

/** Pone en el cache el dominio que devolvió el backend y pide la lista de nuevo. */
function storeDomain(client: QueryClient, app: AppRef, domain: AppDomain): Promise<void> {
	client.setQueryData<readonly AppDomain[]>(ingressKeys.domains(app), (current) => {
		if (!current) return current
		const exists = current.some((item) => item.hostname === domain.hostname)
		return exists ? replaceDomain(current, domain) : sortDomains([...current, domain])
	})
	return client.invalidateQueries({ queryKey: ingressKeys.domains(app) })
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
			return client.invalidateQueries({ queryKey: ingressKeys.domains(ref.app) })
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
