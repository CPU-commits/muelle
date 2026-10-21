<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Banner, Button, Card, Field, firstError, Input } from '@/shared/ui'
	import { UnitInput } from '../../components/unitInput'
	import type { SectionForm } from './sectionForm.svelte.ts'
	import type { ResourcesValues } from './sections'
	import { form as formClass, grid3 } from './styles.css'

	let { section, id }: { section: SectionForm<ResourcesValues>; id: string } = $props()
</script>

<form
	class={formClass}
	novalidate
	onsubmit={(event) => {
		event.preventDefault()
		event.stopPropagation()
		void section.form.handleSubmit()
	}}
>
	<Card
		{id}
		title={m.catalog_config_resources()}
		description={m.catalog_config_resources_description()}
	>
		{#if section.saver.formError}
			<Banner tone="error" title={section.saver.formError} />
		{/if}
		<div class={grid3}>
			<section.form.Field name="memoryMb">
				{#snippet children(field)}
					<Field
						label={m.catalog_config_memory()}
						optional
						error={firstError(field.state.meta.errors) ??
							section.saver.serverError('memoryMb', field.state.value)}
					>
						{#snippet children(a11y)}
							<UnitInput
								{...a11y}
								unit={m.catalog_unit_mb()}
								type="number"
								inputmode="numeric"
								min="16"
								placeholder={m.catalog_config_unlimited()}
								name={field.name}
								value={field.state.value}
								onblur={field.handleBlur}
								oninput={(event) => field.handleChange(event.currentTarget.value)}
							/>
						{/snippet}
					</Field>
				{/snippet}
			</section.form.Field>

			<section.form.Field name="cpus">
				{#snippet children(field)}
					<Field
						label={m.catalog_config_cpus()}
						optional
						error={firstError(field.state.meta.errors) ??
							section.saver.serverError('cpus', field.state.value)}
					>
						{#snippet children(a11y)}
							<Input
								{...a11y}
								mono
								type="number"
								inputmode="decimal"
								step="0.1"
								min="0.01"
								placeholder={m.catalog_config_unlimited()}
								name={field.name}
								value={field.state.value}
								onblur={field.handleBlur}
								oninput={(event) => field.handleChange(event.currentTarget.value)}
							/>
						{/snippet}
					</Field>
				{/snippet}
			</section.form.Field>

			<section.form.Field name="deployTimeoutSeconds">
				{#snippet children(field)}
					<Field
						label={m.catalog_config_timeout()}
						error={firstError(field.state.meta.errors) ??
							section.saver.serverError('deployTimeoutSeconds', field.state.value)}
					>
						{#snippet children(a11y)}
							<UnitInput
								{...a11y}
								unit={m.catalog_unit_seconds()}
								type="number"
								inputmode="numeric"
								min="30"
								max="3600"
								name={field.name}
								value={field.state.value}
								onblur={field.handleBlur}
								oninput={(event) => field.handleChange(event.currentTarget.value)}
							/>
						{/snippet}
					</Field>
				{/snippet}
			</section.form.Field>
		</div>

		{#snippet footer()}
			<Button type="submit" size="sm" loading={section.saver.isPending}>
				{m.common_save()}
			</Button>
		{/snippet}
	</Card>
</form>
