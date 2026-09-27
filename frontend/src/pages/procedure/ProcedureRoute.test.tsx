import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { onAppEvent } from "@/shared/api/wails";
import {
  cancelProcedureRevision,
  exportCompletedProcedure,
  getProcedure,
  respondToProcedureElicitation,
  saveProcedureDraft,
  type ProcedureView,
} from "@/shared/api/wails/procedure";
import { ProcedureRoute } from "./ProcedureRoute";

vi.mock("@/shared/api/wails", () => ({
  onAppEvent: vi.fn(() => vi.fn()),
  onSystemWake: vi.fn(() => vi.fn()),
  parseAppError: vi.fn(() => null),
}));
vi.mock("@/shared/api/wails/procedure", () => ({
  getProcedure: vi.fn(),
  getProcedureEvidence: vi.fn(),
  saveProcedureDraft: vi.fn(),
  requestProcedureRevision: vi.fn(),
  cancelProcedureRevision: vi.fn(),
  respondToProcedureElicitation: vi.fn(),
  completeProcedure: vi.fn(),
  exportCompletedProcedure: vi.fn(),
}));

const view: ProcedureView = {
  project: { projectId: "p-1", name: "Project" },
  procedure: {
    procedureId: "d-1",
    revision: 1,
    revisionNumber: 1,
    status: "draft",
    document: { title: "初期", overview: "概要", prerequisites: [], steps: [] },
  },
  source: { executionId: "e-1", executionRevision: 1, checkCount: 0, evidenceCount: 0 },
  evidence: { ai: [], human: [] },
  integrity: { status: "valid", issues: [] },
  conversation: { items: [], hasPrevious: false },
  changeSequence: 1,
};

beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(getProcedure).mockResolvedValue(view);
  vi.mocked(onAppEvent).mockReturnValue(vi.fn());
});
afterEach(cleanup);

it("Eventで実画面を再取得し、編集保存のrevisionと結果を反映する", async () => {
  let emit!: Parameters<typeof onAppEvent>[0];
  vi.mocked(onAppEvent).mockImplementation((callback) => {
    emit = callback;
    return vi.fn();
  });
  let current = view;
  vi.mocked(getProcedure).mockImplementation(async () => current);
  render(
    <MemoryRouter initialEntries={["/projects/p-1/procedure"]}>
      <Routes>
        <Route path="/projects/:projectId/procedure" element={<ProcedureRoute />} />
      </Routes>
    </MemoryRouter>,
  );
  expect(await screen.findByRole("heading", { name: "初期" })).toBeTruthy();
  current = {
    ...view,
    changeSequence: 2,
    procedure: { ...view.procedure, document: { ...view.procedure.document, title: "外部更新" } },
  };
  act(() =>
    emit({ aggregateId: "p-1", correlation: {}, changeSequence: 2 } as Parameters<typeof emit>[0]),
  );
  expect(await screen.findByRole("heading", { name: "外部更新" })).toBeTruthy();
  fireEvent.change(screen.getByLabelText("手順書タイトル"), { target: { value: "保存済み" } });
  current = {
    ...current,
    changeSequence: 3,
    procedure: {
      ...current.procedure,
      revision: 2,
      document: { ...current.procedure.document, title: "保存済み" },
    },
  };
  fireEvent.click(screen.getByRole("button", { name: "保存" }));
  await waitFor(() =>
    expect(saveProcedureDraft).toHaveBeenCalledWith(
      "d-1",
      expect.objectContaining({ title: "保存済み" }),
      1,
      expect.any(String),
    ),
  );
  expect(await screen.findByRole("heading", { name: "保存済み" })).toBeTruthy();
});

it("完成版の出力取消を成功扱いせず、保存先選択へ渡す", async () => {
  vi.mocked(getProcedure).mockResolvedValue({
    ...view,
    procedure: { ...view.procedure, status: "completed" },
  });
  vi.mocked(exportCompletedProcedure).mockResolvedValue(false);
  render(
    <MemoryRouter initialEntries={["/projects/p-1/procedure"]}>
      <Routes>
        <Route path="/projects/:projectId/procedure" element={<ProcedureRoute />} />
      </Routes>
    </MemoryRouter>,
  );
  expect(await screen.findByRole("heading", { name: "初期" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: /Markdown/ }));
  await waitFor(() => expect(exportCompletedProcedure).toHaveBeenCalled());
  expect(vi.mocked(exportCompletedProcedure).mock.calls[0]?.[0].procedure.procedureId).toBe("d-1");
  expect(vi.mocked(exportCompletedProcedure).mock.calls[0]?.[1]).toBe("markdown");
  expect(vi.mocked(exportCompletedProcedure).mock.calls[0]?.[2]).toBe(1);
  expect(screen.queryByText("出力しました。")).toBeNull();
});

it("AI修正の取消とAgent確認の回答後に手順書を再取得する", async () => {
  vi.mocked(getProcedure).mockResolvedValue({
    ...view,
    activeRevision: { sessionId: "s-1", turnId: "t-1", jobId: "j-1", status: "running" },
    elicitations: [
      { elicitationRequestId: "q-1", mode: "form", message: "確認してください", status: "pending" },
    ],
  });
  render(
    <MemoryRouter initialEntries={["/projects/p-1/procedure"]}>
      <Routes>
        <Route path="/projects/:projectId/procedure" element={<ProcedureRoute />} />
      </Routes>
    </MemoryRouter>,
  );
  fireEvent.click(await screen.findByRole("button", { name: "AIの修正を取り消す" }));
  await waitFor(() =>
    expect(cancelProcedureRevision).toHaveBeenCalledWith("s-1", "t-1", expect.any(String)),
  );
  await waitFor(() => expect(getProcedure).toHaveBeenCalledTimes(2));
  fireEvent.click(screen.getByRole("button", { name: "応答する" }));
  await waitFor(() =>
    expect(respondToProcedureElicitation).toHaveBeenCalledWith(
      "q-1",
      "accept",
      "{}",
      expect.any(String),
    ),
  );
  await waitFor(() => expect(getProcedure).toHaveBeenCalledTimes(3));
});
