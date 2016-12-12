package smtp

import (
	"io"
	"net"
)

func AcceptConnection(conn net.Conn, server Server) error {
	conn.Close()
	return nil
}
