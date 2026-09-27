export type EvidenceSummary = {
  evidenceId: string;
  actor: "ai" | "human";
  kind: "text" | "command_output" | "image" | "file";
  displayName: string;
  createdAt: string;
};

export type EvidenceDetail = {
  summary: EvidenceSummary;
  source: { actor: "ai"; runId: string; toolCallId?: string } | { actor: "human" };
  command?: string;
  exitCode?: number;
  textPage?: { content: string; nextCursor?: string; truncated: boolean };
  image?: { previewUrl: string; alt: string };
  integrity: "verified" | "missing" | "hash_mismatch";
};

export type ProcedureDocument = {
  title: string;
  overview: string;
  prerequisites: string[];
  steps: {
    stepId?: string;
    clientKey: string;
    title: string;
    description: string;
    command?: string;
    notes: string[];
    evidenceRefs: { evidenceId: string; displayName: string; included: boolean }[];
  }[];
};

export type ProcedureView = {
  project: { projectId: string; name: string };
  procedure: {
    procedureId: string;
    revision: number;
    revisionNumber: number;
    status: "generating" | "draft" | "checking" | "completed" | "failed";
    document: ProcedureDocument;
  };
  source: {
    executionId: string;
    executionRevision: number;
    checkCount: number;
    evidenceCount: number;
  };
  evidence: { ai: EvidenceSummary[]; human: EvidenceSummary[] };
  integrity: {
    status: "valid" | "warning" | "blocked";
    issues: {
      code: string;
      severity: "warning" | "blocking";
      message: string;
      stepId?: string;
      evidenceId?: string;
    }[];
  };
  conversation: {
    items: {
      messageId: string;
      turnId: string;
      role: "user" | "agent" | "thought" | "system";
      content: { type: string; text?: string }[];
      status: string;
    }[];
    previousCursor?: number;
    hasPrevious: boolean;
  };
  activeRevision?: { sessionId: string; turnId: string; jobId: string; status: string };
  elicitations?: {
    elicitationRequestId: string;
    mode: string;
    message: string;
    status: string;
    requestedSchema?: unknown;
    url?: string;
  }[];
  changeSequence: number;
};

export function normalizeProcedureView(view: GeneratedProcedureView): ProcedureView {
  return {
    project: { projectId: view.project.projectId, name: view.project.name },
    procedure: {
      procedureId: view.procedure.procedureId,
      revision: view.procedure.revision,
      revisionNumber: view.procedure.revisionNumber,
      status: view.procedure.status as ProcedureView["procedure"]["status"],
      document: {
        title: view.procedure.document.title,
        overview: view.procedure.document.overview,
        prerequisites: view.procedure.document.prerequisites ?? [],
        steps: (view.procedure.document.steps ?? []).map((step) => ({
          ...step,
          notes: step.notes ?? [],
          evidenceRefs: step.evidenceRefs ?? [],
        })),
      },
    },
    source: view.source,
    evidence: {
      ai: (view.evidence.ai ?? []).map((item) => ({
        ...item,
        actor: "ai" as const,
        kind: item.kind as EvidenceSummary["kind"],
      })),
      human: (view.evidence.human ?? []).map((item) => ({
        ...item,
        actor: "human" as const,
        kind: item.kind as EvidenceSummary["kind"],
      })),
    },
    activeRevision: view.activeRevision ?? undefined,
    elicitations: view.elicitations ?? [],
    integrity: {
      status: view.integrity.status as ProcedureView["integrity"]["status"],
      issues: (view.integrity.issues ?? []).map((item) => ({
        ...item,
        severity: item.severity as "warning" | "blocking",
      })),
    },
    conversation: {
      ...view.conversation,
      previousCursor: view.conversation.previousCursor ?? undefined,
      items: (view.conversation.items ?? []).map((item) => ({
        ...item,
        role: item.role as ProcedureView["conversation"]["items"][number]["role"],
        content: (item.content ?? []).map((part) => ({ type: part.type, text: part.text })),
      })),
    },
    changeSequence: view.changeSequence,
  };
}

