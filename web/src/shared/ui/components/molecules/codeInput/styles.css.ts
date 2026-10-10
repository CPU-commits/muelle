import { globalStyle, style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const root = style({
	position: 'relative',
	width: '100%',
})

// El input real cubre las celdas, transparente: recibe el foco, el pegado, el
// autocompletado del sistema (`one-time-code`) y lo que lee un lector de pantalla.
export const input = style({
	position: 'absolute',
	inset: 0,
	width: '100%',
	height: '100%',
	margin: 0,
	padding: 0,
	border: 0,
	opacity: 0,
	fontSize: '16px',
	cursor: 'text',
	selectors: {
		'&:disabled': { cursor: 'not-allowed' },
	},
})

export const cells = style({
	display: 'grid',
	gridTemplateColumns: 'repeat(var(--code-length, 6), minmax(0, 1fr))',
	gap: theme.space[8],
	pointerEvents: 'none',
})

export const cell = style({
	display: 'grid',
	placeItems: 'center',
	height: theme.control.xl,
	border: `1px solid ${theme.colors.borders.control}`,
	borderRadius: theme.radius.md,
	backgroundColor: theme.colors.background.page,
	fontFamily: theme.typography.fontFamily.mono,
	fontSize: theme.typography.fontSize[20],
	transition: `border-color ${theme.transition.fast}, box-shadow ${theme.transition.fast}`,
})

export const cellFilled = style({
	borderColor: theme.colors.borders.focus,
})

export const cellActive = style({})

export const invalid = style({})

export const disabled = style({ opacity: 0.5 })

globalStyle(`${root}:focus-within ${cellActive}`, {
	borderColor: theme.colors.borders.focus,
	boxShadow: theme.shadows.focusRing,
})

globalStyle(`${root}:has(input:focus-visible)`, {
	outline: `2px solid ${theme.colors.borders.outline}`,
	outlineOffset: '4px',
	borderRadius: theme.radius.md,
})

globalStyle(`${invalid} ${cell}`, {
	borderColor: theme.colors.status.error,
})
