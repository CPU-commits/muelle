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

// Sin las dos claves que pueden faltar: `stage` (omitempty) y `created_by` (backend viejo).
const TOKEN_BASE = {
	id: 7,
	name: 'github-actions',
	project: 'auditfy',
	created_at: '2026-10-10T19:36:53.770Z',
	last_used_at: null,
	expires_at: '2026-12-31T19:36:53.770Z',
}

const TOKEN_DTO = { ...TOKEN_BASE, stage: 'staging', created_by: 'felipe' }

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
			scope: { project: 'auditfy', stage: 'staging' },
			createdBy: { kind: 'user', username: 'felipe' },
			createdAt: new Date('2026-10-10T19:36:53.770Z'),
			lastUsedAt: null,
			expiresAt: new Date('2026-12-31T19:36:53.770Z'),
		})
	})

	it('sin stage es todo el proyecto; created_by null es un usuario borrado', () => {
		const token = tokenFromDto({ ...TOKEN_BASE, created_by: null })

		expect(token.scope).toEqual({ project: 'auditfy', stage: null })
		expect(token.createdBy).toEqual({ kind: 'deleted' })
	})

	it('un backend que no manda created_by deja el creador como desconocido', () => {
		const parsed = tokenDtoSchema.parse({ ...TOKEN_BASE, stage: 'staging' })

		expect(tokenFromDto(parsed).createdBy).toEqual({ kind: 'unknown' })
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
			scope: { project: 'auditfy', stage: null },
			expiresInDays: 0,
		}

		expect(createTokenPayload(token)).toEqual({
			name: 'ci',
			project: 'auditfy',
			stage: '',
			expires_in_days: 0,
		})
		expect(
			createTokenPayload({ ...token, scope: { project: 'auditfy', stage: 'prod' } }).stage,
		).toBe('prod')
	})

	it('de catalog solo toma el slug y el nombre', () => {
		expect(projectRefFromDto({ slug: 'auditfy', name: 'Auditfy' })).toEqual({
			slug: 'auditfy',
			name: 'Auditfy',
		})
		expect(stageRefFromDto({ slug: 'staging' })).toEqual({ slug: 'staging' })
	})
})
