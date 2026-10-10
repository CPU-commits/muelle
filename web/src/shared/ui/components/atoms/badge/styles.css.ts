import { style, styleVariants } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const badge = style({
	display: 'inline-flex',
	alignItems: 'center',
	gap: theme.space[6],
	flexShrink: 0,
	paddingInline: theme.space[8],
	border: '1px solid',
	fontSize: theme.typography.fontSize[12],
	fontWeight: theme.typography.fontWeight.medium,
	lineHeight: 1,
	whiteSpace: 'nowrap',
})

export const tones = styleVariants({
	neutral: { borderColor: theme.colors.borders.control, color: theme.colors.text.secondary },
	ok: { borderColor: theme.colors.borders.control, color: theme.colors.text.secondary },
	warn: { borderColor: theme.colors.status.warnBorder, color: theme.colors.status.warnText },
	error: { borderColor: theme.colors.status.errorBorder, color: theme.colors.status.errorText },
	// El "compose" de una fila: etiqueta técnica en Geist Mono.
	mono: {
		borderColor: theme.colors.borders.control,
		color: theme.colors.text.secondary,
		fontFamily: theme.typography.fontFamily.mono,
		fontSize: theme.typography.fontSize[11],
		fontWeight: theme.typography.fontWeight.regular,
		paddingInline: theme.space[6],
	},
	// "nuevo" en la lista de tokens: más claro que el neutro.
	strong: { borderColor: theme.colors.borders.strong, color: theme.colors.text.primary },
})

export const shapes = styleVariants({
	tag: { borderRadius: theme.radius.xs },
	pill: { borderRadius: theme.radius.full },
})

export const sizes = styleVariants({
	sm: { height: theme.control.badge },
	md: { height: theme.control.badgeLg, paddingInline: theme.space[10] },
})

export type BadgeTone = keyof typeof tones
export type BadgeShape = keyof typeof shapes
export type BadgeSize = keyof typeof sizes
