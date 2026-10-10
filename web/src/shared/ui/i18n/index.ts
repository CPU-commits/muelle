import {
	baseLocale,
	getLocale,
	isLocale,
	locales,
	setLocale,
} from '@/shared/ui/i18n/paraglide/runtime'

export type Locale = (typeof locales)[number]

export { baseLocale, getLocale, isLocale, locales, setLocale }

export const LOCALE_LABELS: Record<string, string> = {
	es: 'Español',
	en: 'English',
}

export function localeLabel(locale: string): string {
	return LOCALE_LABELS[locale] ?? locale
}

// El otro idioma, para el item "English" / "Español" del menú de la cuenta.
export function nextLocale(current: string = getLocale()): Locale {
	const index = locales.indexOf(current as Locale)
	return locales[(index + 1) % locales.length] ?? baseLocale
}
