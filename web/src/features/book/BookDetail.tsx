import {
  type Component,
  createResource,
  createSignal,
  createMemo,
  Show,
  For,
  Suspense,
  ErrorBoundary,
} from "solid-js";
import { useParams, useNavigate } from "@solidjs/router";
import {
  BookOpen,
  Headphones,
  BookImage,
  ChevronDown,
  ChevronUp,
  Trash2,
  BookOpenCheck,
  BookMarked,
  Search,
  Mail,
  X,
  CheckCircle,
} from "lucide-solid";
import { api } from "../../shared/api/client";
import { useAuth } from "../auth/AuthProvider";
import Button from "../../shared/ui/Button";
import Skeleton from "../../shared/ui/Skeleton";
import { showToast } from "../../shared/ui/Toast";
import AddToShelfDialog from "../shelf/AddToShelfDialog";
import MetadataSearch from "./MetadataSearch";
import { t } from "../../shared/i18n/i18n";
import type { BookDetail as BookDetailType, BookFile, Shelf, SimilarBook } from "../library/types";

// ---- API ----

async function fetchBookDetail(id: number): Promise<BookDetailType> {
  return api<BookDetailType>(`/books/${id}`);
}

async function fetchBookShelves(id: number): Promise<Shelf[]> {
  return api<Shelf[]>(`/books/${id}/shelves`);
}

async function fetchSimilarBooks(id: number): Promise<SimilarBook[]> {
  return api<SimilarBook[]>(`/books/${id}/similar`);
}

interface EmailRecipient {
  id: number;
  name?: string;
  emailAddress: string;
}

async function fetchEmailRecipients(): Promise<EmailRecipient[]> {
  return api<EmailRecipient[]>("/email/recipients");
}

async function sendBookToDevice(bookId: number, recipientIds: number[]): Promise<void> {
  await api(`/books/${bookId}/send`, {
    method: "POST",
    body: JSON.stringify({ recipientIds, providerId: 0 }),
  });
}

// ---- Helpers ----

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024)
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

function formatDuration(secs: number): string {
  const h = Math.floor(secs / 3600);
  const m = Math.floor((secs % 3600) / 60);
  const s = secs % 60;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
}

// ---- Sub-components ----

const FormatBadge: Component<{ format: string }> = (props) => {
  const colorMap: Record<string, string> = {
    EPUB: "bg-emerald-700/80 text-emerald-100",
    PDF: "bg-red-700/80 text-red-100",
    CBZ: "bg-amber-700/80 text-amber-100",
    CBR: "bg-amber-700/80 text-amber-100",
    CB7: "bg-amber-700/80 text-amber-100",
    MP3: "bg-purple-700/80 text-purple-100",
    M4B: "bg-purple-700/80 text-purple-100",
    FLAC: "bg-purple-700/80 text-purple-100",
  };
  const cls = createMemo(
    () => colorMap[props.format] ?? "bg-slate-600/80 text-slate-100",
  );
  return (
    <span
      class={`inline-flex items-center rounded px-2 py-0.5 text-xs font-semibold uppercase tracking-wide ${cls()}`}
    >
      {props.format}
    </span>
  );
};

const CoverPlaceholder: Component<{
  bookType: "EBOOK" | "AUDIOBOOK" | "COMIC";
}> = (props) => (
  <div class="flex h-full w-full items-center justify-center bg-slate-700">
    <Show
      when={props.bookType === "AUDIOBOOK"}
      fallback={
        <Show
          when={props.bookType === "COMIC"}
          fallback={<BookOpen class="h-20 w-20 text-slate-500" />}
        >
          <BookImage class="h-20 w-20 text-slate-500" />
        </Show>
      }
    >
      <Headphones class="h-20 w-20 text-slate-500" />
    </Show>
  </div>
);

