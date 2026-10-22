import { describe, expect, it } from 'vitest'
import type { NewApiToken, TokenName } from '../../../domain'
import { createdTokenDtoSchema, tokenDtoSchema } from '../../dtos/identityDto'
import {
	createdTokenFromDto,
	createTokenPayload,
	inviteFromDto,
	projectRefFromDto,
	stageRefFromDto,
	tokenFromDto,
	userFromDto,
} from '../identityMapper'

// Sin `stage`, que falta en un token de todo el proyecto (omitempty). `project` solo falta
// en uno de todos los proyectos.
const TOKEN_BASE = {
	id: 7,
	name: 'github-actions',
	project: 'auditfy',
	read_only: false,
	all_projects: false,
	created_by: 'felipe',
	created_at: '2026-10-10T19:36:53.770Z',
	last_used_at: null,
	expires_at: '2026-12-31T19:36:53.770Z',
}

const TOKEN_DTO = { ...TOKEN_BASE, stage: 'staging' }

/** El DTO sin una de sus claves (no `undefined`: que no venga). */
function without(dto: object, key: string): object {
	return Object.fromEntries(Object.entries(dto).filter(([name]) => name !== key))
}

// Un token de todos los proyectos: sin `project`, de solo lectura.
const ALL_PROJECTS_DTO = {
	...without(TOKEN_BASE, 'project'),
	read_only: true,
	all_projects: true,
}

