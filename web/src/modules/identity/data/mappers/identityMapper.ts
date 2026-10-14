import type {
	ApiToken,
	CreatedApiToken,
	Invite,
	NewApiToken,
	ProjectRef,
	StageRef,
	TokenCreator,
	TokenId,
	User,
} from '../../domain'
import type {
	CreatedTokenDto,
	CreateTokenPayload,
	InviteDto,
	ProjectRefDto,
	StageRefDto,
	TokenDto,
	UserDto,
} from '../dtos/identityDto'

export function userFromDto(dto: UserDto): User {
	return {
		username: dto.username,
		totpEnabled: dto.totp_enabled,
		createdAt: new Date(dto.created_at),
	}
}

export function inviteFromDto(dto: InviteDto): Invite {
	return { url: dto.url, expiresAt: new Date(dto.expires_at) }
}

function creatorFromDto(createdBy: TokenDto['created_by']): TokenCreator {
	if (createdBy === undefined) return { kind: 'unknown' }
	if (createdBy === null) return { kind: 'deleted' }
	return { kind: 'user', username: createdBy }
}

function dateOrNull(value: string | null): Date | null {
	return value === null ? null : new Date(value)
}

export function tokenFromDto(dto: TokenDto): ApiToken {
	return {
		id: dto.id as TokenId,
		name: dto.name,
		scope: { project: dto.project, stage: dto.stage ? dto.stage : null },
		createdBy: creatorFromDto(dto.created_by),
		createdAt: new Date(dto.created_at),
		lastUsedAt: dateOrNull(dto.last_used_at),
		expiresAt: dateOrNull(dto.expires_at),
	}
}

export function createdTokenFromDto(dto: CreatedTokenDto): CreatedApiToken {
	const { token, ...rest } = dto
	return { token: tokenFromDto(rest), secret: token }
}

export function createTokenPayload(token: NewApiToken): CreateTokenPayload {
	return {
		name: token.name,
		project: token.scope.project,
		stage: token.scope.stage ?? '',
		expires_in_days: token.expiresInDays,
	}
}

export function projectRefFromDto(dto: ProjectRefDto): ProjectRef {
	return { slug: dto.slug, name: dto.name }
}

export function stageRefFromDto(dto: StageRefDto): StageRef {
	return { slug: dto.slug }
}
