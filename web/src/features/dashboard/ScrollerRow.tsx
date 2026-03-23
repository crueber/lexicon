import { type Component, createSignal, Show, For, onMount } from "solid-js";
import { useNavigate } from "@solidjs/router";
import { ChevronLeft, ChevronRight } from "lucide-solid";
import BookCard from "../book/BookCard";
import type { Book } from "../library/types";

interface ScrollerRowProps {
  title: string;
  books: Book[];
  seeAllHref?: string;
}

const ScrollerRow: Component<ScrollerRowProps> = (props) => {
  const navigate = useNavigate();
  let scrollRef: HTMLDivElement | undefined;
  const [canScrollLeft, setCanScrollLeft] = createSignal(false);
  const [canScrollRight, setCanScrollRight] = createSignal(false);

  // Update scroll button visibility based on current scroll position.
  function updateScrollButtons() {
    if (!scrollRef) return;
    setCanScrollLeft(scrollRef.scrollLeft > 0);
    setCanScrollRight(
      scrollRef.scrollLeft < scrollRef.scrollWidth - scrollRef.clientWidth - 1,
    );
  }

  // Check initial scroll state after mount.
  onMount(() => {
    updateScrollButtons();
  });

  function scrollLeft() {
    if (!scrollRef) return;
    scrollRef.scrollBy({ left: -320, behavior: "smooth" });
  }

  function scrollRight() {
    if (!scrollRef) return;
    scrollRef.scrollBy({ left: 320, behavior: "smooth" });
  }

  return (
    <section class="flex flex-col gap-3">
      {/* Row header */}
      <div class="flex items-center justify-between px-1">
        <h2 class="text-base font-semibold text-slate-100">{props.title}</h2>
        <Show when={props.seeAllHref}>
          <a
            href={props.seeAllHref}
            class="text-xs text-indigo-400 hover:text-indigo-300 transition-colors"
          >
            See all
          </a>
        </Show>
      </div>

      {/* Scroller container */}
      <Show
        when={props.books.length > 0}
        fallback={
          <div class="flex h-32 items-center justify-center rounded-lg border border-slate-700/50 bg-slate-800/30">
            <p class="text-sm text-slate-500">No books yet</p>
          </div>
        }
      >
        <div class="group/row relative">
          {/* Left scroll button */}
          <Show when={canScrollLeft()}>
            <button
              onClick={scrollLeft}
              class="absolute left-0 top-1/2 z-10 -translate-y-1/2 -translate-x-2 flex h-8 w-8 items-center justify-center rounded-full bg-slate-700 shadow-lg opacity-0 group-hover/row:opacity-100 transition-opacity hover:bg-slate-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-indigo-500"
              aria-label="Scroll left"
            >
              <ChevronLeft class="h-4 w-4 text-slate-200" />
            </button>
          </Show>

          {/* Scrollable book strip */}
          <div
            ref={scrollRef}
            onScroll={updateScrollButtons}
            // Use a ResizeObserver-friendly approach: check on mount via ref callback
            class="flex gap-3 overflow-x-auto pb-2 scrollbar-hide"
            style="scroll-snap-type: x mandatory;"
          >
            <For each={props.books}>
              {(book) => (
                <div
                  class="w-32 flex-none sm:w-36"
                  style="scroll-snap-align: start;"
                >
                  <BookCard
                    book={book}
                    onClick={() => navigate(`/books/${book.id}`)}
                  />
                </div>
              )}
            </For>
          </div>

          {/* Right scroll button */}
          <Show when={canScrollRight()}>
            <button
              onClick={scrollRight}
              class="absolute right-0 top-1/2 z-10 -translate-y-1/2 translate-x-2 flex h-8 w-8 items-center justify-center rounded-full bg-slate-700 shadow-lg opacity-0 group-hover/row:opacity-100 transition-opacity hover:bg-slate-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-indigo-500"
              aria-label="Scroll right"
            >
              <ChevronRight class="h-4 w-4 text-slate-200" />
            </button>
          </Show>
        </div>
      </Show>
    </section>
  );
};

export default ScrollerRow;
