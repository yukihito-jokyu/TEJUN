import { describe, expect, it, vi } from "vitest";
import { Dialogs } from "@wailsio/runtime";
import { ProcedureService } from "../../../../bindings/github.com/yukihito-jokyu/TEJUN/internal/adapter/wails";
import type { ProcedureView as GeneratedProcedureView } from "../../../../bindings/github.com/yukihito-jokyu/TEJUN/internal/application/models";
import { exportCompletedProcedure, getProcedure, normalizeProcedureView } from "./procedure";

vi.mock("@wailsio/runtime", () => ({ Dialogs: { SaveFile: vi.fn() } }));
vi.mock("../../../../bindings/github.com/yukihito-jokyu/TEJUN/internal/adapter/wails", () => ({
  ProcedureService: {
    GetProcedure: vi.fn(),
    PrepareExportProcedure: vi.fn(),
    ExportProcedure: vi.fn(),
  },
}));

describe("normalizeProcedureView", () => {
  it("生成 DTO の null collection を画面用の空配列へ変換する", () => {
    const view = {
      project: { projectId: "p", name: "Project" },
      procedure: {
        procedureId: "d",
        revision: 1,
        revisionNumber: 1,
        status: "draft",
        document: {
          title: "Title",
          overview: "",
          prerequisites: null,
          steps: [
            { clientKey: "s", title: "Step", description: "", notes: null, evidenceRefs: null },
          ],
        },
      },
      source: { executionId: "e", executionRevision: 1, checkCount: 0, evidenceCount: 0 },
      evidence: { ai: null, human: null },
      integrity: { status: "valid", issues: null },
      conversation: { items: null },
      changeSequence: 1,
    } as unknown as GeneratedProcedureView;

    expect(normalizeProcedureView(view)).toMatchObject({
      procedure: { document: { prerequisites: [], steps: [{ notes: [], evidenceRefs: [] }] } },
      evidence: { ai: [], human: [] },
      integrity: { issues: [] },
      conversation: { items: [] },
    });
  });
});

it("取得時に会話cursorを渡し、保存先選択取消では出力しない", async () => {
  const raw = {
    project: { projectId: "p", name: "Project" },
    procedure: { procedureId: "d", revision: 2, document: { prerequisites: null, steps: null } },
    evidence: { ai: null, human: null },
    integrity: { issues: null },
    conversation: { items: null, previousCursor: 10, hasPrevious: true },
  } as unknown as GeneratedProcedureView;
  vi.mocked(ProcedureService.GetProcedure).mockResolvedValue(raw as never);
  const result = await getProcedure("p", 10);
  expect(ProcedureService.GetProcedure).toHaveBeenCalledWith({
    projectId: "p",
    conversationCursor: 10,
    conversationLimit: 50,
  });
  expect(result.procedure.document.steps).toEqual([]);
  vi.mocked(Dialogs.SaveFile).mockResolvedValue("");
  expect(await exportCompletedProcedure(result, "pdf", 2, "operation-1")).toBe(false);
  expect(ProcedureService.PrepareExportProcedure).not.toHaveBeenCalled();
  expect(ProcedureService.ExportProcedure).not.toHaveBeenCalled();
});

it("HTML出力の保存先フィルタと取消を扱う", async () => {
  const view = { project: { name: "Project" }, procedure: { procedureId: "d" } } as Parameters<
    typeof exportCompletedProcedure
  >[0];
  vi.mocked(Dialogs.SaveFile).mockResolvedValue("");
  expect(await exportCompletedProcedure(view, "html", 2, "html-op")).toBe(false);
  expect(Dialogs.SaveFile).toHaveBeenCalledWith(
    expect.objectContaining({
      Filename: "Project.html",
      Filters: [{ DisplayName: "HTML", Pattern: "*.html" }],
    }),
  );
});
