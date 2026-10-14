import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

// Una app en Proyectos: toda la fila es un link. Las columnas tienen el ancho del
// diseño y envuelven en un teléfono.
export const row = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	gap: `${theme.space[4]} ${theme.space[16]}`,
	minHeight: '52px',
	boxSizing: 'border-box',
	padding: `${theme.space[8]} ${theme.space[12]} ${theme.space[8]} ${theme.space[16]}`,
	backgroundColor: theme.colors.background.surface,
	color: theme.colors.text.primary,
	textDecoration: 'none',
	transition: `background-color ${theme.transition.fast}`,
	selectors: {
		'&:hover': {
			backgroundColor: theme.colors.background.hover,
			color: theme.colors.text.primary,
		},
		// Dentro de una tarjeta con overflow hidden, el anillo va hacia adentro.
		'&:focus-visible': { outlineOffset: '-2px' },
	},
})

export const name = style({
	flex: '1 1 200px',
	minWidth: 0,
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	gap: `${theme.space[6]} ${theme.space[10]}`,
})

export const slug = style({ fontWeight: theme.typography.fontWeight.medium })

export const dot = style({ display: 'inline-flex', width: '8px', flex: 'none' })

export const status = style({
	flex: '0 0 150px',
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.secondary,
})

export const image = style({
	flex: '2 1 220px',
	minWidth: 0,
	overflow: 'hidden',
	textOverflow: 'ellipsis',
	whiteSpace: 'nowrap',
	fontFamily: theme.typography.fontFamily.mono,
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.secondary,
})

export const deploy = style({
	flex: '0 0 170px',
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.secondary,
})

export const chevron = style({
	display: 'inline-flex',
	color: theme.colors.text.disabled,
})
