<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import {
		Banner,
		Button,
		Field,
		firstError,
		Input,
		Modal,
		SegmentedControl,
		Textarea,
	} from '@/shared/ui'
	import { APP_SLUG_MAX, type ProjectSlug, type StageSlug } from '../../../domain'
	import { validateAppSlug, validateComposeYaml, validateImage, validatePort } from './dialogForm'
	import { NewAppViewModel } from './NewApp.viewModel.svelte.ts'
	import { form as formClass, kind, kindHint, kindLabel, row, yaml } from './styles.css'

	type Props = { project: ProjectSlug; stage: StageSlug; onClose: () => void }

	let { project, stage, onClose }: Props = $props()

	const vm = new NewAppViewModel(
		() => ({ project, stage }),
		() => onClose(),
	)
	const formId = $props.id()
</script>

<Modal
	open
	title={m.catalog_new_app_title({ project: vm.project, stage: vm.stage })}
	description={m.catalog_new_app_description()}
	onClose={() => !vm.isPending && onClose()}
>
	<form
		id={formId}
		class={formClass}
		novalidate
		onsubmit={(event) => {
			event.preventDefault()
			event.stopPropagation()
			void vm.form.handleSubmit()
		}}
	>
		{#if vm.errors.form}
			<Banner tone="error" title={vm.errors.form} />
		{/if}

		<vm.form.Field
			name="slug"
			validators={{
				onDynamic: ({ value }) => validateAppSlug(value),
			}}
		>
			{#snippet children(field)}
				<Field
					label={m.catalog_slug()}
					hint={m.catalog_slug_hint({ max: APP_SLUG_MAX })}
					error={firstError(field.state.meta.errors) ?? vm.errors.fields.slug ?? null}
				>
					{#snippet children(a11y)}
						<Input
							{...a11y}
							mono
							name={field.name}
							placeholder={m.catalog_app_slug_placeholder()}
							maxlength={APP_SLUG_MAX}
							autocomplete="off"
							autocapitalize="none"
							spellcheck={false}
							value={field.state.value}
							onblur={field.handleBlur}
							oninput={(event) => {
								field.handleChange(event.currentTarget.value)
								vm.edited('slug')
							}}
						/>
					{/snippet}
				</Field>
			{/snippet}
		</vm.form.Field>

		<div class={kind}>
			<span class={kindLabel} id={`${formId}-kind`}>{m.catalog_app_kind()}</span>
			<SegmentedControl
				labelledBy={`${formId}-kind`}
				name={`${formId}-kind-choice`}
				options={vm.kindOptions}
				value={vm.kind}
				onChange={vm.setKind}
			/>
			<p class={kindHint}>{vm.kindHint}</p>
		</div>

		{#if vm.kind === 'service'}
			<div class={row}>
				<vm.form.Field
					name="image"
					validators={{
						onDynamic: ({ value, fieldApi }) =>
							fieldApi.form.getFieldValue('kind') === 'service'
								? validateImage(value)
								: undefined,
					}}
				>
					{#snippet children(field)}
						<Field
							label={m.catalog_app_image()}
							error={firstError(field.state.meta.errors) ??
								vm.errors.fields.image ??
								null}
						>
							{#snippet children(a11y)}
								<Input
									{...a11y}
									mono
									name={field.name}
									placeholder={m.catalog_app_image_placeholder()}
									autocomplete="off"
									autocapitalize="none"
									spellcheck={false}
									value={field.state.value}
									onblur={field.handleBlur}
									oninput={(event) => {
										field.handleChange(event.currentTarget.value)
										vm.edited('image')
									}}
								/>
							{/snippet}
						</Field>
					{/snippet}
				</vm.form.Field>

				<vm.form.Field
					name="port"
					validators={{
						onDynamic: ({ value, fieldApi }) =>
							fieldApi.form.getFieldValue('kind') === 'service'
								? validatePort(value)
								: undefined,
					}}
				>
					{#snippet children(field)}
						<Field
							label={m.catalog_app_port()}
							error={firstError(field.state.meta.errors) ??
								vm.errors.fields.port ??
								null}
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
									placeholder="3000"
									value={field.state.value}
									onblur={field.handleBlur}
									oninput={(event) => {
										field.handleChange(event.currentTarget.value)
										vm.edited('port')
									}}
								/>
							{/snippet}
						</Field>
					{/snippet}
				</vm.form.Field>
			</div>
		{:else}
			<vm.form.Field
				name="composeYaml"
				validators={{
					onDynamic: ({ value, fieldApi }) =>
						fieldApi.form.getFieldValue('kind') === 'compose'
							? validateComposeYaml(value)
							: undefined,
				}}
			>
				{#snippet children(field)}
					<Field
						label={m.catalog_app_compose_yaml()}
						error={firstError(field.state.meta.errors) ??
							vm.errors.fields.composeYaml ??
							null}
					>
						{#snippet children(a11y)}
							<Textarea
								{...a11y}
								mono
								class={yaml}
								name={field.name}
								spellcheck={false}
								value={field.state.value}
								onblur={field.handleBlur}
								oninput={(event) => {
									field.handleChange(event.currentTarget.value)
									vm.edited('composeYaml')
								}}
							/>
						{/snippet}
					</Field>
				{/snippet}
			</vm.form.Field>
		{/if}
	</form>

	{#snippet footer()}
		<Button variant="secondary" size="sm" disabled={vm.isPending} onclick={onClose}>
			{m.common_cancel()}
		</Button>
		<Button type="submit" form={formId} size="sm" loading={vm.isPending}>
			{m.common_create()}
		</Button>
	{/snippet}
</Modal>
