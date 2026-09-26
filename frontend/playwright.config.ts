import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  workers: 1,
  retries: 0,
  forbidOnly: true,
  timeout: 30_000,
  expect: { timeout: 5_000 },
  outputDir: "./test-results/results",
  reporter: [["list"], ["html", { outputFolder: "./test-results/report", open: "never" }]],
  use: {
    baseURL: "http://127.0.0.1:9245",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  webServer: {
    command: "sh e2e/start-server.sh",
    url: "http://127.0.0.1:9245",
    reuseExistingServer: false,
    timeout: 120_000,
    gracefulShutdown: { signal: "SIGTERM", timeout: 10_000 },
  },
});
