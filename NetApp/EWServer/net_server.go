package ewserver

import (
	"fmt"
	"net"
	"os"
)

func ServerHandleError(err error, when string) {
	if err != nil {
		fmt.Println(err, when)
		os.Exit(1)
	}
}

func ChatWith(conn net.Conn) {
	buffer := make([]byte, 1024)

	for {
		n, err := conn.Read(buffer)
		ServerHandleError(err, "conn.read buffer")

		clientMsg := string(buffer[0:n])
		fmt.Printf("recv msg:%s", clientMsg)

		if clientMsg != "im off" {
			conn.Write([]byte("read:" + clientMsg))
		} else {
			conn.Write([]byte("bye"))
			break
		}
	}

	conn.Close()
	fmt.Printf("client disconnect", conn.RemoteAddr())
}

func main() {
	listenter, err := net.Listen("tcp", "127.0.0.1:9000")
	ServerHandleError(err, "net.listen")

	for {
		conn, e := listenter.Accept()
		ServerHandleError(e, "listener.accept")
		go ChatWith(conn)
	}
}
