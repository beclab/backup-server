package storage

import (
	"context"
	"fmt"

	"bytetrade.io/web3os/backup-server/pkg/constant"
	"bytetrade.io/web3os/backup-server/pkg/handlers"
	integration "bytetrade.io/web3os/backup-server/pkg/integration"
	"bytetrade.io/web3os/backup-server/pkg/util/log"

	backupssdk "bytetrade.io/web3os/backups-sdk"
	backupssdkoptions "bytetrade.io/web3os/backups-sdk/pkg/options"
	backupssdkstorage "bytetrade.io/web3os/backups-sdk/pkg/storage"
)

type StorageRegion struct {
	Handlers handlers.Interface
}

func (s *StorageRegion) GetRegions(ctx context.Context, owner, olaresId string) ([]map[string]string, error) {
	var spaceToken, err = integration.IntegrationManager().GetIntegrationSpaceToken(ctx, owner, olaresId) // only for Space
	if err != nil {
		err = fmt.Errorf("get space token error: %v", err)
		return nil, err
	}

	var spaceRegionOption = &backupssdkoptions.SpaceRegionOptions{
		OlaresDid:      spaceToken.OlaresDid,
		AccessToken:    spaceToken.AccessToken,
		CloudApiMirror: constant.SyncServerURL,
	}

	var regionService = backupssdk.NewRegionService(&backupssdkstorage.RegionOption{
		Ctx:    context.Background(),
		Logger: log.GetLogger(),
		Space:  spaceRegionOption,
	})

	return regionService.Regions()
}
