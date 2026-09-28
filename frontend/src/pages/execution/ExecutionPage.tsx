import { AIUpdateRipple } from "@/shared/ui/AIUpdateRipple";
import { useEffect, useRef, useState } from "react";
import {
  ArrowLeft,
  Bot,
  CheckCircle2,
  ChevronRight,
  Copy,
  FileText,
  LoaderCircle,
  Paperclip,
  Terminal,
  UserRound,
} from "lucide-react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Spinner } from "@/components/ui/Spinner";
import { Textarea } from "@/components/ui/Textarea";
import { FoldWelcomeCharacterIcon } from "@/components/icons/FoldWelcomeCharacterIcon";
import type { ExecutionView } from "@/shared/api/wails/execution";
import "./ExecutionPage.css";

type Props = {
  view?: ExecutionView;
  loading: boolean;
  busy: boolean;
  loadingOlder?: boolean;
  onLoadPrevious?: () => void;
  error: string;
  onRun: () => void;
  onHumanCheck: (checkId: string, checked: boolean) => void;
  onEvidence: (checkId: string, kind: string, text: string, path: string) => Promise<boolean>;
  onImageEvidence?: (checkId: string, file: File) => Promise<boolean>;
  onPermission: (sessionId: string, id: string, optionId: string) => void;
  onGenerate: () => void;
};

