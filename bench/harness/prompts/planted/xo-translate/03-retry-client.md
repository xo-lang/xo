<!-- Planted bug author prompt, Xo (translate the Go bugs), task 03-retry-client. Render with bench/harness/prompts.py. -->
{{include bench/harness/prompts/common/xo-context.md}}

{{include bench/harness/prompts/common/planted-xo-translate.md}}

# The task the solution implements

{{include bench/tasks/03-retry-client.md}}

# The Go bugs to translate (manifest of the Go planted set)

{{include bench/planted/03-retry-client/manifest.json}}

# The Xo solution (base), every file

{{files bench/xo/v0.4/03-retry-client}}

{{include bench/harness/prompts/common/planted-output-xo.md}}
