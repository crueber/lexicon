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
  Inbox,
  Check,
  X,
  Import,
  Trash2,
} from "lucide-solid";
import { api } from "../../shared/api/client";
import { useWSEvent } from "../../shared/ws/WSProvider";
import Button from "../../shared/ui/Button";
import { showToast } from "../../shared/ui/Toast";
import { t } from "../../shared/i18n/i18n";

// --- Types ---

interface BookdropFile {
  id: number;
  originalFilename: string;
  fileSize: number;
  status: string;
  extractedTitle?: string;
  extractedAuthors?: string;
  extractedCoverPath?: string;
  createdAt: string;
}

// --- API ---

async function fetchBookdropFiles(): Promise<BookdropFile[]> {
  return api<BookdropFile[]>("/bookdrop/files");
}

async function importFile(id: number): Promise<{ bookId: number }> {
  return api<{ bookId: number }>(`/bookdrop/files/${id}/import`, { method: "POST" });
}

async function rejectFile(id: number): Promise<void> {
  await api(`/bookdrop/files/${id}/reject`, { method: "POST" });
}

async function importAll(): Promise<{ imported: number; total: number; errors: number }> {
  return api<{ imported: number; total: number; errors: number }>("/bookdrop/files/import-all", { method: "POST" });
}

// --- Helpers ---

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

// --- Main ---

const BookDropQueue: Component = () => {
  const [files, { refetch }] = createResource(fetchBookdropFiles);
  const [importing, setImporting] = createStore<Record<number, boolean>>({});
  const [rejecting, setRejecting] = createStore<Record<number, boolean>>({});
  const [bulkImporting, setBulkImporting] = createSignal(false);

  // WebSocket event
  useWSEvent("BOOKDROP_FILE_ARRIVED", () => {
    void refetch();
    showToast(t("admin.newBookdropFile"), "info");
  });

  async function handleImport(id: number) {
    setImporting(id, true);
    try {
      await importFile(id);
      showToast(t("admin.fileImported"), "success");
      void refetch();
    } catch (err: unknown) {
      showToast(err instanceof Error ? err.message : t("admin.failedToImportFile"), "error");
    } finally {
      setImporting(id, false);
    }
  }

  async function handleReject(id: number) {
    setRejecting(id, true);
    try {
      await rejectFile(id);
      showToast(t("admin.fileRejected"), "success");
      void refetch();
    } catch (err: unknown) {
      showToast(err instanceof Error ? err.message : t("admin.failedToRejectFile"), "error");
    } finally {
      setRejecting(id, false);
    }
  }

  async function handleImportAll() {
    setBulkImporting(true);
    try {
      const result = await importAll();
      showToast(
        `${result.imported} of ${result.total} files imported`,
        result.errors === 0 ? "success" : "info"
      );
      void refetch();
    } catch (err: unknown) {
      showToast(err instanceof Error ? err.message : t("admin.failedToImportAll"), "error");
    } finally {
      setBulkImporting(false);
    }
  }

  return (
    <div class="flex flex-1 flex-col">
      {/* Page header */}
      <div class="flex items-center justify-between border-b border-slate-800 px-6 py-5">
        <div>
          <h1 class="text-xl font-bold text-slate-100">{t("admin.bookdrop")}</h1>
          <p class="mt-1 text-sm text-slate-400">{t("admin.reviewAndImportFiles")}</p>
        </div>
        <Button onClick={handleImportAll} loading={bulkImporting()}>
          <Import class="h-4 w-4" />
          {t("admin.importAll")}
        </Button>
      </div>

      {/* Content */}
      <div class="flex-1 p-6">
        <ErrorBoundary
          fallback={(err) => (
            <div class="flex flex-col items-center justify-center gap-3 py-20 text-center">
              <p class="text-lg font-medium text-red-400">{t("admin.failedToLoadBookdrop")}</p>
              <p class="text-sm text-slate-500">{err.message}</p>
            </div>
          )}
        >
          <Suspense fallback={<p class="text-slate-400">{t("common.loading")}</p>}>
            <Show when={!files.loading} fallback={<p class="text-slate-400">{t("common.loading")}</p>}>
              <Show
                when={(files() ?? []).length > 0}
                fallback={
                  <div class="flex flex-col items-center justify-center gap-4 py-20 text-center">
                    <Inbox class="h-16 w-16 text-slate-600" />
                    <p class="text-lg font-medium text-slate-300">{t("admin.noBookdropFiles")}</p>
                    <p class="text-sm text-slate-500">{t("admin.dropFilesHere")}</p>
                  </div>
                }
              >
                <div class="overflow-x-auto rounded-xl border border-slate-700">
                  <table class="w-full text-sm">
                    <thead>
                      <tr class="border-b border-slate-700 bg-slate-800/50">
                        <th class="px-4 py-3 text-left font-medium text-slate-400">{t("admin.filename")}</th>
                        <th class="px-4 py-3 text-left font-medium text-slate-400">{t("admin.title")}</th>
                        <th class="px-4 py-3 text-left font-medium text-slate-400">{t("admin.authors")}</th>
                        <th class="px-4 py-3 text-left font-medium text-slate-400">{t("admin.size")}</th>
                        <th class="px-4 py-3 text-right font-medium text-slate-400">{t("admin.actions")}</th>
                      </tr>
                    </thead>
                    <tbody>
                      <For each={files()}>
                        {(file) => (
                          <tr class="border-b border-slate-800 hover:bg-slate-800/30 transition-colors">
                            <td class="px-4 py-3 text-slate-200 font-medium">{file.originalFilename}</td>
                            <td class="px-4 py-3 text-slate-300">
                              {file.extractedTitle ?? <span class="text-slate-500">—</span>}
                            </td>
                            <td class="px-4 py-3 text-slate-300">
                              {file.extractedAuthors ?? <span class="text-slate-500">—</span>}
                            </td>
                            <td class="px-4 py-3 text-slate-300 whitespace-nowrap">
                              {formatFileSize(file.fileSize)}
                            </td>
                            <td class="px-4 py-3">
                              <div class="flex items-center justify-end gap-2">
                                <Button
                                  variant="primary"
                                  size="sm"
                                  loading={importing[file.id]}
                                  onClick={() => handleImport(file.id)}
                                >
                                  <Check class="h-3.5 w-3.5" />
                                  {t("common.import")}
                                </Button>
                                <Button
                                  variant="secondary"
                                  size="sm"
                                  loading={rejecting[file.id]}
                                  onClick={() => handleReject(file.id)}
                                >
                                  <X class="h-3.5 w-3.5" />
                                  {t("common.reject")}
                                </Button>
                              </div>
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

export default BookDropQueue;
