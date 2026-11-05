// The visitor's selected city, remembered per browser.
import { listCities, type City } from '#lib/visitor.ts';

const KEY = 'invisible-museum.city';

function remembered(): string | null {
	try {
		return typeof localStorage === 'undefined' ? null : localStorage.getItem(KEY);
	} catch {
		return null;
	}
}

class CityState {
	cities: City[] = $state([]);
	current: City | null = $state(null);
	status: 'idle' | 'loading' | 'ready' | 'error' = $state('idle');
	message = $state('');

	async load(): Promise<void> {
		if (this.status === 'loading' || this.status === 'ready') return;
		this.status = 'loading';
		try {
			const page = await listCities();
			this.cities = page.items;
			const slug = remembered();
			this.current = this.cities.find((c) => c.slug === slug) ?? this.cities[0] ?? null;
			this.status = 'ready';
		} catch (e) {
			this.message = e instanceof Error ? e.message : 'Could not load cities.';
			this.status = 'error';
		}
	}

	select(c: City): void {
		this.current = c;
		try {
			localStorage.setItem(KEY, c.slug);
		} catch {
			// private mode: nothing to remember into
		}
	}
}

export const city = new CityState();
