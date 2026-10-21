import { style, styleVariants } from '@vanilla-extract/css'
import { spin } from '@/shared/ui/themes/animations.css'
import { theme } from '@/shared/ui/themes/theme.css'

export const spinner = style({
	display: 'inline-block',
	flexShrink: 0,
	border: '2px solid currentColor',
	borderTopColor: 'transparent',
	borderRadius: theme.radius.full,
	animation: `${spin} 0.6s linear infinite`,
	'@media': {
		'(prefers-reduced-motion: reduce)': { animationDuration: '1.6s' },
	},
})

export const sizes = styleVariants({
	sm: { width: '14px', height: '14px' },
	md: { width: '18px', height: '18px' },
	lg: { width: '24px', height: '24px' },
})

export type SpinnerSize = keyof typeof sizes
