import { Dialogs } from "@wailsio/runtime";
import { beforeEach, expect, it, vi } from "vitest";

import { ProjectService } from "../../../../bindings/github.com/yukihito-jokyu/TEJUN/internal/adapter/wails";
import {
  chooseExportDestination,
  chooseWorkspaceDirectory,
  createRevision,
  deleteProject,
  exportProcedure,
  type ProjectSummary,
} from "./projects";

vi.mock("@wailsio/runtime", () => ({ Dialogs: { SaveFile: vi.fn(), OpenFile: vi.fn() } }));
vi.mock("../../../../bindings/github.com/yukihito-jokyu/TEJUN/internal/adapter/wails", () => ({
  ProjectService: {
    CreateRevision: vi.fn(),
    DeleteProject: vi.fn(),
    PrepareExportProcedure: vi.fn(),
    ExportProcedure: vi.fn(),
  },
}));

const project = {
  projectId: "p",
  name: "手順書",
  workspacePath: "/tmp/work",
  currentProcedureId: "procedure",
  currentProcedureRevision: 3,
} as ProjectSummary;

beforeEach(() => vi.clearAllMocks());

it("作業場所の選択はディレクトリだけを許可し、取消時は空文字を返す", async () => {
  vi.mocked(Dialogs.OpenFile).mockResolvedValueOnce("/tmp/work").mockResolvedValueOnce("");
  await expect(chooseWorkspaceDirectory()).resolves.toBe("/tmp/work");
  expect(Dialogs.OpenFile).toHaveBeenCalledWith(
    expect.objectContaining({ CanChooseDirectories: true, CanChooseFiles: false }),
  );
  await expect(chooseWorkspaceDirectory()).resolves.toBe("");
});

it("改訂は一覧snapshotの完成版revisionを送る", async () => {
  await createRevision(project, "改訂", "/tmp/work", "op");
  expect(ProjectService.CreateRevision).toHaveBeenCalledWith({
    sourceProjectId: "p",
    sourceProcedureId: "procedure",
    sourceProcedureRevision: 3,
    name: "改訂",
    workspacePath: "/tmp/work",
    operationId: "op",
  });
});

it("完全削除は対象revisionとoperationIdを渡す", async () => {
  await deleteProject({ ...project, revision: 3 }, "delete-op");
  expect(ProjectService.DeleteProject).toHaveBeenCalledWith({
    projectId: "p",
    expectedRevision: 3,
    operationId: "delete-op",
  });
});

it("保存先選択取消はprepareせず、確認済みidentityをexportへ渡す", async () => {
  vi.mocked(Dialogs.SaveFile).mockResolvedValueOnce("").mockResolvedValueOnce("/tmp/out.pdf");
  await expect(chooseExportDestination(project, "pdf")).resolves.toBeNull();
  expect(ProjectService.PrepareExportProcedure).not.toHaveBeenCalled();
  const prepared = {
    destination: {
      absolutePath: "/tmp/out.pdf",
      resolvedPath: "/tmp/out.pdf",
      verifiedRootId: "root",
    },
    overwriteIdentity: { size: 42, sha256: "a".repeat(64), device: 1, inode: 2 },
    destinationDisplayName: "out.pdf",
    overwriteRequired: true,
  };
  vi.mocked(ProjectService.PrepareExportProcedure).mockResolvedValue(prepared);
  await expect(chooseExportDestination(project, "pdf")).resolves.toEqual(prepared);
  expect(ProjectService.PrepareExportProcedure).toHaveBeenCalledWith({
    procedureId: "procedure",
    procedureRevision: 3,
    absolutePath: "/tmp/out.pdf",
  });
  await exportProcedure(project, "pdf", prepared, "op");
  expect(ProjectService.ExportProcedure).toHaveBeenCalledWith({
    procedureId: "procedure",
    procedureRevision: 3,
    format: "pdf",
    destination: prepared.destination,
    overwriteConfirmed: true,
    overwriteIdentity: prepared.overwriteIdentity,
    operationId: "op",
  });
});
