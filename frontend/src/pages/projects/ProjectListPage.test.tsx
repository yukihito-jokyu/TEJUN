import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import {
  createProject,
  createRevision,
  deleteProject,
  chooseExportDestination,
  chooseWorkspaceDirectory,
  exportProcedure,
  listProjects,
  onAppEvent,
  onSystemWake,
  parseAppError,
  type AppEvent,
  type ProjectSummary,
} from "@/shared/api/wails";

import { ProjectListPage } from "./ProjectListPage";

vi.mock("@/shared/api/wails", () => ({
  listProjects: vi.fn(),
  createProject: vi.fn(),
  createRevision: vi.fn(),
  deleteProject: vi.fn(),
  chooseExportDestination: vi.fn(),
  chooseWorkspaceDirectory: vi.fn(),
  exportProcedure: vi.fn(),
  duplicateProject: vi.fn(),
  archiveProject: vi.fn(),
  reconnectProject: vi.fn(),
  onAppEvent: vi.fn(() => vi.fn()),
  onSystemWake: vi.fn(() => vi.fn()),
  parseAppError: vi.fn(() => null),
}));

const project: ProjectSummary = {
  projectId: "one",
  name: "手順書 A",
  description: "説明",
  workspacePath: "/tmp/project-a",
  status: "error",
  currentStage: "preparation",
  progress: { completed: 1, total: 3 },
  attentionRank: 1,
  attentionReason: "再接続が必要",
  updatedAt: "2026-09-26T00:00:00Z",
  completedAt: null,
  resumeRoute: "#/projects/one/prepare",
  currentProcedureId: null,
  connectionState: "disconnected",
  errorSummary: "接続が切れました",
  revision: 2,
};

beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(parseAppError).mockReturnValue(null);
  vi.mocked(onAppEvent).mockReturnValue(vi.fn());
  vi.mocked(onSystemWake).mockReturnValue(vi.fn());
  vi.mocked(listProjects).mockResolvedValue({
    items: [project],
    total: 1,
    nextCursor: null,
    generatedAt: "",
    changeSequence: 1,
  });
});
afterEach(cleanup);

function selectProjectAction(name: string) {
  fireEvent.pointerDown(screen.getByRole("button", { name: "手順書 Aのその他の操作" }), {
    button: 0,
    ctrlKey: false,
    pointerType: "mouse",
  });
  fireEvent.click(screen.getByRole("menuitem", { name }));
}

it("作業場所をGUIで選び、取消時は入力を保持する", async () => {
  vi.mocked(chooseWorkspaceDirectory)
    .mockResolvedValueOnce("/tmp/chosen")
    .mockResolvedValueOnce("");
  render(<ProjectListPage />, { wrapper: MemoryRouter });
  await screen.findAllByText("手順書 A");
  fireEvent.click(screen.getByRole("button", { name: "新しい手順書" }));
  fireEvent.click(screen.getByRole("button", { name: "作業場所のフォルダを選ぶ" }));
  await waitFor(() =>
    expect(screen.getByRole<HTMLInputElement>("textbox", { name: "作業場所" }).value).toBe(
      "/tmp/chosen",
    ),
  );
  fireEvent.click(screen.getByRole("button", { name: "作業場所のフォルダを選ぶ" }));
  await waitFor(() => expect(chooseWorkspaceDirectory).toHaveBeenCalledTimes(2));
  expect(screen.getByRole<HTMLInputElement>("textbox", { name: "作業場所" }).value).toBe(
    "/tmp/chosen",
  );
});

it("一覧に状態アイコンと相対時刻と三点メニューを表示する", async () => {
  vi.setSystemTime(new Date("2026-09-26T00:05:00Z"));
  render(<ProjectListPage />, { wrapper: MemoryRouter });
  await screen.findAllByText("手順書 A");
  expect(screen.getByText("5分前")).toBeTruthy();
  expect(document.querySelector(".project-state-error svg")).toBeTruthy();
  expect(screen.getByRole("button", { name: "手順書 Aのその他の操作" })).toBeTruthy();
  vi.useRealTimers();
});

