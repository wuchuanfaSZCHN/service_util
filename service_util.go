package service_util

import (
	"errors"
	"git.woda.ink/woda/pb/ComMessage"
	pbng "git.woda.ink/woda/pb/NodeAgent"
	"github.com/hashicorp/consul/api"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/xiaomi-tc/log15"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultConsulAddr    = "127.0.0.1:8500"
	NodeAgentAddr        = "127.0.0.1:30030"
	NodeAgentServiceName = "NodeAgent"
)

type ServiceUtil struct {
	serviceId   string //当前实例在consul中的标识
	serviceName string //服务名

	consulClient *api.Client //consul agent客户端对象
	consulAddr   string      //consul agent地址

	nodeAgentConn *grpc.ClientConn //nodeAgent连接句柄

	sm *SafeMap //服务列表缓存

	healthCheckPort    int
	srvPort            int
	regFlag            bool
	healthCheckFlag     bool

	serviceQuality map[string]int

	lastDay int //日志轮滚使用

	hostIP   string
	httpSvr *http.Server

	versionGenerator chan string
	dcItems          map[ComMessage.CBSType]string
}

var (
	suSingleton *ServiceUtil
	suLock      sync.Mutex
)

//仅用于服务发现，单例模式一目了然
func GetInstance() *ServiceUtil {
	if suSingleton == nil {
		suLock.Lock()
		defer suLock.Unlock()
		if suSingleton == nil {
			suSingleton = &ServiceUtil{
				consulAddr: defaultConsulAddr,
			}
			suSingleton.init()
		}
	}

	return suSingleton
}

//支持扩展性
func GetInstanceWithOptions(opts ...Option) *ServiceUtil {
	if suSingleton == nil {
		suLock.Lock()
		defer suLock.Unlock()
		if suSingleton == nil {
			suSingleton = &ServiceUtil{
				consulAddr: defaultConsulAddr,
			}

			for _, opt := range opts {
				opt(suSingleton)
			}
			suSingleton.init()
		}
	}

	return suSingleton
}

// Deprecated:
func NewServiceUtilWithConsulAgentPort(serviceName string, yamlPath string, consulAgentAddr string) (s *ServiceUtil, port int) {
	GetInstanceWithOptions(WithConsulSrvAddr(consulAgentAddr))
	return NewServiceUtil(serviceName, yamlPath)
}

func NewServiceUtilWithOptions(serviceName string, yamlPath string, ops ...Option) (u *ServiceUtil, port int) {
	ops = append(ops, WithServiceName(serviceName))
	GetInstanceWithOptions(ops...)
	return NewServiceUtil(serviceName, yamlPath)
}

func NewServiceUtil(serviceName string, yamlPath string) (u *ServiceUtil, port int) {
	u = GetInstanceWithOptions(WithServiceName(serviceName))
	if !u.healthCheckFlag { //尚未启动用于健康检查的http服务
		var err error
		port, err = getServiceConfigPort(yamlPath)
		if err != nil {
			checkErr(err, "NewServiceUtil", false)
		}
		var srvPort int
		//如果是AAS，则不更新端口号
		if serviceName != "AASManager" && serviceName != NodeAgentServiceName {
			srvPort = getUsablePort(port)
			if srvPort != port {
				setServiceConfigPort(yamlPath, srvPort)
			}
		} else {
			srvPort = port
		}

		u.srvPort = srvPort
		u.serviceName = serviceName

		if u.srvPort != 0 { //正常初始化时开监听
			//同一个进程只一个监控端口
			u.healthCheckPort = u.srvPort + 1
			go u.startService(":"+strconv.Itoa(u.healthCheckPort), u.serviceName)
		}
		u.srvPort = srvPort
		u.serviceName = serviceName
		u.healthCheckFlag = true
	}

	return u, u.srvPort
}

// Deprecated: Use GetInstance instead.
func NewServiceUtilForSearch() *ServiceUtil {
	return GetInstance()
}

