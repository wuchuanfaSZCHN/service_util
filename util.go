package service_util

import (
	"os"
	"net"
	"fmt"
	"net/http"
	"github.com/satori/go.uuid"
	log "github.com/xiaomi-tc/log15"
)

//简单的生成器，带缓冲区的channel是为了提升并发能力
//毕竟这里是定时任务发起的，并发量最大等于本dc中的服务实例数
func generateVersion() chan string {
	ch := make(chan string, 10)
	go func() {
		for {
			ch <- uuid.NewV4().String() //对uuid的唯一性还是有问题的，这个库只是一个随机数，用在这里显然不合适
		}
	}()
	return ch
}

func checkErr(err error, errMessage string, isQuit bool) {
	if err != nil {
		log.Error(errMessage, "error", err)
		if isQuit {
			os.Exit(1)
		}
	}
}

//获取本机局域网IP地址
func GetHostIP() string {
	return getHostIP()
}

func getHostIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		os.Stderr.WriteString("Oops:" + err.Error())
		os.Exit(1)
	}
	for _, a := range addrs {
		//判断是否正确获取到IP
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && !ipnet.IP.IsLinkLocalMulticast() && !ipnet.IP.IsLinkLocalUnicast() {
			//fmt.Println(ipnet.IP.String())
			log.Info("getHostIP", "ip", ipnet)
			if ipnet.IP.To4() != nil {
				//os.Stdout.WriteString(ipnet.IP.String() + "\n")
				return ipnet.IP.String()
			}
		}
	}

	os.Stderr.WriteString("No Networking Interface Err!")
	log.Error("getHostIP", "error", err)
	os.Exit(1)
	return ""
}

//状态检查服务
func statusHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "status ok!")
}
