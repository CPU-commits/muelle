<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Banner, Button, Card, Field, firstError, Input, SegmentedControl } from '@/shared/ui'
	import { UnitInput } from '../../components/unitInput'
	import type { ServiceConfigViewModel } from './ServiceConfig.viewModel.svelte.ts'
	import { form as formClass, grid4, narrow, preview } from './styles.css'

	let { vm, id }: { vm: ServiceConfigViewModel; id: string } = $props()

	const timings = [
		{ name: 'intervalSeconds', label: m.catalog_config_health_interval(), unit: true },
		{ name: 'timeoutSeconds', label: m.catalog_config_health_timeout(), unit: true },
		{ name: 'retries', label: m.catalog_config_health_retries(), unit: false },
		{ name: 'startPeriodSeconds', label: m.catalog_config_health_start(), unit: true },
	] as const
</script>

<form
	class={formClass}
	novalidate
	onsubmit={(event) => {
		event.preventDefault()
		event.stopPropagation()
		void vm.health.form.handleSubmit()
	}}
>
	<Card
		{id}
		title={m.catalog_config_health()}
		description={m.catalog_config_health_description()}
		note={vm.healthRequirement}
	>
		{#if vm.health.saver.formError}
			<Banner tone="error" title={vm.health.saver.formError} />
		{/if}

		<SegmentedControl
			label={m.catalog_config_health()}
			name={`${id}-type`}
			options={vm.healthOptions}
			value={vm.healthType}
			onChange={vm.setHealthType}
		/>

		{#if vm.healthType === 'http' || vm.healthType === 'bun-http'}
			<div class={narrow}>
				<vm.health.form.Field name="path">
					{#snippet children(field)}
						<Field
							label={m.catalog_config_health_path()}
							error={firstError(field.state.meta.errors) ??
								vm.health.saver.serverError('path', field.state.value)}
						>
							{#snippet children(a11y)}
								<Input
									{...a11y}
									mono
									name={field.name}
									autocomplete="off"
									autocapitalize="none"
									spellcheck={false}
									value={field.state.value}
									onblur={field.handleBlur}
									oninput={(event) =>
										field.handleChange(event.currentTarget.value)}
								/>
							{/snippet}
						</Field>
					{/snippet}
				</vm.health.form.Field>
			</div>
		{:else if vm.healthType === 'command'}
			<div class={narrow}>
				<vm.health.form.Field name="command">
					{#snippet children(field)}
						<Field
							label={m.catalog_config_command()}
							hint={m.catalog_config_command_hint()}
							error={firstError(field.state.meta.errors) ??
								vm.health.saver.serverError('command', field.state.value)}
						>
							{#snippet children(a11y)}
								<Input
									{...a11y}
									mono
									name={field.name}
									placeholder="node dist/healthcheck.js"
									autocomplete="off"
									autocapitalize="none"
									spellcheck={false}
									value={field.state.value}
									onblur={field.handleBlur}
									oninput={(event) =>
										field.handleChange(event.currentTarget.value)}
								/>
							{/snippet}
						</Field>
					{/snippet}
				</vm.health.form.Field>
			</div>
		{/if}

		{#if vm.healthType === 'none'}
			<Banner tone="warn" title={m.catalog_config_health_none_warning()} />
		{:else}
			<div class={grid4}>
				{#each timings as timing (timing.name)}
					<vm.health.form.Field name={timing.name}>
						{#snippet children(field)}
							<Field
								label={timing.label}
								error={firstError(field.state.meta.errors) ??
									vm.health.saver.serverError(timing.name, field.state.value)}
							>
								{#snippet children(a11y)}
									{#if timing.unit}
										<UnitInput
											{...a11y}
											unit={m.catalog_unit_seconds()}
											type="number"
											inputmode="numeric"
											name={field.name}
											value={field.state.value}
											onblur={field.handleBlur}
											oninput={(event) =>
												field.handleChange(event.currentTarget.value)}
										/>
									{:else}
										<Input
											{...a11y}
											mono
											type="number"
											inputmode="numeric"
											name={field.name}
											value={field.state.value}
											onblur={field.handleBlur}
											oninput={(event) =>
												field.handleChange(event.currentTarget.value)}
										/>
									{/if}
								{/snippet}
							</Field>
						{/snippet}
					</vm.health.form.Field>
				{/each}
			</div>
			{#if vm.healthPreview}
				<pre
					class={preview}
					aria-label={m.catalog_config_health_preview()}>{vm.healthPreview}</pre>
			{/if}
		{/if}

		{#snippet footer()}
			<Button type="submit" size="sm" loading={vm.health.saver.isPending}>
				{m.common_save()}
			</Button>
		{/snippet}
	</Card>
</form>
