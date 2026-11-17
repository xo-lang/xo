# std/ui: a user update's model is the one the page showed

Observed 2026-11-17 (std/ui user-state property test, decision 0672).

## Symptom

A user act that emits a shared Msg with an id derived from the model,
such as `Msg.Add(id: "p${m.todos.len()}", ...)`, lost posts: two Posts
in one sync, or two principals posting at once, made two Adds with one
id, and the shared update kept only the first (20 todos for 23 posts).

## Cause

`with_user`'s update gets `m`, the shared model the page showed when it
synced (`shown`), not the model after the Msgs emitted so far. Every
act of one sync sees the same `m`, and two principals' syncs can see
the same `m` too. Emitted Msgs are applied after the user commit.

## Fix

Derive ids from the user state or the act (a draft's own text, a
counter in the user state, an id the client made), never from `m`.
Use `m` only to read what the user saw (stock, a price shown).
