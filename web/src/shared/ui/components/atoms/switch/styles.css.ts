import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

// El `m-switch` del diseño (Ajustes · Exigir 2FA): un checkbox nativo con `role="switch"`.
export const track = style({
	appearance: 'none',
	WebkitAppearance: 'none',
	position: 'relative',
	flexShrink: 0,
	width: '32px',
	height: '18px',
	margin: 0,
	borderRadius: theme.radius.full,
	backgroundColor: theme.colors.borders.control,
	cursor: 'pointer',
	transition: `background-color ${theme.transition.fast}`,
	selectors: {
		'&::before': {
			content: '""',
			position: 'absolute',
			top: '2px',
			left: '2px',
			width: '14px',
			height: '14px',
			borderRadius: theme.radius.full,
			backgroundColor: theme.colors.text.secondary,
			transition: `transform ${theme.transition.fast}, background-color ${theme.transition.fast}`,
		},
		'&:checked': { backgroundColor: theme.colors.background.inverse },
		'&:checked::before': {
			transform: 'translateX(14px)',
			backgroundColor: theme.colors.background.page,
		},
		'&:disabled': { opacity: 0.5, cursor: 'not-allowed' },
	},
})
