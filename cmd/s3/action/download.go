package action

import (
	"context"
	"os"
	"path/filepath"

	"bytetrade.io/web3os/backup-server/cmd/s3/options"
	"bytetrade.io/web3os/backup-server/pkg/signals"
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
)

func defaultStorePath() string {
	pwd, err := os.Getwd()
	if err == nil && pwd != "" {
		return pwd
	}

	var home string
	if pwd == "" {
		home, err = homedir.Dir()
		if err == nil {
			return home
		}
	}
	return ""
}

func NewS3DownloadCommand(o *options.Option) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "download",
		Short: "s3 object download",
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error

			if err = o.Complete(); err != nil {
				return err
			}

			err = downloadS3Object(o, signals.SetupSignalContext())
			if err != nil {
				return err
			}
			return nil
		},
	}

	flag := cmd.PersistentFlags()
	flag.StringVarP(&o.DownloadKeyName, "key", "", o.DownloadKeyName, "download s3 object key name")
	flag.StringVarP(&o.DownloadStorePath, "store-path", "", defaultStorePath(), "download object and save to path")

	return cmd
}

func downloadS3Object(o *options.Option, ctx context.Context) error {
	bc := o.BackupConfigSpec()

	client, err := util.NewS3Client(bc)
	if err != nil {
		return err
	}
	storePath := filepath.Join(o.DownloadStorePath, filepath.Base(o.DownloadKeyName))
	_, err = client.DownloadFile(ctx, o.DownloadKeyName, storePath)
	if err != nil {
		return err
	}
	log.Debugf("successfully download %q to %q", o.DownloadKeyName, storePath)
	return nil
}
