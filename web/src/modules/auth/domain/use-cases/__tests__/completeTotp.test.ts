import { beforeEach, describe, expect, it } from 'vitest'
import { InvalidCode, MalformedCode, makeCompleteTotp, makeLogin, SessionExpired } from '../..'
import {
	FAKE_VALID_CODE,
	InMemoryAuthRepository,
} from '../../repositories/__fakes__/inMemoryAuthRepository'

describe('completeTotp', () => {
	let auth: InMemoryAuthRepository

	beforeEach(async () => {
		auth = new InMemoryAuthRepository({
			users: [{ username: 'ana', password: 'otra-contraseña-larga', totpEnabled: true }],
		})
		await makeLogin({ auth })({ username: 'ana', password: 'otra-contraseña-larga' })
	})

	it('con el código correcto completa la sesión', async () => {
		await makeCompleteTotp({ auth })(FAKE_VALID_CODE)

		expect(auth.session?.mfaPending).toBe(false)
	})

	it('acepta el código con espacios', async () => {
		await makeCompleteTotp({ auth })('123 456')

		expect(auth.session?.mfaPending).toBe(false)
	})

	it('rechaza un código que no tiene 6 dígitos sin llamar al backend', async () => {
		await expect(makeCompleteTotp({ auth })('12')).rejects.toBeInstanceOf(MalformedCode)
		expect(auth.calls.completeTotp).toBe(0)
	})

	it('propaga un código incorrecto', async () => {
		await expect(makeCompleteTotp({ auth })('000000')).rejects.toBeInstanceOf(InvalidCode)
	})

	it('un doble envío (la sesión ya está completa) cuenta como éxito', async () => {
		const complete = makeCompleteTotp({ auth })
		await complete(FAKE_VALID_CODE)

		await expect(complete(FAKE_VALID_CODE)).resolves.toBeUndefined()
	})

	it('sin sesión pendiente propaga SessionExpired', async () => {
		auth.session = null

		await expect(makeCompleteTotp({ auth })(FAKE_VALID_CODE)).rejects.toBeInstanceOf(
			SessionExpired,
		)
	})
})
