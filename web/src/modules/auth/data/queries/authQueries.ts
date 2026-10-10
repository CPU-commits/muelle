import { type QueryClient, type QueryKey, queryOptions } from '@tanstack/svelte-query'
import { authUseCases } from '../../container'
import {
	type AcceptInviteInput,
	type LoginInput,
	type Me,
	type SessionState,
	type SetupInput,
	sessionStateFromError,
} from '../../domain'

export const authKeys = {
	all: ['auth'] as QueryKey,
	me: (): QueryKey => ['auth', 'me'],
}

/** Quién tiene la sesión. Lo carga el guard de `(app)`; las pantallas lo leen del cache. */
export function meQuery() {
	return queryOptions({
		queryKey: authKeys.me(),
		queryFn: ({ signal }) => authUseCases.getMe(signal),
		staleTime: 60_000,
	})
}

/**
 * Para los guards de las rutas (`+layout.ts`, `+page.ts`): consulta `me` a través del
 * cache y lo traduce a un estado. Un backend caído se propaga (la ruta muestra el error)
 * en vez de mandar al login.
 */
export async function loadSessionState(client: QueryClient): Promise<SessionState> {
	try {
		const me: Me = await client.fetchQuery({ ...meQuery(), retry: false })
		return { status: 'signed_in', me }
	} catch (error) {
		const state = sessionStateFromError(error)
		if (state) return state
		throw error
	}
}

// Al abrir o cerrar una sesión, lo que había en el cache era de otra (o de ninguna).
function forgetSession(client: QueryClient): void {
	client.removeQueries({ queryKey: authKeys.all })
}

export function loginMutation(client: QueryClient) {
	return {
		mutationFn: (input: LoginInput) => authUseCases.login(input),
		onSuccess: () => forgetSession(client),
	}
}

export function completeTotpMutation(client: QueryClient) {
	return {
		mutationFn: (code: string) => authUseCases.completeTotp(code),
		onSuccess: () => forgetSession(client),
	}
}

/** Cerrar sesión vacía TODO el cache: el siguiente usuario no ve datos del anterior. */
export function logoutMutation(client: QueryClient) {
	return {
		mutationFn: () => authUseCases.logout(),
		onSettled: () => client.clear(),
	}
}

export function setupMutation(client: QueryClient) {
	return {
		mutationFn: (input: SetupInput) => authUseCases.setup(input),
		onSuccess: () => forgetSession(client),
	}
}

export function acceptInviteMutation(client: QueryClient) {
	return {
		mutationFn: (input: AcceptInviteInput) => authUseCases.acceptInvite(input),
		onSuccess: () => forgetSession(client),
	}
}

export function beginTotpSetupMutation() {
	return {
		mutationFn: () => authUseCases.beginTotpSetup(),
	}
}

export function enableTotpMutation(client: QueryClient) {
	return {
		mutationFn: (code: string) => authUseCases.enableTotp(code),
		onSuccess: () => forgetSession(client),
	}
}
