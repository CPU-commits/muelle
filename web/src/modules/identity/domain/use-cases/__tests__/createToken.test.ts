import { beforeEach, describe, expect, it } from 'vitest'
import {
	InvalidTokenExpiry,
	InvalidTokenName,
	makeCreateToken,
	MissingTokenProject,
	TokenScopeNotFound,
} from '../..'
import { InMemoryIdentityRepository } from '../../repositories/__fakes__/inMemoryIdentityRepository'

const TODAY = new Date(2026, 9, 10, 9, 0)

describe('createToken', () => {
	let repo: InMemoryIdentityRepository

	beforeEach(() => {
		repo = new InMemoryIdentityRepository({
			projects: { auditfy: ['staging', 'production'] },
			now: () => TODAY,
		})
	})

	const create = () => makeCreateToken({ tokens: repo.asTokens() }, () => TODAY)

	it('crea un token de stage con vencimiento y devuelve el valor en claro', async () => {
		const created = await create()({
			name: ' github-actions ',
			project: 'auditfy',
			stage: 'staging',
			expiresOn: '2026-12-31',
		})

		expect(created.secret).toMatch(/^mk_/)
		expect(created.token).toMatchObject({
			name: 'github-actions',
			scope: { project: 'auditfy', stage: 'staging' },
			createdBy: { kind: 'user', username: 'felipe' },
			lastUsedAt: null,
		})
		expect(created.token.expiresAt?.getTime()).toBe(new Date(2026, 9, 10 + 82, 9, 0).getTime())
	})

	it('sin stage es de todo el proyecto, y sin fecha no vence', async () => {
		const created = await create()({
			name: 'ci',
			project: 'auditfy',
			stage: '',
			expiresOn: '',
		})

		expect(created.token.scope).toEqual({ project: 'auditfy', stage: null })
		expect(created.token.expiresAt).toBeNull()
	})

	it('valida nombre, proyecto y fecha antes de mandar nada', async () => {
		const input = { name: 'ci', project: 'auditfy', stage: '', expiresOn: '' }

		await expect(create()({ ...input, name: '  ' })).rejects.toBeInstanceOf(InvalidTokenName)
		await expect(create()({ ...input, project: '' })).rejects.toBeInstanceOf(
			MissingTokenProject,
		)
		await expect(create()({ ...input, expiresOn: '2026-10-10' })).rejects.toBeInstanceOf(
			InvalidTokenExpiry,
		)
		expect(repo.calls.create).toBe(0)
	})

	it('un proyecto o stage que no existe vuelve como TokenScopeNotFound', async () => {
		const input = { name: 'ci', project: 'auditfy', stage: 'qa', expiresOn: '' }

		await expect(create()(input)).rejects.toBeInstanceOf(TokenScopeNotFound)
		await expect(create()({ ...input, project: 'otro', stage: '' })).rejects.toBeInstanceOf(
			TokenScopeNotFound,
		)
	})
})