it("新規作成ボタンと優先カードを承認済みの構成で表示する", async () => {
  vi.mocked(listProjects).mockResolvedValue({
    items: [
      {
        ...project,
        projectId: "human",
        name: "人間の確認",
        status: "human_waiting",
        attentionRank: 5,
        attentionReason: "人間の確認待ち",
        description: "証跡を確認してください",
        errorSummary: null,
      },
      {
        ...project,
        projectId: "ai",
        name: "AIの処理",
        status: "ai_running",
        attentionRank: 3,
        attentionReason: "AI実行中",
        description: "テストを実行中",
        errorSummary: null,
      },
    ],
    total: 2,
    nextCursor: null,
    generatedAt: "",
    changeSequence: 1,
  });
  render(<ProjectListPage />, { wrapper: MemoryRouter });
  await screen.findAllByRole("heading", { name: "人間の確認" });
  expect(
    screen.getByRole("button", { name: "新しい手順書" }).querySelector("svg.lucide-plus"),
  ).toBeTruthy();
  const attention = screen.getByRole("region", { name: "次に対応すること" });
  expect(attention.textContent).toContain("優先度順");
  expect(attention.querySelectorAll(".project-card")).toHaveLength(2);
  expect(attention.querySelector(".project-attention-icon svg.lucide-user-round")).toBeTruthy();
  expect(attention.querySelector(".project-attention-icon svg.lucide-bot")).toBeTruthy();
  expect(attention.textContent).toContain("証跡を確認してください");
  expect(attention.getElementsByTagName("button")[0]?.textContent).toContain("確認を続ける");
  expect(attention.getElementsByTagName("button")[1]?.textContent).toContain("状況を見る");
});

it("三点メニューの複製とアーカイブにアイコンを表示する", async () => {
  render(<ProjectListPage />, { wrapper: MemoryRouter });
  await screen.findAllByText("手順書 A");
  fireEvent.pointerDown(screen.getByRole("button", { name: "手順書 Aのその他の操作" }), {
    button: 0,
    ctrlKey: false,
    pointerType: "mouse",
  });
  expect(
    screen.getByRole("menuitem", { name: "複製" }).querySelector("svg.lucide-copy"),
  ).toBeTruthy();
  expect(
    screen.getByRole("menuitem", { name: "アーカイブ" }).querySelector("svg.lucide-archive"),
  ).toBeTruthy();
  expect(
    screen.getByRole("menuitem", { name: "削除" }).querySelector("svg.lucide-trash-2"),
  ).toBeTruthy();
  expect(
    screen.getByRole("menuitem", { name: "削除" }).classList.contains("project-menu-delete"),
  ).toBe(true);
});

it("完全削除は確認後にのみ実行する", async () => {
  vi.mocked(deleteProject).mockResolvedValue({
    data: { projectId: "one", deletedAt: "2026-09-26T00:00:00Z" },
    receipt: { operationId: "op", committedAt: "2026-09-26T00:00:00Z", changeSequence: 2 },
  });
  render(<ProjectListPage />, { wrapper: MemoryRouter });
  await screen.findAllByText("手順書 A");
  selectProjectAction("削除");
  expect(deleteProject).not.toHaveBeenCalled();
  expect(screen.getByText(/外部へ出力済みのファイルは残ります/)).toBeTruthy();
  expect(screen.getByRole("button", { name: "完全に削除する" }).getAttribute("data-variant")).toBe(
    "destructive",
  );
  fireEvent.click(screen.getByRole("button", { name: "完全に削除する" }));
  await waitFor(() => expect(deleteProject).toHaveBeenCalledWith(project, expect.any(String)));
});

it("検索と状態条件をGoへ送り、0件と読取失敗を区別する", async () => {
  const view = render(<ProjectListPage />, { wrapper: MemoryRouter });
  await screen.findAllByText("手順書 A");
  expect(
    screen.getByRole("heading", { name: "次に対応すること" }).querySelector("svg.lucide-sparkles"),
  ).toBeTruthy();
  expect(
    screen
      .getByRole("heading", { name: "すべてのプロジェクト" })
      .querySelector("svg.lucide-folder-open"),
  ).toBeTruthy();
  expect(document.querySelector(".project-search svg.lucide-search")).toBeTruthy();
  expect(screen.queryByText("プロジェクトを検索")).toBeNull();
  fireEvent.change(screen.getByRole("searchbox", { name: "プロジェクトを検索" }), {
    target: { value: "不存在" },
  });
  vi.mocked(listProjects).mockResolvedValue({
    items: [],
    total: 0,
    nextCursor: null,
    generatedAt: "",
    changeSequence: 1,
  });
  fireEvent.click(screen.getByRole("button", { name: "完成" }));
  await waitFor(() => expect(listProjects).toHaveBeenCalledWith("不存在", "complete"));
  view.unmount();
});

