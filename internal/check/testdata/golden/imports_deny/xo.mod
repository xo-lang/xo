module example.com/notes

deny example.com/notes/domain -> example.com/notes/store, example.com/notes/api
deny example.com/notes/store -> example.com/notes/api/...
deny example.com/notes/ghost -> example.com/notes/store
deny example.com/notes/server -> example.com/notes/nowhere/..., std/nosuch
