import { InvalidTokenExpiry, type TokenExpiryProblem } from '../errors/identityErrors'

// El backend recibe `expires_in_days` (0 = no vence) y lo suma a su reloj; el panel pide
// una fecha, como el diseño. Este archivo traduce una a la otra.
export const TOKEN_MAX_DAYS = 3650

const DAY_MS = 24 * 60 * 60 * 1000
const DATE_INPUT = /^(\d{4})-(\d{2})-(\d{2})$/

function startOfDay(date: Date): Date {
	return new Date(date.getFullYear(), date.getMonth(), date.getDate())
}

/** El valor de un `<input type="date">` ("2026-12-31") como medianoche local, o null. */
function parseDateInput(value: string): Date | null {
	const match = DATE_INPUT.exec(value)
	if (!match) return null
	const [, year = '', month = '', day = ''] = match
	const date = new Date(Number(year), Number(month) - 1, Number(day))
	// `new Date(2026, 1, 31)` es el 3 de marzo: una fecha que no existe se rechaza.
	if (date.getMonth() !== Number(month) - 1 || date.getDate() !== Number(day)) return null
	return date
}

function daysBetween(from: Date, to: Date): number {
	// Redondeado: un día con cambio de horario dura 23 o 25 horas.
	return Math.round((startOfDay(to).getTime() - startOfDay(from).getTime()) / DAY_MS)
}

export function tokenExpiryProblem(
	dateInput: string,
	today: Date = new Date(),
): TokenExpiryProblem | null {
	if (dateInput.trim() === '') return null
	const date = parseDateInput(dateInput.trim())
	if (!date) return 'invalid'
	const days = daysBetween(today, date)
	if (days < 1) return 'past'
	if (days > TOKEN_MAX_DAYS) return 'too_far'
	return null
}

/** Días desde hoy hasta la fecha elegida; `''` es 0, que el backend lee como "no vence". */
export function expiresInDays(dateInput: string, today: Date = new Date()): number {
	const problem = tokenExpiryProblem(dateInput, today)
	if (problem) throw new InvalidTokenExpiry(problem)
	const date = parseDateInput(dateInput.trim())
	return date ? daysBetween(today, date) : 0
}

function toDateInput(date: Date): string {
	const month = String(date.getMonth() + 1).padStart(2, '0')
	const day = String(date.getDate()).padStart(2, '0')
	return `${date.getFullYear()}-${month}-${day}`
}

/**
 * Lo que propone el panel para un token de todos los proyectos (diseño del MCP, §9.3): se
 * puede cambiar o borrar. El backend no lo exige.
 */
export const ALL_PROJECTS_EXPIRY_DAYS = 90

/** El valor del `<input type="date">` de hoy + `days` días de calendario. */
export function expiryInputInDays(days: number, today: Date = new Date()): string {
	const start = startOfDay(today)
	return toDateInput(new Date(start.getFullYear(), start.getMonth(), start.getDate() + days))
}

/** Los límites del `<input type="date">`: de mañana a 3650 días. */
export function expiryInputRange(today: Date = new Date()): { min: string; max: string } {
	const start = startOfDay(today)
	const min = new Date(start.getFullYear(), start.getMonth(), start.getDate() + 1)
	const max = new Date(start.getFullYear(), start.getMonth(), start.getDate() + TOKEN_MAX_DAYS)
	return { min: toDateInput(min), max: toDateInput(max) }
}
