import { describe, expect, it } from 'vitest'
import {
	InvalidRegistryHost,
	makeAddRegistry,
	makeListRegistries,
	makeRemoveRegistry,
	makeUpdateRegistry,
	RegistryExists,
} from '../..'
import {
	fakeRegistry,
	InMemoryRegistryRepository,
} from '../../repositories/__fakes__/inMemoryAdminRepository'

describe('registries', () => {
	it('agrega, cambia las credenciales y borra; la contraseña no vuelve', async () => {
		const registries = new InMemoryRegistryRepository()
		const deps = { registries }

		await makeAddRegistry(deps)({
			host: 'index.docker.io',
			username: 'cpu-commits',
			password: 'ghp_x',
		})
		await makeUpdateRegistry(deps)({ host: 'docker.io', username: 'bot', password: 'ghp_y' })

		// El alias de Docker Hub se guardó como docker.io; la contraseña no vuelve.
		const [registry] = await makeListRegistries(deps)()
		expect(registry).toMatchObject({ host: 'docker.io', username: 'bot' })
		expect(registry).not.toHaveProperty('password')
		expect(registry?.updatedAt).toBeInstanceOf(Date)
		expect(registries.registries[0]?.password).toBe('ghp_y')

		await makeRemoveRegistry(deps)('docker.io')
		await expect(makeListRegistries(deps)()).resolves.toEqual([])
	})

	it('valida el host antes de mandarlo y propaga un host repetido', async () => {
		const registries = new InMemoryRegistryRepository([fakeRegistry({ host: 'ghcr.io' })])
		const add = makeAddRegistry({ registries })

		await expect(
			add({ host: 'ghcr.io/org', username: 'a', password: 'b' }),
		).rejects.toBeInstanceOf(InvalidRegistryHost)
		expect(registries.calls.create).toBe(0)
		await expect(add({ host: 'ghcr.io', username: 'a', password: 'b' })).rejects.toBeInstanceOf(
			RegistryExists,
		)
	})
})
