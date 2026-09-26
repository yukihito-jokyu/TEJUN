import { expect, test, type Page } from "@playwright/test";

const method = {
  startup: 2958778619,
  get: 18778174,
  brief: 3827854321,
  plan: 2230699497,
  workspace: 489623341,
  policy: 3814117507,
  message: 3930952125,
  start: 2744344285,
  cancel: 3271773890,
  elicitation: 3945556279,
  configuration: 1100524509,
} as const;

const snapshot = () => ({
  project: {
    projectId: "p",
    name: "準備プロジェクト",
    description: "確認用",
    workspacePath: "/tmp/work",
    revision: 1,
    currentStage: "preparation",
  },
  preparation: {
    purpose: "元の目的",
    completionCriteria: ["完了"],
    intendedUsers: "利用者",
    revision: 1,
  },
  checkPlan: {
    planId: "plan",
    revision: 1,
    items: [
      {
        checkId: "check-1",
        sequence: 1,
        title: "確認",
        instruction: "実行する",
        expectedResult: "成功",
        suggestedCommand: "",
        aiRequired: false,
        humanRequired: true,
        humanEvidenceRequirement: "none",
      },
    ],
  },
  session: {
    sessionId: "session",
    state: "ready",
    revision: 1,
    permissionPolicy: { sessionId: "session", mode: "ask_every_time", revision: 1 },
    modes: {
      currentModeId: "safe",
      available: [
        { modeId: "safe", name: "安全" },
        { modeId: "fast", name: "高速" },
      ],
    },
    configOptions: [
      {
        configId: "speed",
        type: "select",
        name: "速度",
        currentValue: "safe",
        options: {
          layout: "flat",
          items: [
            { value: "safe", name: "安全" },
            { value: "fast", name: "高速" },
          ],
        },
      },
    ],
  },
  conversation: {
    items: [] as Array<{
      messageId: string;
      turnId: string;
      role: string;
      status: string;
      content: { text: string }[];
    }>,
    hasPrevious: false,
  },
  chats: [{ sessionId: "session", title: "現在の相談", startedAt: "2026-09-26T12:00:00Z" }],
  elicitations: [] as Array<{
    elicitationRequestId: string;
    mode: string;
    message: string;
    status: string;
    requestedSchema?: Record<string, unknown>;
  }>,
  readiness: { canStartExecution: true, blockingReasons: [] },
  changeSequence: 1,
});

async function mockPreparation(page: Page, state: ReturnType<typeof snapshot>, failBrief = false) {
  const calls: { method: number; input: Record<string, unknown> }[] = [];
  let pastMessages: ReturnType<typeof snapshot>["conversation"]["items"] = [];
  await page.route("**/wails/runtime", async (route) => {
    const body = route.request().postDataJSON() as {
      args?: { methodID?: number; args?: unknown[] };
    };
    const id = body.args?.methodID ?? 0;
    const input = (body.args?.args?.[0] ?? {}) as Record<string, unknown>;
    if (id === method.startup) {
      return route.fulfill({
        json: { initialSetupRequired: false, nextRoute: "/projects", changeSequence: 1 },
      });
    }
    if (id === method.get) {
      expect(input).toMatchObject({ projectId: "p", conversationLimit: 50 });
      if (input.chatId === "session" && state.session.sessionId !== "session") {
        return route.fulfill({
          json: { ...state, conversation: { items: pastMessages, hasPrevious: false } },
        });
      }
      return route.fulfill({ json: state });
    }
    calls.push({ method: id, input });
    const receipt = { operationId: input.operationId, committedAt: "2026-09-26T00:00:00Z" };
    if (id === method.brief) {
      if (failBrief) {
        failBrief = false;
        return route.fulfill({
          status: 500,
          json: {
            kind: "RuntimeError",
            message: "conflict",
            cause: {
              code: "revision_conflict",
              message: "別の変更が保存されました",
              currentRevision: 2,
              retryable: false,
            },
          },
        });
      }
      state.preparation.purpose = (input.brief as { purpose: string }).purpose;
      state.preparation.revision++;
    }
    if (id === method.plan) state.checkPlan.revision++;
    if (id === method.workspace) {
      if (input.newWorkspacePath === state.project.workspacePath) {
        pastMessages = [...state.conversation.items];
        state.conversation.items = [];
        state.session.sessionId = "new-session";
        state.chats.unshift({
          sessionId: "new-session",
          title: "新しいチャット",
          startedAt: "2026-09-26T13:00:00Z",
        });
      }
      state.project.workspacePath = input.newWorkspacePath as string;
      state.project.revision++;
      state.session.state = "reconnecting";
    }
    if (id === method.message) {
      state.session.state = "busy";
      state.conversation.items.push({
        messageId: "message",
        turnId: "turn",
        role: "user",
        status: "streaming",
        content: [{ text: "相談" }],
      });
    }
    if (id === method.cancel) {
      state.session.state = "ready";
      state.conversation.items[0].status = "cancelled";
    }
    if (id === method.elicitation) state.elicitations[0].status = "responded";
    if (id === method.configuration) state.session.revision++;
    if (id === method.policy) state.session.permissionPolicy.mode = input.mode as "ask_every_time";
    const data = id === method.start ? { nextRoute: "#/projects/p/check" } : {};
    return route.fulfill({ json: { data, receipt } });
  });
  return calls;
}

