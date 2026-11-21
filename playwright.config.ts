import { defineConfig, devices } from "@playwright/test";

const PORT = 8181;

export default defineConfig({
  testDir: "e2e",
  // Tests share one server database, so run them one at a time.
  workers: 1,
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["html", { open: "never" }], ["list"]] : "list",
  use: {
    baseURL: `http://localhost:${PORT}`,
    ...devices["iPhone 13"],
    // "Today" depends on the server's TIMEZONE; pin the browser to the same zone.
    timezoneId: "Europe/Stockholm",
    trace: "retain-on-failure",
  },
  projects: [
    { name: "chromium", use: { browserName: "chromium" } },
    { name: "webkit", use: { browserName: "webkit" } },
  ],
  webServer: {
    command: "./scripts/e2e-server.sh",
    url: `http://localhost:${PORT}/`,
    env: { PORT: String(PORT) },
    reuseExistingServer: false,
    timeout: 120_000,
  },
});
