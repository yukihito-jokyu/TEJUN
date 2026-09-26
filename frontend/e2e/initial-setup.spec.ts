import { expect, type Page, test } from "@playwright/test";

const methods = {
  authenticate: 1005382116,
  check: 1491784629,
  complete: 2972149189,
  elicitation: 3945556279,
  logout: 870717045,
  startup: 2958778619,
  candidates: 748173126,
  projects: 2753046913,
} as const;

type FailureMode = "incompatible" | "timeout" | "exit" | "cancelled";
type Mode = "noauth" | "agent" | "terminal" | FailureMode;

const connection = {
  displayName: "Fake ACP",
  command: "/tmp/fake-acp",
  args: [],
  transport: "stdio",
  environmentOverrides: [],
};

const failures: Record<FailureMode, { code: string; message: string; title: string }> = {
  incompatible: {
    code: "acp_not_compatible",
    message: "ACP protocol versionに互換性がありません",
    title: "このAgentは互換性がありません",
  },
  timeout: {
    code: "acp_timeout",
    message: "Agentから時間内に応答がありませんでした",
    title: "処理を完了できませんでした",
  },
  exit: {
    code: "agent_process_exited",
    message: "Agent processが異常終了しました",
    title: "処理を完了できませんでした",
  },
  cancelled: {
    code: "operation_cancelled",
    message: "Agentへの接続を取り消しました",
    title: "処理を完了できませんでした",
  },
};

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

async function mockWails(page: Page, mode: Mode) {
  let configured = false;
  let authenticated = false;
  let checks = 0;
  let latestProbeId = "";

  await page.route("**/wails/runtime", async (route) => {
    const request = route.request().postDataJSON() as {
      args?: { methodID?: number; args?: unknown[] };
    };
    const call = request.args;
    const method = call?.methodID;
    const args = call?.args;
    const input = args?.[0] as Record<string, unknown> | undefined;

    if (method === methods.startup) {
      expect(args).toEqual([]);
      return route.fulfill({
        json: {
          initialSetupRequired: !configured,
          nextRoute: configured ? "/projects" : "/setup",
          defaultConnection: null,
        },
      });
    }
    if (method === methods.projects) {
      return route.fulfill({
        json: {
          items: [],
          total: 0,
          nextCursor: null,
          generatedAt: "2026-09-26T00:00:00Z",
          changeSequence: 1,
        },
      });
    }
    if (method === methods.candidates) {
      expect(args).toEqual([{ refresh: false }]);
      return route.fulfill({
        json: {
          items: [
            {
              candidateKey: "fake-acp",
              displayName: "Fake ACP",
              command: "/tmp/fake-acp",
              args: [],
              transport: "stdio",
              source: "path",
              warnings: [],
            },
          ],
          scannedAt: "2026-09-26T00:00:00Z",
          warnings: [],
        },
      });
    }
    if (method === methods.check) {
      expect(args).toHaveLength(1);
      expect(input).toMatchObject({ connection });
      expect(input?.operationId).toMatch(uuid);
      checks += 1;
      if (mode in failures && checks === 1) {
        const failure = failures[mode as FailureMode];
        return route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({
            kind: "RuntimeError",
            message: "check failed",
            cause: {
              code: failure.code,
              message: failure.message,
              retryable: true,
            },
          }),
        });
      }
      const requiresAuth = mode === "agent" || mode === "terminal";
      const type = mode === "terminal" ? "terminal" : "agent";
      latestProbeId = `probe-${checks}`;
      return route.fulfill({
        json: {
          probeId: latestProbeId,
          configFingerprint: "fingerprint",
          compatibility: "compatible",
          authState: requiresAuth ? (authenticated ? "authenticated" : "unknown") : "not_required",
          expiresAt: "2026-09-26T00:01:00Z",
          authMethods: requiresAuth
            ? [
                {
                  type,
                  authMethodId: `${type}-auth`,
                  name: type === "terminal" ? "Terminal login" : "Agent login",
                  environmentNames: [],
                },
              ]
            : [],
          authObservation: {
            status: requiresAuth ? (authenticated ? "authenticated" : "unknown") : "not_required",
            source: "initialize_only",
            observedAt: "2026-09-26T00:00:00Z",
            processGeneration: checks,
          },
        },
      });
    }
    if (method === methods.authenticate) {
      expect(args).toHaveLength(1);
      expect(input).toMatchObject({
        probeId: latestProbeId,
        connectionId: "",
        authMethodId: `${mode}-auth`,
      });
      expect(input?.operationId).toMatch(uuid);
      authenticated = true;
      return route.fulfill({ json: { data: { jobId: "job-1" }, receipt: { operationId: "op" } } });
    }
    if (method === methods.complete) {
      expect(args).toHaveLength(1);
      expect(input).toMatchObject({ connection, probeId: latestProbeId });
      expect(input?.operationId).toMatch(uuid);
      configured = true;
      return route.fulfill({
        json: {
          data: { connectionId: "connection-1", nextRoute: "/projects" },
          receipt: { operationId: "op" },
        },
      });
    }
    if (method === methods.logout) {
      expect(args).toHaveLength(1);
      expect(input).toMatchObject({ connectionId: "connection-1" });
      expect(input?.operationId).toMatch(uuid);
      return route.fulfill({ json: { data: {}, receipt: { operationId: "op" } } });
    }
    if (method === methods.elicitation) {
      expect(args).toHaveLength(1);
      expect(input).toMatchObject({
        elicitationRequestId: "elicitation-1",
        action: "cancel",
        content: "",
      });
      expect(input?.operationId).toMatch(uuid);
      return route.fulfill({ json: { data: {}, receipt: { operationId: "op" } } });
    }
    return route.fulfill({ status: 404, body: "unexpected Wails call" });
  });
}