it("作成失敗後も入力を保ち、同じoperationIdで再試行できる", async () => {
  vi.mocked(createProject).mockRejectedValueOnce(new Error("failed"));
  render(<ProjectListPage />, { wrapper: MemoryRouter });
  await screen.findAllByText("手順書 A");
  fireEvent.click(screen.getByRole("button", { name: "新しい手順書" }));
  fireEvent.change(screen.getByRole("textbox", { name: "手順書の名前" }), {
    target: { value: "新規" },
  });
  fireEvent.change(screen.getByRole("textbox", { name: "作業場所" }), {
    target: { value: "/tmp/new" },
  });
  fireEvent.click(screen.getByRole("button", { name: "準備工程へ進む" }));
  await screen.findByRole("alert");
  expect((screen.getByRole("textbox", { name: "手順書の名前" }) as HTMLInputElement).value).toBe(
    "新規",
  );
  fireEvent.click(screen.getByRole("button", { name: "準備工程へ進む" }));
  await waitFor(() => expect(createProject).toHaveBeenCalledTimes(2));
  expect(vi.mocked(createProject).mock.calls[0][2]).toBe(vi.mocked(createProject).mock.calls[1][2]);
});

it("失敗後に入力を変えた再送は新しいoperationIdを使う", async () => {
  vi.mocked(createProject).mockRejectedValue(new Error("failed"));
  render(<ProjectListPage />, { wrapper: MemoryRouter });
  await screen.findAllByText("手順書 A");
  fireEvent.click(screen.getByRole("button", { name: "新しい手順書" }));
  fireEvent.change(screen.getByRole("textbox", { name: "手順書の名前" }), {
    target: { value: "新規" },
  });
  fireEvent.change(screen.getByRole("textbox", { name: "作業場所" }), {
    target: { value: "/tmp/new" },
  });
  fireEvent.click(screen.getByRole("button", { name: "準備工程へ進む" }));
  await screen.findByRole("alert");
  fireEvent.change(screen.getByRole("textbox", { name: "手順書の名前" }), {
    target: { value: "修正" },
  });
  fireEvent.click(screen.getByRole("button", { name: "準備工程へ進む" }));
  await waitFor(() => expect(createProject).toHaveBeenCalledTimes(2));
  expect(vi.mocked(createProject).mock.calls[0][2]).not.toBe(
    vi.mocked(createProject).mock.calls[1][2],
  );
});

it("追加読込失敗後も一覧とcursor再試行を維持する", async () => {
  vi.mocked(listProjects)
    .mockResolvedValueOnce({
      items: [project],
      total: 2,
      nextCursor: "page-2",
      generatedAt: "",
      changeSequence: 1,
    })
    .mockRejectedValueOnce(new Error("failed"))
    .mockResolvedValueOnce({
      items: [{ ...project, projectId: "two", name: "手順書 B" }],
      total: 2,
      nextCursor: null,
      generatedAt: "",
      changeSequence: 1,
    });
  render(<ProjectListPage />, { wrapper: MemoryRouter });
  await screen.findAllByText("手順書 A");
  fireEvent.click(screen.getByRole("button", { name: "さらに読み込む" }));
  await screen.findByRole("button", { name: "再試行する" });
  expect(screen.getAllByText("手順書 A").length).toBeGreaterThan(0);
  fireEvent.click(screen.getByRole("button", { name: "再試行する" }));
  await screen.findAllByText("手順書 B");
  expect(listProjects).toHaveBeenLastCalledWith("", "all", "page-2");
});

it("Eventより古いsnapshotを表示せず再取得する", async () => {
  let emit: (event: AppEvent) => void = () => undefined;
  vi.mocked(onAppEvent).mockImplementation((callback) => {
    emit = callback;
    return vi.fn();
  });
  vi.mocked(listProjects)
    .mockResolvedValueOnce({
      items: [project],
      total: 1,
      nextCursor: null,
      generatedAt: "",
      changeSequence: 1,
    })
    .mockResolvedValueOnce({
      items: [{ ...project, name: "古い値" }],
      total: 1,
      nextCursor: null,
      generatedAt: "",
      changeSequence: 1,
    })
    .mockResolvedValueOnce({
      items: [{ ...project, name: "新しい値" }],
      total: 1,
      nextCursor: null,
      generatedAt: "",
      changeSequence: 2,
    });
  render(<ProjectListPage />, { wrapper: MemoryRouter });
  await screen.findAllByText("手順書 A");
  act(() =>
    emit({
      eventId: "e2",
      name: "project.updated",
      emittedAt: "",
      aggregateType: "project",
      aggregateId: "one",
      changeSequence: 2,
      streamKey: "project:one",
      streamRevision: 2,
      correlation: {},
      payload: {},
    }),
  );
  await screen.findAllByText("新しい値");
  expect(screen.queryByText("古い値")).toBeNull();
  expect(listProjects).toHaveBeenCalledTimes(3);
});

