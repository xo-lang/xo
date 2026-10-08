# Task 01: CRUD API

Build an HTTP JSON API for managing users, stored in memory.

## Requirements

- `User` has `id: Int`, `name: Str` (1 to 100 characters), `email: Str`
  (must contain `@`), optional `nickname`.
- Endpoints:
  - `POST /users`: body `{name, email, nickname?}`. Assigns the next id
    starting at 1. Returns 201 and the user. Returns 400 with
    `{"error": "..."}` on invalid input. Returns 409 if the email already exists.
  - `GET /users/{id}`: 200 and the user, or 404.
  - `GET /users`: 200 and all users ordered by id.
  - `PATCH /users/{id}`: any subset of `name`, `email`, `nickname`. Same
    validation as create. 200, 400, 404, or 409.
  - `DELETE /users/{id}`: 204 or 404.
- The store must be safe under concurrent requests.
- Listen on the port from the `PORT` environment variable, default 8080.

## Tests to write

- Create then get returns the same user.
- Duplicate email returns 409.
- Invalid email returns 400.
- Patch with only `nickname` keeps other fields.
- Delete then get returns 404.
