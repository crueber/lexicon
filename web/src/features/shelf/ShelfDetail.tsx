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
import { BookMarked, Pencil, Trash2, X, Minus } from "lucide-solid";
import { api } from "../../shared/api/client";
import { useAuth } from "../auth/AuthProvider";
import Button from "../../shared/ui/Button";
import Input from "../../shared/ui/Input";
import Skeleton from "../../shared/ui/Skeleton";
import { t } from "../../shared/i18n/i18n";
import type { Shelf, ShelfBook } from "../library/types";

// ---- API ----

async function fetchShelf(id: number): Promise<Shelf> {
  return api<Shelf>(`/shelves/${id}`);
}

async function fetchShelfBooks(id: number): Promise<ShelfBook[]> {
  return api<ShelfBook[]>(`/shelves/${id}/books`);
}

async function updateShelf(
  id: number,
  params: {
    name: string;
    description: string;
    icon: string;
    iconColor: string;
    isPublic: boolean;
  }
): Promise<void> {
  await api(`/shelves/${id}`, {
    method: "PUT",
    body: JSON.stringify(params),
  });
}

async function deleteShelf(id: number): Promise<void> {
  await api(`/shelves/${id}`, { method: "DELETE" });
}

async function removeBookFromShelf(shelfId: number, bookId: number): Promise<void> {
  await api(`/shelves/${shelfId}/books/${bookId}`, { method: "DELETE" });
}

// ---- Sub-components ----

const ShelfBookCard: Component<{
  book: ShelfBook;
  isOwner: boolean;
  onRemove: () => void;
  onClick: () => void;
}> = (props) => {
  const [imgError, setImgError] = createSignal(false);
  const isAudiobook = () => props.book.bookType === "AUDIOBOOK";

  return (
    <div class="group relative flex flex-col gap-2 rounded-lg bg-slate-800 p-2">
      <button
        onClick={props.onClick}
        class="flex flex-col gap-2 text-left focus-visible:outline-none"
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
              alt={props.book.title ?? t("common.bookCover")}
              loading="lazy"
              class="h-full w-full object-cover transition-transform duration-200 group-hover:scale-105"
              onError={() => setImgError(true)}
            />
          </Show>
        </div>
        <div class="min-w-0 flex-1">
          <p class="line-clamp-2 text-xs font-medium leading-tight text-slate-100">
            {props.book.title ?? t("common.untitled")}
          </p>
        </div>
      </button>

      {/* Remove button — owner only */}
      <Show when={props.isOwner}>
        <button
          onClick={(e) => {
            e.stopPropagation();
            props.onRemove();
          }}
          class="absolute right-1.5 top-1.5 hidden rounded-full bg-red-600/90 p-1 text-white transition-colors hover:bg-red-500 group-hover:flex"
          title={t("shelf.removeFromShelf")}
        >
          <Minus class="h-3 w-3" />
        </button>
      </Show>
    </div>
  );
};

// ---- Edit Shelf Dialog ----

