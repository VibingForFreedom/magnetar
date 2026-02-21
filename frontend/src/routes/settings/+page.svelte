<script lang="ts">
	import { fetchSettings, toggleCrawler, toggleMatcher, type Settings } from '$lib/api';
	import SettingsSection from '$lib/components/SettingsSection.svelte';
	import SettingsRow from '$lib/components/SettingsRow.svelte';
	import { Loader2 } from 'lucide-svelte';
	import { onMount } from 'svelte';

	let settings = $state<Settings | null>(null);
	let error = $state<string | null>(null);
	let crawlerToggling = $state(false);
	let matcherToggling = $state(false);

	onMount(async () => {
		try {
			settings = await fetchSettings();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load settings';
		}
	});

	async function onCrawlerToggle() {
		if (!settings || crawlerToggling) return;
		crawlerToggling = true;
		try {
			const res = await toggleCrawler(!settings.crawler.paused);
			settings = { ...settings, crawler: { ...settings.crawler, paused: res.paused } };
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to toggle crawler';
		} finally {
			crawlerToggling = false;
		}
	}

	async function onMatcherToggle() {
		if (!settings || matcherToggling) return;
		matcherToggling = true;
		try {
			const res = await toggleMatcher(!settings.matcher.paused);
			settings = { ...settings, matcher: { ...settings.matcher, paused: res.paused } };
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to toggle matcher';
		} finally {
			matcherToggling = false;
		}
	}
</script>

<div class="space-y-6">
	<div>
		<h1 class="text-xl font-semibold text-text-primary">Settings</h1>
		<p class="mt-1 text-sm text-text-secondary">Runtime configuration and controls</p>
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
				<SettingsRow label="Enabled" value={settings.crawler.enabled} />
				{#if settings.crawler.enabled}
					<div class="flex items-center justify-between px-4 py-2.5">
						<span class="text-sm text-text-secondary">Status</span>
						<div class="flex items-center gap-3">
							<span class="text-sm font-medium {settings.crawler.paused ? 'text-accent-amber' : 'text-accent-green'}">
								{settings.crawler.paused ? 'Paused' : 'Running'}
							</span>
							<button
								onclick={onCrawlerToggle}
								disabled={crawlerToggling}
								class="rounded-md px-3 py-1.5 text-xs font-medium transition-colors
									{settings.crawler.paused
										? 'bg-accent-green/15 text-accent-green hover:bg-accent-green/25'
										: 'bg-accent-amber/15 text-accent-amber hover:bg-accent-amber/25'}
									disabled:opacity-50"
							>
								{#if crawlerToggling}
									<Loader2 class="h-3 w-3 animate-spin" />
								{:else}
									{settings.crawler.paused ? 'Resume' : 'Pause'}
								{/if}
							</button>
						</div>
					</div>
				{/if}
				<SettingsRow label="Workers" value={settings.crawler.workers} />
				<SettingsRow label="DHT Port" value={settings.crawler.port} />
			</SettingsSection>

			<SettingsSection title="Matcher">
				<SettingsRow label="Enabled" value={settings.matcher.enabled} />
				{#if settings.matcher.enabled}
					<div class="flex items-center justify-between px-4 py-2.5">
						<span class="text-sm text-text-secondary">Status</span>
						<div class="flex items-center gap-3">
							<span class="text-sm font-medium {settings.matcher.paused ? 'text-accent-amber' : 'text-accent-green'}">
								{settings.matcher.paused ? 'Paused' : 'Running'}
							</span>
							<button
								onclick={onMatcherToggle}
								disabled={matcherToggling}
								class="rounded-md px-3 py-1.5 text-xs font-medium transition-colors
									{settings.matcher.paused
										? 'bg-accent-green/15 text-accent-green hover:bg-accent-green/25'
										: 'bg-accent-amber/15 text-accent-amber hover:bg-accent-amber/25'}
									disabled:opacity-50"
							>
								{#if matcherToggling}
									<Loader2 class="h-3 w-3 animate-spin" />
								{:else}
									{settings.matcher.paused ? 'Resume' : 'Pause'}
								{/if}
							</button>
						</div>
					</div>
				{/if}
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
