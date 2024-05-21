/*
Copyright 2024.

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
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	hanav1 "github.com/SAP-samples/hana-cloud-instance-mapping-operator-for-kyma/api/v1"
	"github.com/SAP-samples/hana-cloud-instance-mapping-operator-for-kyma/internal/inventory"
)

const (
	defaultBTPOperatorConfigmapNamespace = "kyma-system"
	defaultBTPOperatorConfigmapName      = "sap-btp-operator-config"

	finalizerName = "hanamappings.hana.cloud.sap.com/finalizer"

	conditionTypeReady = "Ready"

	conditionReasonInProgress = "InProgress"
	conditionReasonSucceeded  = "Succeeded"
	conditionReasonFailed     = "Failed"
)

// HANAMappingReconciler reconciles a HANAMapping object
type HANAMappingReconciler struct {
	Client             client.Client
	Log                logr.Logger
	Scheme             *runtime.Scheme
	GetInventoryClient func(adminAPIAccessBinding inventory.Binding) inventory.Client
}

// SetupWithManager sets up the controller with the Manager.
func (r *HANAMappingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&hanav1.HANAMapping{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}

//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups=hana.cloud.sap.com,resources=hanamappings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=hana.cloud.sap.com,resources=hanamappings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=hana.cloud.sap.com,resources=hanamappings/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the HANAMapping object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *HANAMappingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("hanamapping", req.NamespacedName).WithValues("correlation_id", uuid.New().String())

	hanaMapping := &hanav1.HANAMapping{}
	if err := r.Client.Get(ctx, req.NamespacedName, hanaMapping); err != nil {
		if !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	log.Info(fmt.Sprintf("got hanamapping gen %d", hanaMapping.Generation))
	hanaMapping = hanaMapping.DeepCopy()

	if !hanaMapping.ObjectMeta.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(hanaMapping, finalizerName) {
			if err := r.deleteMapping(ctx, hanaMapping); err != nil {
				if statusErr := r.setStatusFailed(ctx, hanaMapping, err); statusErr != nil {
					return ctrl.Result{}, statusErr
				}
				return ctrl.Result{}, err
			}
			log.Info("deleted mapping")

			controllerutil.RemoveFinalizer(hanaMapping, finalizerName)
			if err := r.Client.Update(ctx, hanaMapping); err != nil {
				return ctrl.Result{}, err
			}
			log.Info("removed finalizer")
		}

		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(hanaMapping, finalizerName) {
		controllerutil.AddFinalizer(hanaMapping, finalizerName)
		if err := r.Client.Update(ctx, hanaMapping); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("added finalizer")
	}

	if hanaMapping.Status.Conditions == nil {
		hanaMapping.Status.Conditions = make([]metav1.Condition, 0)
		if statusErr := r.setStatusInProgress(ctx, hanaMapping); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		log.Info("initialized status")
	}

	if len(hanaMapping.Spec.Mapping.TargetNamespace) == 0 {
		hanaMapping.Spec.Mapping.TargetNamespace = hanaMapping.Namespace
		if err := r.Client.Update(ctx, hanaMapping); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("set default targetNamespace")
		return ctrl.Result{Requeue: true}, nil
	}

	newMappingID, err := r.syncMapping(ctx, hanaMapping)
	if err != nil {
		if statusErr := r.setStatusFailed(ctx, hanaMapping, err); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, err
	}
	log.Info("synced mapping")

	if statusErr := r.setStatusSucceeded(ctx, hanaMapping, newMappingID); statusErr != nil {
		return ctrl.Result{}, statusErr
	}

	return ctrl.Result{}, nil
}

func (r *HANAMappingReconciler) syncMapping(ctx context.Context, hanaMapping *hanav1.HANAMapping) (*hanav1.MappingID, error) {
	clusterID, err := r.getClusterID(ctx, hanaMapping)
	if err != nil {
		return nil, err
	}

	oldMappingID := hanaMapping.Status.MappingID
	newMappingID := &hanav1.MappingID{
		ServiceInstanceID: hanaMapping.Spec.Mapping.ServiceInstanceID,
		PrimaryID:         clusterID,
		SecondaryID:       hanaMapping.Spec.Mapping.TargetNamespace,
	}
	deleteOldMapping := (oldMappingID != nil) && (*newMappingID != *oldMappingID)
	overwriteNewMapping := (oldMappingID != nil) && (*newMappingID == *oldMappingID)

	adminAPIAccessBinding, err := r.getAdminAPIAccessBinding(ctx, hanaMapping)
	if err != nil {
		return nil, err
	}

	inventoryClient := r.GetInventoryClient(adminAPIAccessBinding)

	if deleteOldMapping {
		inventoryErr := inventoryClient.DeleteMapping(ctx, oldMappingID.ServiceInstanceID, oldMappingID.PrimaryID, oldMappingID.SecondaryID)
		if inventoryErr != nil {
			if inventoryErr != inventory.ErrMappingNotFound {
				return nil, inventoryErr
			}
		}
	}

	mapping := inventory.Mapping{
		Platform:    "kubernetes",
		PrimaryID:   newMappingID.PrimaryID,
		SecondaryID: newMappingID.SecondaryID,
	}

	inventoryErr := inventoryClient.CreateMapping(ctx, newMappingID.ServiceInstanceID, mapping)
	if inventoryErr != nil {
		if !overwriteNewMapping || (overwriteNewMapping && (inventoryErr != inventory.ErrMappingAlreadyExists)) {
			return nil, inventoryErr
		}
	}

	return newMappingID, nil
}

func (r *HANAMappingReconciler) deleteMapping(ctx context.Context, hanaMapping *hanav1.HANAMapping) error {
	mappingID := hanaMapping.Status.MappingID

	if mappingID != nil {
		adminAPIAccessBinding, err := r.getAdminAPIAccessBinding(ctx, hanaMapping)
		if err != nil {
			return err
		}

		inventoryClient := r.GetInventoryClient(adminAPIAccessBinding)

		inventoryErr := inventoryClient.DeleteMapping(ctx, mappingID.ServiceInstanceID, mappingID.PrimaryID, mappingID.SecondaryID)
		if inventoryErr != nil {
			if inventoryErr != inventory.ErrMappingNotFound {
				return inventoryErr
			}
		}
	}

	return nil
}

func (r *HANAMappingReconciler) getClusterID(ctx context.Context, hanaMapping *hanav1.HANAMapping) (string, error) {
	cmNamespace := hanaMapping.Spec.BTPOperatorConfigmap.Namespace
	if len(cmNamespace) == 0 {
		cmNamespace = defaultBTPOperatorConfigmapNamespace
	}

	cmName := hanaMapping.Spec.BTPOperatorConfigmap.Name
	if len(cmName) == 0 {
		cmName = defaultBTPOperatorConfigmapName
	}

	cm := &corev1.ConfigMap{}
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: cmNamespace, Name: cmName}, cm)
	if err != nil {
		return "", err
	}

	clusterID := cm.Data["CLUSTER_ID"]
	return clusterID, nil
}

func (r *HANAMappingReconciler) getAdminAPIAccessBinding(ctx context.Context, hanaMapping *hanav1.HANAMapping) (inventory.Binding, error) {
	secret := &corev1.Secret{}
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: hanaMapping.Spec.AdminAPIAccessSecret.Namespace, Name: hanaMapping.Spec.AdminAPIAccessSecret.Name}, secret)
	if err != nil {
		return inventory.Binding{}, err
	}

	baseURL := string(secret.Data["baseurl"])

	uaa := inventory.BindingUAA{}
	if err = json.Unmarshal(secret.Data["uaa"], &uaa); err != nil {
		return inventory.Binding{}, err
	}

	binding := inventory.Binding{
		BaseURL: baseURL,
		UAA:     uaa,
	}
	return binding, nil
}

func (r *HANAMappingReconciler) setStatusInProgress(ctx context.Context, hanaMapping *hanav1.HANAMapping) error {
	condition := metav1.Condition{
		Type:   conditionTypeReady,
		Status: metav1.ConditionFalse,
		Reason: conditionReasonInProgress,
	}
	meta.SetStatusCondition(&hanaMapping.Status.Conditions, condition)
	return r.Client.Status().Update(ctx, hanaMapping)
}

func (r *HANAMappingReconciler) setStatusSucceeded(ctx context.Context, hanaMapping *hanav1.HANAMapping, mappingID *hanav1.MappingID) error {
	condition := metav1.Condition{
		Type:   conditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: conditionReasonSucceeded,
	}
	meta.SetStatusCondition(&hanaMapping.Status.Conditions, condition)
	hanaMapping.Status.MappingID = mappingID
	return r.Client.Status().Update(ctx, hanaMapping)
}

func (r *HANAMappingReconciler) setStatusFailed(ctx context.Context, hanaMapping *hanav1.HANAMapping, err error) error {
	condition := metav1.Condition{
		Type:    conditionTypeReady,
		Status:  metav1.ConditionFalse,
		Reason:  conditionReasonFailed,
		Message: err.Error(),
	}
	meta.SetStatusCondition(&hanaMapping.Status.Conditions, condition)
	return r.Client.Status().Update(ctx, hanaMapping)
}
