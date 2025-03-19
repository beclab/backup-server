package storage

import (
	"bytetrade.io/web3os/backup-server/pkg/constant"
	backupssdkoptions "bytetrade.io/web3os/backups-sdk/pkg/options"
	backupssdkstorage "bytetrade.io/web3os/backups-sdk/pkg/storage"
)

func (s *storage) GetRegions() ([]map[string]string, error) {
	olaresDid, olaresAccessToken, _, err := s.GetUserToken()
	if err != nil {
		return nil, err
	}

	regionService := backupssdkstorage.NewRegionService(&backupssdkstorage.RegionOption{
		Space: &backupssdkoptions.SpaceRegionOptions{
			OlaresDid:      olaresDid,
			AccessToken:    olaresAccessToken,
			CloudApiMirror: constant.DefaultSyncServerURL,
		},
	})

	return regionService.Regions()
}
