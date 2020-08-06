// server_app
package main

import (
	"fmt"

	"netapp/EWServer"
)

type ServerTest struct {
}

func (st *ServerTest) Logon(msg []byte, param *ewserver.LoginParam) int {
	return 1
}

func (st *ServerTest) PreLogoff(clientId int) {

}

func main() {
	fmt.Println("Hello World!")

	var session ServerTest
	wan_cfg := ewserver.NewWanConfig(&session)

	var server ewserver.WanServer
	server.Init(wan_cfg)

}
