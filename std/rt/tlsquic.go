package rt

// std/tls QUIC (decision 0644 T14): Go's QUICConn, as it is.

import (
	"crypto/tls"
	"strings"
)

// Tls_QuicConn is a std/tls QuicConn: Go's QUICConn.
type Tls_QuicConn struct {
	q *tls.QUICConn
}

func (Tls_QuicConn) XoDebug(b *strings.Builder) { b.WriteString("<quicconn>") }

var quicLevelNames = [...]string{"Initial", "Early", "Handshake", "Application"}

// M_display is Go's QUICEncryptionLevel.String.
func (l *Tls_QuicEncryptionLevel) M_display(*Task) string { return quicLevelNames[l.Tag-1] }

// Tls_quic_client is `tls.quic_client(config)`: Go's QUICClient.
func Tls_quic_client(tk *Task, cfg Tls_ClientConfig) Tls_QuicConn {
	return Tls_QuicConn{q: tls.QUICClient(&tls.QUICConfig{TLSConfig: clientTLSConfig(tk, cfg)})}
}

// Tls_quic_server is `tls.quic_server(config)`: Go's QUICServer.
func Tls_quic_server(tk *Task, cfg Tls_ServerConfig) Tls_QuicConn {
	gc := serverConfig(tk, cfg)
	setTickets(gc, cfg, nil)
	return Tls_QuicConn{q: tls.QUICServer(&tls.QUICConfig{TLSConfig: gc})}
}

// quicErr is a QUIC error as a TlsErr (Go's: it wraps an AlertError).
func quicErr(err error) Tls_TlsErr { return tlsErrOf(err.Error(), err) }

func quicResult(err error) Result[Unit, Tls_TlsErr] {
	if err != nil {
		return Err[Unit](quicErr(err))
	}
	return Ok[Unit, Tls_TlsErr](Unit{})
}

// M_start is `q.start()`: Go's Start with the task's context.
func (q *Tls_QuicConn) M_start(tk *Task) Result[Unit, Tls_TlsErr] {
	return quicResult(q.q.Start(GoContext(tk)))
}

// M_next_event is `q.next_event()`: Go's NextEvent, its data copied (Go's
// is valid only until the next call).
func (q *Tls_QuicConn) M_next_event(*Task) Tls_QuicEvent {
	e := q.q.NextEvent()
	ev := Tls_QuicEvent{
		F_kind:  Tls_QuicEventKind{Tag: int(e.Kind) + 1},
		F_level: Tls_QuicEncryptionLevel{Tag: int(e.Level) + 1},
		F_data:  Bytes(append([]byte{}, e.Data...)),
		F_suite: int64(e.Suite),
	}
	if e.Err != nil {
		ev.F_err = Option[Tls_TlsErr]{Some: true, V: quicErr(e.Err)}
	}
	return ev
}

// M_handle_data is `q.handle_data(level, data)`: Go's HandleData.
func (q *Tls_QuicConn) M_handle_data(_ *Task, level Tls_QuicEncryptionLevel, data Bytes) Result[Unit, Tls_TlsErr] {
	return quicResult(q.q.HandleData(tls.QUICEncryptionLevel(level.Tag-1), []byte(data)))
}

// M_send_session_ticket is `q.send_session_ticket(...)`: Go's
// SendSessionTicket.
func (q *Tls_QuicConn) M_send_session_ticket(_ *Task, early bool, extra List[Bytes]) Result[Unit, Tls_TlsErr] {
	var ex [][]byte
	for _, b := range extra.s {
		ex = append(ex, []byte(b))
	}
	return quicResult(q.q.SendSessionTicket(tls.QUICSessionTicketOptions{EarlyData: early, Extra: ex}))
}

// M_set_transport_parameters is Go's SetTransportParameters; empty is
// sent as empty (Go's nil and empty both are).
func (q *Tls_QuicConn) M_set_transport_parameters(_ *Task, params Bytes) {
	q.q.SetTransportParameters(append([]byte{}, params...))
}

// M_state is `q.state()`: Go's ConnectionState.
func (q *Tls_QuicConn) M_state(tk *Task) Tls_ConnState { return connStateOf(tk, q.q.ConnectionState()) }

// M_close is `q.close()`: Go's Close.
func (q *Tls_QuicConn) M_close(*Task) Result[Unit, Tls_TlsErr] { return quicResult(q.q.Close()) }
