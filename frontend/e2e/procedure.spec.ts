import { expect, test, type Page } from "@playwright/test";

const method = {
  startup: 2958778619,
  candidates: 748173126,
  get: 2087193246,
  evidence: 3265100212,
  save: 3058994050,
  complete: 1672821581,
  prepareExport: 40576585,
  export: 906820986,
  revise: 1204010540,
  cancel: 3271773890,
  elicitation: 3945556279,
} as const;

const evidence = [
  {
    evidenceId: "ai-1",
    actor: "ai",
    kind: "text",
    displayName: "AI の実行結果",
    createdAt: "2026-09-27T00:00:00Z",
  },
  {
    evidenceId: "human-1",
    actor: "human",
    kind: "image",
    displayName: "人間の確認画像",
    createdAt: "2026-09-27T00:01:00Z",
  },
] as const;
const image =
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScL/nwAAAABJRU5ErkJggg==";

function snapshot() {
  return {
    project: { projectId: "p", name: "確認プロジェクト" },
    procedure: {
      procedureId: "procedure-p",
      revision: 1,
      revisionNumber: 1,
      status: "draft",
      document: {
        title: "確認手順書",
        overview: "動作確認の結果",
        prerequisites: ["作業環境"],
        steps: [
          {
            stepId: "step-1",
            clientKey: "step-1",
            title: "動作確認",
            description: "結果を確認",
            command: "npm test",
            notes: [],
            evidenceRefs: [
              { evidenceId: "ai-1", displayName: "実行結果", included: true },
              { evidenceId: "human-1", displayName: "人間の確認画像", included: true },
            ],
          },
        ],
      },
    },
    source: { executionId: "execution-p", executionRevision: 3, checkCount: 1, evidenceCount: 2 },
    evidence: { ai: [evidence[0]], human: [evidence[1]] },
    integrity: { status: "valid", issues: [] },
    conversation: {
      items: [] as Array<{
        messageId: string;
        turnId: string;
        role: string;
        status: string;
        content: { type: string; text: string }[];
      }>,
      previousCursor: null,
      hasPrevious: false,
    },
    activeRevision: null as null | {
      sessionId: string;
      turnId: string;
      jobId: string;
      status: string;
    },
    elicitations: [] as Array<{
      elicitationRequestId: string;
      mode: string;
      message: string;
      status: string;
      requestedSchema?: unknown;
    }>,
    changeSequence: 1,
  };
}

