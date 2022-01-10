package maincommand

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/stackrox/rox/pkg/buildinfo"
	"github.com/stackrox/rox/pkg/version"
	"github.com/stackrox/rox/roxctl/central"
	"github.com/stackrox/rox/roxctl/cluster"
	"github.com/stackrox/rox/roxctl/collector"
	"github.com/stackrox/rox/roxctl/common/environment"
	"github.com/stackrox/rox/roxctl/common/flags"
	"github.com/stackrox/rox/roxctl/common/printer"
	"github.com/stackrox/rox/roxctl/common/util"
	"github.com/stackrox/rox/roxctl/completion"
	"github.com/stackrox/rox/roxctl/deployment"
	"github.com/stackrox/rox/roxctl/helm"
	"github.com/stackrox/rox/roxctl/image"
	"github.com/stackrox/rox/roxctl/logconvert"
	"github.com/stackrox/rox/roxctl/scanner"
	"github.com/stackrox/rox/roxctl/sensor"
)

func versionCommand() *cobra.Command {
	c := &cobra.Command{
		Use: "version",
		RunE: util.RunENoArgs(func(c *cobra.Command) error {
			if useJSON, _ := c.Flags().GetBool("json"); useJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if buildinfo.ReleaseBuild {
					return enc.Encode(version.GetAllVersionsUnified())
				}
				return enc.Encode(version.GetAllVersionsDevelopment())
			}
			fmt.Println(version.GetMainVersion())
			return nil
		}),
	}
	c.PersistentFlags().Bool("json", false, "display extended version information as JSON")
	return c
}

// Command constructs and returns the roxctl command tree
func Command() *cobra.Command {
	c := &cobra.Command{
		SilenceUsage: true,
		Use:          os.Args[0],
	}

	flags.AddNoColor(c)
	flags.AddPassword(c)
	flags.AddConnectionFlags(c)
	flags.AddAPITokenFile(c)

	// We have chicken and egg problem here. We need to parse flags to know if --no-color was set
	// but at the same time we need to set printer to handle possible flags parsing errors.
	// Instead of using native cobra flags mechanism we can just check if os.Args contains --no-color.
	var colorPrinter printer.ColorfulPrinter
	if flags.HasNoColor(os.Args) {
		colorPrinter = printer.NoColorPrinter()
	} else {
		colorPrinter = printer.DefaultColorPrinter()
	}
	cliEnvironment := environment.NewCLIEnvironment(environment.DefaultIO(), colorPrinter)
	c.SetErr(errorWriter{
		logger: cliEnvironment.Logger(),
	})

	c.AddCommand(
		central.Command(),
		cluster.Command(),
		collector.Command(),
		deployment.Command(cliEnvironment),
		logconvert.Command(),
		image.Command(cliEnvironment),
		scanner.Command(),
		sensor.Command(),
		helm.Command(),
		versionCommand(),
		completion.Command(),
	)

	return c
}
