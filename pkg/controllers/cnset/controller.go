// Copyright 2022 Matrix Origin
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cnset

import (
	"github.com/matrixorigin/matrixone-operator/api/core/v1alpha1"
	"github.com/matrixorigin/matrixone-operator/pkg/controllers/common"
	recon "github.com/matrixorigin/matrixone-operator/runtime/pkg/reconciler"
	"github.com/matrixorigin/matrixone-operator/runtime/pkg/util"
	kruise "github.com/openkruise/kruise-api/apps/v1beta1"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"go.uber.org/multierr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type CNSetActor struct{}

var _ recon.Actor[*v1alpha1.CNSet] = &CNSetActor{}

type WithResources struct {
	*CNSetActor
	sts *kruise.StatefulSet
}

func (c *CNSetActor) with(sts *kruise.StatefulSet) *WithResources {
	return &WithResources{CNSetActor: c, sts: sts}
}

func (c *CNSetActor) Observe(ctx *recon.Context[*v1alpha1.CNSet]) (recon.Action[*v1alpha1.CNSet], error) {
	cn := ctx.Obj

	svc := &corev1.Service{}
	err, foundSvc := util.IsFound(ctx.Get(client.ObjectKey{Namespace: cn.Namespace, Name: svcName(cn)}, svc))
	if err != nil {
		return nil, errors.Wrap(err, "get cn service")
	}

	sts := &kruise.StatefulSet{}
	err, foundSts := util.IsFound(ctx.Get(client.ObjectKey{Namespace: cn.Namespace, Name: stsName(cn)}, sts))
	if err != nil {
		return nil, errors.Wrap(err, "get cn statefulset")
	}

	if !foundSts || !foundSvc {
		return c.Create, nil
	}

	origin := sts.DeepCopy()
	if err := syncPods(ctx, sts); err != nil {
		return nil, err
	}
	if !equality.Semantic.DeepEqual(origin, sts) {
		return c.with(sts).Update, nil
	}

	return nil, nil

}

func (c *WithResources) Scale(ctx *recon.Context[*v1alpha1.CNSet]) error {
	return ctx.Patch(c.sts, func() error {
		syncReplicas(ctx.Obj, c.sts)
		return nil
	})
}

func (c *WithResources) Update(ctx *recon.Context[*v1alpha1.CNSet]) error {
	return ctx.Update(c.sts)
}

func (c *CNSetActor) Finalize(ctx *recon.Context[*v1alpha1.CNSet]) (bool, error) {
	cn := ctx.Obj

	objs := []client.Object{&corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name: headlessSvcName(cn),
	}}, &kruise.StatefulSet{ObjectMeta: metav1.ObjectMeta{
		Name: stsName(cn),
	}}, &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name: svcName(cn),
	}}}
	for _, obj := range objs {
		obj.SetNamespace(cn.Namespace)
		if err := util.Ignore(apierrors.IsNotFound, ctx.Delete(obj)); err != nil {
			return false, err
		}
	}
	for _, obj := range objs {
		exist, err := ctx.Exist(client.ObjectKeyFromObject(obj), obj)
		if err != nil {
			return false, err
		}
		if exist {
			return false, nil
		}
	}

	return true, nil
}

func (c *CNSetActor) Create(ctx *recon.Context[*v1alpha1.CNSet]) error {
	klog.V(recon.Info).Info("dn set create...")
	cn := ctx.Obj

	hSvc := buildHeadlessSvc(cn)
	cnSet := buildCNSet(cn)
	svc := buildSvc(cn)
	syncReplicas(cn, cnSet)
	syncPodMeta(cn, cnSet)
	syncPodSpec(cn, cnSet)
	syncPersistentVolumeClaim(cn, cnSet)
	configMap, err := buildCNSetConfigMap(cn)
	if err != nil {
		return err
	}

	if err := common.SyncConfigMap(ctx, &cnSet.Spec.Template.Spec, configMap); err != nil {
		return err
	}

	// create all resources
	err = lo.Reduce[client.Object, error]([]client.Object{
		hSvc,
		svc,
		cnSet,
	}, func(errs error, o client.Object, _ int) error {
		err := ctx.CreateOwned(o)
		return multierr.Append(errs, util.Ignore(apierrors.IsAlreadyExists, err))
	}, nil)
	if err != nil {
		return errors.Wrap(err, "create dn service")
	}

	return nil
}

func (c *CNSetActor) Reconcile(mgr manager.Manager) error {
	err := recon.Setup[*v1alpha1.CNSet](&v1alpha1.CNSet{}, "cnset", mgr, c,
		recon.WithBuildFn(func(b *builder.Builder) {
			b.Owns(&kruise.StatefulSet{}).
				Owns(&corev1.Service{})
		}))
	if err != nil {
		return err
	}

	return nil
}
func syncPods(ctx *recon.Context[*v1alpha1.CNSet], sts *kruise.StatefulSet) error {
	cm, err := buildCNSetConfigMap(ctx.Obj)
	if err != nil {
		return err
	}

	syncPodMeta(ctx.Obj, sts)
	syncPodSpec(ctx.Obj, sts)

	return common.SyncConfigMap(ctx, &sts.Spec.Template.Spec, cm)
}
