import {
  type Component,
  createResource,
  createSignal,
  createEffect,
  Show,
  For,
  Suspense,
  ErrorBoundary,
} from "solid-js";
import { createStore } from "solid-js/store";
import {
  ClipboardList,
  Play,
  X,
  Loader2,
  AlertCircle,
  CheckCircle2,
  Clock,
} from "lucide-solid";
import { api } from "../../shared/api/client";
import { useWS, useWSEvent } from "../../shared/ws/WSProvider";
import Button from "../../shared/ui/Button";
import { showToast } from "../../shared/ui/Toast";
import { t } from "../../shared/i18n/i18n";

// --- Types ---

interface Task {
  id: number;
  taskType: string;
  status: string;
  progress: number;
  total: number;
  message?: string;
  error?: string;
  createdAt: string;
  startedAt?: string;
  completedAt?: string;
}

// --- API ---

async function fetchTasks(): Promise<Task[]> {
  return api<Task[]>("/tasks");
}

async function runTask(type: string): Promise<{ taskId: number }> {
  return api<{ taskId: number }>(`/tasks/${type}/run`, { method: "POST" });
}

async function cancelTask(id: number): Promise<void> {
  await api(`/tasks/${id}`, { method: "DELETE" });
}

// --- Helpers ---

function formatDate(iso: string | undefined): string {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

function statusIcon(status: string) {
  switch (status) {
    case "PENDING":
      return <Clock class="h-4 w-4 text-slate-400" />;
    case "RUNNING":
      return <Loader2 class="h-4 w-4 animate-spin text-indigo-400" />;
    case "COMPLETED":
      return <CheckCircle2 class="h-4 w-4 text-emerald-400" />;
    case "FAILED":
      return <AlertCircle class="h-4 w-4 text-red-400" />;
    case "CANCELLED":
      return <X class="h-4 w-4 text-slate-400" />;
    default:
      return <Clock class="h-4 w-4 text-slate-400" />;
  }
}

function statusClass(status: string): string {
  switch (status) {
    case "PENDING":
      return "bg-slate-700 text-slate-300";
    case "RUNNING":
      return "bg-indigo-600/20 text-indigo-300";
    case "COMPLETED":
      return "bg-emerald-600/20 text-emerald-300";
    case "FAILED":
      return "bg-red-600/20 text-red-300";
    case "CANCELLED":
      return "bg-slate-700 text-slate-400";
    default:
      return "bg-slate-700 text-slate-300";
  }
}

// --- Main ---

const TaskMonitor: Component = () => {
  const [tasks, { refetch }] = createResource(fetchTasks);
  const [running, setRunning] = createStore<Record<string, boolean>>({});
  const [cancelling, setCancelling] = createStore<Record<number, boolean>>({});

  // WebSocket events
  useWSEvent("TASK_PROGRESS", () => {
    void refetch();
  });
  useWSEvent("TASK_COMPLETE", (msg) => {
    void refetch();
    const payload = msg.payload as { taskId?: number };
    if (payload.taskId) {
      showToast(t("admin.taskCompleted"), "success");
    }
  });
  useWSEvent("TASK_FAILED", (msg) => {
    void refetch();
    const payload = msg.payload as { taskId?: number; error?: string };
    if (payload.taskId) {
      showToast(
        payload.error ?? t("admin.taskFailed"),
        "error"
      );
    }
  });

  // Auto-refresh every 5 seconds as fallback.
  createEffect(() => {
    const id = setInterval(() => {
      void refetch();
    }, 5000);
    return () => clearInterval(id);
  });

  async function handleRun(type: string) {
    setRunning(type, true);
    try {
      await runTask(type);
      showToast(t("admin.taskEnqueued"), "success");
      void refetch();
    } catch (err: unknown) {
      showToast(err instanceof Error ? err.message : t("admin.failedToRunTask"), "error");
    } finally {
      setRunning(type, false);
    }
  }

  async function handleCancel(id: number) {
    setCancelling(id, true);
    try {
      await cancelTask(id);
      showToast(t("admin.taskCancelled"), "success");
      void refetch();
    } catch (err: unknown) {
      showToast(err instanceof Error ? err.message : t("admin.failedToCancelTask"), "error");
    } finally {
      setCancelling(id, false);
    }
  }

  const taskTypes = ["LIBRARY_SCAN", "METADATA_UPDATE", "COVER_EXTRACT", "DUPLICATE_CHECK", "VECTOR_REBUILD"];

  return (
    <div class="flex flex-1 flex-col">
      {/* Page header */}
      <div class="flex items-center justify-between border-b border-slate-800 px-6 py-5">
        <div>
          <h1 class="text-xl font-bold text-slate-100">{t("admin.taskMonitor")}</h1>
          <p class="mt-1 text-sm text-slate-400">{t("admin.monitorBackgroundTasks")}</p>
        </div>
        <div class="flex flex-wrap gap-2">
          <For each={taskTypes}>
            {(type) => (
              <Button
                size="sm"
                variant="secondary"
                loading={running[type]}
                onClick={() => handleRun(type)}
              >
                <Play class="h-3.5 w-3.5" />
                {type}
              </Button>
            )}
          </For>
        </div>
      </div>

      {/* Content */}
      <div class="flex-1 p-6">
        <ErrorBoundary
          fallback={(err) => (
            <div class="flex flex-col items-center justify-center gap-3 py-20 text-center">
              <p class="text-lg font-medium text-red-400">{t("admin.failedToLoadTasks")}</p>
              <p class="text-sm text-slate-500">{err.message}</p>
            </div>
          )}
        >
          <Suspense fallback={<p class="text-slate-400">{t("common.loading")}</p>}>
            <Show when={!tasks.loading} fallback={<p class="text-slate-400">{t("common.loading")}</p>}>
              <Show
                when={(tasks() ?? []).length > 0}
                fallback={
                  <div class="flex flex-col items-center justify-center gap-4 py-20 text-center">
                    <ClipboardList class="h-16 w-16 text-slate-600" />
                    <p class="text-lg font-medium text-slate-300">{t("admin.noTasksYet")}</p>
                  </div>
                }
              >
                <div class="overflow-x-auto rounded-xl border border-slate-700">
                  <table class="w-full text-sm">
                    <thead>
                      <tr class="border-b border-slate-700 bg-slate-800/50">
                        <th class="px-4 py-3 text-left font-medium text-slate-400">{t("admin.id")}</th>
                        <th class="px-4 py-3 text-left font-medium text-slate-400">{t("admin.type")}</th>
                        <th class="px-4 py-3 text-left font-medium text-slate-400">{t("admin.status")}</th>
                        <th class="px-4 py-3 text-left font-medium text-slate-400">{t("admin.progress")}</th>
                        <th class="px-4 py-3 text-left font-medium text-slate-400">{t("admin.started")}</th>
                        <th class="px-4 py-3 text-left font-medium text-slate-400">{t("admin.completed")}</th>
                        <th class="px-4 py-3 text-right font-medium text-slate-400">{t("admin.actions")}</th>
                      </tr>
                    </thead>
                    <tbody>
                      <For each={tasks()}>
                        {(task) => (
                          <tr class="border-b border-slate-800 hover:bg-slate-800/30 transition-colors">
                            <td class="px-4 py-3 text-slate-300">#{task.id}</td>
                            <td class="px-4 py-3 text-slate-200 font-medium">{task.taskType}</td>
                            <td class="px-4 py-3">
                              <span class={`inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium ${statusClass(task.status)}`}>
                                {statusIcon(task.status)}
                                {task.status}
                              </span>
                            </td>
                            <td class="px-4 py-3 text-slate-300">
                              <Show when={task.total > 0} fallback={"—"}>
                                <div class="flex items-center gap-2">
                                  <div class="h-2 w-20 overflow-hidden rounded-full bg-slate-700">
                                    <div
                                      class="h-full bg-indigo-500 transition-all"
                                      style={{ width: `${Math.min(100, Math.round((task.progress / task.total) * 100))}%` }}
                                    />
                                  </div>
                                  <span class="text-xs text-slate-400">
                                    {task.progress}/{task.total}
                                  </span>
                                </div>
                              </Show>
                            </td>
                            <td class="px-4 py-3 text-slate-300 whitespace-nowrap">
                              {formatDate(task.startedAt)}
                            </td>
                            <td class="px-4 py-3 text-slate-300 whitespace-nowrap">
                              {formatDate(task.completedAt)}
                            </td>
                            <td class="px-4 py-3">
                              <Show when={task.status === "RUNNING" || task.status === "PENDING"}>
                                <div class="flex items-center justify-end">
                                  <Button
                                    variant="danger"
                                    size="sm"
                                    loading={cancelling[task.id]}
                                    onClick={() => handleCancel(task.id)}
                                  >
                                    <X class="h-3.5 w-3.5" />
                                    {t("common.cancel")}
                                  </Button>
                                </div>
                              </Show>
                            </td>
                          </tr>
                        )}
                      </For>
                    </tbody>
                  </table>
                </div>
              </Show>
            </Show>
          </Suspense>
        </ErrorBoundary>
      </div>
    </div>
  );
};

export default TaskMonitor;
