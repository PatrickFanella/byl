import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vitest/config';
import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';

// The admin app is a single-page app served as static files; the API lives at /api on the
// same origin in production (nginx) and is proxied here in development and preview.
const apiTarget = process.env.VITE_API_PROXY ?? 'http://localhost:8080';
const proxy = {
	'/api': { target: apiTarget, changeOrigin: true },
	'/media': { target: apiTarget, changeOrigin: true },
	'/healthz': { target: apiTarget, changeOrigin: true }
};

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit({
			compilerOptions: {
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},
			adapter: adapter({ fallback: 'index.html', strict: false })
		})
	],
	server: { proxy },
	preview: { proxy },
	test: {
		expect: { requireAssertions: true },
		projects: [
			{
				extends: './vite.config.ts',
				test: {
					name: 'server',
					environment: 'node',
					include: ['src/**/*.{test,spec}.{js,ts}'],
					exclude: ['src/**/*.svelte.{test,spec}.{js,ts}']
				}
			}
		]
	}
});
