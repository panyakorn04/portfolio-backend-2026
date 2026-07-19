package main

import (
	"context"
	"flag"
	"os"

	"portfolio-backend/internal/config"
	"portfolio-backend/internal/handler"
	"portfolio-backend/internal/observability"
	"portfolio-backend/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest"
)

var configFile = flag.String("f", "etc/portfolio-api.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	if err := config.ApplyEnvironmentOverrides(&c, os.LookupEnv); err != nil {
		observability.Error(context.Background(), "config.environment.invalid", "Invalid runtime environment configuration", err)
		os.Exit(1)
	}

	requestRouter := observability.NewHTTPRouter(nil, c.CorsOrigins...)
	server := rest.MustNewServer(c.RestConf, rest.WithRouter(requestRouter))
	defer server.Stop()

	ctx, err := svc.NewServiceContext(c)
	if err != nil {
		observability.Error(context.Background(), "service.initialization.failed", "Failed to initialize service context", err)
		os.Exit(1)
	}
	defer ctx.Close()

	routePatterns := handler.RegisterHandlers(server, ctx)
	requestRouter.SetRoutePatterns(routePatterns)
	runner := handler.NewStudioExecutionRunner(ctx)
	runner.Start(context.Background())
	defer runner.Close()

	logx.Infow("portfolio API starting",
		logx.Field("event", "service.starting"),
		logx.Field("host", c.Host),
		logx.Field("port", c.Port),
	)
	server.Start()
}
