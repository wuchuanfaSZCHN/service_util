package service_util

import (
	"errors"
	log "github.com/xiaomi-tc/log15"
	"google.golang.org/grpc"
)

const NotFound = "{\"Code\":990004,\"Desc\":\"未找到服务\"}"

var NotFoundErr = errors.New(NotFound)

type InvokeServiceRet struct {
	ret string
	err error
}

type ConnKVP struct {
	key   string
	value *grpc.ClientConn
}

var (
	map_servicesList      = make(map[string]*grpc.ClientConn)
	ch_inquiryServiceList = make(chan string)
	ch_inquiryServiceRet  = make(chan *grpc.ClientConn)
	ch_updateServiceList  = make(chan ConnKVP)

	ch_NodeAgentConnReq   = make(chan bool)
	ch_NodeAgentConnRsp   = make(chan *grpc.ClientConn)
	ch_NodeAgentConnSet   = make(chan bool)
)

//获取连接
func getServiceConn(targetAddr string) *grpc.ClientConn {
	ch_inquiryServiceList <- targetAddr
	ret := <-ch_inquiryServiceRet
	return ret
}

func updateServiceConn(kvp ConnKVP) {
	ch_updateServiceList <- kvp
}

func processServiceConn() {
	for {
		select {
		case key := <-ch_inquiryServiceList:
			ret := getServiceConnFunc(key)
			ch_inquiryServiceRet <- ret
		case kvp := <-ch_updateServiceList:
			updateServiceConnFunc(kvp)
		}
	}
}

func updateServiceConnFunc(kvp ConnKVP) {
	delete(map_servicesList, kvp.key)
	map_servicesList[kvp.key] = kvp.value
}

func getServiceConnFunc(key string) *grpc.ClientConn {
	value, _ := map_servicesList[key]
	return value
}

//获取Grpc连接
func GetGrpcConn(targetAddr string) (*grpc.ClientConn, error) {
	var ret *grpc.ClientConn
	var err error
	conn := getServiceConn(targetAddr)
	if conn == nil {
		conn, err = grpc.Dial(targetAddr, grpc.WithInsecure())
		if err != nil {
			ret = nil
			log.Error("InvokeRemoteService", "did not connect", err)
			return nil, NotFoundErr
		}
		kvp := ConnKVP{targetAddr, conn}
		updateServiceConn(kvp)
	}
	ret = conn

	return ret, nil
}
