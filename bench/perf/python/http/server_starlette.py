"""Starlette on uvicorn (httptools and uvloop, one worker) for crud.py: the
common Python framework stack, keep alive, loopback ($PORT)."""
import os

import uvicorn
from starlette.applications import Starlette
from starlette.requests import Request
from starlette.responses import Response
from starlette.routing import Route

import crud


async def users(request: Request) -> Response:
    status, body = crud.handle(request.method, request.url.path, await request.body())
    if body is None:
        return Response(status_code=status)
    media = "text/plain; charset=utf-8" if body.startswith(b"404") else "application/json"
    return Response(body, status_code=status, media_type=media)


app = Starlette(routes=[
    Route("/users", users, methods=["GET", "POST"]),
    Route("/users/{id}", users, methods=["GET", "PATCH", "DELETE"]),
])

if __name__ == "__main__":
    uvicorn.run(app, host="127.0.0.1", port=int(os.environ.get("PORT", "8080")), log_level="warning", access_log=False, server_header=False, loop="uvloop", http="httptools")