type State = ReturnType<typeof snapshot>;
type Call = { id: number; input: Record<string, unknown> };
async function mockProcedure(page: Page, state: State) {
  const calls: Call[] = [];
  let saveConflict = false;
  let exportFailure = false;
  let selectedPath: string | null = "/tmp/procedure.md";
  await page.route("**/wails/runtime", async (route) => {
    const body = route.request().postDataJSON() as {
      object?: number;
      method?: number;
      args?: { methodID?: number; args?: unknown[] };
    };
    if (body.object === 5 && body.method === 5) return route.fulfill({ body: selectedPath ?? "" });
    const id = body.args?.methodID ?? 0;
    const input = (body.args?.args?.[0] ?? {}) as Record<string, unknown>;
    if (id === method.startup)
      return route.fulfill({
        json: {
          initialSetupRequired: false,
          nextRoute: "#/projects/p/procedure",
          changeSequence: 1,
        },
      });
    if (id === method.candidates) return route.fulfill({ json: { items: [] } });
    if (id === method.get) return route.fulfill({ json: state });
    if (id === method.evidence) {
      calls.push({ id, input });
      const summary = evidence.find((item) => item.evidenceId === input.evidenceId);
      if (!summary)
        return route.fulfill({ status: 404, json: { message: "証跡が見つかりません" } });
      return route.fulfill({
        json: {
          summary,
          source: summary.actor === "ai" ? { actor: "ai", runId: "run-p" } : { actor: "human" },
          command: summary.actor === "ai" ? "npm test" : "",
          exitCode: summary.actor === "ai" ? 0 : null,
          textPage:
            summary.actor === "ai"
              ? {
                  content: input.textCursor ? "全件成功" : "テスト開始\n",
                  nextCursor: input.textCursor ? null : "next",
                  truncated: !input.textCursor,
                }
              : null,
          image: summary.actor === "human" ? { previewUrl: image, alt: "確認画像" } : null,
          integrity: "verified",
        },
      });
    }
    if (!(Object.values(method) as number[]).includes(id))
      throw new Error(`Unexpected Wails methodID: ${id}`);
    calls.push({ id, input });
    if (id === method.save && saveConflict) {
      saveConflict = false;
      return route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({
          kind: "RuntimeError",
          message: "版が競合しました",
          cause: { code: "revision_conflict", message: "版が競合しました", retryable: true },
        }),
      });
    }
    if (id === method.export && exportFailure) {
      exportFailure = false;
      return route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({
          kind: "RuntimeError",
          message: "出力に失敗しました",
          cause: { code: "export_failure", message: "出力に失敗しました", retryable: true },
        }),
      });
    }
    if (id === method.save) {
      state.procedure.document = input.document as State["procedure"]["document"];
      state.procedure.revision++;
    }
    if (id === method.complete) {
      state.procedure.status = "completed";
      state.procedure.revision++;
    }
    if (id === method.revise)
      state.activeRevision = {
        sessionId: "session-p",
        turnId: "turn-p",
        jobId: "job-p",
        status: "running",
      };
    if (id === method.cancel) state.activeRevision = null;
    if (id === method.elicitation) state.elicitations = [];
    if (id === method.prepareExport)
      return route.fulfill({
        json: {
          destination: {
            absolutePath: selectedPath,
            resolvedPath: selectedPath,
            verifiedRootId: "root",
          },
          overwriteIdentity: { device: 1, inode: 2, size: 3, modifiedNanos: 4 },
          destinationDisplayName: "procedure.md",
          overwriteRequired: true,
        },
      });
    state.changeSequence++;
    return route.fulfill({ json: { data: {}, receipt: { operationId: input.operationId } } });
  });
  return {
    calls,
    conflict: () => {
      saveConflict = true;
    },
    failExport: () => {
      exportFailure = true;
    },
    path: (value: string | null) => {
      selectedPath = value;
    },
  };
}

