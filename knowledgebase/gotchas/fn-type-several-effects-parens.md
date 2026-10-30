# A function type with several effects needs parentheses

Status: Observed (2026-10-30, decision 0422)

## Symptom

A generated binding declared `opts: List[fn(Options) uses go, fx]` and the
checker reported `List takes 1 type argument(s), found 2` and "`fx` is an
effect variable, not a type".

## Cause

A function *type* takes one effect or a parenthesized list
(`FnType ... [ "uses" ( Effect | "(" EffectList ")" ) ]`, spec grammar);
only a function declaration's clause allows `uses a, b` bare. Inside type
arguments the comma separates the arguments.

## Fix

Write `fn(Options) uses (go, fx)` in any generated or written type.
