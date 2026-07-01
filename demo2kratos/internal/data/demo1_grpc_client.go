package data

import (
	"context"

	"log/slog"

	nacosregist "github.com/go-kratos/kratos/contrib/registry/nacos/v3"
	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport/grpc"
	demo1student "github.com/yylego/kratos-examples/demo1kratos/api/student"
	"github.com/yylego/must"
	"github.com/yylego/rese"
	grpcconn "google.golang.org/grpc"
)

type Demo1GrpcClient struct {
	conn          *grpcconn.ClientConn
	studentClient demo1student.StudentServiceClient
}

func NewDemo1GrpcClient(nacosNamingClient *NacosNamingClient, logger *slog.Logger) (*Demo1GrpcClient, func()) {
	// 这个写得非常好可以在更换时自动监听和更换IP地址，使用起来非常方便
	conn := rese.P1(grpc.NewClient(
		context.Background(),
		grpc.WithEndpoint("discovery:///demo1kratos.grpc"),
		grpc.WithDiscovery(nacosregist.New(nacosNamingClient.namingClient, nacosregist.WithGroup("demokratos"))),
		grpc.WithMiddleware(func(handler middleware.Handler) middleware.Handler {
			logger.Info("handle grpc request in middleware")
			return func(ctx context.Context, req any) (any, error) {
				// set auth info into context then request remote
				return handler(ctx, req)
			}
		}),
	))
	// cp from https://github.com/go-kratos/examples/blob/61daed1ec4d5a94d689bc8fab9bc960c6af73ead/registry/nacos/client/main.go#L51
	studentClient := demo1student.NewStudentServiceClient(conn)
	cleanup := func() {
		must.Done(conn.Close())
	}
	return &Demo1GrpcClient{
		conn:          conn,
		studentClient: studentClient,
	}, cleanup
}

func (c *Demo1GrpcClient) GetStudentClient() demo1student.StudentServiceClient {
	return c.studentClient
}
