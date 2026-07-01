# Changes

Code differences compared to source project.

## cmd/demo2kratos/main.go (+46 -7)

```diff
@@ -1,37 +1,44 @@
 package main
 
 import (
-	"flag"
+	"fmt"
 	"log/slog"
 	"os"
 
+	nacosconfig "github.com/go-kratos/kratos/contrib/config/nacos/v3"
 	"github.com/go-kratos/kratos/contrib/otel/v3/tracing"
 	"github.com/go-kratos/kratos/v3"
 	"github.com/go-kratos/kratos/v3/config"
-	"github.com/go-kratos/kratos/v3/config/file"
 	"github.com/go-kratos/kratos/v3/log"
 	"github.com/go-kratos/kratos/v3/transport/grpc"
 	"github.com/go-kratos/kratos/v3/transport/http"
+	"github.com/nacos-group/nacos-sdk-go/clients"
+	"github.com/nacos-group/nacos-sdk-go/common/constant"
+	"github.com/nacos-group/nacos-sdk-go/vo"
 	"github.com/yylego/done"
 	"github.com/yylego/kratos-examples/demo2kratos/internal/conf"
 	"github.com/yylego/must"
+	"github.com/yylego/neatjson/neatjsons"
 	"github.com/yylego/rese"
+	"github.com/yylego/tern/zerotern"
 
 	_ "go.uber.org/automaxprocs"
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
 
 func newApp(logger *slog.Logger, gs *grpc.Server, hs *http.Server) *kratos.App {
@@ -49,7 +56,9 @@
 }
 
 func main() {
-	flag.Parse()
+	// 有的时候会没有服务名称，需要默认值
+	Name = zerotern.VV(Name, defaultServiceName)
+
 	logger := log.NewLogger(
 		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
 			AddSource: true,
@@ -62,9 +71,33 @@
 		slog.String("service.version", Version),
 	)
 	log.SetDefault(logger)
+
+	// The config source uses the nacos-sdk-go v1 config client (required by contrib config/nacos/v3)
+	// 配置源用 nacos-sdk-go v1 的配置客户端（contrib config/nacos/v3 要求 v1）
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
+	source := nacosconfig.NewConfigSource(configClient, nacosconfig.WithGroup(nacosGroup), nacosconfig.WithDataID(dataID))
+
 	c := config.New(
 		config.WithSource(
-			file.NewSource(flagconf),
+			source,
 		),
 	)
 	defer rese.F0(c.Close)
@@ -73,6 +106,12 @@
 
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

## cmd/demo2kratos/wire_gen.go (+8 -1)

```diff
@@ -28,8 +28,13 @@
 	if err != nil {
 		return nil, nil, err
 	}
-	articleUsecase, err := biz.NewArticleUsecase(dataData, logger)
+	nacosNamingClient := data.NewNacosNamingClient()
+	demo1GrpcClient, cleanup2 := data.NewDemo1GrpcClient(nacosNamingClient, logger)
+	demo1HttpClient, cleanup3 := data.NewDemo1HttpClient(nacosNamingClient, logger)
+	articleUsecase, err := biz.NewArticleUsecase(dataData, demo1GrpcClient, demo1HttpClient, logger)
 	if err != nil {
+		cleanup3()
+		cleanup2()
 		cleanup()
 		return nil, nil, err
 	}
@@ -38,6 +43,8 @@
 	httpServer := server.NewHTTPServer(confServer, articleService, logger)
 	app := newApp(logger, grpcServer, httpServer)
 	return app, func() {
+		cleanup3()
+		cleanup2()
 		cleanup()
 	}, nil
 }
```

## internal/biz/article.go (+29 -4)

```diff
@@ -4,8 +4,10 @@
 	"context"
 	"errors"
 	"log/slog"
+	"math/rand/v2"
 
 	"github.com/yylego/kratos-ebz/ebzkratos"
+	demo1student "github.com/yylego/kratos-examples/demo1kratos/api/student"
 	pb "github.com/yylego/kratos-examples/demo2kratos/api/article"
 	"github.com/yylego/kratos-examples/demo2kratos/internal/data"
 	"github.com/yylego/must"
@@ -29,18 +31,20 @@
 func (Article) TableName() string { return "articles" }
 
 type ArticleUsecase struct {
-	data *data.Data
-	slog *slog.Logger
+	data            *data.Data
+	demo1GrpcClient *data.Demo1GrpcClient
+	demo1HttpClient *data.Demo1HttpClient
+	slog            *slog.Logger
 }
 
