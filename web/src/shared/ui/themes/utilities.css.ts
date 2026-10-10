import { style } from '@vanilla-extract/css'
import { theme } from './theme.css'

export const srOnly = style({
	position: 'absolute',
	width: '1px',
	height: '1px',
	padding: 0,
	margin: '-1px',
	overflow: 'hidden',
	clip: 'rect(0, 0, 0, 0)',
	whiteSpace: 'nowrap',
	borderWidth: 0,
})

// Un link dentro de un texto: subrayado, para que no dependa solo del color.
export const textLink = style({
	color: theme.colors.text.primary,
	textDecoration: 'underline',
	textUnderlineOffset: '3px',
	textDecorationColor: theme.colors.borders.focus,
	selectors: {
		'&:hover': {
			color: theme.colors.text.strong,
			textDecorationColor: theme.colors.text.primary,
		},
	},
})

// Recorta con "…" lo que no entra en una línea (imágenes, digests, redes).
export const truncate = style({
	minWidth: 0,
	overflow: 'hidden',
	textOverflow: 'ellipsis',
	whiteSpace: 'nowrap',
})
