import type { Brand } from './branded'
import { ValidationError } from './errors'

export type Email = Brand<string, 'Email'>

const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]{2,}$/

export function isEmail(raw: string): boolean {
	return EMAIL_PATTERN.test(raw.trim())
}

export function parseEmail(raw: string, field = 'email'): Email {
	const normalized = raw.trim().toLowerCase()
	if (!EMAIL_PATTERN.test(normalized)) {
		throw new ValidationError(`"${raw}" no es un email válido`, field)
	}
	return normalized as Email
}