test("手順書を編集し、分離した証跡を確認して完成・出力する", async ({ page }) => {
  const state = snapshot();
  const mock = await mockProcedure(page, state);
  await page.goto("/#/projects/p/procedure");
  await expect(page.getByRole("heading", { name: "確認手順書" })).toBeVisible();
  await expect(page.getByText("動作チェックとの整合性を確認済み")).toBeVisible();
  const evidenceButton = page.getByRole("button", { name: "動作確認の証跡を見る" });
  await expect(evidenceButton).toHaveCount(1);
  await expect(page.getByText("$ npm test")).toBeVisible();
  await expect(
    page.getByRole("region", { name: "手順書本文" }).getByText("テスト開始"),
  ).toBeVisible();
  await expect
    .poll(() =>
      page
        .getByRole("region", { name: "手順書本文" })
        .getByRole("img", { name: "確認画像" })
        .evaluate((element: HTMLImageElement) => element.naturalWidth),
    )
    .toBeGreaterThan(0);

  await evidenceButton.click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("テスト開始");
  await expect(dialog.getByRole("region", { name: "AIの証跡" })).toContainText("実行結果");
  await expect(dialog.getByRole("region", { name: "人間の証跡" })).toContainText("人間の確認画像");
  await expect(dialog.getByRole("img", { name: "確認画像" })).toBeVisible();
  await dialog.getByRole("button", { name: "続きを読む" }).click();
  await expect(dialog).toContainText("全件成功");
  await dialog.getByRole("button", { name: "閉じる" }).click();
  await expect(evidenceButton).toBeFocused();
  const evidenceIds = mock.calls
    .filter((call) => call.id === method.evidence)
    .map((call) => call.input.evidenceId);
  expect(evidenceIds.filter((id) => id === "ai-1").length).toBeGreaterThanOrEqual(2);
  expect(evidenceIds).toContain("human-1");

  await page.getByRole("button", { name: "直接編集" }).click();
  await page.getByLabel("手順書タイトル").fill("更新した手順書");
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await expect(page.getByRole("heading", { name: "更新した手順書" })).toBeVisible();
  expect(mock.calls.find((call) => call.id === method.save)?.input).toMatchObject({
    expectedRevision: 1,
    document: { title: "更新した手順書" },
  });
  await page.getByRole("button", { name: "手順書を完成" }).click();
  await expect(page.getByText("手順書が完成しました")).toBeVisible();
  await expect(page.getByRole("status").filter({ hasText: "手順書を完成しました" })).toBeVisible();
  await expect(page.getByRole("button", { name: "直接編集" })).toHaveCount(0);
  expect(mock.calls.find((call) => call.id === method.complete)?.input.expectedRevision).toBe(2);

  mock.path(null);
  await page.getByRole("button", { name: "Markdownを出力" }).click();
  await expect(page.getByRole("button", { name: "Markdownを出力" })).toBeEnabled();
  expect(mock.calls.filter((call) => call.id === method.prepareExport)).toHaveLength(0);
  mock.path("/tmp/procedure.md");
  page.once("dialog", (nativeDialog) => nativeDialog.dismiss());
  await page.getByRole("button", { name: "Markdownを出力" }).click();
  await expect(page.getByRole("button", { name: "Markdownを出力" })).toBeEnabled();
  expect(mock.calls.filter((call) => call.id === method.export)).toHaveLength(0);
  mock.failExport();
  page.once("dialog", (nativeDialog) => nativeDialog.accept());
  await page.getByRole("button", { name: "Markdownを出力" }).click();
  await expect.poll(() => mock.calls.filter((call) => call.id === method.export).length).toBe(1);
  await expect(page.getByRole("status").filter({ hasText: "出力に失敗しました" })).toBeVisible();
  page.once("dialog", (nativeDialog) => nativeDialog.accept());
  await page.getByRole("button", { name: "Markdownを出力" }).click();
  await expect(
    page.getByRole("status").filter({ hasText: "Markdownの出力を受け付けました" }),
  ).toBeVisible();
  const exports = mock.calls.filter((call) => call.id === method.export);
  expect(exports).toHaveLength(2);
  expect(exports[0].input).toMatchObject({
    procedureId: "procedure-p",
    procedureRevision: 3,
    format: "markdown",
    overwriteConfirmed: true,
  });
  expect(exports[0].input.operationId).toBe(exports[1].input.operationId);

  mock.path("/tmp/procedure.pdf");
  page.once("dialog", (nativeDialog) => nativeDialog.accept());
  await page.getByRole("button", { name: "PDFを出力" }).click();
  await expect(
    page.getByRole("status").filter({ hasText: "PDFの出力を受け付けました" }),
  ).toBeVisible();
  expect(mock.calls.filter((call) => call.id === method.export).at(-1)?.input).toMatchObject({
    procedureId: "procedure-p",
    procedureRevision: 3,
    format: "pdf",
    overwriteConfirmed: true,
  });

  mock.path(null);
  const preparesBeforeCancel = mock.calls.filter((call) => call.id === method.prepareExport).length;
  const exportsBeforeCancel = mock.calls.filter((call) => call.id === method.export).length;
  await page.getByRole("button", { name: "HTMLを出力" }).click();
  await expect(page.getByRole("button", { name: "HTMLを出力" })).toBeEnabled();
  expect(mock.calls.filter((call) => call.id === method.prepareExport)).toHaveLength(
    preparesBeforeCancel,
  );
  expect(mock.calls.filter((call) => call.id === method.export)).toHaveLength(exportsBeforeCancel);
  mock.path("/tmp/procedure.html");
  page.once("dialog", (nativeDialog) => nativeDialog.accept());
  await page.getByRole("button", { name: "HTMLを出力" }).click();
  await expect(
    page.getByRole("status").filter({ hasText: "HTMLの出力を受け付けました" }),
  ).toBeVisible();
  expect(mock.calls.filter((call) => call.id === method.export)).toHaveLength(
    exportsBeforeCancel + 1,
  );
  expect(mock.calls.filter((call) => call.id === method.export).at(-1)?.input).toMatchObject({
    procedureId: "procedure-p",
    procedureRevision: 3,
    format: "html",
    overwriteConfirmed: true,
  });
});

