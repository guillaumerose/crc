package forwarder

import (
	"fmt"
	"net"

	log "github.com/sirupsen/logrus"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
	"gvisor.dev/gvisor/pkg/waiter"
)

func UDP(s *stack.Stack) *udp.Forwarder {
	return udp.NewForwarder(s, func(r *udp.ForwarderRequest) {
		outbound, err := net.Dial("udp", fmt.Sprintf("%s:%d", r.ID().LocalAddress, r.ID().LocalPort))
		if err != nil {
			log.Errorf("net.Dial() = %v", err)
			return
		}

		var wq waiter.Queue
		ep, tcpErr := r.CreateEndpoint(&wq)
		if tcpErr != nil {
			log.Errorf("r.CreateEndpoint() = %v", tcpErr)
			return
		}

		go pipe(gonet.NewUDPConn(s, &wq, ep), outbound)
	})
}

func pipe(conn1 net.Conn, conn2 net.Conn) {
	defer func() {
		_ = conn1.Close()
		_ = conn2.Close()
	}()
	chan1 := chanFromConn(conn1)
	chan2 := chanFromConn(conn2)

	for {
		select {
		case b1 := <-chan1:
			if b1 == nil {
				return
			}
			_, _ = conn2.Write(b1)
		case b2 := <-chan2:
			if b2 == nil {
				return
			}
			_, _ = conn1.Write(b2)
		}
	}
}

func chanFromConn(conn net.Conn) chan []byte {
	c := make(chan []byte)

	go func() {
		b := make([]byte, 1024)

		for {
			n, err := conn.Read(b)
			if n > 0 {
				res := make([]byte, n)
				copy(res, b[:n])
				c <- res
			}
			if err != nil {
				c <- nil
				break
			}
		}
	}()

	return c
}
