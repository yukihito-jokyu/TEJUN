import { expect, type Page, test } from "@playwright/test";

const method = {
  startup: 2958778619,
  list: 2753046913,
  create: 909220800,
  duplicate: 1269568665,
  revision: 3792176978,
  archive: 2033535480,
  delete: 2850077653,
  reconnect: 1544727671,
  prepareExport: 199265905,
  export: 4197448530,
} as const;

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const base = {
  description: "説明",
  workspacePath: "/tmp/project",
  currentStage: "preparation",
  progress: { completed: 1, total: 3 },
  attentionRank: 0,
  attentionReason: null,
  updatedAt: "2026-09-26T00:00:00Z",
  completedAt: null,
  currentProcedureId: null,
  currentProcedureRevision: null,
  connectionState: "connected",
  errorSummary: null,
  revision: 2,
};

const projects = [
  {
    ...base,
    projectId: "active",
    name: "作業中A",
    status: "human_waiting",
    attentionRank: 1,
    attentionReason: "確認待ち",
    resumeRoute: "#/projects/active/prepare",
  },
  {
    ...base,
    projectId: "done",
    name: "完成B",
    status: "completed",
    currentProcedureId: "procedure-b",
    currentProcedureRevision: 3,
    resumeRoute: "#/projects/done/procedure",
  },
  {
    ...base,
    projectId: "error",
    name: "接続切れC",
    status: "error",
    connectionState: "disconnected",
    attentionRank: 2,
    attentionReason: "再接続が必要",
    errorSummary: "接続が切れました",
    resumeRoute: "#/projects/error/prepare",
  },
  {
    ...base,
    projectId: "running",
    name: "実行中D",
    status: "ai_running",
    resumeRoute: "#/projects/running/prepare",
  },
  {
    ...base,
    projectId: "later",
    name: "後続E",
    status: "preparing",
    resumeRoute: "#/projects/later/prepare",
  },
];

type Call = {
  object: number;
  method?: number;
  args?: { methodID?: number; args?: unknown[] } | unknown[];
};

