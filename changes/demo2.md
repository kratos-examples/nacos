# Changes

Code differences compared to source project.

## cmd/demo2kratos/main.go (+46 -7)

```diff
@@ -1,34 +1,41 @@
 package main
 
 import (
-	"flag"
+	"fmt"
 	"os"
 
+	nacosconfig "github.com/go-kratos/kratos/contrib/config/nacos/v2"
 	"github.com/go-kratos/kratos/v2"
 	"github.com/go-kratos/kratos/v2/config"
-	"github.com/go-kratos/kratos/v2/config/file"
 	"github.com/go-kratos/kratos/v2/log"
 	"github.com/go-kratos/kratos/v2/middleware/tracing"
 	"github.com/go-kratos/kratos/v2/transport/grpc"
 	"github.com/go-kratos/kratos/v2/transport/http"
+	"github.com/nacos-group/nacos-sdk-go/clients"
+	"github.com/nacos-group/nacos-sdk-go/common/constant"
+	"github.com/nacos-group/nacos-sdk-go/vo"
 	"github.com/yylego/done"
 	"github.com/yylego/kratos-examples/demo2kratos/internal/conf"
 	"github.com/yylego/must"
+	"github.com/yylego/neatjson/neatjsons"
 	"github.com/yylego/rese"
+	"github.com/yylego/tern/zerotern"
 )
 
+const nacosGroup = "demokratos"
+const dataID = "demo2kratos.yaml" //需要带后缀，根据后缀选择解码器，假如不带后缀拿到的是Base64编码的数据
+const defaultServiceName = "demo2kratos"
+
 // go build -ldflags "-X main.Version=x.y.z"
 var (
 	// Name is the name of the compiled software.
 	Name string
 	// Version is the version of the compiled software.
 	Version string
-	// flagconf is the config flag.
-	flagconf string
 )
 
 func init() {
-	flag.StringVar(&flagconf, "conf", "./configs", "config path, eg: -conf config.yaml")
+	fmt.Println("service-name:", Name)
 }
 
 func newApp(logger log.Logger, gs *grpc.Server, hs *http.Server) *kratos.App {
@@ -46,7 +53,9 @@
 }
 
 func main() {
-	flag.Parse()
+	// 有的时候会没有服务名称，需要默认值
+	Name = zerotern.VV(Name, defaultServiceName)
+
 	logger := log.With(log.NewStdLogger(os.Stdout),
 		"ts", log.DefaultTimestamp,
 		"caller", log.DefaultCaller,
@@ -56,9 +65,33 @@
 		"trace.id", tracing.TraceID(),
 		"span.id", tracing.SpanID(),
 	)
+
+	// cp from https://github.com/go-kratos/kratos/blob/d6f5f00cf562b46322b0ed42d183b1b873c0a68f/contrib/config/nacos/config_test.go#L16
+	sc := []constant.ServerConfig{
+		*constant.NewServerConfig("127.0.0.1", 8848),
+	}
+
+	cc := &constant.ClientConfig{
+		TimeoutMs:           5000,
+		NotLoadCacheAtStart: true,
+		LogDir:              "/tmp/nacos/demo2kratos/log",
+		CacheDir:            "/tmp/nacos/demo2kratos/cache",
+		LogLevel:            "debug",
+	}
+
+	configClient := rese.V1(clients.NewConfigClient(
+		vo.NacosClientParam{
+			ClientConfig:  cc,
+			ServerConfigs: sc,
+		},
+	))
+
+	// cp from https://github.com/go-kratos/kratos/blob/d6f5f00cf562b46322b0ed42d183b1b873c0a68f/contrib/config/nacos/config_test.go#L39
+	source := nacosconfig.NewConfigSource(configClient, nacosconfig.WithGroup(nacosGroup), nacosconfig.WithDataID(dataID))
+
 	c := config.New(
 		config.WithSource(
-			file.NewSource(flagconf),
+			source,
 		),
 	)
 	defer rese.F0(c.Close)
@@ -67,6 +100,12 @@
 
 	var cfg conf.Bootstrap
 	must.Done(c.Scan(&cfg))
+
+	// 假如需要随着配置更新而更新程序中的配置，就监听data字段的变动，因为data里基本是业务配置
+	must.Done(c.Watch("data", func(s string, value config.Value) {
+		must.Done(c.Scan(&cfg))
+		fmt.Println("config-data-update:", neatjsons.S(&cfg))
+	}))
 
 	app, cleanup := rese.V2(wireApp(cfg.Server, cfg.Data, logger))
 	defer cleanup()
```

