import {
	httpScopeRepository,
	httpTokenRepository,
	httpUserRepository,
} from './data/repositories/httpIdentityRepositories'
import {
	makeCreateInvite,
	makeCreateToken,
	makeDeleteUser,
	makeListScopeProjects,
	makeListScopeStages,
	makeListTokens,
	makeListUsers,
	makeResetUserTotp,
	makeRevokeToken,
} from './domain'

// Composition root del módulo: el ÚNICO lugar que conoce los repositorios concretos.
const users = { users: httpUserRepository }
const tokens = { tokens: httpTokenRepository }
const scopes = { scopes: httpScopeRepository }

export const identityUseCases = {
	listUsers: makeListUsers(users),
	deleteUser: makeDeleteUser(users),
	resetUserTotp: makeResetUserTotp(users),
	createInvite: makeCreateInvite(users),
	listTokens: makeListTokens(tokens),
	createToken: makeCreateToken(tokens),
	revokeToken: makeRevokeToken(tokens),
	listScopeProjects: makeListScopeProjects(scopes),
	listScopeStages: makeListScopeStages(scopes),
}