async function selectAndProbe(page: Page) {
  await page.goto("/");
  await page.getByRole("button", { name: "接続" }).click();
}

test("認証不要Agentを保存し、再起動時はプロジェクト一覧から始める", async ({ page }) => {
  await mockWails(page, "noauth");
  await selectAndProbe(page);
  await expect(page.getByText("エージェントを接続しました")).toBeVisible();
  await page.getByRole("button", { name: /プロジェクト一覧へ/ }).click();
  await expect(page).toHaveURL(/#\/projects$/);
  await page.reload();
  await expect(page.getByRole("heading", { name: "すべてのプロジェクト" })).toBeVisible();
  await page.evaluate(async (source) => {
    const api = (await import(source)) as {
      logoutAgent: (connectionId: string, operationId: string) => Promise<unknown>;
      respondToElicitation: (
        requestId: string,
        action: string,
        content: string,
        operationId: string,
      ) => Promise<unknown>;
    };
    await api.logoutAgent("connection-1", crypto.randomUUID());
    await api.respondToElicitation("elicitation-1", "cancel", "", crypto.randomUUID());
  }, "/src/shared/api/wails/index.ts");
});

for (const mode of ["agent", "terminal"] as const) {
  test(`${mode}認証後に保存できる`, async ({ page }) => {
    await mockWails(page, mode);
    await selectAndProbe(page);
    await expect(page.getByRole("heading", { name: "認証" })).toBeVisible();
    await page
      .getByRole("button", { name: mode === "terminal" ? "ターミナルで認証" : "認証する" })
      .click();
    await expect(page.getByText("認証の完了を待っています。")).toBeVisible();
    await page.evaluate(() => {
      const runtime = window as Window & {
        _wails?: { dispatchWailsEvent?: (event: { name: string; data: unknown }) => void };
      };
      for (const [aggregateType, aggregateId, authState] of [
        ["agent_job", "job-1", "succeeded"],
        ["agent_connection", "probe-1", "pending"],
      ]) {
        runtime._wails?.dispatchWailsEvent?.({
          name: "app:event",
          data: {
            eventId: crypto.randomUUID(),
            name: "agent.authentication.updated",
            emittedAt: new Date().toISOString(),
            aggregateType,
            aggregateId,
            changeSequence: 1,
            streamKey: `${aggregateType}:${aggregateId}`,
            streamRevision: 1,
            correlation: { jobId: "job-1" },
            payload: { connectionId: "probe-1", authState, jobId: "job-1" },
          },
        });
      }
    });
    await expect(page.getByText("エージェントを接続しました")).toBeVisible();
    await page.getByRole("button", { name: /プロジェクト一覧へ/ }).click();
    await expect(page).toHaveURL(/#\/projects$/);
  });
}

for (const mode of ["incompatible", "timeout", "exit", "cancelled"] as const) {
  test(`${mode}エラーから入力を保持して再試行できる`, async ({ page }) => {
    await mockWails(page, mode);
    await selectAndProbe(page);
    await expect(page.getByRole("alert")).toContainText(failures[mode].title);
    await expect(page.getByRole("alert")).toContainText(failures[mode].message);
    await page.getByRole("button", { name: "再試行" }).click();
    await page.getByRole("button", { name: /プロジェクト一覧へ/ }).click();
    await expect(page).toHaveURL(/#\/projects$/);
    await expect(page.getByText(failures[mode].message)).toHaveCount(0);
  });
}