async function mockProjects(
  page: Page,
  options: {
    firstListFailure?: boolean;
    firstCreateFailure?: boolean;
    firstMoreFailure?: boolean;
    firstExportFailure?: boolean;
  } = {},
) {
  const calls: { id: number; input: Record<string, unknown> }[] = [];
  const saved = projects.map((item) => ({ ...item }));
  let sequence = 1;
  let listFailures = 0;
  let createFailures = 0;
  let moreFailures = 0;
  let exportFailures = 0;
  await page.route("**/wails/runtime", async (route) => {
    const body = route.request().postDataJSON() as Call;
    if (body.object === 5 && body.method === 5) {
      return route.fulfill({ json: "/tmp/export.pdf" });
    }
    const call = body.args as { methodID?: number; args?: unknown[] } | undefined;
    const id = call?.methodID;
    const input = (call?.args?.[0] ?? {}) as Record<string, unknown>;
    if (id === method.startup) {
      return route.fulfill({
        json: {
          initialSetupRequired: false,
          nextRoute: "/projects",
          defaultConnection: null,
          changeSequence: 1,
        },
      });
    }
    if (id == null) return route.fulfill({ status: 404, body: "unexpected Wails call" });
    calls.push({ id, input });
    if (id === method.list) {
      if (options.firstListFailure && input.search === "作業中" && listFailures++ === 0)
        return fail(route, "一覧の取得に失敗しました");
      if (input.cursor && options.firstMoreFailure && moreFailures++ === 0)
        return fail(route, "追加読込に失敗しました");
      const statuses = (input.statuses ?? []) as string[];
      const matching = saved
        .filter(
          (item) =>
            (typeof input.search !== "string" || item.name.includes(input.search)) &&
            (statuses.length === 0 || statuses.includes(item.status)),
        )
        .sort((left, right) => right.attentionRank - left.attentionRank);
      const pageSize = saved.length > projects.length ? saved.length : 4;
      const items = input.cursor ? matching.slice(pageSize) : matching.slice(0, pageSize);
      return route.fulfill({
        json: {
          items,
          total: matching.length,
          nextCursor: !input.cursor && matching.length > pageSize ? "next" : null,
          generatedAt: "2026-09-26T00:00:00Z",
          changeSequence: sequence,
        },
      });
    }
    if (!Object.values(method).includes(id as (typeof method)[keyof typeof method]))
      return route.fulfill({ status: 404, body: `unexpected method ${id}` });
    if (id === method.create && options.firstCreateFailure && createFailures++ === 0)
      return fail(route, "保存に失敗しました");
    if (id === method.prepareExport) {
      expect(input).toMatchObject({
        procedureId: "procedure-b",
        procedureRevision: 3,
        absolutePath: "/tmp/export.pdf",
      });
      return route.fulfill({
        json: {
          destination: {
            absolutePath: "/tmp/export.pdf",
            resolvedPath: "/tmp/export.pdf",
            verifiedRootId: "root",
          },
          overwriteIdentity: null,
          destinationDisplayName: "export.pdf",
          overwriteRequired: false,
        },
      });
    }
    expect(input.operationId).toMatch(uuid);
    if (id === method.create || id === method.duplicate || id === method.revision) {
      const projectId = `new-${saved.length}`;
      saved.unshift({
        ...base,
        projectId,
        name: String(input.name),
        workspacePath: String(input.workspacePath),
        status: "preparing",
        resumeRoute: `#/projects/${projectId}/prepare`,
      });
      sequence++;
      return route.fulfill({
        json: {
          data: { projectId, nextRoute: `#/projects/${projectId}/prepare`, revision: 1 },
          receipt: { operationId: input.operationId },
        },
      });
    }
    if (id === method.export && options.firstExportFailure && exportFailures++ === 0)
      return fail(route, "出力に失敗しました");
    if (
      id === method.archive ||
      id === method.delete ||
      id === method.reconnect ||
      id === method.export
    ) {
      if (id === method.archive || id === method.delete) {
        const index = saved.findIndex((item) => item.projectId === input.projectId);
        if (index >= 0) saved.splice(index, 1);
        sequence++;
      }
      return route.fulfill({
        json: { data: { jobId: "job-1" }, receipt: { operationId: input.operationId } },
      });
    }
    return route.fulfill({ status: 404, body: `unexpected method ${id}` });
  });
  return {
    calls,
    update(projectId: string, changes: Partial<(typeof saved)[number]>) {
      const project = saved.find((item) => item.projectId === projectId);
      expect(project).toBeDefined();
      Object.assign(project!, changes);
      sequence++;
      return sequence;
    },
  };
}

async function emitProjectEvent(page: Page, sequence: number, projectId: string) {
  await page.evaluate(
    ({ sequence, projectId }) => {
      const runtime = window as Window & {
        _wails?: { dispatchWailsEvent?: (event: { name: string; data: unknown }) => void };
      };
      if (!runtime._wails?.dispatchWailsEvent) throw new Error("Wails event bridge unavailable");
      runtime._wails.dispatchWailsEvent({
        name: "app:event",
        data: {
          eventId: crypto.randomUUID(),
          name: "project.updated",
          emittedAt: new Date().toISOString(),
          aggregateType: "project",
          aggregateId: projectId,
          changeSequence: sequence,
          streamKey: `project:${projectId}`,
          streamRevision: sequence,
          correlation: { projectId },
          payload: {},
        },
      });
    },
    { sequence, projectId },
  );
}

function fail(route: Parameters<Parameters<Page["route"]>[1]>[0], message: string) {
  return route.fulfill({
    status: 500,
    contentType: "application/json",
    body: JSON.stringify({
      kind: "RuntimeError",
      message,
      cause: { code: "internal", message, retryable: true },
    }),
  });
}

async function openList(page: Page) {
  await page.goto("/#/projects");
  await expect(page.getByRole("heading", { name: "すべてのプロジェクト" })).toBeVisible();
}

function row(page: Page, name: string) {
  return page.locator(".project-row").filter({
    has: page.getByRole("heading", { name, exact: true }),
  });
}

