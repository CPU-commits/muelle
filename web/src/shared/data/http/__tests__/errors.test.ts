import { describe, expect, it } from 'vitest'
import { apiErrorCode, apiErrorFromResponse, apiErrorStatus, UNKNOWN_ERROR_CODE } from '../errors'

describe('apiErrorFromResponse', () => {
	it('lee el sobre {"error":{code,message}} del backend', () => {
		const error = apiErrorFromResponse(401, {
			error: { code: 'invalid_credentials', message: 'usuario o contraseña incorrectos' },
		})

		expect(error.status).toBe(401)
		expect(error.code).toBe('invalid_credentials')
		expect(error.message).toBe('usuario o contraseña incorrectos')
	})

	it('sin sobre (un 502 del proxy, un HTML) queda con code unknown', () => {
		const error = apiErrorFromResponse(502, '<html>Bad Gateway</html>')

		expect(error.code).toBe(UNKNOWN_ERROR_CODE)
		expect(error.message).toBe('HTTP 502')
		expect(error.payload).toBe('<html>Bad Gateway</html>')
	})

	it('no acepta un sobre a medias', () => {
		expect(apiErrorFromResponse(400, { error: { code: 'x' } }).code).toBe(UNKNOWN_ERROR_CODE)
		expect(apiErrorFromResponse(400, null).code).toBe(UNKNOWN_ERROR_CODE)
	})

	it('expone status y code solo de un ApiError', () => {
		const error = apiErrorFromResponse(404, {
			error: { code: 'not_found', message: 'no existe' },
		})

		expect(apiErrorStatus(error)).toBe(404)
		expect(apiErrorCode(error)).toBe('not_found')
		expect(apiErrorStatus(new Error('otro'))).toBeNull()
		expect(apiErrorCode('nada')).toBeNull()
	})
})
