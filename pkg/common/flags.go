package common

import (
	"bytetrade.io/web3os/backup-server/pkg/util"
	"bytetrade.io/web3os/backup-server/pkg/util/log"
	"bytetrade.io/web3os/backup-server/pkg/velero"
	"github.com/spf13/pflag"
)

func AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&velero.DefaultVeleroNamespace, "velero-namespace",
		util.EnvOrDefault("VELERO_NAMESPACE", velero.DefaultVeleroNamespace), "backup in namespace")
	fs.StringVar(&velero.DefaultVeleroServiceAccountName, "velero-service-account",
		util.EnvOrDefault("VELERO_SERVICE_ACCOUNT", velero.DefaultVeleroServiceAccountName), "service account name")
	fs.StringVar(&velero.DefaultVeleroImage, "velero-image",
		util.EnvOrDefault("VELERO_IMAGE", velero.DefaultVeleroImage), "velero image")
	fs.StringVar(&velero.DefaultVeleroSecretName, "velero-secret",
		util.EnvOrDefault("VELERO_SECRET", velero.DefaultVeleroSecretName), "secret name")
}

func PrintFlagAndValues(args ...any) {
	all := []any{
		"velero-namespace", velero.DefaultVeleroNamespace,
		"velero-service-account", velero.DefaultVeleroServiceAccountName,
		"velero-image", velero.DefaultVeleroImage,
		"velero-secret", velero.DefaultVeleroSecretName,
	}
	if len(args) > 0 && len(args)%2 == 0 {
		all = append(all, args...)
	}
	log.Debugw("server flags", all...)
}