const EditShelfDialog: Component<{
  shelf: Shelf;
  onClose: () => void;
  onSaved: () => void;
}> = (props) => {
  const [name, setName] = createSignal(props.shelf.name);
  const [description, setDescription] = createSignal(props.shelf.description ?? "");
  const [icon, setIcon] = createSignal(props.shelf.icon ?? "");
  const [iconColor, setIconColor] = createSignal(props.shelf.iconColor ?? "");
  const [saving, setSaving] = createSignal(false);
  const [error, setError] = createSignal("");

  async function handleSubmit(e: Event) {
    e.preventDefault();
    if (!name().trim()) {
      setError(t("common.nameRequired"));
      return;
    }
    setSaving(true);
    setError("");
    try {
      await updateShelf(props.shelf.id, {
        name: name().trim(),
        description: description().trim(),
        icon: icon().trim(),
        iconColor: iconColor().trim(),
        isPublic: props.shelf.isPublic,
      });
      props.onSaved();
    } catch {
      setError(t("common.failedToSaveChanges"));
    } finally {
      setSaving(false);
    }
  }

  return (
    <div
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
      onClick={(e) => {
        if (e.target === e.currentTarget) props.onClose();
      }}
    >
      <div class="w-full max-w-md rounded-xl bg-slate-800 shadow-2xl">
        <div class="flex items-center justify-between border-b border-slate-700 px-6 py-4">
          <h2 class="text-lg font-semibold text-slate-100">{t("shelf.editShelf")}</h2>
          <button
            onClick={props.onClose}
            class="rounded-lg p-1.5 text-slate-400 hover:bg-slate-700 hover:text-slate-200 transition-colors"
          >
            <X class="h-5 w-5" />
          </button>
        </div>

        <form onSubmit={handleSubmit} class="flex flex-col gap-4 p-6">
          <Input
            label={t("common.name")}
            value={name()}
            onInput={(e) => setName(e.currentTarget.value)}
            required
          />
          <Input
            label={t("common.descriptionOptional")}
            value={description()}
            onInput={(e) => setDescription(e.currentTarget.value)}
          />
          <div class="grid grid-cols-2 gap-4">
            <Input
              label={t("common.iconOptional")}
              placeholder="📚"
              value={icon()}
              onInput={(e) => setIcon(e.currentTarget.value)}
            />
            <Input
              label={t("common.colorOptional")}
              placeholder="#6366f1"
              value={iconColor()}
              onInput={(e) => setIconColor(e.currentTarget.value)}
            />
          </div>

          <Show when={error()}>
            <p class="text-sm text-red-400">{error()}</p>
          </Show>

          <div class="flex justify-end gap-3 pt-2">
            <Button variant="ghost" type="button" onClick={props.onClose}>
              {t("common.cancel")}
            </Button>
            <Button variant="primary" type="submit" loading={saving()}>
              {t("common.saveChanges")}
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
};

// ---- Skeleton ----

const ShelfDetailSkeleton: Component = () => (
  <div class="flex flex-1 flex-col">
    <div class="border-b border-slate-800 px-6 py-5">
      <Skeleton class="h-7 w-48 rounded" />
      <Skeleton class="mt-2 h-4 w-32 rounded" />
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

const ShelfDetailInner: Component<{ shelfId: number }> = (props) => {
  const navigate = useNavigate();
  const auth = useAuth();

  const [shelf, { refetch: refetchShelf }] = createResource(
    () => props.shelfId,
    fetchShelf
  );
  const [books, { refetch: refetchBooks }] = createResource(
    () => props.shelfId,
    fetchShelfBooks
  );

  const [showEdit, setShowEdit] = createSignal(false);
  const [showDeleteConfirm, setShowDeleteConfirm] = createSignal(false);
  const [deleting, setDeleting] = createSignal(false);

  const isOwner = createMemo(() => {
    const s = shelf();
    if (!s) return false;
    return s.userId === auth.user()?.id;
  });

  async function handleDelete() {
    setDeleting(true);
    try {
      await deleteShelf(props.shelfId);
      navigate("/shelves");
    } catch {
      setDeleting(false);
      setShowDeleteConfirm(false);
    }
  }

  async function handleRemoveBook(bookId: number) {
    try {
      await removeBookFromShelf(props.shelfId, bookId);
      void refetchBooks();
    } catch {
      // Non-fatal: silently fail
    }
  }

  function handleEditSaved() {
    setShowEdit(false);
    void refetchShelf();
  }

  return (
    <Show when={shelf()} fallback={<ShelfDetailSkeleton />}>
      {(s) => (
        <div class="flex flex-1 flex-col">
          {/* Header */}
          <div class="flex items-start justify-between border-b border-slate-800 px-6 py-5">
            <div class="flex items-center gap-4">
              <button
                onClick={() => navigate("/shelves")}
                class="text-sm text-slate-400 hover:text-slate-200 transition-colors"
              >
                ← {t("common.shelves")}
              </button>
              <div class="flex items-center gap-3">
                <Show when={s().icon} fallback={<BookMarked class="h-6 w-6 text-indigo-400" />}>
                  <span class="text-2xl">{s().icon}</span>
                </Show>
                <div>
                  <h1 class="text-xl font-bold text-slate-100">{s().name}</h1>
                  <Show when={s().description}>
                    <p class="mt-0.5 text-sm text-slate-400">{s().description}</p>
                  </Show>
                </div>
              </div>
            </div>

            {/* Owner actions */}
            <Show when={isOwner()}>
              <div class="flex items-center gap-2">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setShowEdit(true)}
                >
                  <Pencil class="h-4 w-4" />
                  {t("common.edit")}
                </Button>
                <Show
                  when={!showDeleteConfirm()}
                  fallback={
                    <div class="flex items-center gap-2">
                      <span class="text-sm text-slate-400">{t("common.areYouSure")}</span>
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
                    <BookMarked class="h-12 w-12 text-slate-600" />
                    <div>
                      <p class="text-lg font-medium text-slate-300">
                        {t("shelf.noBooksInShelf")}
                      </p>
                      <p class="mt-1 text-sm text-slate-500">
                        {t("shelf.addBooksFromDetail")}
                      </p>
                    </div>
                  </div>
                }
              >
                <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
                  <For each={books() ?? []}>
                    {(book) => (
                      <ShelfBookCard
                        book={book}
                        isOwner={isOwner()}
                        onRemove={() => handleRemoveBook(book.id)}
                        onClick={() => navigate(`/books/${book.id}`)}
                      />
                    )}
                  </For>
                </div>
              </Show>
            </Show>
          </div>

          {/* Edit dialog */}
          <Show when={showEdit()}>
            <EditShelfDialog
              shelf={s()}
              onClose={() => setShowEdit(false)}
              onSaved={handleEditSaved}
            />
          </Show>
        </div>
      )}
    </Show>
  );
};

// ---- Page wrapper ----

const ShelfDetail: Component = () => {
  const params = useParams<{ id: string }>();
  const shelfId = createMemo(() => parseInt(params.id, 10));

  return (
    <ErrorBoundary
      fallback={(err) => (
        <div class="flex flex-1 flex-col items-center justify-center gap-3 p-8 text-center">
          <p class="text-lg font-medium text-red-400">
            {err.message.includes("404") ? t("shelf.shelfNotFound") : t("shelf.failedToLoadShelf")}
          </p>
          <p class="text-sm text-slate-500">{err.message}</p>
        </div>
      )}
    >
      <Suspense fallback={<ShelfDetailSkeleton />}>
        <ShelfDetailInner shelfId={shelfId()} />
      </Suspense>
    </ErrorBoundary>
  );
};

export default ShelfDetail;
