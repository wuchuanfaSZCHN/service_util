package service_util

import "git.woda.ink/woda/pb/ComMessage"

type Option func(u *ServiceUtil)

func WithConsulSrvAddr(consulSrvAddr string) Option {
	return func(u *ServiceUtil) {
		u.consulAddr = consulSrvAddr
	}
}

func WithServiceName(serviceName string) Option {
	return func(u *ServiceUtil) {
		u.serviceName = serviceName
	}
}

func WithMultiDataCenter(cbsType ComMessage.CBSType, dcName string) Option {
	return func(u *ServiceUtil) {
		if u.dcItems == nil {
			u.dcItems = make(map[ComMessage.CBSType]string, 4)
		}
		u.dcItems[cbsType] = dcName
	}
}