func GetConsulclient() *api.Client {
	return GetInstance().consulClient
}

func (u *ServiceUtil) init() {
	u.sm = NewSafeMap()

	u.lastDay = time.Now().Day()

	u.serviceQuality = make(map[string]int)

	config := api.DefaultConfig()
	config.Address = u.consulAddr

	client, err := api.NewClient(config)
	if err != nil {
		log.Error("ServiceUtil init", "error", err)
		os.Exit(1)
	}
	u.consulClient = client

	if u.serviceName == NodeAgentServiceName {
		//NodeAgent和consul agent交互
		go u.nodeAgentTask()

	} else {
		//初始化与NodeAgent之间的gRPC连接
		u.nodeAgentConn = newNodeAgentConn()

		//services和NodeAgent交互
		go u.serviceTask()

		//services的conn检查
		//		go u.NodeAgentConnCheck()
		//services的conn的获取和变更
		go u.nodeAgentConnGetAndSet()
	}
}

//启动健康检查服务
func (u *ServiceUtil) startService(addr, serviceName string) {
	u.httpSvr = &http.Server{Addr: addr}
	http.HandleFunc("/"+serviceName+"/status", statusHandler)
	log.Info("start listen...", "addr", addr)
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		err := u.httpSvr.ListenAndServe()
		checkErr(err, "startService", true)
	}()
}

func (u *ServiceUtil) destroyUnnormalServices(port int) {
	serviceID := u.hostIP + "-" + strconv.Itoa(port)

	servicesData, _, err := u.consulClient.Health().State("any", &api.QueryOptions{})
	if err != nil {
		log.Error("destroyUnnormalServices", "err", err)
		return
	}
	for _, entry := range servicesData {
		if strings.Contains(entry.ServiceID, serviceID) {
			u.removeService(ComMessage.CBSType_Own, entry.ServiceID)
		}
	}
}

func (u *ServiceUtil) removeService(cbsType ComMessage.CBSType, serviceID string) {
	if cbsType != ComMessage.CBSType_Own {
		//跨系统发起服务注销，暂时只剔除本地缓存
		//认为大部分服务是用于系统内调用，只是兼顾提供系统间调用，如果系统间调用发现需要剔除其他consul dc上的失效实例，这个动作系统内也能发现
		log.Debug("removeService", "cbsType", cbsType, "serviceID", serviceID)
		u.delFromServiceMap(cbsType, serviceID)
		return
	}

	if u.consulClient == nil {
		log.Error("removeService consulClient nil")
		return
	}

	if err := u.consulClient.Agent().ServiceDeregister(serviceID); err != nil {
		log.Debug("removeService", "serviceID", serviceID, "error", err)
	} else {
		log.Debug("removeService", "serviceID", serviceID)
		u.delFromServiceMap(ComMessage.CBSType_Own, serviceID)
	}
}

//func (u *ServiceUtil) removeServiceCrossSystem(cbsType ComMessage.CBSType, serviceID string) {
//	if u.consulClient == nil {
//		log.Error("removeServiceCrossSystem", "error", "consulClient nil")
//		return
//	}
//
//	var dc string
//
//	if err := u.consulClient.Agent().ServiceDeregister(serviceID + "&dc=" + dc); err != nil {
//		log.Debug("UnRegistService", "serviceID", serviceID)
//		log.Error("UnRegistService", "error", err)
//	} else {
//		log.Debug("UnRegistService", "serviceID:", serviceID)
//		u.delFromServiceMap(cbsType, serviceID)
//	}
//}

func (u *ServiceUtil) delFromServiceMap(cbsType ComMessage.CBSType, serviceID string) {
	u.sm.eraseNodeByServiceID(cbsType, serviceID)
}

