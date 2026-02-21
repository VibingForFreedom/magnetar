<script lang="ts">
	import { fetchSettings, type Settings } from '$lib/api';
	import SettingsSection from '$lib/components/SettingsSection.svelte';
	import SettingsRow from '$lib/components/SettingsRow.svelte';
	import { Loader2 } from 'lucide-svelte';
	import { onMount } from 'svelte';

	let settings = $state<Settings | null>(null);
	let error = $state<string | null>(null);

	onMount(async () => {
		try {
			settings = await fetchSettings();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load settings';
		}
	});
</script>

<div class="space-y-6">
	<div>
		<h1 class="text-xl font-semibold text-text-primary">Settings</h1>
		<p class="mt-1 text-sm text-text-secondary">Runtime configuration (read-only)</p>
	</div>

	{#if error}
		<div class="rounded-lg border border-accent-red/30 bg-accent-red/10 p-4 text-sm text-accent-red">
			{error}
		</div>
	{:else if !settings}
		<div class="flex items-center justify-center py-12">
			<Loader2 class="h-6 w-6 animate-spin text-text-secondary" />
		</div>
	{:else}
		<div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
			<SettingsSection title="Database">
				<SettingsRow label="Backend" value={settings.database.backend} />
				{#if settings.database.path}
					<SettingsRow label="Path" value={settings.database.path} />
				{/if}
			</SettingsSection>

			<SettingsSection title="Crawler">
				<SettingsRow label="Status" value={settings.crawler.enabled} />
				<SettingsRow label="Workers" value={settings.crawler.workers} />
				<SettingsRow label="DHT Port" value={settings.crawler.port} />
			</SettingsSection>

			<SettingsSection title="Matcher">
				<SettingsRow label="Status" value={settings.matcher.enabled} />
				<SettingsRow label="Batch Size" value={settings.matcher.batch_size} />
				<SettingsRow label="Interval" value={settings.matcher.interval} />
				<SettingsRow label="TMDB" value={settings.matcher.tmdb_enabled} />
				<SettingsRow label="TVDB" value={settings.matcher.tvdb_enabled} />
			</SettingsSection>

			<SettingsSection title="API">
				<SettingsRow label="Port" value={settings.api.port} />
				<SettingsRow label="Authentication" value={settings.api.auth_enabled} />
			</SettingsSection>
		</div>
	{/if}
</div>
