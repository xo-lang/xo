# Xo for front end work: benchmarks, a reactive framework, and where Xo fits

The owner's questions (2026-11-05): could Xo do well in the
js-framework-benchmark (https://krausest.github.io/js-framework-benchmark/current.html),
would we want it to, and what would a full reactive framework in Xo offer?
This is exploration, not a plan: the owner said "we won't do this now".
Nothing here is decided or scheduled; a step that becomes work gets its own
ADR. Not measured: every claim about Xo's WebAssembly output size or speed
below is an expectation, not a figure.

## The benchmark

It builds and changes a table in the browser: create 1,000 and 10,000
rows, replace them, update every 10th row, select a row, swap two rows,
remove a row, append 1,000, clear; plus startup (bundle size, blocking
time) and memory. The time is almost all DOM work, not the language's own
speed. The leaders are vanilla JS, ivi, Solid, Svelte and Inferno.

## How Xo would do

- Xo reaches the browser through its WebAssembly backend, and WebAssembly
  cannot touch the DOM directly: every DOM change crosses into JavaScript.
  Rust's frameworks (Leptos, Dioxus, Sycamore) land near the top JS ones
  but behind them; virtual DOM designs (Yew) well behind. Xo would face the
  same ceiling.
- In Xo's favor: reference counting, so no collector to ship and no
  collection pauses, and compact compiled code.
- Against: startup and bundle size. A WebAssembly program with its runtime
  is usually far larger than Solid's few KB. Xo's size for a table renderer
  is unknown; it is the first number to measure.
- The score depends mostly on the framework's design (fine-grained
  reactivity as in Solid, against a virtual DOM as in React), and Xo has no
  framework.

## Would we want to?

Not as a goal in itself: Xo's pitch is backends and web services that
agents write, check and debug. Front end options that fit that pitch, most
useful first:

1. Server-rendered HTML with light interactivity (the HTMX style): plays to
   Xo's HTTP server (190.6k req/s against Go's 125.1k on the 2026-11-05
   profile), needs no WebAssembly, simplest for agents to get right.
2. Shared types between server and browser: generate TypeScript types and
   clients from Xo's API types so the front end cannot drift from the
   backend. Cheap and useful.
3. Small WebAssembly islands for heavy work on the page (parsing, crypto,
   image processing), where Xo's speed counts and the DOM crossing does not.
4. A full reactive UI framework: months of work in a crowded field, worth
   it only to make Xo a full stack language the way Leptos is for Rust.

## What a reactive framework in Xo would offer

Not speed or features, which Solid, Svelte and Leptos already have, but
what Xo's checker and toolchain can verify that JavaScript frameworks can't:

1. A server and browser boundary checked by the compiler. Effects already
   show what touches the network, disk or a database; browser code calling
   a server-only capability could fail to compile, so secrets, database
   handles and file access cannot leak to the client by accident. Next.js
   and Remix enforce this by convention and runtime errors.
2. Supply chain safety. A dependency's effects are in its signatures, so a
   UI library that starts wanting network access shows up when it is
   checked, against the routine npm compromises.
3. Whole bug classes gone. The commonest React bugs (stale closures, wrong
   useEffect dependency lists, server and client rendering different HTML)
   go away when the compiler knows what each piece of UI reads and works
   out the dependencies itself; insertion-ordered collections keep server
   and client output identical.
4. Agents build a whole app end to end: one language and toolchain, stable
   JSON diagnostics, `xo debug`, no npm, bundler or node_modules; each layer
   from a database query to a button checked against the others.
5. Smaller: steady frame times with no collector, deterministic rendering
   so snapshot tests do not flake.

Against it:

- Models know React best. Today's models have seen vast amounts of React,
  so an agent writing React is already good at it; Xo starts with no
  examples. This is the biggest obstacle and undercuts point 4.
- No ecosystem: component libraries, design systems, browser devtools,
  charts, date pickers. Teams pick frameworks largely for these.
- WebAssembly costs larger downloads and slower first load than Solid or
  Svelte, which hurts public sites and SEO.
- Shared types alone are not unique: TypeScript across the stack already
  gives most teams that.

Where it could win: not public consumer sites, but internal tools, admin
panels and dashboards that agents build and maintain, for companies that
care about security. Points 1, 2 and 4 matter most there, and bundle size
and ecosystem least.

## If this is ever picked up

1. Measure first: compile a 1,000-row table renderer to WebAssembly and
   record its size and startup time against Solid and Leptos. That number
   decides whether options 3 and 4 are realistic.
2. Then server-rendered pages with small Xo islands, plus the compile-time
   server and browser boundary (point 1), which tests the most distinctive
   idea for a fraction of a framework's cost.

Related: [ai-and-ml.md](ai-and-ml.md) (effects showing what touches the
network or disk), [wasm-and-jit.md](wasm-and-jit.md) (Xo's WebAssembly
target), [positioning.md](positioning.md).
