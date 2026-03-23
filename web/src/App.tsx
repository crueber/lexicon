import type { Component } from "solid-js";
import { BookOpen } from "lucide-solid";

const App: Component = () => {
  return (
    <div class="flex min-h-screen flex-col items-center justify-center bg-zinc-950 text-zinc-100">
      <div class="flex flex-col items-center gap-6">
        <BookOpen class="h-16 w-16 text-indigo-400" />
        <h1 class="text-4xl font-bold tracking-tight">Lexicon</h1>
        <p class="text-lg text-zinc-400">
          Your self-hosted digital library
        </p>
      </div>
    </div>
  );
};

export default App;
