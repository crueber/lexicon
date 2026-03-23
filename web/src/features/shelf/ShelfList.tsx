import {
  type Component,
  createResource,
  createSignal,
  Show,
  For,
  Suspense,
  ErrorBoundary,
} from "solid-js";
import { useNavigate } from "@solidjs/router";
import { BookMarked, Plus, X, Wand2 } from "lucide-solid";
import { api } from "../../shared/api/client";
import Button from "../../shared/ui/Button";
import Input from "../../shared/ui/Input";
import Skeleton from "../../shared/ui/Skeleton";
import type { Shelf, MagicShelf } from "../library/types";

// ---- API ----

async function fetchShelves(): Promise<Shelf[]> {
  return api<Shelf[]>("/shelves");
}

async function fetchMagicShelves(): Promise<MagicShelf[]> {
  return api<MagicShelf[]>("/magic-shelves");
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

// ---- Sub-components ----

const ShelfCard: Component<{ shelf: Shelf; onClick: () => void }> = (props) => (
  <button
    onClick={props.onClick}
    class="group flex flex-col gap-3 rounded-xl bg-slate-800 p-5 text-left transition-colors hover:bg-slate-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-indigo-500 focus-visible:ring-offset-2 focus-visible:ring-offset-slate-900"
  >
    <div class="flex items-center gap-3">
      <div
        class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-xl"
        style={{
          "background-color": props.shelf.iconColor
            ? `${props.shelf.iconColor}33`
            : undefined,
        }}
      >
        <Show when={props.shelf.icon} fallback={<BookMarked class="h-5 w-5 text-indigo-400" />}>
          <span>{props.shelf.icon}</span>
        </Show>
      </div>
      <div class="min-w-0 flex-1">
        <p class="truncate font-semibold text-slate-100 group-hover:text-white">
          {props.shelf.name}
        </p>
        <p class="text-sm text-slate-400">
          {props.shelf.bookCount ?? 0}{" "}
          {(props.shelf.bookCount ?? 0) === 1 ? "book" : "books"}
        </p>
      </div>
    </div>
    <Show when={props.shelf.description}>
      <p class="line-clamp-2 text-sm text-slate-400">{props.shelf.description}</p>
    </Show>
  </button>
);

// ---- Create Shelf Dialog ----

const CreateShelfDialog: Component<{
  onClose: () => void;
  onCreated: (shelf: Shelf) => void;
}> = (props) => {
  const [name, setName] = createSignal("");
  const [description, setDescription] = createSignal("");
  const [icon, setIcon] = createSignal("");
  const [iconColor, setIconColor] = createSignal("");
  const [creating, setCreating] = createSignal(false);
  const [error, setError] = createSignal("");

  async function handleSubmit(e: Event) {
    e.preventDefault();
    if (!name().trim()) {
      setError("Name is required");
      return;
    }
    setCreating(true);
    setError("");
    try {
      const shelf = await createShelf({
        name: name().trim(),
        description: description().trim(),
        icon: icon().trim(),
        iconColor: iconColor().trim(),
        isPublic: false,
      });
      props.onCreated(shelf);
    } catch {
      setError("Failed to create shelf. Please try again.");
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
      <div class="w-full max-w-md rounded-xl bg-slate-800 shadow-2xl">
        <div class="flex items-center justify-between border-b border-slate-700 px-6 py-4">
          <h2 class="text-lg font-semibold text-slate-100">New Shelf</h2>
          <button
            onClick={props.onClose}
            class="rounded-lg p-1.5 text-slate-400 hover:bg-slate-700 hover:text-slate-200 transition-colors"
          >
            <X class="h-5 w-5" />
          </button>
        </div>

        <form onSubmit={handleSubmit} class="flex flex-col gap-4 p-6">
          <Input
            label="Name"
            placeholder="My Reading List"
            value={name()}
            onInput={(e) => setName(e.currentTarget.value)}
            required
          />
          <Input
            label="Description (optional)"
            placeholder="Books I want to read this year"
            value={description()}
            onInput={(e) => setDescription(e.currentTarget.value)}
          />
          <div class="grid grid-cols-2 gap-4">
            <Input
              label="Icon (emoji, optional)"
              placeholder="📚"
              value={icon()}
              onInput={(e) => setIcon(e.currentTarget.value)}
            />
            <Input
              label="Color (hex, optional)"
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
              Cancel
            </Button>
            <Button variant="primary" type="submit" loading={creating()}>
              Create Shelf
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
};

// ---- Skeleton ----

const ShelfListSkeleton: Component = () => (
  <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
    <For each={Array.from({ length: 6 })}>
      {() => (
        <div class="flex flex-col gap-3 rounded-xl bg-slate-800 p-5">
          <div class="flex items-center gap-3">
            <Skeleton class="h-10 w-10 rounded-lg" />
            <div class="flex flex-1 flex-col gap-2">
              <Skeleton class="h-4 w-3/4 rounded" />
              <Skeleton class="h-3 w-1/3 rounded" />
            </div>
          </div>
        </div>
      )}
    </For>
  </div>
);

// ---- Magic Shelf Card ----

const MagicShelfCard: Component<{ shelf: MagicShelf; onClick: () => void }> = (props) => (
  <button
    onClick={props.onClick}
    class="group flex flex-col gap-3 rounded-xl bg-slate-800 p-5 text-left transition-colors hover:bg-slate-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-indigo-500 focus-visible:ring-offset-2 focus-visible:ring-offset-slate-900"
  >
    <div class="flex items-center gap-3">
      <div
        class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-xl"
        style={{
          "background-color": props.shelf.iconColor
            ? `${props.shelf.iconColor}33`
            : undefined,
        }}
      >
        <Show when={props.shelf.icon} fallback={<Wand2 class="h-5 w-5 text-indigo-400" />}>
          <span>{props.shelf.icon}</span>
        </Show>
      </div>
      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-2">
          <p class="truncate font-semibold text-slate-100 group-hover:text-white">
            {props.shelf.name}
          </p>
          <span class="shrink-0 rounded-full bg-indigo-600/20 px-1.5 py-0.5 text-[10px] font-medium text-indigo-400">
            Magic
          </span>
        </div>
        <p class="text-sm text-slate-400">Dynamic collection</p>
      </div>
    </div>
    <Show when={props.shelf.description}>
      <p class="line-clamp-2 text-sm text-slate-400">{props.shelf.description}</p>
    </Show>
  </button>
);

// ---- Inner component ----

const ShelfListInner: Component = () => {
  const navigate = useNavigate();
  const [showCreate, setShowCreate] = createSignal(false);
  const [shelves, { refetch }] = createResource(fetchShelves);
  const [magicShelves] = createResource(fetchMagicShelves);

  function handleCreated() {
    setShowCreate(false);
    void refetch();
  }

  return (
    <div class="flex flex-1 flex-col">
      {/* Header */}
      <div class="flex items-center justify-between border-b border-slate-800 px-6 py-5">
        <div>
          <h1 class="text-xl font-bold text-slate-100">Shelves</h1>
          <Show when={shelves()}>
            {(data) => (
              <p class="mt-1 text-sm text-slate-400">
                {data().length} {data().length === 1 ? "shelf" : "shelves"}
              </p>
            )}
          </Show>
        </div>
        <div class="flex items-center gap-2">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => navigate("/magic-shelves/new")}
          >
            <Wand2 class="h-4 w-4" />
            New Magic Shelf
          </Button>
          <Button variant="primary" size="sm" onClick={() => setShowCreate(true)}>
            <Plus class="h-4 w-4" />
            New Shelf
          </Button>
        </div>
      </div>

      {/* Content */}
      <div class="flex-1 overflow-y-auto p-6">
        {/* Regular shelves */}
        <Show
          when={!shelves.loading}
          fallback={<ShelfListSkeleton />}
        >
          <Show
            when={(shelves() ?? []).length > 0}
            fallback={
              <div class="flex flex-col items-center justify-center gap-4 py-12 text-center">
                <BookMarked class="h-12 w-12 text-slate-600" />
                <div>
                  <p class="text-lg font-medium text-slate-300">No shelves yet</p>
                  <p class="mt-1 text-sm text-slate-500">
                    Create one to organize your books.
                  </p>
                </div>
                <Button variant="primary" onClick={() => setShowCreate(true)}>
                  <Plus class="h-4 w-4" />
                  Create a Shelf
                </Button>
              </div>
            }
          >
            <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
              <For each={shelves() ?? []}>
                {(shelf) => (
                  <ShelfCard
                    shelf={shelf}
                    onClick={() => navigate(`/shelves/${shelf.id}`)}
                  />
                )}
              </For>
            </div>
          </Show>
        </Show>

        {/* Magic shelves section */}
        <Show when={(magicShelves() ?? []).length > 0}>
          <div class="mt-8">
            <div class="mb-4 flex items-center gap-2">
              <Wand2 class="h-4 w-4 text-indigo-400" />
              <h2 class="text-sm font-semibold uppercase tracking-wider text-slate-400">
                Magic Shelves
              </h2>
            </div>
            <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
              <For each={magicShelves() ?? []}>
                {(ms) => (
                  <MagicShelfCard
                    shelf={ms}
                    onClick={() => navigate(`/magic-shelves/${ms.id}`)}
                  />
                )}
              </For>
            </div>
          </div>
        </Show>
      </div>

      {/* Create dialog */}
      <Show when={showCreate()}>
        <CreateShelfDialog
          onClose={() => setShowCreate(false)}
          onCreated={handleCreated}
        />
      </Show>
    </div>
  );
};

// ---- Page wrapper ----

const ShelfList: Component = () => (
  <ErrorBoundary
    fallback={(err) => (
      <div class="flex flex-1 flex-col items-center justify-center gap-3 p-8 text-center">
        <p class="text-lg font-medium text-red-400">Failed to load shelves</p>
        <p class="text-sm text-slate-500">{err.message}</p>
      </div>
    )}
  >
    <Suspense
      fallback={
        <div class="flex flex-1 flex-col">
          <div class="border-b border-slate-800 px-6 py-5">
            <Skeleton class="h-7 w-32 rounded" />
          </div>
          <div class="p-6">
            <ShelfListSkeleton />
          </div>
        </div>
      }
    >
      <ShelfListInner />
    </Suspense>
  </ErrorBoundary>
);

export default ShelfList;