const MetaRow: Component<{ label: string; value: string }> = (props) => (
  <div class="flex flex-col gap-0.5">
    <span class="text-xs font-medium uppercase tracking-wide text-slate-500">
      {props.label}
    </span>
    <span class="text-sm text-slate-200">{props.value}</span>
  </div>
);

const FileRow: Component<{ file: BookFile }> = (props) => (
  <div class="flex items-center gap-3 rounded-lg bg-slate-800 px-4 py-3">
    <FormatBadge format={props.file.format} />
    <div class="min-w-0 flex-1">
      <Show when={props.file.trackTitle}>
        <p class="truncate text-sm text-slate-200">{props.file.trackTitle}</p>
      </Show>
      <Show when={!props.file.trackTitle}>
        <p class="truncate text-sm text-slate-400">{props.file.filePath}</p>
      </Show>
    </div>
    <div class="flex shrink-0 items-center gap-3 text-xs text-slate-500">
      <Show when={props.file.durationSecs !== undefined}>
        <span>{formatDuration(props.file.durationSecs!)}</span>
      </Show>
      <Show when={props.file.fileSize !== undefined}>
        <span>{formatFileSize(props.file.fileSize!)}</span>
      </Show>
    </div>
  </div>
);

const SimilarBooksSection: Component<{ bookId: number }> = (props) => {
  const navigate = useNavigate();
  const [similarBooks] = createResource(() => props.bookId, fetchSimilarBooks);

  return (
    <div class="flex flex-col gap-3">
      <span class="text-xs font-medium uppercase tracking-wide text-slate-500">
        {t("book.similarBooks")}
      </span>
      <Suspense fallback={<div class="h-24 animate-pulse rounded-lg bg-slate-800" />}>
        <Show
          when={(similarBooks() ?? []).length > 0}
          fallback={
            <p class="text-sm text-slate-500">
              {t("book.noRecommendations")}
            </p>
          }
        >
          <div class="flex gap-4 overflow-x-auto pb-2">
            <For each={similarBooks() ?? []}>
              {(similarBook) => (
                <button
                  onClick={() => navigate(`/books/${similarBook.id}`)}
                  class="flex w-24 shrink-0 flex-col gap-1 text-left"
                >
                  <div class="aspect-[2/3] w-full overflow-hidden rounded-lg bg-slate-700">
                    <Show
                      when={similarBook.coverPath}
                      fallback={
                        <CoverPlaceholder bookType={similarBook.bookType} />
                      }
                    >
                      <img
                        src={`/api/books/${similarBook.id}/cover`}
                        alt={similarBook.title ?? t("common.bookCover")}
                        class="h-full w-full object-cover"
                      />
                    </Show>
                  </div>
                  <p class="line-clamp-2 text-xs font-medium text-slate-200">
                    {similarBook.title ?? t("common.untitled")}
                  </p>
                  <p class="text-xs text-slate-500">
                    {Math.round(similarBook.similarity * 100)}{t("book.percentMatch")}
                  </p>
                </button>
              )}
            </For>
          </div>
        </Show>
      </Suspense>
    </div>
  );
};

// ---- Skeleton ----

const BookDetailSkeleton: Component = () => (
  <div class="flex flex-1 flex-col gap-6 p-6 md:flex-row">
    {/* Cover skeleton */}
    <div class="w-full shrink-0 md:w-64">
      <Skeleton class="aspect-[2/3] w-full rounded-xl" />
      <div class="mt-3 flex gap-2">
        <Skeleton class="h-6 w-14 rounded" />
        <Skeleton class="h-6 w-14 rounded" />
      </div>
    </div>
    {/* Metadata skeleton */}
    <div class="flex flex-1 flex-col gap-4">
      <Skeleton class="h-8 w-3/4 rounded" />
      <Skeleton class="h-5 w-1/2 rounded" />
      <Skeleton class="h-4 w-1/3 rounded" />
      <div class="mt-2 flex flex-col gap-2">
        <Skeleton class="h-4 w-full rounded" />
        <Skeleton class="h-4 w-full rounded" />
        <Skeleton class="h-4 w-5/6 rounded" />
      </div>
    </div>
  </div>
);

