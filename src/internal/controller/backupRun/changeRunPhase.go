/*
Copyright 2023.

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

package backuprun

import (
	"context"
	"fmt"

	"backup-operator.io/internal/controller/utils"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	backupoperatoriov1 "backup-operator.io/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Change run conditions and status according to the new state
func ChangeRunState(ctx context.Context, c client.Client,
	run *backupoperatoriov1.BackupRun, ct backupoperatoriov1.BackupRunConditionType,
	state *BackupRunState) (err error) {

	return retry.RetryOnConflict(retry.DefaultRetry, func() (err error) {
		if err = c.Get(ctx, client.ObjectKeyFromObject(run), run); err != nil {
			return err
		}
		// We prepare reason, message and each condition status basing on ct we have received
		var reason, message string
		var inProgress, failed, successful metav1.ConditionStatus
		switch ct {
		case backupoperatoriov1.BackupRunConditionTypeInProgress:
			inProgressRuns.Store(run.UID, true)
			inProgress = metav1.ConditionTrue
			failed = metav1.ConditionFalse
			successful = metav1.ConditionFalse
			switch {
			case state.HaveToBackup:
				reason = "Backuping"
				message = "Making a backup"
				run.Status.State = ptr.To[string]("Backuping")
			case state.HaveToRestore:
				reason = "Restoring"
				message = "Restoring the backup"
				run.Status.State = ptr.To[string]("Restoring")
			default:
				reason = "Unknown"
				message = string(ct)
				run.Status.State = ptr.To[string]("Unknown")
			}
		case backupoperatoriov1.BackupRunConditionTypeFailed:
			inProgress = metav1.ConditionFalse
			failed = metav1.ConditionTrue
			successful = metav1.ConditionFalse
			switch {
			case state.HaveToBackup:
				reason = "BackupFailed"
				message = "Backup failed"
				run.Status.State = ptr.To[string]("BackupFailed")
			case state.HaveToRestore:
				reason = "RestoreFailed"
				message = "Restore failed"
				run.Status.State = ptr.To[string]("RestoreFailed")
			case state.Interrupted:
				reason = "Interrupted"
				message = "Run has been interrupted and considered as failed"
				run.Status.State = ptr.To[string]("InterruptedFailed")
			default:
				reason = "Unknown"
				message = string(ct)
				run.Status.State = ptr.To[string]("Unknown")
			}
		case backupoperatoriov1.BackupRunConditionTypeSuccessful:
			inProgress = metav1.ConditionFalse
			failed = metav1.ConditionFalse
			successful = metav1.ConditionTrue
			switch {
			case state.HaveToBackup:
				reason = "BackupSuccessful"
				message = "Backup successful"
				run.Status.State = ptr.To[string]("BackupSuccessful")
			case state.HaveToRestore:
				reason = "RestoreSuccessful"
				message = "Restore successful"
				run.Status.State = ptr.To[string]("RestoreSuccessful")
			default:
				reason = "Unknown"
				message = string(ct)
				run.Status.State = ptr.To[string]("Unknown")
			}
		default:
			err = fmt.Errorf("no case to change phase to %s", string(ct))
			return
		}
		run.Status.Conditions = *utils.AddOrUpdateConditions(run.Status.Conditions,
			metav1.Condition{
				Type:               string(backupoperatoriov1.BackupRunConditionTypeInProgress),
				Status:             inProgress,
				Reason:             reason,
				Message:            message,
				LastTransitionTime: metav1.Now(),
				ObservedGeneration: run.Generation,
			},
			metav1.Condition{
				Type:               string(backupoperatoriov1.BackupRunConditionTypeFailed),
				Status:             failed,
				Reason:             reason,
				Message:            message,
				LastTransitionTime: metav1.Now(),
				ObservedGeneration: run.Generation,
			},
			metav1.Condition{
				Type:               string(backupoperatoriov1.BackupRunConditionTypeSuccessful),
				Status:             successful,
				Reason:             reason,
				Message:            message,
				LastTransitionTime: metav1.Now(),
				ObservedGeneration: run.Generation,
			},
		)
		return c.Status().Update(ctx, run)
	})
}
