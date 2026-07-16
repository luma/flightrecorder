package version

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/luma/flightrecorder/env"
)

// Command returns the version display command
func Command() *cli.Command {
	return &cli.Command{
		Name:    "version",
		Aliases: []string{"v"},
		Usage:   "Display version information",
		Description: `
The version command displays build and version information about the Flightrecorder API.
This includes version number, build ID, branch, build time, and platform details.
`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Output version info as JSON",
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			info := env.GetInfo()

			if c.Bool("json") {
				jsonBytes, err := json.MarshalIndent(info, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(jsonBytes))
				return nil
			}

			// Pretty-print version info
			fmt.Printf("Flightrecorder %s\n", info.Version)
			fmt.Printf("Build:     %s\n", info.Build)
			fmt.Printf("Branch:    %s\n", info.Branch)
			fmt.Printf("Built:     %s\n", info.BuildTime)
			fmt.Printf("Go:        %s\n", info.GoVersion)
			fmt.Printf("Platform:  %s\n", info.Platform)

			return nil
		},
	}
}
