import { ProjectService } from "../../../../bindings/github.com/yukihito-jokyu/TEJUN/internal/adapter/wails";
import { Dialogs } from "@wailsio/runtime";
import type {
  ProjectListQuery,
  ProjectSummary as GeneratedProjectSummary,
  PreparedExportProcedure,
} from "../../../../bindings/github.com/yukihito-jokyu/TEJUN/internal/adapter/wails/models";

export type ProjectSummary = GeneratedProjectSummary;
export type ProjectFilter = "all" | "active" | "complete";

export async function listProjects(search: string, filter: ProjectFilter, cursor?: string) {
  const query: ProjectListQuery = {
    search,
    statuses:
      filter === "complete"
        ? ["completed"]
        : filter === "active"
          ? ["preparing", "ai_running", "human_waiting", "procedure_editing", "error"]
          : [],
    sort: "attention_desc",
    cursor,
    limit: 50,
  };
  const result = await ProjectService.ListProjects(query);
  return { ...result, items: result.items ?? [] };
}

export function createProject(name: string, workspacePath: string, operationId: string) {
  return ProjectService.CreateProject({
    name,
    description: "",
    workspacePath,
    connectionId: "",
    operationId,
  });
}

export async function chooseWorkspaceDirectory() {
  const path = await Dialogs.OpenFile({
    Title: "作業場所を選ぶ",
    CanChooseDirectories: true,
    CanChooseFiles: false,
    CanCreateDirectories: true,
  });
  return typeof path === "string" ? path : (path[0] ?? "");
}

export function duplicateProject(
  source: ProjectSummary,
  name: string,
  workspacePath: string,
  operationId: string,
) {
  return ProjectService.DuplicateProject({
    sourceProjectId: source.projectId,
    sourceRevision: source.revision,
    name,
    workspacePath,
    operationId,
  });
}

export function archiveProject(source: ProjectSummary, operationId: string) {
  return ProjectService.ArchiveProject({
    projectId: source.projectId,
    expectedRevision: source.revision,
    operationId,
  });
}

export function deleteProject(source: ProjectSummary, operationId: string) {
  return ProjectService.DeleteProject({
    projectId: source.projectId,
    expectedRevision: source.revision,
    operationId,
  });
}

export function reconnectProject(source: ProjectSummary, operationId: string) {
  return ProjectService.ReconnectProjectSession({
    projectId: source.projectId,
    expectedRevision: source.revision,
    strategy: "auto",
    operationId,
  });
}

export function createRevision(
  source: ProjectSummary,
  name: string,
  workspacePath: string,
  operationId: string,
) {
  if (!source.currentProcedureId || source.currentProcedureRevision == null)
    throw new Error("完成版の手順書情報を取得できません。再読み込みしてください");
  return ProjectService.CreateRevision({
    sourceProjectId: source.projectId,
    sourceProcedureId: source.currentProcedureId,
    sourceProcedureRevision: source.currentProcedureRevision,
    name,
    workspacePath,
    operationId,
  });
}

export async function chooseExportDestination(source: ProjectSummary, format: "markdown" | "pdf") {
  if (!source.currentProcedureId || source.currentProcedureRevision == null)
    throw new Error("完成版の手順書情報を取得できません。再読み込みしてください");
  const extension = format === "pdf" ? "pdf" : "md";
  const path = await Dialogs.SaveFile({
    Title: "手順書の保存先を選ぶ",
    Filename: `${source.name}.${extension}`,
    Filters: [{ DisplayName: format === "pdf" ? "PDF" : "Markdown", Pattern: `*.${extension}` }],
  });
  if (!path) return null;
  return ProjectService.PrepareExportProcedure({
    procedureId: source.currentProcedureId,
    procedureRevision: source.currentProcedureRevision,
    absolutePath: path,
  });
}

export function exportProcedure(
  source: ProjectSummary,
  format: "markdown" | "pdf",
  prepared: PreparedExportProcedure,
  operationId: string,
) {
  if (!source.currentProcedureId || source.currentProcedureRevision == null)
    throw new Error("完成版の手順書情報を取得できません。再読み込みしてください");
  return ProjectService.ExportProcedure({
    procedureId: source.currentProcedureId,
    procedureRevision: source.currentProcedureRevision,
    format,
    destination: prepared.destination,
    overwriteConfirmed: prepared.overwriteRequired,
    overwriteIdentity: prepared.overwriteIdentity,
    operationId,
  });
}

export type { PreparedExportProcedure };
