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

// Una sesión nueva (login, código, setup, invitación) o cerrada: TODO el cache era de la
// anterior (o de ninguna). Se vacía entero para que nadie vea datos de otro usuario.
function startOver(client: QueryClient): void {
	client.clear()
}

// Mismo usuario, otro estado (activó 2FA): solo `me` cambió.
function forgetMe(client: QueryClient): void {
	client.removeQueries({ queryKey: authKeys.all })
}

export function loginMutation(client: QueryClient) {
	return {
		mutationFn: (input: LoginInput) => authUseCases.login(input),
		onSuccess: () => startOver(client),
	}
}

export function completeTotpMutation(client: QueryClient) {
	return {
		mutationFn: (code: string) => authUseCases.completeTotp(code),
		onSuccess: () => startOver(client),
	}
}

/**
 * Cerrar sesión vacía TODO el cache, pero solo si salió bien: si falló, la sesión sigue
 * abierta y la pantalla queda como estaba (ver `makeLogout`).
 */
export function logoutMutation(client: QueryClient) {
	return {
		mutationFn: () => authUseCases.logout(),
		onSuccess: () => startOver(client),
	}
}

export function setupMutation(client: QueryClient) {
	return {
		mutationFn: (input: SetupInput) => authUseCases.setup(input),
		onSuccess: () => startOver(client),
	}
}

export function acceptInviteMutation(client: QueryClient) {
	return {
		mutationFn: (input: AcceptInviteInput) => authUseCases.acceptInvite(input),
		onSuccess: () => startOver(client),
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
		onSuccess: () => forgetMe(client),
	}
}