//func (u *ServiceUtil) unRegisterOccupyPort(port int) {
//	//正确注销没有被注销的服务
//	ip := u.hostIP
//	log.Debug("unRegisterOccupyPort", "ip", ip, "port", port)
//	u.sm.mutex.RLock()
//	xmap := u.sm.Map
//	for _, s := range xmap {
//		for _, x := range s.MapServiceID {
//			log.Debug("unRegisterOccupyPort", "service", x.ServiceID)
//			if x.Port == port && x.IP == ip {
//				u.UnRegistService(x.ServiceID)
//			}
//		}
//	}
//	u.sm.mutex.RUnlock()
//}

//执行服务注册，注册成功返回服务ID
//注册失败重试三次
func (u *ServiceUtil) RegistService(serviceName string, packageName string) (serviceID string, err error) {
	if u.srvPort == 0 {
		return "", errors.New("服务端口错误")
	}
	//判断只允许一个进程跑一个服务
	if u.regFlag == true {
		log.Error("RegistService", "error", "只允许一个进程跑一个服务")
		return "", errors.New("只允许一个进程跑一个服务")
	}

	if u.hostIP == "" {
		u.hostIP = GetHostIP() //服务注册时才需要本机ip
	}

	myServiceID := serviceName + "-" + u.hostIP + "-" + strconv.Itoa(u.srvPort)
	u.serviceId = myServiceID
	//剔除原来占用该端口的服务
	u.destroyUnnormalServices(u.srvPort)

	var tags = []string{packageName, "wdservice-" + u.hostIP + ":" + strconv.Itoa(u.healthCheckPort)}
	service := &api.AgentServiceRegistration{
		ID:      myServiceID,
		Name:    serviceName,
		Port:    u.srvPort,
		Address: u.hostIP,
		Tags:    tags,
		Check: &api.AgentServiceCheck{
			HTTP:     "http://" + u.hostIP + ":" + strconv.Itoa(u.healthCheckPort) + "/" + serviceName + "/status",
			Interval: "10s",
			Timeout:  "1s",
		},
	}

	if err := u.consulClient.Agent().ServiceRegister(service); err != nil {
		log.Error("RegistService", "error", err)
		if _, exist := u.serviceQuality[myServiceID]; exist {
			u.serviceQuality[myServiceID] = u.serviceQuality[myServiceID] + 1
			if u.serviceQuality[myServiceID] >= 3 {
				log.Debug("RegistService", "my_service_id", myServiceID, "service_quality", u.serviceQuality[myServiceID])
				log.Error("RegistService", "error", err)
				os.Exit(1)
			}
		} else {
			u.serviceQuality[myServiceID] = 1
		}
		time.Sleep(time.Second * 5)
		u.RegistService(serviceName, packageName)
	}

	log.Info("RegistService", "Registered service", serviceName, "tags", strings.Join(tags, ","))
	//discoverServices(false, service_name) //初始化发现服务

	u.regFlag = true
	return myServiceID, err
}

//注销当前进程所有服务
func (u *ServiceUtil) UnRegistAllService() {
	u.UnRegistService(u.serviceId)
}

//服务注销
func (u *ServiceUtil) UnRegistService(serviceID string) {
	u.removeService(ComMessage.CBSType_Own, serviceID)
	if err := u.httpSvr.Shutdown(nil); err != nil {
		log.Error("http_svr Shutdown", "error", err)
	}
}

//注销信息不一致的服务
func (u *ServiceUtil) UnRegistServiceID(cbsType ComMessage.CBSType, serviceID string) {
	u.removeService(cbsType, serviceID)
}

//提供对外的服务查询
//如果failed_service_id不为空，则记录并剔除该服务
// Deprecated: Use GetServiceByNameInternal instead.
func (u *ServiceUtil) GetServiceByName(serviceName string, failedServiceID string) []*ServiceInfo {
	return u.getServiceByTypeAndName(ComMessage.CBSType_Own, serviceName)
}

