import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

// El selector de healthcheck de Configuración: radios nativos escondidos dentro de
// labels con forma de pestaña.
export const group = style({
	display: 'flex',
	flexWrap: 'wrap',
	gap: theme.space[2],
	width: 'fit-content',
	maxWidth: '100%',
	padding: '3px',
	border: `1px solid ${theme.colors.borders.subtle}`,
	borderRadius: theme.radius.lg,
	backgroundColor: theme.colors.background.page,
})

export const option = style({
	position: 'relative',
	display: 'inline-flex',
	alignItems: 'center',
	height: theme.control.xs,
	paddingInline: theme.space[12],
	borderRadius: theme.radius.sm,
	fontSize: theme.typography.fontSize[13],
	fontWeight: theme.typography.fontWeight.medium,
	color: theme.colors.text.secondary,
	cursor: 'pointer',
	transition: `background-color ${theme.transition.fast}, color ${theme.transition.fast}`,
	selectors: {
		'&:hover': { color: theme.colors.text.primary },
		'&:has(input:focus-visible)': {
			outline: `2px solid ${theme.colors.borders.outline}`,
			outlineOffset: '1px',
		},
		'&:has(input:disabled)': { opacity: 0.5, cursor: 'not-allowed' },
	},
})

export const optionSelected = style({
	backgroundColor: theme.colors.background.raised,
	color: theme.colors.text.primary,
	boxShadow: `inset 0 0 0 1px ${theme.colors.borders.control}`,
})

export const input = style({
	position: 'absolute',
	opacity: 0,
	width: '1px',
	height: '1px',
	margin: 0,
	pointerEvents: 'none',
})
