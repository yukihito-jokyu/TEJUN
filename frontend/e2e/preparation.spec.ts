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
  activity: undefined as
    | undefined
    | {
        turnId: string;
        phase: string;
        items: {
          messageId: string;
          turnId: string;
          role: string;
          status: string;
          content: { text: string }[];
        }[];
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
  expect(calls.filter((call) => call.method !== 3565193614).map((call) => call.method)).toEqual([
    method.brief,
    method.plan,
    method.start,
  ]);
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
  await page.getByRole("combobox", { name: "過去のチャット" }).click();
  await page.getByRole("option", { name: /現在の相談/ }).click();
  await expect(page.getByText("以前の相談")).toBeVisible();
  await expect(page.getByLabel("AIに相談する")).toHaveCount(0);
  expect(calls.find((call) => call.method === method.workspace)?.input).toMatchObject({
    newWorkspacePath: "/tmp/work",
  });
});

test("ヘッダーを表示したまま会話と準備を別々にスクロールできる", async ({ page }) => {
  const state = snapshot();
  state.conversation.items = Array.from({ length: 30 }, (_, index) => ({
    messageId: `message-${index}`,
    turnId: `turn-${index}`,
    role: "user",
    status: "completed",
    content: [{ text: index === 0 ? "長い文章".repeat(100) : `相談 ${index}` }],
  }));
  await mockPreparation(page, state);
  await page.setViewportSize({ width: 1280, height: 720 });
  await page.goto("/#/projects/p/prepare");

  const messages = page.locator(".preparation-messages");
  const details = page.locator(".preparation-details");
  const header = page.locator(".preparation-header");
  expect(await messages.evaluate((element) => element.scrollHeight > element.clientHeight)).toBe(
    true,
  );
  expect(await messages.evaluate((element) => element.scrollWidth)).toBeLessThanOrEqual(
    await messages.evaluate((element) => element.clientWidth),
  );
  expect(await details.evaluate((element) => element.scrollHeight > element.clientHeight)).toBe(
    true,
  );
  await messages.evaluate((element) => (element.scrollTop = 200));
  expect(await messages.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
  expect(await details.evaluate((element) => element.scrollTop)).toBe(0);
  await details.evaluate((element) => (element.scrollTop = 200));
  expect(await details.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
  expect(await messages.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
  expect(
    await page.evaluate(() => {
      window.scrollTo(0, 1000);
      return window.scrollY;
    }),
  ).toBe(0);
  await expect(header).toBeInViewport();
});

test("AIの返答とツール作業を処理中に表示する", async ({ page }) => {
  const state = snapshot();
  await mockPreparation(page, state);
  await page.goto("/#/projects/p/prepare");
  await page.getByLabel("AIに相談する").fill("準備内容を調べて");
  await page.getByRole("button", { name: "送信" }).click();
  await expect(page.getByText("応答を準備しています…")).toBeVisible();

  state.activity = {
    turnId: "turn",
    phase: "tool",
    items: [
      {
        messageId: "tool-log",
        turnId: "turn",
        role: "system",
        status: "streaming",
        content: [{ text: "READMEを読む" }],
      },
    ],
  };
  await expect(page.getByText("ツールを実行中")).toBeVisible();
  await expect(page.getByText("実行中：READMEを読む")).toBeVisible();
  state.activity = {
    ...state.activity,
    phase: "responding",
    items: [
      {
        messageId: "tool-log",
        turnId: "turn",
        role: "system",
        status: "completed",
        content: [{ text: "READMEを読む" }],
      },
      {
        messageId: "progress",
        turnId: "turn",
        role: "agent",
        status: "completed",
        content: [{ text: "READMEを確認しています。" }],
      },
      {
        messageId: "answer",
        turnId: "turn",
        role: "agent",
        status: "streaming",
        content: [{ text: "手順を整理しています。" }],
      },
    ],
  };
  await expect(page.getByText("AIが回答中")).toBeVisible();
  await expect(page.getByText("完了：READMEを読む")).toBeVisible();
  await expect(page.getByText("READMEを確認しています。")).toBeVisible();
  await expect(page.getByText("手順を整理しています。")).toBeVisible();
  expect(
    await page
      .locator(".preparation-message")
      .filter({ hasText: "READMEを確認しています。" })
      .count(),
  ).toBe(1);
  expect(
    await page
      .locator(".preparation-message")
      .filter({ hasText: "手順を整理しています。" })
      .count(),
  ).toBe(1);

  state.activity = undefined;
  state.session.state = "ready";
  state.conversation.items[0].status = "completed";
  state.conversation.items.push(
    {
      messageId: "tool-log",
      turnId: "turn",
      role: "system",
      status: "completed",
      content: [{ text: "READMEを読む" }],
    },
    {
      messageId: "progress",
      turnId: "turn",
      role: "agent",
      status: "completed",
      content: [{ text: "READMEを確認しています。" }],
    },
    {
      messageId: "answer",
      turnId: "turn",
      role: "agent",
      status: "completed",
      content: [{ text: "手順を整理しました。" }],
    },
  );
  await expect(page.getByText("AIが回答中")).toHaveCount(0);
  await expect(page.getByText("完了：READMEを読む")).toBeVisible();
  await expect(page.getByText("READMEを確認しています。")).toBeVisible();
  await expect(page.getByText("手順を整理しました。")).toBeVisible();
  expect(
    await page.locator(".preparation-message").filter({ hasText: "完了：READMEを読む" }).count(),
  ).toBe(1);
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