//获取服务列表，用于系统内调用
func (u *ServiceUtil) GetServiceByNameInternal(serviceName string) []*ServiceInfo {
	return u.getServiceByTypeAndName(ComMessage.CBSType_Own, serviceName)
}

//跨系统获取服务列表，用于系统间调用
func (u *ServiceUtil) GetServiceByNameCrossSystem(cbsType ComMessage.CBSType, serviceName string) []*ServiceInfo {
	return u.getServiceByTypeAndName(cbsType, serviceName)
}

func (u *ServiceUtil) getServiceByTypeAndName(cbsType ComMessage.CBSType, serviceName string) []*ServiceInfo {
	//利用map迭代器的无序--实现服务列表的随机顺序
	r := u.sm.getServiceInfoList(cbsType, serviceName)
	//还是要有一次洗牌操作
	r = randShuffle(r)

	if len(r) == 0 {
		//没有找到服务, 从NodeAgent服务获取服务列表
		strVersion, list := getServiceByNodeAgent(cbsType, serviceName)
		if strVersion == "" || len(list) == 0 {
			log.Error("getServiceByTypeAndName getServiceByNodeAgent null", "cbsType", cbsType, "serviceName", serviceName)
			return nil
		}

		//缓存列表
		u.sm.addNewNode(cbsType, serviceName, strVersion, list)
		return list
	}

	return r
}

//如果failedServiceID不为空，则记录并剔除该服务
func (u *ServiceUtil) ReportFailedService(serviceName string, failedServiceID string) {
	log.Debug("ReportFailedService", "serviceName", serviceName, "failedServiceID", failedServiceID)

	u.sm.mutex.Lock()
	defer u.sm.mutex.Unlock()

	sMap := u.sm.Map
	if _, ok := sMap[ComMessage.CBSType_Own]; ok {
		if _, ok := sMap[ComMessage.CBSType_Own][serviceName]; ok {
			for _, serviceInfo := range sMap[ComMessage.CBSType_Own][serviceName].MapServiceID {
				if serviceInfo.ServiceID == failedServiceID {
					if len(sMap[ComMessage.CBSType_Own][serviceName].MapServiceID) == 1 {
						delete(sMap[ComMessage.CBSType_Own], serviceName)
					} else {
						delete(sMap[ComMessage.CBSType_Own][serviceName].MapServiceID, failedServiceID)
						s := sMap[ComMessage.CBSType_Own][serviceName]
						s.ServiceVersion = "service cache changed"
					}
				}
			}
		}
	}
}

func (u *ServiceUtil) ReportFailedServiceWithCBSType(cbsType ComMessage.CBSType, serviceName string, failedServiceID string) {
	log.Debug("ReportFailedService", "cbsType", cbsType, "serviceName", serviceName, "failedServiceID", failedServiceID)

	u.sm.mutex.Lock()
	defer u.sm.mutex.Unlock()

	sMap := u.sm.Map
	if _, ok := sMap[cbsType]; ok { //对于绝大多数普通服务而言，依赖的服务列表不会很庞大，多级map的代价最多8*o(1)，优先避免做内存拷贝
		if _, ok := sMap[cbsType][serviceName]; ok {
			for _, serviceInfo := range sMap[cbsType][serviceName].MapServiceID {
				if serviceInfo.ServiceID == failedServiceID {
					if len(sMap[cbsType][serviceName].MapServiceID) == 1 {
						delete(sMap[cbsType], serviceName)
					} else {
						delete(sMap[cbsType][serviceName].MapServiceID, failedServiceID)
						s := sMap[cbsType][serviceName]
						s.ServiceVersion = "service cache changed"
					}
				}
			}
		}
	}
}

//获取Consul的KV列表
func (u *ServiceUtil) GetConsulKVs(prefix string, q *api.QueryOptions) (api.KVPairs, error) {
	list, _, err := u.consulClient.KV().List(prefix, q)
	if err == nil {
		return list, err
	} else {
		log.Error("GetConsulKVs", "error", err)
		return nil, err
	}
}