test("手順が多くても画面全体ではなく手順書本文をスクロールできる", async ({ page }) => {
  await page.setViewportSize({ width: 1200, height: 600 });
  const state = snapshot();
  state.procedure.document.steps = Array.from({ length: 20 }, (_, index) => ({
    stepId: `step-${index}`,
    clientKey: `step-${index}`,
    title: `動作確認 ${index + 1}`,
    description: "結果を確認",
    command: "npm test",
    notes: [],
    evidenceRefs: [],
  }));
  state.conversation.items = Array.from({ length: 30 }, (_, index) => ({
    messageId: `message-${index}`,
    turnId: `turn-${index}`,
    role: index % 2 ? "agent" : "user",
    status: "completed",
    content: [{ type: "text", text: `会話 ${index + 1}` }],
  }));
  await mockProcedure(page, state);
  await page.goto("/#/projects/p/procedure");
  const workspace = page.getByRole("region", { name: "手順書本文" });
  await expect(page.getByRole("heading", { name: "動作確認 20" })).toBeAttached();
  const before = await workspace.evaluate((element) => ({
    client: element.clientHeight,
    content: element.scrollHeight,
    top: element.scrollTop,
  }));
  expect(before.content).toBeGreaterThan(before.client);
  await workspace.evaluate((element) => element.scrollTo({ top: element.scrollHeight }));
  expect(await workspace.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
  await expect(page.getByRole("heading", { name: "動作確認 20" })).toBeVisible();
  await expect(page.getByRole("navigation", { name: "作成工程" })).toBeVisible();
  const chat = page.getByRole("list").filter({ hasText: "会話 30" });
  expect(await chat.evaluate((element) => element.scrollHeight)).toBeGreaterThan(
    await chat.evaluate((element) => element.clientHeight),
  );
  await chat.evaluate((element) => element.scrollTo({ top: element.scrollHeight }));
  expect(await chat.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
  await page.evaluate(() => window.scrollTo(0, 1000));
  expect(await page.evaluate(() => window.scrollY)).toBe(0);
});

test("修正依頼と確認に応答し、競合入力の保持と編集破棄を確認する", async ({ page }) => {
  const state = snapshot();
  const mock = await mockProcedure(page, state);
  await page.goto("/#/projects/p/procedure");
  await page.getByLabel("手順書の修正をAIへ依頼").fill("説明を明確にする");
  await page.getByRole("button", { name: "修正を依頼" }).click();
  await expect(page.getByRole("button", { name: "AIの修正を取り消す" })).toBeVisible();
  await page.getByRole("button", { name: "AIの修正を取り消す" }).click();
  await expect(page.getByRole("button", { name: "AIの修正を取り消す" })).toHaveCount(0);
  state.elicitations = [
    {
      elicitationRequestId: "ask-p",
      mode: "form",
      message: "確認してください",
      status: "pending",
      requestedSchema: { type: "object" },
    },
  ];
  await page.reload();
  await expect(page.getByText("確認してください")).toBeVisible();
  await page.getByLabel("回答内容").fill('{"ok":true}');
  await page.getByRole("button", { name: "応答する" }).click();
  await expect(page.getByText("確認してください")).toHaveCount(0);
  expect(mock.calls.find((call) => call.id === method.elicitation)?.input).toMatchObject({
    elicitationRequestId: "ask-p",
    action: "accept",
    content: '{"ok":true}',
  });

  await page.getByRole("button", { name: "直接編集" }).click();
  await page.getByLabel("概要").fill("保存したい内容");
  mock.conflict();
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await expect(page.getByLabel("概要")).toHaveValue("保存したい内容");
  await expect(
    page.getByRole("status").filter({ hasText: "入力は保持されています" }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await page.reload();
  await page.getByRole("button", { name: "直接編集" }).click();
  await expect(page.getByLabel("概要")).toHaveValue("動作確認の結果");
  expect(state.procedure.status).toBe("draft");
});
