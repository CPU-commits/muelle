import { type QueryClient, type QueryKey, queryOptions } from '@tanstack/svelte-query'
import { identityUseCases } from '../../container'
import type { CreateTokenInput, DeleteUserInput, TokenId } from '../../domain'

export const identityKeys = {
	all: ['identity'] as QueryKey,
	users: (): QueryKey => ['identity', 'users'],
	tokens: (): QueryKey => ['identity', 'tokens'],
	scopeProjects: (): QueryKey => ['identity', 'scope', 'projects'],
	scopeStages: (project: string): QueryKey => [
		'identity',
		'scope',
		'projects',
		project,
		'stages',
	],
}

export function usersQuery() {
	return queryOptions({
		queryKey: identityKeys.users(),
		queryFn: ({ signal }) => identityUseCases.listUsers(signal),
	})
}

export function tokensQuery() {
	return queryOptions({
		queryKey: identityKeys.tokens(),
		queryFn: ({ signal }) => identityUseCases.listTokens(signal),
	})
}

/** Los proyectos para el alcance de un token. Se piden al abrir el modal, siempre frescos. */
export function scopeProjectsQuery() {
	return queryOptions({
		queryKey: identityKeys.scopeProjects(),
		queryFn: ({ signal }) => identityUseCases.listScopeProjects(signal),
		staleTime: 0,
	})
}

export function scopeStagesQuery(project: string) {
	return queryOptions({
		queryKey: identityKeys.scopeStages(project),
		queryFn: ({ signal }) => identityUseCases.listScopeStages(project, signal),
		enabled: project !== '',
		staleTime: 0,
	})
}

/** Borrar un usuario cambia la lista y el "creado por" de sus tokens. */
export function deleteUserMutation(client: QueryClient) {
	return {
		mutationFn: (input: DeleteUserInput) => identityUseCases.deleteUser(input),
		onSettled: async () => {
			await Promise.all([
				client.invalidateQueries({ queryKey: identityKeys.users() }),
				client.invalidateQueries({ queryKey: identityKeys.tokens() }),
			])
		},
	}
}

export function resetUserTotpMutation(client: QueryClient) {
	return {
		mutationFn: (username: string) => identityUseCases.resetUserTotp(username),
		onSettled: () => client.invalidateQueries({ queryKey: identityKeys.users() }),
	}
}

// La URL de la invitación y el valor del token van en claro: `gcTime: 0` los saca del
// cache de mutaciones apenas nadie los mira. El viewModel los guarda lo justo para
// mostrarlos.
export function createInviteMutation() {
	return {
		mutationFn: () => identityUseCases.createInvite(),
		gcTime: 0,
	}
}

export function createTokenMutation(client: QueryClient) {
	return {
		mutationFn: (input: CreateTokenInput) => identityUseCases.createToken(input),
		gcTime: 0,
		onSuccess: () => client.invalidateQueries({ queryKey: identityKeys.tokens() }),
	}
}

export function revokeTokenMutation(client: QueryClient) {
	return {
		mutationFn: (id: TokenId) => identityUseCases.revokeToken(id),
		onSettled: () => client.invalidateQueries({ queryKey: identityKeys.tokens() }),
	}
}
