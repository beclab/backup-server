package main

import (
	"fmt"
	"os"

	"bytetrade.io/web3os/backup-server/cmd/backup-server/apiserver"
	"bytetrade.io/web3os/backup-server/cmd/backup-server/controller"
	vcontroller "bytetrade.io/web3os/backup-server/cmd/backup-server/velero_backup"
	"github.com/spf13/cobra"
	// "bytetrade.io/web3os/backup-server/cmd/backup-server/controller"
	// vcontroller "bytetrade.io/web3os/backup-server/cmd/backup-server/velero_backup"
)

var rootCommand = cobra.Command{
	Use: "backup-server",
}

func completionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "completion",
		Short: "Generate the autocompletion script for the specified shell",
	}
}

func init() {
	completion := completionCommand()
	completion.Hidden = true
	rootCommand.AddCommand(completion)

	rootCommand.AddCommand(apiserver.NewAPIServerCommand())
	rootCommand.AddCommand(controller.NewControllerCommand()) // todo replaced?
	rootCommand.AddCommand(vcontroller.NewVeleroBackupControllerCommand())

}

func main() {
	if err := rootCommand.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%+v\n", err)
		os.Exit(1)
	}
}
