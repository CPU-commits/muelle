import { style, styleVariants } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const mono = style({
	fontFamily: theme.typography.fontFamily.mono,
	fontVariantLigatures: 'none',
})

export const tones = styleVariants({
	inherit: {},
	primary: { color: theme.colors.text.primary },
	secondary: { color: theme.colors.text.secondary },
	tertiary: { color: theme.colors.text.tertiary },
	// El sufijo apagado de una tarea, un "—": legible (≥ 4.5:1), apenas más tenue que tertiary.
	disabled: { color: theme.colors.text.faint },
})

export const sizes = styleVariants({
	inherit: {},
	11: { fontSize: theme.typography.fontSize[11] },
	12: { fontSize: theme.typography.fontSize[12] },
	13: { fontSize: theme.typography.fontSize[13] },
	14: { fontSize: theme.typography.fontSize[14] },
})

export const truncate = style({
	display: 'inline-block',
	maxWidth: '100%',
	minWidth: 0,
	overflow: 'hidden',
	textOverflow: 'ellipsis',
	whiteSpace: 'nowrap',
	verticalAlign: 'bottom',
})

export type MonoTone = keyof typeof tones
export type MonoSize = keyof typeof sizes