export async function getProcedure(projectId: string, cursor?: number): Promise<ProcedureView> {
  const value = await ProcedureService.GetProcedure({
    projectId,
    conversationCursor: cursor ?? null,
    conversationLimit: 50,
  });
  return normalizeProcedureView(value);
}

export const cancelProcedureRevision = (sessionId: string, turnId: string, operationId: string) =>
  AgentControlService.CancelAgentOperation({ sessionId, turnId, operationId });

export const respondToProcedureElicitation = (
  elicitationRequestId: string,
  action: string,
  content: string,
  operationId: string,
) =>
  AgentControlService.RespondToElicitation({ elicitationRequestId, action, content, operationId });

export function saveProcedureDraft(
  procedureId: string,
  document: ProcedureDocument,
  expectedRevision: number,
  operationId: string,
) {
  return ProcedureService.SaveProcedureDraft({
    procedureId,
    document,
    expectedRevision,
    operationId,
  });
}

export function requestProcedureRevision(
  procedureId: string,
  text: string,
  expectedRevision: number,
  operationId: string,
) {
  return ProcedureService.RequestProcedureRevision({
    procedureId,
    expectedRevision,
    content: [{ type: "text", text, evidenceId: "", url: "", name: "", mimeType: "" }],
    operationId,
  });
}

export function completeProcedure(
  procedureId: string,
  expectedRevision: number,
  operationId: string,
) {
  return ProcedureService.CompleteProcedure({ procedureId, expectedRevision, operationId });
}

export async function getProcedureEvidence(
  procedureId: string,
  evidenceId: string,
  cursor?: string,
): Promise<EvidenceDetail> {
  const detail: ProcedureEvidenceDetail = await ProcedureService.GetEvidence({
    procedureId,
    evidenceId,
    textCursor: cursor,
    textLimit: 4000,
  });
  const source = detail.source as EvidenceDetail["source"];
  return {
    ...detail,
    summary: detail.summary as EvidenceSummary,
    source,
    integrity: detail.integrity as EvidenceDetail["integrity"],
    textPage: detail.textPage
      ? { ...detail.textPage, nextCursor: detail.textPage.nextCursor ?? undefined }
      : undefined,
    image: detail.image ?? undefined,
  };
}

export async function exportCompletedProcedure(
  view: ProcedureView,
  format: "markdown" | "pdf",
  revision: number,
  operationId: string,
): Promise<boolean> {
  const extension = format === "pdf" ? "pdf" : "md";
  const path = await Dialogs.SaveFile({
    Title: "手順書の保存先を選ぶ",
    Filename: `${view.project.name}.${extension}`,
    Filters: [{ DisplayName: format === "pdf" ? "PDF" : "Markdown", Pattern: `*.${extension}` }],
  });
  if (!path) return false;
  const procedureId = view.procedure.procedureId;
  const prepared = await ProcedureService.PrepareExportProcedure({
    procedureId,
    procedureRevision: revision,
    absolutePath: path,
  });
  if (
    prepared.overwriteRequired &&
    !window.confirm(`既存ファイル ${prepared.destinationDisplayName} を置き換えますか？`)
  )
    return false;
  await ProcedureService.ExportProcedure({
    procedureId,
    procedureRevision: revision,
    format,
    destination: prepared.destination,
    overwriteConfirmed: prepared.overwriteRequired,
    overwriteIdentity: prepared.overwriteIdentity,
    operationId,
  });
  return true;
}
import { Dialogs } from "@wailsio/runtime";
import {
  AgentControlService,
  ProcedureService,
} from "../../../../bindings/github.com/yukihito-jokyu/TEJUN/internal/adapter/wails";
import type { ProcedureEvidenceDetail } from "../../../../bindings/github.com/yukihito-jokyu/TEJUN/internal/adapter/wails/models";
import type { ProcedureView as GeneratedProcedureView } from "../../../../bindings/github.com/yukihito-jokyu/TEJUN/internal/application/models";
