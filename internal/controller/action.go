package controller

import (
	"context"
	"fmt"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Action interface {
	Execute(ctx context.Context) error
}

type PatchStatus struct {
	Client    client.Client
	origin    client.Object
	newObject client.Object
}

func (p PatchStatus) Execute(ctx context.Context) error {
	if reflect.DeepEqual(p.origin, p.newObject) {
		return nil
	}
	if err := p.Client.Status().Patch(ctx, p.newObject, client.MergeFrom(p.origin)); err != nil {
		return fmt.Errorf("update status error %s", err.Error())
	}
	return nil
}
