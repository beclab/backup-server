package operator

import "bytetrade.io/web3os/backup-server/pkg/client"

type SnapshotOperator struct {
	factory client.Factory
}

func NewSnapshotOperator(f client.Factory) *SnapshotOperator {
	return &SnapshotOperator{
		factory: f,
	}
}
