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

package v1

import (
	"fmt"
	"regexp"
	"strings"

	"backup-operator.io/internal/controller/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var backuprunlog = logf.Log.WithName("backuprun-resource")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *BackupRun) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-backup-operator-io-v1-backuprun,mutating=true,failurePolicy=fail,sideEffects=None,groups=backup-operator.io,resources=backupruns,verbs=create;update,versions=v1,name=mbackuprun.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &BackupRun{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *BackupRun) Default() {
	backuprunlog.Info("default", "name", r.Name)
	r.TemplateStoragePath()
}

// TemplateStoragePath renders the backup path template and validates the result.
// It renders the backup path template using TextTemplateSprig from the utils package
// and updates the Spec.Storage.Path with the rendered result.
// It then validates the rendered path against the backupPathPattern regular expression.
// If the path does not match the pattern, it returns an error.
//
// The backupPathPattern is "^(/[^/]+)*$" and is used to validate the rendered path.
//
// Returns an error if:
// - Rendering the backup path fails.
// - Compiling the backupPathPattern regex fails.
// - The rendered path does not match the backupPathPattern regex.
func (r *BackupRun) TemplateStoragePath() (err error) {
	// Template backupPath
	var renderedPath string
	if renderedPath, err = utils.TextTemplateSprig(r.Spec.Storage.Path, struct{}{}); err != nil {
		err = fmt.Errorf("failed render backup path: %s", err.Error())
		return
	}
	r.Spec.Storage.Path = strings.TrimSpace(renderedPath)
	// Validate result BackupPath
	backupPathPattern := "^(/[^/]+)*$"
	var regex *regexp.Regexp
	if regex, err = regexp.Compile(backupPathPattern); err != nil {
		err = fmt.Errorf("failed to compile backup path regex: %s", err.Error())
	} else {
		if !regex.MatchString(r.Spec.Storage.Path) {
			err = fmt.Errorf("templated backup path does not match regex '%s': %s", backupPathPattern, r.Spec.Storage.Path)
		}
	}
	return
}

//+kubebuilder:webhook:path=/validate-backup-operator-io-v1-backuprun,mutating=false,failurePolicy=fail,sideEffects=None,groups=backup-operator.io,resources=backupruns,verbs=create;update;delete,versions=v1,name=vbackuprun.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &BackupRun{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *BackupRun) ValidateCreate() (admission.Warnings, error) {
	backuprunlog.Info("validate create", "name", r.Name)
	return nil, r.validateBackupRun()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *BackupRun) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	backuprunlog.Info("validate update", "name", r.Name)
	return nil, r.validateBackupRun()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *BackupRun) ValidateDelete() (admission.Warnings, error) {
	backuprunlog.Info("validate delete", "name", r.Name)
	return nil, nil
}

// validateBackupRun checks the overall BackupRun for correctness by validating its spec.
// It calls validateBackupRunSpec to validate the spec and aggregates any validation errors into
// a field.ErrorList. If there are no validation errors, it returns nil. Otherwise, it returns
// an apierrors.Invalid error containing the aggregated field.ErrorList.
func (r *BackupRun) validateBackupRun() error {
	var allErrs field.ErrorList
	if err := r.validateBackupRunSpec(); err != nil {
		allErrs = append(allErrs, err)
	}
	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{
			Group: r.GroupVersionKind().Group,
			Kind:  r.Kind,
		}, r.Name, allErrs)
}

// validateBackupRunSpec checks the BackupRun spec for correctness and returns a field.Error if validation fails.
// The function validates that either the Backup or Restore block is set, and if Restore is set, it checks
// for the presence of the decryption key in the Encryption block.
// Returns nil if the spec is valid, otherwise returns a field.Error indicating the validation error.
func (r *BackupRun) validateBackupRunSpec() (err *field.Error) {
	if r.Spec.Backup == nil && r.Spec.Restore == nil {
		fld := field.NewPath("spec")
		msg := "neither the backup nor the restore block has been set, but at least one is required"
		err = field.Invalid(fld, r.Spec, msg)
	} else if r.Spec.Restore != nil && r.Spec.Encryption != nil && r.Spec.Encryption.DecryptionKey == nil {
		fld := field.NewPath("spec").Child("encryption").Child("decryptionKey")
		msg := "both restore and encryption blocks are present, but not decryption key provided for decryption"
		err = field.Invalid(fld, r.Spec.Encryption, msg)
	} else if r.Spec.Backup == nil && r.Spec.Restore != nil && (*r.Spec.RetainPolicy) != BackupRetainRetain {
		fld := field.NewPath("spec").Child("RetainPolicy")
		msg := "only Retain policy is allowed for .spec.retainPolicy in restore-only mode"
		err = field.Invalid(fld, r.Spec.RetainPolicy, msg)
	} else if e := r.TemplateStoragePath(); e != nil {
		fld := field.NewPath("spec").Child("storage").Child("path")
		msg := e.Error()
		err = field.Invalid(fld, r.Spec.Storage.Path, msg)
	}
	return
}