-func NewArticleUsecase(data *data.Data, logger *slog.Logger) (*ArticleUsecase, error) {
+func NewArticleUsecase(data *data.Data, demo1GrpcClient *data.Demo1GrpcClient, demo1HttpClient *data.Demo1HttpClient, logger *slog.Logger) (*ArticleUsecase, error) {
 	// Migrate the owned table plus the mirrored students table (needed in the
 	// existence check); both services share one database
 	// 建好本服务拥有的 articles 表，外加镜像的 students 表（供存在性校验用）
 	if err := data.DB().AutoMigrate(&Article{}, &Student{}); err != nil {
 		return nil, err
 	}
-	return &ArticleUsecase{data: data, slog: logger}, nil
+	return &ArticleUsecase{data: data, demo1GrpcClient: demo1GrpcClient, demo1HttpClient: demo1HttpClient, slog: logger}, nil
 }
 
 func (uc *ArticleUsecase) CreateArticle(ctx context.Context, a *Article) (*Article, *ebzkratos.Ebz) {
@@ -55,6 +59,27 @@
 	// （它持 FOR UPDATE）在本事务提交前删除该学生，从而绝不会创建出指向
 	// "正在被删除的学生"的文章
 	res := &Article{Title: a.Title, Content: a.Content, StudentID: a.StudentID}
+
+	// 跨服务调用 demo1kratos：经 nacos 服务发现拿到 demo1 的地址后调用，随机用 grpc 或 http 演示
+	// Cross-service call to demo1 discovered via nacos; randomly use grpc or http
+	if rand.IntN(2) == 0 {
+		resp, err := uc.demo1GrpcClient.GetStudentClient().CreateStudent(ctx, &demo1student.CreateStudentRequest{
+			Name: a.Title,
+		})
+		if err != nil {
+			return nil, ebzkratos.New(pb.ErrorServerError("call demo1 over grpc: %v", err))
+		}
+		res.Content = res.Content + " [grpc-resp:" + resp.GetStudent().GetName() + "]"
+	} else {
+		resp, err := uc.demo1HttpClient.GetStudentClient().CreateStudent(ctx, &demo1student.CreateStudentRequest{
+			Name: a.Title,
+		})
+		if err != nil {
+			return nil, ebzkratos.New(pb.ErrorServerError("call demo1 over http: %v", err))
+		}
+		res.Content = res.Content + " [http-resp:" + resp.GetStudent().GetName() + "]"
+	}
+
 	err := uc.data.DB().WithContext(ctx).Transaction(func(db *gorm.DB) error {
 		var student Student
 		if err := db.Clauses(clause.Locking{Strength: clause.LockingStrengthShare}).First(&student, a.StudentID).Error; err != nil {
```

## internal/data/data.go (+1 -1)

```diff
@@ -11,7 +11,7 @@
 	"gorm.io/gorm"
 )
 
-var ProviderSet = wire.NewSet(NewData)
+var ProviderSet = wire.NewSet(NewData, NewNacosNamingClient, NewDemo1GrpcClient, NewDemo1HttpClient)
 
 type Data struct {
 	db *gorm.DB
```

## internal/data/demo1_grpc_client.go (+49 -0)

```diff
@@ -0,0 +1,49 @@
+package data
+
+import (
+	"context"
+
+	"log/slog"
+
+	nacosregist "github.com/go-kratos/kratos/contrib/registry/nacos/v3"
+	"github.com/go-kratos/kratos/v3/middleware"
+	"github.com/go-kratos/kratos/v3/transport/grpc"
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
+func NewDemo1GrpcClient(nacosNamingClient *NacosNamingClient, logger *slog.Logger) (*Demo1GrpcClient, func()) {
+	// 这个写得非常好可以在更换时自动监听和更换IP地址，使用起来非常方便
+	conn := rese.P1(grpc.NewClient(
+		context.Background(),
+		grpc.WithEndpoint("discovery:///demo1kratos.grpc"),
+		grpc.WithDiscovery(nacosregist.New(nacosNamingClient.namingClient, nacosregist.WithGroup("demokratos"))),
+		grpc.WithMiddleware(func(handler middleware.Handler) middleware.Handler {
+			logger.Info("handle grpc request in middleware")
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

## internal/data/demo1_http_client.go (+48 -0)

```diff
@@ -0,0 +1,48 @@
+package data
+
+import (
+	"context"
+
+	"log/slog"
+
+	nacosregist "github.com/go-kratos/kratos/contrib/registry/nacos/v3"
+	"github.com/go-kratos/kratos/v3/middleware"
+	"github.com/go-kratos/kratos/v3/transport/http"
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
+func NewDemo1HttpClient(nacosNamingClient *NacosNamingClient, logger *slog.Logger) (*Demo1HttpClient, func()) {
+	// 这个写得非常好可以在更换时自动监听和更换IP地址，使用起来非常方便
+	client := rese.P1(http.NewClient(
+		context.Background(),
+		http.WithEndpoint("discovery:///demo1kratos.http"),
+		http.WithDiscovery(nacosregist.New(nacosNamingClient.namingClient, nacosregist.WithGroup("demokratos"))),
+		http.WithMiddleware(func(handler middleware.Handler) middleware.Handler {
+			logger.Info("handle http request in middleware")
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
+	"github.com/nacos-group/nacos-sdk-go/v2/clients"
+	"github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
+	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
+	"github.com/nacos-group/nacos-sdk-go/v2/vo"
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
@@ -3,6 +3,7 @@
 import (
 	"log/slog"
 
+	"github.com/go-kratos/kratos/v3/middleware/logging"
 	"github.com/go-kratos/kratos/v3/middleware/recovery"
 	"github.com/go-kratos/kratos/v3/transport/grpc"
 	pb "github.com/yylego/kratos-examples/demo2kratos/api/article"
@@ -14,6 +15,7 @@
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
@@ -3,6 +3,7 @@
 import (
 	"log/slog"
 
+	"github.com/go-kratos/kratos/v3/middleware/logging"
 	"github.com/go-kratos/kratos/v3/middleware/recovery"
 	"github.com/go-kratos/kratos/v3/transport/http"
 	pb "github.com/yylego/kratos-examples/demo2kratos/api/article"
@@ -14,6 +15,7 @@
 	var opts = []http.ServerOption{
 		http.Middleware(
 			recovery.Recovery(),
+			logging.Server(logger),
 		),
 	}
 	if c.Http.Network != "" {
```

