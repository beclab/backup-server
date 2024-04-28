
# backup-server

[![](https://github.com/beclab/backup-server/actions/workflows/release.yaml/badge.svg?branch=main)](https://github.com/beclab/backup-server/actions/workflows/release.yaml)

## Description
As part of the system, the backup-server provides functionality for backing up and restoring system and user data.

## Getting Started

### How to build
```bash
make
```

### Running
Before running backup-server, please make sure that the necessary Kubernetes resources it depends on have been created. The resource files are located at /confg/crds/backup/v1/.

```bash
kubectl apply -f ./config/crds/backup/v1/
```

1. API Server
```bash
backup-server apiserver --velero-namespace os-system \
  --velero-service-account os-internal \
  --backup-bucket terminus-us-west-1 \
  --backup-key-prefix <userid>
```

2. Controller
```bash
backup-server controller --velero-namespace os-system \
  --velero-service-account os-internal \
  --backup-bucket terminus-us-west-1 \
  --backup-key-prefix <userid> \
  --backup-retain-days "30"
```

3. VController
```bash
backup-server vcontroller --velero-namespace os-system \
  --velero-service-account os-internal
```

4. SicdecarBackupSync
```bash
backup-sync --log-level debug --sync-interval "10"
```



5. Install Velero
```bash
# create velero backup-location
velero backup-location create terminus-cloud \
  --provider terminus \
  --namespace os-system \
  --prefix "" \
  --bucket terminus-cloud


# install velero plugin
velero install \
  --no-default-backup-location \
  --namespace os-system \
  --image beclab/velero:v1.11.1 \
  --use-volume-snapshots=false \
  --no-secret \
  --plugins beclab/velero-plugin-for-terminus:v1.0.1 \
  --wait
```

6. Create Backup-Plan

Create a backup task on the Backup page of the Terminus system Settings.