## cmd/demo2kratos/wire_gen.go (+6 -1)

```diff
@@ -23,12 +23,17 @@
 	if err != nil {
 		return nil, nil, err
 	}
-	articleUsecase := biz.NewArticleUsecase(dataData, logger)
+	nacosNamingClient := data.NewNacosNamingClient()
+	demo1GrpcClient, cleanup2 := data.NewDemo1GrpcClient(nacosNamingClient, logger)
+	demo1HttpClient, cleanup3 := data.NewDemo1HttpClient(nacosNamingClient, logger)
+	articleUsecase := biz.NewArticleUsecase(dataData, demo1GrpcClient, demo1HttpClient, logger)
 	articleService := service.NewArticleService(articleUsecase)
 	grpcServer := server.NewGRPCServer(confServer, articleService, logger)
 	httpServer := server.NewHTTPServer(confServer, articleService, logger)
 	app := newApp(logger, grpcServer, httpServer)
 	return app, func() {
+		cleanup3()
+		cleanup2()
 		cleanup()
 	}, nil
 }
```

## internal/biz/article.go (+28 -4)

```diff
@@ -2,10 +2,12 @@
 
 import (
 	"context"
+	"math/rand/v2"
 
 	"github.com/brianvoe/gofakeit/v7"
 	"github.com/go-kratos/kratos/v2/log"
 	"github.com/yylego/kratos-ebz/ebzkratos"
+	demo1student "github.com/yylego/kratos-examples/demo1kratos/api/student"
 	pb "github.com/yylego/kratos-examples/demo2kratos/api/article"
 	"github.com/yylego/kratos-examples/demo2kratos/internal/data"
 )
@@ -18,18 +20,40 @@
 }
 
 type ArticleUsecase struct {
-	data *data.Data
-	log  *log.Helper
+	data            *data.Data
+	demo1GrpcClient *data.Demo1GrpcClient
+	demo1HttpClient *data.Demo1HttpClient
+	log             *log.Helper
 }
 
-func NewArticleUsecase(data *data.Data, logger log.Logger) *ArticleUsecase {
-	return &ArticleUsecase{data: data, log: log.NewHelper(logger)}
+func NewArticleUsecase(data *data.Data, demo1GrpcClient *data.Demo1GrpcClient, demo1HttpClient *data.Demo1HttpClient, logger log.Logger) *ArticleUsecase {
+	return &ArticleUsecase{data: data, demo1GrpcClient: demo1GrpcClient, demo1HttpClient: demo1HttpClient, log: log.NewHelper(logger)}
 }
 
 func (uc *ArticleUsecase) CreateArticle(ctx context.Context, a *Article) (*Article, *ebzkratos.Ebz) {
 	var res Article
 	if err := gofakeit.Struct(&res); err != nil {
 		return nil, ebzkratos.New(pb.ErrorArticleCreateFailure("fake: %v", err))
+	}
+	// 这里是两个demo，grpc和http，随便用哪个都行，使用随机演示
+	if rand.IntN(2) == 0 {
+		// cp from https://github.com/go-kratos/examples/blob/61daed1ec4d5a94d689bc8fab9bc960c6af73ead/registry/nacos/client/main.go#L52
+		resp, err := uc.demo1GrpcClient.GetStudentClient().CreateStudent(ctx, &demo1student.CreateStudentRequest{
+			Name: res.Title,
+		})
+		if err != nil {
+			return nil, ebzkratos.New(pb.ErrorServerError("grpc: %v", err))
+		}
+		res.Title = "message:[grpc-resp:" + resp.GetStudent().GetName() + "]"
+	} else {
+		// cp from https://github.com/go-kratos/kratos/blob/d6f5f00cf562b46322b0ed42d183b1b873c0a68f/transport/http/client_test.go#L354
+		resp, err := uc.demo1HttpClient.GetStudentClient().CreateStudent(ctx, &demo1student.CreateStudentRequest{
+			Name: res.Title,
+		})
+		if err != nil {
+			return nil, ebzkratos.New(pb.ErrorServerError("http: %v", err))
+		}
+		res.Title = "message:[http-resp:" + resp.GetStudent().GetName() + "]"
 	}
 	return &res, nil
 }
```

