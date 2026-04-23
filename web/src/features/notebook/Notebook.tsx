import {
  type Component,
  createSignal,
  createResource,
  createMemo,
  For,
  Show,
  Suspense,
  ErrorBoundary,
} from "solid-js";
import { useNavigate } from "@solidjs/router";
import { BookOpen, Trash2, Search, Filter } from "lucide-solid";
import { api } from "../../shared/api/client";
import { t } from "../../shared/i18n/i18n";

// ---- Types ----

interface Annotation {
  id: number;
  userId: number;
  bookId: number;
  bookFileId: number | null;
  type: string;
  cfi: string | null;
  pageNumber: number | null;
  text: string | null;
  note: string | null;
  color: string;
  createdAt: string;
  updatedAt: string;
  bookTitle?: string | null;
  coverPath?: string | null;
}

interface NotebookResponse {
  annotations: Annotation[];
  total: number;
  limit: number;
  offset: number;
}

// ---- Color helpers ----

const colorClasses: Record<string, string> = {
  yellow: "bg-yellow-400/20 border-yellow-400/50 text-yellow-300",
  green: "bg-green-400/20 border-green-400/50 text-green-300",
  blue: "bg-blue-400/20 border-blue-400/50 text-blue-300",
  pink: "bg-pink-400/20 border-pink-400/50 text-pink-300",
  purple: "bg-purple-400/20 border-purple-400/50 text-purple-300",
};

const colorDotClasses: Record<string, string> = {
  yellow: "bg-yellow-400",
  green: "bg-green-400",
  blue: "bg-blue-400",
  pink: "bg-pink-400",
  purple: "bg-purple-400",
};

function colorClass(color: string): string {
  return colorClasses[color] ?? colorClasses.yellow;
}

function colorDotClass(color: string): string {
  return colorDotClasses[color] ?? colorDotClasses.yellow;
}

// ---- API helpers ----

async function fetchNotebook(page: number): Promise<NotebookResponse> {
  return api<NotebookResponse>(`/notebook?page=${page}&limit=50`);
}

async function deleteAnnotation(id: number, bookId: number): Promise<void> {
  await api(`/reader/books/${bookId}/annotations/${id}`, { method: "DELETE" });
}

// ---- Notebook component ----

