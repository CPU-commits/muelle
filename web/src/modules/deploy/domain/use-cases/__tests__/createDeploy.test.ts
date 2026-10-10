import { describe, expect, it } from 'vitest'
import { ValidationError } from '@/shared/domain'
import { APP, makeDeploy } from '../../repositories/__fakes__/deployFixtures'
import { InMemoryDeployRepository } from '../../repositories/__fakes__/inMemoryDeployRepository'
import { makeCreateDeploy } from '../createDeploy'

describe('createDeploy', () => {
	it('encola con la imagen escrita y devuelve el id', async () => {
		const repo = new InMemoryDeployRepository()
		repo.deploys = [makeDeploy(143)]

		const accepted = await makeCreateDeploy({ deploys: repo })({
			app: APP,
			services: ['api'],
			overrides: { api: 'ghcr.io/auditfy/api:sha-1' },
		})

		expect(accepted.id).toBe(144)
		expect(repo.created).toEqual([
			{ app: APP, request: { kind: 'image', image: 'ghcr.io/auditfy/api:sha-1' } },
		])
	})

	it('sin imagen redespliega', async () => {
		const repo = new InMemoryDeployRepository()
		await makeCreateDeploy({ deploys: repo })({ app: APP, services: ['api'], overrides: {} })
		expect(repo.created[0]?.request).toEqual({ kind: 'redeploy' })
	})

	it('una imagen inválida rechaza sin llamar al repositorio', async () => {
		const repo = new InMemoryDeployRepository()
		await expect(
			makeCreateDeploy({ deploys: repo })({
				app: APP,
				services: ['api'],
				overrides: { api: 'no vale' },
			}),
		).rejects.toBeInstanceOf(ValidationError)
		expect(repo.created).toEqual([])
	})
})
