<script lang="ts">
	import { ICONS, type IconName, type IconShape } from './icons'
	import { icon } from './styles.css'

	type Props = {
		name: IconName
		/** En px. */
		size?: number
		/** Si el ícono es el único contenido con significado, su texto. Sin label es decorativo. */
		label?: string
	}

	let { name, size = 16, label }: Props = $props()

	const shape: IconShape = $derived(ICONS[name])
</script>

<svg
	width={size}
	height={size}
	viewBox="0 0 24 24"
	fill={shape.fill ? 'currentColor' : 'none'}
	stroke={shape.fill ? 'none' : 'currentColor'}
	stroke-width="2"
	stroke-linecap="round"
	stroke-linejoin="round"
	role={label ? 'img' : undefined}
	aria-label={label}
	aria-hidden={label ? undefined : 'true'}
	focusable="false"
	class={icon}
>
	{#each shape.paths as d (d)}
		<path {d} />
	{/each}
	{#each shape.circles ?? [] as [cx, cy, r] (`${cx}-${cy}`)}
		<circle {cx} {cy} {r} />
	{/each}
	{#each shape.rects ?? [] as [x, y, width, height, rx] (`${x}-${y}`)}
		<rect {x} {y} {width} {height} {rx} />
	{/each}
</svg>
