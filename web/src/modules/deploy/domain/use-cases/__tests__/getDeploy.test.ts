import { describe, expect, it } from 'vitest'
import { NotFoundError } from '@/shared/domain'
import { APP, makeDeploy } from '../../repositories/__fakes__/deployFixtures'
import { InMemoryDeployRepository } from '../../repositories/__fakes__/inMemoryDeployRepository'
import { asDeployId } from '../../value-objects/deployId'
import { makeGetDeploy } from '../getDeploy'

describe('getDeploy', () => {
	it('busca el deploy por id', async () => {
		const repo = new InMemoryDeployRepository()
		repo.deploys = [makeDeploy(143)]
		const get = makeGetDeploy({ deploys: repo })

		await expect(get(APP, asDeployId(143))).resolves.toMatchObject({ id: 143 })
		await expect(get(APP, asDeployId(9))).rejects.toBeInstanceOf(NotFoundError)
	})
})
