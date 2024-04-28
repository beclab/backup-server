package main

import (
	"fmt"
	"os"

	"bytetrade.io/web3os/backup-server/cmd/s3/action"
	"bytetrade.io/web3os/backup-server/cmd/s3/options"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var rootCommand = cobra.Command{
	Use: "s3",
}

func completionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "completion",
		Short: "Generate the autocompletion script for the specified shell",
	}
}

func init() {
	log.InitLog("debug")

	completion := completionCommand()
	completion.Hidden = true
	rootCommand.AddCommand(completion)
}

func addFlags(o *options.Option, cmd *pflag.FlagSet) {
	cmd.StringVarP(&o.Provider, "provider", "", o.Provider, "s3 provider, aws or minio")
	cmd.StringVarP(&o.Region, "region", "", o.Region, "s3 region")
	cmd.StringVarP(&o.Endpoint, "endpoint", "", o.Endpoint, "s3 endpoint, for minio")
	cmd.StringVarP(&o.Bucket, "bucket", "", o.Bucket, "s3 bucket name")
	cmd.StringVarP(&o.AccessKey, "ak", "", o.AccessKey, "s3 bucket access key")
	cmd.StringVarP(&o.SecretKey, "sk", "", o.SecretKey, "s3 bucket secret key")
}

func main() {
	o := options.New()
	flags := rootCommand.PersistentFlags()
	addFlags(o, flags)

	rootCommand.AddCommand(action.NewS3DownloadCommand(o))

	if err := rootCommand.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%+v\n", err)
		os.Exit(1)
	}
}
