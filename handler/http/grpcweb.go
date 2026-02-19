package httpserver

import (
	"net/http"

	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"google.golang.org/grpc"
)

func NewHandler(s *grpc.Server) http.Handler {
    wrapped := grpcweb.WrapServer(s, grpcweb.WithOriginFunc(func(origin string) bool { return true }))
    return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
        if wrapped.IsGrpcWebRequest(req) || wrapped.IsAcceptableGrpcCorsRequest(req) || wrapped.IsGrpcWebSocketRequest(req) {
            wrapped.ServeHTTP(resp, req)
            return
        }
        s.ServeHTTP(resp, req)
    })
}
