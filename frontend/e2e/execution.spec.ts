import { expect, test, type Page } from "@playwright/test";

const method = {
  startup: 2958778619,
  candidates: 748173126,
  list: 2753046913,
  preparation: 18778174,
  start: 2744344285,
  get: 3565193614,
  run: 138840161,
  message: 2090120471,
  evidence: 2224839479,
  human: 2726358031,
  permission: 3397895456,
  generate: 4023294307,
  procedure: 2087193246,
} as const;

const side = (checked = false) => ({
  required: true,
  status: checked ? "completed" : "pending",
  checked,
  checkedAt: null,
  failureSummary: "",
});
const snapshot = () => ({
  project: {
    projectId: "p",
    name: "確認プロジェクト",
    workspacePath: "/tmp/work",
    revision: 1,
    currentStage: "execution",
  },
  execution: {
    executionId: "execution",
    status: "ready",
    revision: 1,
    startedAt: "2026-09-27T00:00:00Z",
    completedAt: null,
  },
  session: {
    sessionId: "session",
    state: "ready",
    revision: 1,
    permissionPolicy: { sessionId: "session", mode: "ask_every_time", revision: 1 },
    modes: null,
    configOptions: null,
  },
  activeRun: null as null | {
    runId: string;
    status: string;
    targetedCheckIds: string[];
    startedAt: string;
    finishedAt: null;
  },
  checks: [
    {
      checkId: "check",
      sequence: 1,
      title: "画面を確認",
      instruction: "画面を開く",
      expectedResult: "表示される",
      suggestedCommand: "task dev",
      ai: side(),
      human: side(),
      humanEvidenceRequirement: "text_or_image",
      overallStatus: "pending",
      evidence: {
        ai: [] as Array<Record<string, unknown>>,
        human: [] as Array<Record<string, unknown>>,
      },
    },
  ],
  pendingPermissions: [] as Array<Record<string, unknown>>,
  conversation: {
    items: [] as Array<Record<string, unknown>>,
    previousCursor: null,
    hasPrevious: false,
  },
  readiness: {
    canGenerateProcedure: false,
    blockingReasons: [
      { code: "incomplete", message: "チェックを完了してください", checkId: "check" },
    ],
  },
  changeSequence: 1,
});