## internal/data/data.go (+1 -1)

```diff
@@ -10,7 +10,7 @@
 	"gorm.io/gorm"
 )
 
-var ProviderSet = wire.NewSet(NewData)
+var ProviderSet = wire.NewSet(NewData, NewNacosNamingClient, NewDemo1GrpcClient, NewDemo1HttpClient)
 
 type Data struct {
 	db *gorm.DB
```

## internal/data/demo1_grpc_client.go (+50 -0)

```diff
@@ -0,0 +1,50 @@
+package data
+
+import (
+	"context"
+
+	nacosregist "github.com/go-kratos/kratos/contrib/registry/nacos/v2"
+	"github.com/go-kratos/kratos/v2/log"
+	"github.com/go-kratos/kratos/v2/middleware"
+	"github.com/go-kratos/kratos/v2/transport/grpc"
+	demo1student "github.com/yylego/kratos-examples/demo1kratos/api/student"
+	"github.com/yylego/must"
+	"github.com/yylego/rese"
+	grpcconn "google.golang.org/grpc"
+)
+
+type Demo1GrpcClient struct {
+	conn          *grpcconn.ClientConn
+	studentClient demo1student.StudentServiceClient
+}
+
+func NewDemo1GrpcClient(nacosNamingClient *NacosNamingClient, logger log.Logger) (*Demo1GrpcClient, func()) {
+	LOG := log.NewHelper(logger)
+
+	// 这个写得非常好可以在更换时自动监听和更换IP地址，使用起来非常方便
+	conn := rese.P1(grpc.DialInsecure(
+		context.Background(),
+		grpc.WithEndpoint("discovery:///demo1kratos.grpc"),
+		grpc.WithDiscovery(nacosregist.New(nacosNamingClient.namingClient, nacosregist.WithGroup("demokratos"))),
+		grpc.WithMiddleware(func(handler middleware.Handler) middleware.Handler {
+			LOG.Infof("handle grpc request in middleware")
+			return func(ctx context.Context, req any) (any, error) {
+				// set auth info into context then request remote
+				return handler(ctx, req)
+			}
+		}),
+	))
+	// cp from https://github.com/go-kratos/examples/blob/61daed1ec4d5a94d689bc8fab9bc960c6af73ead/registry/nacos/client/main.go#L51
+	studentClient := demo1student.NewStudentServiceClient(conn)
+	cleanup := func() {
+		must.Done(conn.Close())
+	}
+	return &Demo1GrpcClient{
+		conn:          conn,
+		studentClient: studentClient,
+	}, cleanup
+}
+
+func (c *Demo1GrpcClient) GetStudentClient() demo1student.StudentServiceClient {
+	return c.studentClient
+}
```

## internal/data/demo1_http_client.go (+49 -0)

```diff
@@ -0,0 +1,49 @@
+package data
+
+import (
+	"context"
+
+	nacosregist "github.com/go-kratos/kratos/contrib/registry/nacos/v2"
+	"github.com/go-kratos/kratos/v2/log"
+	"github.com/go-kratos/kratos/v2/middleware"
+	"github.com/go-kratos/kratos/v2/transport/http"
+	demo1student "github.com/yylego/kratos-examples/demo1kratos/api/student"
+	"github.com/yylego/must"
+	"github.com/yylego/rese"
+)
+
+type Demo1HttpClient struct {
+	client        *http.Client
+	studentClient demo1student.StudentServiceHTTPClient
+}
+
+func NewDemo1HttpClient(nacosNamingClient *NacosNamingClient, logger log.Logger) (*Demo1HttpClient, func()) {
+	LOG := log.NewHelper(logger)
+
+	// 这个写得非常好可以在更换时自动监听和更换IP地址，使用起来非常方便
+	client := rese.P1(http.NewClient(
+		context.Background(),
+		http.WithEndpoint("discovery:///demo1kratos.http"),
+		http.WithDiscovery(nacosregist.New(nacosNamingClient.namingClient, nacosregist.WithGroup("demokratos"))),
+		http.WithMiddleware(func(handler middleware.Handler) middleware.Handler {
+			LOG.Infof("handle http request in middleware")
+			return func(ctx context.Context, req any) (any, error) {
+				// set auth info into context then request remote
+				return handler(ctx, req)
+			}
+		}),
+	))
+	// cp from https://github.com/go-kratos/kratos/blob/d6f5f00cf562b46322b0ed42d183b1b873c0a68f/transport/http/client_test.go#L339
+	studentClient := demo1student.NewStudentServiceHTTPClient(client)
+	cleanup := func() {
+		must.Done(client.Close())
+	}
+	return &Demo1HttpClient{
+		client:        client,
+		studentClient: studentClient,
+	}, cleanup
+}
+
+func (c *Demo1HttpClient) GetStudentClient() demo1student.StudentServiceHTTPClient {
+	return c.studentClient
+}
```