//获取白名单配置信息
func (u *ServiceUtil) GetWhiteServiceList(appKey string) map[string]string {
	var whiteList = make(map[string]string)

	list, err := u.GetConsulKVs(appKey, nil)
	if err != nil {
		log.Error("GetWhiteServiceList", "error", err)
	}
	for i := 0; i < len(list); i++ {
		if string(list[i].Value) == "" {
			continue
		} else {
			whiteList[list[i].Key] = string(list[i].Value)
		}
		log.Debug("GetWhiteServiceList", list[i].Key, list[i].Value)
	}
	return whiteList
}

func (u *ServiceUtil) InvokeService(serviceName string, callback func(conn *grpc.ClientConn) ([]byte, error)) (result []byte, err error) {
	var connService *grpc.ClientConn
	sList := u.GetServiceByName(serviceName, "")
	if len(sList) == 0 {
		log.Error("GetServiceByName result count = 0")
		return result, errors.New("GetServiceByName result count = 0")
	}

	connErr := errors.New("未连接")
	for _, value := range sList {
		strAddress := value.IP + ":" + strconv.Itoa(value.Port)
		log.Debug("GetServiceByName get service", "strAddress", strAddress)
		connService, connErr = grpc.Dial(strAddress, grpc.WithInsecure())

		if connErr != nil {
			log.Error("grpc.Dial error...", "error", err)
			u.ReportFailedService(serviceName, value.ServiceID)
			continue
		}

		result, err = callback(connService)
		if err != nil {
			u.ReportFailedService(serviceName, value.ServiceID)
		} else {
			break
		}
	}
	if connErr == nil {
		defer connService.Close()
	}

	return result, err
}

// code by kdjie, @2018.7.17 --------------------------------
func (u *ServiceUtil) GetServiceIP() string {
	return u.hostIP
}
func (u *ServiceUtil) GetServicePort() int {
	return u.srvPort
}
func (u *ServiceUtil) GetServiceName() string {
	return u.serviceName
}
func (u *ServiceUtil) GetServiceID() string {
	return u.serviceId
}

// code by kdjie, @2018.7.17 --------------------------------

func (u *ServiceUtil) GetServiceInfoByServiceName(cbsType ComMessage.CBSType, serviceName string) (string, []*ServiceInfo) {
	u.sm.mutex.RLock()
	defer u.sm.mutex.RUnlock()

	if m, ok := u.sm.Map[cbsType]; ok {
		if s, ok := m[serviceName]; ok {
			infoList := make([]*ServiceInfo, 0, len(s.MapServiceID))
			for _, v := range s.MapServiceID {
				infoList = append(infoList, v)
			}
			return s.ServiceVersion, infoList
		}
	}

	return "", nil
}

//是否已加入服务发现的任务中
func (u *ServiceUtil) HasSDTask(cbsType ComMessage.CBSType, serviceName string) bool {
	u.sm.mutex.RLock()
	defer u.sm.mutex.RUnlock()

	if m, ok := u.sm.Map[cbsType]; ok {
		if _, ok := m[serviceName]; ok {
			return true
		}
	}

	return false
}

func (u *ServiceUtil) nodeAgentTask() {
	u.versionGenerator = generateVersion()

	ticker := time.NewTicker(time.Second * 5)
	for {
		select {
		case <-ticker.C:
			u.doDiscoveryService()
			u.doDiscoveryServiceOtherDC()
			u.logRotate() //本地日志按照日期轮滚
		}
	}
}

func (u *ServiceUtil) logRotate() {
	today := time.Now().Day()
	if today != u.lastDay {
		log.LogRotate()
		u.lastDay = today
	}
}

func (u *ServiceUtil) serviceTask() {
	ticker := time.NewTicker(time.Second * 5)
	for {
		select {
		case <-ticker.C:
			u.doDiscoveryServiceByNodeAgent()
			u.logRotate() //本地日志按照日期轮滚
		}
	}
}

