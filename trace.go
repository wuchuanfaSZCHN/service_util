package service_util

import (
	"strconv"
	"github.com/opentracing/opentracing-go"
	zipkin "github.com/openzipkin/zipkin-go-opentracing"
)

func NewTracer(serviceName string) opentracing.Tracer {
	//zipkin init
	source, err := GetConfig("zipkin_info")
	if err != nil {
		checkErr(err, "NewTracer", false)
	}

	url := "http://" + source["ip"].(string) + ":" + strconv.Itoa(source["port"].(int)) + "/api/v1/spans"
	collector, err := zipkin.NewHTTPCollector(url)
	if err != nil {
		checkErr(err, "NewTracer", false)
	}

	tracer, err := zipkin.NewTracer(
		zipkin.NewRecorder(collector, false, "localhost:0", serviceName),
		zipkin.ClientServerSameSpan(true),
		zipkin.TraceID128Bit(true),
	)
	if err != nil {
		checkErr(err, "NewTracer", false)
	}
	opentracing.InitGlobalTracer(tracer)
	return tracer
}
