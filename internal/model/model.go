package model

import "fmt"

//============================================= Connection Key =======================================

type ConnectionKey struct {
	Local  string
	Remote string
	Key    string
}

func NewConnectionKey(local string, remote string) ConnectionKey {
	return ConnectionKey{
		local,
		remote,
		fmt.Sprintf("%s%s", local, remote),
	}
}

//============================================= Stream =============================================

type Stream struct {
	StreamId uint32
	ProxyMap map[string]Proxy
}

type StreamData struct {
	StubIp      string
	ServerIp    string
	ClientIp    string
	ServerPorts []uint32
	ClientPorts []uint32
	StreamState StreamState
}

type StreamState int

const (
	Create   StreamState = 0
	Play                 = 1
	Teardown             = 2
)

// ============================================= Proxy =============================================
type Proxy struct {
	ProxyIp     string
	StreamState StreamState
	Clients     []Client
}

type Client struct {
	ClientId string
	ClientIp string
	Port     uint32
}
