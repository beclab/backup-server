package velero

import (
	informers "github.com/vmware-tanzu/velero/pkg/generated/informers/externalversions"
)

var stopCh = make(chan struct{})

func (v *velero) NewInformerFactory() (informers.SharedInformerFactory, error) {
	client, err := v.factory.Client()
	if err != nil {
		return nil, err
	}

	sif := informers.NewSharedInformerFactory(client, 0)
	go sif.Start(stopCh)
	return sif, nil
}

func (v *velero) AddInformer() {

}
