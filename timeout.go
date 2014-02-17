package httphack

import (
	"net"
	"time"
)

// TimeoutConn wraps a connection to establish a read/write deadline
// on every write, and reset it on every read.
//
// WARNING: The timeout logic underneath TimeoutConn may not work
// if the server responds to a request being written before the
// request payload is done being written. This will be observed
// as an undue timeout on the read channel.
type TimeoutConn struct {
	net.Conn
	timeout time.Duration
}

func NewTimeoutConn(conn net.Conn, timeout time.Duration) *TimeoutConn {
	return &TimeoutConn{conn, timeout}
}

func (c *TimeoutConn) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	c.Conn.SetReadDeadline(time.Time{})
	return
}

func (c *TimeoutConn) Write(b []byte) (n int, err error) {
	c.Conn.SetDeadline(time.Now().Add(c.timeout))
	return c.Conn.Write(b)
}
