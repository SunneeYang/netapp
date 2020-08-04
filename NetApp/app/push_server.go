package main

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/panjf2000/ants/v2"
	"github.com/panjf2000/gnet"
)

type WanServer struct {
	*gnet.EventServer
	tick             time.Duration
	connectedSockets sync.Map
	unauthClient     sync.Map

	config tagConfig

	client        []tagClient
	freeClient    chan *tagClient
	destroyClient chan *tagClient
}

// 初始化
func (ws *WanServer) Init(param *tagConfig) {
	ws.config = *param
	ws.client = make([]tagClient, ws.config.maxLoad)
	ws.freeClient = make(chan *tagClient, ws.config.maxLoad)
	ws.destroyClient = make(chan *tagClient, ws.config.maxLoad)

	ants.NewPool(ws.config.maxLoad, ants.WithPreAlloc(true))

	for i := 0; i < ws.config.maxLoad; i++ {
		client := &ws.client[i]
		client.clientId = -1
		client.connection = nil
		client.connectTime = 0
		client.recvSerial = 0
		client.shutdown = 0
		client.sendQueue = make(chan []byte, ws.config.maxSend)
		client.recvQueue = make(chan []byte, ws.config.maxRecv)
		ws.freeClient <- &ws.client[i]
	}

	ants.Submit(func() {
		ws.RoutineUnauth()
	})

	ants.Submit(func() {
		ws.RoutineDestroy()
	})

	log.Fatal(gnet.Serve(ws, fmt.Sprintf("tcp://:%d", ws.config.port), gnet.WithMulticore(true)))
}

// 销毁
func (ws *WanServer) Destroy() {
	ants.Release()
}

// 发送
func (ws *WanServer) Send(client *tagClient, msg []byte) {
	if atomic.LoadInt32(&client.shutdown) != 0 {
		return
	}

	client.connection.AsyncWrite(msg)

	/*
		select {
		case client.sendQueue <- msg:
			break

		default:
			ws.DisconnectClient(client, 0)
		}
	*/
}

// 接收
func (ws *WanServer) Recv(client *tagClient) []byte {
	if atomic.LoadInt32(&client.shutdown) != 0 {
		return nil
	}

	return <-client.recvQueue
}

// 断开
func (ws *WanServer) Kick(client *tagClient) {
	ws.DisconnectClient(client, 0)
}

func (ws *WanServer) RoutineUnauth() {

	ws.unauthClient.Range(func(key, value interface{}) bool {
		client := value.(*tagClient)

		if atomic.LoadInt32(&client.shutdown) != 0 {
			return true
		}

		if time.Now().UnixNano()-client.connectTime < ws.config.authTime {
			return true
		}

		ws.DisconnectClient(client, 0)
		ws.unauthClient.Delete(key)

		return true
	})
}

func (ws *WanServer) RoutineDestroy() {

	for {
		client := <-ws.destroyClient

		if client.clientId > 0 {
			ws.config.logHandler.PreLogoff(client.clientId)
			client.clientId = -1
		}

		ws.DestroyClient(client)
	}
}

func (ws *WanServer) DisconnectClient(client *tagClient, reason int) {
	if !atomic.CompareAndSwapInt32(&client.shutdown, 0, 1) {
		return
	}

	ws.destroyClient <- client
}

func (ws *WanServer) DestroyClient(client *tagClient) {
	// 删除认证
	ws.unauthClient.Delete(client.connection)

	// 丢弃所有包
	/*
		clear := false
		for {
			if clear {
				break
			}

			select {
			case <-client.sendQueue:
				break
			default:
				clear = true
				break
			}
		}
	*/

	clear := false
	for {
		if clear {
			break
		}

		select {
		case <-client.recvQueue:
			break
		default:
			clear = true
			break
		}
	}

	// 销毁
	client.connection.Close()

	// 重置
	client.clientId = -1
	client.connection = nil
	client.connectTime = 0
	client.recvSerial = 0
	atomic.StoreInt32(&client.shutdown, 0)

	// 回收
	ws.freeClient <- client
}

// 实现EventHandler接口
func (ws *WanServer) OnInitComplete(srv gnet.Server) (action gnet.Action) {
	log.Printf("Push server is listening on %s (multi-cores: %t, loops: %d), "+
		"pushing data every %s ...\n", srv.Addr.String(), srv.Multicore, srv.NumEventLoop, ws.tick.String())
	return
}

func (ws *WanServer) OnOpened(c gnet.Conn) (out []byte, action gnet.Action) {
	log.Printf("Socket with addr: %s has been opened...\n", c.RemoteAddr().String())

	select {
	case client := <-ws.freeClient:
		client.clientId = -1
		client.connection = c
		client.connectTime = int64(time.Now().UnixNano() / 1000)
		client.recvSerial = 0
		atomic.StoreInt32(&client.shutdown, 0)
		ws.unauthClient.Store(c, client)
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

	ws.connectedSockets.Delete(c)
	ws.unauthClient.Delete(c)

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

	if atomic.LoadInt32(&client.shutdown) != 0 {
		return
	}

	if client.clientId < 0 {
		if time.Now().UnixNano()-client.connectTime > ws.config.authTime {
			ws.DisconnectClient(client, 0)
			return
		}

		var param tagLoginParam
		param.client = client
		param.addr = c.RemoteAddr().String()

		client.clientId = ws.config.logHandler.Logon(frame, &param)
		if client.clientId < 0 {
			ws.DisconnectClient(client, 0)
			return
		}
	}

	select {
	case client.recvQueue <- frame:
		break
	default:
		ws.DisconnectClient(client, 0)
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
	connectTime int64

	sendQueue chan []byte
	recvQueue chan []byte

	connection gnet.Conn
	shutdown   int32
}

type tagLoginParam struct {
	client *tagClient
	addr   string
}

type LogHandler interface {
	Logon(msg []byte, param *tagLoginParam) int
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
		authTime:    32 * 1000 * 1000,
		maxTickSend: 4 * 1024 * 1024,
		maxSend:     5120,
		maxRecv:     128,
	}
}

func main() {
	push := &WanServer{tick: 10000000000}
	log.Fatal(gnet.Serve(push, "tcp://:9000", gnet.WithMulticore(true), gnet.WithTicker(true)))
}
