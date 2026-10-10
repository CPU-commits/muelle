import { getLocale } from '@/shared/ui/i18n'

export function formatDate(value: Date, locale: string = getLocale()): string {
	return new Intl.DateTimeFormat(locale, { dateStyle: 'medium' }).format(value)
}

export function formatDateTime(value: Date, locale: string = getLocale()): string {
	return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(
		value,
	)
}

export function formatRelative(value: Date, locale: string = getLocale()): string {
	const diffMs = value.getTime() - Date.now()
	const units: [Intl.RelativeTimeFormatUnit, number][] = [
		['year', 1000 * 60 * 60 * 24 * 365],
		['month', 1000 * 60 * 60 * 24 * 30],
		['day', 1000 * 60 * 60 * 24],
		['hour', 1000 * 60 * 60],
		['minute', 1000 * 60],
	]
	const formatter = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' })
	for (const [unit, ms] of units) {
		if (Math.abs(diffMs) >= ms) return formatter.format(Math.round(diffMs / ms), unit)
	}
	return formatter.format(0, 'minute')
}

export function formatNumber(value: number, locale: string = getLocale()): string {
	return new Intl.NumberFormat(locale).format(value)
}
