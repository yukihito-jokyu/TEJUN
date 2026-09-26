import { useEffect, useRef, useState } from "react";
import {
  Bot,
  Check,
  ChevronRight,
  Folder,
  LockKeyhole,
  MessageSquareText,
  Pencil,
  Send,
  Terminal,
  UserRound,
} from "lucide-react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Textarea } from "@/components/ui/Textarea";
import { FoldWelcomeCharacterIcon } from "@/components/icons/FoldWelcomeCharacterIcon";
import type { PreparationView, CheckItemInput } from "@/shared/api/wails/preparation";
import "./PreparationPage.css";

type Action =
  | "brief"
  | "plan"
  | "workspace"
  | "policy"
  | "message"
  | "start"
  | "cancel"
  | "elicitation"
  | "configuration";

export type PreparationPageProps = {
  snapshot?: PreparationView;
  loading?: boolean;
  busy?: Action;
  error?: string;
  conflictRevision?: number;
  selectedChatId?: string;
  onSelectChat?: (chatId: string) => void;
  onNewChat?: () => void;
  onLoadPrevious?: () => void;
  onRefresh: () => void;
  onSaveBrief: (brief: PreparationView["preparation"]) => void;
  onSavePlan: (items: CheckItemInput[]) => void;
  onChangeWorkspace: (path: string) => void;
  onSavePolicy: (mode: "ask_every_time" | "reuse_explicit_always_choice") => void;
  onSendMessage: (text: string) => void;
  onStart: () => void;
  onCancel: () => void;
  onRespondElicitation: (
    id: string,
    action: "accept" | "decline" | "cancel",
    content: string,
  ) => void;
  onSetMode: (modeId: string) => void;
  onSetConfigOption: (configId: string, value: string | boolean) => void;
};

