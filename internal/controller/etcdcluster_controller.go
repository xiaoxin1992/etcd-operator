/*
Copyright 2023 xiaoxin.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	clusterv1beta1 "github.com/xiaoxin1992/etcd-operator/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"strconv"
)

// EtcdClusterReconciler reconciles a EtcdCluster object
type EtcdClusterReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder // 增加Event记录, 需要载man函数也增加相关事件
}

//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.etcd.io,resources=etcdclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.etcd.io,resources=etcdclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cluster.etcd.io,resources=etcdclusters/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the EtcdCluster object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.15.0/pkg/reconcile
func (r *EtcdClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var etcdCluster clusterv1beta1.EtcdCluster
	if err := r.Get(ctx, req.NamespacedName, &etcdCluster); err != nil {
		// 如果已经被删除了，应该忽略当前资源对象
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	// 判断副本数量是否为奇数
	if *etcdCluster.Spec.Replicas%2 == 0 {
		err := fmt.Errorf("the number of replicas cannot be an odd number")
		r.Recorder.Event(&etcdCluster, corev1.EventTypeWarning, "etcdCluster", err.Error())
		logger.Error(err, "etcdCluster Result", "etcdCluster", etcdCluster)
		return ctrl.Result{}, err
	}
	for _, container := range etcdCluster.Spec.Template.Spec.Containers {
		for _, env := range container.Env {
			if env.Name == "INITIAL_CLUSTER_SIZE" {
				size, err := strconv.Atoi(env.Value)
				if err != nil {
					r.Recorder.Event(&etcdCluster, corev1.EventTypeWarning, "etcdCluster", err.Error())
					return ctrl.Result{}, err
				}
				// INITIAL_CLUSTER_SIZE 的值不能大于当前副本数量，这个是值主要是用于定义初始化集群用到的大小
				// 当第一次初始化集群的时候需要跟replicas数量保持一致之外，其他操作一律不能修改这个值
				if size > int(*etcdCluster.Spec.Replicas) {
					err = fmt.Errorf("replicas %d Cannot be less than %d", *etcdCluster.Spec.Replicas, size)
					r.Recorder.Event(&etcdCluster, corev1.EventTypeWarning, "etcdCluster", err.Error())
					return ctrl.Result{}, err
				}
			}
		}
	}
	// 创建Service资源对象
	var svc corev1.Service
	svc.Namespace = etcdCluster.Namespace
	svc.Name = etcdCluster.Spec.ServiceName
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		op, err := ctrl.CreateOrUpdate(ctx, r.Client, &svc, func() error {
			r.mutateHeadLessSvc(&svc, etcdCluster)
			return controllerutil.SetControllerReference(&etcdCluster, &svc, r.Scheme)
		})
		if err != nil {
			r.Recorder.Event(&etcdCluster, corev1.EventTypeWarning, "service", fmt.Sprintf("create services erorr %s", err.Error()))
			logger.Error(err, "CreateOrUpdate Service Result", "Service", op)
		} else {
			r.Recorder.Event(&etcdCluster, corev1.EventTypeNormal, "service", fmt.Sprintf("create services success"))
			logger.Info("CreateOrUpdate Service Result", "Service", op)
		}
		return err
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	//创建statefulSet
	var sts appsv1.StatefulSet
	sts.Namespace = etcdCluster.Namespace
	sts.Name = fmt.Sprintf("%s-etcd", etcdCluster.Name)
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var op controllerutil.OperationResult
		op, err = ctrl.CreateOrUpdate(ctx, r.Client, &sts, func() error {
			r.mutateStateFulSet(&sts, &etcdCluster)
			return controllerutil.SetControllerReference(&etcdCluster, &sts, r.Scheme)
		})
		if err != nil {
			r.Recorder.Event(&etcdCluster, corev1.EventTypeWarning, "statefulSet", fmt.Sprintf("create statefulSet erorr %s", err.Error()))
			logger.Error(err, "CreateOrUpdate Result", "statefulSet", op)
		} else {
			r.Recorder.Event(&etcdCluster, corev1.EventTypeNormal, "statefulSet", fmt.Sprintf("create statefulSet success"))
			logger.Info("CreateOrUpdate Result", "statefulSet", op)
		}
		return err
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	r.updateClusterStatus(ctx, &etcdCluster, &sts)
	return ctrl.Result{}, nil
}

func (r *EtcdClusterReconciler) updateClusterStatus(ctx context.Context, etcdCluster *clusterv1beta1.EtcdCluster, sts *appsv1.StatefulSet) {
	logger := log.FromContext(ctx)
	err := controllerutil.SetControllerReference(etcdCluster, sts, r.Scheme)
	if err != nil {
		r.Recorder.Event(etcdCluster, corev1.EventTypeWarning, "etcdCluster", fmt.Sprintf("check etcdCluster status error %s", err.Error()))
		logger.Error(err, "check etcd cluster status error", "etcdCluster", etcdCluster)
		return
	}
	presentStatus := sts.Status.DeepCopy()
	newsEtcdCluster := etcdCluster.DeepCopy()
	newsEtcdCluster.Status = clusterv1beta1.EtcdClusterStatus{
		ObservedGeneration: presentStatus.ObservedGeneration,
		Replicas:           presentStatus.Replicas,
		ReadyReplicas:      presentStatus.ReadyReplicas,
		CurrentReplicas:    presentStatus.CurrentReplicas,
		UpdatedReplicas:    presentStatus.UpdatedReplicas,
		CurrentRevision:    presentStatus.CurrentRevision,
		UpdateRevision:     presentStatus.UpdateRevision,
		CollisionCount:     presentStatus.CollisionCount,
		Conditions:         presentStatus.Conditions,
		AvailableReplicas:  presentStatus.AvailableReplicas,
		Ready:              fmt.Sprintf("%d/%d", presentStatus.ReadyReplicas, *etcdCluster.Spec.Replicas),
	}
	if !reflect.DeepEqual(etcdCluster, newsEtcdCluster) {
		err = r.Status().Patch(ctx, newsEtcdCluster, client.MergeFrom(etcdCluster))
		if err != nil {
			r.Recorder.Event(etcdCluster, corev1.EventTypeWarning, "etcdCluster", fmt.Sprintf("update etcdCluster status error %s", err.Error()))
			logger.Error(err, "update etcd cluster status error", "etcdCluster", etcdCluster)
		}
	}
	return
}
func (r *EtcdClusterReconciler) mutateStateFulSet(sts *appsv1.StatefulSet, etcdCluster *clusterv1beta1.EtcdCluster) {
	// 创建statefulSet
	// MY_NAMESPACE="defualt"      当前POD命名空间
	// INITIAL_CLUSTER_SIZE=3      集群节点数量，必须是奇数
	// CLUSTER_TOKEN="new-clutesr" 集群初始化Token
	// CLUSTER_NAME=name1          集群名称
	// SERVICE_NAME=nginx          service名称
	sts.Labels = etcdCluster.Labels
	sts.Spec = appsv1.StatefulSetSpec{
		Replicas: etcdCluster.Spec.Replicas,
		Selector: etcdCluster.Spec.Selector,
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: etcdCluster.Spec.Template.Labels,
			},
			Spec: etcdCluster.Spec.Template.Spec,
		},
		VolumeClaimTemplates:                 etcdCluster.Spec.VolumeClaimTemplates,
		ServiceName:                          etcdCluster.Spec.ServiceName,
		PodManagementPolicy:                  etcdCluster.Spec.PodManagementPolicy,
		UpdateStrategy:                       etcdCluster.Spec.UpdateStrategy,
		RevisionHistoryLimit:                 etcdCluster.Spec.RevisionHistoryLimit,
		MinReadySeconds:                      etcdCluster.Spec.MinReadySeconds,
		PersistentVolumeClaimRetentionPolicy: etcdCluster.Spec.PersistentVolumeClaimRetentionPolicy,
		Ordinals:                             etcdCluster.Spec.Ordinals,
	}
}
func (r *EtcdClusterReconciler) mutateHeadLessSvc(svc *corev1.Service, etcdCluster clusterv1beta1.EtcdCluster) {
	// 创建service资源
	svc.Labels = etcdCluster.Labels
	svc.Spec = corev1.ServiceSpec{
		Ports:     etcdCluster.Spec.Ports,
		Selector:  etcdCluster.Spec.Template.Labels,
		ClusterIP: corev1.ClusterIPNone,
	}
	return
}

// SetupWithManager sets up the controller with the Manager.
func (r *EtcdClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1beta1.EtcdCluster{}).
		Owns(&appsv1.StatefulSet{}).Owns(&corev1.Service{}).
		Complete(r)
}
