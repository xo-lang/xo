# A park timer that closes the descriptor ends an h2 connection that only wanted its ping

Status: Observed 2026-11-01

- Symptom: http_h2_config printed `keepalive: closed, ping seen false`: a parked h2 connection with a ping interval was closed when its timer fired, so the server never sent the PING.
- Cause: hserve.sweep_idle closes the descriptor of an expired parked connection, which is right for the h1 idle timeout (the resume sees a closed connection and ends) but wrong for an h2 timer, which is the connection's own (ping, deadline).
- Fix: the parked record's PC_KEEP flag (set for h2): the sweep unparks and fires the resume without closing; the resume's wait()/keepalive() decides. idle() returns -1 for a timer already due so the resume does not park again.