async function selectRowAction(page: Page, name: string, action: string) {
  await row(page, name)
    .getByRole("button", { name: `${name}のその他の操作` })
    .click();
  await page.getByRole("menuitem", { name: action }).click();
}

test("一覧の検索・絞り込み・attention・追加読込と解除", async ({ page }) => {
  const { calls } = await mockProjects(page);
  await openList(page);
  await expect(page.getByRole("heading", { name: "次に対応すること" })).toBeVisible();
  await expect(page.locator(".project-attention .project-card h3")).toHaveText([
    "接続切れC",
    "作業中A",
  ]);
  await expect(row(page, "後続E")).toHaveCount(0);
  await page.getByRole("button", { name: "さらに読み込む" }).click();
  await expect(row(page, "後続E")).toBeVisible();
  expect(
    calls.find((call) => call.id === method.list && call.input.cursor === "next")?.input,
  ).toMatchObject({ sort: "attention_desc", limit: 50 });
  await page.getByRole("searchbox", { name: "プロジェクトを検索" }).fill("存在しない");
  await expect(page.getByText("条件に一致するプロジェクトがありません")).toBeVisible();
  expect(calls.some((call) => call.id === method.list && call.input.search === "存在しない")).toBe(
    true,
  );
  await page.getByRole("button", { name: "条件を解除" }).click();
  await expect(row(page, "作業中A")).toBeVisible();
  await page.getByRole("button", { name: "完成", exact: true }).click();
  await expect(row(page, "完成B")).toBeVisible();
  await expect(row(page, "作業中A")).toHaveCount(0);
  await page.getByRole("button", { name: "作業中", exact: true }).click();
  await expect(row(page, "作業中A")).toBeVisible();
  await expect(row(page, "完成B")).toHaveCount(0);
  await page.getByRole("button", { name: "すべて", exact: true }).click();
  await expect(row(page, "完成B")).toBeVisible();
  expect(
    calls.some(
      (call) => call.id === method.list && JSON.stringify(call.input.statuses) === '["completed"]',
    ),
  ).toBe(true);
  expect(
    calls.some(
      (call) =>
        call.id === method.list &&
        JSON.stringify(call.input.statuses) ===
          '["preparing","ai_running","human_waiting","procedure_editing","error"]',
    ),
  ).toBe(true);
  expect(calls.at(-1)?.input.statuses).toEqual([]);
});

test("一覧失敗と追加読込失敗を区別して再試行できる", async ({ page }) => {
  const { calls } = await mockProjects(page, { firstListFailure: true, firstMoreFailure: true });
  await openList(page);
  await expect(row(page, "作業中A")).toBeVisible();
  await page.getByRole("searchbox", { name: "プロジェクトを検索" }).fill("作業中");
  await expect(page.getByText("一覧の取得に失敗しました")).toBeVisible();
  await page.getByRole("button", { name: "再試行" }).click();
  await expect(row(page, "作業中A")).toBeVisible();
  await page.getByRole("searchbox", { name: "プロジェクトを検索" }).fill("");
  await expect(page.getByRole("button", { name: "さらに読み込む" })).toBeVisible();
  await page.getByRole("button", { name: "さらに読み込む" }).click();
  await expect(page.getByText("追加読込に失敗しました")).toBeVisible();
  await expect(row(page, "作業中A")).toBeVisible();
  await page.getByRole("button", { name: "再試行する" }).click();
  await expect(row(page, "後続E")).toBeVisible();
  expect(
    calls.filter((call) => call.id === method.list && call.input.cursor === "next"),
  ).toHaveLength(2);
});

