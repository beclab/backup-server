package apiserver

import (
	"context"
	"net/http"

	"bytetrade.io/web3os/backup-server/cmd/backup-server/apiserver/options"
	"bytetrade.io/web3os/backup-server/pkg/client"
	"bytetrade.io/web3os/backup-server/pkg/integration"
	"bytetrade.io/web3os/backup-server/pkg/postgres"
	"bytetrade.io/web3os/backup-server/pkg/signals"
	"github.com/lithammer/dedent"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func NewAPIServerCommand() *cobra.Command {
	o := options.NewServerRunOptions()

	cmd := &cobra.Command{
		Use:   "apiserver",
		Short: "start apiserver",
		Long:  dedent.Dedent(`The apiserver provides restful api service.`),
		RunE: func(cmd *cobra.Command, args []string) error {
			err := o.Validate()
			if err != nil {
				return err
			}

			if err = o.Complete(); err != nil {
				return err
			}

			if err = Run(o, signals.SetupSignalContext()); err != nil {
				return err
			}
			return nil
		},
		Args: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	cmd.PersistentFlags().StringVarP(&o.LogLevel, "log-level", "l", "debug", "logging level")
	cmd.PersistentFlags().BoolVarP(&o.SkipKubeClient, "skip-kubeclient", "", false, "skip kubernetes client")

	fs := cmd.Flags()
	o.AddFlags(fs)

	return cmd
}

func Run(o *options.ServerRunOptions, ctx context.Context) error {
	ictx, cancel := context.WithCancel(context.TODO())
	errCh := make(chan error)
	defer close(errCh)

	go func() {
		if err := run(o, ictx); err != nil {
			errCh <- err
		}
	}()

	for {
		select {
		case <-ctx.Done():
			cancel()
			return nil
		case err := <-errCh:
			if err != nil {
				cancel()
				return err
			}
		}
	}
}

func run(o *options.ServerRunOptions, ctx context.Context) error {
	apiserver, err := o.NewAPIServer()
	if err != nil {
		return err
	}

	if !o.SkipKubeClient {
		err = client.Init(o.LogLevel)
		if err != nil {
			return err
		}
	}

	var factory = client.ClientFactory()
	integration.NewIntegrationManager(factory)

	postgres.InitPostgres()

	if err := apiserver.PrepareRun(); err != nil {
		return errors.Errorf("apiserver prepare run: %v", err)
	}

	err = apiserver.Run(ctx)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
