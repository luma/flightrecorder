package main

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/luma/flightrecorder/cmd/migrate"
	"github.com/luma/flightrecorder/cmd/service"
	"github.com/luma/flightrecorder/cmd/version"
)

func main() {
	app := &cli.Command{
		Name:  "pithy",
		Usage: "Your second brain",
		Commands: []*cli.Command{
			service.Command(),
			migrate.Command(),
			version.Command(),
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
