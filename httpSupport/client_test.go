package httpSupport

import (
	"fmt"
	"goRPC"
	"net"
	"os"
	"runtime"
	"testing"
)

// XDial calls different functions to connect to a RPC server
func TestXDial(t *testing.T) {
	if runtime.GOOS == "linux" {
		ch := make(chan struct{})
		addr := "/tmp/rpc.sock"
		go func() {
			_ = os.Remove(addr)
			l, err := net.Listen("unix", addr)
			if err != nil {
				t.Fatal("failed to listen unix socket")
			}
			ch <- struct{}{}
			goRPC.Accept(l)
		}()
		<-ch
		_, err := goRPC.XDial("unix@" + addr)
		_assert(err == nil, "failed to connect unix socket")
	}
}
func _assert(condition bool, msg string, v ...interface{}) {
	if !condition {
		panic(fmt.Sprintf("assertion failed: "+msg, v...))
	}
}
