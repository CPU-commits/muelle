import { style, styleVariants } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

// Base compartida de input, textarea y select: fondo de página, borde de control y, con
// foco, borde más claro y un anillo suave (el `:focus-visible` global queda apagado).
export const control = style({
	width: '100%',
	minWidth: 0,
	paddingInline: theme.space[12],
	backgroundColor: theme.colors.background.page,
	color: theme.colors.text.primary,
	border: `1px solid ${theme.colors.borders.control}`,
	borderRadius: theme.radius.md,
	fontSize: theme.typography.fontSize[14],
	fontFamily: 'inherit',
	transition: `border-color ${theme.transition.fast}, box-shadow ${theme.transition.fast}`,
	selectors: {
		'&::placeholder': { color: theme.colors.text.tertiary },
		'&:focus, &:focus-visible': {
			outline: 'none',
			borderColor: theme.colors.borders.focus,
			boxShadow: theme.shadows.focusRing,
		},
		'&:disabled': {
			opacity: 0.5,
			cursor: 'not-allowed',
		},
		'&[readonly]': {
			color: theme.colors.text.secondary,
		},
	},
})

export const controlSizes = styleVariants({
	sm: { height: theme.control.sm, fontSize: theme.typography.fontSize[13] },
	md: { height: theme.control.md },
	lg: { height: theme.control.lg },
})

export const mono = style({
	fontFamily: theme.typography.fontFamily.mono,
	fontSize: theme.typography.fontSize[13],
})

export const invalid = style({
	borderColor: theme.colors.status.error,
	selectors: {
		'&:focus, &:focus-visible': { borderColor: theme.colors.status.error },
	},
})

export const multiline = style({
	height: 'auto',
	minHeight: '96px',
	paddingBlock: theme.space[10],
	lineHeight: theme.typography.lineHeight.relaxed,
	resize: 'vertical',
})

export const select = style({
	paddingInline: `${theme.space[10]} ${theme.space[32]}`,
	appearance: 'none',
	// Flecha dibujada con dos gradientes: sin imágenes ni hex sueltos.
	backgroundImage: `linear-gradient(45deg, transparent 50%, ${theme.colors.text.tertiary} 50%), linear-gradient(135deg, ${theme.colors.text.tertiary} 50%, transparent 50%)`,
	backgroundPosition: 'calc(100% - 17px) 50%, calc(100% - 12px) 50%',
	backgroundSize: '5px 5px, 5px 5px',
	backgroundRepeat: 'no-repeat',
	cursor: 'pointer',
})

export type ControlSize = keyof typeof controlSizes
