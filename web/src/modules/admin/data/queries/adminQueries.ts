import {
	infiniteQueryOptions,
	type QueryClient,
	type QueryKey,
	queryOptions,
} from '@tanstack/svelte-query'
import { adminUseCases } from '../../container'
import type {
	AuditEventId,
	RegistryInput,
	Settings,
	UpdateProxyPortsInput,
	UpdatePublicIpInput,
	UpdateSecurityInput,
} from '../../domain'

export const adminKeys = {
	all: ['admin'] as QueryKey,
	settings: (): QueryKey => ['admin', 'settings'],
	audit: (): QueryKey => ['admin', 'audit'],
	registries: (): QueryKey => ['admin', 'registries'],
}

export function settingsQuery() {
	return queryOptions({
		queryKey: adminKeys.settings(),
		queryFn: ({ signal }) => adminUseCases.getSettings(signal),
	})
}

/** La auditoría, de a una página: cada una pide la siguiente con `before`. */
export function auditQuery() {
	return infiniteQueryOptions({
		queryKey: adminKeys.audit(),
		queryFn: ({ pageParam, signal }) => adminUseCases.listAudit({ before: pageParam }, signal),
		initialPageParam: null as AuditEventId | null,
		getNextPageParam: (lastPage) => lastPage.nextBefore,
		staleTime: 0,
	})
}

export function registriesQuery() {
	return queryOptions({
		queryKey: adminKeys.registries(),
		queryFn: ({ signal }) => adminUseCases.listRegistries(signal),
	})
}

// Cada cambio de ajustes devuelve los ajustes resultantes: van directo al cache. Y deja
// un evento en la auditoría.
function settingsMutation<TInput>(client: QueryClient, run: (input: TInput) => Promise<Settings>) {
	return {
		mutationFn: run,
		onSuccess: async (settings: Settings) => {
			client.setQueryData(adminKeys.settings(), settings)
			await client.invalidateQueries({ queryKey: adminKeys.audit() })
		},
	}
}

export function updateSecurityMutation(client: QueryClient) {
	return settingsMutation(client, (input: UpdateSecurityInput) =>
		adminUseCases.updateSecurity(input),
	)
}

export function updatePublicIpMutation(client: QueryClient) {
	return settingsMutation(client, (input: UpdatePublicIpInput) =>
		adminUseCases.updatePublicIp(input),
	)
}

export function updateAcmeEmailMutation(client: QueryClient) {
	return settingsMutation(client, (email: string) => adminUseCases.updateAcmeEmail(email))
}

export function updateProxyPortsMutation(client: QueryClient) {
	return settingsMutation(client, (input: UpdateProxyPortsInput) =>
		adminUseCases.updateProxyPorts(input),
	)
}

// El token de Cloudflare y la contraseña de un registry van en las variables de la
// mutación: `gcTime: 0` las saca del cache de mutaciones apenas el viewModel hace `reset()`.
const SECRET = { gcTime: 0 } as const

export function replaceCloudflareTokenMutation(client: QueryClient) {
	return {
		...settingsMutation(client, (token: string) => adminUseCases.replaceCloudflareToken(token)),
		...SECRET,
	}
}

function registryMutation<TInput, TResult>(
	client: QueryClient,
	run: (input: TInput) => Promise<TResult>,
) {
	return {
		mutationFn: run,
		onSettled: async () => {
			await Promise.all([
				client.invalidateQueries({ queryKey: adminKeys.registries() }),
				client.invalidateQueries({ queryKey: adminKeys.audit() }),
			])
		},
	}
}

export function addRegistryMutation(client: QueryClient) {
	return {
		...registryMutation(client, (input: RegistryInput) => adminUseCases.addRegistry(input)),
		...SECRET,
	}
}

export function updateRegistryMutation(client: QueryClient) {
	return {
		...registryMutation(client, (input: RegistryInput) => adminUseCases.updateRegistry(input)),
		...SECRET,
	}
}

export function removeRegistryMutation(client: QueryClient) {
	return registryMutation(client, (host: string) => adminUseCases.removeRegistry(host))
}