test("briefとplanを保存し、開始routeへ進む", async ({ page }) => {
  const state = snapshot();
  const calls = await mockPreparation(page, state);
  await page.goto("/#/projects/p/prepare");
  await page.getByRole("button", { name: "修正" }).click();
  await page.getByLabel("手順書の目的").fill("新しい目的");
  await page.getByRole("button", { name: "目的を保存" }).click();
  await expect(page.getByRole("definition").filter({ hasText: "新しい目的" })).toBeVisible();
  await page.getByRole("button", { name: "項目を編集" }).click();
  await page.getByLabel("確認内容").fill("新しい確認");
  await page.getByRole("button", { name: "チェック案を保存" }).click();
  await page.getByRole("button", { name: "動作チェックを開始" }).click();
  await expect(page).toHaveURL(/#\/projects\/p\/check$/);
  expect(calls.map((call) => call.method)).toEqual([method.brief, method.plan, method.start]);
  expect(calls[1].input).toMatchObject({ expectedPreparationRevision: 2, expectedPlanRevision: 1 });
  expect(calls[2].input).toMatchObject({ expectedPreparationRevision: 2, expectedPlanRevision: 2 });
});

test("競合時は入力を保持し、workspace変更は確認する", async ({ page }) => {
  const state = snapshot();
  const calls = await mockPreparation(page, state, true);
  await page.goto("/#/projects/p/prepare");
  await page.getByRole("button", { name: "修正" }).click();
  await page.getByLabel("手順書の目的").fill("編集中の目的");
  await page.getByRole("button", { name: "目的を保存" }).click();
  await expect(page.getByRole("alert")).toContainText("別の変更が保存されました");
  await expect(page.getByLabel("手順書の目的")).toHaveValue("編集中の目的");
  await page.getByRole("button", { name: "変更" }).click();
  await page.getByLabel("workspace path").fill("/tmp/new-work");
  await page.getByRole("button", { name: "変更する" }).click();
  await expect(page.getByRole("group", { name: "作業ディレクトリ変更の確認" })).toBeVisible();
  expect(calls.some((call) => call.method === method.workspace)).toBe(false);
  await page.getByRole("button", { name: "変更を確定" }).click();
  await expect(page.getByText("/tmp/new-work")).toBeVisible();
  expect(calls.find((call) => call.method === method.workspace)?.input).toMatchObject({
    confirmSessionReset: true,
  });
});

test("新規チャットを作り、過去のチャットを閲覧する", async ({ page }) => {
  const state = snapshot();
  state.conversation.items.push({
    messageId: "past",
    turnId: "past-turn",
    role: "user",
    status: "completed",
    content: [{ text: "以前の相談" }],
  });
  const calls = await mockPreparation(page, state);
  await page.goto("/#/projects/p/prepare");
  await expect(page.getByText("以前の相談")).toBeVisible();
  await page.getByRole("button", { name: "新規チャット" }).click();
  await expect(page.getByText("どのような手順書を作りますか？")).toBeVisible();
  await page.getByLabel("過去のチャット").selectOption("session");
  await expect(page.getByText("以前の相談")).toBeVisible();
  await expect(page.getByLabel("AIに相談する")).toHaveCount(0);
  expect(calls.find((call) => call.method === method.workspace)?.input).toMatchObject({
    newWorkspacePath: "/tmp/work",
  });
});

test("turn取消とelicitation、session設定を操作する", async ({ page }) => {
  const state = snapshot();
  state.elicitations.push({
    elicitationRequestId: "request",
    mode: "form",
    message: "確認してください",
    status: "pending",
    requestedSchema: { type: "object" },
  });
  const calls = await mockPreparation(page, state);
  await page.goto("/#/projects/p/prepare");
  await page.getByLabel("AIに相談する").fill("相談");
  await page.getByRole("button", { name: "送信" }).click();
  await expect(page.getByRole("button", { name: "処理を取り消す" })).toBeEnabled();
  await page.getByRole("button", { name: "処理を取り消す" }).click();
  await expect(page.getByRole("button", { name: "処理を取り消す" })).toHaveCount(0);
  await page.getByRole("button", { name: "応答する" }).click();
  await expect(page.getByText("確認してください")).toHaveCount(0);
  await page.getByRole("button", { name: "変更" }).click();
  await page.getByLabel("現在のセッションで使うモード").selectOption("fast");
  await page.getByLabel("速度").selectOption("fast");
  await page
    .getByRole("region", { name: "Agentの設定" })
    .getByRole("button", { name: "設定を保存" })
    .click();
  expect(calls.find((call) => call.method === method.cancel)?.input).toMatchObject({
    sessionId: "session",
    turnId: "turn",
  });
  expect(calls.find((call) => call.method === method.elicitation)?.input).toMatchObject({
    elicitationRequestId: "request",
    action: "accept",
  });
  expect(calls.filter((call) => call.method === method.configuration)).toHaveLength(2);
});
