import { AlertCircle, Bot, CheckCircle2, FileText, FolderOpen, UserRound } from "lucide-react";

export const labels: Record<string, string> = {
  preparing: "準備中",
  ai_running: "AI実行中",
  human_waiting: "人間の確認待ち",
  procedure_editing: "手順書編集中",
  completed: "完成",
  error: "エラー",
};

export const statusIcons = {
  preparing: FolderOpen,
  ai_running: Bot,
  human_waiting: UserRound,
  procedure_editing: FileText,
  completed: CheckCircle2,
  error: AlertCircle,
};

export function relativeTime(value: string, now = Date.now()) {
  const elapsed = Math.max(0, now - new Date(value).getTime());
  if (!Number.isFinite(elapsed)) return "日時不明";
  const minutes = Math.floor(elapsed / 60_000);
  if (minutes < 1) return "たった今";
  if (minutes < 60) return `${minutes}分前`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}時間前`;
  const days = Math.floor(hours / 24);
  return `${days}日前`;
}