## internal/data/nacos_naming_client.go (+40 -0)

```diff
@@ -0,0 +1,40 @@
+package data
+
+import (
+	"github.com/nacos-group/nacos-sdk-go/clients"
+	"github.com/nacos-group/nacos-sdk-go/clients/naming_client"
+	"github.com/nacos-group/nacos-sdk-go/common/constant"
+	"github.com/nacos-group/nacos-sdk-go/vo"
+	"github.com/yylego/rese"
+)
+
+type NacosNamingClient struct {
+	namingClient naming_client.INamingClient
+}
+
+func NewNacosNamingClient() *NacosNamingClient {
+	// cp from https://github.com/go-kratos/examples/blob/61daed1ec4d5a94d689bc8fab9bc960c6af73ead/registry/nacos/client/main.go#L16
+	sc := []constant.ServerConfig{
+		*constant.NewServerConfig("127.0.0.1", 8848),
+	}
+
+	cc := &constant.ClientConfig{
+		NamespaceId:         "public",
+		TimeoutMs:           5000,
+		NotLoadCacheAtStart: true,
+		LogDir:              "/tmp/nacos/demo2kratos/log",
+		CacheDir:            "/tmp/nacos/demo2kratos/cache",
+		LogLevel:            "debug",
+	}
+
+	// cp from https://github.com/go-kratos/examples/blob/61daed1ec4d5a94d689bc8fab9bc960c6af73ead/registry/nacos/client/main.go#L31
+	namingClient := rese.V1(clients.NewNamingClient(
+		vo.NacosClientParam{
+			ClientConfig:  cc,
+			ServerConfigs: sc,
+		},
+	))
+	return &NacosNamingClient{
+		namingClient: namingClient,
+	}
+}
```

## internal/server/grpc.go (+2 -0)

```diff
@@ -2,6 +2,7 @@
 
 import (
 	"github.com/go-kratos/kratos/v2/log"
+	"github.com/go-kratos/kratos/v2/middleware/logging"
 	"github.com/go-kratos/kratos/v2/middleware/recovery"
 	"github.com/go-kratos/kratos/v2/transport/grpc"
 	pb "github.com/yylego/kratos-examples/demo2kratos/api/article"
@@ -13,6 +14,7 @@
 	var opts = []grpc.ServerOption{
 		grpc.Middleware(
 			recovery.Recovery(),
+			logging.Server(logger),
 		),
 	}
 	if c.Grpc.Network != "" {
```

## internal/server/http.go (+2 -0)

```diff
@@ -2,6 +2,7 @@
 
 import (
 	"github.com/go-kratos/kratos/v2/log"
+	"github.com/go-kratos/kratos/v2/middleware/logging"
 	"github.com/go-kratos/kratos/v2/middleware/recovery"
 	"github.com/go-kratos/kratos/v2/transport/http"
 	pb "github.com/yylego/kratos-examples/demo2kratos/api/article"
@@ -13,6 +14,7 @@
 	var opts = []http.ServerOption{
 		http.Middleware(
 			recovery.Recovery(),
+			logging.Server(logger),
 		),
 	}
 	if c.Http.Network != "" {
```

