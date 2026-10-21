import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

// Un input con la unidad adentro, a la derecha ("10 s", "512 MB"), como los del
// diseño. El borde y el foco son los de los controles de `shared/ui`.
export const box = style({
	display: 'flex',
	alignItems: 'center',
	height: theme.control.md,
	boxSizing: 'border-box',
	minWidth: 0,
	border: `1px solid ${theme.colors.borders.control}`,
	borderRadius: theme.radius.md,
	backgroundColor: theme.colors.background.page,
	overflow: 'hidden',
	transition: `border-color ${theme.transition.fast}, box-shadow ${theme.transition.fast}`,
	selectors: {
		'&:focus-within': {
			borderColor: theme.colors.borders.focus,
			boxShadow: theme.shadows.focusRing,
		},
	},
})

export const boxInvalid = style({
	borderColor: theme.colors.status.error,
	selectors: {
		'&:focus-within': { borderColor: theme.colors.status.error },
	},
})

export const input = style({
	flex: '1 1 auto',
	minWidth: 0,
	width: '100%',
	height: '100%',
	paddingInline: theme.space[12],
	border: 0,
	backgroundColor: 'transparent',
	color: theme.colors.text.primary,
	fontFamily: theme.typography.fontFamily.mono,
	fontSize: theme.typography.fontSize[13],
	selectors: {
		'&:focus, &:focus-visible': { outline: 'none' },
		'&::placeholder': { color: theme.colors.text.tertiary },
	},
})

export const unit = style({
	flex: 'none',
	paddingInline: theme.space[10],
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
	userSelect: 'none',
})
