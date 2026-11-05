// The visitor's selected city, remembered per browser.
import { getCity, listCities, type City } from '#lib/visitor.ts';

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
			const selected = this.cities.find((c) => c.slug === slug) ?? this.cities[0] ?? null;
			this.current = selected ? await getCity(selected.slug) : null;
			this.status = 'ready';
		} catch (e) {
			this.message = e instanceof Error ? e.message : 'Could not load cities.';
			this.status = 'error';
		}
	}

	select(c: City): void {
		this.current = c;
		void getCity(c.slug)
			.then((detail) => {
				if (this.current?.id === c.id) this.current = detail;
			})
			.catch(() => {
				this.message = 'Could not load this city’s topics.';
			});
		try {
			localStorage.setItem(KEY, c.slug);
		} catch {
			// private mode: nothing to remember into
		}
	}
}

export const city = new CityState();