type State = ReturnType<typeof snapshot>;
async function mockExecution(page: Page, state: State, initialRoute = "#/projects/p/check") {
  const calls: Array<{ id: number; input: Record<string, unknown> }> = [];
  let failImage = true;
  let startupRoute = initialRoute;
  await page.route("**/wails/runtime", async (route) => {
    const body = route.request().postDataJSON() as {
      args?: { methodID?: number; args?: unknown[] };
    };
    const id = body.args?.methodID ?? 0;
    const input = (body.args?.args?.[0] ?? {}) as Record<string, unknown>;
    if (id === method.startup)
      return route.fulfill({
        json: {
          initialSetupRequired: false,
          nextRoute: startupRoute,
          changeSequence: 1,
        },
      });
    if (id === method.candidates) return route.fulfill({ json: { items: [] } });
    if (id === method.list)
      return route.fulfill({
        json: {
          items: [
            {
              projectId: "p",
              name: state.project.name,
              description: "確認用",
              workspacePath: state.project.workspacePath,
              status: "preparing",
              currentStage: "preparation",
              progress: { completed: 0, total: 1 },
              attentionRank: 0,
              attentionReason: "",
              updatedAt: "2026-09-27T00:00:00Z",
              completedAt: null,
              resumeRoute: "#/projects/p/prepare",
              currentProcedureId: null,
              connectionState: "connected",
              errorSummary: null,
              revision: 1,
            },
          ],
          total: 1,
          nextCursor: null,
          changeSequence: state.changeSequence,
        },
      });
    if (id === method.preparation)
      return route.fulfill({
        json: {
          project: { ...state.project, currentStage: "preparation", description: "" },
          preparation: {
            purpose: "確認",
            completionCriteria: ["完了"],
            intendedUsers: "利用者",
            revision: 1,
          },
          checkPlan: {
            planId: "plan",
            revision: 1,
            items: [
              {
                checkId: "check",
                sequence: 1,
                title: "画面を確認",
                instruction: "画面を開く",
                expectedResult: "表示される",
                suggestedCommand: "",
                aiRequired: true,
                humanRequired: true,
                humanEvidenceRequirement: "text_or_image",
              },
            ],
          },
          session: state.session,
          conversation: state.conversation,
          chats: [],
          elicitations: [],
          readiness: { canStartExecution: true, blockingReasons: [] },
          changeSequence: state.changeSequence,
        },
      });
    if (id === method.get) return route.fulfill({ json: state });
    if (id === method.procedure)
      return route.fulfill({
        json: {
          project: { projectId: "p", name: state.project.name },
          procedure: {
            procedureId: "procedure",
            revision: 1,
            revisionNumber: 1,
            status: "draft",
            document: {
              title: "生成した手順書",
              overview: "動作確認",
              prerequisites: [],
              steps: [],
            },
          },
          source: {
            executionId: state.execution.executionId,
            executionRevision: state.execution.revision,
            checkCount: 1,
            evidenceCount: 1,
          },
          evidence: { ai: [], human: [] },
          integrity: { status: "valid", issues: [] },
          conversation: { items: [], previousCursor: null, hasPrevious: false },
          activeRevision: null,
          elicitations: [],
          changeSequence: state.changeSequence,
        },
      });
    if (
      !(
        [
          method.start,
          method.run,
          method.message,
          method.evidence,
          method.human,
          method.permission,
          method.generate,
        ] as number[]
      ).includes(id)
    )
      throw new Error(`Unexpected Wails methodID: ${id}`);
    calls.push({ id, input });
    if (id === method.evidence && input.kind === "image" && failImage) {
      failImage = false;
      return route.fulfill({
        status: 500,
        json: {
          kind: "RuntimeError",
          message: "保存失敗",
          cause: {
            code: "storage_failure",
            message: "画像を保存できませんでした",
            retryable: true,
          },
        },
      });
    }
    if (id === method.run)
      state.activeRun = {
        runId: "run",
        status: "running",
        targetedCheckIds: ["check"],
        startedAt: new Date().toISOString(),
        finishedAt: null,
      };
    if (id === method.message)
      state.conversation.items.push({
        messageId: "message",
        turnId: "turn",
        role: "user",
        status: "completed",
        content: input.content,
      });
    if (id === method.evidence)
      state.checks[0].evidence.human.push({
        evidenceId: `evidence-${state.checks[0].evidence.human.length}`,
        actor: "human",
        kind: input.kind,
        text: input.kind === "text" ? input.text : "",
        displayName: input.kind === "text" ? input.text : input.displayName,
        mimeType: input.kind === "text" ? "text/plain" : "image/png",
        size: 10,
        previewUrl:
          input.kind === "image"
            ? "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScL/nwAAAABJRU5ErkJggg=="
            : "",
        createdAt: "2026-09-27T00:02:00Z",
      });
    if (id === method.human) state.checks[0].human = side(input.checked === true);
    if (id === method.permission) state.pendingPermissions = [];
    if (id === method.start) {
      startupRoute = "#/projects/p/check";
      return route.fulfill({
        json: {
          data: { nextRoute: "#/projects/p/check" },
          receipt: { operationId: input.operationId },
        },
      });
    }
    if (id === method.generate)
      return route.fulfill({
        json: {
          data: { nextRoute: "#/projects/p/procedure" },
          receipt: { operationId: input.operationId },
        },
      });
    state.execution.revision++;
    state.changeSequence++;
    return route.fulfill({ json: { data: {}, receipt: { operationId: input.operationId } } });
  });
  return calls;
}

