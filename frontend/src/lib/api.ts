const BASE = '';

export interface MetricsSnapshot {
	dht_nodes_visited: number;
	dht_info_hashes_recv: number;
	torrents_discovered: number;
	metadata_fetched: number;
	metadata_failed: number;
	match_attempts: number;
	match_successes: number;
	match_failures: number;
	torrents_saved: number;
	discovery_rate: number;
	metadata_rate: number;
	match_rate: number;
	uptime_seconds: number;
}

export interface Stats {
	total_torrents: number;
	unmatched: number;
	matched: number;
	failed: number;
	db_size: number;
	last_crawl: number;
	uptime: number;
	start_time: string;
	matcher_paused: boolean;
	metrics?: MetricsSnapshot;
}

export interface TorrentResult {
	info_hash: string;
	name: string;
	size: number;
	category: string;
	quality: string;
	imdb_id?: string;
	tmdb_id?: number;
	seeders: number;
	leechers: number;
	discovered_at: number;
}

export interface SearchResponse {
	results: TorrentResult[];
	total: number;
	page: number;
	limit: number;
}

export interface SearchParams {
	q: string;
	page?: number;
	limit?: number;
	category?: number;
	quality?: number;
}

export interface LatestParams {
	page?: number;
	limit?: number;
	category?: number;
	quality?: number;
}

export interface Settings {
	database: {
		backend: string;
		path?: string;
	};
	crawler: {
		enabled: boolean;
		paused: boolean;
		workers: number;
		port: number;
		rate: number;
	};
	matcher: {
		enabled: boolean;
		paused: boolean;
		batch_size: number;
		interval: string;
		max_attempts: number;
		tmdb_enabled: boolean;
		tvdb_enabled: boolean;
	};
	tracker: {
		enabled: boolean;
		trackers: string[];
		timeout: string;
	};
	general: {
		log_level: string;
		animedb_enabled: boolean;
	};
	api: {
		port: number;
		auth_enabled: boolean;
	};
}

export interface MatcherEntry {
	info_hash: string;
	name: string;
	size: number;
	category: string;
	quality: string;
	imdb_id?: string;
	tmdb_id?: number;
	tvdb_id?: number;
	anilist_id?: number;
	kitsu_id?: number;
	media_year?: number;
	match_status: string;
	match_attempts: number;
	match_after: number;
	seeders: number;
	leechers: number;
	updated_at: number;
}

export interface MatcherListResponse {
	results: MatcherEntry[];
	total: number;
	page: number;
	limit: number;
}

export interface ToggleResponse {
	component: string;
	paused: boolean;
}

async function fetchJSON<T>(url: string): Promise<T> {
	const resp = await fetch(`${BASE}${url}`);
	if (!resp.ok) {
		throw new Error(`HTTP ${resp.status}: ${resp.statusText}`);
	}
	return resp.json();
}

async function postJSON<T>(url: string, body: unknown): Promise<T> {
	const resp = await fetch(`${BASE}${url}`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(body)
	});
	if (!resp.ok) {
		throw new Error(`HTTP ${resp.status}: ${resp.statusText}`);
	}
	return resp.json();
}

export async function fetchStats(): Promise<Stats> {
	return fetchJSON('/api/stats');
}

export async function search(params: SearchParams): Promise<SearchResponse> {
	const query = new URLSearchParams();
	query.set('q', params.q);
	if (params.page) query.set('page', String(params.page));
	if (params.limit) query.set('limit', String(params.limit));
	if (params.category !== undefined) query.set('category', String(params.category));
	if (params.quality !== undefined) query.set('quality', String(params.quality));
	return fetchJSON(`/api/search?${query}`);
}

export async function fetchLatest(params: LatestParams): Promise<SearchResponse> {
	const query = new URLSearchParams();
	if (params.page) query.set('page', String(params.page));
	if (params.limit) query.set('limit', String(params.limit));
	if (params.category !== undefined) query.set('category', String(params.category));
	if (params.quality !== undefined) query.set('quality', String(params.quality));
	return fetchJSON(`/api/torrents/latest?${query}`);
}

async function putJSON<T>(url: string, body: unknown): Promise<T> {
	const resp = await fetch(`${BASE}${url}`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(body)
	});
	if (!resp.ok) {
		throw new Error(`HTTP ${resp.status}: ${resp.statusText}`);
	}
	return resp.json();
}

export async function fetchSettings(): Promise<Settings> {
	return fetchJSON('/api/settings');
}

export async function putSetting(key: string, value: string): Promise<{ status: string; key: string; requires_restart: boolean }> {
	return putJSON('/api/settings', { key, value });
}

export async function toggleCrawler(paused: boolean): Promise<ToggleResponse> {
	return postJSON('/api/crawler/toggle', { paused });
}

export async function toggleMatcher(paused: boolean): Promise<ToggleResponse> {
	return postJSON('/api/matcher/toggle', { paused });
}

export async function fetchMatcherRecent(page = 1, limit = 20): Promise<MatcherListResponse> {
	return fetchJSON(`/api/matcher/recent?page=${page}&limit=${limit}`);
}

export async function fetchMatcherFailures(page = 1, limit = 20): Promise<MatcherListResponse> {
	return fetchJSON(`/api/matcher/failures?page=${page}&limit=${limit}`);
}

export async function triggerMatcher(): Promise<{ triggered: boolean; batch_size: number }> {
	return postJSON('/api/matcher/trigger', {});
}

export async function rematch(): Promise<{ reset: number }> {
	return postJSON('/api/matcher/rematch', {});
}

export function connectSSE(onMessage: (snap: MetricsSnapshot) => void): EventSource {
	const es = new EventSource(`${BASE}/api/events`);
	es.onmessage = (e) => {
		try {
			onMessage(JSON.parse(e.data));
		} catch {
			// ignore parse errors
		}
	};
	return es;
}
