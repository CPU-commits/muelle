import type {
	ApiToken,
	CreatedApiToken,
	Invite,
	NewApiToken,
	ProjectRef,
	StageRef,
	TokenCreator,
	TokenId,
	TokenScope,
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
	if (createdBy === null) return { kind: 'deleted' }
	return { kind: 'user', username: createdBy }
}

function dateOrNull(value: string | null): Date | null {
	return value === null ? null : new Date(value)
}

// El schema ya exige un proyecto sin `all_projects`.
function scopeFromDto(dto: TokenDto): TokenScope {
	if (dto.all_projects || dto.project === undefined) return { kind: 'allProjects' }
	return { kind: 'project', project: dto.project, stage: dto.stage ? dto.stage : null }
}

export function tokenFromDto(dto: TokenDto): ApiToken {
	return {
		id: dto.id as TokenId,
		name: dto.name,
		scope: scopeFromDto(dto),
		readOnly: dto.read_only,
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
	const { scope } = token
	if (scope.kind === 'allProjects') {
		return {
			name: token.name,
			project: '',
			stage: '',
			read_only: true,
			all_projects: true,
			expires_in_days: token.expiresInDays,
		}
	}
	return {
		name: token.name,
		project: scope.project,
		stage: scope.stage ?? '',
		read_only: token.readOnly,
		all_projects: false,
		expires_in_days: token.expiresInDays,
	}
}

export function projectRefFromDto(dto: ProjectRefDto): ProjectRef {
	return { slug: dto.slug, name: dto.name }
}

export function stageRefFromDto(dto: StageRefDto): StageRef {
	return { slug: dto.slug }
}