//func (u *ServiceUtil) NodeAgentConnCheck(){
//	t := time.NewTicker(time.Second * 5)
//	for {
//		select {
//		case <-t.C:
//			{
//				if getNodeAgentConn() == nil{
//					log.Info("NodeAgentConn is nil")
//					setNodeAgentConn(newNodeAgentConn())
//				}
//			}
//		}
//	}
//}

func (u *ServiceUtil) nodeAgentConnGetAndSet() {
	for {
		select {
		case <-ch_NodeAgentConnReq:
			if u.nodeAgentConn == nil {
				u.nodeAgentConn = newNodeAgentConn()
			}
			ch_NodeAgentConnRsp <- u.nodeAgentConn
		case <-ch_NodeAgentConnSet:
			if u.nodeAgentConn != nil {
				u.nodeAgentConn.Close()
				u.nodeAgentConn = nil
				//此处不立即重新建立连接
			}
		}
	}
}

//获取当前dc上所有的服务列表
func (u *ServiceUtil) doDiscoveryService() {
	bRest, listMap := u.getAllPassingServices()
	if !bRest {
		return
	}

	//移除多余项
	u.sm.mutex.Lock()
	if m, ok := u.sm.Map[ComMessage.CBSType_Own]; ok {
		for serviceName := range m {
			if _, exist := listMap[serviceName]; !exist {
				log.Debug("doDiscoveryService delete service", "serviceName", serviceName)
				delete(u.sm.Map[ComMessage.CBSType_Own], serviceName)
			}
		}
	}
	u.sm.mutex.Unlock()

	//更新最新列表
	for serviceName := range listMap {
		m := u.sm.getServiceInfoMap(ComMessage.CBSType_Own, serviceName)
		if len(m) > 0 {
			if isNeedChangeVersion(m, listMap[serviceName].MapServiceID) {
				newVersion := <-u.versionGenerator
				log.Info("doDiscoveryService service version update", "serviceName", serviceName, "newVersion", newVersion)
				u.sm.addNewNode2(ComMessage.CBSType_Own, serviceName, newVersion, listMap[serviceName].MapServiceID)
			}
		} else {
			version := <-u.versionGenerator
			log.Info("doDiscoveryService service version add", "serviceName", serviceName, "version", version)
			u.sm.addNewNode2(ComMessage.CBSType_Own, serviceName, version, listMap[serviceName].MapServiceID)
		}
	}
}

//获取其他dc上需要的服务列表
func (u *ServiceUtil) doDiscoveryServiceOtherDC() {
	serviceNameMap := u.sm.getServiceNameOtherDC()
	if serviceNameMap == nil {
		return
	}

	for cbsType, serviceNameList := range serviceNameMap {

		for _, serviceName := range serviceNameList {
			if serviceName == "" {
				continue
			}

			newItems, err := u.GetServiceListOtherDC(cbsType, serviceName, false)
			if err != nil {
				continue
			}

			oldItems := u.sm.getServiceInfoMap(cbsType, serviceName)
			if isNeedChangeVersion(oldItems, newItems) {
				newVersion := <-u.versionGenerator
				log.Info("doDiscoveryServiceOtherDC service version update", "cbsType", cbsType, "serviceName", serviceName, "newVersion", newVersion)
				u.sm.addNewNode2(cbsType, serviceName, newVersion, newItems)
			}
		}
	}
}

