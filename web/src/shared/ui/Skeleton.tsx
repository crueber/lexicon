import { type Component } from "solid-js";

// Skeleton renders an animated placeholder for loading states.
// Usage: <Skeleton class="h-48 w-full rounded-lg" />
const Skeleton: Component<{ class?: string }> = (props) => (
  <div class={`animate-pulse bg-slate-700 rounded ${props.class ?? ""}`} />
);

export default Skeleton;
