package main

import (
	"fmt"
	"log/slog"
	"os"

	nacosconfig "github.com/go-kratos/kratos/contrib/config/nacos/v3"
	"github.com/go-kratos/kratos/contrib/otel/v3/tracing"
	"github.com/go-kratos/kratos/v3"
	"github.com/go-kratos/kratos/v3/config"
	"github.com/go-kratos/kratos/v3/log"
	"github.com/go-kratos/kratos/v3/transport/grpc"
	"github.com/go-kratos/kratos/v3/transport/http"
	"github.com/nacos-group/nacos-sdk-go/clients"
	"github.com/nacos-group/nacos-sdk-go/common/constant"
	"github.com/nacos-group/nacos-sdk-go/vo"
	"github.com/yylego/done"
	"github.com/yylego/kratos-examples/demo2kratos/internal/conf"
	"github.com/yylego/must"
	"github.com/yylego/neatjson/neatjsons"
	"github.com/yylego/rese"
	"github.com/yylego/tern/zerotern"

	_ "go.uber.org/automaxprocs"
)

const nacosGroup = "demokratos"
const dataID = "demo2kratos.yaml" //需要带后缀，根据后缀选择解码器，假如不带后缀拿到的是Base64编码的数据
const defaultServiceName = "demo2kratos"

// go build -ldflags "-X main.Version=x.y.z"
var (
	// Name is the name of the compiled software.
	Name string
	// Version is the version of the compiled software.
	Version string
)

func init() {
	fmt.Println("service-name:", Name)
}

func newApp(logger *slog.Logger, gs *grpc.Server, hs *http.Server) *kratos.App {
	return kratos.New(
		kratos.ID(done.VCE(os.Hostname()).Omit()),
		kratos.Name(Name),
		kratos.Version(Version),
		kratos.Metadata(map[string]string{}),
		kratos.Logger(logger),
		kratos.Server(
			gs,
			hs,
		),
	)
}

func main() {
	// 有的时候会没有服务名称，需要默认值
	Name = zerotern.VV(Name, defaultServiceName)

	logger := log.NewLogger(
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelInfo,
		}),
		log.WithExtractor(tracing.TraceAttrs),
	).With(
		slog.String("service.id", done.VCE(os.Hostname()).Omit()),
		slog.String("service.name", Name),
		slog.String("service.version", Version),
	)
	log.SetDefault(logger)

	// The config source uses the nacos-sdk-go v1 config client (required by contrib config/nacos/v3)
	// 配置源用 nacos-sdk-go v1 的配置客户端（contrib config/nacos/v3 要求 v1）
	sc := []constant.ServerConfig{
		*constant.NewServerConfig("127.0.0.1", 8848),
	}

	cc := &constant.ClientConfig{
		TimeoutMs:           5000,
		NotLoadCacheAtStart: true,
		LogDir:              "/tmp/nacos/demo2kratos/log",
		CacheDir:            "/tmp/nacos/demo2kratos/cache",
		LogLevel:            "debug",
	}

	configClient := rese.V1(clients.NewConfigClient(
		vo.NacosClientParam{
			ClientConfig:  cc,
			ServerConfigs: sc,
		},
	))

	source := nacosconfig.NewConfigSource(configClient, nacosconfig.WithGroup(nacosGroup), nacosconfig.WithDataID(dataID))

	c := config.New(
		config.WithSource(
			source,
		),
	)
	defer rese.F0(c.Close)

	must.Done(c.Load())

	var cfg conf.Bootstrap
	must.Done(c.Scan(&cfg))

	// 假如需要随着配置更新而更新程序中的配置，就监听data字段的变动，因为data里基本是业务配置
	must.Done(c.Watch("data", func(s string, value config.Value) {
		must.Done(c.Scan(&cfg))
		fmt.Println("config-data-update:", neatjsons.S(&cfg))
	}))

	app, cleanup := rese.V2(wireApp(cfg.Server, cfg.Data, logger))
	defer cleanup()

	// start and wait for stop signal
	must.Done(app.Run())
}
