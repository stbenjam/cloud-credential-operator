/*
Copyright 2018 The OpenShift Authors.

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
	configv1 "github.com/openshift/api/config/v1"
	awsactuator "github.com/openshift/cloud-credential-operator/pkg/aws/actuator"
	"github.com/openshift/cloud-credential-operator/pkg/azure"
	gcpactuator "github.com/openshift/cloud-credential-operator/pkg/gcp/actuator"
	"github.com/openshift/cloud-credential-operator/pkg/kubevirt"
	"github.com/openshift/cloud-credential-operator/pkg/openstack"
	"github.com/openshift/cloud-credential-operator/pkg/operator/cleanup"
	"github.com/openshift/cloud-credential-operator/pkg/operator/credentialsrequest"
	"github.com/openshift/cloud-credential-operator/pkg/operator/credentialsrequest/actuator"
	"github.com/openshift/cloud-credential-operator/pkg/operator/loglevel"
	"github.com/openshift/cloud-credential-operator/pkg/operator/metrics"
	"github.com/openshift/cloud-credential-operator/pkg/operator/platform"
	"github.com/openshift/cloud-credential-operator/pkg/operator/secretannotator"
	"github.com/openshift/cloud-credential-operator/pkg/operator/status"
	"github.com/openshift/cloud-credential-operator/pkg/operator/utils"
	"github.com/openshift/cloud-credential-operator/pkg/ovirt"
	"github.com/openshift/cloud-credential-operator/pkg/util"
	vsphereactuator "github.com/openshift/cloud-credential-operator/pkg/vsphere/actuator"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/manager"

	log "github.com/sirupsen/logrus"
)

const (
	installConfigMap   = "cluster-config-v1"
	installConfigMapNS = "kube-system"
)

func init() {
	AddToManagerFuncs = append(AddToManagerFuncs, metrics.Add)
	AddToManagerFuncs = append(AddToManagerFuncs, secretannotator.Add)
	AddToManagerFuncs = append(AddToManagerFuncs, status.Add)
	AddToManagerFuncs = append(AddToManagerFuncs, loglevel.Add)
	AddToManagerFuncs = append(AddToManagerFuncs, cleanup.Add)
	AddToManagerWithActuatorFuncs = append(AddToManagerWithActuatorFuncs, credentialsrequest.AddWithActuator)
}

// AddToManagerFuncs is a list of functions to add all Controllers to the Manager.
// String parameter is to pass in any specific override for the kubeconfig file to use.
var AddToManagerFuncs []func(manager.Manager, manager.Manager, string) error

// AddToManagerWithActuatorFuncs is a list of functions to add all Controllers with Actuators to the Manager
var AddToManagerWithActuatorFuncs []func(manager.Manager, manager.Manager, actuator.Actuator, configv1.PlatformType, corev1client.CoreV1Interface) error

// AddToManager adds all Controllers to the Manager
func AddToManager(m, rootM manager.Manager, explicitKubeconfig string, coreClient corev1client.CoreV1Interface) error {
	for _, f := range AddToManagerFuncs {
		if err := f(m, rootM, explicitKubeconfig); err != nil {
			return err
		}
	}
	for _, f := range AddToManagerWithActuatorFuncs {
		// Check for supported platform types, dummy if not found:
		// TODO: Use infrastructure type to determine this in future, it's not being populated yet:
		// https://github.com/openshift/api/blob/master/config/v1/types_infrastructure.go#L11
		var err error
		var a actuator.Actuator
		infraStatus, err := platform.GetInfraStatusUsingKubeconfig(explicitKubeconfig)
		if err != nil {
			log.Fatal(err)
		}
		platformType := platform.GetType(infraStatus)
		switch platformType {
		case configv1.AWSPlatformType:
			log.Info("initializing AWS actuator")
			a, err = awsactuator.NewAWSActuator(m.GetClient(), rootM.GetClient(), utils.LiveClient(m), m.GetScheme())
			if err != nil {
				return err
			}
		case configv1.AzurePlatformType:
			log.Info("initializing Azure actuator")
			a, err = azure.NewActuator(m.GetClient(), rootM.GetClient(), util.GetAzureCloudName(infraStatus))
			if err != nil {
				return err
			}
		case configv1.OpenStackPlatformType:
			log.Info("initializing OpenStack actuator")
			a, err = openstack.NewOpenStackActuator(m.GetClient(), rootM.GetClient())
			if err != nil {
				return err
			}
		case configv1.GCPPlatformType:
			log.Info("initializing GCP actuator")
			if infraStatus.PlatformStatus == nil || infraStatus.PlatformStatus.GCP == nil {
				log.Fatalf("missing GCP configuration in platform status")
			}
			a, err = gcpactuator.NewActuator(m.GetClient(), rootM.GetClient(), infraStatus.PlatformStatus.GCP.ProjectID)
			if err != nil {
				return err
			}
		case configv1.OvirtPlatformType:
			log.Info("initializing Ovirt actuator")
			if infraStatus.PlatformStatus == nil || infraStatus.PlatformStatus.Ovirt == nil {
				log.Fatalf("missing Ovirt configuration in platform status")
			}
			a, err = ovirt.NewActuator(m.GetClient(), rootM.GetClient())
			if err != nil {
				return err
			}
		case configv1.VSpherePlatformType:
			log.Info("initializing VSphere actuator")
			a, err = vsphereactuator.NewVSphereActuator(m.GetClient(), rootM.GetClient())
			if err != nil {
				return err
			}
		case configv1.KubevirtPlatformType:
			log.Info("initializing Kubevirt actuator")
			a, err = kubevirt.NewActuator(m.GetClient(), rootM.GetClient())
			if err != nil {
				return err
			}
		default:
			log.Info("initializing no-op actuator (unsupported platform)")
			a = &actuator.DummyActuator{}
		}
		if err := f(m, rootM, a, platformType, coreClient); err != nil {
			return err
		}
	}
	return nil
}
