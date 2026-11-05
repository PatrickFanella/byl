import { defineConfig } from '@playwright/test';

// End-to-end tests need a running API with the test database; set E2E_API_URL to enable them.
const apiUrl = process.env.E2E_API_URL;
const executablePath = process.env.PLAYWRIGHT_CHROMIUM_PATH;

export default defineConfig({
	testDir: 'e2e',
	testMatch: '**/*.e2e.{ts,js}',
	timeout: 60_000,
	workers: 2,
	use: {
		baseURL: 'http://localhost:4173',
		...(executablePath ? { launchOptions: { executablePath } } : {})
	},
	webServer: {
		command: 'npm run build && npm run preview -- --port 4173',
		port: 4173,
		reuseExistingServer: true,
		env: { VITE_API_PROXY: apiUrl ?? 'http://localhost:8080' }
	}
});