test("新規作成は失敗後に入力を保持し、同じ操作IDで再試行する", async ({ page }) => {
  const { calls } = await mockProjects(page, { firstCreateFailure: true });
  await openList(page);
  await expect(row(page, "作業中A")).toBeVisible();
  await page.getByRole("button", { name: "新しい手順書" }).first().click();
  await page.getByRole("textbox", { name: "手順書の名前" }).fill("新しい作業");
  await page.getByRole("textbox", { name: "作業場所" }).fill("/tmp/new");
  await page.getByRole("button", { name: "準備工程へ進む" }).click();
  await expect(page.getByRole("alert")).toContainText("保存に失敗しました");
  await expect(page.getByRole("textbox", { name: "手順書の名前" })).toHaveValue("新しい作業");
  await page.getByRole("button", { name: "準備工程へ進む" }).click();
  await expect(page).toHaveURL(/#\/projects\/new-5\/prepare$/);
  await page.goto("/#/projects");
  await expect(row(page, "新しい作業")).toBeVisible();
  const creates = calls.filter((call) => call.id === method.create);
  expect(creates).toHaveLength(2);
  expect(creates[0].input).toMatchObject({ name: "新しい作業", workspacePath: "/tmp/new" });
  expect(creates[0].input.operationId).toBe(creates[1].input.operationId);
});

test("キーボードで作成dialogを操作し、失敗時はエラーへfocusする", async ({ page }) => {
  await mockProjects(page, { firstCreateFailure: true });
  await openList(page);
  const create = page.getByRole("button", { name: "新しい手順書" }).first();
  await create.focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("dialog")).toBeVisible();
  await expect(page.getByRole("dialog")).toContainText("新しい手順書を始める");
  await page.getByRole("textbox", { name: "手順書の名前" }).fill("keyboard");
  await page.getByRole("textbox", { name: "作業場所" }).fill("/tmp/keyboard");
  await page.getByRole("button", { name: "準備工程へ進む" }).click();
  await expect(page.getByRole("dialog").getByRole("alert")).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(create).toBeFocused();
});

