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
	"fmt"

	"github.com/go-logr/logr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hanav1 "github.com/SAP-samples/hana-cloud-instance-mapping-operator-for-kyma/api/v1"
	"github.com/SAP-samples/hana-cloud-instance-mapping-operator-for-kyma/internal/inventory"
)

const (
	hanamappingName              = "test-hanamapping"
	hanamappingServiceInstanceID = "test-serviceinstanceid"
	hanamappingTargetNamespace   = "test-targetnamespace"
)

var _ = Describe("HANAMapping Controller", func() {
	var (
		log logr.Logger
		ctx context.Context
	)

	BeforeEach(func() {
		log = ctrl.Log.WithName("test-log")
		ctx = context.Background()
	})

	AfterEach(func() {
	})

	Describe("invalid BTP operator configuration", func() {
		BeforeEach(func() {
			hanamapping := newHANAMapping(hanamappingName)
			hanamapping.Spec.BTPOperatorConfigmap.Name = "invalid"
			Expect(k8sClient.Create(ctx, hanamapping)).To(Succeed())
		})

		AfterEach(func() {
			hanamapping := &hanav1.HANAMapping{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: hanamappingName}, hanamapping)
			Expect(err).NotTo(HaveOccurred())

			hanamapping.ObjectMeta.Finalizers = []string{}
			Expect(k8sClient.Update(ctx, hanamapping)).To(Succeed())

			Expect(k8sClient.Delete(ctx, hanamapping)).To(Succeed())
		})

		It("should fail to reconcile a mapping", func() {
			controllerReconciler := &HANAMappingReconciler{
				Client:             k8sClient,
				Log:                log,
				Scheme:             k8sClient.Scheme(),
				GetInventoryClient: func(adminAPIAccessBinding inventory.Binding) inventory.Client { return &inventoryClientStub{} },
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: hanamappingName},
			})

			Expect(err).To(HaveOccurred())
		})
	})

	Describe("invalid admin api access secret", func() {
		BeforeEach(func() {
			hanamapping := newHANAMapping(hanamappingName)
			hanamapping.Spec.AdminAPIAccessSecret.Name = "invalid"
			Expect(k8sClient.Create(ctx, hanamapping)).To(Succeed())
		})

		AfterEach(func() {
			hanamapping := &hanav1.HANAMapping{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: hanamappingName}, hanamapping)
			Expect(err).NotTo(HaveOccurred())

			hanamapping.ObjectMeta.Finalizers = []string{}
			Expect(k8sClient.Update(ctx, hanamapping)).To(Succeed())

			Expect(k8sClient.Delete(ctx, hanamapping)).To(Succeed())
		})

		It("should fail to reconcile a mapping", func() {
			controllerReconciler := &HANAMappingReconciler{
				Client:             k8sClient,
				Log:                log,
				Scheme:             k8sClient.Scheme(),
				GetInventoryClient: func(adminAPIAccessBinding inventory.Binding) inventory.Client { return &inventoryClientStub{} },
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: hanamappingName},
			})

			Expect(err).To(HaveOccurred())
		})
	})

	Describe("create hanamapping CR", func() {
		AfterEach(func() {
			hanamapping := &hanav1.HANAMapping{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: hanamappingName}, hanamapping)
			Expect(err).NotTo(HaveOccurred())

			hanamapping.ObjectMeta.Finalizers = []string{}
			Expect(k8sClient.Update(ctx, hanamapping)).To(Succeed())

			Expect(k8sClient.Delete(ctx, hanamapping)).To(Succeed())
		})

		It("should succeed to reconcile a mapping", func() {
			hanamapping := newHANAMapping(hanamappingName)
			Expect(k8sClient.Create(ctx, hanamapping)).To(Succeed())

			inventoryClientStub := &inventoryClientStub{}
			inventoryClientStub.CreateMappingReturns(nil)

			controllerReconciler := &HANAMappingReconciler{
				Client:             k8sClient,
				Log:                log,
				Scheme:             k8sClient.Scheme(),
				GetInventoryClient: func(adminAPIAccessBinding inventory.Binding) inventory.Client { return inventoryClientStub },
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: hanamappingName},
			})

			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: hanamappingName}, hanamapping)).To(Succeed())
			Expect(len(hanamapping.Status.Conditions)).Should(Equal(1))
			Expect(hanamapping.Status.Conditions[0].Type).Should(Equal(conditionTypeReady))
			Expect(hanamapping.Status.Conditions[0].Status).Should(Equal(metav1.ConditionTrue))
			Expect(hanamapping.Status.Conditions[0].Reason).Should(Equal(conditionReasonSucceeded))
		})

		It("should requeue a mapping", func() {
			hanamapping := newHANAMapping(hanamappingName)
			hanamapping.Spec.Mapping.TargetNamespace = ""
			Expect(k8sClient.Create(ctx, hanamapping)).To(Succeed())

			inventoryClientStub := &inventoryClientStub{}
			inventoryClientStub.CreateMappingReturns(nil)

			controllerReconciler := &HANAMappingReconciler{
				Client:             k8sClient,
				Log:                log,
				Scheme:             k8sClient.Scheme(),
				GetInventoryClient: func(adminAPIAccessBinding inventory.Binding) inventory.Client { return inventoryClientStub },
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: hanamappingName},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).Should(Equal(true))

			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: hanamappingName}, hanamapping)).To(Succeed())
			Expect(hanamapping.Spec.Mapping.TargetNamespace).Should(Equal(hanamapping.Namespace))
			Expect(len(hanamapping.Status.Conditions)).Should(Equal(1))
			Expect(hanamapping.Status.Conditions[0].Type).Should(Equal(conditionTypeReady))
			Expect(hanamapping.Status.Conditions[0].Status).Should(Equal(metav1.ConditionFalse))
			Expect(hanamapping.Status.Conditions[0].Reason).Should(Equal(conditionReasonInProgress))
		})

		It("should fail to reconcile a mapping", func() {
			hanamapping := newHANAMapping(hanamappingName)
			Expect(k8sClient.Create(ctx, hanamapping)).To(Succeed())

			inventoryClientStub := &inventoryClientStub{}
			inventoryClientStub.CreateMappingReturns(fmt.Errorf("inventory error"))

			controllerReconciler := &HANAMappingReconciler{
				Client:             k8sClient,
				Log:                log,
				Scheme:             k8sClient.Scheme(),
				GetInventoryClient: func(adminAPIAccessBinding inventory.Binding) inventory.Client { return inventoryClientStub },
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: hanamappingName},
			})

			Expect(err).To(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: hanamappingName}, hanamapping)).To(Succeed())
			Expect(len(hanamapping.Status.Conditions)).Should(Equal(1))
			Expect(hanamapping.Status.Conditions[0].Type).Should(Equal(conditionTypeReady))
			Expect(hanamapping.Status.Conditions[0].Status).Should(Equal(metav1.ConditionFalse))
			Expect(hanamapping.Status.Conditions[0].Reason).Should(Equal(conditionReasonFailed))
		})
	})

	Describe("delete hanamapping CR", func() {
		BeforeEach(func() {
			hanamapping := newHANAMapping(hanamappingName)
			Expect(k8sClient.Create(ctx, hanamapping)).To(Succeed())

			hanamapping.ObjectMeta.Finalizers = []string{finalizerName}
			Expect(k8sClient.Update(ctx, hanamapping)).To(Succeed())

			hanamapping.Status.Conditions = []metav1.Condition{{
				LastTransitionTime: metav1.Now(),
				Type:               conditionTypeReady,
				Status:             metav1.ConditionTrue,
				Reason:             conditionReasonSucceeded,
				Message:            "",
			}}
			hanamapping.Status.MappingID = &hanav1.MappingID{
				ServiceInstanceID: hanamappingServiceInstanceID,
				PrimaryID:         clusterID,
				SecondaryID:       hanamappingTargetNamespace,
			}
			Expect(k8sClient.Status().Update(ctx, hanamapping)).To(Succeed())

			Expect(k8sClient.Delete(ctx, hanamapping)).To(Succeed())
		})

		AfterEach(func() {
			hanamapping := &hanav1.HANAMapping{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: hanamappingName}, hanamapping)
			if err == nil {
				hanamapping.ObjectMeta.Finalizers = []string{}
				Expect(k8sClient.Update(ctx, hanamapping)).To(Succeed())
			}
		})

		It("should succeed to delete a mapping", func() {
			inventoryClientStub := &inventoryClientStub{}
			inventoryClientStub.DeleteMappingReturns(nil)

			controllerReconciler := &HANAMappingReconciler{
				Client:             k8sClient,
				Log:                log,
				Scheme:             k8sClient.Scheme(),
				GetInventoryClient: func(adminAPIAccessBinding inventory.Binding) inventory.Client { return inventoryClientStub },
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: hanamappingName},
			})

			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail to delete a mapping", func() {
			inventoryClientStub := &inventoryClientStub{}
			inventoryClientStub.DeleteMappingReturns(fmt.Errorf("inventory error"))

			controllerReconciler := &HANAMappingReconciler{
				Client:             k8sClient,
				Log:                log,
				Scheme:             k8sClient.Scheme(),
				GetInventoryClient: func(adminAPIAccessBinding inventory.Binding) inventory.Client { return inventoryClientStub },
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: hanamappingName},
			})

			Expect(err).To(HaveOccurred())
		})
	})
})

