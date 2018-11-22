package service_util

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"git.woda.ink/woda/pb/ComMessage"
)

func TestWithConsulSrvAddr(t *testing.T) {
	su := GetInstanceWithOptions(WithConsulSrvAddr("192.168.0.1:8500"))
	assert.Equal(t, "192.168.0.1:8500", su.consulAddr, "unexpected consul agent addr")
	suSingleton = nil
}

func TestWithServiceName(t *testing.T) {
	su := GetInstanceWithOptions(WithServiceName("Microservice1"))
	assert.Equal(t, "Microservice1", su.serviceName, "unexpected service name")
	suSingleton = nil
}

func TestWithMultiDataCenter(t *testing.T) {
	options := []Option {
		WithMultiDataCenter(ComMessage.CBSType_Base, "base"),
		WithMultiDataCenter(ComMessage.CBSType_Woda, "woda"),
		WithMultiDataCenter(ComMessage.CBSType_JFF, "jff"),
		WithMultiDataCenter(ComMessage.CBSType_ZXX, "zxx"),
	}
	su := GetInstanceWithOptions(options...)
	assert.Containsf(t, su.dcItems, ComMessage.CBSType_Base, "not contain base item")
	assert.Containsf(t, su.dcItems, ComMessage.CBSType_Woda, "not contain woda item")
	assert.Containsf(t, su.dcItems, ComMessage.CBSType_JFF, "not contain jff item")
	assert.Containsf(t, su.dcItems, ComMessage.CBSType_ZXX, "not contain zxx item")

	suSingleton = nil
}