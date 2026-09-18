package controller

import (
	"io"

	"github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

type modelOutputMappingResponseWriter struct {
	gin.ResponseWriter
	mapper *common.ModelOutputMapper
}

func newModelOutputMappingResponseWriter(writer gin.ResponseWriter, rawMapping string) (*modelOutputMappingResponseWriter, error) {
	mapper, err := common.NewModelOutputMapper(rawMapping)
	if err != nil {
		return nil, err
	}
	return &modelOutputMappingResponseWriter{ResponseWriter: writer, mapper: mapper}, nil
}

func (w *modelOutputMappingResponseWriter) Write(data []byte) (int, error) {
	rewritten := w.mapper.RewritePayload(data)
	written, err := w.ResponseWriter.Write(rewritten)
	if err != nil {
		return 0, err
	}
	if written != len(rewritten) {
		return 0, io.ErrShortWrite
	}
	return len(data), nil
}

func (w *modelOutputMappingResponseWriter) WriteHeader(statusCode int) {
	w.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *modelOutputMappingResponseWriter) WriteHeaderNow() {
	w.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeaderNow()
}

func (w *modelOutputMappingResponseWriter) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}