func newHANAMapping(name string) *hanav1.HANAMapping {
	hanamapping := &hanav1.HANAMapping{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      name,
		},
		Spec: hanav1.HANAMappingSpec{
			BTPOperatorConfigmap: hanav1.NamespacedName{
				Namespace: testNamespace,
				Name:      btpOperatorConfigmap,
			},
			AdminAPIAccessSecret: hanav1.NamespacedName{
				Namespace: testNamespace,
				Name:      adminAPIAccessSecret,
			},
			Mapping: hanav1.Mapping{
				ServiceInstanceID: hanamappingServiceInstanceID,
				TargetNamespace:   hanamappingTargetNamespace,
			},
		},
	}
	return hanamapping
}

type inventoryClientStub struct {
	listMappingsReturns struct {
		mappings []inventory.Mapping
		err      error
	}
	createMappingsReturns error
	deleteMappingsReturns error
}

func (c *inventoryClientStub) ListMappingsReturns(mappings []inventory.Mapping, err error) {
	c.listMappingsReturns = struct {
		mappings []inventory.Mapping
		err      error
	}{
		mappings: mappings,
		err:      err,
	}
}

func (c *inventoryClientStub) CreateMappingReturns(err error) {
	c.createMappingsReturns = err
}

func (c *inventoryClientStub) DeleteMappingReturns(err error) {
	c.deleteMappingsReturns = err
}

func (c *inventoryClientStub) ListMappings(ctx context.Context, serviceInstanceID string) ([]inventory.Mapping, error) {
	return c.listMappingsReturns.mappings, c.listMappingsReturns.err
}

func (c *inventoryClientStub) CreateMapping(ctx context.Context, serviceInstanceID string, mapping inventory.Mapping) error {
	return c.createMappingsReturns
}

func (c *inventoryClientStub) DeleteMapping(ctx context.Context, serviceInstanceID string, primaryID, secondaryID string) error {
	return c.deleteMappingsReturns
}
