export const breakpoints = {
	xs: '480px',
	sm: '640px',
	md: '768px',
	lg: '1024px',
	xl: '1280px',
} as const

// Mobile-first: cada query aplica DESDE ese ancho.
export const media = {
	xs: `screen and (min-width: ${breakpoints.xs})`,
	sm: `screen and (min-width: ${breakpoints.sm})`,
	md: `screen and (min-width: ${breakpoints.md})`,
	lg: `screen and (min-width: ${breakpoints.lg})`,
	xl: `screen and (min-width: ${breakpoints.xl})`,
} as const
