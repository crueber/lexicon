import {
  type Component,
  createResource,
  createMemo,
  Show,
  For,
  Suspense,
  ErrorBoundary,
  createSignal,
} from "solid-js";
import { useParams, useNavigate } from "@solidjs/router";
import { BookMarked, Pencil, Trash2, Wand2 } from "lucide-solid";
import { api } from "../../shared/api/client";
import { useAuth } from "../auth/AuthProvider";
import Button from "../../shared/ui/Button";
import Skeleton from "../../shared/ui/Skeleton";
import type { MagicShelf, MagicShelfBook, RuleGroup, RuleItem } from "../library/types";

// ---- API ----

async function fetchMagicShelf(id: number): Promise<MagicShelf> {
  return api<MagicShelf>(`/magic-shelves/${id}`);
}

async function fetchMagicShelfBooks(id: number): Promise<MagicShelfBook[]> {
  return api<MagicShelfBook[]>(`/magic-shelves/${id}/books`);
}

async function deleteMagicShelf(id: number): Promise<void> {
  await api(`/magic-shelves/${id}`, { method: "DELETE" });
}

// ---- Rule summary ----

function fieldLabel(field: string): string {
  const labels: Record<string, string> = {
    title: "Title",
    author: "Author",
    category: "Category",
    tag: "Tag",
    series: "Series",
    language: "Language",
    book_type: "Book Type",
    format: "Format",
    publisher: "Publisher",
    added_date: "Added Date",
    page_count: "Page Count",
  };
  return labels[field] ?? field;
}

function operatorLabel(op: string): string {
  const labels: Record<string, string> = {
    contains: "contains",
    equals: "equals",
    starts_with: "starts with",
    ends_with: "ends with",
    greater_than: "greater than",
    less_than: "less than",
    is_empty: "is empty",
    is_not_empty: "is not empty",
  };
  return labels[op] ?? op;
}

function ruleItemSummary(item: RuleItem): string {
  if (item.type === "group" && item.group) {
    return `(${ruleGroupSummary(item.group)})`;
  }
  const field = fieldLabel(item.field ?? "");
  const op = operatorLabel(item.operator ?? "");
  const val = item.value ? ` "${item.value}"` : "";
  return `${field} ${op}${val}`;
}

function ruleGroupSummary(group: RuleGroup): string {
  if (group.rules.length === 0) return "No rules";
  const parts = group.rules.map(ruleItemSummary);
  return parts.join(` ${group.operator} `);
}

// ---- Sub-components ----

const MagicBookCard: Component<{
  book: MagicShelfBook;
  onClick: () => void;
}> = (props) => {
  const [imgError, setImgError] = createSignal(false);
  const isAudiobook = () => props.book.bookType === "AUDIOBOOK";

  return (
    <button
      onClick={props.onClick}
      class="group flex flex-col gap-2 rounded-lg bg-slate-800 p-2 text-left focus-visible:outline-none"
    >
      <div
        class={`relative w-full overflow-hidden rounded-md bg-slate-700 ${
          isAudiobook() ? "aspect-square" : "aspect-[2/3]"
        }`}
      >
        <Show
          when={props.book.id && !imgError()}
          fallback={
            <div class="flex h-full w-full items-center justify-center bg-slate-700">
              <BookMarked class="h-10 w-10 text-slate-500" />
            </div>
          }
        >
          <img
            src={`/api/books/${props.book.id}/cover/thumbnail`}
            alt={props.book.title ?? "Book cover"}
            loading="lazy"
            class="h-full w-full object-cover transition-transform duration-200 group-hover:scale-105"
            onError={() => setImgError(true)}
          />
        </Show>
      </div>
      <div class="min-w-0 flex-1">
        <p class="line-clamp-2 text-xs font-medium leading-tight text-slate-100">
          {props.book.title ?? "Untitled"}
        </p>
      </div>
    </button>
  );
};

// ---- Skeleton ----

const MagicShelfDetailSkeleton: Component = () => (
  <div class="flex flex-1 flex-col">
    <div class="border-b border-slate-800 px-6 py-5">
      <Skeleton class="h-7 w-48 rounded" />
      <Skeleton class="mt-2 h-4 w-64 rounded" />
    </div>
    <div class="p-6">
      <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
        <For each={Array.from({ length: 12 })}>
          {() => (
            <div class="flex flex-col gap-2 rounded-lg bg-slate-800 p-2">
              <Skeleton class="aspect-[2/3] w-full rounded-md" />
              <Skeleton class="h-3 w-full rounded" />
            </div>
          )}
        </For>
      </div>
    </div>
  </div>
);

// ---- Inner component ----

