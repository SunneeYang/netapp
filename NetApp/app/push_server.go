package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/panjf2000/ants/v2"
	"github.com/panjf2000/gnet"
)

type WanServer struct {
	*gnet.EventServer
	tick             time.Duration
	connectedSockets sync.Map

	config tagConfig

	client        []tagClient
	freeClient    chan *tagClient
	unauthClient  chan *tagClient
	destroyClient chan *tagClient
}

// 初始化
func (ws *WanServer) Init(param *tagConfig) {
	ws.config = *param
	ws.client = make([]tagClient, ws.config.maxLoad)
	ws.freeClient = make(chan *tagClient, ws.config.maxLoad)
	ws.unauthClient = make(chan *tagClient, ws.config.maxLoad)
	ws.destroyClient = make(chan *tagClient, ws.config.maxLoad)

	ants.NewPool(ws.config.maxLoad, ants.WithPreAlloc(true))

	for i := 0; i < ws.config.maxLoad; i++ {
		ws.freeClient <- &ws.client[i]
	}

	ants.Submit(func() {
		ws.RoutineUnauth()
	})

	log.Fatal(gnet.Serve(ws, fmt.Sprintf("tcp://:%d", ws.config.port), gnet.WithMulticore(true)))
}

func (ws *WanServer) RoutineSend(client *tagClient) {
	ws.connectedSockets.Range(func(key, value interface{}) bool {
		addr := key.(string)
		c := value.(gnet.Conn)
		c.AsyncWrite([]byte(fmt.Sprintf("heart beating to %s\n", addr)))
		return true
	})

	buffer := make([]byte, ws.config.maxTickSend)

}

func (ws *WanServer) RoutineUnauth() {
	for {
		client := <-ws.unauthClient

		if client.shutdown {
			continue
		}

		if client.clientId < 0 {
			diff := time.Now().Sub(client.connectTime).Microseconds() - ws.config.authTime
			if diff > 0 {
				client.connection.Close()
			} else {
				ws.unauthClient <- client
			}
		}
	}
}

// 实现EventHandler接口
func (ws *WanServer) OnInitComplete(srv gnet.Server) (action gnet.Action) {
	log.Printf("Push server is listening on %s (multi-cores: %t, loops: %d), "+
		"pushing data every %s ...\n", srv.Addr.String(), srv.Multicore, srv.NumEventLoop, ws.tick.String())
	return
}

func (ws *WanServer) OnOpened(c gnet.Conn) (out []byte, action gnet.Action) {
	log.Printf("Socket with addr: %s has been opened...\n", c.RemoteAddr().String())

	if len(ws.freeClient) <= 0 {
		c.Close()
		return
	}

	select {
	case client := <-ws.freeClient:
		client.clientId = -1
		client.shutdown = false
		client.connection = c
		client.connectTime = time.Now()
		client.recvSerial = 0
		client.sendQueue = make(chan []byte, ws.config.maxSend)
		client.recvQueue = make(chan []byte, ws.config.maxRecv)
		ws.unauthClient <- client
		ws.connectedSockets.Store(c, client)
		break

	default:
		c.Close()
		break
	}

	return
}

func (ws *WanServer) OnClosed(c gnet.Conn, err error) (action gnet.Action) {
	log.Printf("Socket with addr: %s is closing...\n", c.RemoteAddr().String())
	ws.connectedSockets.Delete(c.RemoteAddr().String())

	return
}

func (ws *WanServer) React(frame []byte, c gnet.Conn) (out []byte, action gnet.Action) {
	clientMsg := string(frame)
	fmt.Printf("recv:%s", clientMsg)

	connection, ok := ws.connectedSockets.Load(c)
	if !ok {
		c.Close()
		return
	}

	client := connection.(*tagClient)

	if client.shutdown {
		return
	}

	if client.clientId < 0 {
		if time.Now().Sub(client.connectTime).Microseconds() > int64(ws.config.authTime) {
			c.Close()
			return
		}

		client.clientId = ws.config.logHandler.Logon(frame)
		if client.clientId < 0 {
			c.Close()
			return
		}
	}

	select {
	case client.recvQueue <- frame:
		break
	default:
		c.Close()
	}

	return
}

func (ws *WanServer) OnShutdown(server gnet.Server) {
	log.Printf("Wan server shutdown\n")
}

func (ws *WanServer) PreWrite() {
	log.Printf("Wan server PreWrite\n")
}

func (ws *WanServer) Tick() (delay time.Duration, action gnet.Action) {
	log.Printf("Wan server Tick\n")
	return
}

type tagClient struct {
	clientId    int
	recvSerial  int
	connectTime time.Time

	sendQueue chan []byte
	recvQueue chan []byte

	connection gnet.Conn
	shutdown   bool
}

type LogHandler interface {
	Logon(msg []byte) int
	PreLogoff(clientId int)
}

type tagConfig struct {
	logHandler LogHandler

	port     int
	maxLoad  int
	authTime int64

	maxTickSend int

	maxSend int
	maxRecv int
}

func newTagConfig(logHandler LogHandler) *tagConfig {
	return &tagConfig{
		logHandler:  logHandler,
		port:        6008,
		maxLoad:     5000,
		authTime:    32 * 1000,
		maxTickSend: 4 * 1024 * 1024,
		maxSend:     5120,
		maxRecv:     128,
	}
}

func main() {
	push := &WanServer{tick: 10000000000}
	log.Fatal(gnet.Serve(push, "tcp://:9000", gnet.WithMulticore(true), gnet.WithTicker(true)))
}
