"""asyncio.Protocol HTTP/1.1 server for crud.py: the standard library only,
keep alive, one process, listening on loopback ($PORT)."""
import asyncio
import os

import crud

_REASONS = {200: b"OK", 201: b"Created", 204: b"No Content", 400: b"Bad Request", 404: b"Not Found", 405: b"Method Not Allowed", 409: b"Conflict", 500: b"Internal Server Error"}
MAX_BODY = 1 << 20


class Http(asyncio.Protocol):
    __slots__ = ("transport", "buf")

    def connection_made(self, transport):
        self.transport = transport
        self.buf = b""

    def data_received(self, data):
        self.buf += data
        while True:
            end = self.buf.find(b"\r\n\r\n")
            if end < 0:
                return
            head = self.buf[:end].split(b"\r\n")
            method, target, _ = head[0].split(b" ", 2)
            length = 0
            for h in head[1:]:
                k, _, v = h.partition(b":")
                if k.strip().lower() == b"content-length":
                    length = int(v)
            if length > MAX_BODY:
                self._send(400, crud._fail(400, "could not read body")[1], close=True)
                return
            if len(self.buf) < end + 4 + length:
                return
            body = self.buf[end + 4:end + 4 + length]
            self.buf = self.buf[end + 4 + length:]
            status, out = crud.handle(method.decode(), target.decode(), body)
            self._send(status, out)

    def _send(self, status, body, close=False):
        if body is None:
            msg = b"HTTP/1.1 %d %s\r\nContent-Length: 0\r\n\r\n" % (status, _REASONS[status])
        else:
            ctype = b"text/plain; charset=utf-8" if status in (404, 405) and body.startswith(b"404") else b"application/json"
            msg = b"HTTP/1.1 %d %s\r\nContent-Type: %s\r\nContent-Length: %d\r\n\r\n" % (status, _REASONS[status], ctype, len(body)) + body
        self.transport.write(msg)
        if close:
            self.transport.close()


async def main():
    loop = asyncio.get_running_loop()
    port = int(os.environ.get("PORT", "8080"))
    server = await loop.create_server(Http, "127.0.0.1", port)
    print(f"listening on 127.0.0.1:{port}", flush=True)
    async with server:
        await server.serve_forever()


if __name__ == "__main__":
    asyncio.run(main())
