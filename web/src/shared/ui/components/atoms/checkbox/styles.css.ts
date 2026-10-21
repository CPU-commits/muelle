import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const label = style({
	display: 'inline-flex',
	alignItems: 'center',
	gap: theme.space[8],
	minHeight: theme.control.xs,
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.secondary,
	cursor: 'pointer',
	selectors: {
		'&:has(input:disabled)': { opacity: 0.5, cursor: 'not-allowed' },
	},
})

// Nativo con el color del diseño (`accent-color`): el navegador dibuja el tilde y el
// foco, y sigue funcionando con el teclado y los lectores de pantalla.
export const box = style({
	margin: 0,
	width: '16px',
	height: '16px',
	flexShrink: 0,
	accentColor: theme.colors.text.primary,
	cursor: 'inherit',
})
