import { describe, expect, it } from 'vitest'
import { NoPreviousDeploy } from '../../errors/deployErrors'
import { APP, makeDeploy } from '../../repositories/__fakes__/deployFixtures'
import { InMemoryDeployRepository } from '../../repositories/__fakes__/inMemoryDeployRepository'
import { makeRollbackDeploy } from '../rollbackDeploy'

describe('rollbackDeploy', () => {
	it('con un succeeded anterior encola el rollback', async () => {
		const repo = new InMemoryDeployRepository()
		repo.deploys = [makeDeploy(140), makeDeploy(143)]

		await expect(makeRollbackDeploy({ deploys: repo })(APP)).resolves.toEqual({ id: 144 })
		expect(repo.rollbacks).toBe(1)
	})

	it('sin anterior rechaza con NoPreviousDeploy (el 409 del spec)', async () => {
		const repo = new InMemoryDeployRepository()
		repo.deploys = [makeDeploy(1)]

		await expect(makeRollbackDeploy({ deploys: repo })(APP)).rejects.toBeInstanceOf(
			NoPreviousDeploy,
		)
	})
})