func (u *ServiceUtil) GetServiceListOtherDC(cbsType ComMessage.CBSType, serviceName string, updateFlag bool) (map[string]*ServiceInfo, error) {
	if _, ok := u.dcItems[cbsType]; !ok {
		log.Info("GetServiceListOtherDC dc item not found", "cbsType", cbsType)
		return nil, nil
	}
	dcName := u.dcItems[cbsType]

	healthChecks, _, err := u.consulClient.Health().Checks(serviceName, &api.QueryOptions{
		Datacenter: dcName,
	})
	if err != nil {
		log.Error("GetServiceListOtherDC health check", "error", err)
		return nil, err
	}

	items := make(map[string]*ServiceInfo, len(healthChecks))

	for _, entry := range healthChecks {
		if entry.Status != "passing" {
			continue
		}

		sInfo := new(ServiceInfo)
		sInfo.ServiceName = entry.ServiceName
		sInfo.ServiceID = entry.ServiceID
		if len(entry.ServiceTags) > 0 {
			sInfo.PackageName = entry.ServiceTags[0]
		}

		strIP, nPort := getBaseInfoWithServiceID(sInfo.ServiceName, sInfo.ServiceID)
		if strIP == "" || nPort == 0 {
			continue
		}
		sInfo.IP = strIP
		sInfo.Port = nPort
		sInfo.Status = entry.Status

		items[sInfo.ServiceID] = sInfo
	}

	if updateFlag && len(items) > 0 {
		//更新缓存
		version := <-u.versionGenerator
		u.sm.addNewNode2(cbsType, serviceName, version, items)
	}

	return items, nil
}

func (u *ServiceUtil) doDiscoveryServiceByNodeAgent() {
	listCheck := u.sm.getServiceNameList()
	if len(listCheck) == 0 {
		return
	}

	conn := getNodeAgentConn()
	if conn == nil {
		log.Error("doDiscoveryServiceByNodeAgent getNodeAgentConn() nil")
		return
	}

	stReq := &pbng.ServicesChangeListReq{}
	stReq.CheckList = listCheck

	stRsp := &pbng.ServicesChangeListRsp{}

	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	err := grpc.Invoke(ctx, "NodeAgent.NodeAgent/Node_Agent_GetServicesChangeList", stReq, stRsp, conn)
	if err != nil {
		log.Error("doDiscoveryService grpc.Invoke Node_Agent_GetServicesChangeList", "error", err)

		//TODO 粗暴的认为gRPC调用失败就是链路原因
		closeNodeAgentConn()
		return
	}

	if len(stRsp.ChangeList) == 0 {
		return
	}

	for _, v := range stRsp.ChangeList {
		if v.ServiceVersion == "" {
			log.Info("delete no exist service ...", "v.ServiceName", v.ServiceName)
			u.sm.eraseNodeByServiceName(v.CbsType, v.ServiceName)
			continue
		}

		var ListTemp []*ServiceInfo
		for _, value := range v.Info {
			stTemp := new(ServiceInfo)
			stTemp.ServiceName = v.ServiceName
			stTemp.ServiceID = value.ServiceID
			stTemp.IP = value.ServiceIP
			stTemp.Port = int(value.ServicePort)
			stTemp.PackageName = value.PackageName
			stTemp.Status = value.ServiceStatus
			ListTemp = append(ListTemp, stTemp)
		}
		log.Debug("sm.addNewNode ...", "v.ServiceName", v.ServiceName, "v.ServiceVersion", v.ServiceVersion, "ListTemp", ListTemp)
		u.sm.addNewNode(ComMessage.CBSType_Own, v.ServiceName, v.ServiceVersion, ListTemp)
	}
}

