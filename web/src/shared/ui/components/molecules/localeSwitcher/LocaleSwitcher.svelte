<script lang="ts">
	import { getLocale, isLocale, localeLabel, locales, setLocale } from '@/shared/ui/i18n'
	import { srOnly } from '@/shared/ui/themes/utilities.css'
	import { root, select } from './styles.css'

	let { label }: { label: string } = $props()
</script>

<label class={root}>
	<span class={srOnly}>{label}</span>
	<select
		class={select}
		value={getLocale()}
		onchange={(event) => {
			const next = event.currentTarget.value
			if (isLocale(next)) void setLocale(next)
		}}
	>
		{#each locales as locale (locale)}
			<option value={locale}>{localeLabel(locale)}</option>
		{/each}
	</select>
</label>
