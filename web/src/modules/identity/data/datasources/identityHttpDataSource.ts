import { z } from 'zod'
import { http } from '@/shared/data/http/client'
import { parseOrThrow } from '@/shared/data/http/parse'
import {
	type CreatedTokenDto,
	createdTokenDtoSchema,
	type CreateTokenPayload,
	type InviteDto,
	inviteDtoSchema,
	type ProjectRefDto,
	projectRefDtoSchema,
	type StageRefDto,
	stageRefDtoSchema,
	type TokenDto,
	tokenDtoSchema,
	type UserDto,
	userDtoSchema,
} from '../dtos/identityDto'

const usersSchema = z.array(userDtoSchema)
const tokensSchema = z.array(tokenDtoSchema)
const projectsSchema = z.array(projectRefDtoSchema)
const stagesSchema = z.array(stageRefDtoSchema)

const segment = encodeURIComponent

function withSignal(signal: AbortSignal | undefined) {
	return signal ? { signal } : {}
}

// Las rutas de administración de identity (routes.go): todas exigen una sesión completa.
export const identityHttpDataSource = {
	async listUsers(signal?: AbortSignal): Promise<UserDto[]> {
		const payload = await http.get<unknown>('/users', withSignal(signal))
		return parseOrThrow(usersSchema, payload, 'identity.users')
	},

	async deleteUser(username: string): Promise<void> {
		await http.delete<unknown>(`/users/${segment(username)}`)
	},

	async resetUserTotp(username: string): Promise<void> {
		await http.delete<unknown>(`/users/${segment(username)}/totp`)
	},

	async createInvite(): Promise<InviteDto> {
		const payload = await http.post<unknown>('/invites')
		return parseOrThrow(inviteDtoSchema, payload, 'identity.invite')
	},

	async listTokens(signal?: AbortSignal): Promise<TokenDto[]> {
		const payload = await http.get<unknown>('/tokens', withSignal(signal))
		return parseOrThrow(tokensSchema, payload, 'identity.tokens')
	},

	async createToken(body: CreateTokenPayload): Promise<CreatedTokenDto> {
		const payload = await http.post<unknown>('/tokens', { body })
		return parseOrThrow(createdTokenDtoSchema, payload, 'identity.token.create')
	},

	async deleteToken(id: number): Promise<void> {
		await http.delete<unknown>(`/tokens/${id}`)
	},
}

// Lo que el modal de tokens lee de catalog. Es un datasource propio y no el barrel de
// catalog porque los dos módulos se hicieron en paralelo; la integración puede cambiarlo.
export const scopeHttpDataSource = {
	async listProjects(signal?: AbortSignal): Promise<ProjectRefDto[]> {
		const payload = await http.get<unknown>('/projects', withSignal(signal))
		return parseOrThrow(projectsSchema, payload, 'identity.scope.projects')
	},

	async listStages(project: string, signal?: AbortSignal): Promise<StageRefDto[]> {
		const payload = await http.get<unknown>(
			`/projects/${segment(project)}/stages`,
			withSignal(signal),
		)
		return parseOrThrow(stagesSchema, payload, 'identity.scope.stages')
	},
}
