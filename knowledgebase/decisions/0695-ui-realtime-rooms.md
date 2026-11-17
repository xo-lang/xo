# 0695: std/ui real-time rooms

Status: Accepted (owner std/ui track, xo-4c, 2026-11-17)

## Context

Decision 0686 plan step 7: games at 20 to 60 updates a second cannot
write the store on every Msg. A Host (decisions 0664, 0686) appends each
Msg batch to its store and renders a view per version: at 60 inputs a
second from each of 30 players that is 1,800 appends a second for one
room, and a view rendered as HTML per player per version. A game needs
the opposite trade: the state lives in memory on one instance, steps on
a fixed clock whatever the inputs, and only a periodic snapshot is
durable (losing the last few seconds of a match on a crash is fine;
losing the room is not).

## Decision

`std/ui` gets rooms, separate from Hosts:

- `ui.game(init, step, view)`: a room's state `S` starts at
  `init(key)` (or the room's last snapshot), steps once per tick through
  `step(S, List[RoomEvent[I]]) -> S` with the tick's events
  (`Join(player)`, `Leave(player)`, `Input(player, input)`, in arrival
  order), and each player is shown `view(S, player) -> Bytes`.
- `ui.rooms(game, store, tasks, clock, hz: 20, snapshot_ms: 5000,
  idle_ms: 10000, auth:, room_of:)`: a room opens on its first
  connection, and a task in `tasks` steps it `hz` times a second until
  `idle_ms` after its last player leaves. Its state goes to `store` as a
  snapshot (`put_snapshot`, the Host's own snapshot slot) every
  `snapshot_ms` and when it closes, never per input; a room opened again
  starts from its snapshot. A tick that starts late is counted and the
  clock does not try to catch up (no burst of steps after a stall).
- Latest wins: each connection has a one slot outbox. A tick replaces
  what a slow connection has not sent yet, so a slow player skips to the
  newest view instead of queueing, and a slow player never slows the
  tick or another player.
- Frames are binary: a kind byte, the tick and the ack (the newest input
  seq of this connection the state includes, so a client can drop the
  inputs it predicted) as uvarints, then the full view, or a delta from
  the bytes this connection was last sent (the view's length, then runs
  of kept bytes, new byte count, new bytes). The smaller is sent; a game
  whose view is a fixed layout sends only the bytes that moved.
  `ui.room_frame` and `ui.room_view` are the encoder and decoder.
- Inputs are JSON text messages `{"s": seq, "i": input}`; a connection's
  first frame is always a full view.
- `auth(req) -> Str?` names the player (None answers 401); without it
  each connection is a player of its own. `room_of(name, player) ->
  Str?` maps the room a player asks for to a key (None answers 404).
- `Rooms.mount(router, prefix)` serves the websocket at
  `prefix/_xo/room?room=NAME` and the browser client at
  `prefix/_xo/room.js`: `xoRoom(url, onframe)` reconnects with backoff,
  rebuilds each view from its delta and calls `onframe(view, tick, ack)`
  with a Uint8Array; its `send(input)` queues while disconnected and
  answers the input's seq.
- `Rooms.stats()` (open rooms, ticks, step time, late ticks, snapshots)
  and `tick_of(key)`.

One instance serves a room: route a room's connections to one instance
(a sticky load balancer key on the room name) until rooms take a lease
(decision 0686 item 6), the next stage.

## Consequences

examples/ui-arena is a canvas game on rooms: dots moved by arrow keys
and a ball that bounces every tick (each tick changes every player's
view, the worst case for deltas), 32 slots, a 165 byte view (a frame
averages about 17 bytes at 20 Hz: 349 bytes a second a player). Its load
client (`examples/ui-arena/load`) opens ROOMS rooms of PER players, each
sending RATE moves a second, and reports the tick latency (the time
from an input to the first frame that acks it), frames and bytes a
second. The served `/_xo/room.js`, run in node 22 against the arena
server (2026-11-17), rebuilt every delta to the 165 byte view: two
players in one room each got 21 frames in a second at 20 Hz, three
moves of 10 moved a dot 30 units with ack 3, and the other player saw
both dots. The canvas drawing has not been checked in a browser yet
(the agent browser could not open a loopback origin).

Measured on the Mac Studio (2026-11-17), server and client on loopback,
4 players a room, 10 moves a second each, 1 ms between dials:

| hz | players | p50 | p99 | bytes/s a player | server CPU | late ticks | RSS |
|---|---|---|---|---|---|---|---|
| 20 | 400 | 25.2 ms | 50.0 ms | 349 | 3.2 s in 13 s | 0 | 52 MB |
| 20 | 2,000 | 25.3 ms | 49.9 ms | 373 | 11.9 s in 13.3 s | 0 | 239 MB |
| 20 | 4,000 | 25.7 ms | 50.3 ms | 338 | 26.2 s in 15.4 s | 0 | 406 MB |
| 60 | 1,000 | 8.8 ms | 17.2 ms | 826 | 11.9 s in 13 s | 0 | |
| 60 | 2,000 | 54 ms | 242 ms | | about 29 s in 13 s | 0 to 106 | |

At 20 Hz the latency is what the tick allows (half a tick at p50, one at
p99) up to 4,000 players on one instance. At 60 Hz one Mac instance
holds about 1,000 players at a tick's latency; at 2,000 the run varies
(a p99 of 242 ms to 3.2 s). Splitting the load across 2 and 4 client
processes made it worse (433 and 11,998 late ticks), so the server is
the limit. Its CPU profile at 60 Hz and 2,000 players: 53% in
`_ulock_wait` under the scheduler's idle worker park and wake, 35% in
`_sendto` (about 10 us a websocket frame on macOS loopback), 6% in
kevent, and under 3% in the rooms code (step, view, frames). The 60 Hz
ceiling is the runtime's syscall and wakeup cost, not the rooms design;
fewer wakeups (a writer that batches frames, workers that spin before
they park) are the next step there.

Measured on offload host 2 (linux/arm64, 18 CPUs, somaxconn 4096,
2026-11-17), the same runs:

| hz | players | p50 | p99 | max | bytes/s a player | server CPU | late ticks |
|---|---|---|---|---|---|---|---|
| 20 | 2,000 | 25.7 ms | 50.8 ms | 61 ms | 365 | 5.3 s in 13.7 s | 0 |
| 20 | 4,000 | 25.7 ms | 71.1 ms | 201 ms | 331 | 14.8 s in 15.7 s | 677 |
| 60 | 1,000 | 8.4 ms | 16.6 ms | 20 ms | 826 | 7.2 s in 13 s | 0 |
| 60 | 2,000 | 8.5 ms | 16.6 ms | 79 ms | 848 | 16.3 s in 13.3 s | 0 |
| 60 | 4,000 | 8.8 ms | 17.8 ms | 208 ms | 763 | 29.3 s in 15.7 s | 0 |

On Linux one instance holds 4,000 players at 60 Hz with a tick's
latency at p99 (no failed connection, no late tick), four times the
Mac's ceiling, which fits the Mac profile above (park and wake, and
sendto, cost less on Linux). Server and load client share the 18 CPUs
in these runs, so the server's own ceiling is higher. The 20 Hz 4,000
run had 677 late ticks and a 201 ms max where the 60 Hz run at the
same size had none; where in the run they fall (the dial ramp or
later) is not measured yet.

A burst of 2,000 dials at once overflows macOS's listen backlog of 128
and the kernel resets some connections (213 and 226 of 2,000 against
the Xo server; 540 and 626 against a Go net/http server under the same
burst); a 1 ms stagger loses none. The load client staggers by default.

Rooms keep a room's state in one task, so `step` and `view` need no
locks, `view` runs once a tick for each player (shared by that
player's connections), and the store sees one write per room every `snapshot_ms`.

Stages left: multi-instance rooms (a lease per room so any instance can
take a connection and forward it), client side prediction in wasm (the
ack is already in every frame), and interest management (a view per
player already allows it).

## Alternatives

- Rooms as a Host whose Msgs are inputs: every input an append and a
  version, and a view rendered per version per player; at 60 Hz a room
  of 30 is 1,800 appends a second. Rejected (the point of this step).
- JSON frames: easy to read in the browser, but every frame carries
  every field and its name, where a delta carries the bytes that moved.
- A full view every tick: simpler, but 165 bytes a frame against about
  17 in the arena, and the gap grows with the view.
- Queueing every frame to a slow connection: a stalled player's queue
  grows without bound, and it sees stale state when it recovers.
  Latest wins drops what it cannot use.
- Stepping on inputs instead of a clock: a fair game needs time to move
  for everyone (the ball moves with no input), and a fixed tick bounds
  the work per second whatever players send.
