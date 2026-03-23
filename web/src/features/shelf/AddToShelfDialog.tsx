import {
  type Component,
  createResource,
  createSignal,
  Show,
  For,
} from "solid-js";
import { BookMarked, Check, Plus, X, Loader2 } from "lucide-solid";
import { api } from "../../shared/api/client";
import Button from "../../shared/ui/Button";
import Input from "../../shared/ui/Input";
import type { Shelf } from "../library/types";

// ---- API ----

async function fetchShelves(): Promise<Shelf[]> {
  return api<Shelf[]>("/shelves");
}

async function fetchBookShelves(bookId: number): Promise<Shelf[]> {
  return api<Shelf[]>(`/books/${bookId}/shelves`);
}

async function addBookToShelf(shelfId: number, bookId: number): Promise<void> {
  await api(`/shelves/${shelfId}/books`, {
    method: "POST",
    body: JSON.stringify({ bookId }),
  });
}

async function removeBookFromShelf(shelfId: number, bookId: number): Promise<void> {
  await api(`/shelves/${shelfId}/books/${bookId}`, { method: "DELETE" });
}

async function createShelf(params: {
  name: string;
  description: string;
  icon: string;
  iconColor: string;
  isPublic: boolean;
}): Promise<Shelf> {
  return api<Shelf>("/shelves", {
    method: "POST",
    body: JSON.stringify(params),
  });
}

// ---- Props ----

interface AddToShelfDialogProps {
  bookId: number;
  onClose: () => void;
}

// ---- Component ----

const AddToShelfDialog: Component<AddToShelfDialogProps> = (props) => {
  const [shelves, { refetch: refetchShelves }] = createResource(fetchShelves);
  const [bookShelves, { refetch: refetchBookShelves }] = createResource(
    () => props.bookId,
    fetchBookShelves
  );

  const [toggling, setToggling] = createSignal<number | null>(null);
  const [showNewShelf, setShowNewShelf] = createSignal(false);
  const [newName, setNewName] = createSignal("");
  const [creating, setCreating] = createSignal(false);
  const [createError, setCreateError] = createSignal("");

  function isInShelf(shelfId: number): boolean {
    return (bookShelves() ?? []).some((s) => s.id === shelfId);
  }

  async function handleToggle(shelfId: number) {
    setToggling(shelfId);
    try {
      if (isInShelf(shelfId)) {
        await removeBookFromShelf(shelfId, props.bookId);
      } else {
        await addBookToShelf(shelfId, props.bookId);
      }
      void refetchBookShelves();
    } catch {
      // Non-fatal: silently fail
    } finally {
      setToggling(null);
    }
  }

  async function handleCreateShelf(e: Event) {
    e.preventDefault();
    if (!newName().trim()) return;
    setCreating(true);
    setCreateError("");
    try {
      const shelf = await createShelf({
        name: newName().trim(),
        description: "",
        icon: "",
        iconColor: "",
        isPublic: false,
      });
      // Add the book to the newly created shelf.
      await addBookToShelf(shelf.id, props.bookId);
      setNewName("");
      setShowNewShelf(false);
      void refetchShelves();
      void refetchBookShelves();
    } catch {
      setCreateError("Failed to create shelf. Please try again.");
    } finally {
      setCreating(false);
    }
  }

  return (
    <div
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
      onClick={(e) => {
        if (e.target === e.currentTarget) props.onClose();
      }}
    >
      <div class="w-full max-w-sm rounded-xl bg-slate-800 shadow-2xl">
        {/* Header */}
        <div class="flex items-center justify-between border-b border-slate-700 px-5 py-4">
          <h2 class="text-base font-semibold text-slate-100">Add to Shelf</h2>
          <button
            onClick={props.onClose}
            class="rounded-lg p-1.5 text-slate-400 hover:bg-slate-700 hover:text-slate-200 transition-colors"
          >
            <X class="h-4 w-4" />
          </button>
        </div>

        {/* Shelf list */}
        <div class="max-h-72 overflow-y-auto">
          <Show
            when={!shelves.loading && !bookShelves.loading}
            fallback={
              <div class="flex items-center justify-center py-8">
                <Loader2 class="h-5 w-5 animate-spin text-slate-400" />
              </div>
            }
          >
            <Show
              when={(shelves() ?? []).length > 0}
              fallback={
                <div class="flex flex-col items-center gap-2 py-8 text-center">
                  <BookMarked class="h-8 w-8 text-slate-600" />
                  <p class="text-sm text-slate-400">No shelves yet</p>
                </div>
              }
            >
              <For each={shelves() ?? []}>
                {(shelf) => {
                  const inShelf = () => isInShelf(shelf.id);
                  const isToggling = () => toggling() === shelf.id;

                  return (
                    <button
                      onClick={() => handleToggle(shelf.id)}
                      disabled={isToggling()}
                      class="flex w-full items-center gap-3 px-5 py-3 text-left transition-colors hover:bg-slate-700 disabled:opacity-50"
                    >
                      <div
                        class={`flex h-8 w-8 shrink-0 items-center justify-center rounded-lg transition-colors ${
                          inShelf()
                            ? "bg-indigo-600/30 text-indigo-400"
                            : "bg-slate-700 text-slate-400"
                        }`}
                      >
                        <Show
                          when={isToggling()}
                          fallback={
                            <Show
                              when={inShelf()}
                              fallback={
                                <Show
                                  when={shelf.icon}
                                  fallback={<BookMarked class="h-4 w-4" />}
                                >
                                  <span class="text-sm">{shelf.icon}</span>
                                </Show>
                              }
                            >
                              <Check class="h-4 w-4" />
                            </Show>
                          }
                        >
                          <Loader2 class="h-4 w-4 animate-spin" />
                        </Show>
                      </div>
                      <div class="min-w-0 flex-1">
                        <p class="truncate text-sm font-medium text-slate-100">
                          {shelf.name}
                        </p>
                        <p class="text-xs text-slate-500">
                          {shelf.bookCount ?? 0}{" "}
                          {(shelf.bookCount ?? 0) === 1 ? "book" : "books"}
                        </p>
                      </div>
                      <Show when={inShelf()}>
                        <span class="text-xs text-indigo-400">Added</span>
                      </Show>
                    </button>
                  );
                }}
              </For>
            </Show>
          </Show>
        </div>

        {/* New shelf section */}
        <div class="border-t border-slate-700 p-4">
          <Show
            when={showNewShelf()}
            fallback={
              <button
                onClick={() => setShowNewShelf(true)}
                class="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm text-slate-400 transition-colors hover:bg-slate-700 hover:text-slate-200"
              >
                <Plus class="h-4 w-4" />
                New Shelf
              </button>
            }
          >
            <form onSubmit={handleCreateShelf} class="flex flex-col gap-3">
              <Input
                placeholder="Shelf name"
                value={newName()}
                onInput={(e) => setNewName(e.currentTarget.value)}
                autofocus
              />
              <Show when={createError()}>
                <p class="text-xs text-red-400">{createError()}</p>
              </Show>
              <div class="flex gap-2">
                <Button
                  variant="ghost"
                  size="sm"
                  type="button"
                  onClick={() => {
                    setShowNewShelf(false);
                    setNewName("");
                    setCreateError("");
                  }}
                >
                  Cancel
                </Button>
                <Button
                  variant="primary"
                  size="sm"
                  type="submit"
                  loading={creating()}
                  disabled={!newName().trim()}
                >
                  Create & Add
                </Button>
              </div>
            </form>
          </Show>
        </div>
      </div>
    </div>
  );
};

export default AddToShelfDialog;
