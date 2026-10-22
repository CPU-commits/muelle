import { beforeEach, describe, expect, it } from 'vitest'
import {
	ALL_PROJECTS,
	createTokenProblems,
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
			readOnly: false,
			expiresOn: '2026-12-31',
		})

		expect(created.secret).toMatch(/^mk_/)
		expect(created.token).toMatchObject({
			name: 'github-actions',
			scope: { kind: 'project', project: 'auditfy', stage: 'staging' },
			readOnly: false,
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
			readOnly: false,
			expiresOn: '',
		})

		expect(created.token.scope).toEqual({ kind: 'project', project: 'auditfy', stage: null })
		expect(created.token.expiresAt).toBeNull()
	})

	it('un token de un proyecto puede ser de solo lectura', async () => {
		const created = await create()({
			name: 'claude',
			project: 'auditfy',
			stage: 'staging',
			readOnly: true,
			expiresOn: '',
		})

		expect(created.token).toMatchObject({
			scope: { kind: 'project', project: 'auditfy', stage: 'staging' },
			readOnly: true,
		})
	})

	it('"*" es todos los proyectos: siempre de solo lectura, sin stage', async () => {
		const created = await create()({
			name: 'claude-debug',
			project: ALL_PROJECTS,
			// Un stage o un readOnly que quedaron de antes no cuentan.
			stage: 'staging',
			readOnly: false,
			expiresOn: '2027-01-08',
		})

		expect(created.token).toMatchObject({ scope: { kind: 'allProjects' }, readOnly: true })
		expect(created.token.expiresAt?.getTime()).toBe(new Date(2026, 9, 10 + 90, 9, 0).getTime())
	})

	it('valida nombre, proyecto y fecha antes de mandar nada', async () => {
		const input = { name: 'ci', project: 'auditfy', stage: '', readOnly: false, expiresOn: '' }

		await expect(create()({ ...input, name: '  ' })).rejects.toBeInstanceOf(InvalidTokenName)
		await expect(create()({ ...input, project: '' })).rejects.toBeInstanceOf(
			MissingTokenProject,
		)
		await expect(create()({ ...input, expiresOn: '2026-10-10' })).rejects.toBeInstanceOf(
			InvalidTokenExpiry,
		)
		expect(repo.calls.create).toBe(0)
	})

	it('createTokenProblems junta todos los problemas, no solo el primero', () => {
		const problems = createTokenProblems(
			{ name: '', project: '', stage: '', readOnly: false, expiresOn: '2026-10-10' },
			TODAY,
		)

		expect(problems.map((problem) => problem.constructor)).toEqual([
			InvalidTokenName,
			MissingTokenProject,
			InvalidTokenExpiry,
		])
		expect(
			createTokenProblems(
				{ name: 'ci', project: 'auditfy', stage: '', readOnly: false, expiresOn: '' },
				TODAY,
			),
		).toEqual([])
	})

	it('con "*" el proyecto está elegido, y sin fecha también vale (no vence)', () => {
		expect(
			createTokenProblems(
				{ name: 'ci', project: ALL_PROJECTS, stage: '', readOnly: true, expiresOn: '' },
				TODAY,
			),
		).toEqual([])
	})

	it('un proyecto o stage que no existe vuelve como TokenScopeNotFound', async () => {
		const input = {
			name: 'ci',
			project: 'auditfy',
			stage: 'qa',
			readOnly: false,
			expiresOn: '',
		}

		await expect(create()(input)).rejects.toBeInstanceOf(TokenScopeNotFound)
		await expect(create()({ ...input, project: 'otro', stage: '' })).rejects.toBeInstanceOf(
			TokenScopeNotFound,
		)
	})
})
