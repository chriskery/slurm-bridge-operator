package v1alpha1

import (
	"fmt"
	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
)

func ValidateV1alphaSlurmBridgeJob(SlurmBridgeJob *SlurmBridgeJob) error {
	if errs := apimachineryvalidation.NameIsDNS1035Label(SlurmBridgeJob.ObjectMeta.Name, false); errs != nil {
		return fmt.Errorf("TFSlurmBridgeJob name is invalid: %v", errs)
	}
	if err := validateV1alphaSlurmBridgeJobSpecs(SlurmBridgeJob.Spec); err != nil {
		return err
	}
	return nil
}

func validateV1alphaSlurmBridgeJobSpecs(spec SlurmBridgeJobSpec) error {
	if len(spec.Partition) == 0 {
		return fmt.Errorf("SlurmBridgeJob is not valid: SlurmBridgeJob partition expected")
	}
	if len(spec.SbatchScript) == 0 {
		return fmt.Errorf("SlurmBridgeJob is not valid: SlurmBridgeJob SbatchScript expected")
	}
	return nil
}