export function PreparationPage(props: PreparationPageProps) {
  const { snapshot, loading, busy, error } = props;
  const [purposeDraft, setPurpose] = useState<string>();
  const [criteriaDraft, setCriteria] = useState<string>();
  const [usersDraft, setUsers] = useState<string>();
  const [workspaceDraft, setWorkspace] = useState<string>();
  const [message, setMessage] = useState("");
  const [elicitationContent, setElicitationContent] = useState<Record<string, string>>({});
  const [configDraft, setConfigDraft] = useState<Record<string, string>>({});
  const [itemsDraft, setItemsDraft] = useState<CheckItemInput[]>();
  const [policyDraft, setPolicy] = useState<"ask_every_time" | "reuse_explicit_always_choice">();
  const [workspaceConfirm, setWorkspaceConfirm] = useState(false);
  const [editor, setEditor] = useState<"brief" | "workspace" | "plan" | null>(null);
  const workspaceTrigger = useRef<HTMLButtonElement>(null);
  const workspaceConfirmButton = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (workspaceConfirm) workspaceConfirmButton.current?.focus();
  }, [workspaceConfirm]);
  const [dirty, setDirty] = useState({
    brief: false,
    plan: false,
    workspace: false,
    policy: false,
  });

  const purpose = purposeDraft ?? snapshot?.preparation.purpose ?? "";
  const criteria = criteriaDraft ?? snapshot?.preparation.completionCriteria.join("\n") ?? "";
  const users = usersDraft ?? snapshot?.preparation.intendedUsers ?? "";
  const workspace = workspaceDraft ?? snapshot?.project.workspacePath ?? "";
  const items =
    itemsDraft ??
    snapshot?.checkPlan.items.map((item) => ({ ...item, clientKey: item.checkId })) ??
    [];
  const policy = policyDraft ?? snapshot?.session?.permissionPolicy.mode ?? "ask_every_time";
  const setItems = (update: (current: CheckItemInput[]) => CheckItemInput[]) =>
    setItemsDraft(update(items));

  if (!snapshot) {
    return (
      <main className="preparation-page">
        <h1>準備</h1>
        {loading ? (
          <p role="status">準備内容を読み込んでいます…</p>
        ) : (
          <Alert variant="destructive">
            <AlertTitle>準備内容を表示できません</AlertTitle>
            <AlertDescription>
              <p>{error ?? "再取得してください。"}</p>
              <Button onClick={props.onRefresh}>再試行</Button>
            </AlertDescription>
          </Alert>
        )}
      </main>
    );
  }

  const session = snapshot.session;
  const pending = busy !== undefined;
  const viewingPastChat = !!props.selectedChatId && props.selectedChatId !== session?.sessionId;
  return (
    <main className="preparation-page" aria-labelledby="preparation-title">
      <header className="preparation-header">
        <div>
          <strong className="preparation-brand">
            <span aria-hidden="true">
              <FoldWelcomeCharacterIcon size={32} />
            </span>{" "}
            TEJUN
          </strong>
          <p className="preparation-eyebrow">{snapshot.project.name} / 準備</p>
        </div>
        <nav className="preparation-steps" aria-label="作成工程">
          <span className="active">
            <b>1</b> 準備
          </span>
          <ChevronRight size={15} />
          <span>
            <b>2</b> 動作チェック
          </span>
          <ChevronRight size={15} />
          <span>
            <b>3</b> 手順書
          </span>
        </nav>
      </header>
      {error && (
        <Alert variant="destructive" role="alert">
          <AlertTitle>
            {props.conflictRevision === undefined
              ? "操作を完了できませんでした"
              : "別の変更が保存されました"}
          </AlertTitle>
          <AlertDescription>
            <p>{error}</p>
            {props.conflictRevision !== undefined && (
              <p>
                現在の revision: {props.conflictRevision}
                。入力は保持しています。最新の状態を確認してください。
              </p>
            )}
          </AlertDescription>
        </Alert>
      )}
      <div className="preparation-layout">
        <section className="preparation-chat" aria-labelledby="conversation-title">
          <div className="preparation-chat-heading">
            <div>
              <p className="preparation-eyebrow">AIアシスタント</p>
              <h1 id="preparation-title">手順書の準備</h1>
            </div>
            <span
              className="preparation-chat-badge"
              title={`セッション: ${session?.state ?? "未接続"}`}
            >
              <Terminal size={14} /> ターミナル
            </span>
          </div>
          <h2 id="conversation-title" className="sr-only">
            会話
          </h2>
          <div className="preparation-chat-navigation">
            <label htmlFor="preparation-chat-history" className="sr-only">
              過去のチャット
            </label>
            <select
              id="preparation-chat-history"
              value={props.selectedChatId || session?.sessionId || ""}
              onChange={(event) =>
                props.onSelectChat?.(
                  event.target.value === session?.sessionId ? "" : event.target.value,
                )
              }
            >
              {!session && <option value="">まだチャットがありません</option>}
              {(snapshot.chats?.length
                ? snapshot.chats
                : session
                  ? [{ sessionId: session.sessionId, title: "現在のチャット", startedAt: "" }]
                  : []
              ).map((chat) => (
                <option key={chat.sessionId} value={chat.sessionId}>
                  {chat.title}
                  {chat.sessionId === session?.sessionId ? "（現在）" : ""}
                  {chat.startedAt ? ` · ${new Date(chat.startedAt).toLocaleString("ja-JP")}` : ""}
                </option>
              ))}
            </select>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={pending || session?.state === "busy"}
              onClick={props.onNewChat}
            >
              新規チャット
            </Button>
          </div>
          <ol className="preparation-messages" aria-live="polite">
            {snapshot.conversation.hasPrevious && (
              <li>
                <Button type="button" variant="ghost" size="sm" onClick={props.onLoadPrevious}>
                  以前のメッセージを読む
                </Button>
              </li>
            )}
            {snapshot.conversation.items.map((item) => (
              <li
                key={item.messageId}
                className={`preparation-message preparation-message-${item.role}`}
              >
                <span className="preparation-avatar">
                  {item.role === "user" ? <UserRound size={16} /> : <Bot size={16} />}
                </span>
                <div>
                  <span className="sr-only">
                    {item.role === "user"
                      ? "あなた"
                      : item.role === "agent"
                        ? "AI"
                        : item.role === "thought"
                          ? "思考"
                          : "システム"}
                  </span>
                  <p>
                    {item.content
                      .map((part) => part.text ?? part.name ?? part.url ?? "添付")
                      .join("\n")}
                  </p>
                  {item.status !== "completed" && <small>{item.status}</small>}
                </div>
              </li>
            ))}
            {snapshot.conversation.items.length === 0 && (
              <li className="preparation-message preparation-message-agent">
                <span className="preparation-avatar">
                  <Bot size={16} />
                </span>
                <div>
                  <p>どのような手順書を作りますか？</p>
                </div>
              </li>
            )}
          </ol>
          {viewingPastChat && (
            <p className="preparation-chat-archive">
              過去のチャットを表示しています。現在のチャットを選ぶと会話を続けられます。
            </p>
          )}
          {!viewingPastChat && (
            <form
              onSubmit={(event) => {
                event.preventDefault();
                if (message.trim()) props.onSendMessage(message);
              }}
            >
              <label htmlFor="preparation-message" className="sr-only">
                AIに相談する
              </label>
              <div className="preparation-composer">
                <Textarea
                  id="preparation-message"
                  placeholder="条件や確認項目を追加…"
                  value={message}
                  onChange={(event) => setMessage(event.target.value)}
                  disabled={pending}
                />
                <Button
                  type="submit"
                  size="icon"
                  aria-label="送信"
                  disabled={!message.trim() || pending}
                >
                  <Send size={16} />
                </Button>
              </div>
              {session?.state === "busy" && (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  disabled={pending}
                  onClick={props.onCancel}
                >
                  処理を取り消す
                </Button>
              )}
            </form>
          )}
        </section>
        <div className="preparation-details">
          <div className="preparation-workspace-heading">
            <div>
              <p className="preparation-eyebrow">準備</p>
              <h2>内容を確認して動作チェックを開始</h2>
              <p>AIとの対話から整理した目的・権限・確認項目です。</p>
            </div>
            <span className="preparation-status">
              {snapshot.readiness.canStartExecution ? (
                <>
                  <Check size={14} /> 準備完了
                </>
              ) : (
                "準備中"
              )}
            </span>
          </div>
          <section className="preparation-card" aria-labelledby="brief-title">
            <div className="preparation-section-heading">
              <span className="preparation-heading-icon">
                <MessageSquareText size={18} />
              </span>
              <div>
                <h2 id="brief-title">目的と完了条件</h2>
                <p>AIが理解した内容</p>
              </div>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setEditor(editor === "brief" ? null : "brief")}
              >
                <Pencil size={15} /> 修正
              </Button>
            </div>
            <dl className="preparation-summary">
              <div>
                <dt>手順書の目的</dt>
                <dd>{purpose || "未設定"}</dd>
              </div>
              <div>
                <dt>完了条件</dt>
                <dd>{criteria || "未設定"}</dd>
              </div>
              <div>
                <dt>操作対象</dt>
                <dd>
                  <Terminal size={15} /> ターミナル
                </dd>
              </div>
              <div>
                <dt>想定する利用者</dt>
                <dd>{users || "未設定"}</dd>
              </div>
            </dl>
            {editor === "brief" && (
              <form
                onSubmit={(event) => {
                  event.preventDefault();
                  props.onSaveBrief({
                    ...snapshot.preparation,
                    purpose,
                    completionCriteria: criteria
                      .split("\n")
                      .map((line) => line.trim())
                      .filter(Boolean),
                    intendedUsers: users,
                  });
                }}
              >
                <label htmlFor="prep-purpose">手順書の目的</label>
                <Textarea
                  id="prep-purpose"
                  value={purpose}
                  onChange={(event) => {
                    setPurpose(event.target.value);
                    setDirty((value) => ({ ...value, brief: true }));
                  }}
                />
                <label htmlFor="prep-criteria">完了条件（1行に1つ）</label>
                <Textarea
                  id="prep-criteria"
                  value={criteria}
                  onChange={(event) => {
                    setCriteria(event.target.value);
                    setDirty((value) => ({ ...value, brief: true }));
                  }}
                />
                <label htmlFor="prep-users">想定する利用者</label>
                <Input
                  id="prep-users"
                  value={users}
                  onChange={(event) => {
                    setUsers(event.target.value);
                    setDirty((value) => ({ ...value, brief: true }));
                  }}
                />
                <Button type="submit" disabled={pending || !dirty.brief}>
                  目的を保存
                </Button>
              </form>
            )}
          </section>
          <section className="preparation-card" aria-labelledby="workspace-title">
            <div className="preparation-section-heading">
              <span className="preparation-heading-icon">
                <LockKeyhole size={18} />
              </span>
              <div>
                <h2 id="workspace-title">作業場所とAIの権限</h2>
                <p>このプロジェクトだけに適用</p>
              </div>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setEditor(editor === "workspace" ? null : "workspace")}
              >
                <Pencil size={15} /> 変更
              </Button>
            </div>
            <div className="preparation-directory">
              <Folder size={17} />
              <code>{workspace || "未設定"}</code>
            </div>
            <p className="preparation-policy-summary">
              権限要求：{policy === "ask_every_time" ? "毎回確認する" : "明示した判断を再利用する"}
            </p>
            {editor === "workspace" && (
              <>
                <label htmlFor="prep-workspace">workspace path</label>
                <Input
                  id="prep-workspace"
                  value={workspace}
                  onChange={(event) => {
                    setWorkspace(event.target.value);
                    setDirty((value) => ({ ...value, workspace: true }));
                  }}
                />
                <Button
                  ref={workspaceTrigger}
                  variant="outline"
                  disabled={pending || !dirty.workspace || !workspace.trim()}
                  onClick={() => setWorkspaceConfirm(true)}
                >
                  変更する
                </Button>
                {workspaceConfirm && (
                  <div role="group" aria-label="作業ディレクトリ変更の確認">
                    <p>
                      変更すると現在のセッションと権限設定が失効し、新しい場所で接続し直します。
                    </p>
                    <Button
                      ref={workspaceConfirmButton}
                      variant="destructive"
                      disabled={pending}
                      onClick={() => {
                        props.onChangeWorkspace(workspace);
                        setWorkspaceConfirm(false);
                        workspaceTrigger.current?.focus();
                      }}
                    >
                      変更を確定
                    </Button>
                    <Button
                      variant="outline"
                      onClick={() => {
                        setWorkspaceConfirm(false);
                        workspaceTrigger.current?.focus();
                      }}
                    >
                      戻る
                    </Button>
                  </div>
                )}
                <h3 id="policy-title">権限要求の扱い</h3>
                <p>Agentが提示した選択肢だけを使用します。設定は現在のセッションに限ります。</p>
                <fieldset disabled={!session || pending}>
                  <legend>許可の選択</legend>
                  <label>
                    <input
                      type="radio"
                      name="permission-policy"
                      checked={policy === "ask_every_time"}
                      onChange={() => {
                        setPolicy("ask_every_time");
                        setDirty((value) => ({ ...value, policy: true }));
                      }}
                    />{" "}
                    毎回確認する
                  </label>
                  <label>
                    <input
                      type="radio"
                      name="permission-policy"
                      checked={policy === "reuse_explicit_always_choice"}
                      onChange={() => {
                        setPolicy("reuse_explicit_always_choice");
                        setDirty((value) => ({ ...value, policy: true }));
                      }}
                    />{" "}
                    明示した「常に許可/拒否」を再利用する
                  </label>
                </fieldset>
                <Button
                  variant="outline"
                  disabled={!session || pending || !dirty.policy}
                  onClick={() => props.onSavePolicy(policy)}
                >
                  設定を保存
                </Button>
                {session?.modes && (
                  <section className="preparation-agent-settings" aria-labelledby="mode-title">
                    <h2 id="mode-title">Agentのモード</h2>
                    <label htmlFor="prep-mode">現在のセッションで使うモード</label>
                    <select
                      id="prep-mode"
                      value={session.modes.currentModeId}
                      disabled={pending}
                      onChange={(event) => props.onSetMode(event.target.value)}
                    >
                      {session.modes.available.map((mode) => (
                        <option key={mode.modeId} value={mode.modeId}>
                          {mode.name}
                        </option>
                      ))}
                    </select>
                  </section>
                )}
                {session?.configOptions && session.configOptions.length > 0 && (
                  <section className="preparation-agent-settings" aria-labelledby="config-title">
                    <h2 id="config-title">Agentの設定</h2>
                    {session.configOptions.map((option) => (
                      <div key={option.id}>
                        <label htmlFor={`config-${option.id}`}>{option.name}</label>
                        {typeof option.currentValue === "boolean" ? (
                          <input
                            id={`config-${option.id}`}
                            type="checkbox"
                            checked={option.currentValue}
                            disabled={pending}
                            onChange={(event) =>
                              props.onSetConfigOption(option.id, event.target.checked)
                            }
                          />
                        ) : (
                          <>
                            <select
                              id={`config-${option.id}`}
                              value={configDraft[option.id] ?? option.currentValue}
                              disabled={pending || option.choices.length === 0}
                              onChange={(event) =>
                                setConfigDraft((current) => ({
                                  ...current,
                                  [option.id]: event.target.value,
                                }))
                              }
                            >
                              {option.choices.map((choice) => (
                                <option key={choice.value} value={choice.value}>
                                  {choice.name}
                                </option>
                              ))}
                            </select>
                            <Button
                              variant="outline"
                              disabled={
                                pending ||
                                configDraft[option.id] === undefined ||
                                configDraft[option.id] === option.currentValue
                              }
                              onClick={() =>
                                props.onSetConfigOption(option.id, configDraft[option.id])
                              }
                            >
                              設定を保存
                            </Button>
                          </>
                        )}
                      </div>
                    ))}
                  </section>
                )}
              </>
            )}
          </section>
          {snapshot.elicitations
            ?.filter((item) => item.status === "pending")
            .map((item) => (
              <section
                className="preparation-card"
                aria-labelledby={`elicitation-${item.elicitationRequestId}`}
                key={item.elicitationRequestId}
              >
                <h2 id={`elicitation-${item.elicitationRequestId}`}>Agentからの確認</h2>
                <p>{item.message}</p>
                {item.mode === "form" && (
                  <>
                    <p>JSON形式で回答してください。</p>
                    {item.requestedSchema && (
                      <pre>{JSON.stringify(item.requestedSchema, null, 2)}</pre>
                    )}
                    <label htmlFor={`elicitation-content-${item.elicitationRequestId}`}>
                      回答内容
                      <Textarea
                        id={`elicitation-content-${item.elicitationRequestId}`}
                        value={elicitationContent[item.elicitationRequestId] ?? "{}"}
                        onChange={(event) =>
                          setElicitationContent((current) => ({
                            ...current,
                            [item.elicitationRequestId]: event.target.value,
                          }))
                        }
                      />
                    </label>
                  </>
                )}
                {item.mode === "url" && item.url && /^https?:\/\//.test(item.url) && (
                  <a href={item.url} target="_blank" rel="noreferrer">
                    Agentの確認ページを開く
                  </a>
                )}
                {item.mode === "url" && <p>確認後、Agentからの完了通知を待ちます。</p>}
                {item.mode === "unsupported" && <p>この確認形式には対応していません。</p>}
                <div className="preparation-actions">
                  {item.mode === "form" && (
                    <Button
                      disabled={pending}
                      onClick={() =>
                        props.onRespondElicitation(
                          item.elicitationRequestId,
                          "accept",
                          elicitationContent[item.elicitationRequestId] ?? "{}",
                        )
                      }
                    >
                      応答する
                    </Button>
                  )}
                  <Button
                    variant="outline"
                    disabled={pending}
                    onClick={() =>
                      props.onRespondElicitation(item.elicitationRequestId, "decline", "")
                    }
                  >
                    辞退する
                  </Button>
                </div>
              </section>
            ))}
          <section className="preparation-card" aria-labelledby="plan-title">
            <div className="preparation-section-heading">
              <span className="preparation-heading-icon">
                <Terminal size={18} />
              </span>
              <div>
                <h2 id="plan-title">動作チェック案</h2>
                <p>{items.length}件・実行順に確認します</p>
              </div>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setEditor(editor === "plan" ? null : "plan")}
              >
                <Pencil size={15} /> 項目を編集
              </Button>
            </div>
            {editor !== "plan" && (
              <ol className="preparation-plan-summary">
                {items.map((item, index) => (
                  <li key={item.clientKey}>
                    <span className="preparation-plan-number">{index + 1}</span>
                    <div>
                      <strong>{item.title || "確認内容未設定"}</strong>
                      <code>{item.suggestedCommand && `$ ${item.suggestedCommand}`}</code>
                      <small>{item.humanRequired ? "人間確認あり" : "AI確認"}</small>
                    </div>
                  </li>
                ))}
              </ol>
            )}
            {editor === "plan" && (
              <>
                <ol className="preparation-plan">
                  {items.map((item, index) => (
                    <li key={item.clientKey}>
                      <label>
                        確認内容
                        <Input
                          value={item.title}
                          onChange={(event) => {
                            setItems((current) =>
                              current.map((entry, i) =>
                                i === index ? { ...entry, title: event.target.value } : entry,
                              ),
                            );
                            setDirty((value) => ({ ...value, plan: true }));
                          }}
                        />
                      </label>
                      <label>
                        手順
                        <Textarea
                          value={item.instruction}
                          onChange={(event) => {
                            setItems((current) =>
                              current.map((entry, i) =>
                                i === index ? { ...entry, instruction: event.target.value } : entry,
                              ),
                            );
                            setDirty((value) => ({ ...value, plan: true }));
                          }}
                        />
                      </label>
                      <label>
                        期待結果
                        <Input
                          value={item.expectedResult}
                          onChange={(event) => {
                            setItems((current) =>
                              current.map((entry, i) =>
                                i === index
                                  ? { ...entry, expectedResult: event.target.value }
                                  : entry,
                              ),
                            );
                            setDirty((value) => ({ ...value, plan: true }));
                          }}
                        />
                      </label>
                      <label>
                        推奨コマンド
                        <Input
                          value={item.suggestedCommand ?? ""}
                          onChange={(event) => {
                            setItems((current) =>
                              current.map((entry, i) =>
                                i === index
                                  ? { ...entry, suggestedCommand: event.target.value }
                                  : entry,
                              ),
                            );
                            setDirty((value) => ({ ...value, plan: true }));
                          }}
                        />
                      </label>
                      <label>
                        <input
                          type="checkbox"
                          checked={item.aiRequired}
                          onChange={(event) => {
                            setItems((current) =>
                              current.map((entry, i) =>
                                i === index
                                  ? { ...entry, aiRequired: event.target.checked }
                                  : entry,
                              ),
                            );
                            setDirty((value) => ({ ...value, plan: true }));
                          }}
                        />{" "}
                        AIによる確認
                      </label>
                      <label>
                        <input
                          type="checkbox"
                          checked={item.humanRequired}
                          onChange={(event) => {
                            setItems((current) =>
                              current.map((entry, i) =>
                                i === index
                                  ? { ...entry, humanRequired: event.target.checked }
                                  : entry,
                              ),
                            );
                            setDirty((value) => ({ ...value, plan: true }));
                          }}
                        />{" "}
                        人による確認
                      </label>
                      <label>
                        人の証跡
                        <select
                          value={item.humanEvidenceRequirement}
                          onChange={(event) => {
                            setItems((current) =>
                              current.map((entry, i) =>
                                i === index
                                  ? { ...entry, humanEvidenceRequirement: event.target.value }
                                  : entry,
                              ),
                            );
                            setDirty((value) => ({ ...value, plan: true }));
                          }}
                        >
                          <option value="none">不要</option>
                          <option value="text">テキスト</option>
                          <option value="image">画像</option>
                          <option value="text_or_image">テキストまたは画像</option>
                        </select>
                      </label>
                      <div className="preparation-actions">
                        <Button
                          variant="ghost"
                          size="sm"
                          disabled={index === 0}
                          onClick={() => {
                            setItems((current) => {
                              const next = [...current];
                              [next[index - 1], next[index]] = [next[index], next[index - 1]];
                              return next;
                            });
                            setDirty((value) => ({ ...value, plan: true }));
                          }}
                        >
                          上へ
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          disabled={index === items.length - 1}
                          onClick={() => {
                            setItems((current) => {
                              const next = [...current];
                              [next[index + 1], next[index]] = [next[index], next[index + 1]];
                              return next;
                            });
                            setDirty((value) => ({ ...value, plan: true }));
                          }}
                        >
                          下へ
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => {
                            setItems((current) => current.filter((_, i) => i !== index));
                            setDirty((value) => ({ ...value, plan: true }));
                          }}
                        >
                          削除
                        </Button>
                      </div>
                    </li>
                  ))}
                </ol>
                <div className="preparation-actions">
                  <Button
                    variant="outline"
                    onClick={() => {
                      setItems((current) => [
                        ...current,
                        {
                          clientKey: crypto.randomUUID(),
                          title: "",
                          instruction: "",
                          expectedResult: "",
                          aiRequired: false,
                          humanRequired: true,
                          humanEvidenceRequirement: "none",
                        },
                      ]);
                      setDirty((value) => ({ ...value, plan: true }));
                    }}
                  >
                    項目を追加
                  </Button>
                  <Button disabled={pending || !dirty.plan} onClick={() => props.onSavePlan(items)}>
                    チェック案を保存
                  </Button>
                </div>
              </>
            )}
          </section>
          <section className="preparation-start" aria-labelledby="start-title">
            <div>
              <h2 id="start-title">動作チェックを開始</h2>
              <p>準備内容を確認して次の工程へ進みます。</p>
            </div>
            {snapshot.readiness.blockingReasons.length > 0 && (
              <ul>
                {snapshot.readiness.blockingReasons.map((reason) => (
                  <li key={`${reason.code}-${reason.checkId ?? ""}`}>{reason.message}</li>
                ))}
              </ul>
            )}
            <Button
              disabled={!snapshot.readiness.canStartExecution || pending}
              onClick={props.onStart}
            >
              動作チェックを開始 <ChevronRight size={17} />
            </Button>
          </section>
        </div>
      </div>
    </main>
  );
}