// ---- Main inner component ----

const BookDetailInner: Component<{ bookId: number }> = (props) => {
  const navigate = useNavigate();
  const auth = useAuth();

  const [book] = createResource(() => props.bookId, fetchBookDetail);
  const [bookShelves, { refetch: refetchBookShelves }] = createResource(
    () => props.bookId,
    fetchBookShelves
  );
  const [imgError, setImgError] = createSignal(false);
  const [descExpanded, setDescExpanded] = createSignal(false);
  const [deleting, setDeleting] = createSignal(false);
  const [showDeleteConfirm, setShowDeleteConfirm] = createSignal(false);
  const [showShelfDialog, setShowShelfDialog] = createSignal(false);
  const [showMetadataSearch, setShowMetadataSearch] = createSignal(false);
  const [showSendDialog, setShowSendDialog] = createSignal(false);
  const [recipients] = createResource(fetchEmailRecipients);
  const [selectedRecipientIds, setSelectedRecipientIds] = createSignal<number[]>([]);
  const [sending, setSending] = createSignal(false);

  const isAudiobook = createMemo(() => book()?.bookType === "AUDIOBOOK");

  // Collect unique formats from files.
  const formats = createMemo(() => {
    const files = book()?.files ?? [];
    const seen = new Set<string>();
    return files.filter((f) => {
      if (seen.has(f.format)) return false;
      seen.add(f.format);
      return true;
    });
  });

  const seriesLabel = createMemo(() => {
    const series = book()?.series ?? [];
    if (series.length === 0) return null;
    return series
      .map((s) =>
        s.seriesNumber !== undefined
          ? `Book ${s.seriesNumber} of ${s.name}`
          : s.name,
      )
      .join(", ");
  });

  async function handleDelete() {
    setDeleting(true);
    try {
      await api(`/books/${props.bookId}`, { method: "DELETE" });
      navigate("/libraries");
    } catch {
      setDeleting(false);
      setShowDeleteConfirm(false);
    }
  }

  return (
    <>
    <Show when={book()} fallback={<BookDetailSkeleton />}>
      {(b) => (
        <div class="flex flex-1 flex-col">
          {/* Header bar */}
          <div class="flex items-center justify-between border-b border-slate-800 px-6 py-4">
            <button
              onClick={() => navigate(-1)}
              class="text-sm text-slate-400 hover:text-slate-200 transition-colors"
            >
              {t("book.back")}
            </button>
            <div class="flex items-center gap-2">
              <Button
                variant="primary"
                size="sm"
                onClick={() => navigate(`/books/${props.bookId}/read`)}
              >
                <BookOpenCheck class="h-4 w-4" />
                {t("book.read")}
              </Button>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => setShowShelfDialog(true)}
              >
                <BookMarked class="h-4 w-4" />
                {t("book.addToShelf")}
              </Button>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => setShowMetadataSearch(true)}
              >
                <Search class="h-4 w-4" />
                {t("book.findMetadata")}
              </Button>
              <Show when={auth.canEmailSend()}>
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => setShowSendDialog(true)}
                >
                  <Mail class="h-4 w-4" />
                  {t("book.sendToDevice")}
                </Button>
              </Show>
              <Show when={auth.isAdmin()}>
                <Show
                  when={!showDeleteConfirm()}
                  fallback={
                    <div class="flex items-center gap-2">
                      <span class="text-sm text-slate-400">{t("book.areYouSure")}</span>
                      <Button
                        variant="danger"
                        size="sm"
                        onClick={handleDelete}
                        loading={deleting()}
                      >
                        {t("common.delete")}
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => setShowDeleteConfirm(false)}
                      >
                        {t("common.cancel")}
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
                    {t("common.delete")}
                  </Button>
                </Show>
              </Show>
            </div>
          </div>

          {/* Content */}
          <div class="flex flex-1 flex-col gap-8 overflow-y-auto p-6 md:flex-row md:items-start">
            {/* Left column: cover + format badges */}
            <div class="w-full shrink-0 md:w-64">
              <div
                class={`relative overflow-hidden rounded-xl bg-slate-700 shadow-xl ${
                  isAudiobook() ? "aspect-square" : "aspect-[2/3]"
                }`}
              >
                <Show
                  when={!imgError()}
                  fallback={<CoverPlaceholder bookType={b().bookType} />}
                >
                  <img
                    src={`/api/books/${props.bookId}/cover`}
                    alt={b().metadata?.title ?? b().title ?? t("common.bookCover")}
                    class="h-full w-full object-cover"
                    onError={() => setImgError(true)}
                  />
                </Show>
              </div>

              {/* Format badges */}
              <Show when={formats().length > 0}>
                <div class="mt-3 flex flex-wrap gap-2">
                  <For each={formats()}>
                    {(f) => <FormatBadge format={f.format} />}
                  </For>
                </div>
              </Show>
            </div>

            {/* Right column: metadata */}
            <div class="flex flex-1 flex-col gap-6 min-w-0">
              {/* Title & subtitle */}
              <div>
                <h1 class="text-2xl font-bold leading-tight text-slate-100 md:text-3xl">
                  {b().metadata?.title ?? b().title ?? t("common.untitled")}
                </h1>
                <Show when={b().metadata?.subtitle}>
                  <p class="mt-1 text-lg text-slate-400">
                    {b().metadata!.subtitle}
                  </p>
                </Show>
              </div>

              {/* Authors */}
              <Show when={(b().authors ?? []).length > 0}>
                <div class="flex flex-wrap gap-1 text-sm text-slate-300">
                  <For each={b().authors}>
                    {(author, i) => (
                      <>
                        <span>{author.name}</span>
                        <Show when={i() < b().authors.length - 1}>
                          <span class="text-slate-600">,</span>
                        </Show>
                      </>
                    )}
                  </For>
                </div>
              </Show>

              {/* Series */}
              <Show when={seriesLabel()}>
                <p class="text-sm font-medium text-indigo-400">
                  {seriesLabel()}
                </p>
              </Show>

              {/* Description */}
              <Show when={b().metadata?.description}>
                <div class="flex flex-col gap-2">
                  <p
                    class={`text-sm leading-relaxed text-slate-300 ${
                      descExpanded() ? "" : "line-clamp-5"
                    }`}
                  >
                    {b().metadata!.description}
                  </p>
                  <button
                    onClick={() => setDescExpanded((v) => !v)}
                    class="flex items-center gap-1 self-start text-xs text-indigo-400 hover:text-indigo-300 transition-colors"
                  >
                    <Show
                      when={descExpanded()}
                      fallback={
                        <>
                          {t("book.showMore")} <ChevronDown class="h-3 w-3" />
                        </>
                      }
                    >
                      <>
                        {t("book.showLess")} <ChevronUp class="h-3 w-3" />
                      </>
                    </Show>
                  </button>
                </div>
              </Show>

              {/* Metadata grid */}
              <div class="grid grid-cols-2 gap-4 sm:grid-cols-3">
                <Show when={b().metadata?.publisher}>
                  <MetaRow label={t("book.publisher")} value={b().metadata!.publisher!} />
                </Show>
                <Show when={b().metadata?.publishDate}>
                  <MetaRow
                    label={t("book.published")}
                    value={b().metadata!.publishDate!}
                  />
                </Show>
                <Show when={b().metadata?.language}>
                  <MetaRow label={t("book.language")} value={b().metadata!.language!} />
                </Show>
                <Show when={b().metadata?.pageCount !== undefined}>
                  <MetaRow
                    label={t("book.pageCount")}
                    value={String(b().metadata!.pageCount)}
                  />
                </Show>
                <Show when={b().metadata?.isbn13}>
                  <MetaRow label={t("book.isbn13")} value={b().metadata!.isbn13!} />
                </Show>
                <Show when={b().metadata?.isbn10}>
                  <MetaRow label={t("book.isbn10")} value={b().metadata!.isbn10!} />
                </Show>
                <Show when={b().addedDate}>
                  <MetaRow label={t("book.added")} value={b().addedDate!} />
                </Show>
              </div>

              {/* Categories */}
              <Show when={(b().categories ?? []).length > 0}>
                <div class="flex flex-col gap-2">
                  <span class="text-xs font-medium uppercase tracking-wide text-slate-500">
                    {t("book.categories")}
                  </span>
                  <div class="flex flex-wrap gap-2">
                    <For each={b().categories}>
                      {(cat) => (
                        <span class="rounded-full bg-slate-700 px-3 py-1 text-xs text-slate-300">
                          {cat.name}
                        </span>
                      )}
                    </For>
                  </div>
                </div>
              </Show>

              {/* Tags */}
              <Show when={(b().tags ?? []).length > 0}>
                <div class="flex flex-col gap-2">
                  <span class="text-xs font-medium uppercase tracking-wide text-slate-500">
                    {t("book.tags")}
                  </span>
                  <div class="flex flex-wrap gap-2">
                    <For each={b().tags}>
                      {(tag) => (
                        <span class="rounded-full bg-slate-800 px-3 py-1 text-xs text-slate-400 ring-1 ring-slate-700">
                          {tag.name}
                        </span>
                      )}
                    </For>
                  </div>
                </div>
              </Show>

              {/* Files */}
              <Show when={(b().files ?? []).length > 0}>
                <div class="flex flex-col gap-2">
                  <span class="text-xs font-medium uppercase tracking-wide text-slate-500">
                    {t("book.files")}
                  </span>
                  <div class="flex flex-col gap-2">
                    <For each={b().files}>
                      {(file) => <FileRow file={file} />}
                    </For>
                  </div>
                </div>
              </Show>

              {/* Shelves */}
              <Show when={(bookShelves() ?? []).length > 0}>
                <div class="flex flex-col gap-2">
                  <span class="text-xs font-medium uppercase tracking-wide text-slate-500">
                    {t("book.inShelves")}
                  </span>
                  <div class="flex flex-wrap gap-2">
                    <For each={bookShelves() ?? []}>
                      {(shelf) => (
                        <button
                          onClick={() => navigate(`/shelves/${shelf.id}`)}
                          class="flex items-center gap-1.5 rounded-full bg-indigo-600/20 px-3 py-1 text-xs font-medium text-indigo-300 transition-colors hover:bg-indigo-600/30"
                        >
                          <Show when={shelf.icon}>
                            <span>{shelf.icon}</span>
                          </Show>
                          {shelf.name}
                        </button>
                      )}
                    </For>
                  </div>
                </div>
              </Show>

              {/* Similar Books */}
              <SimilarBooksSection bookId={props.bookId} />
            </div>
          </div>
        </div>
      )}
    </Show>

    {/* Add to Shelf dialog */}
    <Show when={showShelfDialog()}>
      <AddToShelfDialog
        bookId={props.bookId}
        onClose={() => {
          setShowShelfDialog(false);
          void refetchBookShelves();
        }}
      />
    </Show>

    {/* Metadata search panel */}
    <Show when={showMetadataSearch() && book()}>
      <MetadataSearch
        bookId={props.bookId}
        bookType={book()!.bookType}
        onClose={() => setShowMetadataSearch(false)}
      />
    </Show>

    {/* Send to Device dialog */}
    <Show when={showSendDialog()}>
      <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
        <div class="w-full max-w-sm rounded-xl bg-slate-900 border border-slate-700 shadow-2xl">
          <div class="flex items-center justify-between border-b border-slate-700 px-6 py-4">
            <h2 class="text-lg font-semibold text-slate-100">{t("book.sendToDevice")}</h2>
            <button
              onClick={() => setShowSendDialog(false)}
              class="rounded-lg p-1.5 text-slate-400 hover:bg-slate-800 hover:text-slate-200 transition-colors"
            >
              <X class="h-5 w-5" />
            </button>
          </div>
          <div class="flex flex-col gap-4 p-6">
            <Show when={!recipients.loading} fallback={<p class="text-sm text-slate-400">{t("common.loading")}</p>}>
              <Show
                when={(recipients() ?? []).length > 0}
                fallback={<p class="text-sm text-slate-400">{t("book.noRecipients")}</p>}
              >
                <div class="flex flex-col gap-2">
                  <For each={recipients() ?? []}>
                    {(r) => {
                      const isSelected = () => selectedRecipientIds().includes(r.id);
                      return (
                        <button
                          type="button"
                          onClick={() => {
                            setSelectedRecipientIds((prev) =>
                              isSelected()
                                ? prev.filter((id) => id !== r.id)
                                : [...prev, r.id]
                            );
                          }}
                          class={`flex items-center gap-3 rounded-lg border px-3 py-2 text-sm transition-colors ${
                            isSelected()
                              ? "border-indigo-500 bg-indigo-600/20 text-indigo-300"
                              : "border-slate-700 bg-slate-800 text-slate-300 hover:border-slate-600"
                          }`}
                        >
                          <Show
                            when={isSelected()}
                            fallback={<div class="h-4 w-4 rounded border border-slate-600" />}
                          >
                            <CheckCircle class="h-4 w-4 text-indigo-400" />
                          </Show>
                          <div class="flex flex-col items-start">
                            <Show when={r.name}>
                              <span class="font-medium">{r.name}</span>
                            </Show>
                            <span class="text-xs text-slate-400">{r.emailAddress}</span>
                          </div>
                        </button>
                      );
                    }}
                  </For>
                </div>
              </Show>
            </Show>
            <div class="flex justify-end gap-3">
              <Button variant="secondary" size="sm" onClick={() => setShowSendDialog(false)}>
                {t("common.cancel")}
              </Button>
              <Button
                size="sm"
                loading={sending()}
                disabled={selectedRecipientIds().length === 0}
                onClick={async () => {
                  setSending(true);
                  try {
                    await sendBookToDevice(props.bookId, selectedRecipientIds());
                    showToast(t("book.sentSuccessfully"), "success");
                    setShowSendDialog(false);
                    setSelectedRecipientIds([]);
                  } catch (err: unknown) {
                    showToast(err instanceof Error ? err.message : t("book.sendFailed"), "error");
                  } finally {
                    setSending(false);
                  }
                }}
              >
                <Mail class="h-4 w-4" />
                {t("book.send")}
              </Button>
            </div>
          </div>
        </div>
      </div>
    </Show>
    </>
  );
};

// ---- Page wrapper ----

const BookDetail: Component = () => {
  const params = useParams<{ id: string }>();
  const bookId = createMemo(() => parseInt(params.id, 10));

  return (
    <ErrorBoundary
      fallback={(err) => (
        <div class="flex flex-1 flex-col items-center justify-center gap-3 p-8 text-center">
          <p class="text-lg font-medium text-red-400">
            {err.message.includes("404") ? t("book.bookNotFound") : t("book.failedToLoadBook")}
          </p>
          <p class="text-sm text-slate-500">{err.message}</p>
        </div>
      )}
    >
      <Suspense fallback={<BookDetailSkeleton />}>
        <BookDetailInner bookId={bookId()} />
      </Suspense>
    </ErrorBoundary>
  );
};

export default BookDetail;
