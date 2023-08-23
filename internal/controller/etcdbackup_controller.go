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
	batch "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1beta1 "github.com/xiaoxin1992/etcd-operator/api/v1beta1"
)

// EtcdBackupReconciler reconciles a EtcdBackup object
type EtcdBackupReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.etcd.io,resources=etcdbackups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.etcd.io,resources=etcdbackups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cluster.etcd.io,resources=etcdbackups/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the EtcdBackup object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.15.0/pkg/reconcile
func (r *EtcdBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	etcdBackup := clusterv1beta1.EtcdBackup{}
	err := r.Get(ctx, req.NamespacedName, &etcdBackup)
	if err != nil {
		// 对于不存在的资源不做处理
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	// 创建cronjob
	var cronJob batch.CronJob
	cronJob.Namespace = etcdBackup.Namespace
	cronJob.Name = etcdBackup.Name
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var op controllerutil.OperationResult
		op, err = ctrl.CreateOrUpdate(ctx, r.Client, &cronJob, func() error {
			r.mutateCronJob(&cronJob, &etcdBackup)
			return controllerutil.SetControllerReference(&etcdBackup, &cronJob, r.Scheme)
		})
		if err != nil {
			r.Recorder.Event(&etcdBackup, corev1.EventTypeWarning, "cronjob", fmt.Sprintf("create cronjob erorr %s", err.Error()))
			logger.Error(err, "CreateOrUpdate Result", "cronjob", op)
		} else {
			r.Recorder.Event(&etcdBackup, corev1.EventTypeNormal, "cronjob", fmt.Sprintf("create cronjob success"))
			logger.Info("CreateOrUpdate Result", "cronjob", op)
		}
		return err
	})
	if err == nil {
		r.updateBackupStatus(ctx, &etcdBackup, &cronJob)
	}
	return ctrl.Result{}, err
}
func (r *EtcdBackupReconciler) updateBackupStatus(ctx context.Context, etcdBackup *clusterv1beta1.EtcdBackup, cronJob *batch.CronJob) {
	logger := log.FromContext(ctx)
	err := controllerutil.SetControllerReference(etcdBackup, cronJob, r.Scheme)
	if err != nil {
		r.Recorder.Event(etcdBackup, corev1.EventTypeWarning, "etcdBackup", fmt.Sprintf("check etcd backup status error %s", err.Error()))
		logger.Error(err, "check etcd backup status error", "etcdBackup", etcdBackup)
		return
	}
	presentStatus := cronJob.Status.DeepCopy()
	newEtcdBackup := etcdBackup.DeepCopy()
	newEtcdBackup.Status.Active = len(presentStatus.Active)

	for _, jobObj := range presentStatus.Active {
		key := client.ObjectKey{
			Namespace: jobObj.Namespace,
			Name:      jobObj.Name,
		}
		var job batch.Job
		err = r.Get(ctx, key, &job)
		if err != nil {
			r.Recorder.Event(etcdBackup, corev1.EventTypeWarning, "etcdBackup", fmt.Sprintf("check etcd backup status error %s", err.Error()))
			logger.Error(err, "check etcd backup status error", "etcdBackup", etcdBackup)
			continue
		}
		newEtcdBackup.Status.Succeeded = job.Status.Succeeded
		newEtcdBackup.Status.Failed = job.Status.Failed
		newEtcdBackup.Status.RunPod = job.Status.Active
	}
	if len(presentStatus.Active) == 0 {
		newEtcdBackup.Status.RunPod = 0
	}
	newEtcdBackup.Status.LastSuccessfulTime = presentStatus.LastScheduleTime
	newEtcdBackup.Status.LastSuccessfulTime = presentStatus.LastSuccessfulTime
	if !reflect.DeepEqual(etcdBackup, newEtcdBackup) {
		err = r.Status().Patch(ctx, newEtcdBackup, client.MergeFrom(etcdBackup))
		if err != nil {
			r.Recorder.Event(etcdBackup, corev1.EventTypeWarning, "etcdBackup", fmt.Sprintf("update etcd backup status error %s", err.Error()))
			logger.Error(err, "update etcd backup status error", "etcdBackup", etcdBackup)
		}
	}
	return
}

func (r *EtcdBackupReconciler) mutateEnv(etcdBackup *clusterv1beta1.EtcdBackup) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "OPTIONTYPE",
			Value: "backup",
		},
		{
			Name:  "BUCKET",
			Value: etcdBackup.Spec.COS.Bucket,
		},
		{
			Name:  "FILENAME",
			Value: etcdBackup.Spec.COS.Filename,
		},
		{
			Name:  "AK",
			Value: etcdBackup.Spec.COS.SecretId,
		},
		{
			Name:  "SK",
			Value: etcdBackup.Spec.COS.SecretKey,
		},
		{
			Name:  "REGION",
			Value: etcdBackup.Spec.COS.Region,
		},
		{
			Name:  "ENDPOINTS",
			Value: etcdBackup.Spec.Endpoints,
		},
	}
}

func (r *EtcdBackupReconciler) mutateCronJob(cronjob *batch.CronJob, etcdBackup *clusterv1beta1.EtcdBackup) {
	containers := make([]corev1.Container, 0)
	for _, container := range etcdBackup.Spec.Job.JobTemplate.Spec.Template.Spec.Containers {
		container.Env = append(container.Env, r.mutateEnv(etcdBackup)...)
		containers = append(containers, container)
	}
	etcdBackup.Spec.Job.JobTemplate.Spec.Template.Spec.Containers = containers
	cronjob.Spec = etcdBackup.Spec.Job
	return
}

// SetupWithManager sets up the controller with the Manager.
func (r *EtcdBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1beta1.EtcdBackup{}).
		Owns(&batch.CronJob{}).
		Owns(&batch.Job{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}
