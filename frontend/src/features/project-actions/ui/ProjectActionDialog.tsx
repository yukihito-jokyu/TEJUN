import { useEffect, useRef, useState, type RefObject } from "react";
import { useNavigate } from "react-router-dom";
import { Dialog } from "radix-ui";
import { FolderOpen } from "lucide-react";

import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import {
  archiveProject,
  createProject,
  createRevision,
  deleteProject,
  chooseExportDestination,
  chooseWorkspaceDirectory,
  duplicateProject,
  exportProcedure,
  parseAppError,
  reconnectProject,
  type ProjectSummary,
  type PreparedExportProcedure,
} from "@/shared/api/wails";

import "./ProjectActionDialog.css";

export type ProjectAction =
  | "create"
  | "duplicate"
  | "revision"
  | "archive"
  | "delete"
  | "reconnect"
  | "export";

function errorMessage(cause: unknown) {
  return parseAppError(cause)?.message ?? "予期しないエラーが発生しました";
}

export function ProjectActionDialog({
  action,
  selected,
  actionTrigger,
  onClose,
  onNotice,
  refresh,
}: {
  action: ProjectAction;
  selected?: ProjectSummary;
  actionTrigger: RefObject<HTMLElement | null>;
  onClose: () => void;
  onNotice: (message: string) => void;
  refresh: () => void;
}) {
  const navigate = useNavigate();
  const [name, setName] = useState(
    action === "duplicate" && selected
      ? `${selected.name} のコピー`
      : action === "revision" && selected
        ? selected.name
        : "",
  );
  const [workspacePath, setWorkspacePath] = useState(
    (action === "duplicate" || action === "revision") && selected ? selected.workspacePath : "",
  );
  const [format, setFormat] = useState<"markdown" | "pdf">("pdf");
  const [prepared, setPrepared] = useState<PreparedExportProcedure | null>(null);
  const [actionError, setActionError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [choosingWorkspace, setChoosingWorkspace] = useState(false);
  const operationId = useRef(crypto.randomUUID());
  const operationPayload = useRef("");
  const actionErrorRef = useRef<HTMLParagraphElement>(null);

  useEffect(() => {
    if (actionError && !submitting) actionErrorRef.current?.focus();
  }, [actionError, submitting]);

  async function chooseDestination() {
    if (!selected || submitting) return;
    setSubmitting(true);
    setActionError("");
    setPrepared(null);
    operationId.current = crypto.randomUUID();
    operationPayload.current = "";
    try {
      setPrepared(await chooseExportDestination(selected, format));
    } catch (cause) {
      setActionError(errorMessage(cause));
    } finally {
      setSubmitting(false);
    }
  }

  async function chooseWorkspace() {
    if (submitting || choosingWorkspace) return;
    setChoosingWorkspace(true);
    setActionError("");
    try {
      const path = await chooseWorkspaceDirectory();
      if (path) setWorkspacePath(path);
    } catch (cause) {
      setActionError(errorMessage(cause));
    } finally {
      setChoosingWorkspace(false);
    }
  }

  async function submitAction() {
    if (!action) return;
    if (submitting) return;
    const payload = JSON.stringify([
      action,
      selected?.projectId,
      name.trim(),
      workspacePath.trim(),
      format,
      prepared?.destination,
      prepared?.overwriteIdentity,
    ]);
    if (operationPayload.current && operationPayload.current !== payload)
      operationId.current = crypto.randomUUID();
    operationPayload.current = payload;
    setSubmitting(true);
    setActionError("");
    try {
      if (action === "create") {
        const result = await createProject(name.trim(), workspacePath.trim(), operationId.current);
        onClose();
        await navigate(result.data.nextRoute.replace(/^#/, ""));
      } else if (action === "duplicate" && selected) {
        const result = await duplicateProject(
          selected,
          name.trim(),
          workspacePath.trim(),
          operationId.current,
        );
        onClose();
        onNotice("手順書を複製しました");
        refresh();
        await navigate(result.data.nextRoute.replace(/^#/, ""));
      } else if (action === "revision" && selected) {
        const result = await createRevision(
          selected,
          name.trim(),
          workspacePath.trim(),
          operationId.current,
        );
        onClose();
        onNotice("改訂版を作成しました");
        refresh();
        await navigate(result.data.nextRoute.replace(/^#/, ""));
      } else if (action === "archive" && selected) {
        await archiveProject(selected, operationId.current);
        onClose();
        onNotice("プロジェクトをアーカイブしました");
        refresh();
      } else if (action === "delete" && selected) {
        await deleteProject(selected, operationId.current);
        onClose();
        onNotice("プロジェクトを削除しました");
        refresh();
      } else if (action === "reconnect" && selected) {
        await reconnectProject(selected, operationId.current);
        onClose();
        onNotice("再接続を受け付けました");
        refresh();
      } else if (action === "export" && selected && prepared) {
        await exportProcedure(selected, format, prepared, operationId.current);
        onClose();
        onNotice("出力を受け付けました");
        refresh();
      }
    } catch (cause) {
      const appError = parseAppError(cause);
      const destinationInvalid =
        action === "export" &&
        appError?.code === "validation_error" &&
        (appError.fieldErrors?.destination ||
          appError.fieldErrors?.overwriteIdentity ||
          appError.fieldErrors?.overwriteConfirmed);
      if (destinationInvalid) {
        setPrepared(null);
        operationId.current = crypto.randomUUID();
        operationPayload.current = "";
      }
      setActionError(
        destinationInvalid
          ? `${appError.message}。保存先を選び直してください`
          : errorMessage(cause),
      );
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog.Root
      open={true}
      onOpenChange={(open) => {
        if (!open && !submitting) onClose();
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className="project-overlay" />
        <Dialog.Content
          className="project-action"
          aria-describedby={action === "delete" ? "project-delete-description" : undefined}
          onCloseAutoFocus={(event) => {
            if (actionTrigger.current?.isConnected) {
              event.preventDefault();
              actionTrigger.current.focus();
            }
          }}
          onEscapeKeyDown={(event) => {
            if (submitting) event.preventDefault();
          }}
          onPointerDownOutside={(event) => {
            if (submitting) event.preventDefault();
          }}
        >
          <Dialog.Title id="project-action-title">
            {
              {
                create: "新しい手順書を始める",
                duplicate: "手順書を複製",
                revision: "完成版を改訂",
                archive: "プロジェクトをアーカイブ",
                delete: "プロジェクトを削除",
                reconnect: "ACPへ再接続",
                export: "手順書を出力",
              }[action ?? "create"]
            }
          </Dialog.Title>
          {selected && <p>{selected.name}</p>}
          {(action === "create" || action === "duplicate" || action === "revision") && (
            <>
              <label htmlFor="action-name">手順書の名前</label>
              <Input
                id="action-name"
                value={name}
                disabled={submitting}
                onChange={(event) => setName(event.target.value)}
              />
              <label htmlFor="action-path">作業場所</label>
              <div className="project-path-input">
                <Input
                  id="action-path"
                  value={workspacePath}
                  disabled={submitting || choosingWorkspace}
                  onChange={(event) => setWorkspacePath(event.target.value)}
                />
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  aria-label="作業場所のフォルダを選ぶ"
                  disabled={submitting || choosingWorkspace}
                  onClick={() => void chooseWorkspace()}
                >
                  <FolderOpen size={18} />
                </Button>
              </div>
            </>
          )}
          {action === "archive" && <p>一覧から非表示にします。保存した内容は削除されません。</p>}
          {action === "delete" && (
            <Dialog.Description id="project-delete-description">
              このプロジェクトと内部の手順書・証跡を完全に削除します。元の作業フォルダ、派生プロジェクト、外部へ出力済みのファイルは残ります。この操作は取り消せません。
            </Dialog.Description>
          )}
          {action === "reconnect" && (
            <p>保存済みの接続先に再接続します。作業内容は保持されます。</p>
          )}
          {action === "revision" && <p>完成版を残して、新しい改訂版を作成します。</p>}
          {action === "export" && (
            <>
              <label htmlFor="export-format">出力形式</label>
              <select
                id="export-format"
                value={format}
                disabled={submitting}
                onChange={(event) => {
                  setFormat(event.target.value as "markdown" | "pdf");
                  setPrepared(null);
                }}
              >
                <option value="pdf">PDF</option>
                <option value="markdown">Markdown</option>
              </select>
              <Button
                type="button"
                variant="outline"
                disabled={submitting}
                onClick={() => void chooseDestination()}
              >
                保存先を選ぶ
              </Button>
              {prepared && (
                <div role="status">
                  <p>保存先: {prepared.destinationDisplayName}</p>
                  {prepared.overwriteRequired ? (
                    <p>
                      既存ファイルを置き換えます。現在のサイズ {prepared.overwriteIdentity?.size}{" "}
                      バイト、SHA-256 {prepared.overwriteIdentity?.sha256}
                      。保存先が変わっていた場合は出力しません。
                    </p>
                  ) : (
                    <p>新しいファイルを作成します。</p>
                  )}
                </div>
              )}
              <p>キャンセルした保存先の確認情報は、最長5分で失効します。</p>
            </>
          )}
          {actionError && (
            <p ref={actionErrorRef} role="alert" tabIndex={-1} className="project-action-error">
              {actionError}
            </p>
          )}
          <div className="project-action-buttons">
            <Button type="button" variant="outline" disabled={submitting} onClick={onClose}>
              キャンセル
            </Button>
            <Button
              type="button"
              variant={action === "delete" ? "destructive" : "default"}
              loading={submitting}
              disabled={
                ((action === "create" || action === "duplicate" || action === "revision") &&
                  (!name.trim() || !workspacePath.trim())) ||
                (action === "export" && !prepared)
              }
              onClick={() => void submitAction()}
            >
              {
                {
                  create: "準備工程へ進む",
                  duplicate: "複製する",
                  revision: "改訂版を作る",
                  archive: "アーカイブする",
                  delete: "完全に削除する",
                  reconnect: "再接続する",
                  export: prepared?.overwriteRequired ? "既存ファイルを置き換えて出力" : "出力する",
                }[action ?? "create"]
              }
            </Button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
