<script lang="ts">
	import Icon from '../../atoms/icon/Icon.svelte'
	import Logo from '../../atoms/logo/Logo.svelte'
	import Menu from '../menu/Menu.svelte'
	import {
		chevron,
		home,
		initial as initialClass,
		item as itemClass,
		list,
		nav,
		selector,
		separator,
		segment,
		segmentLink,
	} from './styles.css'
	import type { BreadcrumbItem } from './types'

	// "logo / proyecto / stage / app" del header de una app. El logo vuelve al inicio. Un
	// segmento puede ser link, texto o el selector del diseño (`kind: 'menu'`).
	type Props = {
		/** Nombre de la nav ("Ubicación"). */
		label: string
		homeHref: string
		/** Nombre del link del logo ("muelle, inicio"). */
		homeLabel: string
		items: readonly BreadcrumbItem[]
	}

	let { label, homeHref, homeLabel, items }: Props = $props()

	const selectorClass = [segment, segmentLink, selector].join(' ')
</script>

{#snippet content(crumb: BreadcrumbItem)}
	{#if crumb.initial}
		<span class={initialClass} aria-hidden="true">{crumb.initial}</span>
	{/if}
	{crumb.label}
{/snippet}

<!-- eslint-disable svelte/no-navigation-without-resolve -- los href llegan ya resueltos (resolve()) desde quien usa el componente -->
<nav aria-label={label} class={nav}>
	<ol class={list}>
		<li class={itemClass}>
			<a href={homeHref} aria-label={homeLabel} class={home}><Logo /></a>
		</li>
		{#each items as crumb, index (index)}
			{@const isLast = index === items.length - 1}
			<li class={itemClass}>
				<span class={separator} aria-hidden="true">/</span>
				{#if crumb.kind === 'menu'}
					<Menu
						label={crumb.ariaLabel}
						items={crumb.items}
						triggerClass={selectorClass}
						align="start"
					>
						{#snippet trigger()}
							{@render content(crumb)}
							<span class={chevron}><Icon name="chevronUpDown" size={14} /></span>
						{/snippet}
					</Menu>
				{:else if crumb.href}
					<a
						href={crumb.href}
						class={[segment, segmentLink]}
						aria-current={isLast ? 'page' : undefined}
					>
						{@render content(crumb)}
					</a>
				{:else}
					<span class={segment} aria-current={isLast ? 'page' : undefined}>
						{@render content(crumb)}
					</span>
				{/if}
			</li>
		{/each}
	</ol>
</nav>
