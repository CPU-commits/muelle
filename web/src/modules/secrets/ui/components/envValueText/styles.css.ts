import { style, styleVariants } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

const mono = {
	fontFamily: theme.typography.fontFamily.mono,
	fontSize: theme.typography.fontSize[13],
}

export const tones = styleVariants({
	secondary: { color: theme.colors.text.secondary },
	tertiary: { color: theme.colors.text.tertiary },
})

export type ValueTone = keyof typeof tones

// Una sola línea: un valor de varias líneas (una clave PEM) se ve recortado.
export const text = style({
	...mono,
	minWidth: 0,
	overflow: 'hidden',
	textOverflow: 'ellipsis',
	whiteSpace: 'nowrap',
})

export const struck = style({
	textDecoration: 'line-through',
})

export const masked = style({
	...mono,
	display: 'inline-flex',
	alignItems: 'center',
	gap: theme.space[8],
	minWidth: 0,
})

export const mask = style({
	whiteSpace: 'nowrap',
})

// El candado, más apagado que el texto (como en el diseño).
export const lock = style({
	display: 'inline-flex',
	flex: 'none',
	color: theme.colors.text.tertiary,
})

export const secretLabel = style({
	fontFamily: theme.typography.fontFamily.sans,
})

export const empty = style({
	fontSize: theme.typography.fontSize[13],
	fontStyle: 'italic',
	color: theme.colors.text.tertiary,
})