//获取所有passing状态的服务
func (u *ServiceUtil) getAllPassingServices() (bool, map[string]*ServiceVersionInfo) {
	healthChecks, _, err := u.consulClient.Health().State("passing",
		&api.QueryOptions{
			WaitTime: 3 * time.Second,
		})
	if err != nil || len(healthChecks) == 0 {
		log.Error("getAllPassingServices", "err", err)
		return false, nil
	}

	listMap := make(map[string]*ServiceVersionInfo, len(healthChecks))

	for _, entry := range healthChecks {
		sInfo := new(ServiceInfo)
		sInfo.ServiceName = entry.ServiceName
		sInfo.ServiceID = entry.ServiceID
		if len(entry.ServiceTags) > 0 {
			sInfo.PackageName = entry.ServiceTags[0]
		}

		strIP, nPort := getBaseInfoWithServiceID(sInfo.ServiceName, sInfo.ServiceID)
		if strIP == "" || nPort == 0 {
			continue
		}
		sInfo.IP = strIP
		sInfo.Port = nPort
		sInfo.Status = entry.Status

		if _, exist := listMap[entry.ServiceName]; exist {
			listMap[entry.ServiceName].MapServiceID[sInfo.ServiceID] = sInfo
		} else {
			stInfo := &ServiceVersionInfo{}
			stInfo.MapServiceID = make(map[string]*ServiceInfo)
			stInfo.MapServiceID[sInfo.ServiceID] = sInfo
			listMap[entry.ServiceName] = stInfo
		}
	}
	return true, listMap
}

func getServiceByNodeAgent(cbsType ComMessage.CBSType, serviceName string) (version string, list []*ServiceInfo) {
	conn := getNodeAgentConn()
	if conn == nil {
		log.Error("getServiceByNodeAgent getNodeAgentConn() nil")
		return
	}

	stReq := &pbng.SingleServiceListReq{}
	stReq.ServiceName = serviceName
	stReq.CbsType = cbsType

	stRsp := &pbng.ServicesList{}

	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	err := grpc.Invoke(ctx, "NodeAgent.NodeAgent/Node_Agent_GetSingleServiceList", stReq, stRsp, conn)
	if err != nil {
		log.Error("getServiceByNodeAgent grpc.Invoke", "error", err)
		closeNodeAgentConn()
		return
	}

	//log.Debug("getServiceByNodeAgent grpc.Invoke result", "stRsp", stRsp)

	if stRsp.ServiceName == serviceName && stRsp.ServiceVersion != "" && len(stRsp.Info) > 0 {
		list = make([]*ServiceInfo, len(stRsp.Info))
		for k, value := range stRsp.Info {
			list[k] = &ServiceInfo{
				ServiceName: serviceName,
				ServiceID:   value.ServiceID,
				IP:          value.ServiceIP,
				Port:        int(value.ServicePort),
				Status:      value.ServiceStatus,
				PackageName: value.PackageName,
			}
		}
		version = stRsp.ServiceVersion
	}

	return
}

func newNodeAgentConn() *grpc.ClientConn {
	target := NodeAgentAddr
	conn, err := grpc.Dial(target, grpc.WithInsecure())
	if err != nil {
		log.Error("newNodeAgentConn grpc.Dial", "target", target, "error", err)
		return nil
	}

	return conn
}

func getNodeAgentConn() *grpc.ClientConn {
	ch_NodeAgentConnReq <- true
	return <-ch_NodeAgentConnRsp
}

func closeNodeAgentConn() {
	ch_NodeAgentConnSet <- true
}

func getBaseInfoWithServiceID(strServiceName, strServiceId string) (string, int) {
	listStr := strings.Split(strServiceId, "-")
	if len(listStr) == 3 && listStr[0] == strServiceName && listStr[1] != "" && listStr[2] != "" {
		nPort, _ := strconv.Atoi(listStr[2])
		if nPort > 0 {
			return listStr[1], nPort
		}
	}

	return "", 0
}

func isNeedChangeVersion(before, after map[string]*ServiceInfo) bool {
	for key, value := range before {
		if _, ok := after[key]; ok {
			if value.ServiceName != after[key].ServiceName || value.ServiceID != after[key].ServiceID ||
				value.IP != after[key].IP || value.Port != after[key].Port {
				return true
			}
		} else {
			return true
		}
	}

	for key, value := range after {
		if _, ok := before[key]; ok {
			if value.ServiceName != before[key].ServiceName || value.ServiceID != before[key].ServiceID ||
				value.IP != before[key].IP || value.Port != before[key].Port {
				return true
			}
		} else {
			return true
		}
	}

	return false
}
