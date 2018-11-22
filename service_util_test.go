package service_util

import (
	"testing"
	"git.woda.ink/woda/pb/ComMessage"
	"fmt"
	"time"
)

//func TestMain(m *testing.M) {
//	code := m.Run()
//
//	os.Exit(code)
//}

//func TestGetWhiteServiceList(t *testing.T) {
//
//	GetWhiteServiceList("WDApp")
//	//if got == nil {
//	//	t.Errorf("got [%s] expected [%s]", got, nil)
//	//}
//}

//func TestRegistService(t *testing.T) {
//	_, err := RegistService("testing", 3456, nil)
//	if err != nil {
//		t.Errorf("expect nil but [%s]", err)
//	}
//}

func TestNewServiceUtilForSearch(t *testing.T) {
	NewServiceUtilForSearch()
	suSingleton = nil
}

func TestServiceUtil_RegistService(t *testing.T) {
	su, port := NewServiceUtil("Test", "C:\\etc\\woda\\modules\\TestService\\TestService.yaml")
	su.RegistService("Test", "Test")
	fmt.Printf("port:%d\n", port)
	//su.UnRegistAllService()
}

func TestGetServiceByName(t *testing.T) {
	su := GetInstance()
	list := su.GetServiceByNameInternal("WDGP_AuditHelper")
	for _, value := range list {
		fmt.Println(value.ServiceID)
	}
}

func TestGetServiceByNameUseCache(t *testing.T) { //覆盖率工具使用
	su := GetInstance()
	list := su.GetServiceByNameInternal("WDGP_AuditHelper")
	for _, value := range list {
		fmt.Println(value.ServiceID)
	}

	list = su.GetServiceByNameInternal("WDGP_AuditHelper")
	for _, value := range list {
		fmt.Println(value.ServiceID)
	}
}

func TestGetServiceByNameCrossSystem(t *testing.T) {
	su := GetInstance()
	list := su.GetServiceByNameCrossSystem(ComMessage.CBSType_Base, "WDGP_AuditHelper")
	for _, value := range list {
		fmt.Println(value.ServiceID)
	}
}

func TestReportFailedService(t *testing.T) {
	su := GetInstance()
	list := su.GetServiceByNameCrossSystem(ComMessage.CBSType_Own, "Aliyun")
	for _, value := range list {
		su.ReportFailedService(value.ServiceName, value.ServiceID)
	}

	time.Sleep(10*time.Second)
}

func TestServiceUtil_UnRegistServiceID(t *testing.T) {
	su := GetInstance()
	list := su.GetServiceByNameCrossSystem(ComMessage.CBSType_Own, "Aliyun")
	for _, value := range list {
		su.UnRegistServiceID(ComMessage.CBSType_Own, value.ServiceID)
	}

	suSingleton = nil
}