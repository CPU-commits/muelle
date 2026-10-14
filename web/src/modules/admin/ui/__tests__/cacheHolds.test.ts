import { describe, expect, it } from 'vitest'
import { createQueryClient } from '@/shared/data/query'
import { ValidationError } from '@/shared/domain'
import { cacheHolds } from './cacheHolds'

// El helper con el que los tests de la UI aseguran que un secreto no quedó en el cache. Si
// no mirara adentro de los errores (JSON.stringify de un Error es `{}`), un secreto en el
// mensaje de un error de una mutación pasaría sin que nadie lo vea.
describe('cacheHolds', () => {
	async function failedMutation(error: unknown) {
		const client = createQueryClient({ retry: false })
		await client
			.getMutationCache()
			.build(client, { mutationFn: () => Promise.reject(error) })
			.execute(undefined)
			.catch(() => undefined)
		return client
	}

	it('encuentra un secreto en el mensaje de un error', async () => {
		const client = await failedMutation(new Error('falló con mk_secreto_en_el_mensaje'))

		expect(await cacheHolds(client, 'mk_secreto_en_el_mensaje')).toBe(true)
	})

	it('y en sus propiedades propias y en su `cause`', async () => {
		const withCause = new ValidationError('inválido', 'name', {
			cause: new Error('el backend dijo mk_secreto_en_la_causa'),
		})
		const withProperty = Object.assign(new Error('x'), { body: 'mk_secreto_en_el_body' })

		expect(await cacheHolds(await failedMutation(withCause), 'mk_secreto_en_la_causa')).toBe(
			true,
		)
		expect(await cacheHolds(await failedMutation(withProperty), 'mk_secreto_en_el_body')).toBe(
			true,
		)
	})

	it('en las variables de una mutación y en los datos de una query', async () => {
		const client = createQueryClient({ retry: false })
		client.setQueryData(['x'], { token: 'mk_en_una_query' })
		await client
			.getMutationCache()
			.build(client, { mutationFn: () => Promise.resolve('ok') })
			.execute({ password: 'clave-en-variables' })

		expect(await cacheHolds(client, 'mk_en_una_query')).toBe(true)
		expect(await cacheHolds(client, 'clave-en-variables')).toBe(true)
	})

	it('y no lo inventa', async () => {
		const client = await failedMutation(new Error('falló sin nada raro'))

		expect(await cacheHolds(client, 'mk_que_no_esta')).toBe(false)
	})
})
