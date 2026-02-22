<script lang="ts">
	import { fetchSettings, toggleCrawler, toggleMatcher, type Settings } from '$lib/api';
	import SettingsSection from '$lib/components/SettingsSection.svelte';
	import SettingsRow from '$lib/components/SettingsRow.svelte';
	import SettingsEditRow from '$lib/components/SettingsEditRow.svelte';
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

	function onSettingUpdate(section: string, field: string) {
		return (newValue: string) => {
			if (!settings) return;
			if (section === 'tracker') {
				if (field === 'enabled') {
					settings = { ...settings, tracker: { ...settings.tracker, enabled: newValue === 'true' } };
				} else if (field === 'trackers') {
					settings = { ...settings, tracker: { ...settings.tracker, trackers: newValue.split('\n').filter(Boolean) } };
				} else if (field === 'timeout') {
					settings = { ...settings, tracker: { ...settings.tracker, timeout: newValue } };
				}
			} else if (section === 'matcher') {
				if (field === 'batch_size') {
					settings = { ...settings, matcher: { ...settings.matcher, batch_size: Number(newValue) } };
				} else if (field === 'interval') {
					settings = { ...settings, matcher: { ...settings.matcher, interval: newValue } };
				} else if (field === 'max_attempts') {
					settings = { ...settings, matcher: { ...settings.matcher, max_attempts: Number(newValue) } };
				}
			} else if (section === 'crawler') {
				if (field === 'rate') {
					settings = { ...settings, crawler: { ...settings.crawler, rate: Number(newValue) } };
				} else if (field === 'workers') {
					settings = { ...settings, crawler: { ...settings.crawler, workers: Number(newValue) } };
				}
			} else if (section === 'general') {
				if (field === 'log_level') {
					settings = { ...settings, general: { ...settings.general, log_level: newValue } };
				} else if (field === 'animedb_enabled') {
					settings = { ...settings, general: { ...settings.general, animedb_enabled: newValue === 'true' } };
				}
			}
		};
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
				<SettingsEditRow
					label="Workers"
					settingKey="crawl_workers"
					value={settings.crawler.workers}
					type="number"
					hint="(max 4, restart required)"
					onUpdate={onSettingUpdate('crawler', 'workers')}
				/>
				<SettingsRow label="DHT Port" value={settings.crawler.port} />
				<SettingsEditRow
					label="Rate"
					settingKey="crawl_rate"
					value={settings.crawler.rate}
					type="number"
					onUpdate={onSettingUpdate('crawler', 'rate')}
				/>
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
				<SettingsEditRow
					label="Batch Size"
					settingKey="match_batch_size"
					value={settings.matcher.batch_size}
					type="number"
					onUpdate={onSettingUpdate('matcher', 'batch_size')}
				/>
				<SettingsEditRow
					label="Interval"
					settingKey="match_interval"
					value={settings.matcher.interval}
					type="text"
					onUpdate={onSettingUpdate('matcher', 'interval')}
				/>
				<SettingsEditRow
					label="Max Attempts"
					settingKey="match_max_attempts"
					value={settings.matcher.max_attempts}
					type="number"
					onUpdate={onSettingUpdate('matcher', 'max_attempts')}
				/>
				<SettingsRow label="TMDB" value={settings.matcher.tmdb_enabled} />
				<SettingsRow label="TVDB" value={settings.matcher.tvdb_enabled} />
			</SettingsSection>

			<SettingsSection title="Tracker">
				<SettingsEditRow
					label="Enabled"
					settingKey="tracker_enabled"
					value={settings.tracker.enabled}
					type="boolean"
					onUpdate={onSettingUpdate('tracker', 'enabled')}
				/>
				<SettingsEditRow
					label="Trackers"
					settingKey="tracker_list"
					value={settings.tracker.trackers.join('\n')}
					type="textarea"
					onUpdate={onSettingUpdate('tracker', 'trackers')}
				/>
				<SettingsEditRow
					label="Timeout"
					settingKey="tracker_timeout"
					value={settings.tracker.timeout}
					type="text"
					onUpdate={onSettingUpdate('tracker', 'timeout')}
				/>
			</SettingsSection>

			<SettingsSection title="General">
				<SettingsEditRow
					label="Log Level"
					settingKey="log_level"
					value={settings.general.log_level}
					type="text"
					onUpdate={onSettingUpdate('general', 'log_level')}
				/>
				<SettingsEditRow
					label="Anime DB"
					settingKey="animedb_enabled"
					value={settings.general.animedb_enabled}
					type="boolean"
					onUpdate={onSettingUpdate('general', 'animedb_enabled')}
				/>
			</SettingsSection>

			<SettingsSection title="API">
				<SettingsRow label="Port" value={settings.api.port} />
				<SettingsRow label="Authentication" value={settings.api.auth_enabled} />
			</SettingsSection>
		</div>
	{/if}
</div>
