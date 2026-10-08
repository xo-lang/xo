<!-- Planted bug author prompt, Xo (translate the Go bugs), task 02-worker-pool. Render with bench/harness/prompts.py. -->
{{include bench/harness/prompts/common/xo-context.md}}

{{include bench/harness/prompts/common/planted-xo-translate.md}}

# The task the solution implements

{{include bench/tasks/02-worker-pool.md}}

# The Go bugs to translate (manifest of the Go planted set)

{{include bench/planted/02-worker-pool/manifest.json}}

# The Xo solution (base), every file

{{files bench/xo/v0.4/02-worker-pool}}

{{include bench/harness/prompts/common/planted-output-xo.md}}
