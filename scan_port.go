package service_util

import (
	"net"
	"sort"
	"sync"
)

const (
	portNumbers = 50
)

var ip net.IP
var okList []int

//检查某个端口是否可用
func checkPort(ip net.IP, port int, wg *sync.WaitGroup) {
	defer wg.Done()
	tcpAddr := net.TCPAddr{
		IP:   ip,
		Port: port,
	}
	conn, err := net.DialTCP("tcp", nil, &tcpAddr)
	if err == nil {
		conn.Close()
	} else {
		okList = append(okList, port)
	}
}

//获取可用端口
func getUsablePort(initPort int) (retPort int) {

	ip = net.ParseIP("127.0.0.1")
	for {
		if initPort > 30000 {
			//只允许2W到3W范围内
			initPort = 20000
		}
		// once scan 50 ports
		if vaildPort, ok := scanPortRange(initPort, initPort+portNumbers); ok {
			return vaildPort
		}
		initPort = initPort + portNumbers + 1
	}
}

func scanPortRange(startPort, endPort int) (vPort int, getOk bool) {

	okList = okList[:0:0]
	// 用于协程任务控制
	wg := sync.WaitGroup{}
	wg.Add(endPort - startPort + 1)

	for i := startPort; i <= endPort; i++ {
		go checkPort(ip, i, &wg)
	}
	wg.Wait()

	sort.Ints(okList)

	loopEnd := len(okList)
	if loopEnd < 5 {
		return 0, false
	}

	for i := 0; i < loopEnd-5; i++ {
		if okList[i]%5 == 0 && okList[i+4] == okList[i]+4 {
			return okList[i], true
		}
	}
	return okList[0], false
}
