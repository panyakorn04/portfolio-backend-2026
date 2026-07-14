package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"portfolio-backend/internal/config"
	"portfolio-backend/internal/handler"
	"portfolio-backend/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/rest"
)

var configFile = flag.String("f", "etc/portfolio-api.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	server := rest.MustNewServer(c.RestConf, rest.WithCors(c.CorsOrigins...))
	defer server.Stop()

	ctx, err := svc.NewServiceContext(c)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init service context: %v\n", err)
		os.Exit(1)
	}
	defer ctx.Close()

	handler.RegisterHandlers(server, ctx)
	runner := handler.NewStudioExecutionRunner(ctx)
	runner.Start(context.Background())
	defer runner.Close()

	fmt.Printf("Starting portfolio-api at %s:%d...\n", c.Host, c.Port)
	server.Start()
}