export function ExecutionPage({
  view,
  loading,
  busy,
  loadingOlder,
  onLoadPrevious,
  error,
  onRun,
  onHumanCheck,
  onEvidence,
  onImageEvidence,
  onPermission,
  onGenerate,
}: Props) {
  const [evidence, setEvidence] = useState<Record<string, string>>({});
  const [paths, setPaths] = useState<Record<string, string>>({});
  const [editingEvidence, setEditingEvidence] = useState<string | null>(null);
  const [copiedCheckId, setCopiedCheckId] = useState<string | null>(null);
  const [copyErrorCheckId, setCopyErrorCheckId] = useState<string | null>(null);
  const [imageErrorCheckId, setImageErrorCheckId] = useState<string | null>(null);
  const messagesRef = useRef<HTMLDivElement>(null);
  const followMessages = useRef(true);
  const previousAI = useRef<Record<string, string> | undefined>(undefined);
  const [updatedChecks, setUpdatedChecks] = useState<string[]>([]);
  const aiResults = JSON.stringify(
    view?.checks.map((check) => [
      check.checkId,
      check.ai.status,
      check.ai.checked,
      check.ai.failureSummary,
      check.evidence.ai.map((item) => item.evidenceId),
    ]) ?? [],
  );
  useEffect(() => {
    const current = Object.fromEntries(
      (view?.checks ?? []).map((check) => [
        check.checkId,
        JSON.stringify([
          check.ai.status,
          check.ai.checked,
          check.ai.failureSummary,
          check.evidence.ai.map((item) => item.evidenceId),
        ]),
      ]),
    );
    const changed = Object.keys(current).filter(
      (id) => previousAI.current?.[id] !== undefined && previousAI.current[id] !== current[id],
    );
    previousAI.current = current;
    if (!changed.length) return;
    setUpdatedChecks([]);
    let replayFrame = 0;
    const frame = requestAnimationFrame(() => {
      replayFrame = requestAnimationFrame(() => setUpdatedChecks(changed));
    });
    return () => {
      cancelAnimationFrame(frame);
      cancelAnimationFrame(replayFrame);
    };
  }, [aiResults, view?.checks]);
  useEffect(() => {
    if (followMessages.current && messagesRef.current) {
      messagesRef.current.scrollTop = messagesRef.current.scrollHeight;
    }
  }, [view?.conversation.items, view?.activity?.items]);
  const required =
    view?.checks.reduce(
      (count, check) => count + Number(check.ai.required) + Number(check.human.required),
      0,
    ) ?? 0;
  const complete =
    view?.checks.reduce(
      (count, check) =>
        count +
        Number(check.ai.required && check.ai.checked) +
        Number(check.human.required && check.human.checked),
      0,
    ) ?? 0;
  const running = view?.activeRun?.status === "running";
  const [elapsedSeconds, setElapsedSeconds] = useState(0);
  useEffect(() => {
    const startedAt = view?.activeRun?.startedAt;
    if (!startedAt) return;
    const update = () =>
      setElapsedSeconds(
        Math.max(0, Math.floor((Date.now() - new Date(startedAt).getTime()) / 1000)),
      );
    update();
    const timer = window.setInterval(update, 1000);
    return () => window.clearInterval(timer);
  }, [view?.activeRun?.startedAt]);
  const runnable = view?.checks.some((check) =>
    ["pending", "failed", "not_required"].includes(check.ai.status),
  );
  const runCompleted =
    view?.activeRun?.targetedCheckIds.filter((id) =>
      view.checks.some(
        (check) => check.checkId === id && ["completed", "failed"].includes(check.ai.status),
      ),
    ).length ?? 0;
  const visibleMessages = [...(view?.conversation.items ?? []), ...(view?.activity?.items ?? [])];
  const addImage = (checkId: string, file: File) => {
    if (
      !onImageEvidence ||
      !["image/png", "image/jpeg"].includes(file.type) ||
      file.size > 25 * 1024 * 1024
    ) {
      setImageErrorCheckId(checkId);
      return;
    }
    setImageErrorCheckId(null);
    void onImageEvidence(checkId, file).catch(() => setImageErrorCheckId(checkId));
  };

  return (
    <main className="execution-page" aria-label="動作チェック">
      <AIUpdateRipple />
      <header className="execution-header">
        <div>
          <strong className="execution-brand">
            <span aria-hidden="true">
              <FoldWelcomeCharacterIcon size={32} />
            </span>{" "}
            TEJUN
          </strong>
          <p className="execution-eyebrow">{view?.project.name ?? "手順書"} / 動作チェック</p>
        </div>
        <div className="execution-header-actions">
          <nav className="execution-steps" aria-label="作成工程">
            <span>
              <b>1</b> 準備
            </span>
            <ChevronRight size={15} />
            <span className="active" aria-current="step">
              <b>2</b> 動作チェック
            </span>
            <ChevronRight size={15} />
            <span>
              <b>3</b> 手順書
            </span>
          </nav>
          <Button asChild variant="outline" size="sm">
            <a href="#/projects">
              <ArrowLeft size={16} aria-hidden="true" />
              一覧に戻る
            </a>
          </Button>
        </div>
      </header>
      {error && (
        <Alert variant="destructive" role="alert" className="execution-error">
          <AlertTitle>操作を完了できませんでした</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      {!view ? (
        <div className="execution-empty" role="status">
          {loading
            ? "動作チェックを読み込み中です"
            : "動作チェックを表示できません。更新してください。"}
        </div>
      ) : (
        <div className="execution-layout">
          <aside className="execution-chat" role="region" aria-label="AIとの会話">
            <div className="execution-panel-heading">
              <div>
                <p className="execution-eyebrow">AIアシスタント</p>
                <h1>動作チェック</h1>
              </div>
              <span className="execution-pill">ターミナル</span>
            </div>
            <div className="execution-assistant-status" aria-live="polite">
              <span className="execution-assistant-icon">
                <Bot size={20} />
              </span>
              <div>
                <strong>
                  {running
                    ? "AIが確認しています"
                    : view.readiness.canGenerateProcedure
                      ? "確認が揃いました"
                      : "確認項目を準備しました"}
                </strong>
                <p>
                  {running
                    ? "実行結果と証跡を記録中です。"
                    : view.readiness.canGenerateProcedure
                      ? "手順書の下書きを作成できます。"
                      : view.checks.length + "件の項目を、AIとあなたで確認します。"}
                </p>
              </div>
            </div>
            <div
              className="execution-thread"
              aria-live="polite"
              ref={messagesRef}
              onScroll={(event) => {
                const target = event.currentTarget;
                followMessages.current =
                  target.scrollHeight - target.scrollTop - target.clientHeight < 48;
              }}
            >
              {view.conversation.hasPrevious && (
                <Button variant="ghost" size="sm" disabled={loadingOlder} onClick={onLoadPrevious}>
                  以前の会話を読み込む
                </Button>
              )}
              {visibleMessages.length === 0 && (
                <p className="execution-chat-empty">
                  AIへの確認依頼や実行結果がここに表示されます。
                </p>
              )}
              <ol className="execution-messages">
                {visibleMessages.map((item) => (
                  <li
                    key={item.messageId}
                    className={
                      "execution-message " + (item.role === "user" ? "is-user" : "is-agent")
                    }
                  >
                    <span className="execution-avatar" aria-hidden="true">
                      {item.role === "user" ? (
                        <UserRound size={16} />
                      ) : item.role === "system" ? (
                        <Terminal size={16} />
                      ) : (
                        <Bot size={16} />
                      )}
                    </span>
                    <div>
                      <span className="sr-only">
                        {item.role === "user"
                          ? "あなた"
                          : item.role === "agent"
                            ? "AI"
                            : "システム"}
                      </span>
                      <p>
                        {item.role === "system" &&
                          (item.status === "failed"
                            ? "失敗："
                            : item.status === "completed"
                              ? "完了："
                              : "実行中：")}
                        {item.content
                          .map((part) => part.text ?? part.name ?? part.url ?? "添付")
                          .filter(Boolean)
                          .join("\n")}
                      </p>
                      {item.role !== "system" && item.status !== "completed" && (
                        <small>{item.status}</small>
                      )}
                    </div>
                  </li>
                ))}
                {view.activeRun && ["queued", "running"].includes(view.activeRun.status) && (
                  <li className="execution-message">
                    <span className="execution-avatar" aria-hidden="true">
                      <Bot size={16} />
                    </span>
                    <div>
                      <p className="execution-activity-status">
                        <Spinner aria-hidden="true" role="presentation" />
                        {view.activity?.phase === "tool"
                          ? "ツールを実行中"
                          : view.activity?.phase === "responding"
                            ? "AIが回答中"
                            : "AIが内容を確認中"}
                      </p>
                    </div>
                  </li>
                )}
              </ol>
              {view.pendingPermissions
                .filter((item) => item.status === "pending")
                .map((item) => (
                  <section
                    className="execution-permission"
                    key={item.permissionRequestId}
                    aria-label="操作の許可"
                  >
                    <strong>操作の許可</strong>
                    <p>{item.toolCall.title}</p>
                    {item.toolCall.command && <code>{item.toolCall.command}</code>}
                    <div>
                      {item.options.map((option) => (
                        <Button
                          key={option.optionId}
                          variant="outline"
                          size="sm"
                          disabled={busy}
                          onClick={() =>
                            onPermission(item.sessionId, item.permissionRequestId, option.optionId)
                          }
                        >
                          {option.name}
                        </Button>
                      ))}
                    </div>
                  </section>
                ))}
            </div>
            <div className="execution-composer">
              <Button disabled={busy || !!view.activeRun || !runnable} onClick={onRun}>
                AIチェックを実行
              </Button>
            </div>
          </aside>

          <section className="execution-workspace" aria-label="チェック項目">
            <div className="execution-workspace-heading">
              <div>
                <div className="execution-title-line">
                  <Terminal size={21} />
                  <h2>{view.project.name}</h2>
                  <span
                    className={
                      "execution-pill " +
                      (view.readiness.canGenerateProcedure ? "is-success" : "is-warning")
                    }
                  >
                    {view.readiness.canGenerateProcedure
                      ? "確認完了"
                      : running
                        ? "AI実行中"
                        : "確認中"}
                  </span>
                </div>
                <p>AIとあなたの確認が揃うと、手順書を作成できます。</p>
              </div>
              <div
                className="execution-progress"
                aria-label={complete + " / " + required + " チェック完了"}
              >
                <strong>
                  {complete} / {required}
                </strong>
                <span>チェック完了</span>
              </div>
            </div>
            <div className="execution-role-guide">
              <div>
                <Bot size={18} />
                <span>
                  <strong>AI確認</strong> 実行結果を自動判定
                </span>
              </div>
              <div>
                <UserRound size={18} />
                <span>
                  <strong>あなたの確認</strong> 実際の状態を記録
                </span>
              </div>
            </div>
            {view.activeRun && (
              <div className="execution-running" role="status">
                <LoaderCircle size={21} aria-hidden="true" />
                <div>
                  <strong>
                    {running ? "AIチェックを実行中です" : "AIチェックの開始を待っています"}
                  </strong>
                  <p>
                    {runCompleted} / {view.activeRun.targetedCheckIds.length}件を確認 · 経過時間{" "}
                    {Math.floor(elapsedSeconds / 60)}分{elapsedSeconds % 60}秒
                  </p>
                  <small>AIの結果は処理が終わると各項目に反映されます。</small>
                </div>
              </div>
            )}
            {view.checks.length === 0 && (
              <p className="execution-no-checks">チェック項目はありません。</p>
            )}
            <ol className="execution-step-list">
              {view.checks.map((check) => {
                const needsEvidence =
                  check.humanEvidenceRequirement !== "" &&
                  check.humanEvidenceRequirement !== "none";
                const hasEvidence = check.evidence.human.some((item) =>
                  check.humanEvidenceRequirement === "text_or_image"
                    ? item.kind === "text" || item.kind === "image"
                    : item.kind === check.humanEvidenceRequirement,
                );
                const waitingForAI = check.ai.required && !check.ai.checked;
                return (
                  <li key={check.checkId} className="execution-step-card">
                    <div className="execution-step-title">
                      <h3>
                        <span className="execution-step-number">{check.sequence}</span>{" "}
                        {check.title}
                      </h3>
                      <p>{check.instruction}</p>
                      <small>期待結果：{check.expectedResult}</small>
                      {check.suggestedCommand && (
                        <div className="execution-step-command">
                          <code>{check.suggestedCommand}</code>
                          <Button
                            variant="ghost"
                            size="icon"
                            aria-label={`${check.sequence}番のコマンドをコピー`}
                            title="コマンドをコピー"
                            onClick={() => {
                              if (!navigator.clipboard) {
                                setCopyErrorCheckId(check.checkId);
                                return;
                              }
                              void navigator.clipboard.writeText(check.suggestedCommand).then(
                                () => {
                                  setCopiedCheckId(check.checkId);
                                  setCopyErrorCheckId(null);
                                },
                                () => setCopyErrorCheckId(check.checkId),
                              );
                            }}
                          >
                            {copiedCheckId === check.checkId ? (
                              <CheckCircle2 size={16} />
                            ) : (
                              <Copy size={16} />
                            )}
                          </Button>
                          {copyErrorCheckId === check.checkId && (
                            <span role="alert">コピーできませんでした</span>
                          )}
                        </div>
                      )}
                    </div>
                    <div className="execution-check-columns">
                      <section
                        className={`execution-check-side is-ai${updatedChecks.includes(check.checkId) ? " execution-ai-update" : ""}`}
                        aria-label={check.title + "のAIチェック"}
                      >
                        <div className="execution-check-heading">
                          <input
                            type="checkbox"
                            checked={check.ai.checked}
                            disabled
                            readOnly
                            aria-label="AI確認済み"
                          />
                          <Bot size={17} />
                          <strong>AIのチェック</strong>
                          <span
                            className={"execution-pill " + (check.ai.checked ? "is-success" : "")}
                          >
                            {!check.ai.required && check.ai.status === "not_required"
                              ? "任意"
                              : checkStatus(check.ai.status)}
                          </span>
                        </div>
                        {check.ai.failureSummary && (
                          <p className="execution-side-error">{check.ai.failureSummary}</p>
                        )}
                        {check.ai.status !== "failed" && (
                          <div className="execution-evidence-summary">
                            <FileText size={16} />
                            <div>
                              <span>証跡</span>
                              {check.evidence.ai.length ? (
                                check.evidence.ai.map((item) => (
                                  <Evidence key={item.evidenceId} item={item} />
                                ))
                              ) : (
                                <p>AIの実行結果を待っています</p>
                              )}
                            </div>
                          </div>
                        )}
                      </section>
                      <section
                        className="execution-check-side is-human"
                        aria-label={check.title + "の人間チェック"}
                      >
                        <div className="execution-check-heading">
                          <input
                            type="checkbox"
                            checked={check.human.checked}
                            disabled={
                              busy ||
                              !check.human.required ||
                              waitingForAI ||
                              (needsEvidence && !hasEvidence)
                            }
                            aria-label="確認済み"
                            onChange={(event) => onHumanCheck(check.checkId, event.target.checked)}
                          />
                          <UserRound size={17} />
                          <strong>あなたのチェック</strong>
                          <span
                            className={
                              "execution-pill " + (check.human.checked ? "is-success" : "")
                            }
                          >
                            {!check.human.required
                              ? "対象外"
                              : check.human.checked
                                ? "完了"
                                : needsEvidence && !hasEvidence
                                  ? "証跡が必要"
                                  : "未確認"}
                          </span>
                        </div>
                        {needsEvidence && (
                          <p className="execution-evidence-requirement">
                            必要な証跡：{evidenceRequirementLabel(check.humanEvidenceRequirement)}
                          </p>
                        )}
                        {check.evidence.human.length > 0 && (
                          <div className="execution-evidence-summary">
                            <Paperclip size={16} />
                            <div>
                              <span>証跡</span>
                              {check.evidence.human.map((item) => (
                                <Evidence key={item.evidenceId} item={item} />
                              ))}
                            </div>
                          </div>
                        )}
                        {check.human.required && (
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={busy || waitingForAI}
                            aria-expanded={editingEvidence === check.checkId}
                            onClick={() =>
                              setEditingEvidence((current) =>
                                current === check.checkId ? null : check.checkId,
                              )
                            }
                          >
                            <Paperclip size={15} /> 証跡を追加
                          </Button>
                        )}
                        {editingEvidence === check.checkId && (
                          <div
                            className="execution-evidence-editor"
                            role="group"
                            aria-label={check.title + "の証跡入力"}
                            tabIndex={0}
                            onPaste={(event) => {
                              const file = Array.from(event.clipboardData.items)
                                .map((item) => item.getAsFile())
                                .find((item) => item?.type.startsWith("image/"));
                              if (file) {
                                event.preventDefault();
                                addImage(check.checkId, file);
                              }
                            }}
                            onDragOver={(event) => event.preventDefault()}
                            onDrop={(event) => {
                              event.preventDefault();
                              const file = Array.from(event.dataTransfer.files).find((item) =>
                                item.type.startsWith("image/"),
                              );
                              if (file) addImage(check.checkId, file);
                            }}
                          >
                            <label htmlFor={"evidence-" + check.checkId}>テキスト証跡</label>
                            <Textarea
                              id={"evidence-" + check.checkId}
                              value={evidence[check.checkId] ?? ""}
                              onChange={(event) =>
                                setEvidence({ ...evidence, [check.checkId]: event.target.value })
                              }
                            />
                            <Button
                              variant="outline"
                              size="sm"
                              disabled={busy || !evidence[check.checkId]?.trim()}
                              onClick={() => {
                                const sent = evidence[check.checkId].trim();
                                void onEvidence(check.checkId, "text", sent, "")
                                  .then((ok) => {
                                    if (ok)
                                      setEvidence((current) => ({
                                        ...current,
                                        [check.checkId]:
                                          current[check.checkId]?.trim() === sent
                                            ? ""
                                            : current[check.checkId],
                                      }));
                                  })
                                  .catch(() => undefined);
                              }}
                            >
                              テキストを添付
                            </Button>
                            <label htmlFor={"path-" + check.checkId}>画像ファイルのパス</label>
                            <input
                              id={"path-" + check.checkId}
                              value={paths[check.checkId] ?? ""}
                              onChange={(event) =>
                                setPaths({ ...paths, [check.checkId]: event.target.value })
                              }
                            />
                            <Button
                              variant="outline"
                              size="sm"
                              disabled={busy || !paths[check.checkId]?.trim()}
                              onClick={() => {
                                const sent = paths[check.checkId].trim();
                                void onEvidence(check.checkId, "image", "", sent)
                                  .then((ok) => {
                                    if (ok)
                                      setPaths((current) => ({
                                        ...current,
                                        [check.checkId]:
                                          current[check.checkId]?.trim() === sent
                                            ? ""
                                            : current[check.checkId],
                                      }));
                                  })
                                  .catch(() => undefined);
                              }}
                            >
                              画像を添付
                            </Button>
                            <label htmlFor={"image-file-" + check.checkId}>
                              画像ファイルを選択
                            </label>
                            <input
                              id={"image-file-" + check.checkId}
                              type="file"
                              accept="image/png,image/jpeg"
                              onChange={(event) => {
                                const file = event.currentTarget.files?.[0];
                                if (file) addImage(check.checkId, file);
                                event.currentTarget.value = "";
                              }}
                            />
                            <p>
                              画像はここにドラッグ＆ドロップ、またはCtrl+V / ⌘Vで貼り付けできます。
                            </p>
                            {imageErrorCheckId === check.checkId && (
                              <p role="alert">PNGまたはJPEGの画像（25MB以下）を選んでください。</p>
                            )}
                          </div>
                        )}
                      </section>
                    </div>
                  </li>
                );
              })}
            </ol>
            <footer className="execution-workspace-footer">
              <div className="execution-readiness">
                <span className="execution-readiness-icon">
                  {view.readiness.canGenerateProcedure ? (
                    <CheckCircle2 size={20} />
                  ) : (
                    required - complete
                  )}
                </span>
                <div>
                  <strong>
                    {view.readiness.canGenerateProcedure
                      ? "すべての確認が完了しました"
                      : "未完了のチェックがあります"}
                  </strong>
                  <p>
                    {view.readiness.canGenerateProcedure
                      ? "内容を基に手順書を作成できます。"
                      : (view.readiness.blockingReasons[0]?.message ??
                        "AIとあなたのチェックを完了してください。")}
                  </p>
                </div>
              </div>
              <div className="execution-footer-actions">
                <Button
                  disabled={busy || !view.readiness.canGenerateProcedure}
                  onClick={onGenerate}
                >
                  手順書の下書きを生成 <ChevronRight size={17} />
                </Button>
              </div>
            </footer>
          </section>
        </div>
      )}
    </main>
  );
}

function Evidence({ item }: { item: ExecutionView["checks"][number]["evidence"]["ai"][number] }) {
  return (
    <figure className="execution-evidence-item">
      {item.kind === "text" ? (
        <p>{item.text}</p>
      ) : (
        <>
          {item.previewUrl && item.kind === "image" && (
            <img src={item.previewUrl} alt={item.displayName || "証跡画像"} />
          )}
          <figcaption>{item.displayName || item.kind}</figcaption>
        </>
      )}
    </figure>
  );
}

function checkStatus(status: string) {
  return (
    (
      {
        pending: "待機中",
        queued: "待機中",
        running: "探索中",
        completed: "完了",
        failed: "失敗",
        cancelled: "取消済み",
        interrupted: "中断",
      } as Record<string, string>
    )[status] ?? status
  );
}

function evidenceRequirementLabel(requirement: string) {
  return (
    (
      { text: "テキスト", image: "画像", text_or_image: "テキストまたは画像" } as Record<
        string,
        string
      >
    )[requirement] ?? requirement
  );
}