it("不正Event通知とsystem wakeで一覧を再取得する", async () => {
  let invalid: () => void = () => undefined;
  let wake: () => void = () => undefined;
  vi.mocked(onAppEvent).mockImplementation((_callback, onInvalid) => {
    invalid = onInvalid ?? invalid;
    return vi.fn();
  });
  vi.mocked(onSystemWake).mockImplementation((callback) => {
    wake = callback;
    return vi.fn();
  });
  render(<ProjectListPage />, { wrapper: MemoryRouter });
  await screen.findAllByText("手順書 A");
  act(invalid);
  await waitFor(() => expect(listProjects).toHaveBeenCalledTimes(2));
  act(wake);
  await waitFor(() => expect(listProjects).toHaveBeenCalledTimes(3));
});

const completeProject: ProjectSummary = {
  ...project,
  status: "completed",
  currentProcedureId: "procedure-one",
  currentProcedureRevision: 3,
};

it("完成版の改訂に現在の手順書版を渡し、失敗時は入力とoperationIdを保持する", async () => {
  vi.mocked(listProjects).mockResolvedValue({
    items: [completeProject],
    total: 1,
    nextCursor: null,
    generatedAt: "",
    changeSequence: 1,
  });
  vi.mocked(createRevision).mockRejectedValue(new Error("failed"));
  render(<ProjectListPage />, { wrapper: MemoryRouter });
  await screen.findAllByText("手順書 A");
  selectProjectAction("改訂");
  expect((screen.getByRole("textbox", { name: "手順書の名前" }) as HTMLInputElement).value).toBe(
    "手順書 A",
  );
  fireEvent.click(screen.getByRole("button", { name: "改訂版を作る" }));
  await screen.findByRole("alert");
  fireEvent.click(screen.getByRole("button", { name: "改訂版を作る" }));
  await waitFor(() => expect(createRevision).toHaveBeenCalledTimes(2));
  expect(vi.mocked(createRevision).mock.calls[0][0]).toMatchObject({
    currentProcedureId: "procedure-one",
    currentProcedureRevision: 3,
  });
  expect(vi.mocked(createRevision).mock.calls[0][3]).toBe(
    vi.mocked(createRevision).mock.calls[1][3],
  );
});

it("保存先選択取消は送信せず、既存file identityを確認して同じoperationIdで再送する", async () => {
  vi.mocked(listProjects).mockResolvedValue({
    items: [completeProject],
    total: 1,
    nextCursor: null,
    generatedAt: "",
    changeSequence: 1,
  });
  vi.mocked(chooseExportDestination)
    .mockResolvedValueOnce(null)
    .mockResolvedValueOnce({
      destination: {
        absolutePath: "/tmp/existing.pdf",
        resolvedPath: "/tmp/existing.pdf",
        verifiedRootId: "root-1",
      },
      overwriteIdentity: { size: 42, sha256: "abc", device: 1, inode: 2 },
      destinationDisplayName: "existing.pdf",
      overwriteRequired: true,
    });
  vi.mocked(exportProcedure).mockRejectedValue(new Error("changed"));
  render(<ProjectListPage />, { wrapper: MemoryRouter });
  await screen.findAllByText("手順書 A");
  selectProjectAction("出力");
  fireEvent.click(screen.getByRole("button", { name: "保存先を選ぶ" }));
  await waitFor(() => expect(chooseExportDestination).toHaveBeenCalledTimes(1));
  expect(exportProcedure).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "保存先を選ぶ" }));
  await screen.findByText(/現在のサイズ 42 バイト/);
  fireEvent.click(screen.getByRole("button", { name: "既存ファイルを置き換えて出力" }));
  await screen.findByRole("alert");
  expect(document.activeElement).toBe(screen.getByRole("alert"));
  fireEvent.click(screen.getByRole("button", { name: "既存ファイルを置き換えて出力" }));
  await waitFor(() => expect(exportProcedure).toHaveBeenCalledTimes(2));
  expect(vi.mocked(exportProcedure).mock.calls[0][2]).toMatchObject({
    overwriteIdentity: { sha256: "abc" },
  });
  expect(vi.mocked(exportProcedure).mock.calls[0][3]).toBe(
    vi.mocked(exportProcedure).mock.calls[1][3],
  );
});

