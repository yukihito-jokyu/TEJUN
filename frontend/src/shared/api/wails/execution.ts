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
    activeRun: view.activeRun
      ? { ...view.activeRun, targetedCheckIds: nonNull(view.activeRun.targetedCheckIds) }
      : null,
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
    activity: view.activity
      ? {
          ...view.activity,
          items: nonNull(view.activity.items).map((item) => ({
            ...item,
            content: nonNull(item.content),
          })),
        }
      : null,
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
export async function attachHumanImageEvidence(
  executionId: string,
  checkId: string,
  expectedRevision: number,
  file: File,
) {
  const imageData = await new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () =>
      resolve(typeof reader.result === "string" ? (reader.result.split(",", 2)[1] ?? "") : "");
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
  });
  return ExecutionService.AttachHumanEvidence({
    executionId,
    checkId,
    expectedRevision,
    kind: "image",
    imageData,
    displayName: file.name || "貼り付けた画像",
    operationId: crypto.randomUUID(),
  });
}
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
