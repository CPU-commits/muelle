import type { z } from 'zod'
import { ContractError } from './errors'

// Valida la forma de una respuesta en el borde. Si el backend cambia un campo, el
// error aparece aca con la ruta exacta, no tres capas mas arriba como un
// `undefined` en la UI.
export function parseOrThrow<T>(schema: z.ZodType<T>, payload: unknown, context: string): T {
	const result = schema.safeParse(payload)
	if (!result.success) {
		const detail = result.error.issues
			.map((issue) => `${issue.path.join('.') || '(root)'}: ${issue.message}`)
			.join('; ')
		throw new ContractError(context, detail)
	}
	return result.data
}