const Notebook: Component = () => {
  const navigate = useNavigate();
  const [page, setPage] = createSignal(1);
  const [search, setSearch] = createSignal("");
  const [filterColor, setFilterColor] = createSignal<string | null>(null);
  const [filterBookId, setFilterBookId] = createSignal<number | null>(null);

  const [data, { refetch }] = createResource(page, fetchNotebook);

  // Derive unique books from loaded annotations for the filter dropdown.
  const books = createMemo(() => {
    const annotations = data()?.annotations ?? [];
    const seen = new Map<number, string>();
    for (const a of annotations) {
      if (!seen.has(a.bookId)) {
        seen.set(a.bookId, a.bookTitle ?? `Book ${a.bookId}`);
      }
    }
    return Array.from(seen.entries()).map(([id, title]) => ({ id, title }));
  });

  // Filter annotations client-side by search text, color, and book.
  const filtered = createMemo(() => {
    const annotations = data()?.annotations ?? [];
    const q = search().toLowerCase();
    const color = filterColor();
    const bookId = filterBookId();

    return annotations.filter((a) => {
      if (color && a.color !== color) return false;
      if (bookId !== null && a.bookId !== bookId) return false;
      if (q) {
        const inText = a.text?.toLowerCase().includes(q) ?? false;
        const inNote = a.note?.toLowerCase().includes(q) ?? false;
        const inTitle = a.bookTitle?.toLowerCase().includes(q) ?? false;
        if (!inText && !inNote && !inTitle) return false;
      }
      return true;
    });
  });

  // Group filtered annotations by book.
  const grouped = createMemo(() => {
    const groups = new Map<number, { title: string; coverPath: string | null; annotations: Annotation[] }>();
    for (const a of filtered()) {
      if (!groups.has(a.bookId)) {
        groups.set(a.bookId, {
          title: a.bookTitle ?? `Book ${a.bookId}`,
          coverPath: a.coverPath ?? null,
          annotations: [],
        });
      }
      groups.get(a.bookId)!.annotations.push(a);
    }
    return Array.from(groups.entries()).map(([bookId, group]) => ({ bookId, ...group }));
  });

  async function handleDelete(annotation: Annotation) {
    try {
      await deleteAnnotation(annotation.id, annotation.bookId);
      refetch();
    } catch {
      // Non-fatal.
    }
  }

  function formatDate(iso: string): string {
    try {
      return new Date(iso).toLocaleDateString(undefined, {
        year: "numeric",
        month: "short",
        day: "numeric",
      });
    } catch {
      return iso;
    }
  }

  return (
    <div class="flex flex-1 flex-col overflow-hidden">
      {/* Header */}
      <div class="border-b border-slate-800 px-6 py-5">
        <h1 class="text-xl font-bold text-slate-100">{t("notebook.notebookTitle")}</h1>
        <p class="mt-1 text-sm text-slate-400">{t("notebook.notebookSubtitle")}</p>
      </div>

      {/* Filters */}
      <div class="flex flex-wrap items-center gap-3 border-b border-slate-800 px-6 py-3">
        {/* Search */}
        <div class="relative flex-1 min-w-48">
          <Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500" />
          <input
            type="text"
            placeholder={t("notebook.searchAnnotations")}
            value={search()}
            onInput={(e) => setSearch(e.currentTarget.value)}
            class="w-full rounded-lg border border-slate-700 bg-slate-800 py-2 pl-9 pr-3 text-sm text-slate-200 placeholder-slate-500 focus:border-indigo-500 focus:outline-none"
          />
        </div>

        {/* Color filter */}
        <div class="flex items-center gap-1">
          <Filter class="h-4 w-4 text-slate-500" />
          <For each={["yellow", "green", "blue", "pink", "purple"]}>
            {(color) => (
              <button
                onClick={() => setFilterColor(filterColor() === color ? null : color)}
                class={`h-5 w-5 rounded-full transition-all ${colorDotClass(color)} ${
                  filterColor() === color ? "ring-2 ring-white ring-offset-1 ring-offset-slate-900" : "opacity-60 hover:opacity-100"
                }`}
                title={color}
              />
            )}
          </For>
        </div>

        {/* Book filter */}
        <Show when={books().length > 0}>
          <select
            value={filterBookId() ?? ""}
            onChange={(e) => {
              const v = e.currentTarget.value;
              setFilterBookId(v === "" ? null : Number(v));
            }}
            class="rounded-lg border border-slate-700 bg-slate-800 py-2 px-3 text-sm text-slate-200 focus:border-indigo-500 focus:outline-none"
          >
            <option value="">{t("notebook.allBooks")}</option>
            <For each={books()}>
              {(book) => <option value={book.id}>{book.title}</option>}
            </For>
          </select>
        </Show>
      </div>

      {/* Content */}
      <div class="flex-1 overflow-y-auto px-6 py-6">
        <ErrorBoundary
          fallback={(err) => (
            <div class="flex items-center justify-center py-16">
              <p class="text-red-400">{t("notebook.failedToLoadAnnotations")}: {err.message}</p>
            </div>
          )}
        >
          <Suspense
            fallback={
              <div class="flex items-center justify-center py-16">
                <BookOpen class="h-8 w-8 animate-pulse text-indigo-400" />
              </div>
            }
          >
            <Show
              when={grouped().length > 0}
              fallback={
                <div class="flex flex-col items-center justify-center py-16 text-center">
                  <BookOpen class="mb-4 h-12 w-12 text-slate-600" />
                  <p class="text-slate-400">{t("notebook.noAnnotationsYet")}</p>
                  <p class="mt-1 text-sm text-slate-500">
                    {t("notebook.highlightTextWhileReading")}
                  </p>
                </div>
              }
            >
              <div class="flex flex-col gap-8">
                <For each={grouped()}>
                  {(group) => (
                    <div>
                      {/* Book header */}
                      <button
                        onClick={() => navigate(`/books/${group.bookId}`)}
                        class="mb-3 flex items-center gap-3 hover:opacity-80 transition-opacity"
                      >
                        <Show
                          when={group.coverPath}
                          fallback={
                            <div class="flex h-12 w-8 shrink-0 items-center justify-center rounded bg-slate-700">
                              <BookOpen class="h-4 w-4 text-slate-500" />
                            </div>
                          }
                        >
                          <img
                            src={`/api/books/${group.bookId}/cover`}
                            alt=""
                            class="h-12 w-8 shrink-0 rounded object-cover"
                          />
                        </Show>
                        <div class="text-left">
                          <p class="font-semibold text-slate-200">{group.title}</p>
                          <p class="text-xs text-slate-500">
                            {group.annotations.length}{" "}
                            {group.annotations.length === 1 ? t("common.annotation") : t("common.annotations")}
                          </p>
                        </div>
                      </button>

                      {/* Annotations */}
                      <div class="flex flex-col gap-2 pl-11">
                        <For each={group.annotations}>
                          {(annotation) => (
                            <div
                              class={`rounded-lg border p-3 ${colorClass(annotation.color)}`}
                            >
                              <div class="flex items-start justify-between gap-2">
                                <div class="flex-1 min-w-0">
                                  <Show when={annotation.text}>
                                    <p class="text-sm leading-relaxed">
                                      "{annotation.text}"
                                    </p>
                                  </Show>
                                  <Show when={annotation.note}>
                                    <p class="mt-1 text-xs text-slate-400 italic">
                                      {annotation.note}
                                    </p>
                                  </Show>
                                  <p class="mt-1 text-xs text-slate-500">
                                    {formatDate(annotation.createdAt)}
                                  </p>
                                </div>
                                <button
                                  onClick={() => handleDelete(annotation)}
                                  class="shrink-0 rounded p-1 text-slate-500 hover:bg-white/10 hover:text-red-400 transition-colors"
                                  title={t("notebook.deleteAnnotation")}
                                >
                                  <Trash2 class="h-3.5 w-3.5" />
                                </button>
                              </div>
                            </div>
                          )}
                        </For>
                      </div>
                    </div>
                  )}
                </For>
              </div>
            </Show>

            {/* Pagination */}
            <Show when={(data()?.total ?? 0) > (data()?.limit ?? 50)}>
              <div class="mt-8 flex items-center justify-center gap-3">
                <button
                  onClick={() => setPage((p) => Math.max(1, p - 1))}
                  disabled={page() === 1}
                  class="rounded-lg px-4 py-2 text-sm text-slate-300 hover:bg-slate-800 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                >
                  {t("notebook.previous")}
                </button>
                <span class="text-sm text-slate-500">{t("notebook.page")} {page()}</span>
                <button
                  onClick={() => setPage((p) => p + 1)}
                  disabled={(data()?.offset ?? 0) + (data()?.limit ?? 50) >= (data()?.total ?? 0)}
                  class="rounded-lg px-4 py-2 text-sm text-slate-300 hover:bg-slate-800 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                >
                  {t("notebook.next")}
                </button>
              </div>
            </Show>
          </Suspense>
        </ErrorBoundary>
      </div>
    </div>
  );
};

export default Notebook;