test("複製・改訂・archive・再接続・出力の契約と取消を確認する", async ({ page }) => {
  const mock = await mockProjects(page);
  const { calls } = mock;
  await openList(page);
  await expect(row(page, "完成B")).toBeVisible();
  const duplicate = row(page, "完成B").getByRole("button", { name: "完成Bのその他の操作" });
  await selectRowAction(page, "完成B", "複製");
  await page.getByRole("button", { name: "キャンセル" }).click();
  await expect(duplicate).toBeFocused();
  expect(calls.filter((call) => call.id === method.duplicate)).toHaveLength(0);
  await selectRowAction(page, "完成B", "複製");
  await page.getByRole("button", { name: "複製する" }).click();
  await expect(page).toHaveURL(/#\/projects\/new-5\/prepare$/);
  expect(calls.find((call) => call.id === method.duplicate)?.input).toMatchObject({
    sourceProjectId: "done",
    sourceRevision: 2,
  });
  await page.goto("/#/projects");
  await expect(row(page, "完成B のコピー")).toBeVisible();
  await expect(row(page, "完成B")).toBeVisible();
  await selectRowAction(page, "完成B", "改訂");
  await page.getByRole("textbox", { name: "手順書の名前" }).fill("完成B 改訂");
  await page.getByRole("button", { name: "改訂版を作る" }).click();
  await expect(page).toHaveURL(/#\/projects\/new-6\/prepare$/);
  expect(calls.find((call) => call.id === method.revision)?.input).toMatchObject({
    sourceProjectId: "done",
    sourceProcedureId: "procedure-b",
    sourceProcedureRevision: 3,
  });
  await page.goto("/#/projects");
  await expect(row(page, "完成B のコピー")).toBeVisible();
  await expect(row(page, "完成B 改訂")).toBeVisible();
  await expect(row(page, "完成B")).toBeVisible();
  await row(page, "実行中D").getByRole("button", { name: "実行中Dのその他の操作" }).click();
  await expect(page.getByRole("menuitem", { name: "アーカイブ" })).toBeDisabled();
  await page.keyboard.press("Escape");
  await selectRowAction(page, "作業中A", "アーカイブ");
  await page.getByRole("button", { name: "アーカイブする" }).click();
  await expect(page.getByText("プロジェクトをアーカイブしました")).toBeVisible();
  expect(calls.find((call) => call.id === method.archive)?.input).toMatchObject({
    projectId: "active",
    expectedRevision: 2,
  });
  await selectRowAction(page, "接続切れC", "再接続");
  await page.getByRole("button", { name: "再接続する" }).click();
  await expect(page.getByText("再接続を受け付けました")).toBeVisible();
  expect(calls.find((call) => call.id === method.reconnect)?.input).toMatchObject({
    projectId: "error",
    expectedRevision: 2,
    strategy: "auto",
  });
  const callsBeforeEvent = calls.filter((call) => call.id === method.list).length;
  await emitProjectEvent(
    page,
    mock.update("error", {
      status: "preparing",
      connectionState: "connected",
      errorSummary: null,
      attentionRank: 0,
      attentionReason: null,
    }),
    "error",
  );
  await row(page, "接続切れC").getByRole("button", { name: "接続切れCのその他の操作" }).click();
  await expect(page.getByRole("menuitem", { name: "再接続" })).toHaveCount(0);
  await page.keyboard.press("Escape");
  await expect(row(page, "接続切れC")).toContainText("準備中");
  expect(calls.filter((call) => call.id === method.list).length).toBeGreaterThan(callsBeforeEvent);
  await selectRowAction(page, "完成B", "出力");
  await page.getByRole("button", { name: "保存先を選ぶ" }).click();
  await expect(page.getByText("新しいファイルを作成します。")).toBeVisible();
  await page.getByRole("button", { name: "出力する" }).click();
  await expect(page.getByText("出力を受け付けました")).toBeVisible();
  const callsBeforeExportEvent = calls.filter((call) => call.id === method.list).length;
  await emitProjectEvent(page, mock.update("done", { updatedAt: "2026-09-27T00:00:00Z" }), "done");
  await expect(row(page, "完成B").locator('time[datetime="2026-09-27T00:00:00Z"]')).toBeVisible();
  expect(calls.filter((call) => call.id === method.list).length).toBeGreaterThan(
    callsBeforeExportEvent,
  );
  expect(calls.find((call) => call.id === method.export)?.input).toMatchObject({
    procedureId: "procedure-b",
    procedureRevision: 3,
    format: "pdf",
    overwriteConfirmed: false,
  });
});

test("出力失敗後も完成版を表示し、同じ操作IDで再試行できる", async ({ page }) => {
  const { calls } = await mockProjects(page, { firstExportFailure: true });
  await openList(page);
  await selectRowAction(page, "完成B", "出力");
  await page.getByRole("button", { name: "保存先を選ぶ" }).click();
  await expect(page.getByText("新しいファイルを作成します。")).toBeVisible();
  await page.getByRole("button", { name: "出力する" }).click();
  await expect(page.getByRole("dialog").getByRole("alert")).toContainText("出力に失敗しました");
  await expect(page.getByRole("dialog")).toContainText("完成B");
  await page.getByRole("button", { name: "出力する" }).click();
  await expect(page.getByText("出力を受け付けました")).toBeVisible();
  const exports = calls.filter((call) => call.id === method.export);
  expect(exports).toHaveLength(2);
  expect(exports[0].input.operationId).toBe(exports[1].input.operationId);
  await page.reload();
  await expect(row(page, "完成B")).toBeVisible();
});

test("削除は赤い確認ボタンを経て実行し、対象だけ一覧から消す", async ({ page }) => {
  const { calls } = await mockProjects(page);
  await openList(page);
  await selectRowAction(page, "完成B", "削除");
  await expect(page.getByRole("dialog")).toContainText("この操作は取り消せません");
  await expect(page.getByRole("button", { name: "完全に削除する" })).toHaveAttribute(
    "data-variant",
    "destructive",
  );
  await page.getByRole("button", { name: "キャンセル" }).click();
  expect(calls.filter((call) => call.id === method.delete)).toHaveLength(0);
  await selectRowAction(page, "完成B", "削除");
  await page.getByRole("button", { name: "完全に削除する" }).click();
  await expect(row(page, "完成B")).toHaveCount(0);
  await expect(row(page, "作業中A")).toBeVisible();
  expect(calls.find((call) => call.id === method.delete)?.input).toMatchObject({
    projectId: "done",
    expectedRevision: 2,
  });
});
