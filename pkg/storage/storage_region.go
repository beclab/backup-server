package storage

import (
	"bufio"
	"context"
	"errors"
	"os/exec"
	"strings"

	"bytetrade.io/web3os/backup-server/pkg/constant"
	integration "bytetrade.io/web3os/backup-server/pkg/integration"
)

func (s *storage) GetRegions(ctx context.Context, olaresId string) (string, error) {
	var tokenService = &integration.Integration{
		Factory:  s.factory,
		Owner:    s.owner,
		Location: constant.BackupLocationSpace.String(),
		Name:     olaresId,
	}

	token, err := tokenService.GetIntegrationToken()
	if err != nil {
		return "", err
	}

	spaceToken := token.(*integration.IntegrationSpace)

	var parms = []string{"region", "space",
		"--olares-did", spaceToken.OlaresDid,
		"--access-token", "bc894878b7b046f08bb5b2e5a9e2c7d5", // spaceToken.AccessToken,
		"--cloud-api-mirror", "https://cloud-dev-api.olares.xyz", //constant.DefaultSyncServerURL,
	}

	cmd := exec.CommandContext(ctx, "/tmp/backup-cli", parms...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return "", err
	}

	var result string
	var errmsg string

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		if line := scanner.Text(); line != "" {
			if strings.Contains(line, "panic:") {
				errmsg += line
				break
			}
			result += line
		}
	}

	defer func() (string, error) {
		if err := cmd.Wait(); err != nil {
			return "", err
		}
		return result, nil
	}()

	if errmsg != "" {
		return "", errors.New(errmsg)
	}

	return result, nil
}