describe('identityMapper', () => {
	it('traduce un usuario de snake_case al dominio', () => {
		expect(
			userFromDto({
				username: 'soporte',
				totp_enabled: true,
				created_at: '2026-10-09T12:00:00.000Z',
			}),
		).toEqual({
			username: 'soporte',
			totpEnabled: true,
			createdAt: new Date('2026-10-09T12:00:00.000Z'),
		})
	})

	it('traduce la invitación', () => {
		expect(
			inviteFromDto({ url: 'https://panel/invite/abc', expires_at: '2026-10-11T12:00:00Z' }),
		).toEqual({ url: 'https://panel/invite/abc', expiresAt: new Date('2026-10-11T12:00:00Z') })
	})

	it('traduce un token con stage, creador y vencimiento', () => {
		expect(tokenFromDto(TOKEN_DTO)).toEqual({
			id: 7,
			name: 'github-actions',
			scope: { kind: 'project', project: 'auditfy', stage: 'staging' },
			readOnly: false,
			createdBy: { kind: 'user', username: 'felipe' },
			createdAt: new Date('2026-10-10T19:36:53.770Z'),
			lastUsedAt: null,
			expiresAt: new Date('2026-12-31T19:36:53.770Z'),
		})
	})

	it('sin stage es todo el proyecto; created_by null es un usuario borrado', () => {
		const token = tokenFromDto({ ...TOKEN_BASE, created_by: null })

		expect(token.scope).toEqual({ kind: 'project', project: 'auditfy', stage: null })
		expect(token.createdBy).toEqual({ kind: 'deleted' })
	})

	it('un token de solo lectura lo dice; uno de todos los proyectos viene sin proyecto', () => {
		expect(tokenFromDto({ ...TOKEN_DTO, read_only: true }).readOnly).toBe(true)

		const all = tokenDtoSchema.parse(ALL_PROJECTS_DTO)

		expect(tokenFromDto(all)).toMatchObject({ scope: { kind: 'allProjects' }, readOnly: true })
	})

	it('read_only y all_projects vienen siempre: sin ellos no es un token', () => {
		expect(tokenDtoSchema.safeParse(without(TOKEN_DTO, 'read_only')).success).toBe(false)
		expect(tokenDtoSchema.safeParse(without(TOKEN_DTO, 'all_projects')).success).toBe(false)
	})

	it('es de un proyecto o de todos (y entonces de solo lectura y sin stage), nunca los dos', () => {
		expect(tokenDtoSchema.safeParse(ALL_PROJECTS_DTO).success).toBe(true)
		// Los dos a la vez, o ninguno.
		expect(tokenDtoSchema.safeParse({ ...ALL_PROJECTS_DTO, project: 'auditfy' }).success).toBe(
			false,
		)
		expect(tokenDtoSchema.safeParse(without(TOKEN_BASE, 'project')).success).toBe(false)
		// No hay un token de escritura de todos los proyectos (contrato §8.1), ni uno con stage.
		expect(tokenDtoSchema.safeParse({ ...ALL_PROJECTS_DTO, read_only: false }).success).toBe(
			false,
		)
		expect(tokenDtoSchema.safeParse({ ...ALL_PROJECTS_DTO, stage: 'staging' }).success).toBe(
			false,
		)
		// El 201 de POST /tokens sigue la misma regla.
		expect(
			createdTokenDtoSchema.safeParse({
				...ALL_PROJECTS_DTO,
				project: 'auditfy',
				token: 'mk_x',
			}).success,
		).toBe(false)
	})

	it('created_by viene siempre (un username o null): sin él no es un token', () => {
		const { created_by: _creator, ...missing } = TOKEN_DTO
		expect(_creator).toBe('felipe')
		expect(tokenDtoSchema.safeParse(missing).success).toBe(false)
	})

	it('acepta fechas UTC con milisegundos y RFC 3339 con offset', () => {
		const withOffset = { ...TOKEN_DTO, created_at: '2026-10-10T16:36:53.123456789-03:00' }

		expect(tokenDtoSchema.safeParse(TOKEN_DTO).success).toBe(true)
		expect(tokenFromDto(tokenDtoSchema.parse(withOffset)).createdAt.toISOString()).toBe(
			'2026-10-10T19:36:53.123Z',
		)
		expect(tokenDtoSchema.safeParse({ ...TOKEN_DTO, created_at: 'ayer' }).success).toBe(false)
	})

	it('separa el valor en claro del token recién creado', () => {
		const created = createdTokenFromDto(
			createdTokenDtoSchema.parse({ ...TOKEN_DTO, token: 'mk_secreto' }),
		)

		expect(created.secret).toBe('mk_secreto')
		expect(created.token).not.toHaveProperty('token')
		expect(created.token.name).toBe('github-actions')
	})

	it('arma el cuerpo con los nombres exactos de form.CreateToken (json/v2 estricto)', () => {
		const token: NewApiToken = {
			name: 'ci' as TokenName,
			scope: { kind: 'project', project: 'auditfy', stage: null },
			readOnly: false,
			expiresInDays: 0,
		}

		expect(createTokenPayload(token)).toEqual({
			name: 'ci',
			project: 'auditfy',
			stage: '',
			read_only: false,
			all_projects: false,
			expires_in_days: 0,
		})
		const staging = createTokenPayload({
			...token,
			scope: { kind: 'project', project: 'auditfy', stage: 'prod' },
			readOnly: true,
		})
		expect(staging).toMatchObject({ stage: 'prod', read_only: true, all_projects: false })
	})

	it('un token de todos los proyectos va sin proyecto ni stage, y de solo lectura', () => {
		expect(
			createTokenPayload({
				name: 'claude-debug' as TokenName,
				scope: { kind: 'allProjects' },
				readOnly: true,
				expiresInDays: 90,
			}),
		).toEqual({
			name: 'claude-debug',
			project: '',
			stage: '',
			read_only: true,
			all_projects: true,
			expires_in_days: 90,
		})
	})

	it('de catalog solo toma el slug y el nombre', () => {
		expect(projectRefFromDto({ slug: 'auditfy', name: 'Auditfy' })).toEqual({
			slug: 'auditfy',
			name: 'Auditfy',
		})
		expect(stageRefFromDto({ slug: 'staging' })).toEqual({ slug: 'staging' })
	})
})
