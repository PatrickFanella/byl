declare global {
	namespace App {
		// interface Error {}
		// interface Locals {}
		// interface PageData {}
		// interface PageState {}
		// interface Platform {}
	}
}

interface ImportMetaEnv {
	readonly VITE_API_BASE_URL?: string;
	readonly VITE_GOOGLE_CLIENT_ID?: string;
	readonly VITE_API_PROXY?: string;
}

export {};
