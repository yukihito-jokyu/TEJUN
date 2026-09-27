import { ExecutionService } from "../../../../bindings/github.com/yukihito-jokyu/TEJUN/internal/adapter/wails";
import { nonNull } from "./index";

export async function getExecution(projectId: string, conversationCursor?: number) {
  const view = await ExecutionService.GetExecution({
    projectId,
    conversationCursor,
    conversationLimit: 50,
  });
  return {
    ...view,
    checks: nonNull(view.checks).map((check) => ({
      ...check,
      evidence: { ai: nonNull(check.evidence.ai), human: nonNull(check.evidence.human) },
    })),
    pendingPermissions: nonNull(view.pendingPermissions).map((permission) => ({
      ...permission,
      options: nonNull(permission.options),
    })),
    conversation: {
      ...view.conversation,
      items: nonNull(view.conversation.items).map((item) => ({
        ...item,
        content: nonNull(item.content),
      })),
    },
    readiness: { ...view.readiness, blockingReasons: nonNull(view.readiness.blockingReasons) },
  };
}
export type ExecutionView = Awaited<ReturnType<typeof getExecution>>;

export const runPendingChecks = (executionId: string, expectedRevision: number) =>
  ExecutionService.RunPendingChecks({
    executionId,
    expectedRevision,
    operationId: crypto.randomUUID(),
  });
export const sendExecutionMessage = (executionId: string, text: string) =>
  ExecutionService.SendExecutionMessage({
    executionId,
    content: [{ type: "text", text, evidenceId: "", url: "", name: "", mimeType: "" }],
    operationId: crypto.randomUUID(),
  });
export const setHumanCheck = (
  executionId: string,
  checkId: string,
  checked: boolean,
  expectedRevision: number,
) =>
  ExecutionService.SetHumanCheck({
    executionId,
    checkId,
    checked,
    expectedRevision,
    operationId: crypto.randomUUID(),
  });
export const attachHumanEvidence = (
  executionId: string,
  checkId: string,
  expectedRevision: number,
  kind: string,
  text: string,
  sourcePath: string,
) =>
  ExecutionService.AttachHumanEvidence({
    executionId,
    checkId,
    expectedRevision,
    kind,
    text,
    sourcePath,
    displayName: sourcePath.split(/[\\/]/).pop() ?? "",
    operationId: crypto.randomUUID(),
  });
export const respondToExecutionPermission = (
  sessionId: string,
  permissionRequestId: string,
  optionId: string,
) =>
  ExecutionService.RespondToPermissionRequest({
    sessionId,
    permissionRequestId,
    optionId,
    operationId: crypto.randomUUID(),
  });
export const generateProcedureDraft = (executionId: string, expectedRevision: number) =>
  ExecutionService.GenerateProcedureDraft({
    executionId,
    expectedRevision,
    operationId: crypto.randomUUID(),
  });
