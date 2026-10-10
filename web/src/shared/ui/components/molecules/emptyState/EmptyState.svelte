<script lang="ts">
	import type { Snippet } from 'svelte'
	import Icon from '../../atoms/icon/Icon.svelte'
	import type { IconName } from '../../atoms/icon/icons'
	import {
		action as actionClass,
		body as bodyClass,
		icon as iconClass,
		panel,
		title as titleClass,
	} from './styles.css'

	type Props = { title: string; body?: string; icon?: IconName; action?: Snippet }

	let { title, body, icon, action }: Props = $props()
</script>

<div class={panel}>
	{#if icon}
		<span class={iconClass}><Icon name={icon} size={18} /></span>
	{/if}
	<p class={titleClass}>{title}</p>
	{#if body}
		<p class={bodyClass}>{body}</p>
	{/if}
	{#if action}
		<div class={actionClass}>{@render action()}</div>
	{/if}
</div>