it.each(["destination", "overwriteIdentity"])(
  "%sが無効なら保存先の再選択を求め、新しいoperationIdで送る",
  async (field) => {
    vi.mocked(listProjects).mockResolvedValue({
      items: [completeProject],
      total: 1,
      nextCursor: null,
      generatedAt: "",
      changeSequence: 1,
    });
    const prepared = {
      destination: {
        absolutePath: "/tmp/existing.pdf",
        resolvedPath: "/tmp/existing.pdf",
        verifiedRootId: "root-1",
      },
      overwriteIdentity: { size: 42, sha256: "abc", device: 1, inode: 2 },
      destinationDisplayName: "existing.pdf",
      overwriteRequired: true,
    };
    vi.mocked(chooseExportDestination).mockResolvedValue(prepared);
    vi.mocked(exportProcedure).mockRejectedValueOnce(new Error("invalid"));
    vi.mocked(parseAppError).mockReturnValue({
      code: "validation_error",
      message: "確認情報が無効です",
      retryable: false,
      fieldErrors: { [field]: "無効です" },
    });
    render(<ProjectListPage />, { wrapper: MemoryRouter });
    await screen.findAllByText("手順書 A");
    selectProjectAction("出力");
    fireEvent.click(screen.getByRole("button", { name: "保存先を選ぶ" }));
    await screen.findByText(/現在のサイズ 42 バイト/);
    fireEvent.click(screen.getByRole("button", { name: "既存ファイルを置き換えて出力" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("保存先を選び直してください");
    expect(document.activeElement).toBe(alert);
    expect((screen.getByRole("button", { name: "出力する" }) as HTMLButtonElement).disabled).toBe(
      true,
    );
    fireEvent.click(screen.getByRole("button", { name: "保存先を選ぶ" }));
    await screen.findByText(/現在のサイズ 42 バイト/);
    fireEvent.click(screen.getByRole("button", { name: "既存ファイルを置き換えて出力" }));
    await waitFor(() => expect(exportProcedure).toHaveBeenCalledTimes(2));
    expect(vi.mocked(exportProcedure).mock.calls[0][3]).not.toBe(
      vi.mocked(exportProcedure).mock.calls[1][3],
    );
  },
);

it("保存先選択後に新規ファイルが作られたら確認をやり直し、新しいoperationIdで送る", async () => {
  vi.mocked(listProjects).mockResolvedValue({
    items: [completeProject],
    total: 1,
    nextCursor: null,
    generatedAt: "",
    changeSequence: 1,
  });
  const destination = {
    absolutePath: "/tmp/new.pdf",
    resolvedPath: "/tmp/new.pdf",
    verifiedRootId: "root-1",
  };
  vi.mocked(chooseExportDestination)
    .mockResolvedValueOnce({
      destination,
      destinationDisplayName: "new.pdf",
      overwriteRequired: false,
      overwriteIdentity: null,
    })
    .mockResolvedValueOnce({
      destination,
      destinationDisplayName: "new.pdf",
      overwriteRequired: true,
      overwriteIdentity: { size: 42, sha256: "abc", device: 1, inode: 2 },
    });
  vi.mocked(exportProcedure).mockRejectedValueOnce(new Error("file appeared"));
  vi.mocked(parseAppError).mockReturnValue({
    code: "validation_error",
    message: "上書き確認が必要です",
    retryable: false,
    fieldErrors: { overwriteConfirmed: "確認してください" },
  });
  render(<ProjectListPage />, { wrapper: MemoryRouter });
  await screen.findAllByText("手順書 A");
  selectProjectAction("出力");
  fireEvent.click(screen.getByRole("button", { name: "保存先を選ぶ" }));
  await screen.findByText("新しいファイルを作成します。");
  fireEvent.click(screen.getByRole("button", { name: "出力する" }));
  const alert = await screen.findByRole("alert");
  expect(alert.textContent).toContain("保存先を選び直してください");
  expect(document.activeElement).toBe(alert);
  expect((screen.getByRole("button", { name: "出力する" }) as HTMLButtonElement).disabled).toBe(
    true,
  );
  fireEvent.click(screen.getByRole("button", { name: "保存先を選ぶ" }));
  await screen.findByText(/現在のサイズ 42 バイト/);
  fireEvent.click(screen.getByRole("button", { name: "既存ファイルを置き換えて出力" }));
  await waitFor(() => expect(exportProcedure).toHaveBeenCalledTimes(2));
  expect(vi.mocked(exportProcedure).mock.calls[0][2].overwriteRequired).toBe(false);
  expect(vi.mocked(exportProcedure).mock.calls[1][2].overwriteRequired).toBe(true);
  expect(vi.mocked(exportProcedure).mock.calls[0][3]).not.toBe(
    vi.mocked(exportProcedure).mock.calls[1][3],
  );
});