const MagicShelfDetailInner: Component<{ shelfId: number }> = (props) => {
  const navigate = useNavigate();
  const auth = useAuth();

  const [shelf, { refetch: refetchShelf }] = createResource(
    () => props.shelfId,
    fetchMagicShelf
  );
  const [books] = createResource(() => props.shelfId, fetchMagicShelfBooks);

  const [showDeleteConfirm, setShowDeleteConfirm] = createSignal(false);
  const [deleting, setDeleting] = createSignal(false);

  const isOwner = createMemo(() => {
    const s = shelf();
    if (!s) return false;
    return s.userId === auth.user()?.id;
  });

  const ruleSummary = createMemo(() => {
    const s = shelf();
    if (!s) return "";
    try {
      const group = JSON.parse(s.rules) as RuleGroup;
      return ruleGroupSummary(group);
    } catch {
      return "Invalid rules";
    }
  });

  async function handleDelete() {
    setDeleting(true);
    try {
      await deleteMagicShelf(props.shelfId);
      navigate("/shelves");
    } catch {
      setDeleting(false);
      setShowDeleteConfirm(false);
    }
  }

  // Suppress unused refetchShelf warning — available for future use.
  void refetchShelf;

  return (
    <Show when={shelf()} fallback={<MagicShelfDetailSkeleton />}>
      {(s) => (
        <div class="flex flex-1 flex-col">
          {/* Header */}
          <div class="flex items-start justify-between border-b border-slate-800 px-6 py-5">
            <div class="flex items-center gap-4">
              <button
                onClick={() => navigate("/shelves")}
                class="text-sm text-slate-400 hover:text-slate-200 transition-colors"
              >
                ← Shelves
              </button>
              <div class="flex items-center gap-3">
                <Show
                  when={s().icon}
                  fallback={<Wand2 class="h-6 w-6 text-indigo-400" />}
                >
                  <span class="text-2xl">{s().icon}</span>
                </Show>
                <div>
                  <div class="flex items-center gap-2">
                    <h1 class="text-xl font-bold text-slate-100">{s().name}</h1>
                    <span class="rounded-full bg-indigo-600/20 px-2 py-0.5 text-xs font-medium text-indigo-400">
                      Magic
                    </span>
                  </div>
                  <Show when={s().description}>
                    <p class="mt-0.5 text-sm text-slate-400">{s().description}</p>
                  </Show>
                  <p class="mt-1 text-xs text-slate-500 line-clamp-1">
                    {ruleSummary()}
                  </p>
                </div>
              </div>
            </div>

            {/* Owner actions */}
            <Show when={isOwner()}>
              <div class="flex items-center gap-2">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => navigate(`/magic-shelves/${props.shelfId}/edit`)}
                >
                  <Pencil class="h-4 w-4" />
                  Edit Rules
                </Button>
                <Show
                  when={!showDeleteConfirm()}
                  fallback={
                    <div class="flex items-center gap-2">
                      <span class="text-sm text-slate-400">Are you sure?</span>
                      <Button
                        variant="danger"
                        size="sm"
                        onClick={handleDelete}
                        loading={deleting()}
                      >
                        Delete
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => setShowDeleteConfirm(false)}
                      >
                        Cancel
                      </Button>
                    </div>
                  }
                >
                  <Button
                    variant="danger"
                    size="sm"
                    onClick={() => setShowDeleteConfirm(true)}
                  >
                    <Trash2 class="h-4 w-4" />
                    Delete
                  </Button>
                </Show>
              </div>
            </Show>
          </div>

          {/* Book grid */}
          <div class="flex-1 p-6">
            <Show
              when={!books.loading}
              fallback={
                <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
                  <For each={Array.from({ length: 12 })}>
                    {() => (
                      <div class="flex flex-col gap-2 rounded-lg bg-slate-800 p-2">
                        <Skeleton class="aspect-[2/3] w-full rounded-md" />
                        <Skeleton class="h-3 w-full rounded" />
                      </div>
                    )}
                  </For>
                </div>
              }
            >
              <Show
                when={(books() ?? []).length > 0}
                fallback={
                  <div class="flex flex-col items-center justify-center gap-4 py-20 text-center">
                    <Wand2 class="h-12 w-12 text-slate-600" />
                    <div>
                      <p class="text-lg font-medium text-slate-300">
                        No books match these rules
                      </p>
                      <p class="mt-1 text-sm text-slate-500">
                        Try adjusting the rules to find matching books.
                      </p>
                    </div>
                    <Button
                      variant="ghost"
                      onClick={() =>
                        navigate(`/magic-shelves/${props.shelfId}/edit`)
                      }
                    >
                      <Pencil class="h-4 w-4" />
                      Edit Rules
                    </Button>
                  </div>
                }
              >
                <div class="mb-4 text-sm text-slate-400">
                  {(books() ?? []).length}{" "}
                  {(books() ?? []).length === 1 ? "book" : "books"} matched
                </div>
                <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
                  <For each={books() ?? []}>
                    {(book) => (
                      <MagicBookCard
                        book={book}
                        onClick={() => navigate(`/books/${book.id}`)}
                      />
                    )}
                  </For>
                </div>
              </Show>
            </Show>
          </div>
        </div>
      )}
    </Show>
  );
};

// ---- Page wrapper ----

const MagicShelfDetail: Component = () => {
  const params = useParams<{ id: string }>();
  const shelfId = createMemo(() => parseInt(params.id, 10));

  return (
    <ErrorBoundary
      fallback={(err) => (
        <div class="flex flex-1 flex-col items-center justify-center gap-3 p-8 text-center">
          <p class="text-lg font-medium text-red-400">
            {err.message.includes("404")
              ? "Magic shelf not found"
              : "Failed to load magic shelf"}
          </p>
          <p class="text-sm text-slate-500">{err.message}</p>
        </div>
      )}
    >
      <Suspense fallback={<MagicShelfDetailSkeleton />}>
        <MagicShelfDetailInner shelfId={shelfId()} />
      </Suspense>
    </ErrorBoundary>
  );
};

export default MagicShelfDetail;
