package server

import (
	"net"
	"testing"

	"golang.org/x/sys/unix"
)

func TestManagementListenerRejectsImplicitMPTCP(t *testing.T) {
	t.Setenv("GODEBUG", "multipathtcp=1")
	listener, err := listenTCP("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	raw, err := listener.(*net.TCPListener).SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var protocol int
	var socketErr error
	if err := raw.Control(func(fd uintptr) {
		protocol, socketErr = unix.GetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_PROTOCOL)
	}); err != nil {
		t.Fatal(err)
	}
	if socketErr != nil || protocol != unix.IPPROTO_TCP {
		t.Fatalf("management listener protocol=%d, error=%v; expected ordinary TCP", protocol, socketErr)
	}
}