test("準備から開始し、AI結果と人間証跡を確認して生成する", async ({ page }) => {
  const state = snapshot();
  const calls = await mockExecution(page, state, "#/projects");
  await page.goto("/#/projects");
  await expect(page.getByRole("heading", { name: "すべてのプロジェクト" })).toBeVisible();
  await page
    .getByRole("article")
    .filter({ hasText: state.project.name })
    .getByRole("button", { name: "開く" })
    .click();
  await expect(page).toHaveURL(/#\/projects\/p\/prepare$/);
  await page.getByRole("button", { name: "動作チェックを開始" }).click();
  await expect(page).toHaveURL(/#\/projects\/p\/check$/);
  await expect(
    page.getByRole("navigation", { name: "作成工程" }).getByText("2 動作チェック"),
  ).toHaveAttribute("aria-current", "step");
  await expect(page.getByRole("region", { name: "AIとの会話" })).toBeVisible();
  await expect(page.getByRole("region", { name: "チェック項目" })).toContainText("1 画面を確認");
  await expect(page.getByRole("button", { name: "手順書の下書きを生成" })).toBeDisabled();
  await expect(page.getByLabel("AIにメッセージを送る")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "AIチェックを実行" })).toHaveCount(1);
  await expect(
    page.locator(".execution-chat").getByRole("button", { name: "AIチェックを実行" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "AIチェックを実行" }).click();
  await expect(page.getByRole("status")).toContainText("実行中");
  expect(state.checks[0].ai.checked).toBe(false);
  state.activeRun = null;
  state.checks[0].ai = side(true);
  state.changeSequence++;
  await expect(page.getByLabel("AI確認済み")).toBeChecked();
  await expect(page.getByLabel("確認済み", { exact: true })).toBeDisabled();
  await page.getByRole("button", { name: "証跡を追加" }).click();
  await page.getByLabel("テキスト証跡").fill("表示を確認した");
  await page.getByRole("button", { name: "テキストを添付" }).click();
  await expect(page.getByText("表示を確認した")).toBeVisible();
  await expect(page.getByLabel("確認済み", { exact: true })).toBeEnabled();
  await page.getByLabel("確認済み", { exact: true }).click();
  await expect(page.getByLabel("確認済み", { exact: true })).toBeChecked();
  state.readiness = { canGenerateProcedure: true, blockingReasons: [] };
  await page.reload();
  await expect(page.getByRole("button", { name: "手順書の下書きを生成" })).toBeEnabled();
  await expect(page.getByText("表示を確認した")).toBeVisible();
  await page.getByRole("button", { name: "手順書の下書きを生成" }).click();
  await expect(page).toHaveURL(/#\/projects\/p\/procedure$/);
  await expect(page.getByRole("heading", { name: "生成した手順書" })).toBeVisible();
  expect(calls.map((call) => call.id)).toEqual([
    method.start,
    method.run,
    method.evidence,
    method.human,
    method.generate,
  ]);
});

test("画像失敗を再試行し、permissionの提示optionだけを返す", async ({ page }) => {
  const state = snapshot();
  state.checks[0].ai = side(true);
  state.pendingPermissions.push({
    permissionRequestId: "permission",
    sessionId: "session",
    runId: "run",
    status: "pending",
    toolCall: {
      toolCallId: "tool",
      title: "ファイルを読む",
      kind: "read",
      command: "",
      status: "pending",
    },
    options: [{ optionId: "deny", name: "拒否", kind: "deny" }],
  });
  const calls = await mockExecution(page, state);
  await page.goto("/#/projects/p/check");
  await expect(page.getByRole("button", { name: "許可" })).toHaveCount(0);
  await page.getByRole("button", { name: "拒否" }).click();
  await expect(page.getByRole("button", { name: "拒否" })).toHaveCount(0);
  await page.getByRole("button", { name: "証跡を追加" }).click();
  await page.getByLabel("画像ファイルのパス").fill("/tmp/evidence.png");
  await page.getByRole("button", { name: "画像を添付" }).click();
  await expect(page.getByRole("alert")).toContainText("画像を保存できませんでした");
  await expect(page.getByLabel("画像ファイルのパス")).toHaveValue("/tmp/evidence.png");
  await page.getByRole("button", { name: "画像を添付" }).click();
  await expect(page.getByRole("img", { name: "evidence.png" })).toBeVisible();
  expect(calls.find((call) => call.id === method.permission)?.input).toMatchObject({
    permissionRequestId: "permission",
    optionId: "deny",
  });
  expect(calls.filter((call) => call.id === method.evidence)).toHaveLength(2);
});

test("Event欠落後の再表示でsnapshotを取得する", async ({ page }) => {
  const state = snapshot();
  const calls = await mockExecution(page, state);
  await page.goto("/#/projects/p/check");
  await expect(page.getByText("AIへの確認依頼や実行結果がここに表示されます。")).toBeVisible();
  state.conversation.items.push({
    messageId: "remote",
    turnId: "remote",
    role: "assistant",
    status: "completed",
    content: [
      { type: "text", text: "外部で更新", evidenceId: "", url: "", name: "", mimeType: "" },
    ],
  });
  state.changeSequence++;
  await page.reload();
  await expect(page.getByText("外部で更新")).toBeVisible();
  await expect(page.getByLabel("AIにメッセージを送る")).toHaveCount(0);
  expect(calls.find((call) => call.id === method.message)).toBeUndefined();
});

test("AIチェックの実行中に各項目のログと証跡を順に表示する", async ({ page }) => {
  const state = snapshot();
  state.checks.push({ ...state.checks[0], checkId: "second", sequence: 2, title: "二番目" });
  await mockExecution(page, state);
  await page.goto("/#/projects/p/check");
  await page.getByRole("button", { name: "AIチェックを実行" }).click();

  state.checks[0].ai = side(true);
  state.checks[0].evidence.ai.push({
    evidenceId: "first-evidence",
    kind: "text",
    text: "最初の実行結果\n終了コード0を確認",
    displayName: "",
  });
  state.conversation.items.push({
    messageId: "first-log",
    role: "agent",
    content: [{ type: "text", text: "1. 画面を確認: 完了" }],
  });
  state.changeSequence++;

  await expect(page.getByText("1. 画面を確認: 完了")).toBeVisible();
  await expect(page.getByRole("region", { name: "画面を確認のAIチェック" })).toContainText(
    "最初の実行結果",
  );
  await expect(page.getByRole("region", { name: "画面を確認のAIチェック" })).toContainText(
    "終了コード0を確認",
  );
  await expect(page.getByRole("region", { name: "二番目のAIチェック" })).toContainText("待機中");
  await expect(page.getByRole("button", { name: "AIチェックを実行" })).toBeDisabled();
});

test("画像の貼り付けとドロップで証跡を追加する", async ({ page }) => {
  const state = snapshot();
  state.checks[0].ai = side(true);
  const calls = await mockExecution(page, state);
  await page.goto("/#/projects/p/check");
  await expect(page.getByRole("checkbox", { name: "確認済み", exact: true })).toBeDisabled();
  await page.getByRole("button", { name: "証跡を追加" }).click();

  for (const type of ["paste", "drop"] as const) {
    await page.locator(".execution-evidence-editor").evaluate((element, eventType) => {
      const bytes = Uint8Array.from(
        atob(
          "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScL/nwAAAABJRU5ErkJggg==",
        ),
        (char) => char.charCodeAt(0),
      );
      const file = new File([bytes], `${eventType}.png`, { type: "image/png" });
      const data = new DataTransfer();
      data.items.add(file);
      element.dispatchEvent(
        eventType === "paste"
          ? new ClipboardEvent("paste", { bubbles: true, cancelable: true, clipboardData: data })
          : new DragEvent("drop", { bubbles: true, cancelable: true, dataTransfer: data }),
      );
    }, type);
    await expect
      .poll(() => calls.filter((call) => call.id === method.evidence).length)
      .toBe(type === "paste" ? 1 : 2);
  }
  expect(
    calls.filter((call) => call.id === method.evidence).every((call) => call.input.imageData),
  ).toBe(true);
  await expect(page.getByRole("checkbox", { name: "確認済み", exact: true })).toBeEnabled();
});

test("AI確認が任意の項目だけでもAIチェックを開始できる", async ({ page }) => {
  const state = snapshot();
  state.checks[0].ai = { ...side(), required: false, status: "not_required" };
  const calls = await mockExecution(page, state);
  await page.goto("/#/projects/p/check");
  const button = page.getByRole("button", { name: "AIチェックを実行" });
  await expect(button).toBeEnabled();
  await button.click();
  expect(calls.filter((call) => call.id === method.run)).toHaveLength(1);
});

test("準備で決めたコマンドを表示し、アイコンボタンでコピーできる", async ({ page }) => {
  const state = snapshot();
  await mockExecution(page, state);
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/#/projects/p/check");
  await expect(page.getByText("task dev", { exact: true })).toBeVisible();
  const copy = page.getByRole("button", { name: "1番のコマンドをコピー" });
  await expect(copy.locator("svg.lucide-copy")).toBeVisible();
  await copy.click();
  await expect(copy.locator("svg.lucide-copy")).toHaveCount(0);
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe("task dev");
});

test("画面全体は固定し、会話とチェック項目を別々にスクロールする", async ({ page }) => {
  const state = snapshot();
  state.checks = Array.from({ length: 18 }, (_, index) => ({
    ...state.checks[0],
    checkId: `check-${index}`,
    sequence: index + 1,
    title: `確認 ${index + 1}`,
  }));
  state.conversation.items = Array.from({ length: 30 }, (_, index) => ({
    messageId: `message-${index}`,
    turnId: `turn-${index}`,
    role: "agent",
    status: "completed",
    content: [{ type: "text", text: `会話 ${index + 1}` }],
  }));
  await mockExecution(page, state);
  await page.goto("/#/projects/p/check");
  await expect(page.getByRole("region", { name: "チェック項目" })).toContainText("確認 18");

  for (const width of [1200, 700]) {
    await page.setViewportSize({ width, height: 720 });
    const size = await page.evaluate(() => {
      const chat = document.querySelector(".execution-thread")!;
      const checks = document.querySelector(".execution-workspace")!;
      return {
        pageScroll: document.documentElement.scrollHeight > window.innerHeight,
        chatOverflow: chat.scrollHeight > chat.clientHeight,
        checkOverflow: checks.scrollHeight > checks.clientHeight,
        headerTop: document.querySelector(".execution-header")!.getBoundingClientRect().top,
      };
    });
    expect(size).toEqual({
      pageScroll: false,
      chatOverflow: true,
      checkOverflow: true,
      headerTop: 0,
    });
  }
});
