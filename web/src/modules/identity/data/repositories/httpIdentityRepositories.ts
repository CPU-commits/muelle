import type { ApiError } from '@/shared/data/http/errors'
import { type DomainErrorMap, withDomainErrors } from '@/shared/data/http/domainErrors'
import type { ScopeRepository, TokenRepository, UserRepository } from '../../domain'
import { CannotDeleteSelf, TokenNotFound, TokenScopeNotFound, UserNotFound } from '../../domain'
import { identityHttpDataSource, scopeHttpDataSource } from '../datasources/identityHttpDataSource'
import {
	createdTokenFromDto,
	createTokenPayload,
	inviteFromDto,
	projectRefFromDto,
	stageRefFromDto,
	tokenFromDto,
	userFromDto,
} from '../mappers/identityMapper'

const cause = (error: ApiError) => ({ cause: error })

// Borrar un usuario o quitarle el 2FA: un 404 es que ya no existe.
const USER_ERRORS: DomainErrorMap = {
	codes: { cannot_delete_self: (error) => new CannotDeleteSelf(cause(error)) },
	notFound: (error) => new UserNotFound(cause(error)),
}

// Crear un token: un 404 es el proyecto o el stage del alcance; un 422, un campo.
const CREATE_TOKEN_ERRORS: DomainErrorMap = {
	notFound: (error) => new TokenScopeNotFound(cause(error)),
	fields: { name: 'name', project: 'project', stage: 'stage', expires_in_days: 'expiresOn' },
}

const REVOKE_TOKEN_ERRORS: DomainErrorMap = {
	notFound: (error) => new TokenNotFound(cause(error)),
}

const SCOPE_ERRORS: DomainErrorMap = {
	notFound: (error) => new TokenScopeNotFound(cause(error)),
}

export const httpUserRepository: UserRepository = {
	list: (signal) =>
		withDomainErrors(async () =>
			(await identityHttpDataSource.listUsers(signal)).map(userFromDto),
		),

	delete: (username) =>
		withDomainErrors(() => identityHttpDataSource.deleteUser(username), USER_ERRORS),

	resetTotp: (username) =>
		withDomainErrors(() => identityHttpDataSource.resetUserTotp(username), USER_ERRORS),

	createInvite: () =>
		withDomainErrors(async () => inviteFromDto(await identityHttpDataSource.createInvite())),
}

export const httpTokenRepository: TokenRepository = {
	list: (signal) =>
		withDomainErrors(async () =>
			(await identityHttpDataSource.listTokens(signal)).map(tokenFromDto),
		),

	create: (token) =>
		withDomainErrors(
			async () =>
				createdTokenFromDto(
					await identityHttpDataSource.createToken(createTokenPayload(token)),
				),
			CREATE_TOKEN_ERRORS,
		),

	revoke: (id) =>
		withDomainErrors(() => identityHttpDataSource.deleteToken(id), REVOKE_TOKEN_ERRORS),
}

export const httpScopeRepository: ScopeRepository = {
	projects: (signal) =>
		withDomainErrors(async () =>
			(await scopeHttpDataSource.listProjects(signal)).map(projectRefFromDto),
		),

	stages: (project, signal) =>
		withDomainErrors(
			async () =>
				(await scopeHttpDataSource.listStages(project, signal)).map(stageRefFromDto),
			SCOPE_ERRORS,
		),
}
