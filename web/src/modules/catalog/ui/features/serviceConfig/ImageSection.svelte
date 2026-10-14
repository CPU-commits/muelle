<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Banner, Button, Card, firstError, Input } from '@/shared/ui'
	import type { SectionForm } from './sectionForm.svelte.ts'
	import type { ImageValues } from './sections'
	import { fieldError, form as formClass, imageField } from './styles.css'

	let { section, id }: { section: SectionForm<ImageValues>; id: string } = $props()

	const errorId = $props.id()
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
	<Card {id} title={m.catalog_config_image()} description={m.catalog_config_image_description()}>
		{#if section.saver.formError}
			<Banner tone="error" title={section.saver.formError} />
		{/if}
		<section.form.Field name="image">
			{#snippet children(field)}
				{@const error =
					firstError(field.state.meta.errors) ??
					section.saver.serverError('image', field.state.value)}
				<div class={imageField}>
					<Input
						mono
						name={field.name}
						aria-label={m.catalog_config_image()}
						aria-invalid={error !== null}
						aria-describedby={error ? errorId : undefined}
						autocomplete="off"
						autocapitalize="none"
						spellcheck={false}
						value={field.state.value}
						onblur={field.handleBlur}
						oninput={(event) => field.handleChange(event.currentTarget.value)}
					/>
					{#if error}
						<p id={errorId} class={fieldError}>{error}</p>
					{/if}
				</div>
			{/snippet}
		</section.form.Field>

		{#snippet footer()}
			<Button type="submit" size="sm" loading={section.saver.isPending}>
				{m.common_save()}
			</Button>
		{/snippet}
	</Card>
</form>
