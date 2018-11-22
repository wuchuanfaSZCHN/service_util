package service_util

import (
	"sort"
	"time"
	"math/rand"
)

type ServiceInfoList []*ServiceInfo

//排序规则：首先按Load排序（由小到大），Load相同时按Timestamp进行排序
func (list ServiceInfoList) Len() int {
	return len(list)
}

func (list ServiceInfoList) Less(i, j int) bool {
	if list[i].Status == "passing" && list[j].Status == "critical" {
		return true
	}
	if list[i].Status == "critical" && list[j].Status == "passing" {
		return false
	}
	if list[i].Load < list[j].Load {
		return true
	} else if list[i].Load > list[j].Load {
		return false
	} else if list[i].Times < list[j].Times { //调用次数升序，调用次数多排后面
		return true
	} else if list[i].Times > list[j].Times {
		return false
	} else {
		return list[i].Timestamp < list[j].Timestamp
	}
}

func (list ServiceInfoList) Swap(i, j int) {
	temp := list[i]
	list[i] = list[j]
	list[j] = temp
}

func sortServices(slist []*ServiceInfo) []*ServiceInfo {
	length := len(slist)
	rlist := make(ServiceInfoList, length)
	if length >= 1 {
		for i := 0; i < length; i++ {
			rlist[i] = slist[i]
		}
		sort.Sort(rlist)
	}
	return rlist
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

func randShuffle(in []*ServiceInfo) []*ServiceInfo {
	length := len(in)
	if length <= 1 {
		return in
	}

	for i := 0; i < length-1; i++ {
		n := randRange(i, length)
		in[i], in[n] = in[n], in[i]
	}
	return in
}

func randRange(begin, end int) int {
	if begin >= end {
		return end
	}

	return rand.Intn(end-begin) + begin
}