<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Banner, Button, Card, Field, firstError, Input } from '@/shared/ui'
	import type { SectionForm } from './sectionForm.svelte.ts'
	import type { RunValues } from './sections'
	import { form as formClass, grid2, narrow } from './styles.css'

	let { section, id }: { section: SectionForm<RunValues>; id: string } = $props()
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
	<Card {id} title={m.catalog_config_run()} description={m.catalog_config_run_description()}>
		{#if section.saver.formError}
			<Banner tone="error" title={section.saver.formError} />
		{/if}
		<div class={grid2}>
			<section.form.Field name="port">
				{#snippet children(field)}
					<Field
						label={m.catalog_config_port()}
						error={firstError(field.state.meta.errors) ??
							section.saver.serverError('port', field.state.value)}
					>
						{#snippet children(a11y)}
							<Input
								{...a11y}
								mono
								type="number"
								inputmode="numeric"
								min="1"
								max="65535"
								name={field.name}
								value={field.state.value}
								onblur={field.handleBlur}
								oninput={(event) => field.handleChange(event.currentTarget.value)}
							/>
						{/snippet}
					</Field>
				{/snippet}
			</section.form.Field>

			<section.form.Field name="replicas">
				{#snippet children(field)}
					<Field
						label={m.catalog_config_replicas()}
						error={firstError(field.state.meta.errors) ??
							section.saver.serverError('replicas', field.state.value)}
					>
						{#snippet children(a11y)}
							<Input
								{...a11y}
								mono
								type="number"
								inputmode="numeric"
								min="0"
								max="50"
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

		<div class={narrow}>
			<section.form.Field name="command">
				{#snippet children(field)}
					<Field
						label={m.catalog_config_command()}
						optional
						hint={m.catalog_config_command_hint()}
						error={firstError(field.state.meta.errors) ??
							section.saver.serverError('command', field.state.value)}
					>
						{#snippet children(a11y)}
							<Input
								{...a11y}
								mono
								name={field.name}
								placeholder={m.catalog_config_command_placeholder()}
								autocomplete="off"
								autocapitalize="none"
								spellcheck={false}
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
