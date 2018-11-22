package service_util

import (
	"sync"
	"git.woda.ink/woda/pb/ComMessage"
	pbng "git.woda.ink/woda/pb/NodeAgent"
)

type ServiceInfo struct {
	ServiceID   string
	ServiceName string
	IP          string
	Port        int
	Status      string
	PackageName string
	Load        int
	Times       int   //被调用次数
	Timestamp   int64 //load updated ts
}

type ServiceVersionInfo struct {
	ServiceVersion string
	MapServiceID   map[string]*ServiceInfo
}

type ServiceMap map[ComMessage.CBSType]map[string]*ServiceVersionInfo

type SafeMap struct {
	mutex sync.RWMutex
	Map   ServiceMap
}

func NewSafeMap() *SafeMap {
	sm := new(SafeMap)
	sm.Map = make(ServiceMap)
	return sm
}

func (sm *SafeMap) getServiceNameList() []*pbng.ServicesList {
	var listTemp []*pbng.ServicesList
	sm.mutex.RLock()
	for cbsType := range sm.Map {
		for sName := range sm.Map[cbsType] {
			stTemp := new(pbng.ServicesList)
			stTemp.CbsType = cbsType
			stTemp.ServiceName = sName
			stTemp.ServiceVersion = sm.Map[cbsType][sName].ServiceVersion
			listTemp = append(listTemp, stTemp)
		}
	}
	sm.mutex.RUnlock()
	return listTemp
}

func (sm *SafeMap) Exist(cbsType ComMessage.CBSType, serviceName string) bool {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	if _, ok := sm.Map[cbsType]; ok {
		if _, ok = sm.Map[cbsType][serviceName]; ok {
			return true
		}
	}

	return false
}

func (sm *SafeMap) getServiceInfoList(cbsType ComMessage.CBSType, serviceName string) []*ServiceInfo {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	if subMap, ok := sm.Map[cbsType]; ok {
		if serviceIDMap, ok := subMap[serviceName]; ok {
			slice := make([]*ServiceInfo, 0, len(serviceIDMap.MapServiceID)) //一次分配好空间，避免动态扩容
			for _, v := range serviceIDMap.MapServiceID {
				slice = append(slice, v)
			}
			return slice
		}
	}

	return nil
}

func (sm *SafeMap) getServiceInfoMap(cbsType ComMessage.CBSType, serviceName string) map[string]*ServiceInfo {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	if subMap, ok := sm.Map[cbsType]; ok {
		if serviceIDMap, ok := subMap[serviceName]; ok {
			m := make(map[string]*ServiceInfo, len(serviceIDMap.MapServiceID)) //一次分配好空间，避免动态扩容
			for serviceID, v := range serviceIDMap.MapServiceID {
				m[serviceID] = v
			}
			return m
		}
	}

	return nil
}

func (sm *SafeMap) getServiceNameOtherDC() map[ComMessage.CBSType][]string {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	retM := make(map[ComMessage.CBSType][]string)
	for cbsType := range sm.Map {
		if cbsType != ComMessage.CBSType_Own {
			serviceNames := make([]string, 0, len(sm.Map[cbsType]))
			for serviceName := range sm.Map[cbsType] {
				serviceNames = append(serviceNames, serviceName)
			}
			retM[cbsType] = serviceNames
		}
	}

	return retM
}

func (sm *SafeMap) addNewNode(cbsType ComMessage.CBSType, serviceName, serviceVersion string, serviceList []*ServiceInfo) {
	stTemp := &ServiceVersionInfo{}
	stTemp.ServiceVersion = serviceVersion
	stMap := make(map[string]*ServiceInfo)
	for _, value := range serviceList {
		stMap[value.ServiceID] = value
	}
	stTemp.MapServiceID = stMap

	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if _, ok := sm.Map[cbsType]; ok {
		sm.Map[cbsType][serviceName] = stTemp
	} else {
		sm.Map[cbsType] = map[string]*ServiceVersionInfo{serviceName: stTemp}
	}
}

func (sm *SafeMap) addNewNode2(cbsType ComMessage.CBSType, serviceName, serviceVersion string, mapList map[string]*ServiceInfo) {
	stTemp := &ServiceVersionInfo{}
	stTemp.ServiceVersion = serviceVersion
	stTemp.MapServiceID = mapList

	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if _, ok := sm.Map[cbsType]; ok {
		sm.Map[cbsType][serviceName] = stTemp
	} else {
		sm.Map[cbsType] = map[string]*ServiceVersionInfo{serviceName: stTemp}
	}
}

func (sm *SafeMap) eraseNodeByServiceName(cbsType ComMessage.CBSType, serviceName string) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if _, ok := sm.Map[cbsType]; ok {
		for sName := range sm.Map[cbsType] {
			if sName == serviceName {
				delete(sm.Map[cbsType], sName)
			}
		}
	}
}

func (sm *SafeMap) eraseNodeByServiceID(cbsType ComMessage.CBSType, serviceID string) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if _, ok := sm.Map[cbsType]; ok {
		for sName, serviceIDMap := range sm.Map[cbsType] {
			for sID := range serviceIDMap.MapServiceID {
				if sID == serviceID {
					delete(sm.Map[cbsType][sName].MapServiceID, sID)
				}
			}
		}
	}
}
